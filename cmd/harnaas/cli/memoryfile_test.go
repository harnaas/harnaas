package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// twoInstructions is the fixture the ordering and provenance assertions run
// against, declared out of order so a test that read them in the given order
// would fail.
func twoInstructions() []instruction {
	return []instruction{
		{ID: "review", Source: "acme:instructions/review.md", Content: []byte("Review carefully.\n")},
		{ID: "house-style", Source: ".harnaas/instructions/house-style.md", Content: []byte("Two spaces.\n")},
	}
}

func TestInstructionBlockCarriesContentAndProvenance(t *testing.T) {
	t.Parallel()

	body := renderInstructionBlock(twoInstructions())

	assert.Equal(t, []string{
		`<!-- harnaas instruction "house-style" from .harnaas/instructions/house-style.md -->`,
		"Two spaces.",
		"",
		`<!-- harnaas instruction "review" from acme:instructions/review.md -->`,
		"Review carefully.",
		"",
	}, strings.Split(string(body), "\n"))
}

func TestInstructionBlockIsOrderedByAssetIDNotByPosition(t *testing.T) {
	t.Parallel()

	forward := twoInstructions()
	reversed := []instruction{forward[1], forward[0]}

	assert.Equal(t, string(renderInstructionBlock(forward)), string(renderInstructionBlock(reversed)),
		"reordering the manifest must regenerate a byte-identical block")
}

func TestRenderInstructionBlockDoesNotReorderTheCallersSlice(t *testing.T) {
	t.Parallel()

	instructions := twoInstructions()
	renderInstructionBlock(instructions)

	assert.Equal(t, "review", instructions[0].ID,
		"the ordering is the block's, not a side effect on what the caller passed")
}

func TestInstructionBlockInlinesContentVerbatim(t *testing.T) {
	t.Parallel()

	// Frontmatter, CRLF, trailing whitespace and a blank final line: every one
	// of them is a byte an author chose, and none of them is harnaas's to
	// normalize on the way into a committed file.
	content := []byte("---\r\nname: verbatim\r\n---\r\n\r\nTrailing spaces:   \r\n\r\n")

	body := renderInstructionBlock([]instruction{{ID: "verbatim", Source: "local", Content: content}})

	assert.Contains(t, string(body), string(content))
}

func TestInstructionBlockTerminatesContentWithoutAFinalNewline(t *testing.T) {
	t.Parallel()

	body := renderInstructionBlock([]instruction{
		{ID: "a", Source: "s", Content: []byte("first")},
		{ID: "b", Source: "s", Content: []byte("second")},
	})

	assert.Equal(t, []string{
		`<!-- harnaas instruction "a" from s -->`,
		"first",
		"",
		`<!-- harnaas instruction "b" from s -->`,
		"second",
		"",
	}, strings.Split(string(body), "\n"))
}

func TestWriteInstructionBlockCreatesTheMemoryFileWhenAbsent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	require.NoError(t, writeInstructionBlock(root, twoInstructions()))

	content, err := os.ReadFile(filepath.Join(root, memoryFileName))
	require.NoError(t, err)

	assert.Equal(t, []string{
		"<!-- harnaas:begin instructions -->",
		`<!-- harnaas instruction "house-style" from .harnaas/instructions/house-style.md -->`,
		"Two spaces.",
		"",
		`<!-- harnaas instruction "review" from acme:instructions/review.md -->`,
		"Review carefully.",
		"<!-- harnaas:end instructions -->",
		"",
	}, strings.Split(string(content), "\n"))
}

func TestWriteInstructionBlockPreservesHandWrittenContent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, memoryFileName)
	before := "# Agents\n\nRead the README first.\n"
	require.NoError(t, os.WriteFile(path, []byte(before), managedFilePerm))

	require.NoError(t, writeInstructionBlock(root, twoInstructions()))
	require.NoError(t, writeInstructionBlock(root, twoInstructions()[:1]))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(content), before),
		"the team's own writing survives both the first install and the rewrite")
	assert.NotContains(t, string(content), "house-style")
}

