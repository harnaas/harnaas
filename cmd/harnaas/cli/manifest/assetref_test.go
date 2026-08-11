package manifest_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// TestParseAssetRefAcceptsBothForms covers the keyed remote form and the
// project-local form, which are the only two an asset entry may take.
func TestParseAssetRefAcceptsBothForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		source    string
		wantKey   string
		wantPath  string
		wantLocal bool
	}{
		{
			name:     "keyed remote form",
			source:   "acme:skills/review",
			wantKey:  "acme",
			wantPath: "skills/review",
		},
		{
			name:     "keyed form pointing at a file",
			source:   "acme:instructions/tone.md",
			wantKey:  "acme",
			wantPath: "instructions/tone.md",
		},
		{
			name:     "keyed form is cleaned without changing what it names",
			source:   "acme:./skills//review/",
			wantKey:  "acme",
			wantPath: "skills/review",
		},
		{
			name:      "project-local form",
			source:    ".harnaas/rules/house-style.md",
			wantPath:  ".harnaas/rules/house-style.md",
			wantLocal: true,
		},
		{
			name:      "project-local form staying inside .harnaas after cleaning",
			source:    ".harnaas/skills/../commands/ship",
			wantPath:  ".harnaas/commands/ship",
			wantLocal: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ref, violation := manifest.ParseAssetRef(0, test.source)
			require.Nil(t, violation)
			assert.Equal(t, test.wantKey, ref.SourceKey)
			assert.Equal(t, test.wantPath, ref.Path)
			assert.Equal(t, test.wantLocal, ref.Local())
		})
	}
}

// TestParseAssetRefRejects covers every string that is in neither form, plus
// the containment rules that must hold before any content is read.
func TestParseAssetRefRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		index     int
		source    string
		wantParts []string
	}{
		{
			name:      "an unprefixed relative path states both accepted forms",
			source:    "skills/review",
			wantParts: []string{"neither of the two accepted forms", "acme:skills/review", ".harnaas/rules/house-style.md"},
		},
		{
			name:      "an empty source",
			index:     2,
			source:    "",
			wantParts: []string{"the asset entry at index 2", "declares no source"},
		},
		{
			name:      "a URL",
			source:    "https://example.com/skills/review",
			wantParts: []string{"neither of the two accepted forms"},
		},
		{
			name:      "a POSIX absolute path",
			source:    "/srv/assets/skills/review",
			wantParts: []string{"absolute path", "every asset source is relative"},
		},
		{
			name:      "a Windows absolute path is refused on every platform",
			source:    `C:\assets\skills\review`,
			wantParts: []string{"absolute path", "every asset source is relative"},
		},
		{
			name:      "a UNC path",
			source:    `\\share\assets\skills\review`,
			wantParts: []string{"absolute path"},
		},
		{
			name:      "a local path escaping .harnaas upward",
			source:    ".harnaas/../../etc/passwd",
			wantParts: []string{"does not stay inside .harnaas"},
		},
		{
			name:      "the .harnaas root itself is not an asset",
			source:    ".harnaas/",
			wantParts: []string{"does not stay inside .harnaas"},
		},
		{
			name:      "a keyed path leaving the source root",
			source:    "acme:../secrets/key",
			wantParts: []string{"leaves the root of the source", `"acme"`},
		},
		{
			name:      "a keyed form with an empty path",
			source:    "acme:",
			wantParts: []string{"neither of the two accepted forms"},
		},
		{
			name:      "a backslash separator is not a path separator harnaas accepts",
			source:    `acme:skills\review`,
			wantParts: []string{"separates its path with a backslash", `"/"`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ref, violation := manifest.ParseAssetRef(test.index, test.source)
			require.NotNil(t, violation)
			assert.Equal(t, manifest.AssetRef{}, ref, "a rejected entry must not be half-parsed")
			assert.Equal(t, test.index, violation.Index)
			assert.Equal(t, assetFieldName(test.index), violation.Field)

			for _, part := range test.wantParts {
				assert.Contains(t, violation.String(), part)
			}
		})
	}
}

// TestCheckSourceKeyNamesTheKeyAndTheSourcesBlock covers the one rule that is a
// property of the document rather than of the string: the same asset entry is
// correct in one manifest and wrong in another.
func TestCheckSourceKeyNamesTheKeyAndTheSourcesBlock(t *testing.T) {
	t.Parallel()

	sources := map[string]manifest.Source{
		"acme":  {Key: "acme", Kind: manifest.SourceKindGitHub, Repository: "acme/assets", Ref: "v1.2.0"},
		"house": {Key: "house", Kind: manifest.SourceKindLocal, Repository: ".harnaas"},
	}

	declared, violation := manifest.ParseAssetRef(0, "acme:skills/review")
	require.Nil(t, violation)
	assert.Nil(t, manifest.CheckSourceKey(0, declared, sources))

	local, violation := manifest.ParseAssetRef(1, ".harnaas/rules/house-style.md")
	require.Nil(t, violation)
	assert.Nil(t, manifest.CheckSourceKey(1, local, sources), "the local form references no key")

	undeclared, violation := manifest.ParseAssetRef(2, "acmee:skills/review")
	require.Nil(t, violation)

	violation = manifest.CheckSourceKey(2, undeclared, sources)
	require.NotNil(t, violation)
	assert.Equal(t, 2, violation.Index)
	assert.Contains(t, violation.String(), `"acmee"`)
	assert.Contains(t, violation.String(), `"sources"`)
	assert.Contains(t, violation.String(), `"acme", "house"`, "the recognized keys are listed in a fixed order")
}

