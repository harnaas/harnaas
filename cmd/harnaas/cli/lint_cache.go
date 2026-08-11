package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/jsonutil"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/logging"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source/github"
)

// resolutionFreshness is how long a recorded lookup answers for.
//
// Bounded rather than indefinite because what it caches is a fact about
// somebody else's repository, which is exactly the kind of fact that changes
// without telling you. Short enough that a team notices a release the same day;
// long enough that lint in a pre-commit hook does not resolve every ref on
// every commit.
const resolutionFreshness = time.Hour

// resolutionsDir is the cache's own directory beneath harnaas's cache root,
// beside the archives the install path keeps there.
const resolutionsDir = "resolutions"

// resolutionEntry is one repository's recorded lookup.
//
// The commit and the tags are stored together because they are fetched for the
// same asset in the same run against the same remote, and splitting them would
// halve the value of a hit while doubling the files to keep coherent.
type resolutionEntry struct {
	// ResolvedAt is when the lookup happened, and is what the freshness window
	// is measured against.
	ResolvedAt time.Time `json:"resolvedAt"`

	// Commit is what the ref resolved to, and Mutable whether it can move.
	Commit  string `json:"commit"`
	Mutable bool   `json:"mutable"`

	// Tags is the repository's tag listing, empty where none was asked for.
	Tags []string `json:"tags,omitempty"`
}

// resolutionCache records what lookups answered, so a second lint run inside
// the freshness window makes no request.
//
// Nothing here can fail a run. A miss, an unreadable entry, one that will not
// parse and one past its window are a single answer — ask again — and a damaged
// entry is removed on the way out so the next run does not pay to rediscover
// it. That is the same rule the archive cache holds to, for the same reason: a
// cache that can make a run fail has cost more than it saves.
type resolutionCache struct {
	// dir is the cache's own directory; a cache with none stores and reuses
	// nothing, which is the shape --refresh uses.
	dir string
}

// newResolutionCache resolves where lookups are recorded.
//
// The precedence, documented here at its only read site, is HARNAAS_CACHE_DIR
// then the user cache directory — the same one the archive cache reads, so a
// user who redirected one has redirected both and harnaas never keeps state in
// two places one command knows about and the other does not.
func newResolutionCache() *resolutionCache {
	if override := os.Getenv(source.CacheDirEnvVar); override != "" {
		return &resolutionCache{dir: filepath.Join(override, resolutionsDir)}
	}
	base, err := os.UserCacheDir()
	if err != nil {
		// No cache directory is a cache that stores nothing, not a failure:
		// lint's job is to report findings, and it can do all of it slowly.
		return &resolutionCache{}
	}
	return &resolutionCache{dir: filepath.Join(base, "harnaas", resolutionsDir)}
}

// entryPath is where one repository's lookup is recorded.
//
// The key is hashed with each field length-prefixed, for the reason the archive
// cache's is: unframed, two different repositories can produce one key, and one
// would then be served the other's commit.
func (c *resolutionCache) entryPath(repository, ref string) string {
	if c.dir == "" {
		return ""
	}
	sum := sha256.New()
	for _, field := range []string{repository, ref} {
		_, _ = sum.Write([]byte(strconv.Itoa(len(field)) + ":" + field))
	}
	return filepath.Join(c.dir, hex.EncodeToString(sum.Sum(nil))+".json")
}

// lookup returns a recorded entry that is still fresh.
//
// An entry that cannot be read, cannot be parsed, or is past its window is
// discarded and reported as a miss. Discarding rather than leaving it is what
// keeps one corrupt write from costing every later run the same failed read.
func (c *resolutionCache) lookup(ctx context.Context, repository, ref string) (resolutionEntry, bool) {
	path := c.entryPath(repository, ref)
	if path == "" {
		return resolutionEntry{}, false
	}

	//nolint:gosec // G304: the path is harnaas's own cache directory plus a digest of the key.
	data, err := os.ReadFile(path)
	if err != nil {
		return resolutionEntry{}, false
	}

	var entry resolutionEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		c.discard(ctx, path, "unparseable")
		return resolutionEntry{}, false
	}
	if time.Since(entry.ResolvedAt) > resolutionFreshness {
		c.discard(ctx, path, "stale")
		return resolutionEntry{}, false
	}

	return entry, true
}

