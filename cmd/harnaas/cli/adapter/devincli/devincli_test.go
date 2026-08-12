package devincli_test

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter/devincli"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// asset builds the interpreted entry an adapter is handed. Every field an entry
// may leave undeclared is already answered by the time a [manifest.Asset] exists,
// which is why these tests never construct a half-resolved one.
func asset(assetType manifest.AssetType, id string, scope manifest.Scope) manifest.Asset {
	return manifest.Asset{
		Type:    assetType,
		ID:      id,
		Targets: []harness.ID{harness.DevinCLI},
		Scope:   scope,
	}
}

func TestAdapterRegistersUnderDevinCLI(t *testing.T) {
	t.Parallel()

	assert.Equal(t, harness.DevinCLI, devincli.New().Harness())
}

// TestDestinationMapsEachTypeToItsSurface is the mapping itself: the two types
// this harness has a surface of its own for, each at the path it reads, with the
// asset id as the file stem.
func TestDestinationMapsEachTypeToItsSurface(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		assetType manifest.AssetType
		id        string
		want      string
	}{
		{manifest.AssetTypeRule, "house-style", "rules/house-style.md"},
		{manifest.AssetTypePersona, "reviewer", "agents/reviewer.md"},
	} {
		t.Run(string(tc.assetType), func(t *testing.T) {
			t.Parallel()

			destination, found := devincli.New().Destination(asset(tc.assetType, tc.id, manifest.ScopeProject))

			require.True(t, found)
			assert.Equal(t, tc.want, destination.Path)
			assert.Equal(t, adapter.TierLive, destination.Tier)
			assert.Equal(t, adapter.RendererIdentity, destination.Renderer)
			assert.Empty(t, destination.Note, "a live surface carries no note")
			assert.Empty(t, destination.Replacement, "only a removed surface names a replacement")
		})
	}
}

// TestAPersonaIsAFileRatherThanADirectory pins the choice between the two
// spellings this harness reads. The report names a path, and a path to a file is
// something a reader can open where a path to a directory is something they have
// to go looking inside.
func TestAPersonaIsAFileRatherThanADirectory(t *testing.T) {
	t.Parallel()

	destination, found := devincli.New().Destination(asset(manifest.AssetTypePersona, "reviewer", manifest.ScopeProject))

	require.True(t, found)
	assert.Equal(t, "agents/reviewer.md", destination.Path)
	assert.NotContains(t, destination.Path, "AGENT.md", "the directory spelling is deliberately not used")
}

// TestTypesWithoutASurfaceHereHaveNone pins three deliberate absences with three
// different reasons. A skill and an instruction install through shared locations,
// so answering for them would be a second place harnaas decides where they land;
// a command has no surface on this harness at all, and inventing one would write a
// path it never opens.
func TestTypesWithoutASurfaceHereHaveNone(t *testing.T) {
	t.Parallel()

	for _, assetType := range []manifest.AssetType{
		manifest.AssetTypeSkill,
		manifest.AssetTypeInstruction,
		manifest.AssetTypeCommand,
	} {
		t.Run(string(assetType), func(t *testing.T) {
			t.Parallel()

			destination, found := devincli.New().Destination(asset(assetType, "review", manifest.ScopeProject))

			assert.False(t, found)
			assert.Equal(t, adapter.Destination{}, destination, "nothing a caller could write is handed back")
		})
	}
}

// TestProjectScopeResolvesToTheHarnessDirectory covers the half of scope
// resolution this adapter owns for the scope it offers.
func TestProjectScopeResolvesToTheHarnessDirectory(t *testing.T) {
	t.Parallel()

	root, err := adapter.ResolveRoot(devincli.New(), asset(manifest.AssetTypeRule, "house-style", manifest.ScopeProject))

	require.NoError(t, err)
	assert.Equal(t, ".devin", root)
}

