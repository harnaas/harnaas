package claudecode_test

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter/claudecode"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// asset builds the interpreted entry an adapter is handed. Every field an entry
// may leave undeclared is already answered by the time an [manifest.Asset]
// exists, which is why these tests never construct a half-resolved one.
func asset(assetType manifest.AssetType, id string, scope manifest.Scope) manifest.Asset {
	return manifest.Asset{
		Type:    assetType,
		ID:      id,
		Targets: []harness.ID{harness.ClaudeCode},
		Scope:   scope,
	}
}

func TestAdapterRegistersUnderClaudeCode(t *testing.T) {
	t.Parallel()

	assert.Equal(t, harness.ClaudeCode, claudecode.New().Harness())
}

// TestDestinationMapsEachTypeToItsSurface is the mapping itself: the three types
// with no shared equivalent, each at the path the harness reads, with the asset
// id as the file stem.
func TestDestinationMapsEachTypeToItsSurface(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		assetType manifest.AssetType
		id        string
		want      string
	}{
		{manifest.AssetTypeRule, "house-style", "rules/house-style.md"},
		{manifest.AssetTypeCommand, "review", "commands/review.md"},
		{manifest.AssetTypePersona, "reviewer", "agents/reviewer.md"},
	} {
		t.Run(string(tc.assetType), func(t *testing.T) {
			t.Parallel()

			destination, found := claudecode.New().Destination(asset(tc.assetType, tc.id, manifest.ScopeProject))

			require.True(t, found)
			assert.Equal(t, tc.want, destination.Path)
			assert.Equal(t, adapter.TierLive, destination.Tier)
			assert.Equal(t, adapter.RendererIdentity, destination.Renderer)
			assert.Empty(t, destination.Note, "a live surface carries no note")
			assert.Empty(t, destination.Replacement, "only a removed surface names a replacement")
		})
	}
}

// TestSharedTypesHaveNoSurfaceHere pins the deliberate absence. A skill and an
// instruction install through locations no per-harness code computes, so mapping
// them into `.claude/` as well would write a second copy of content the harness
// already reads — and would make this package a second answer to a question the
// shared targets answer once.
func TestSharedTypesHaveNoSurfaceHere(t *testing.T) {
	t.Parallel()

	for _, assetType := range []manifest.AssetType{manifest.AssetTypeSkill, manifest.AssetTypeInstruction} {
		t.Run(string(assetType), func(t *testing.T) {
			t.Parallel()

			destination, found := claudecode.New().Destination(asset(assetType, "review", manifest.ScopeProject))

			assert.False(t, found)
			assert.Equal(t, adapter.Destination{}, destination, "nothing a caller could write is handed back")
		})
	}
}

// TestTheSameDestinationServesBothScopes is what makes the scope a property of
// the root rather than of the mapping: `user` moves the directory the path is
// joined beneath and leaves the path itself alone, so the lockfile records
// something that means the same thing on another machine.
func TestTheSameDestinationServesBothScopes(t *testing.T) {
	t.Parallel()

	a := claudecode.New()

	project, found := a.Destination(asset(manifest.AssetTypeRule, "house-style", manifest.ScopeProject))
	require.True(t, found)

	user, found := a.Destination(asset(manifest.AssetTypeRule, "house-style", manifest.ScopeUser))
	require.True(t, found)

	assert.Equal(t, project, user)
	assert.Equal(t, "rules/house-style.md", project.Path)
}

// TestBothScopesResolveToTheHarnessDirectory covers the half of scope
// resolution this adapter owns: Claude Code's per-user location is unambiguous,
// so `user` scope is offered here and an asset declaring it is never quietly
// installed at project scope.
func TestBothScopesResolveToTheHarnessDirectory(t *testing.T) {
	t.Parallel()

	for _, scope := range []manifest.Scope{manifest.ScopeProject, manifest.ScopeUser} {
		root, err := adapter.ResolveRoot(claudecode.New(), asset(manifest.AssetTypeRule, "house-style", scope))

		require.NoError(t, err)
		assert.Equal(t, ".claude", root)
	}
}

// TestAScopeThisHarnessDoesNotDefineIsRefused guards the zero value and anything
// a later scope might add: a root is offered for the scopes this harness has a
// directory for and for nothing else.
func TestAScopeThisHarnessDoesNotDefineIsRefused(t *testing.T) {
	t.Parallel()

	root, offered := claudecode.New().Root(manifest.Scope("machine"))

	assert.False(t, offered)
	assert.Empty(t, root)
}

// TestDetectionReadsTheRostersEvidence covers both pieces the roster declares.
// Any one of them is enough, because a project with a `CLAUDE.md` and no
// `.claude/` is still a project using Claude Code.
func TestDetectionReadsTheRostersEvidence(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		project fstest.MapFS
	}{
		{"configuration directory", fstest.MapFS{".claude/settings.json": &fstest.MapFile{}}},
		{"memory file", fstest.MapFS{"CLAUDE.md": &fstest.MapFile{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.True(t, claudecode.New().Detect(tc.project))
		})
	}
}

// TestDetectionReportsAbsenceRatherThanFailure is the other half of a signature
// that cannot write: a harness that is simply not installed is an absence, and
// there is nothing for a caller to handle.
func TestDetectionReportsAbsenceRatherThanFailure(t *testing.T) {
	t.Parallel()

	project := fstest.MapFS{"README.md": &fstest.MapFile{}, ".git/HEAD": &fstest.MapFile{}}

	assert.False(t, claudecode.New().Detect(project))
}

// TestEvidenceIsNotFollowed pins the Lstat rule. A `.claude` link pointing at a
// directory this checkout does not have is still the harness having left
// something behind, and a detection that resolved it would answer "no" for a
// project that plainly says yes.
func TestEvidenceIsNotFollowed(t *testing.T) {
	t.Parallel()

	project := fstest.MapFS{
		".claude": &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("elsewhere")},
	}

	_, err := fs.Stat(project, ".claude")
	require.Error(t, err, "the link resolves to nothing, so a Stat-based detection would miss it")

	assert.True(t, claudecode.New().Detect(project))
}

// TestARuleInstallsAsAStandaloneFile is task 6.7 at this layer. A rule is one
// file the harness discovers on its own, and the adapter's whole answer for one
// is that file: no memory file is named, nothing is appended anywhere, and the
// destination carries no second path a caller could write a reference into.
//
// The managed block is the other place a reference could appear, and it cannot:
// the block holds instruction assets by definition, which its own tests pin
// where it is built.
func TestARuleInstallsAsAStandaloneFile(t *testing.T) {
	t.Parallel()

	destination, found := claudecode.New().Destination(asset(manifest.AssetTypeRule, "house-style", manifest.ScopeProject))

	require.True(t, found)
	assert.Equal(
		t,
		adapter.Destination{Path: "rules/house-style.md", Tier: adapter.TierLive, Renderer: adapter.RendererIdentity},
		destination,
		"the whole of installing a rule is writing one file",
	)
}

// TestNoSurfaceIsAMemoryFile is the negative half of the same rule, asserted
// across every type rather than for a rule alone: this adapter never resolves an
// asset to a file harnaas edits in place, so no installation it describes can
// add a line to one.
func TestNoSurfaceIsAMemoryFile(t *testing.T) {
	t.Parallel()

	for _, assetType := range manifest.AssetTypes() {
		destination, found := claudecode.New().Destination(asset(assetType, "house-style", manifest.ScopeProject))
		if !found {
			continue
		}

		assert.NotContains(t, destination.Path, "CLAUDE.md")
		assert.NotContains(t, destination.Path, "AGENTS.md")
	}
}
