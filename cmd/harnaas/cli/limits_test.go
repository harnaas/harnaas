package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

func TestSkillWithinTheLimitPasses(t *testing.T) {
	t.Parallel()

	assert.NoError(t, checkSkillSize("review", make([]byte, skillFileLimit)))
}

func TestSkillPastTheLimitNamesTheLimitTheValueAndTheAsset(t *testing.T) {
	t.Parallel()

	err := checkSkillSize("review", make([]byte, skillFileLimit+1))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "review")
	assert.Contains(t, err.Error(), "1048577", "the measured value")
	assert.Contains(t, err.Error(), "1048576", "and the limit")
	assert.Contains(t, err.Error(), "without saying so",
		"the reason it is a refusal rather than a warning is the silence, and the message has to say so")
}

func TestMemoryFilePastTheLimitIsRefused(t *testing.T) {
	t.Parallel()

	require.NoError(t, checkMemoryFileSize(make([]byte, memoryFileLimit)))
	assert.Error(t, checkMemoryFileSize(make([]byte, memoryFileLimit+1)))
}

func TestImportChainWithinTheDepthPasses(t *testing.T) {
	t.Parallel()

	files := map[string][]byte{
		"a.md": []byte("@b.md\n"),
		"b.md": []byte("@c.md\n"),
		"c.md": []byte("done\n"),
	}
	err := checkImportDepth("a.md", func(path string) ([]byte, bool) {
		content, found := files[path]
		return content, found
	})

	assert.NoError(t, err)
}

func TestImportChainPastTheDepthIsRefused(t *testing.T) {
	t.Parallel()

	// Six levels, one past the five a harness follows.
	files := map[string][]byte{
		"1.md": []byte("@2.md\n"), "2.md": []byte("@3.md\n"), "3.md": []byte("@4.md\n"),
		"4.md": []byte("@5.md\n"), "5.md": []byte("@6.md\n"), "6.md": []byte("end\n"),
	}
	err := checkImportDepth("1.md", func(path string) ([]byte, bool) {
		content, found := files[path]
		return content, found
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "import chain")
}

func TestACycleIsBoundedByTheDepthCeiling(t *testing.T) {
	t.Parallel()

	files := map[string][]byte{"a.md": []byte("@b.md\n"), "b.md": []byte("@a.md\n")}
	err := checkImportDepth("a.md", func(path string) ([]byte, bool) {
		content, found := files[path]
		return content, found
	})

	require.Error(t, err, "a cycle is refused at the same point a deep chain is, rather than looping")
}

func TestAnUnresolvableImportEndsTheBranchRatherThanFailing(t *testing.T) {
	t.Parallel()

	err := checkImportDepth("a.md", func(path string) ([]byte, bool) {
		if path == "a.md" {
			return []byte("@not-checked-out.md\n"), true
		}
		return nil, false
	})

	assert.NoError(t, err, "a memory file importing something absent is the author's arrangement, not this limit's business")
}

func TestAnIndentedImportIsNotAnImport(t *testing.T) {
	t.Parallel()

	_, isImport := importLine("    @b.md")
	assert.False(t, isImport, "an indented line is prose about an import inside somebody's list")

	_, isImport = importLine("@b.md and then some prose")
	assert.False(t, isImport)

	path, isImport := importLine("@b.md")
	assert.True(t, isImport)
	assert.Equal(t, "b.md", path)
}

func TestAlwaysOnContentUnderTheThresholdSaysNothing(t *testing.T) {
	t.Parallel()

	assert.Empty(t, alwaysOnWarning("project", []contribution{{AssetID: "tone", Characters: 100}}))
}

func TestAlwaysOnContentPastTheThresholdNamesTheTotalAndTheLargest(t *testing.T) {
	t.Parallel()

	warning := alwaysOnWarning("project", []contribution{
		{AssetID: "small", Characters: 100},
		{AssetID: "biggest", Characters: 30_000},
		{AssetID: "middle", Characters: 20_000},
	})

	require.NotEmpty(t, warning)
	assert.Contains(t, warning, "50100", "the total")
	assert.Contains(t, warning, `"biggest"`)
	assert.Contains(t, warning, "Nothing was changed",
		"it is a degradation point rather than something a harness enforces, so it must not read as a failure")
	assert.Less(t, strings.Index(warning, "biggest"), strings.Index(warning, "middle"),
		"the largest contributors come first, because the total alone is not actionable")
}

func TestAlwaysOnWarningIsStableAcrossRuns(t *testing.T) {
	t.Parallel()

	tied := []contribution{{AssetID: "b", Characters: 25_000}, {AssetID: "a", Characters: 25_000}}
	assert.Equal(t, alwaysOnWarning("project", tied), alwaysOnWarning("project", []contribution{tied[1], tied[0]}),
		"two runs over one project must say the same thing")
}

func TestAlwaysOnWarningNamesTheScopeItIsAbout(t *testing.T) {
	t.Parallel()

	// The threshold is per scope, because project-scoped and user-scoped
	// always-on content are assembled into different files and a reader has to
	// know which of the two to go and look at.
	over := []contribution{{AssetID: "tone", Characters: alwaysOnWarnThreshold + 1}}

	assert.Contains(t, alwaysOnWarning(string(manifest.ScopeUser), over), string(manifest.ScopeUser))
	assert.Contains(t, alwaysOnWarning(string(manifest.ScopeProject), over), string(manifest.ScopeProject))
}
