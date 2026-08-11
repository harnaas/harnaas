package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// twoInstalledPaths is the fixture the ordering assertions run against,
// declared out of sorted order so a test that read them as given would fail.
func twoInstalledPaths() []string {
	return []string{
		".claude/rules/house-style.md",
		".agents/skills/review",
	}
}

func TestIgnoreBlockAnchorsEveryEntry(t *testing.T) {
	t.Parallel()

	body, err := renderIgnoreBlock(twoInstalledPaths())
	require.NoError(t, err)

	for _, line := range strings.Split(strings.TrimSuffix(string(body), "\n"), "\n") {
		assert.True(t, strings.HasPrefix(line, "/"),
			"an unanchored entry matches at any depth, so %q would untrack paths harnaas never installed", line)
	}
}

func TestIgnoreBlockListsEachPathIndividually(t *testing.T) {
	t.Parallel()

	body, err := renderIgnoreBlock(twoInstalledPaths())
	require.NoError(t, err)

	assert.Equal(t, []string{
		"/.agents/skills/review",
		"/.claude/rules/house-style.md",
		"",
	}, strings.Split(string(body), "\n"))
}

func TestIgnoreBlockIsOrderedByEntryNotByPosition(t *testing.T) {
	t.Parallel()

	forward := twoInstalledPaths()
	reversed := []string{forward[1], forward[0]}

	forwardBody, err := renderIgnoreBlock(forward)
	require.NoError(t, err)
	reversedBody, err := renderIgnoreBlock(reversed)
	require.NoError(t, err)

	assert.Equal(t, string(forwardBody), string(reversedBody),
		"the block is a function of the installed set, so ordering the input differently changes nothing")
}

func TestIgnoreBlockCollapsesAPathInstalledForSeveralHarnesses(t *testing.T) {
	t.Parallel()

	body, err := renderIgnoreBlock([]string{".agents/skills/review", ".agents/skills/review"})
	require.NoError(t, err)

	assert.Equal(t, "/.agents/skills/review\n", string(body),
		"one path is one file however many harnesses claim it")
}

func TestIgnoreBlockRefusesAnEntryThatCoversAnother(t *testing.T) {
	t.Parallel()

	_, err := renderIgnoreBlock([]string{".agents/skills", ".agents/skills/review"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "/.agents/skills",
		"the refusal names the entry that would untrack somebody's hand-written skill")
}

func TestIgnoreBlockKeepsSiblingsWhoseNamesSharePrefix(t *testing.T) {
	t.Parallel()

	// `/.agents/skills` is a prefix of `/.agents/skills-extra` as a string and
	// is not an ancestor of it as a path, which is the case a raw prefix test
	// would refuse.
	body, err := renderIgnoreBlock([]string{".agents/skills-extra", ".agents/skills"})

	require.NoError(t, err)
	assert.Equal(t, []string{"/.agents/skills", "/.agents/skills-extra", ""},
		strings.Split(string(body), "\n"))
}

func TestWriteIgnoreBlockPreservesHandWrittenRules(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, ignoreFileName)
	before := "node_modules/\n*.log\n"
	require.NoError(t, os.WriteFile(path, []byte(before), 0o600))

	require.NoError(t, writeIgnoreBlock(root, twoInstalledPaths()))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(content), before),
		"everything outside the markers survives byte for byte")
	assert.Contains(t, string(content), "/.agents/skills/review")
}

func TestWriteIgnoreBlockPrunesARemovedDestination(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, writeIgnoreBlock(root, twoInstalledPaths()))
	require.NoError(t, writeIgnoreBlock(root, twoInstalledPaths()[:1]))

	content, err := os.ReadFile(filepath.Join(root, ignoreFileName))
	require.NoError(t, err)
	assert.Contains(t, string(content), "/.claude/rules/house-style.md")
	assert.NotContains(t, string(content), "/.agents/skills/review",
		"regenerating the whole block is what prunes a destination convergence removed")
}

func TestWriteIgnoreBlockRemovesTheBlockWhenNothingIsInstalled(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, ignoreFileName)
	require.NoError(t, os.WriteFile(path, []byte("node_modules/\n"), 0o600))

	require.NoError(t, writeIgnoreBlock(root, twoInstalledPaths()))
	require.NoError(t, writeIgnoreBlock(root, nil))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "node_modules/\n", string(content),
		"an empty block would claim harnaas has something to ignore here, and after a full uninstall it does not")
}
