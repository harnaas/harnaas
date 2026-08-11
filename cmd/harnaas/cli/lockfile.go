package cli

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/jsonutil"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// LockFileName is the lockfile's only accepted name, beside the manifest at the
// project root.
//
// It is committed. An uncommitted lockfile would make ownership a per-machine
// fact, so a teammate's first install would report every already-installed file
// as an unmanaged conflict — and `lint` could not be a CI gate, because there
// would be nothing for it to check against.
const LockFileName = "harnaas.lock.json"

// SupportedLockVersion is the lockfile schema version this binary writes.
const SupportedLockVersion = 1

// lockFilePerm is the mode the lockfile is created with: a committed file that
// no one hand-edits, so the ordinary non-executable default.
const lockFilePerm fs.FileMode = 0o644

// lockDocument is the whole lockfile: what was installed, from where, and what
// landed on disk.
//
// The manifest configures intent and this records facts. Nothing here is
// configuration — there is no field a user would want to change, which is what
// keeps "delete it and re-install" a safe instruction rather than a way to lose
// a setting.
type lockDocument struct {
	// Version is the schema version. It is decoded leniently and on its own, so
	// a lockfile written by a newer harnaas produces "upgrade" rather than a
	// complaint about a field this binary happens not to know.
	Version int `json:"version"`

	// Assets is every recorded asset, sorted by id.
	Assets []lockAsset `json:"assets"`
}

// lockAsset is one asset's provenance and everywhere it was installed.
type lockAsset struct {
	// ID and Type identify the asset the way the manifest does.
	ID   string             `json:"id"`
	Type manifest.AssetType `json:"type"`

	// Source is the normalized source, so a recorded installation names its
	// origin without the reader consulting the manifest current at the time.
	Source string `json:"source"`

	// RequestedRef is the ref the manifest asked for, kept after resolution and
	// never collapsed into the commit. A lockfile holding only the commit could
	// say whether the installed files still match it, but not whether the tag
	// the manifest asked for now points somewhere else — which is the whole of
	// update detection.
	RequestedRef string `json:"requestedRef,omitempty"`

	// ResolvedCommit is the immutable commit the request resolved to. Empty for
	// a kind whose content has no commit, which is every local source.
	ResolvedCommit string `json:"resolvedCommit,omitempty"`

	// SourceDigest covers the content as it was resolved, and answers "has
	// upstream moved?". It is deliberately independent of every installation's
	// InstalledDigest below, which answers "did somebody edit this?".
	SourceDigest source.Digest `json:"sourceDigest"`

	// InstalledAt is when this asset's installation last changed — not when
	// install last ran.
	//
	// The distinction is load-bearing. An install that finds everything already
	// current must produce a byte-identical lockfile, so a run that changed
	// nothing carries the recorded time forward rather than stamping the moment
	// it ran. A timestamp that moved on every run would make the lockfile a
	// permanent source of diff noise and would make "nothing changed"
	// unprovable.
	InstalledAt time.Time `json:"installedAt"`

	// Installations is one record per target this asset was installed for,
	// sorted by harness, then scope, then destination.
	Installations []lockInstallation `json:"installations"`
}

// lockInstallation is one asset landing for one harness.
type lockInstallation struct {
	// Harness is which harness this destination was resolved for.
	//
	// It is attribution and not exclusive ownership. Shared targets mean one
	// file legitimately serves several harnesses, so the same destination
	// appears under each of them, and convergence removes it only when no
	// recorded harness claims it any more.
	Harness harness.ID `json:"harness"`

	// Scope names the root Destination is relative to.
	Scope manifest.Scope `json:"scope"`

	// Destination is relative to the scope's own root, slash-separated, and
	// never absolute.
	//
	// Relative because the lockfile is committed: a user-scoped entry holding
	// one developer's home directory would be a path nobody else's machine has,
	// in a file everybody shares.
	Destination string `json:"destination"`

	// Renderer names how the source content became the installed bytes, and
	// Emulated records that this landed through another type's surface.
	//
	// Both are written so a later run and `lint` can tell an emulated
	// installation from a native one without re-deriving it: the outcome an
	// emulated install reports is not `created` or `updated`, and a run that
	// could not remember which it was would report the wrong one.
	Renderer string `json:"renderer,omitempty"`
	Emulated bool   `json:"emulated,omitempty"`

	// Files is a digest per installed file, keyed by its path relative to
	// Destination and sorted by that path.
	//
	// A slice rather than a map, even though "keyed by path" describes a map,
	// because this order is also the order `lint` reports in and the order the
	// installed digest is computed over. Relying on encoding/json's key sorting
	// would make determinism a property of the encoder rather than of the
	// value.
	Files []lockFile `json:"files"`

	// InstalledDigest covers the bytes that landed, and answers "did somebody
	// edit this?".
	//
	// It differs from the asset's SourceDigest whenever a renderer was not the
	// identity, and that difference is never drift — which is exactly why the
	// two are recorded separately rather than one being derived from the other.
	InstalledDigest source.Digest `json:"installedDigest"`
}

