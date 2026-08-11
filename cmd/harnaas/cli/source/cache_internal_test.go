package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// The archive cache's rules, exercised against a location handed in rather than
// read from the environment. Only the two tests about where that location comes
// from move a variable, and they are the only ones here that cannot run in
// parallel.

// cachedCommit is the commit every entry in this file is of.
const cachedCommit = "9f2a1c4e8b7d6a5f4e3c2b1a0987654321fedcba"

// assetKey is one repository's archive at that commit.
func assetKey() ArchiveKey {
	return ArchiveKey{
		Kind:       manifest.SourceKindGitHub,
		Repository: "acme/assets",
		Commit:     cachedCommit,
	}
}

// archiveBody stands in for a repository archive. Nothing here decompresses it:
// the cache stores bytes and verifies bytes, and what they are is the extractor's
// question.
var archiveBody = []byte("the archive of acme/assets at " + cachedCommit)

func TestAStoredArchiveIsReusedByALaterRun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	newArchiveCache(dir).Store(t.Context(), assetKey(), archiveBody)

	// A second cache over the same location is what a second run is: the memo a
	// kind holds is gone, and only what reached the disk can answer.
	body, cached := newArchiveCache(dir).Lookup(t.Context(), assetKey())

	require.True(t, cached, "the second run reuses what the first one fetched")
	assert.Equal(t, archiveBody, body)
}

func TestAnEmptyCacheIsAMiss(t *testing.T) {
	t.Parallel()

	body, cached := newArchiveCache(t.TempDir()).Lookup(t.Context(), assetKey())

	assert.False(t, cached)
	assert.Nil(t, body)
}

// TestAnEntryIsKeyedByKindRepositoryAndCommit is the rule that keeps a cache
// from handing an asset somebody else's content — and, for the commit, the rule
// that stops a moving ref from being served stale.
func TestAnEntryIsKeyedByKindRepositoryAndCommit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		other ArchiveKey
	}{
		{name: "another kind", other: ArchiveKey{Kind: "gitlab", Repository: "acme/assets", Commit: cachedCommit}},
		{name: "another repository", other: ArchiveKey{Kind: manifest.SourceKindGitHub, Repository: "acme/other", Commit: cachedCommit}},
		{name: "another commit", other: ArchiveKey{Kind: manifest.SourceKindGitHub, Repository: "acme/assets", Commit: "0123456789abcdef0123456789abcdef01234567"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cache := newArchiveCache(t.TempDir())
			cache.Store(t.Context(), assetKey(), archiveBody)

			_, cached := cache.Lookup(t.Context(), tc.other)
			assert.False(t, cached)
		})
	}
}

// TestKeysAreFramedBeforeTheyAreHashed proves the length prefixes are doing
// something: without them these two keys concatenate to the same bytes, and one
// repository's archive would be served for the other.
func TestKeysAreFramedBeforeTheyAreHashed(t *testing.T) {
	t.Parallel()

	cache := newArchiveCache(t.TempDir())

	first := cache.refPath(ArchiveKey{Kind: "github", Repository: "acme/assets", Commit: "a"})
	second := cache.refPath(ArchiveKey{Kind: "github", Repository: "acme/asset", Commit: "sa"})

	assert.NotEqual(t, first, second)
}

// TestACorruptEntryIsDiscardedRatherThanReturned is the rule that keeps a cache
// from being a way to install content nobody published: the bytes are verified
// against the digest they are filed under before they are handed back.
func TestACorruptEntryIsDiscardedRatherThanReturned(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cache := newArchiveCache(dir)
	cache.Store(t.Context(), assetKey(), archiveBody)

	blobs := entriesOf(t, filepath.Join(dir, archivesDir, blobsDir))
	require.Len(t, blobs, 1)
	require.NoError(t, os.WriteFile(blobs[0], []byte("something else entirely"), 0o600))

	body, cached := cache.Lookup(t.Context(), assetKey())
	assert.False(t, cached, "content that no longer hashes to its own name is not content harnaas fetched")
	assert.Nil(t, body)

	assert.Empty(t, entriesOf(t, filepath.Join(dir, archivesDir, refsDir)),
		"the entry is discarded, so the next run meets a clean miss rather than the same damaged files")
	assert.Empty(t, entriesOf(t, filepath.Join(dir, archivesDir, blobsDir)))
}