func TestWriteInstructionBlockRemovesTheBlockWithTheLastInstruction(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, memoryFileName)
	before := "# Agents\n\nRead the README first.\n"
	require.NoError(t, os.WriteFile(path, []byte(before), managedFilePerm))

	require.NoError(t, writeInstructionBlock(root, twoInstructions()))
	require.NoError(t, writeInstructionBlock(root, nil))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, string(content),
		"both markers and everything between them go, and nothing else moves")
}

func TestWriteInstructionBlockIsIdempotent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, memoryFileName)

	require.NoError(t, writeInstructionBlock(root, twoInstructions()))
	first, err := os.ReadFile(path)
	require.NoError(t, err)

	require.NoError(t, writeInstructionBlock(root, twoInstructions()))
	second, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.Equal(t, string(first), string(second))
}

func TestBridgeLineIsCreatedHoldingOnlyItself(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, writeBridgeLine(root))

	content, err := os.ReadFile(filepath.Join(root, bridgeFileName))
	require.NoError(t, err)
	assert.Equal(t, bridgeLine, string(content),
		"a file harnaas created for the import holds the import and nothing else")
}

func TestBridgeLineIsNotDuplicatedOnReRun(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, writeBridgeLine(root))
	require.NoError(t, writeBridgeLine(root))

	content, err := os.ReadFile(filepath.Join(root, bridgeFileName))
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(content), bridgeLine))
}

func TestBridgeLineIsAppendedAfterExistingContent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, bridgeFileName)
	require.NoError(t, os.WriteFile(path, []byte("# House rules\nBe brief."), 0o600))

	require.NoError(t, writeBridgeLine(root))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "# House rules\nBe brief.\n"+bridgeLine, string(content),
		"a file not ending in a newline gets one, because an import sharing a line with a sentence is not an import")
}

func TestBridgeLineAlreadyPresentIsLeftWhereItIs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, bridgeFileName)
	before := bridgeLine + "\n\n# House rules\n"
	require.NoError(t, os.WriteFile(path, []byte(before), 0o600))

	require.NoError(t, writeBridgeLine(root))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, string(content),
		"the line works from anywhere, so moving it would be an edit that satisfies nothing")
}

func TestBridgeLineCollapsesDuplicatesKeepingTheFirst(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, bridgeFileName)
	require.NoError(t, os.WriteFile(path, []byte(bridgeLine+"\n# House rules\n"+bridgeLine+"\n"), 0o600))

	require.NoError(t, writeBridgeLine(root))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, bridgeLine+"\n# House rules\n", string(content),
		"the first is where the author put it; the rest are what a merge left behind")
}

func TestBridgeLineIgnoresAnIndentedMention(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, bridgeFileName)
	require.NoError(t, os.WriteFile(path, []byte("- like this:\n    "+bridgeLine+"\n"), 0o600))

	require.NoError(t, writeBridgeLine(root))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "- like this:\n    "+bridgeLine+"\n"+bridgeLine, string(content),
		"an indented mention is prose about the import inside somebody's list, not the import")
}

func TestDroppingTheBridgeLineRemovesAFileThatHeldOnlyIt(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, writeBridgeLine(root))
	require.NoError(t, dropBridgeLine(root))

	_, err := os.Stat(filepath.Join(root, bridgeFileName))
	assert.ErrorIs(t, err, os.ErrNotExist,
		"a full uninstall must not leave behind a file harnaas created and nobody wrote")
}

func TestDroppingTheBridgeLineKeepsAFileWithOtherContent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, bridgeFileName)
	require.NoError(t, os.WriteFile(path, []byte("# House rules\n"+bridgeLine+"\n"), 0o600))

	require.NoError(t, dropBridgeLine(root))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "# House rules\n", string(content),
		"the file is theirs; only the line harnaas added goes")
}

func TestDroppingTheBridgeLineIsSafeWhenThereIsNoFile(t *testing.T) {
	t.Parallel()

	assert.NoError(t, dropBridgeLine(t.TempDir()))
}
