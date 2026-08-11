package source_test

import (
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// verifiable builds the request and the resolved source verification is handed,
// exactly as the registry would: the asset carries the type and the id, and the
// files carry the paths the kind produced relative to what the entry named.
func verifiable(
	tb testing.TB,
	assetType manifest.AssetType,
	id, subtree string,
	content map[string][]byte,
) (source.Request, *source.Resolved) {
	tb.Helper()

	resolved, err := source.NewResolved(
		source.Provenance{Kind: manifest.SourceKindLocal, Source: subtree},
		content,
	)
	require.NoError(tb, err)

	req := source.Request{
		Asset: manifest.Asset{Type: assetType, ID: id, Ref: manifest.AssetRef{Path: subtree}},
	}
	return req, resolved
}

// skillDocument is a SKILL.md declaring the given name.
func skillDocument(name string) []byte {
	return []byte("---\nname: " + name + "\ndescription: Reviews code.\n---\n\n# Review\n")
}

func TestASkillDirectoryNamingItselfCorrectlyVerifies(t *testing.T) {
	t.Parallel()

	req, resolved := verifiable(t, manifest.AssetTypeSkill, "review", "skills/review", map[string][]byte{
		"SKILL.md":              skillDocument("review"),
		"references/checks.md":  []byte("checks\n"),
		"references/rubric.md":  []byte("rubric\n"),
		"scripts/collect.sh":    []byte("#!/bin/sh\n"),
		"references/README.txt": []byte("read me\n"),
	})

	require.NoError(t, source.Verify(req, resolved))
}

// TestVerificationRewritesNothing is the read-only half of the name check. The
// bytes are compared after a run that had every reason to touch them.
func TestVerificationRewritesNothing(t *testing.T) {
	t.Parallel()

	document := skillDocument("review")
	original := string(document)

	req, resolved := verifiable(t, manifest.AssetTypeSkill, "review", "skills/review", map[string][]byte{
		"SKILL.md": document,
	})
	require.NoError(t, source.Verify(req, resolved))

	assert.Equal(t, original, string(resolved.Files[0].Content))
}

func TestASkillResolvingToASingleFileIsRefusedAsTheWrongShape(t *testing.T) {
	t.Parallel()

	req, resolved := verifiable(t, manifest.AssetTypeSkill, "review", "skills/review.md", map[string][]byte{
		"review.md": skillDocument("review"),
	})

	err := source.Verify(req, resolved)

	var shape *source.ShapeError
	require.ErrorAs(t, err, &shape)
	assert.Equal(t, "review", shape.AssetID)
	assert.Equal(t, manifest.AssetTypeSkill, shape.Type)
	assert.Contains(t, shape.Expected, "directory")
	assert.Equal(t, "a single file", shape.Found)
}

func TestASkillDirectoryWithNoSkillFileNamesTheMissingFile(t *testing.T) {
	t.Parallel()

	req, resolved := verifiable(t, manifest.AssetTypeSkill, "review", "skills/review", map[string][]byte{
		"readme.md":            []byte("# Review\n"),
		"references/checks.md": []byte("checks\n"),
	})

	err := source.Verify(req, resolved)

	var missing *source.MissingSkillFileError
	require.ErrorAs(t, err, &missing)
	assert.Equal(t, "review", missing.AssetID)
	assert.Equal(t, "skills/review", missing.Source)
	assert.Contains(t, err.Error(), source.SkillFileName)
}

func TestAPersonaResolvingToADirectoryIsRefused(t *testing.T) {
	t.Parallel()

	req, resolved := verifiable(t, manifest.AssetTypePersona, "reviewer", "agents/reviewer", map[string][]byte{
		"reviewer.md": []byte("# Reviewer\n"),
		"notes.md":    []byte("notes\n"),
	})

	err := source.Verify(req, resolved)

	var shape *source.ShapeError
	require.ErrorAs(t, err, &shape)
	assert.Equal(t, "a single file", shape.Expected)
	assert.Equal(t, "a directory holding 2 files", shape.Found)
}

// TestEveryNonSkillTypeIsHeldToTheSingleFileShape runs the rule over all four
// types, because the shape requirement belongs to "not a skill" rather than to
// any one of them, and a switch that grew a case would otherwise be silent.
func TestEveryNonSkillTypeIsHeldToTheSingleFileShape(t *testing.T) {
	t.Parallel()

	subtrees := map[manifest.AssetType]string{
		manifest.AssetTypeRule:        "rules/house-style",
		manifest.AssetTypeInstruction: "instructions/context",
		manifest.AssetTypeCommand:     "commands/ship",
		manifest.AssetTypePersona:     "agents/reviewer",
	}

	for assetType, subtree := range subtrees {
		t.Run(string(assetType), func(t *testing.T) {
			t.Parallel()

			id := path.Base(subtree)

			single, resolvedSingle := verifiable(t, assetType, id, subtree+".md", map[string][]byte{
				id + ".md": []byte("content\n"),
			})
			require.NoError(t, source.Verify(single, resolvedSingle))

			many, resolvedMany := verifiable(t, assetType, id, subtree, map[string][]byte{
				"one.md": []byte("one\n"),
				"two.md": []byte("two\n"),
			})
			err := source.Verify(many, resolvedMany)

			var shape *source.ShapeError
			require.ErrorAs(t, err, &shape)
			assert.Contains(t, err.Error(), id, "the diagnostic names the asset")
			assert.Contains(t, err.Error(), string(assetType), "the diagnostic names the type")
		})
	}
}

// TestADirectoryHoldingOneFileIsStillADirectory keeps the count in the message
// honest for the case that reads most like a single file.
func TestADirectoryHoldingOneFileIsStillADirectory(t *testing.T) {
	t.Parallel()

	req, resolved := verifiable(t, manifest.AssetTypeRule, "house-style", "rules/house-style", map[string][]byte{
		"house-style.md.txt": []byte("content\n"),
	})

	err := source.Verify(req, resolved)

	var shape *source.ShapeError
	require.ErrorAs(t, err, &shape)
	assert.Equal(t, "a directory holding 1 file", shape.Found)
}

func TestASkillNameThatDisagreesWithTheIDReportsBothNames(t *testing.T) {
	t.Parallel()

	req, resolved := verifiable(t, manifest.AssetTypeSkill, "review", "skills/review", map[string][]byte{
		"SKILL.md": skillDocument("code-review"),
	})

	err := source.Verify(req, resolved)

	var mismatch *source.SkillNameMismatchError
	require.ErrorAs(t, err, &mismatch)
	assert.Equal(t, "review", mismatch.AssetID)
	assert.Equal(t, "code-review", mismatch.DeclaredName)
	assert.Contains(t, err.Error(), `"review"`)
	assert.Contains(t, err.Error(), `"code-review"`)
}

// TestASkillFileHarnaasCannotReadANameFromIsReported covers the three ways the
// name is unavailable, which are one diagnostic with three reasons because the
// reader's next action is the same in all of them.
func TestASkillFileHarnaasCannotReadANameFromIsReported(t *testing.T) {
	t.Parallel()

	documents := map[string]struct {
		content []byte
		reason  string
		wrapped bool
	}{
		"absent": {content: []byte("# Review\n\nProse.\n"), reason: "no frontmatter"},
		"unterminated": {
			content: []byte("---\nname: review\n"),
			reason:  "no frontmatter",
		},
		"unparseable": {
			content: []byte("---\nname: [review\ndescription: x\n---\n"),
			reason:  "not valid YAML",
			wrapped: true,
		},
		"nameless": {content: []byte("---\ndescription: Reviews code.\n---\n"), reason: "no name"},
	}

	for name, document := range documents {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			req, resolved := verifiable(t, manifest.AssetTypeSkill, "review", "skills/review", map[string][]byte{
				"SKILL.md": document.content,
			})

			err := source.Verify(req, resolved)

			var frontmatter *source.SkillFrontmatterError
			require.ErrorAs(t, err, &frontmatter)
			assert.Equal(t, "review", frontmatter.AssetID)
			assert.Contains(t, frontmatter.Reason, document.reason)
			assert.Contains(t, err.Error(), source.SkillFileName, "the diagnostic names the file")

			if document.wrapped {
				assert.Error(t, frontmatter.Err, "a parse failure stays inspectable")
			}
		})
	}
}

// TestEveryVerificationFailureIsShapedProblemThenFix holds the new diagnostics
// to the shape every user-facing message in harnaas has.
func TestEveryVerificationFailureIsShapedProblemThenFix(t *testing.T) {
	t.Parallel()

	failures := map[string]error{
		"shape": &source.ShapeError{
			AssetID:  "review",
			Type:     manifest.AssetTypeSkill,
			Expected: "a directory containing " + source.SkillFileName,
			Found:    "a single file",
		},
		"missing skill file": &source.MissingSkillFileError{AssetID: "review", Source: "skills/review"},
		"frontmatter":        &source.SkillFrontmatterError{AssetID: "review", Reason: "it has no frontmatter block"},
		"name mismatch":      &source.SkillNameMismatchError{AssetID: "review", DeclaredName: "code-review"},
	}

	for name, err := range failures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			message := err.Error()

			problem, fix, split := strings.Cut(message, "\n\n")
			require.True(t, split, "the problem and the fix must be separated by a blank line: %s", message)
			assert.NotContains(t, problem, "\n", "the problem is one line")
			assert.NotEmpty(t, fix)
			assert.Contains(t, message, "review", "every diagnostic names the asset")
		})
	}
}
