// Package local resolves `local` sources: content this repository holds itself,
// under its own `.harnaas` directory.
//
// It is the one kind that asks nobody anything. There is no ref to resolve, no
// host to reach and no archive to unpack — the files are already here, so an
// offline run and a networked one do exactly the same work, and the run's cache
// has nothing to offer a read that costs one syscall. That is why this kind
// ignores [source.RunOptions] entirely rather than branching on it: the options
// describe requests, and this kind makes none.
//
// What it does have that no other kind does is a live filesystem. The manifest
// grammar already refused a path that leaves `.harnaas` textually, but a
// directory validated in one moment can hold a symbolic link to somewhere else
// in the next, so every read here goes through a handle anchored at `.harnaas`.
// Containment is then the kernel's answer at the moment of the read rather than
// harnaas's answer at the moment of the check — which is the difference between
// a race harnaas would usually win and one it cannot lose. Archive extraction
// checks containment textually for the opposite reason: an archive already in
// hand has no filesystem to race with.
package local

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/paths"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// init registers this kind with [source.Default], so a binary that links this
// package is a binary that resolves `local` sources. Registration lives here
// rather than in the install flow because the alternative is a list of kinds
// maintained somewhere else that a new kind has to remember to join — and the
// failure that produces is a kind which compiles, passes its own tests and is
// missing from the registry.
//
//nolint:gochecknoinits // Self-registration is the source-kind contract; see [source.Registry].
func init() {
	source.Register(manifest.SourceKindLocal, New)
}

// Kind resolves every `local` source of one install run.
//
// It holds nothing, because there is nothing about a local read worth
// remembering between assets: the memo the `github` kind keeps exists to spare a
// second request, and reading a file this repository already contains costs less
// than the bookkeeping would. It is still constructed per run, because that is
// what a kind is.
type Kind struct{}

// New returns the kind for one install run.
//
// The run's options are accepted and deliberately unused. A local source is
// satisfied from the project itself, so an offline run resolves it exactly as a
// networked one does, and there is nothing for the archive cache to hold.
// Taking the parameter anyway is what lets the registry hand every kind the same
// options without knowing which of them care.
func New(_ source.RunOptions) source.Kind { return &Kind{} }

// Resolve produces one asset's content and provenance from the project's own
// `.harnaas` directory.
func (k *Kind) Resolve(ctx context.Context, req source.Request) (*source.Resolved, error) {
	root, err := paths.ProjectRoot(ctx)
	if err != nil {
		return nil, err
	}

	location := sourceLocation(req)

	read := &reader{
		assetID: req.Asset.ID,
		base:    location,
		files:   map[string][]byte{},
	}

	anchor, err := os.OpenRoot(filepath.Join(root, manifest.LocalRoot))
	if err != nil {
		// A project with no `.harnaas` at all reaches here, and it is the same
		// problem as an asset whose own path is missing: the content the entry
		// names is not in the repository. Reporting the directory instead would
		// name a path the author never wrote.
		return nil, read.locationFailure(location, err)
	}
	// The handle is read-only, so a failure closing it changes nothing that was
	// read through it.
	defer anchor.Close()

	read.anchor = anchor
	if err := read.take(withinLocalRoot(location)); err != nil {
		return nil, err
	}

	if len(read.files) == 0 {
		return nil, &EmptySourceError{AssetID: req.Asset.ID, Path: location}
	}

	resolved, err := source.NewResolved(source.Provenance{
		Kind: manifest.SourceKindLocal,
		// Spelled the way the manifest declares it: the keyed form names its
		// `sources` entry, and the project-local form is its own path. A
		// lockfile reader should recognize the line they wrote.
		Source: normalizedSource(req),
		// A local source pins nothing and so is never mutable. Whether the file
		// changed is a question the digests answer, and a different one.
	}, read.files)
	if err != nil {
		return nil, &ReadError{AssetID: req.Asset.ID, Path: location, Err: err}
	}

	return resolved, nil
}

// sourceLocation is where the asset's content sits, relative to the project
// root and still beginning `.harnaas/`.
//
// The two accepted forms arrive spelled differently and mean the same thing: the
// project-local form carries the whole path on the asset, and the keyed form
// splits it between a `sources` entry naming a directory and a path within it.
// Joining them here is what keeps every read below indifferent to which form the
// author used.
func sourceLocation(req source.Request) string {
	if req.Asset.Ref.Local() {
		return req.Asset.Ref.Path
	}
	return path.Join(req.Source.Repository, req.Asset.Ref.Path)
}

// normalizedSource renders the source the way the lockfile records it.
func normalizedSource(req source.Request) string {
	if req.Asset.Ref.Local() {
		return req.Asset.Ref.Path
	}
	return req.Source.String()
}

// withinLocalRoot re-spells a project-root-relative location as one relative to
// the anchor, which is what every method on the handle takes.
func withinLocalRoot(location string) string {
	within := strings.TrimPrefix(location, manifest.LocalRoot+"/")
	if within == "" || within == manifest.LocalRoot {
		// The grammar refuses a source that is `.harnaas` itself, so this is
		// unreachable from a manifest; spelling it as the anchor's own directory
		// keeps a hand-built request from producing a path no method accepts.
		return "."
	}
	return within
}

