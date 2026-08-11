package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hashBlock is a second comment syntax in a second file, declared here rather
// than in the package because no file harnaas writes today takes one. It exists
// so the editor's reusability is exercised rather than asserted: every rule
// below that runs against the memory file's block runs against a block whose
// markers share none of its characters.
func hashBlock(name string) managedBlock {
	return managedBlock{
		file:  ".gitignore",
		begin: "# harnaas:begin " + name,
		end:   "# harnaas:end " + name,
	}
}

func TestSetManagedBlockCreatesTheWholeFileWhenThereIsNoContent(t *testing.T) {
	t.Parallel()

	updated, err := setManagedBlock(nil, instructionBlock, []byte("body\n"))
	require.NoError(t, err)

	assert.Equal(t, []string{
		"<!-- harnaas:begin instructions -->",
		"body",
		"<!-- harnaas:end instructions -->",
		"",
	}, strings.Split(string(updated), "\n"))
}

func TestSetManagedBlockAppendsBeneathExistingContent(t *testing.T) {
	t.Parallel()

	content := []byte("# Agents\n\nHand written.\n")

	updated, err := setManagedBlock(content, instructionBlock, []byte("body\n"))
	require.NoError(t, err)

	assert.True(t, bytes.HasPrefix(updated, content),
		"the previous content must survive byte for byte at the head of the file")
	assert.Equal(t, []string{
		"# Agents",
		"",
		"Hand written.",
		"<!-- harnaas:begin instructions -->",
		"body",
		"<!-- harnaas:end instructions -->",
		"",
	}, strings.Split(string(updated), "\n"))
}

func TestSetManagedBlockGivesAFileWithNoFinalNewlineOne(t *testing.T) {
	t.Parallel()

	updated, err := setManagedBlock([]byte("no newline"), instructionBlock, []byte("body\n"))
	require.NoError(t, err)

	assert.Equal(t, []string{
		"no newline",
		"<!-- harnaas:begin instructions -->",
		"body",
		"<!-- harnaas:end instructions -->",
		"",
	}, strings.Split(string(updated), "\n"))
}

func TestSetManagedBlockRewritesOnlyTheRegionBetweenTheMarkers(t *testing.T) {
	t.Parallel()

	content := []byte(
		"# Agents\n" +
			"\n" +
			"Before.\n" +
			"<!-- harnaas:begin instructions -->\n" +
			"stale\n" +
			"<!-- harnaas:end instructions -->\n" +
			"\n" +
			"After.\n",
	)

	updated, err := setManagedBlock(content, instructionBlock, []byte("fresh\n"))
	require.NoError(t, err)

	assert.Equal(t, []string{
		"# Agents",
		"",
		"Before.",
		"<!-- harnaas:begin instructions -->",
		"fresh",
		"<!-- harnaas:end instructions -->",
		"",
		"After.",
		"",
	}, strings.Split(string(updated), "\n"))
}

func TestSetManagedBlockLeavesAMarkerThatDoesNotStandAlone(t *testing.T) {
	t.Parallel()

	// The marker is quoted in prose and indented inside a list item. Neither is
	// harnaas's region, and reading either as one would put a block in the
	// middle of somebody's sentence.
	content := []byte(
		"Write <!-- harnaas:begin instructions --> to open the block.\n" +
			"\n" +
			"- Example: <!-- harnaas:begin instructions -->\n",
	)

	updated, err := setManagedBlock(content, instructionBlock, []byte("body\n"))
	require.NoError(t, err)

	assert.True(t, bytes.HasPrefix(updated, content))
	assert.Equal(t, 1, strings.Count(string(updated), "\n<!-- harnaas:begin instructions -->\n"),
		"exactly one marker stands on a line of its own: the one harnaas just appended")
}

