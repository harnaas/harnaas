package source_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

func TestFrontmatterIsSplitFromTheBodyItPrecedes(t *testing.T) {
	t.Parallel()

	block, present := source.SplitFrontmatter([]byte("---\nname: review\n---\n# Review\n\nProse.\n"))

	require.True(t, present)
	assert.Equal(t, "name: review\n", string(block.Raw))
	assert.Equal(t, "# Review\n\nProse.\n", string(block.Body))
}

// TestFrontmatterWrittenWithCarriageReturnsIsRecognized keeps the check from
// depending on which machine committed the skill: a file checked out with CRLF
// endings opens with the same three characters and one more byte.
func TestFrontmatterWrittenWithCarriageReturnsIsRecognized(t *testing.T) {
	t.Parallel()

	block, present := source.SplitFrontmatter([]byte("---\r\nname: review\r\n---\r\nbody\r\n"))

	require.True(t, present)
	assert.Equal(t, "name: review\r\n", string(block.Raw))

	var declared struct {
		Name string `yaml:"name"`
	}
	require.NoError(t, block.Decode(&declared))
	assert.Equal(t, "review", declared.Name)
}

func TestADocumentWithNoFrontmatterReportsNoBlock(t *testing.T) {
	t.Parallel()

	for name, content := range map[string]string{
		"prose only":            "# Review\n",
		"delimiter below prose": "# Review\n---\nname: review\n---\n",
		"never closed":          "---\nname: review\n",
		"empty":                 "",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, present := source.SplitFrontmatter([]byte(content))
			assert.False(t, present)
		})
	}
}

// TestAHorizontalRuleInTheBodyDoesNotExtendTheBlock pins the closing delimiter
// to a whole line. A body beginning with a Markdown rule is common, and a match
// on the prefix would swallow the prose above it into the frontmatter.
func TestAHorizontalRuleInTheBodyDoesNotExtendTheBlock(t *testing.T) {
	t.Parallel()

	block, present := source.SplitFrontmatter([]byte("---\nname: review\n---\nintro\n\n----\n\nmore\n"))

	require.True(t, present)
	assert.Equal(t, "name: review\n", string(block.Raw))
	assert.Equal(t, "intro\n\n----\n\nmore\n", string(block.Body))
}

// TestAnEmptyBlockSplitsAndDecodesToNothing covers the document that declares a
// block and puts nothing in it, which is a skill with no name rather than a
// skill with no frontmatter — two reasons the caller reports separately.
func TestAnEmptyBlockSplitsAndDecodesToNothing(t *testing.T) {
	t.Parallel()

	block, present := source.SplitFrontmatter([]byte("---\n---\nbody\n"))

	require.True(t, present)
	assert.Empty(t, block.Raw)

	var declared struct {
		Name string `yaml:"name"`
	}
	require.NoError(t, block.Decode(&declared))
	assert.Empty(t, declared.Name)
}

// TestUnknownFieldsAreIgnored records that harnaas is one reader of a block that
// belongs to its author: a skill carrying fields for other harnesses decodes.
func TestUnknownFieldsAreIgnored(t *testing.T) {
	t.Parallel()

	block, present := source.SplitFrontmatter([]byte("---\nname: review\nallowed-tools: [Read]\nlicense: MIT\n---\n"))
	require.True(t, present)

	var declared struct {
		Name string `yaml:"name"`
	}
	require.NoError(t, block.Decode(&declared))
	assert.Equal(t, "review", declared.Name)
}

func TestABlockThatIsNotYAMLFailsToDecode(t *testing.T) {
	t.Parallel()

	block, present := source.SplitFrontmatter([]byte("---\nname: [review\n---\nbody\n"))
	require.True(t, present)

	var declared struct {
		Name string `yaml:"name"`
	}
	require.Error(t, block.Decode(&declared))
}

// TestTheBlockIsKeptExactlyAsItArrived is the pass-through guarantee stated as a
// test: the bytes a caller can reach are the bytes that were in the file, so
// nothing downstream can install a re-serialized frontmatter even by accident.
func TestTheBlockIsKeptExactlyAsItArrived(t *testing.T) {
	t.Parallel()

	const raw = "name:   review\n# a comment nobody should lose\ndescription: >-\n  folded\n  text\n"

	block, present := source.SplitFrontmatter([]byte("---\n" + raw + "---\nbody\n"))

	require.True(t, present)
	assert.Equal(t, raw, string(block.Raw))
}
