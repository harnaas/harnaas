package adapter_test

import (
	"io/fs"
	"path"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// testAdapter is an adapter shaped like the ones harnaas will ship: a directory
// per scope, a surface for some types and none for others.
//
// The contract is what these tests are about, so the surfaces are declared as
// data rather than mapped by a method — an adapter that computed them would be
// asserting its own arithmetic instead of the shape every adapter has to present.
type testAdapter struct {
	id harness.ID

	// roots is the harness's directory per scope. A scope absent from the map is
	// one this adapter does not offer.
	roots map[manifest.Scope]string

	// surfaces is the destination for each asset type this harness has one for,
	// relative to the scope root, with `{id}` standing in for the asset id.
	surfaces map[manifest.AssetType]adapter.Destination

	// evidence is the path whose presence means the harness is in use.
	evidence string
}

var _ adapter.Adapter = (*testAdapter)(nil)

func (a *testAdapter) Harness() harness.ID { return a.id }

func (a *testAdapter) Detect(project fs.FS) bool {
	_, err := fs.Stat(project, a.evidence)
	return err == nil
}

func (a *testAdapter) Root(scope manifest.Scope) (string, bool) {
	root, offered := a.roots[scope]
	return root, offered
}

func (a *testAdapter) Destination(asset manifest.Asset) (adapter.Destination, bool) {
	surface, supported := a.surfaces[asset.Type]
	if !supported {
		return adapter.Destination{}, false
	}
	if _, offered := a.roots[asset.Scope]; !offered {
		return adapter.Destination{}, false
	}

	surface.Path = path.Join(surface.Path, asset.ID+".md")
	return surface, true
}

// claudeLike is the adapter these tests exercise the contract through: two
// scopes, a live rules surface, and no surface at all for a persona.
func claudeLike() *testAdapter {
	return &testAdapter{
		id: harness.ClaudeCode,
		roots: map[manifest.Scope]string{
			manifest.ScopeProject: ".claude",
			manifest.ScopeUser:    ".claude",
		},
		surfaces: map[manifest.AssetType]adapter.Destination{
			manifest.AssetTypeRule: {Path: "rules", Tier: adapter.TierLive, Renderer: adapter.RendererIdentity},
		},
		evidence: ".claude",
	}
}

func asset(assetType manifest.AssetType, id string, scope manifest.Scope) manifest.Asset {
	return manifest.Asset{Type: assetType, ID: id, Targets: []harness.ID{harness.ClaudeCode}, Scope: scope}
}

// TestDestinationIsRelativeToTheResolvedScopeRoot pins the one thing every
// caller of the contract depends on: the same mapping serves both scopes, so the
// path the lockfile records means the same thing on another machine.
func TestDestinationIsRelativeToTheResolvedScopeRoot(t *testing.T) {
	t.Parallel()

	a := claudeLike()

	project, found := a.Destination(asset(manifest.AssetTypeRule, "house-style", manifest.ScopeProject))
	assert.True(t, found)
	assert.Equal(t, "rules/house-style.md", project.Path)

	user, found := a.Destination(asset(manifest.AssetTypeRule, "house-style", manifest.ScopeUser))
	assert.True(t, found)
	assert.Equal(t, project.Path, user.Path, "the scope moves the root, not the destination beneath it")
}

// TestNoSurfaceForATypeIsReportedRatherThanInvented is task 5.2's whole point.
// A guessed path is indistinguishable from a real one once it has been written,
// so the absence has to be a value the caller cannot mistake for a destination.
func TestNoSurfaceForATypeIsReportedRatherThanInvented(t *testing.T) {
	t.Parallel()

	destination, found := claudeLike().Destination(asset(manifest.AssetTypePersona, "reviewer", manifest.ScopeProject))

	assert.False(t, found)
	assert.Equal(t, adapter.Destination{}, destination, "nothing a caller could write is handed back")
}

// TestAScopeTheAdapterDoesNotOfferHasNoRootAndNoDestination is the shape scope
// validation reads: an adapter states the scopes it has an unambiguous location
// for, and states it in one place.
func TestAScopeTheAdapterDoesNotOfferHasNoRootAndNoDestination(t *testing.T) {
	t.Parallel()

	projectOnly := claudeLike()
	delete(projectOnly.roots, manifest.ScopeUser)

	root, offered := projectOnly.Root(manifest.ScopeUser)
	assert.False(t, offered)
	assert.Empty(t, root)

	destination, found := projectOnly.Destination(asset(manifest.AssetTypeRule, "house-style", manifest.ScopeUser))
	assert.False(t, found)
	assert.Equal(t, adapter.Destination{}, destination)
}

func TestRootIsRelativeToTheScopesOwnRoot(t *testing.T) {
	t.Parallel()

	for _, scope := range []manifest.Scope{manifest.ScopeProject, manifest.ScopeUser} {
		root, offered := claudeLike().Root(scope)

		assert.True(t, offered)
		assert.Equal(t, ".claude", root)
		assert.False(t, path.IsAbs(root), "an adapter never decides where a user's files live")
	}
}

// TestDetectionReadsAndCreatesNothing is carried by the signature rather than by
// this test — an fs.FS has no way to write — so what is asserted here is the
// other half: a harness that is simply not installed is an absence, never a
// failure a caller has to handle.
func TestDetectionReadsAndCreatesNothing(t *testing.T) {
	t.Parallel()

	using := fstest.MapFS{".claude/settings.json": &fstest.MapFile{}}
	assert.True(t, claudeLike().Detect(using))

	empty := fstest.MapFS{"README.md": &fstest.MapFile{}}
	assert.False(t, claudeLike().Detect(empty))
}

// TestASurfaceCarriesItsTermsWithItsPath is why [adapter.Destination] is one
// value: a caller cannot obtain a path and write it without having been told
// whether the harness still reads there and what has to happen to the bytes.
func TestASurfaceCarriesItsTermsWithItsPath(t *testing.T) {
	t.Parallel()

	removed := claudeLike()
	removed.surfaces[manifest.AssetTypeCommand] = adapter.Destination{
		Path:        "commands",
		Tier:        adapter.TierRemoved,
		Replacement: ".claude/skills",
	}

	destination, found := removed.Destination(asset(manifest.AssetTypeCommand, "review", manifest.ScopeProject))

	assert.True(t, found)
	assert.Equal(t, "commands/review.md", destination.Path)
	assert.Equal(t, adapter.TierRemoved, destination.Tier)
	assert.Equal(t, ".claude/skills", destination.Replacement)
}