// record stores a lookup's answer.
//
// A write that fails is a log record and never an error, because the cache
// exists to make a run cheaper and one that can make a run fail has inverted
// its own purpose.
func (c *resolutionCache) record(ctx context.Context, repository, ref string, entry resolutionEntry) {
	path := c.entryPath(repository, ref)
	if path == "" {
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		logging.Info(ctx, "resolution cache directory unavailable", slog.String("error", err.Error()))
		return
	}

	encoded, err := jsonutil.Marshal(entry)
	if err != nil {
		return
	}
	if err := jsonutil.WriteFileAtomic(path, encoded, 0o600); err != nil {
		logging.Info(ctx, "resolution cache write failed", slog.String("error", err.Error()))
	}
}

// discard removes an entry that cannot be trusted.
func (c *resolutionCache) discard(ctx context.Context, path, reason string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logging.Info(ctx, "resolution cache entry could not be discarded",
			slog.String("reason", reason), slog.String("error", err.Error()))
		return
	}
	logging.Info(ctx, "resolution cache entry discarded", slog.String("reason", reason))
}

// cachedResolver wraps ref resolution and tag listing so a repeated run inside
// the freshness window makes no request.
//
// It returns both halves together because they are cached together: the caller
// takes the pair and uses whichever the asset needs, and a caller that took one
// would leave the other's freshness to a second code path.
func cachedResolver(cache *resolutionCache, refresh bool) (refResolver, tagLister) {
	resolve := func(ctx context.Context, req source.Request) (github.RefResolution, error) {
		if !refresh {
			if entry, hit := cache.lookup(ctx, req.Source.Repository, req.Source.Ref); hit {
				return github.RefResolution{Commit: entry.Commit, Mutable: entry.Mutable}, nil
			}
		}

		resolution, err := github.ResolveRef(ctx, req)
		if err != nil {
			// Only an answer is recorded. A failure is the run's to report and
			// the next run's to retry — caching one would make an outage
			// outlive itself.
			return resolution, fmt.Errorf("resolve %s: %w", req.Source.Repository, err)
		}

		cache.record(ctx, req.Source.Repository, req.Source.Ref, resolutionEntry{
			ResolvedAt: time.Now().UTC(), Commit: resolution.Commit, Mutable: resolution.Mutable,
		})
		return resolution, nil
	}

	listTags := func(ctx context.Context, repository string) ([]string, error) {
		const tagsKey = "\x00tags"
		if !refresh {
			if entry, hit := cache.lookup(ctx, repository, tagsKey); hit {
				return entry.Tags, nil
			}
		}

		tags, err := github.ListTags(ctx, repository)
		if err != nil {
			return nil, fmt.Errorf("list tags of %s: %w", repository, err)
		}

		cache.record(ctx, repository, tagsKey, resolutionEntry{ResolvedAt: time.Now().UTC(), Tags: tags})
		return tags, nil
	}

	return resolve, listTags
}

// authorizationRemedy is the sentence an access failure gets instead of a
// manifest edit.
//
// A repository harnaas cannot see is not a line in harnaas.json to correct —
// the manifest is right and the credential is missing — so the remedy names the
// variables a token can arrive through rather than sending the reader to edit a
// correct file.
func authorizationRemedy() string {
	return "Set a token with access to it in HARNAAS_GITHUB_TOKEN, GH_TOKEN or GITHUB_TOKEN, " +
		"then run `harnaas lint` again."
}

// isAuthorizationFailure reports whether an error is the forge declining
// access, which is a different finding from a host that could not be reached:
// one is fixed by supplying a credential and the other by waiting.
func isAuthorizationFailure(err error) bool {
	var authorization *github.AuthorizationError
	return errors.As(err, &authorization)
}