// TestADiscardedEntryIsReplacedByTheNextStore is the second half of "discard and
// re-fetch": the run that met the damage goes on to succeed.
func TestADiscardedEntryIsReplacedByTheNextStore(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cache := newArchiveCache(dir)
	cache.Store(t.Context(), assetKey(), []byte("a truncated download"))

	blobs := entriesOf(t, filepath.Join(dir, archivesDir, blobsDir))
	require.NoError(t, os.WriteFile(blobs[0], []byte("corrupted"), 0o600))
	_, cached := cache.Lookup(t.Context(), assetKey())
	require.False(t, cached)

	cache.Store(t.Context(), assetKey(), archiveBody)

	body, cached := cache.Lookup(t.Context(), assetKey())
	require.True(t, cached)
	assert.Equal(t, archiveBody, body)
}

func TestAnEntryPointingAtNothingIsAMiss(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cache := newArchiveCache(dir)
	cache.Store(t.Context(), assetKey(), archiveBody)

	for _, blob := range entriesOf(t, filepath.Join(dir, archivesDir, blobsDir)) {
		require.NoError(t, os.Remove(blob))
	}

	_, cached := cache.Lookup(t.Context(), assetKey())
	assert.False(t, cached, "an unreadable entry is a miss, never a failed run")
	assert.Empty(t, entriesOf(t, filepath.Join(dir, archivesDir, refsDir)))
}

// TestAPointerThatDoesNotNameADigestIsRefused is why the pointer's contents are
// checked before they are joined into a path: a file on disk is untrusted input,
// and one naming a path outside the blob directory must be a miss and not a
// read.
func TestAPointerThatDoesNotNameADigestIsRefused(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cache := newArchiveCache(dir)
	cache.Store(t.Context(), assetKey(), archiveBody)

	elsewhere := filepath.Join(dir, "elsewhere")
	require.NoError(t, os.WriteFile(elsewhere, []byte("not an archive harnaas fetched"), 0o600))

	for _, pointer := range []string{
		"../../elsewhere",
		filepath.ToSlash(elsewhere),
		"",
		"not-a-digest",
		// A hexadecimal sum of the wrong length is still not a sha256 sum.
		"9f2a1c4e",
	} {
		require.NoError(t, os.WriteFile(cache.refPath(assetKey()), []byte(pointer), 0o600))

		body, cached := cache.Lookup(t.Context(), assetKey())
		assert.False(t, cached, "pointer %q", pointer)
		assert.Nil(t, body)
	}

	remaining, err := os.ReadFile(elsewhere)
	require.NoError(t, err)
	assert.Equal(t, "not an archive harnaas fetched", string(remaining),
		"the file the pointer named was never opened, let alone removed")
}

// TestABypassedCacheStoresAndReusesNothing is the caller-facing bypass: a run
// handed no cache reads nothing an earlier run left and leaves nothing for a
// later one.
func TestABypassedCacheStoresAndReusesNothing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	var bypassed *ArchiveCache
	bypassed.Store(t.Context(), assetKey(), archiveBody)

	body, cached := bypassed.Lookup(t.Context(), assetKey())
	assert.False(t, cached)
	assert.Nil(t, body)

	assert.Empty(t, entriesOf(t, dir), "a bypassed run leaves nothing behind for the next one to find")
}

// TestACacheWithNowhereToLiveIsInert covers the machine the standard library
// cannot name a cache directory for: it installs, one fetch at a time, rather
// than failing.
func TestACacheWithNowhereToLiveIsInert(t *testing.T) {
	t.Parallel()

	cache := newArchiveCache("")
	cache.Store(t.Context(), assetKey(), archiveBody)

	_, cached := cache.Lookup(t.Context(), assetKey())
	assert.False(t, cached)
}

func TestTheCacheLocationOverrideIsHonoured(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(CacheDirEnvVar, dir)

	NewArchiveCache().Store(t.Context(), assetKey(), archiveBody)

	assert.Len(t, entriesOf(t, filepath.Join(dir, archivesDir, blobsDir)), 1)

	userCache, err := os.UserCacheDir()
	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(userCache, cacheDirName, archivesDir),
		"the override replaces the default location rather than adding to it")
}

func TestTheDefaultLocationIsUnderTheUsersOwnCacheDirectory(t *testing.T) {
	t.Setenv(CacheDirEnvVar, "")

	userCache, err := os.UserCacheDir()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(userCache, cacheDirName), NewArchiveCache().dir,
		"harnaas keeps what it fetched outside the team's working tree, like everything else that is not the manifest")
}

// entriesOf returns the full paths of the files in dir, and no paths at all
// where dir does not exist — which is one of the outcomes these tests assert.
func entriesOf(tb testing.TB, dir string) []string {
	tb.Helper()

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(tb, err)

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	return paths
}
