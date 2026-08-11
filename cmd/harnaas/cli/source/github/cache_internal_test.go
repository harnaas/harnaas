package github

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// The cache seen from the kind that fills it. What the store itself guarantees
// is asserted where it lives; what matters here is the order the three
// memories are consulted in — this run's memo, then this machine's cache, then
// the forge — and that only content the forge actually served is filed.
//
// These tests move HARNAAS_CACHE_DIR, so they do not run in parallel.

// cachedRun resolves one asset through its own kind, which is what a second run
// of harnaas is: a fresh memo over the same cache.
func cachedRun(t *testing.T, cache *source.ArchiveCache, fetcher *countingFetcher) error {
	t.Helper()

	kind := newKind(answeringRunner(tagListing, nil), fetcher.fetch, source.Credential{}, source.RunOptions{Cache: cache})

	_, err := kind.Resolve(t.Context(), resolveRequest("review", "v1.2.0", "skills/review"))
	return err
}

func TestASecondRunResolvesFromTheCacheWithoutFetching(t *testing.T) {
	t.Setenv(source.CacheDirEnvVar, t.TempDir())
	cache := source.NewArchiveCache()

	first := &countingFetcher{body: assetArchive(t)}
	require.NoError(t, cachedRun(t, cache, first))
	require.Len(t, first.asked(), 1)

	second := &countingFetcher{err: errors.New("the network is not available")}
	require.NoError(t, cachedRun(t, cache, second),
		"the archive the first run fetched is still on this machine")
	assert.Empty(t, second.asked(), "a cached archive is not fetched again")
}

// TestABypassedCacheFetchesFresh is the same two runs with the bypass, and is
// what makes the test above an assertion about the cache rather than about the
// memo.
func TestABypassedCacheFetchesFresh(t *testing.T) {
	t.Setenv(source.CacheDirEnvVar, t.TempDir())

	first := &countingFetcher{body: assetArchive(t)}
	require.NoError(t, cachedRun(t, nil, first))

	second := &countingFetcher{body: assetArchive(t)}
	require.NoError(t, cachedRun(t, nil, second))

	assert.Len(t, second.asked(), 1, "a run handed no cache reads nothing an earlier one left")
}

// TestAFailedRetrievalIsNotCached keeps the cross-run memory a memory of
// content: the within-run memo remembers a failure so one outage is not
// multiplied by the assets that met it, and that reasoning expires with the run.
func TestAFailedRetrievalIsNotCached(t *testing.T) {
	t.Setenv(source.CacheDirEnvVar, t.TempDir())
	cache := source.NewArchiveCache()

	refused := &countingFetcher{err: errors.New("the host refused")}
	require.Error(t, cachedRun(t, cache, refused))

	recovered := &countingFetcher{body: assetArchive(t)}
	require.NoError(t, cachedRun(t, cache, recovered),
		"the next run tries again rather than replaying yesterday's outage")
}
