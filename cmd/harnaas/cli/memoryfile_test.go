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
