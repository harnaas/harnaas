package manifest_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// minimal is the documented shape: a version, one harness, one source key, and
// asset entries in the string form.
const minimal = `{
  "version": 1,
  "harnesses": ["claude-code"],
  "sources": {
    "acme": "github:acme/assets@v1.2.0"
  },
  "assets": [
    "acme:skills/review",
    ".harnaas/rules/house-style.md"
  ]
}
`

func TestMinimalManifestDecodes(t *testing.T) {
	t.Parallel()

	document, err := manifest.Decode([]byte(minimal))
	require.NoError(t, err)

	assert.Equal(t, manifest.SupportedVersion, document.Version)
	assert.Equal(t, []string{"claude-code"}, document.Harnesses)
	assert.Equal(t, map[string]string{"acme": "github:acme/assets@v1.2.0"}, document.Sources)
	assert.Empty(t, document.Path, "a document decoded from bytes has no file to name")

	require.Len(t, document.Assets, 2)
	assert.Equal(t, "acme:skills/review", document.Assets[0].Source)
	assert.Equal(t, 0, document.Assets[0].Index)
	assert.Equal(t, ".harnaas/rules/house-style.md", document.Assets[1].Source)
	assert.Equal(t, 1, document.Assets[1].Index,
		"an entry's index is how every diagnostic names it, so it must be its position in the array")
}

func TestBothAssetEntryFormsDecode(t *testing.T) {
	t.Parallel()

	document, err := manifest.Decode([]byte(`{
	  "version": 1,
	  "assets": [
	    "acme:skills/review",
	    {
	      "source": "acme:prompts/tone",
	      "type": "instruction",
	      "id": "tone",
	      "targets": ["claude-code"],
	      "scope": "project"
	    }
	  ]
	}`))
	require.NoError(t, err)
	require.Len(t, document.Assets, 2)

	stringForm := document.Assets[0]
	assert.False(t, stringForm.ObjectForm)
	assert.Equal(t, "acme:skills/review", stringForm.Source)
	assert.Empty(t, stringForm.Type, "the string form declares nothing but a source")
	assert.Empty(t, stringForm.ID)
	assert.Nil(t, stringForm.Targets)
	assert.Empty(t, stringForm.Scope)

	objectForm := document.Assets[1]
	assert.True(t, objectForm.ObjectForm)
	assert.Equal(t, "acme:prompts/tone", objectForm.Source)
	assert.Equal(t, "instruction", objectForm.Type)
	assert.Equal(t, "tone", objectForm.ID)
	assert.Equal(t, []string{"claude-code"}, objectForm.Targets)
	assert.Equal(t, "project", objectForm.Scope)
}

// TestDeclaredEmptyTargetsSurviveDecoding pins the distinction the
// interpretation layer depends on: an undeclared `targets` inherits the
// manifest's harnesses, while a declared empty one is an asset that could never
// be installed anywhere and must be reported as such. Decoding both to nil
// would erase the difference before anything could act on it.
func TestDeclaredEmptyTargetsSurviveDecoding(t *testing.T) {
	t.Parallel()

	document, err := manifest.Decode([]byte(`{
	  "version": 1,
	  "assets": [
	    {"source": "acme:skills/a"},
	    {"source": "acme:skills/b", "targets": []}
	  ]
	}`))
	require.NoError(t, err)
	require.Len(t, document.Assets, 2)

	assert.Nil(t, document.Assets[0].Targets, "an undeclared target list is nil")
	assert.NotNil(t, document.Assets[1].Targets, "a declared empty target list is not undeclared")
	assert.Empty(t, document.Assets[1].Targets)
}

func TestUnknownFieldIsRejectedNamingIt(t *testing.T) {
	t.Parallel()

	document, err := manifest.Decode([]byte(`{
	  "version": 1,
	  "assests": ["acme:skills/review"]
	}`))

	require.Nil(t, document, "a decode failure leaves every asset unprocessed")

	var decodeErr *manifest.DecodeError
	require.ErrorAs(t, err, &decodeErr)
	assert.Contains(t, decodeErr.Problem, `"assests"`, "the message must quote the field as written")
}