// TestUserScopeIsRefusedRatherThanResolved is the first real exercise of the
// refusal the contract was built for. This harness keeps its per-user rules under
// one home directory and its per-user skills, personas and memory file under
// another, spelled differently per platform, so there is no single root a
// destination could be counted from — and a refusal is a refusal rather than a
// quieter install at project scope.
func TestUserScopeIsRefusedRatherThanResolved(t *testing.T) {
	t.Parallel()

	root, err := adapter.ResolveRoot(devincli.New(), asset(manifest.AssetTypeRule, "house-style", manifest.ScopeUser))

	require.Error(t, err)
	assert.Empty(t, root)

	var unsupported *adapter.UnsupportedScopeError
	require.ErrorAs(t, err, &unsupported)
	assert.Equal(t, harness.DevinCLI, unsupported.Harness)
	assert.Equal(t, manifest.ScopeUser, unsupported.Scope)
	assert.Equal(t, "house-style", unsupported.AssetID)
	assert.Equal(t, manifest.AssetTypeRule, unsupported.Type)
}

// TestRootIsOfferedForProjectScopeAlone guards the zero value and anything a later
// scope might add: a root is offered for the one scope this harness has a single
// directory for, and for nothing else.
func TestRootIsOfferedForProjectScopeAlone(t *testing.T) {
	t.Parallel()

	for _, scope := range []manifest.Scope{manifest.ScopeUser, manifest.Scope("machine"), manifest.Scope("")} {
		t.Run(string(scope), func(t *testing.T) {
			t.Parallel()

			root, offered := devincli.New().Root(scope)

			assert.False(t, offered)
			assert.Empty(t, root)
		})
	}
}

// TestDetectionReadsTheRostersEvidence covers what the roster declares, which is
// the configuration directory alone. The shared memory file is deliberately not
// evidence: most recognized harnesses read it, so its presence says nothing about
// this one.
func TestDetectionReadsTheRostersEvidence(t *testing.T) {
	t.Parallel()

	assert.True(t, devincli.New().Detect(fstest.MapFS{".devin/config.json": &fstest.MapFile{}}))
}

// TestTheSharedMemoryFileIsNotEvidence is the exclusion stated as a test rather
// than only as a comment. A project that has ever written an `AGENTS.md` is not
// thereby a project using this harness, and detecting one would have `init`
// scaffold a guarantee nobody asked for.
func TestTheSharedMemoryFileIsNotEvidence(t *testing.T) {
	t.Parallel()

	project := fstest.MapFS{
		"AGENTS.md": &fstest.MapFile{},
		"CLAUDE.md": &fstest.MapFile{},
		".claude":   &fstest.MapFile{Mode: fs.ModeDir},
	}

	assert.False(t, devincli.New().Detect(project))
}

// TestDetectionReportsAbsenceRatherThanFailure is the other half of a signature
// that cannot write: a harness that is simply not installed is an absence, and
// there is nothing for a caller to handle.
func TestDetectionReportsAbsenceRatherThanFailure(t *testing.T) {
	t.Parallel()

	project := fstest.MapFS{"README.md": &fstest.MapFile{}, ".git/HEAD": &fstest.MapFile{}}

	assert.False(t, devincli.New().Detect(project))
}

// TestEvidenceIsNotFollowed pins the Lstat rule. A `.devin` link pointing at a
// directory this checkout does not have is still the harness having left something
// behind, and a detection that resolved it would answer "no" for a project that
// plainly says yes.
func TestEvidenceIsNotFollowed(t *testing.T) {
	t.Parallel()

	project := fstest.MapFS{
		".devin": &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("elsewhere")},
	}

	_, err := fs.Stat(project, ".devin")
	require.Error(t, err, "the link resolves to nothing, so a Stat-based detection would miss it")

	assert.True(t, devincli.New().Detect(project))
}

// TestNoSurfaceIsAMemoryFile asserts across every type that this adapter never
// resolves an asset to a file harnaas edits in place, so no installation it
// describes can add a line to one.
func TestNoSurfaceIsAMemoryFile(t *testing.T) {
	t.Parallel()

	for _, assetType := range manifest.AssetTypes() {
		destination, found := devincli.New().Destination(asset(assetType, "house-style", manifest.ScopeProject))
		if !found {
			continue
		}

		assert.NotContains(t, destination.Path, "AGENTS.md")
		assert.NotContains(t, destination.Path, "global_rules.md")
	}
}
