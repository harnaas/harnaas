package manifest_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// exampleManifest is the manifest the specification documents, written the way a
// project would commit it: every accepted asset form, one remote source and one
// local one.
//
// It is a fixture rather than a constructed document so the test covers the
// whole path a real file takes — strict decoding first, interpretation second —
// and would fail if either half stopped accepting what the documentation
// promises.
const exampleManifest = `{
  "version": 1,
  "harnesses": ["claude-code"],
  "sources": {
    "acme": "github:acme/assets@v1.2.0",
    "house": "local:.harnaas/house"
  },
  "assets": [
    "acme:skills/review",
    "acme:agents/reviewer",
    ".harnaas/instructions/tone.md",
    {
      "source": "acme:prompts/triage.md",
      "type": "command",
      "id": "triage",
      "targets": ["claude-code"],
      "scope": "user"
    }
  ]
}`

// TestInterpretAcceptsTheDocumentedExample proves the example a reader is shown
// loads and validates clean, and that every question each entry left open was
// answered the way the documentation says it would be.
func TestInterpretAcceptsTheDocumentedExample(t *testing.T) {
	t.Parallel()

	document, err := manifest.Decode([]byte(exampleManifest))
	require.NoError(t, err)

	interpretation, err := manifest.Interpret(document)
	require.NoError(t, err)
	require.NotNil(t, interpretation)

	assert.Same(t, document, interpretation.Document)
	assert.Equal(t, manifest.Source{
		Key: "acme", Kind: manifest.SourceKindGitHub, Repository: "acme/assets", Ref: "v1.2.0",
	}, interpretation.Sources["acme"])
	assert.Equal(t, manifest.Source{
		Key: "house", Kind: manifest.SourceKindLocal, Repository: ".harnaas/house",
	}, interpretation.Sources["house"])

	targets := []harness.ID{harness.ClaudeCode}
	assert.Equal(t, []manifest.Asset{
		{
			Index:   0,
			Ref:     manifest.AssetRef{SourceKey: "acme", Path: "skills/review"},
			Type:    manifest.AssetTypeSkill,
			ID:      "review",
			Targets: targets,
			Scope:   manifest.ScopeProject,
		},
		{
			Index:   1,
			Ref:     manifest.AssetRef{SourceKey: "acme", Path: "agents/reviewer"},
			Type:    manifest.AssetTypePersona,
			ID:      "reviewer",
			Targets: targets,
			Scope:   manifest.ScopeProject,
		},
		{
			Index:   2,
			Ref:     manifest.AssetRef{Path: ".harnaas/instructions/tone.md"},
			Type:    manifest.AssetTypeInstruction,
			ID:      "tone",
			Targets: targets,
			Scope:   manifest.ScopeProject,
		},
		{
			Index:      3,
			ObjectForm: true,
			Ref:        manifest.AssetRef{SourceKey: "acme", Path: "prompts/triage.md"},
			Type:       manifest.AssetTypeCommand,
			ID:         "triage",
			Targets:    targets,
			Scope:      manifest.ScopeUser,
		},
	}, interpretation.Assets)
}

// TestInterpretReportsEveryIndependentViolation is the reason interpretation
// accumulates rather than returning at the first problem: one run names every
// edit the file needs.
func TestInterpretReportsEveryIndependentViolation(t *testing.T) {
	t.Parallel()

	document, err := manifest.Decode([]byte(`{
	  "version": 1,
	  "harnesses": ["claude-code", "typo-harness"],
	  "sources": {"acme": "svn:acme/assets"},
	  "assets": [
	    "skills/review",
	    "acme:prompts/triage",
	    {"source": "acme:skills/review", "scope": "global"},
	    {"source": "acme:skills/other", "id": "nested/id"}
	  ]
	}`))
	require.NoError(t, err)

	interpretation, err := manifest.Interpret(document)
	require.Nil(t, interpretation, "a document with any violation is never handed to a later phase")

	var validation *manifest.ValidationError
	require.ErrorAs(t, err, &validation)

	fields := make([]string, 0, len(validation.Violations))
	for _, violation := range validation.Violations {
		fields = append(fields, violation.Field)
	}
	assert.Equal(t, []string{
		"harnesses[1]",
		"sources.acme",
		"assets[0].source",
		"assets[1].source",
		"assets[2].scope",
		"assets[3].id",
	}, fields, "every violation is reported, ordered by asset index then field")
}