// lockFile is one installed file's path and digest.
type lockFile struct {
	Path   string        `json:"path"`
	Digest source.Digest `json:"digest"`
}

// newLockDocument builds a document from recorded assets, ordered so identical
// state produces a byte-identical file.
func newLockDocument(assets []lockAsset) *lockDocument {
	document := &lockDocument{Version: SupportedLockVersion, Assets: slices.Clone(assets)}
	document.sort()
	return document
}

// sort puts the document in its canonical order.
//
// Every level is ordered by data rather than by the order a caller happened to
// build it in, because the lockfile is committed and a diff that moved a
// hundred lines because install processed two assets in a different order is a
// diff nobody reads.
func (d *lockDocument) sort() {
	slices.SortStableFunc(d.Assets, func(a, b lockAsset) int {
		if byID := cmp.Compare(a.ID, b.ID); byID != 0 {
			return byID
		}
		// Two assets may share an id across types, because each type is its own
		// namespace to the harness.
		return cmp.Compare(string(a.Type), string(b.Type))
	})

	for i := range d.Assets {
		asset := &d.Assets[i]
		slices.SortStableFunc(asset.Installations, func(a, b lockInstallation) int {
			if byHarness := cmp.Compare(string(a.Harness), string(b.Harness)); byHarness != 0 {
				return byHarness
			}
			if byScope := cmp.Compare(string(a.Scope), string(b.Scope)); byScope != 0 {
				return byScope
			}
			return cmp.Compare(a.Destination, b.Destination)
		})
		for j := range asset.Installations {
			slices.SortStableFunc(asset.Installations[j].Files, func(a, b lockFile) int {
				return cmp.Compare(a.Path, b.Path)
			})
		}
	}
}

// destinationKey identifies an installed destination independently of which
// harness it was resolved for.
//
// Scope is part of it because the same relative path means two different places
// beneath two different roots.
type destinationKey struct {
	Scope       manifest.Scope
	Destination string
}

// claimants reports which harnesses the lockfile records for each destination.
//
// This is what makes the harness field attribution rather than ownership: a
// destination several harnesses read is removed only once none of them claims
// it, and a caller that asked "is this managed?" per harness would delete a
// file another harness still needs.
func (d *lockDocument) claimants() map[destinationKey][]harness.ID {
	claims := make(map[destinationKey][]harness.ID)
	for _, asset := range d.Assets {
		for _, installation := range asset.Installations {
			key := destinationKey{Scope: installation.Scope, Destination: installation.Destination}
			if !slices.Contains(claims[key], installation.Harness) {
				claims[key] = append(claims[key], installation.Harness)
			}
		}
	}
	for key := range claims {
		slices.Sort(claims[key])
	}
	return claims
}

// lockVersionError reports a lockfile this binary cannot interpret.
//
// It is its own type because the remedy is unlike every other lockfile failure:
// nothing in the project is wrong, and no edit to it would help.
type lockVersionError struct {
	// Path is the lockfile that was read, and Found the version it declared.
	Path  string
	Found int
}

func (e *lockVersionError) Error() string {
	return fmt.Sprintf(
		"%s declares version %d, which this harnaas does not understand (it understands %d)\n\n"+
			"Upgrade harnaas to a version that reads it. Do not delete the lockfile: "+
			"it is what records which installed files harnaas owns, and without it every one of them "+
			"becomes unmanaged and is never updated again.",
		e.Path, e.Found, SupportedLockVersion,
	)
}

// lockDecodeError reports a lockfile that will not parse.
//
// The message says what a reader most needs to know at that moment, which is
// that nothing has been touched: a parse failure stops the run before any
// destination is read or written, precisely because the file that says what
// harnaas owns is the one thing it cannot proceed without.
type lockDecodeError struct {
	Path string
	Err  error
}

