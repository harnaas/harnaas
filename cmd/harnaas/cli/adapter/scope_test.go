package adapter_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// twoRooted is an adapter whose two scopes have *different* directories, which
// claudeLike's do not. Scope resolution picking the right one is only assertable
// where the two answers differ.
func twoRooted() *testAdapter {
	a := claudeLike()
	a.roots[manifest.ScopeUser] = ".config/claude"
	return a
}

func TestScopeResolutionReturnsTheAdaptersRootForTheAssetsScope(t *testing.T) {
	t.Parallel()

	a := twoRooted()

	project, err := adapter.ResolveRoot(a, asset(manifest.AssetTypeRule, "house-style", manifest.ScopeProject))
	require.NoError(t, err)
	assert.Equal(t, ".claude", project)

	user, err := adapter.ResolveRoot(a, asset(manifest.AssetTypeRule, "house-style", manifest.ScopeUser))
	require.NoError(t, err)
	assert.Equal(t, ".config/claude", user, "the scope selects the root, and the destination beneath it is unchanged")
}

// TestAnAssetWithNoScopeResolvesBeneathTheProjectRoot pins the default at the
// one place it could still be reached: an [manifest.Asset] always carries a
// scope, so this is about a zero value never producing a diagnostic about a
// scope nobody declared.
func TestAnAssetWithNoScopeResolvesBeneathTheProjectRoot(t *testing.T) {
	t.Parallel()

	a := twoRooted()

	undeclared, err := adapter.ResolveRoot(a, asset(manifest.AssetTypeRule, "house-style", ""))
	require.NoError(t, err)

	declared, err := adapter.ResolveRoot(a, asset(manifest.AssetTypeRule, "house-style", manifest.ScopeProject))
	require.NoError(t, err)

	assert.Equal(t, declared, undeclared)
}

// TestUserScopeOnAnAdapterThatDoesNotOfferItIsRefused is task 5.6. What the
// second assertion is really about is the failure it rules out: a resolution
// that quietly answered with the project root would install the asset somewhere
// its author did not ask for and would have no reason to look.
func TestUserScopeOnAnAdapterThatDoesNotOfferItIsRefused(t *testing.T) {
	t.Parallel()

	projectOnly := twoRooted()
	delete(projectOnly.roots, manifest.ScopeUser)

	root, err := adapter.ResolveRoot(projectOnly, asset(manifest.AssetTypeRule, "house-style", manifest.ScopeUser))

	assert.Empty(t, root, "no silent fall back to the project root")
	var unsupported *adapter.UnsupportedScopeError
	require.ErrorAs(t, err, &unsupported)
	assert.Equal(t, "house-style", unsupported.AssetID)
	assert.Equal(t, manifest.AssetTypeRule, unsupported.Type)
	assert.Equal(t, harness.ClaudeCode, unsupported.Harness)
	assert.Equal(t, manifest.ScopeUser, unsupported.Scope)
}

func TestUnsupportedScopeErrorStatesTheProblemThenTheFix(t *testing.T) {
	t.Parallel()

	err := &adapter.UnsupportedScopeError{
		AssetID: "house-style",
		Type:    manifest.AssetTypeRule,
		Harness: harness.ClaudeCode,
		Scope:   manifest.ScopeUser,
	}

	problem, fix, separated := strings.Cut(err.Error(), "\n\n")

	require.True(t, separated, "the message separates the problem from the fix")
	assert.Contains(t, problem, `"house-style"`, "the problem names the asset")
	assert.Contains(t, problem, `"claude-code"`, "and the harness it could not be placed on")
	assert.Contains(t, problem, `"user"`)
	assert.NotContains(t, problem, "\n", "the problem is one line")
	assert.Contains(t, fix, manifest.FileName, "the remedy is an edit, and it names the file to edit")
	assert.NotContains(t, fix, "harnaas ", "and never a command that makes the edit for the author")
}

// TestProjectScopeRefusalOffersAnEditThatExists covers a pairing a shipped
// adapter cannot produce — every adapter has a project directory — because the
// alternative is a remedy telling an author to remove a `scope` field they never
// wrote.
func TestProjectScopeRefusalOffersAnEditThatExists(t *testing.T) {
	t.Parallel()

	rootless := twoRooted()
	delete(rootless.roots, manifest.ScopeProject)

	_, err := adapter.ResolveRoot(rootless, asset(manifest.AssetTypeRule, "house-style", manifest.ScopeProject))

	require.Error(t, err)
	_, fix, separated := strings.Cut(err.Error(), "\n\n")
	require.True(t, separated)
	assert.Contains(t, fix, `"targets"`)
	assert.NotContains(t, fix, `Remove "scope"`, "there is no scope field on the entry to remove")
}

// TestScopeResolutionDoesNotRestateTheInstructionRule is task 5.7 stated as a
// test. An instruction at `user` scope is refused, but it is refused once, by
// the manifest layer, for a reason that is about what an instruction *is* rather
// than about any harness's directories — so this layer resolves it like any
// other asset, and the two halves are asserted together so a later change cannot
// quietly move the rule and leave both places disagreeing.
func TestScopeResolutionDoesNotRestateTheInstructionRule(t *testing.T) {
	t.Parallel()

	root, err := adapter.ResolveRoot(twoRooted(), asset(manifest.AssetTypeInstruction, "house-rules", manifest.ScopeUser))
	require.NoError(t, err, "the adapter layer answers about directories, not about what an instruction is")
	assert.Equal(t, ".config/claude", root)

	_, violation := manifest.ResolveScope(
		manifest.AssetEntry{Index: 0, Scope: string(manifest.ScopeUser)},
		manifest.AssetTypeInstruction,
		[]harness.ID{harness.ClaudeCode},
	)
	require.NotNil(t, violation, "and the refusal that matters has already happened upstream")
	assert.Contains(t, violation.Problem, string(manifest.AssetTypeInstruction))
}
