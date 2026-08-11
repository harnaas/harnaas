package source

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/jsonutil"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/logging"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// The archive cache: harnaas's memory of what it has already fetched, kept
// between runs rather than only within one.
//
// The in-run memo a kind holds answers "have I fetched this during this
// install?". This answers the question a developer's second run and a CI job's
// warm workspace both ask — "has this machine fetched this before?" — and it is
// the whole reason offline resolution can exist at all: a run with no network
// can only be served from something a previous run left behind.
//
// It lives beside the source contract rather than inside the one kind that
// fetches today, for the reason [Fetcher] does. A forge added later must not be
// able to keep its own store under its own rules, and the structural way to
// guarantee that is for there to be one store and no exported way to build a
// second kind of one.
//
// # Why a token is not part of the key
//
// The archive of a commit is the same bytes whoever fetched it, so the
// credential is an access decision and not a content one, and folding it into
// the key would fetch one repository twice for a run that changed tokens. What
// that decision does mean is that an entry is readable by whoever can read the
// directory it is in — which is why the default location is under the user's
// own cache directory with owner-only permissions. The person who fetched it
// had access at the time; nobody else on the machine gains any. A
// HARNAAS_CACHE_DIR pointing somewhere shared is the same trade made
// deliberately by the person who set it.

// CacheDirEnvVar overrides where harnaas keeps content it has fetched. It is
// read at one site, [NewArchiveCache], which documents the precedence.
const CacheDirEnvVar = "HARNAAS_CACHE_DIR"

// cacheDirName is harnaas's own directory under the user's cache directory, and
// archivesDir the part of it this file owns. The override names harnaas's cache
// root directly, so only the default grows the first component.
const (
	cacheDirName = "harnaas"
	archivesDir  = "archives"

	// blobsDir holds archives named by their own digest, and refsDir the
	// pointers from a repository and commit to one of them. The split is what
	// makes "verify the entry against its expected digest" a comparison against
	// the name the bytes are filed under, rather than against a second fact
	// recorded beside them that could drift out of step with the first.
	blobsDir = "blobs"
	refsDir  = "refs"
)

// hexDigits is the length of a sha256 sum in hexadecimal. A pointer's contents
// are checked against it before they are joined into a path, because a file on
// disk is untrusted input in the same class as an archive entry name: a corrupt
// or hand-edited pointer reading `../../..` must be a miss, not a read.
const hexDigits = 64

// ArchiveKey identifies one cached archive: the kind that fetched it, the
// repository it is of and the commit its content is at.
//
// The kind participates so two forges cannot file different content under one
// `owner/name`, and the commit rather than the ref because a ref is what
// resolution has already spent — an entry named by a moving target would be the
// stale content the offline rules exist to refuse.
type ArchiveKey struct {
	Kind       manifest.SourceKind
	Repository string
	Commit     string
}

// ArchiveCache is the content harnaas has fetched on this machine before.
//
// A nil cache stores and reuses nothing. That is the caller-facing bypass: a
// run that must not read what an earlier one left behind is handed no cache
// rather than handed a cache and asked to remember not to use it.
type ArchiveCache struct {
	// dir is harnaas's cache root. An empty one means no location could be
	// resolved, which behaves exactly as the bypass does — a machine with
	// nowhere to keep a cache still installs, one fetch at a time.
	dir string
}

// NewArchiveCache returns the cache for one run.
//
// Precedence, documented here at its only read site: HARNAAS_CACHE_DIR, then
// harnaas's own directory under the user's cache directory. Neither being
// available is not a failure — it is a run that fetches everything it needs,
// which is what the first run on any machine does anyway.
func NewArchiveCache() *ArchiveCache {
	if override := strings.TrimSpace(os.Getenv(CacheDirEnvVar)); override != "" {
		return newArchiveCache(override)
	}

	cache, err := os.UserCacheDir()
	if err != nil {
		return newArchiveCache("")
	}
	return newArchiveCache(filepath.Join(cache, cacheDirName))
}

// newArchiveCache is [NewArchiveCache] with the location as a parameter, so the
// store's own rules are exercisable without moving an environment variable —
// and so the tests that do move one are only the two about where the location
// comes from.
func newArchiveCache(dir string) *ArchiveCache {
	return &ArchiveCache{dir: dir}
}

// Lookup returns the archive stored for key, if this machine has one that still
// matches the digest it was filed under.
//
// Every way of not having it is the same answer: absent, unreadable, pointing
// at nothing, or no longer hashing to its own name all report a miss, and the
// caller fetches. A cache is an optimization, and an optimization that can fail
// a run is a liability — so nothing here returns an error, and a damaged entry
// is discarded on the way out so the next run does not pay to rediscover it.
func (c *ArchiveCache) Lookup(ctx context.Context, key ArchiveKey) ([]byte, bool) {
	if c == nil || c.dir == "" {
		return nil, false
	}

	recorded, err := os.ReadFile(c.refPath(key))
	if err != nil {
		return nil, false
	}

	sum := strings.TrimSpace(string(recorded))
	if !isHexSum(sum) {
		c.discard(ctx, key, "", "the cache pointer does not name a digest")
		return nil, false
	}

	// sum is checked to be a hexadecimal digest above, so what it can name is a
	// file in the blob directory and nothing else.
	body, err := os.ReadFile(c.blobPath(sum))
	if err != nil {
		c.discard(ctx, key, sum, "the cached archive could not be read")
		return nil, false
	}

	if DigestContent(body) != Digest(digestPrefix+sum) {
		c.discard(ctx, key, sum, "the cached archive no longer matches its digest")
		return nil, false
	}

	return body, true
}