func TestUnknownFieldInAnAssetObjectNamesTheFieldAndTheEntry(t *testing.T) {
	t.Parallel()

	_, err := manifest.Decode([]byte(`{
	  "version": 1,
	  "assets": [
	    "acme:skills/review",
	    {"source": "acme:skills/tone", "scopes": "user"}
	  ]
	}`))

	var decodeErr *manifest.DecodeError
	require.ErrorAs(t, err, &decodeErr)
	assert.Contains(t, decodeErr.Problem, `"scopes"`)
	assert.Contains(t, decodeErr.Problem, "index 1", "the entry has no other name at decode time")
	assert.Zero(t, decodeErr.Line,
		"an entry is decoded from its own bytes, so any offset in the failure counts from the wrong place")
}

func TestMalformedJSONIsRejectedWithItsLocation(t *testing.T) {
	t.Parallel()

	document, err := manifest.Decode([]byte("{\n  \"version\": 1,\n  \"harnesses\": [\"claude-code\",]\n}\n"))

	require.Nil(t, document, "no partial manifest is used")

	var decodeErr *manifest.DecodeError
	require.ErrorAs(t, err, &decodeErr)
	assert.Equal(t, 3, decodeErr.Line, "the parse error names the line it stopped on")
	assert.Positive(t, decodeErr.Column)
	assert.Contains(t, err.Error(), "harnaas.json:3:")
}

func TestAssetsMustBeAnArray(t *testing.T) {
	t.Parallel()

	_, err := manifest.Decode([]byte(`{"version": 1, "assets": "acme:skills/review"}`))

	var decodeErr *manifest.DecodeError
	require.ErrorAs(t, err, &decodeErr)
	assert.Contains(t, decodeErr.Problem, `"assets"`)
	assert.Contains(t, decodeErr.Problem, "array")
}

func TestAssetEntryOfAnotherTypeNamesItsIndex(t *testing.T) {
	t.Parallel()

	for name, entry := range map[string]string{
		"number":  "7",
		"boolean": "true",
		"array":   `["acme:skills/review"]`,
		"null":    "null",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := manifest.Decode([]byte(`{"version": 1, "assets": ["acme:skills/a", ` + entry + `]}`))

			var decodeErr *manifest.DecodeError
			require.ErrorAs(t, err, &decodeErr)
			assert.Contains(t, decodeErr.Problem, "index 1")
			assert.Contains(t, decodeErr.Fix, "source", "the fix states both accepted forms")
		})
	}
}

func TestTopLevelMustBeAnObject(t *testing.T) {
	t.Parallel()

	_, err := manifest.Decode([]byte(`["acme:skills/review"]`))

	var decodeErr *manifest.DecodeError
	require.ErrorAs(t, err, &decodeErr)
	assert.Contains(t, decodeErr.Problem, "object")
}

// TestNewerVersionAsksTheUserToUpgrade covers why the version is read on its own
// before the strict pass. A manifest from a newer harnaas almost certainly
// carries fields this binary does not know, and reporting the first of them as
// an unknown field would send its author hunting for a typo in a correct file.
func TestNewerVersionAsksTheUserToUpgrade(t *testing.T) {
	t.Parallel()

	document, err := manifest.Decode([]byte(`{
	  "version": 99,
	  "harnesses": ["claude-code"],
	  "policies": {"pin": "strict"},
	  "assets": ["acme:skills/review"]
	}`))

	require.Nil(t, document, "no asset is processed from a manifest harnaas cannot read")

	var decodeErr *manifest.DecodeError
	require.ErrorAs(t, err, &decodeErr)
	assert.Contains(t, decodeErr.Problem, "99")
	assert.Contains(t, decodeErr.Fix, "Upgrade harnaas")
	assert.NotContains(t, err.Error(), "policies",
		"the unknown field is a symptom of the version, and naming it sends the author to the wrong edit")
}