func (e *lockDecodeError) Error() string {
	return fmt.Sprintf(
		"%s could not be read: %v\n\n"+
			"Nothing was installed, removed or overwritten. Restore the file from version control; "+
			"if it cannot be restored, deleting it makes every installed file unmanaged rather than "+
			"removing anything.",
		e.Path, e.Err,
	)
}

func (e *lockDecodeError) Unwrap() error { return e.Err }

// lockVersionProbe reads nothing but the version, leniently.
//
// The version is read on its own and before anything else for the reason the
// manifest's is: a lockfile written by a newer harnaas carries fields this
// binary does not know, and reporting one of them would send the reader looking
// for a problem in a correct file. Read first, the same file produces the
// message that helps.
type lockVersionProbe struct {
	Version int `json:"version"`
}

// decodeLock parses lockfile bytes.
//
// Decoding is lenient — unknown fields are ignored — which is the opposite of
// the manifest's rule and follows the same test: who writes the file. The
// lockfile is machine-written and committed, so every version of harnaas on a
// team rewrites it, and a strict reader would let a newer binary brick the file
// for everyone still on an older one.
func decodeLock(data []byte, path string) (*lockDocument, error) {
	var probe lockVersionProbe
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, &lockDecodeError{Path: path, Err: err}
	}
	if probe.Version > SupportedLockVersion {
		return nil, &lockVersionError{Path: path, Found: probe.Version}
	}

	var document lockDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, &lockDecodeError{Path: path, Err: err}
	}

	document.sort()
	return &document, nil
}

// lockPath is where the lockfile lives for a project root.
func lockPath(root string) string {
	return filepath.Join(root, LockFileName)
}

// loadLock reads the lockfile, reporting an absent one as a document recording
// nothing.
//
// An absent lockfile is "nothing is managed" rather than an error, and that is
// the protective reading: every destination already on disk is then unmanaged,
// so a first install against a project full of hand-written harness files
// reports conflicts instead of taking them over. It is also what makes deleting
// the lockfile lose ownership rather than orphan files.
func loadLock(root string) (*lockDocument, error) {
	path := lockPath(root)

	//nolint:gosec // G304: the path is the project root harnaas resolved plus the lockfile's fixed name, opened for reading only.
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &lockDocument{Version: SupportedLockVersion}, nil
	}
	if err != nil {
		return nil, &lockDecodeError{Path: path, Err: err}
	}

	return decodeLock(data, path)
}

// saveLock writes the lockfile atomically.
//
// The document is encoded in full before the write begins, so a value that
// cannot be encoded fails with the previous lockfile untouched — which matters
// more here than anywhere else, since a truncated lockfile is a project whose
// installed files have all become unmanaged.
func saveLock(root string, document *lockDocument) error {
	document.sort()
	document.Version = SupportedLockVersion

	encoded, err := jsonutil.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode %s: %w", LockFileName, err)
	}
	if err := jsonutil.WriteFileAtomic(lockPath(root), encoded, lockFilePerm); err != nil {
		return fmt.Errorf("write %s: %w", LockFileName, err)
	}
	return nil
}

// recordedSource renders a source string for the lockfile with any credential
// removed.
//
// Nothing on today's roster puts a credential in a source string — a GitHub
// source is an owner, a repository and a ref — but the lockfile is committed,
// and a redaction that runs only where a credential is expected is one forge
// away from not running where one arrives. Redacting on the way in makes it a
// property of what is written rather than of who wrote it.
func recordedSource(raw string) string {
	if !strings.Contains(raw, "://") {
		return raw
	}
	return source.RedactURL(raw)
}

// recordedDestination renders a destination for the lockfile, refusing one that
// is not relative.
//
// The refusal is a programming error rather than anything a user did, so it
// reads as one: an absolute path here would be a committed file naming one
// machine's directory layout, which is the failure storing destinations
// relative to their scope root exists to prevent.
func recordedDestination(destination string) (string, error) {
	cleaned := path.Clean(filepath.ToSlash(destination))
	if filepath.IsAbs(destination) || strings.HasPrefix(cleaned, "/") || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf(
			"refusing to record the destination %q, which is not relative to its scope root: "+
				"the lockfile is committed and must mean the same thing on every machine",
			destination,
		)
	}
	return cleaned, nil
}