// Store files an archive under key so a later run can reuse it.
//
// It reports nothing, for the reason [Lookup] returns no error: a machine that
// cannot write its cache is a machine that fetches again, and turning that into
// a failed install would be the cache costing a user the run it exists to make
// cheaper. What a failure does get is a log record, because otherwise the only
// evidence is a run that is inexplicably slow every time.
//
// The blob is written before the pointer, so an interruption leaves an
// unreferenced archive rather than a pointer to one that is not there.
func (c *ArchiveCache) Store(ctx context.Context, key ArchiveKey, body []byte) {
	if c == nil || c.dir == "" {
		return
	}

	sum := strings.TrimPrefix(string(DigestContent(body)), digestPrefix)

	for _, dir := range []string{c.blobDir(), c.refDir()} {
		// 0700: the cache records which repositories this user has fetched,
		// which is nobody else's business on a shared machine.
		if err := os.MkdirAll(dir, 0o700); err != nil {
			c.storeFailed(ctx, key, err)
			return
		}
	}

	// Atomically, through the module's one atomic writer — which lives in
	// jsonutil because that is where the first caller needed it, and takes bytes
	// rather than a document. A half-written blob under a name that promises its
	// digest is precisely the entry verification cannot catch cheaply: a reader
	// would hash it, find a mismatch and discard content that was fine.
	if err := jsonutil.WriteFileAtomic(c.blobPath(sum), body, 0o600); err != nil {
		c.storeFailed(ctx, key, err)
		return
	}
	if err := jsonutil.WriteFileAtomic(c.refPath(key), []byte(sum+"\n"), 0o600); err != nil {
		c.storeFailed(ctx, key, err)
		return
	}

	logging.Debug(ctx, "cached a fetched archive", c.attrs(key)...)
}

// discard removes an entry that could not be used, so the next run meets a
// clean miss rather than the same damaged files.
//
// Both halves go, and best-effort: removing only the pointer would leave a blob
// nothing names, and a removal that fails has already been reported as a miss —
// there is nothing left for an error to change.
func (c *ArchiveCache) discard(ctx context.Context, key ArchiveKey, sum, reason string) {
	_ = os.Remove(c.refPath(key))
	if sum != "" {
		_ = os.Remove(c.blobPath(sum))
	}

	logging.Warn(ctx, "discarded a cache entry and will fetch again",
		append(c.attrs(key), slog.String("reason", reason))...)
}

// storeFailed records a cache write that did not happen.
//
// The cause is a filesystem error, so what it carries is a path and a reason —
// both things the logging rule allows, and neither of them any of the content
// the write was going to store.
func (c *ArchiveCache) storeFailed(ctx context.Context, key ArchiveKey, err error) {
	logging.Warn(ctx, "could not cache a fetched archive",
		append(c.attrs(key), slog.String("error", err.Error()))...)
}

// attrs names an entry in a log record: identifiers and a directory, never
// content.
func (c *ArchiveCache) attrs(key ArchiveKey) []slog.Attr {
	return []slog.Attr{
		slog.String("kind", string(key.Kind)),
		slog.String("repository", key.Repository),
		slog.String("commit", key.Commit),
		slog.String("cache_dir", c.dir),
	}
}

// blobDir and refDir are the two halves of the archive cache.
func (c *ArchiveCache) blobDir() string { return filepath.Join(c.dir, archivesDir, blobsDir) }
func (c *ArchiveCache) refDir() string  { return filepath.Join(c.dir, archivesDir, refsDir) }

// blobPath is where an archive with a given digest is filed. The name is the
// digest itself in hexadecimal — not the [Digest] rendering, whose algorithm
// prefix carries a colon that Windows will not accept in a file name.
func (c *ArchiveCache) blobPath(sum string) string {
	return filepath.Join(c.blobDir(), sum)
}

// refPath is where the pointer for one repository and commit lives.
//
// The name is a digest of the key rather than the key itself, because a
// repository contains a separator and a commit is somebody else's text: hashing
// gives one flat, portable, fixed-length name and no question about what a
// forge is allowed to call itself.
func (c *ArchiveCache) refPath(key ArchiveKey) string {
	sum := strings.TrimPrefix(string(DigestContent(keyBytes(key))), digestPrefix)
	return filepath.Join(c.refDir(), sum)
}

// keyBytes is the serialization a pointer's name is the digest of.
//
// Each field is length-prefixed for the reason a resolved source's paths are:
// without a frame a repository ending in one separator and a commit beginning
// with another can serialize identically to a different pair, and two
// repositories sharing one entry is the cache handing an asset another
// repository's content.
func keyBytes(key ArchiveKey) []byte {
	var canonical bytes.Buffer
	for _, field := range []string{string(key.Kind), key.Repository, key.Commit} {
		fmt.Fprintf(&canonical, "%d %s\n", len(field), field)
	}
	return canonical.Bytes()
}

// isHexSum reports whether s is a sha256 sum in hexadecimal, which is the only
// thing a pointer may contain and the only thing that may become part of a
// path.
func isHexSum(s string) bool {
	if len(s) != hexDigits {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f':
		default:
			return false
		}
	}
	return true
}