// TestInterpretOrdersViolationsIdenticallyOnEveryRun pins the guarantee that two
// people running one command over one file read the same message. The `sources`
// block is what makes this worth asserting: it decodes into a map, whose
// iteration order differs between runs of the same binary.
func TestInterpretOrdersViolationsIdenticallyOnEveryRun(t *testing.T) {
	t.Parallel()

	document, err := manifest.Decode([]byte(`{
	  "version": 1,
	  "harnesses": ["claude-code"],
	  "sources": {
	    "a": "svn:one/assets",
	    "b": "svn:two/assets",
	    "c": "svn:three/assets",
	    "d": "svn:four/assets"
	  },
	  "assets": []
	}`))
	require.NoError(t, err)

	_, first := manifest.Interpret(document)
	require.Error(t, first)

	for range 20 {
		_, again := manifest.Interpret(document)
		require.Error(t, again)
		require.Equal(t, first.Error(), again.Error())
	}

	assert.Equal(t, []string{"sources.a", "sources.b", "sources.c", "sources.d"}, violationFields(t, first))
}

// TestInterpretRejectsADuplicateIDWithinAType covers the one question no single
// entry can answer about itself.
func TestInterpretRejectsADuplicateIDWithinAType(t *testing.T) {
	t.Parallel()

	document, err := manifest.Decode([]byte(`{
	  "version": 1,
	  "harnesses": ["claude-code"],
	  "sources": {"acme": "github:acme/assets@v1", "other": "github:other/assets@v1"},
	  "assets": ["acme:skills/review", "other:skills/review"]
	}`))
	require.NoError(t, err)

	_, err = manifest.Interpret(document)

	var validation *manifest.ValidationError
	require.ErrorAs(t, err, &validation)
	require.Len(t, validation.Violations, 1, "one collision is one violation, not one per entry")

	violation := validation.Violations[0]
	assert.Equal(t, "assets[1].id", violation.Field)
	assert.Contains(t, violation.Problem, "index 0")
	assert.Contains(t, violation.Problem, "index 1", "both entries are named")
	assert.Contains(t, violation.Problem, `skill "review"`)
}

// TestInterpretAllowsOneIDPerType is the other half of the uniqueness rule: each
// type is its own namespace, because that is how a harness addresses the
// installed asset.
func TestInterpretAllowsOneIDPerType(t *testing.T) {
	t.Parallel()

	document, err := manifest.Decode([]byte(`{
	  "version": 1,
	  "harnesses": ["claude-code"],
	  "sources": {"acme": "github:acme/assets@v1"},
	  "assets": ["acme:skills/review", "acme:commands/review"]
	}`))
	require.NoError(t, err)

	interpretation, err := manifest.Interpret(document)
	require.NoError(t, err)
	assert.Len(t, interpretation.Assets, 2)
}

// TestCheckIDRequiresASinglePathSegment covers every spelling that would place
// an installed file outside the directory harnaas manages.
func TestCheckIDRequiresASinglePathSegment(t *testing.T) {
	t.Parallel()

	rejected := []string{"nested/id", `nested\id`, "..", ".", "../escape", "a/../b", "/absolute"}
	for _, id := range rejected {
		t.Run("rejects "+id, func(t *testing.T) {
			t.Parallel()

			violation := manifest.CheckID(2, id)
			require.NotNil(t, violation)
			assert.Equal(t, "assets[2].id", violation.Field)
			assert.Contains(t, violation.Problem, "single path segment")
		})
	}

	accepted := []string{"review", "house-style", "tone.v2", ".editorconfig", "_private"}
	for _, id := range accepted {
		t.Run("accepts "+id, func(t *testing.T) {
			t.Parallel()

			assert.Nil(t, manifest.CheckID(0, id))
		})
	}
}

// TestCheckIDStaysSilentWithoutAnID proves the empty id is not reported twice:
// whatever left the entry without one has already said so.
func TestCheckIDStaysSilentWithoutAnID(t *testing.T) {
	t.Parallel()

	assert.Nil(t, manifest.CheckID(0, ""))
}