func TestMissingVersionIsRejected(t *testing.T) {
	t.Parallel()

	_, err := manifest.Decode([]byte(`{"harnesses": ["claude-code"], "assets": []}`))

	var decodeErr *manifest.DecodeError
	require.ErrorAs(t, err, &decodeErr)
	assert.Contains(t, decodeErr.Problem, `"version"`)
	assert.Contains(t, decodeErr.Fix, `"version": 1`)
}

func TestNonIntegerVersionIsRejected(t *testing.T) {
	t.Parallel()

	for name, version := range map[string]string{
		"string":  `"1"`,
		"decimal": "1.5",
		"null":    "null",
		"array":   "[1]",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := manifest.Decode([]byte(`{"version": ` + version + `, "assets": []}`))

			var decodeErr *manifest.DecodeError
			require.ErrorAs(t, err, &decodeErr)
			assert.Contains(t, decodeErr.Problem, "version")
		})
	}
}

func TestVersionBelowTheDefinedOneIsRejected(t *testing.T) {
	t.Parallel()

	_, err := manifest.Decode([]byte(`{"version": 0, "assets": []}`))

	var decodeErr *manifest.DecodeError
	require.ErrorAs(t, err, &decodeErr)
	assert.NotContains(t, decodeErr.Fix, "Upgrade harnaas",
		"version 0 is not a manifest from the future, so upgrading would not help")
}

// TestTrailingDataIsRejected keeps a second top-level object from being dropped
// in silence: a streaming decoder stops at the end of the first value, and the
// author of the second would be reading declarations harnaas never saw.
func TestTrailingDataIsRejected(t *testing.T) {
	t.Parallel()

	document, err := manifest.Decode([]byte(`{"version": 1, "assets": []}{"version": 1}`))

	require.Nil(t, document)
	var decodeErr *manifest.DecodeError
	require.ErrorAs(t, err, &decodeErr)
	assert.Contains(t, decodeErr.Problem, "after top-level value")
}

// TestEveryDecodeFailureIsShapedProblemThenFix asserts the diagnostic contract
// over the whole failure surface rather than one message at a time: every one
// names the file, states the problem on one line, and gives an edit.
func TestEveryDecodeFailureIsShapedProblemThenFix(t *testing.T) {
	t.Parallel()

	broken := map[string]string{
		"malformed JSON":       `{"version": 1,}`,
		"unknown field":        `{"version": 1, "assests": []}`,
		"missing version":      `{"assets": []}`,
		"non-integer version":  `{"version": "1"}`,
		"newer version":        `{"version": 99}`,
		"assets not an array":  `{"version": 1, "assets": {}}`,
		"entry of wrong type":  `{"version": 1, "assets": [7]}`,
		"unknown entry field":  `{"version": 1, "assets": [{"source": "a:b", "typo": 1}]}`,
		"top level not object": `[]`,
		"trailing data":        `{"version": 1} []`,
	}

	for name, document := range broken {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := manifest.Decode([]byte(document))
			require.Error(t, err)

			message := err.Error()
			location, rest, found := strings.Cut(message, ": ")
			require.True(t, found, "a diagnostic names the file it is about: %s", message)
			assert.True(t, strings.HasPrefix(location, manifest.FileName),
				"a diagnostic names the manifest, not %q", location)

			problem, fix, split := strings.Cut(rest, "\n\n")
			require.True(t, split, "the problem and the fix must be separated by a blank line: %s", message)
			assert.NotContains(t, problem, "\n", "the problem is one line")
			assert.NotEmpty(t, fix)
			assert.NotContains(t, fix, "harnaas fix",
				"harnaas never writes the manifest, so no remedy may offer to")
		})
	}
}