// TestCheckSourceKeyWithNoSourcesDeclared covers the manifest that declares no
// sources at all, where listing the keys that would have worked is not an
// available remedy.
func TestCheckSourceKeyWithNoSourcesDeclared(t *testing.T) {
	t.Parallel()

	ref, violation := manifest.ParseAssetRef(0, "acme:skills/review")
	require.Nil(t, violation)

	violation = manifest.CheckSourceKey(0, ref, nil)
	require.NotNil(t, violation)
	assert.Contains(t, violation.String(), `Declare "acme" in the "sources" block`)
}

// TestInferTypeAndID covers the inference the spec documents, in both entry
// forms, including the extension strip that makes `tone.md` and `tone` the same
// asset.
func TestInferTypeAndID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		source   string
		wantType manifest.AssetType
		wantID   string
	}{
		{"skill from a keyed path", "acme:skills/review", manifest.AssetTypeSkill, "review"},
		{"rule from a keyed path", "acme:rules/house-style.md", manifest.AssetTypeRule, "house-style"},
		{"instruction from a local path", ".harnaas/instructions/tone.md", manifest.AssetTypeInstruction, "tone"},
		{"command from a keyed path", "acme:commands/ship", manifest.AssetTypeCommand, "ship"},
		{"persona from an agents directory", "acme:agents/reviewer", manifest.AssetTypePersona, "reviewer"},
		{"a nested source keeps the containing directory as the type", "acme:packs/skills/review", manifest.AssetTypeSkill, "review"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ref, violation := manifest.ParseAssetRef(0, test.source)
			require.Nil(t, violation)

			assetType, typeViolation := manifest.InferType(0, ref)
			require.Nil(t, typeViolation)
			assert.Equal(t, test.wantType, assetType)

			id, idViolation := manifest.InferID(0, ref)
			require.Nil(t, idViolation)
			assert.Equal(t, test.wantID, id)
		})
	}
}

// TestInferTypeRejectsAnUnrecognizedDirectory asserts the failure directs the
// author to the object form rather than guessing a type, because a wrongly
// typed asset installs somewhere nobody asked for and nobody notices.
func TestInferTypeRejectsAnUnrecognizedDirectory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
	}{
		{"a directory naming no type", "acme:prompts/review"},
		{"no containing directory at all", "acme:review"},
		{"the .harnaas root as the containing directory", ".harnaas/tone.md"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ref, violation := manifest.ParseAssetRef(3, test.source)
			require.Nil(t, violation)

			assetType, typeViolation := manifest.InferType(3, ref)
			require.NotNil(t, typeViolation)
			assert.Empty(t, string(assetType))
			assert.Equal(t, 3, typeViolation.Index)
			assert.Contains(t, typeViolation.String(), "names none of the asset types")
			assert.Contains(t, typeViolation.String(), `"skills/"`)
			assert.Contains(t, typeViolation.String(), `"agents/"`)
			assert.Contains(t, typeViolation.String(), `"type"`, "the remedy directs the author to the object form")
		})
	}
}

// TestInferIDSurvivesAnUnrecognizedDirectory is why type and id are inferred
// separately: an object declaring `type` for an unconventional layout still
// wants its id inferred, and must not be refused for a directory name nobody is
// relying on.
func TestInferIDSurvivesAnUnrecognizedDirectory(t *testing.T) {
	t.Parallel()

	ref, violation := manifest.ParseAssetRef(0, "acme:prompts/review.md")
	require.Nil(t, violation)

	_, typeViolation := manifest.InferType(0, ref)
	require.NotNil(t, typeViolation)

	id, idViolation := manifest.InferID(0, ref)
	require.Nil(t, idViolation)
	assert.Equal(t, "review", id)
}

// TestInferIDKeepsADotfileLeaf pins the one case where stripping the extension
// would leave nothing at all.
func TestInferIDKeepsADotfileLeaf(t *testing.T) {
	t.Parallel()

	ref, violation := manifest.ParseAssetRef(0, "acme:rules/.editorconfig")
	require.Nil(t, violation)

	id, idViolation := manifest.InferID(0, ref)
	require.Nil(t, idViolation)
	assert.Equal(t, ".editorconfig", id)
}

// TestAssetTypesAreStableAcrossCalls keeps the list a diagnostic is built from
// identical on every run, and keeps a caller from reordering it in place.
func TestAssetTypesAreStableAcrossCalls(t *testing.T) {
	t.Parallel()

	want := []manifest.AssetType{
		manifest.AssetTypeSkill,
		manifest.AssetTypeRule,
		manifest.AssetTypeInstruction,
		manifest.AssetTypeCommand,
		manifest.AssetTypePersona,
	}
	assert.Equal(t, want, manifest.AssetTypes())

	first := manifest.AssetTypes()
	first[0] = "mutated"
	assert.Equal(t, want, manifest.AssetTypes(), "a caller cannot reorder the list in place")
}

// assetFieldName spells the field a violation about an asset's source names, so
// the tests assert on what a reader would open rather than on the package's own
// formatting call.
func assetFieldName(index int) string {
	return "assets[" + strconv.Itoa(index) + "].source"
}
