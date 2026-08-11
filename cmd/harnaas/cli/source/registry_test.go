package source_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// recordingKind is a kind that resolves nothing and remembers whether it was
// asked to. What most of these tests assert is which kind was reached, and — for
// an unsupported kind — that none was.
type recordingKind struct {
	calls []source.Request
}

func (k *recordingKind) Resolve(_ context.Context, req source.Request) (*source.Resolved, error) {
	k.calls = append(k.calls, req)
	return source.NewResolved(source.Provenance{Kind: req.Kind()}, map[string][]byte{
		"SKILL.md": []byte(req.Asset.ID),
	})
}

// remoteAssetID is the id every keyed-form request in these tests carries. The
// tests that vary an id vary the local form's, because a request's kind is the
// only thing under test here.
const remoteAssetID = "review"

// githubRequest is one asset resolved through a `sources` entry.
func githubRequest() source.Request {
	return source.Request{
		Asset: manifest.Asset{
			Ref:  manifest.AssetRef{SourceKey: "acme", Path: "skills/" + remoteAssetID},
			Type: manifest.AssetTypeSkill,
			ID:   remoteAssetID,
		},
		Source: manifest.Source{
			Key:        "acme",
			Kind:       manifest.SourceKindGitHub,
			Repository: "acme/assets",
			Ref:        "v1.2.0",
		},
	}
}

// localRequest is one asset resolved from the project's own `.harnaas`, which
// references no `sources` entry at all.
func localRequest(id string) source.Request {
	return source.Request{
		Asset: manifest.Asset{
			Ref:  manifest.AssetRef{Path: manifest.LocalRoot + "/skills/" + id},
			Type: manifest.AssetTypeSkill,
			ID:   id,
		},
	}
}

func TestResolverDispatchesToTheKindThatOwnsTheSource(t *testing.T) {
	t.Parallel()

	github, local := &recordingKind{}, &recordingKind{}
	registry := registryWith(t, map[manifest.SourceKind]source.Kind{
		manifest.SourceKindGitHub: github,
		manifest.SourceKindLocal:  local,
	})

	resolver := registry.NewResolver(source.RunOptions{})

	remote, err := resolver.Resolve(t.Context(), githubRequest())
	require.NoError(t, err)
	assert.Equal(t, manifest.SourceKindGitHub, remote.Provenance.Kind)

	own, err := resolver.Resolve(t.Context(), localRequest("house-style"))
	require.NoError(t, err)
	assert.Equal(t, manifest.SourceKindLocal, own.Provenance.Kind)

	require.Len(t, github.calls, 1)
	assert.Equal(t, remoteAssetID, github.calls[0].Asset.ID)
	require.Len(t, local.calls, 1)
	assert.Equal(t, "house-style", local.calls[0].Asset.ID)
}

// TestUnregisteredKindFailsBeforeAnyKindIsAsked is the dispatch half of "no
// network request or filesystem write occurs": the lookup precedes the work, so
// the registered kind is never entered.
func TestUnregisteredKindFailsBeforeAnyKindIsAsked(t *testing.T) {
	t.Parallel()

	local := &recordingKind{}
	registry := registryWith(t, map[manifest.SourceKind]source.Kind{
		manifest.SourceKindLocal: local,
	})

	resolved, err := registry.NewResolver(source.RunOptions{}).Resolve(t.Context(), githubRequest())

	require.Error(t, err)
	assert.Nil(t, resolved)
	assert.Empty(t, local.calls)

	var unsupported *source.UnsupportedKindError
	require.ErrorAs(t, err, &unsupported)
	assert.Equal(t, manifest.SourceKindGitHub, unsupported.Kind)
	assert.Equal(t, []manifest.SourceKind{manifest.SourceKindLocal}, unsupported.Registered)
}

func TestUnsupportedKindErrorStatesTheProblemThenTheFix(t *testing.T) {
	t.Parallel()

	err := &source.UnsupportedKindError{
		Kind:       "gitlab",
		Registered: []manifest.SourceKind{manifest.SourceKindGitHub, manifest.SourceKindLocal},
	}

	problem, fix, found := strings.Cut(err.Error(), "\n\n")
	require.True(t, found, "the message separates the problem from the fix")
	assert.Contains(t, problem, `"gitlab"`)
	assert.NotContains(t, problem, "\n")
	assert.Contains(t, fix, "github, local")
}

// TestUnsupportedKindErrorListsWhatThisRunCouldResolve pins the captured list:
// the message stays a complete diagnostic wherever it is finally printed, rather
// than describing the registry as it stands when somebody reads it.
func TestUnsupportedKindErrorListsWhatThisRunCouldResolve(t *testing.T) {
	t.Parallel()

	registry := registryWith(t, map[manifest.SourceKind]source.Kind{
		manifest.SourceKindLocal: &recordingKind{},
	})
	resolver := registry.NewResolver(source.RunOptions{})

	registry.Register(manifest.SourceKindGitHub, func(source.RunOptions) source.Kind { return &recordingKind{} })

	_, err := resolver.Resolve(t.Context(), githubRequest())

	var unsupported *source.UnsupportedKindError
	require.ErrorAs(t, err, &unsupported)
	assert.Equal(t, []manifest.SourceKind{manifest.SourceKindLocal}, unsupported.Registered)
}