func TestSetManagedBlockAcceptsCarriageReturnsAndTrailingSpacesOnAMarker(t *testing.T) {
	t.Parallel()

	content := []byte(
		"Before.\r\n" +
			"<!-- harnaas:begin instructions -->  \r\n" +
			"stale\r\n" +
			"<!-- harnaas:end instructions -->\r\n" +
			"After.\r\n",
	)

	updated, err := setManagedBlock(content, instructionBlock, []byte("fresh\n"))
	require.NoError(t, err)

	assert.Equal(t, "Before.\r\n"+
		"<!-- harnaas:begin instructions -->\n"+
		"fresh\n"+
		"<!-- harnaas:end instructions -->\n"+
		"After.\r\n", string(updated))
}

func TestSetManagedBlockTerminatesABodyThatDoesNotEndInANewline(t *testing.T) {
	t.Parallel()

	updated, err := setManagedBlock(nil, instructionBlock, []byte("body"))
	require.NoError(t, err)

	assert.Equal(t, "<!-- harnaas:begin instructions -->\nbody\n<!-- harnaas:end instructions -->\n",
		string(updated))
}

func TestSetManagedBlockWithAnEmptyBodyWritesTheMarkersAlone(t *testing.T) {
	t.Parallel()

	updated, err := setManagedBlock(nil, instructionBlock, nil)
	require.NoError(t, err)

	assert.Equal(t, "<!-- harnaas:begin instructions -->\n<!-- harnaas:end instructions -->\n",
		string(updated))
}

func TestManagedBlockRoundTripRestoresTheOriginalBytes(t *testing.T) {
	t.Parallel()

	for _, block := range []managedBlock{instructionBlock, hashBlock("installed")} {
		original := []byte("# Agents\n\nHand written.\n")

		withBlock, err := setManagedBlock(original, block, []byte("body\n"))
		require.NoError(t, err)
		require.NotEqual(t, string(original), string(withBlock))

		restored, err := removeManagedBlock(withBlock, block)
		require.NoError(t, err)

		// The block is appended with no separating blank line precisely so this
		// holds: an install followed by an uninstall leaves the file the team
		// wrote, not the file plus whatever harnaas used as spacing.
		assert.Equal(t, string(original), string(restored))
	}
}

func TestRemoveManagedBlockKeepsEverythingAroundIt(t *testing.T) {
	t.Parallel()

	content := []byte(
		"node_modules/\n" +
			"# harnaas:begin installed\n" +
			"/.claude/rules/review.md\n" +
			"# harnaas:end installed\n" +
			"*.log\n",
	)

	updated, err := removeManagedBlock(content, hashBlock("installed"))
	require.NoError(t, err)

	assert.Equal(t, "node_modules/\n*.log\n", string(updated))
}

func TestRemoveManagedBlockLeavesAFileWithNoBlockAlone(t *testing.T) {
	t.Parallel()

	content := []byte("# Agents\n")

	updated, err := removeManagedBlock(content, instructionBlock)
	require.NoError(t, err)

	assert.Equal(t, string(content), string(updated))
}

func TestManagedBlockRefusesMarkersThatDoNotPairUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		reason  blockMarkerReason
		marker  string
	}{
		{
			name:    "opened and never closed",
			content: "before\n<!-- harnaas:begin instructions -->\nbody\n",
			reason:  blockUnclosed,
			marker:  instructionBlock.begin,
		},
		{
			name:    "closed without being opened",
			content: "body\n<!-- harnaas:end instructions -->\n",
			reason:  blockUnopened,
			marker:  instructionBlock.end,
		},
		{
			name: "opened twice",
			content: "<!-- harnaas:begin instructions -->\na\n<!-- harnaas:end instructions -->\n" +
				"<!-- harnaas:begin instructions -->\nb\n<!-- harnaas:end instructions -->\n",
			reason: blockDuplicated,
			marker: instructionBlock.begin,
		},
		{
			name:    "opened again before being closed",
			content: "<!-- harnaas:begin instructions -->\n<!-- harnaas:begin instructions -->\n",
			reason:  blockDuplicated,
			marker:  instructionBlock.begin,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			for operation, run := range map[string]func() ([]byte, error){
				"set": func() ([]byte, error) {
					return setManagedBlock([]byte(test.content), instructionBlock, []byte("body\n"))
				},
				"remove": func() ([]byte, error) {
					return removeManagedBlock([]byte(test.content), instructionBlock)
				},
			} {
				updated, err := run()
				assert.Nil(t, updated, "%s must produce no content at all when it refuses", operation)

				var blockErr *managedBlockError
				require.ErrorAs(t, err, &blockErr)
				assert.Equal(t, test.reason, blockErr.Reason)
				assert.Equal(t, test.marker, blockErr.Marker)
				assert.Equal(t, memoryFileName, blockErr.File)
			}
		})
	}
}

