package github

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// Offline resolution, from the kind that has to make no request at all.
//
// Every test here asserts an absence as well as an outcome: the fetcher is
// asked for the URLs it was given and the runner fails the test if it is called,
// because "resolution happened to succeed" and "resolution happened without the
// network" are different claims and only the second one is the feature.
//
// The tests that move HARNAAS_CACHE_DIR do not run in parallel.

// offlineKind is one offline run over cache, which may fetch nothing and look up
// nothing.
func offlineKind(t *testing.T, cache *source.ArchiveCache, fetcher *countingFetcher) *Kind {
	t.Helper()

	return newKind(refusingRunner(t), fetcher.fetch, source.Credential{},
		source.RunOptions{Cache: cache, Offline: true})
}

func TestAnOfflineRunResolvesACachedCommitWithNoRequestOfAnyKind(t *testing.T) {
	t.Setenv(source.CacheDirEnvVar, t.TempDir())
	cache := source.NewArchiveCache()

	// The warming run is what any earlier run is: the same commit, fetched once
	// and filed on this machine.
	warm := &countingFetcher{body: assetArchive(t)}
	online := newKind(refusingRunner(t), warm.fetch, source.Credential{}, source.RunOptions{Cache: cache})
	_, err := online.Resolve(t.Context(), resolveRequest("review", commitID, "skills/review"))
	require.NoError(t, err)
	require.Len(t, warm.asked(), 1)

	unreachable := &countingFetcher{err: errors.New("the network is not available")}
	resolved, err := offlineKind(t, cache, unreachable).
		Resolve(t.Context(), resolveRequest("review", commitID, "skills/review"))
	require.NoError(t, err)

	assert.NotEmpty(t, resolved.Files, "a fully cached source resolves exactly as it would online")
	assert.Equal(t, commitID, resolved.Provenance.ResolvedCommit)
	assert.Empty(t, unreachable.asked(), "an offline run makes no request for content")
}

func TestAnOfflineRunNamesTheAssetAndTheCommitItHasNoArchiveFor(t *testing.T) {
	t.Setenv(source.CacheDirEnvVar, t.TempDir())

	unreachable := &countingFetcher{err: errors.New("the network is not available")}
	resolved, err := offlineKind(t, source.NewArchiveCache(), unreachable).
		Resolve(t.Context(), resolveRequest("review", commitID, "skills/review"))

	require.Nil(t, resolved, "a source that could not be resolved is never handed on as content")

	var offline *OfflineArchiveError
	require.ErrorAs(t, err, &offline)
	assert.Equal(t, "review", offline.AssetID)
	assert.Equal(t, repository, offline.Repository)
	assert.Equal(t, commitID, offline.Commit)
	assert.Empty(t, unreachable.asked(), "an uncached source is a refusal, not a fetch")
}

// TestAnOfflineRunNamesEveryUncachedAssetRatherThanTheFirst holds the half of
// the rule the in-run memo could quietly break: a repository is asked for once
// and the answer remembered, so the second asset of that repository has to be
// told about itself rather than about the asset that arrived first.
func TestAnOfflineRunNamesEveryUncachedAssetRatherThanTheFirst(t *testing.T) {
	t.Setenv(source.CacheDirEnvVar, t.TempDir())

	unreachable := &countingFetcher{err: errors.New("the network is not available")}
	kind := offlineKind(t, source.NewArchiveCache(), unreachable)

	_, first := kind.Resolve(t.Context(), resolveRequest("review", commitID, "skills/review"))
	_, second := kind.Resolve(t.Context(), resolveRequest("release", commitID, "skills/release"))

	require.Error(t, first)
	require.Error(t, second)
	assert.Contains(t, first.Error(), `"review"`)
	assert.Contains(t, second.Error(), `"release"`, "every uncached asset is named, not only the one that got there first")
}

// TestAnOfflineRunRefusesANameWithoutLookingItUp is the rule a cache cannot
// help with: a tag or a branch is a name in somebody else's repository, and what
// it points at today is not a fact this machine holds.
func TestAnOfflineRunRefusesANameWithoutLookingItUp(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		ref   string
		names string
	}{
		{name: "tag", ref: "v1.2.0", names: `"v1.2.0" in acme/assets`},
		{name: "branch", ref: "main", names: `"main" in acme/assets`},
		{name: "abbreviated commit", ref: commitID[:8], names: `"` + commitID[:8] + `" in acme/assets`},
		{name: "no ref at all", ref: "", names: "the default branch of acme/assets"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			unreachable := &countingFetcher{err: errors.New("the network is not available")}
			// refusingRunner is the assertion: a lookup that decided to refuse
			// would still be a lookup, and no lookup is what was asked for.
			kind := newKind(refusingRunner(t), unreachable.fetch, source.Credential{},
				source.RunOptions{Offline: true})

			resolved, err := kind.Resolve(t.Context(), resolveRequest("review", tc.ref, "skills/review"))
			require.Nil(t, resolved)

			var offline *OfflineRefError
			require.ErrorAs(t, err, &offline)
			assert.Equal(t, "review", offline.AssetID)
			assert.Equal(t, tc.ref, offline.Ref)
			assert.Contains(t, err.Error(), tc.names)
			assert.Empty(t, unreachable.asked(), "a ref that did not resolve is never fetched for")
		})
	}
}

// TestAnOfflineRunWithNoCacheStillRefusesRatherThanFetches keeps the two
// caller-facing choices independent: the bypass says the cache may not be read
// and offline says the network may not be reached, and a run making both choices
// resolves nothing rather than quietly picking the one it can still do.
func TestAnOfflineRunWithNoCacheStillRefusesRatherThanFetches(t *testing.T) {
	t.Parallel()

	unreachable := &countingFetcher{body: assetArchive(t)}
	kind := newKind(refusingRunner(t), unreachable.fetch, source.Credential{},
		source.RunOptions{Offline: true})

	resolved, err := kind.Resolve(t.Context(), resolveRequest("review", commitID, "skills/review"))

	require.Nil(t, resolved)
	var offline *OfflineArchiveError
	require.ErrorAs(t, err, &offline)
	assert.Empty(t, unreachable.asked())
}

// TestAnOnlineRunIsStillTheDefault is what makes every assertion above about
// offline mode rather than about a kind that cannot fetch: the same request,
// with the option left at its zero value, reaches both git and the forge.
func TestAnOnlineRunIsStillTheDefault(t *testing.T) {
	t.Parallel()

	var asked []string
	fetcher := &countingFetcher{body: assetArchive(t)}
	kind := newKind(answeringRunner(tagListing, &asked), fetcher.fetch, source.Credential{}, source.RunOptions{})

	_, err := kind.Resolve(t.Context(), resolveRequest("review", "v1.2.0", "skills/review"))
	require.NoError(t, err)

	assert.NotEmpty(t, asked, "a run that did not ask to be offline resolves its ref against the remote")
	assert.Len(t, fetcher.asked(), 1)
}