func TestRegisterPanicsOnADuplicateKind(t *testing.T) {
	t.Parallel()

	registry := registryWith(t, map[manifest.SourceKind]source.Kind{
		manifest.SourceKindLocal: &recordingKind{},
	})

	assert.PanicsWithValue(t, `source: kind "local" is registered twice`, func() {
		registry.Register(manifest.SourceKindLocal, func(source.RunOptions) source.Kind { return &recordingKind{} })
	})
}

// TestKindsIsSortedRatherThanInRegistrationOrder matters because registration
// order is whatever the import graph produces, and this list is rendered into a
// message a user reads.
func TestKindsIsSortedRatherThanInRegistrationOrder(t *testing.T) {
	t.Parallel()

	var registry source.Registry
	for _, name := range []manifest.SourceKind{"local", "github", "acme"} {
		registry.Register(name, func(source.RunOptions) source.Kind { return &recordingKind{} })
	}

	sorted := []manifest.SourceKind{"acme", "github", "local"}
	assert.Equal(t, sorted, registry.Kinds())
	assert.Equal(t, sorted, registry.Kinds(), "and the same list on every call")
}

func TestKindsOfAnEmptyRegistryIsEmpty(t *testing.T) {
	t.Parallel()

	var registry source.Registry
	assert.Empty(t, registry.Kinds())
}

// TestEachRunGetsItsOwnKind is the reason registration takes a constructor: what
// a kind remembers — which archives this run has already fetched — must not
// outlive the run that learned it.
func TestEachRunGetsItsOwnKind(t *testing.T) {
	t.Parallel()

	constructed := 0
	var registry source.Registry
	registry.Register(manifest.SourceKindLocal, func(source.RunOptions) source.Kind {
		constructed++
		return &recordingKind{}
	})

	assert.Equal(t, 0, constructed, "no kind is constructed until a run needs one")

	first, second := registry.NewResolver(source.RunOptions{}), registry.NewResolver(source.RunOptions{})
	assert.Equal(t, 2, constructed)

	_, err := first.Resolve(t.Context(), localRequest("one"))
	require.NoError(t, err)
	_, err = second.Resolve(t.Context(), localRequest("two"))
	require.NoError(t, err)
}

// TestTheRunsOptionsReachEveryKind is why the options are settled at the
// resolver rather than at each kind's own entry point: a run cannot end up with
// one kind reading the cache and another bypassing it.
func TestTheRunsOptionsReachEveryKind(t *testing.T) {
	t.Parallel()

	var given []source.RunOptions
	var registry source.Registry
	for _, name := range []manifest.SourceKind{manifest.SourceKindGitHub, manifest.SourceKindLocal} {
		registry.Register(name, func(opts source.RunOptions) source.Kind {
			given = append(given, opts)
			return &recordingKind{}
		})
	}

	opts := source.RunOptions{Cache: source.NewArchiveCache(), Offline: true}
	registry.NewResolver(opts)

	require.Len(t, given, 2)
	for _, got := range given {
		assert.Same(t, opts.Cache, got.Cache)
		assert.True(t, got.Offline, "a run is offline for every kind or for none; the network is not per kind")
	}
}

// TestResolveReportsTheKindsOwnFailure keeps dispatch transparent: a kind's error
// reaches the caller as it was written, because it is the one that can name the
// repository, the ref and the asset.
func TestResolveReportsTheKindsOwnFailure(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("the remote refused the connection")

	var registry source.Registry
	registry.Register(manifest.SourceKindLocal, func(source.RunOptions) source.Kind {
		return failingKind{err: sentinel}
	})

	resolved, err := registry.NewResolver(source.RunOptions{}).Resolve(t.Context(), localRequest("one"))

	require.ErrorIs(t, err, sentinel)
	assert.Nil(t, resolved)
}

type failingKind struct{ err error }

func (k failingKind) Resolve(context.Context, source.Request) (*source.Resolved, error) {
	return nil, k.err
}

// registryWith builds a registry holding the given kinds, each handed back to
// every run rather than constructed fresh, so a test can hold on to the instance
// it wants to make assertions about.
func registryWith(tb testing.TB, kinds map[manifest.SourceKind]source.Kind) *source.Registry {
	tb.Helper()

	registry := &source.Registry{}
	for name, kind := range kinds {
		registry.Register(name, func(source.RunOptions) source.Kind { return kind })
	}
	return registry
}