func TestManagedBlockErrorStatesTheProblemThenTheFix(t *testing.T) {
	t.Parallel()

	for _, reason := range []blockMarkerReason{blockUnclosed, blockUnopened, blockDuplicated} {
		err := &managedBlockError{File: memoryFileName, Reason: reason, Marker: instructionBlock.begin}

		parts := strings.SplitN(err.Error(), "\n\n", 2)
		require.Len(t, parts, 2, "a diagnostic is a problem, a blank line and a fix")
		assert.Contains(t, parts[0], memoryFileName)
		assert.Contains(t, parts[1], "Edit "+memoryFileName,
			"the remedy is an edit to the file the reader has open")
		assert.NotContains(t, parts[1], "`", "no remedy offers a command that repairs the markers for them")
	}
}

func TestWriteManagedBlockCreatesTheFileWhenItIsAbsent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	require.NoError(t, writeManagedBlock(root, instructionBlock, []byte("body\n")))

	content, err := os.ReadFile(filepath.Join(root, memoryFileName))
	require.NoError(t, err)
	assert.Equal(t, "<!-- harnaas:begin instructions -->\nbody\n<!-- harnaas:end instructions -->\n",
		string(content))
}

func TestWriteManagedBlockLeavesAnUnchangedFileUntouched(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, memoryFileName)

	require.NoError(t, writeManagedBlock(root, instructionBlock, []byte("body\n")))

	before, err := os.Stat(path)
	require.NoError(t, err)

	require.NoError(t, writeManagedBlock(root, instructionBlock, []byte("body\n")))

	after, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, before.ModTime(), after.ModTime(),
		"a run that would write the bytes already there must not write at all")
}

func TestWriteManagedBlockLeavesNoStagingFileBehind(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	require.NoError(t, writeManagedBlock(root, instructionBlock, []byte("body\n")))

	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	assert.Equal(t, []string{memoryFileName}, names)
}

func TestDropManagedBlockOnAnAbsentFileCreatesNothing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	require.NoError(t, dropManagedBlock(root, instructionBlock))

	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries, "removing a block from a file that is not there must not create one")
}

func TestDropManagedBlockKeepsTheFileWhenTheBlockWasAllOfIt(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	require.NoError(t, writeManagedBlock(root, instructionBlock, []byte("body\n")))
	require.NoError(t, dropManagedBlock(root, instructionBlock))

	content, err := os.ReadFile(filepath.Join(root, memoryFileName))
	require.NoError(t, err)
	assert.Empty(t, content, "what harnaas owns is the region between its markers, not the file")
}

func TestWriteManagedBlockRefusesAFileWhoseMarkersDoNotPairUp(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, memoryFileName)
	original := "before\n<!-- harnaas:begin instructions -->\nbody\n"
	require.NoError(t, os.WriteFile(path, []byte(original), managedFilePerm))

	err := writeManagedBlock(root, instructionBlock, []byte("fresh\n"))

	var blockErr *managedBlockError
	require.ErrorAs(t, err, &blockErr)
	assert.Equal(t, memoryFileName, blockErr.File)

	content, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, original, string(content), "a refusal writes nothing")
}