// TestInterpretDoesNotInventAProblemItCannotSee covers the entry whose source
// string did not parse. There is no path to infer a type or an id from, so
// asking would report problems the author never wrote.
func TestInterpretDoesNotInventAProblemItCannotSee(t *testing.T) {
	t.Parallel()

	document, err := manifest.Decode([]byte(`{
	  "version": 1,
	  "harnesses": ["claude-code"],
	  "sources": {},
	  "assets": ["not-a-source-string"]
	}`))
	require.NoError(t, err)

	_, err = manifest.Interpret(document)

	var validation *manifest.ValidationError
	require.ErrorAs(t, err, &validation)
	assert.Equal(t, []string{"assets[0].source"}, violationFields(t, err))
}

// TestInterpretBlamesTheSourceValueRatherThanTheAssetThatReferencesIt: a key
// whose value is malformed is still a declared key, and the asset using it has
// made no mistake of its own.
func TestInterpretBlamesTheSourceValueRatherThanTheAssetThatReferencesIt(t *testing.T) {
	t.Parallel()

	document, err := manifest.Decode([]byte(`{
	  "version": 1,
	  "harnesses": ["claude-code"],
	  "sources": {"acme": "github:acme/assets"},
	  "assets": ["acme:skills/review"]
	}`))
	require.NoError(t, err)

	_, err = manifest.Interpret(document)
	assert.Equal(t, []string{"sources.acme"}, violationFields(t, err))
}

// TestValidationErrorRendersEveryViolationUnderOneHeading covers the aggregate's
// own text: the file, how many problems it has, and each of them in the
// problem-then-fix shape every harnaas diagnostic takes.
func TestValidationErrorRendersEveryViolationUnderOneHeading(t *testing.T) {
	t.Parallel()

	t.Run("several problems", func(t *testing.T) {
		t.Parallel()

		err := &manifest.ValidationError{
			Path: "/repo/harnaas.json",
			Violations: []*manifest.Violation{
				{Index: manifest.DocumentIndex, Field: "harnesses[0]", Problem: "first problem", Fix: "First fix."},
				{Index: 1, Field: "assets[1].scope", Problem: "second problem", Fix: "Second fix."},
			},
		}

		assert.Equal(t, strings.Join([]string{
			"/repo/harnaas.json has 2 problems",
			"",
			"harnesses[0]: first problem",
			"",
			"First fix.",
			"",
			"assets[1].scope: second problem",
			"",
			"Second fix.",
		}, "\n"), err.Error())
	})

	t.Run("one problem reads as one", func(t *testing.T) {
		t.Parallel()

		err := &manifest.ValidationError{
			Violations: []*manifest.Violation{{Index: 0, Field: "assets[0].source", Problem: "only problem", Fix: "Only fix."}},
		}

		assert.Contains(t, err.Error(), manifest.FileName+" has 1 problem\n", "a document decoded from bytes names the file")
	})
}

// TestValidationRemediesAreEditsNeverCommands re-asserts, on the aggregate this
// time, that harnaas never offers to write the manifest for anyone.
func TestValidationRemediesAreEditsNeverCommands(t *testing.T) {
	t.Parallel()

	document, err := manifest.Decode([]byte(`{
	  "version": 1,
	  "harnesses": ["typo-harness"],
	  "sources": {"acme": "svn:acme/assets"},
	  "assets": ["skills/review", {"source": "acme:skills/x", "id": "a/b"}]
	}`))
	require.NoError(t, err)

	_, err = manifest.Interpret(document)

	var validation *manifest.ValidationError
	require.ErrorAs(t, err, &validation)
	require.NotEmpty(t, validation.Violations)

	for _, violation := range validation.Violations {
		assert.NotContains(t, violation.Fix, "harnaas add", violation.Field)
		assert.NotContains(t, violation.Fix, "harnaas update", violation.Field)
		assert.NotContains(t, violation.Fix, "--fix", violation.Field)
	}
}

// violationFields returns the field of every violation in an aggregate, which is
// what most of these tests are really asserting on: which parts of the document
// were blamed, and in what order.
func violationFields(t *testing.T, err error) []string {
	t.Helper()

	var validation *manifest.ValidationError
	require.ErrorAs(t, err, &validation)

	fields := make([]string, 0, len(validation.Violations))
	for _, violation := range validation.Violations {
		fields = append(fields, violation.Field)
	}
	return fields
}