// reader accumulates one asset's files from the anchored handle.
//
// It carries the project-root-relative base as well as the handle because the
// two spellings serve different readers: every read is made relative to
// `.harnaas` so it cannot escape, and every diagnostic names the path relative
// to the project root, which is the path the manifest wrote and the one the
// author will go looking for.
type reader struct {
	anchor  *os.Root
	assetID string
	base    string

	files map[string][]byte
}

// take reads the source the asset names, whichever shape it turns out to have.
//
// A single file resolves under its own leaf name, for the reason archive
// extraction gives: the path relative to a file is nothing, and a resolved file
// with no path could not be told from one that was not there.
func (r *reader) take(within string) error {
	info, err := r.anchor.Stat(within)
	if err != nil {
		return r.locationFailure(r.base, err)
	}

	switch {
	case info.Mode().IsRegular():
		content, err := r.anchor.ReadFile(within)
		if err != nil {
			return r.locationFailure(r.base, err)
		}
		r.files[path.Base(within)] = content
		return nil

	case info.IsDir():
		return r.walk(within, "")

	default:
		return &UnsupportedEntryError{AssetID: r.assetID, Path: r.base, EntryKind: describeMode(info.Mode())}
	}
}

// walk reads every file under a directory the asset named, keyed by its path
// relative to that directory.
func (r *reader) walk(within, relative string) error {
	entries, err := fs.ReadDir(r.anchor.FS(), within)
	if err != nil {
		return r.locationFailure(r.projectPath(relative), err)
	}

	for _, entry := range entries {
		childWithin := path.Join(within, entry.Name())
		childRelative := path.Join(relative, entry.Name())

		// Lstat rather than the directory entry's own type, and rather than
		// Stat: what a link points at is not what harnaas installs, and
		// following one here would let a link back up the tree spin the walk
		// forever. The asset's own path is the one place a link is followed,
		// where the anchor bounds where it can lead.
		info, err := r.anchor.Lstat(childWithin)
		if err != nil {
			return r.locationFailure(r.projectPath(childRelative), err)
		}

		switch {
		case info.IsDir():
			if err := r.walk(childWithin, childRelative); err != nil {
				return err
			}

		case info.Mode().IsRegular():
			content, err := r.anchor.ReadFile(childWithin)
			if err != nil {
				return r.locationFailure(r.projectPath(childRelative), err)
			}
			r.files[childRelative] = content

		default:
			return &UnsupportedEntryError{
				AssetID:   r.assetID,
				Path:      r.projectPath(childRelative),
				EntryKind: describeMode(info.Mode()),
			}
		}
	}

	return nil
}

// projectPath renders a path within the asset's source as one relative to the
// project root, which is how every diagnostic here names a file.
func (r *reader) projectPath(relative string) string {
	if relative == "" {
		return r.base
	}
	return path.Join(r.base, relative)
}

// locationFailure turns a refusal from the anchored handle into the diagnostic
// it is.
//
// Three readings are possible and they need three different edits: the content
// is not there, the path led out of `.harnaas`, or the machine would not let
// harnaas read it. Reporting the second as the third would tell an author to
// check permissions on a file that is fine, and reporting the third as the
// second would accuse them of an escape they never wrote.
func (r *reader) locationFailure(projectPath string, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return &MissingSourceError{AssetID: r.assetID, Path: projectPath}
	case escapesAnchor(err):
		return &ContainmentError{AssetID: r.assetID, Path: projectPath, Err: err}
	default:
		return &ReadError{AssetID: r.assetID, Path: projectPath, Err: err}
	}
}

// pathEscapesMessage is the standard library's own wording for a path that
// leaves an anchored root.
//
// Matching the message is not the way anybody would choose to recognize an
// error, and it is the only way available: os keeps the sentinel unexported, so
// there is nothing to hand errors.Is. The fallback is what makes it safe — a
// wording that changes reports the refusal as a read that failed, which is less
// specific than it could be and never wrong. The same trade is already made
// where encoding/json reports an unknown field.
const pathEscapesMessage = "path escapes from parent"

// escapesAnchor reports whether the handle refused a path for leading outside
// the directory it is anchored at.
func escapesAnchor(err error) bool {
	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) || pathErr.Err == nil {
		return false
	}
	return pathErr.Err.Error() == pathEscapesMessage
}

// describeMode names what was found where a file was expected, in the words the
// message uses. It is the local counterpart of the archive's entry kinds, and
// deliberately reads the same, because a reader meeting one of these has met the
// same problem whichever kind produced it.
func describeMode(mode fs.FileMode) string {
	switch {
	case mode&fs.ModeSymlink != 0:
		return "a symbolic link"
	case mode&(fs.ModeDevice|fs.ModeCharDevice) != 0:
		return "a device"
	case mode&fs.ModeNamedPipe != 0:
		return "a named pipe"
	case mode&fs.ModeSocket != 0:
		return "a socket"
	default:
		return "an entry that is not a file"
	}
}
