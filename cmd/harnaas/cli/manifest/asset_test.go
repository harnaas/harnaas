package manifest_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// claudeCode is the one recognized harness, spelled as a manifest writes it.
const claudeCode = string(harness.ClaudeCode)

// TestResolveTypeAndIDUseWhatTheEntryDeclares covers the object form's whole
// purpose: a source laid out unconventionally is installable because the entry
// says what the path cannot.
func TestResolveTypeAndIDUseWhatTheEntryDeclares(t *testing.T) {
	t.Parallel()

	entry := manifest.AssetEntry{
		Index:      0,
		ObjectForm: true,
		Source:     "acme:prompts/review.md",
		Type:       string(manifest.AssetTypeSkill),
		ID:         "code-review",
	}

	ref, violation := manifest.ParseAssetRef(entry.Index, entry.Source)
	require.Nil(t, violation)

	assetType, typeViolation := manifest.ResolveType(entry, ref)
	require.Nil(t, typeViolation, "a declared type is not checked against the path it overrides")
	assert.Equal(t, manifest.AssetTypeSkill, assetType)

	id, idViolation := manifest.ResolveID(entry, ref)
	require.Nil(t, idViolation)
	assert.Equal(t, "code-review", id)
}

// TestResolveTypeAndIDInferWhereTheEntryIsSilent covers the common case, where
// one string per asset says everything.
func TestResolveTypeAndIDInferWhereTheEntryIsSilent(t *testing.T) {
	t.Parallel()

	entry := manifest.AssetEntry{Index: 1, Source: "acme:skills/review"}

	ref, violation := manifest.ParseAssetRef(entry.Index, entry.Source)
	require.Nil(t, violation)

	assetType, typeViolation := manifest.ResolveType(entry, ref)
	require.Nil(t, typeViolation)
	assert.Equal(t, manifest.AssetTypeSkill, assetType)

	id, idViolation := manifest.ResolveID(entry, ref)
	require.Nil(t, idViolation)
	assert.Equal(t, "review", id)
}

// TestDeclaringOneFieldSuppressesInferenceOnlyForThatField is why type and id
// are resolved separately: either one alone is a complete entry.
func TestDeclaringOneFieldSuppressesInferenceOnlyForThatField(t *testing.T) {
	t.Parallel()

	t.Run("a declared type leaves the id inferred", func(t *testing.T) {
		t.Parallel()

		entry := manifest.AssetEntry{
			Index:      0,
			ObjectForm: true,
			Source:     "acme:prompts/review.md",
			Type:       string(manifest.AssetTypeSkill),
		}

		ref, violation := manifest.ParseAssetRef(entry.Index, entry.Source)
		require.Nil(t, violation)

		assetType, typeViolation := manifest.ResolveType(entry, ref)
		require.Nil(t, typeViolation)
		assert.Equal(t, manifest.AssetTypeSkill, assetType)

		id, idViolation := manifest.ResolveID(entry, ref)
		require.Nil(t, idViolation)
		assert.Equal(t, "review", id)
	})

	t.Run("a declared id leaves the type inferred", func(t *testing.T) {
		t.Parallel()

		entry := manifest.AssetEntry{
			Index:      0,
			ObjectForm: true,
			Source:     "acme:skills/review",
			ID:         "acme-review",
		}

		ref, violation := manifest.ParseAssetRef(entry.Index, entry.Source)
		require.Nil(t, violation)

		assetType, typeViolation := manifest.ResolveType(entry, ref)
		require.Nil(t, typeViolation)
		assert.Equal(t, manifest.AssetTypeSkill, assetType)

		id, idViolation := manifest.ResolveID(entry, ref)
		require.Nil(t, idViolation)
		assert.Equal(t, "acme-review", id)
	})
}

// TestResolveTypeRejectsATypeHarnaasDoesNotInstall keeps the object form from
// being an escape hatch out of the vocabulary itself.
func TestResolveTypeRejectsATypeHarnaasDoesNotInstall(t *testing.T) {
	t.Parallel()

	entry := manifest.AssetEntry{
		Index:      2,
		ObjectForm: true,
		Source:     "acme:skills/review",
		Type:       "prompt",
	}

	ref, violation := manifest.ParseAssetRef(entry.Index, entry.Source)
	require.Nil(t, violation)

	assetType, typeViolation := manifest.ResolveType(entry, ref)
	require.NotNil(t, typeViolation)
	assert.Empty(t, string(assetType))
	assert.Equal(t, 2, typeViolation.Index)
	assert.Equal(t, "assets[2].type", typeViolation.Field)
	assert.Contains(t, typeViolation.String(), `"prompt"`)
	assert.Contains(t, typeViolation.String(), `"skill", "rule", "instruction", "command", "persona"`)
}

// TestResolveTargetsInheritsTheManifestHarnesses covers the default an asset
// gets by saying nothing, which is what keeps the common manifest one string
// per asset.
func TestResolveTargetsInheritsTheManifestHarnesses(t *testing.T) {
	t.Parallel()

	entry := manifest.AssetEntry{Index: 0, Source: "acme:skills/review"}

	targets, violations := manifest.ResolveTargets(entry, []string{claudeCode})
	assert.Empty(t, violations)
	assert.Equal(t, []harness.ID{harness.ClaudeCode}, targets)
}

// TestResolveTargetsPrefersTheEntrysOwnList covers the object form narrowing
// the guarantee for one asset.
func TestResolveTargetsPrefersTheEntrysOwnList(t *testing.T) {
	t.Parallel()

	entry := manifest.AssetEntry{
		Index:      0,
		ObjectForm: true,
		Source:     "acme:skills/review",
		Targets:    []string{claudeCode},
	}

	targets, violations := manifest.ResolveTargets(entry, nil)
	assert.Empty(t, violations, "an entry declaring its own targets needs no manifest default")
	assert.Equal(t, []harness.ID{harness.ClaudeCode}, targets)
}

// TestResolveTargetsRejectsAnEmptyEffectiveList covers both ways an asset ends
// up with nowhere to install, which need different edits and so get different
// messages.
func TestResolveTargetsRejectsAnEmptyEffectiveList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		entry     manifest.AssetEntry
		harnesses []string
		wantParts []string
	}{
		{
			name:      "nothing declared anywhere",
			entry:     manifest.AssetEntry{Index: 1, Source: "acme:skills/review"},
			wantParts: []string{"has no target harness", `"harnesses"`, `"claude-code"`},
		},
		{
			name: "an empty list declared on the entry",
			entry: manifest.AssetEntry{
				Index:      1,
				ObjectForm: true,
				Source:     "acme:skills/review",
				Targets:    []string{},
			},
			harnesses: []string{claudeCode},
			wantParts: []string{"declares an empty", "could never be installed anywhere"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			targets, violations := manifest.ResolveTargets(test.entry, test.harnesses)
			require.Len(t, violations, 1)
			assert.Nil(t, targets)
			assert.Equal(t, 1, violations[0].Index)
			assert.Equal(t, "assets[1].targets", violations[0].Field)

			for _, part := range test.wantParts {
				assert.Contains(t, violations[0].String(), part)
			}
		})
	}
}

// TestResolveTargetsReportsEveryUnrecognizedName asserts two mistakes in one
// list are two edits, each attributed to its own position so the aggregate can
// order them.
func TestResolveTargetsReportsEveryUnrecognizedName(t *testing.T) {
	t.Parallel()

	entry := manifest.AssetEntry{
		Index:      3,
		ObjectForm: true,
		Source:     "acme:skills/review",
		Targets:    []string{"claude-codee", claudeCode, "cursorr"},
	}

	targets, violations := manifest.ResolveTargets(entry, nil)
	require.Len(t, violations, 2)
	assert.Nil(t, targets, "a rejected list must not install to the half of it that parsed")

	assert.Equal(t, "assets[3].targets[0]", violations[0].Field)
	assert.Contains(t, violations[0].String(), `"claude-codee"`)
	assert.Contains(t, violations[0].String(), `"claude-code"`, "the recognized harnesses are listed")

	assert.Equal(t, "assets[3].targets[2]", violations[1].Field)
	assert.Contains(t, violations[1].String(), `"cursorr"`)
}

// TestCheckHarnessesReportsTheDocumentsOwnList covers the one place an
// inherited harness name is checked, so a misspelling there is one message
// rather than one per asset that inherited it.
func TestCheckHarnessesReportsTheDocumentsOwnList(t *testing.T) {
	t.Parallel()

	assert.Empty(t, manifest.CheckHarnesses([]string{claudeCode}))
	assert.Empty(t, manifest.CheckHarnesses(nil), "an unused default is not a problem with the document")

	violations := manifest.CheckHarnesses([]string{claudeCode, "cursor"})
	require.Len(t, violations, 1)
	assert.Equal(t, manifest.DocumentIndex, violations[0].Index)
	assert.Equal(t, "harnesses[1]", violations[0].Field)
	assert.Contains(t, violations[0].String(), `"cursor"`)
	assert.Contains(t, violations[0].String(), `"claude-code"`)
}

// TestResolveScopeDefaultsToProject pins the default an entry gets by saying
// nothing, in both entry forms.
func TestResolveScopeDefaultsToProject(t *testing.T) {
	t.Parallel()

	targets := []harness.ID{harness.ClaudeCode}

	scope, violation := manifest.ResolveScope(manifest.AssetEntry{Index: 0}, manifest.AssetTypeSkill, targets)
	require.Nil(t, violation)
	assert.Equal(t, manifest.ScopeProject, scope)

	declared := manifest.AssetEntry{Index: 0, ObjectForm: true, Scope: string(manifest.ScopeProject)}
	scope, violation = manifest.ResolveScope(declared, manifest.AssetTypeInstruction, targets)
	require.Nil(t, violation, "project scope is always available, including to an instruction")
	assert.Equal(t, manifest.ScopeProject, scope)
}

// TestResolveScopeAcceptsUserWhereTheRosterRecordsALocation covers the accepted
// half of the rule against the real roster.
func TestResolveScopeAcceptsUserWhereTheRosterRecordsALocation(t *testing.T) {
	t.Parallel()

	entry := manifest.AssetEntry{Index: 0, ObjectForm: true, Scope: string(manifest.ScopeUser)}

	scope, violation := manifest.ResolveScope(entry, manifest.AssetTypeSkill, []harness.ID{harness.ClaudeCode})
	require.Nil(t, violation)
	assert.Equal(t, manifest.ScopeUser, scope)
}

// TestResolveScopeRejectsAScopeHarnaasDoesNotDefine keeps a misspelled scope
// from reading as the default it is not.
func TestResolveScopeRejectsAScopeHarnaasDoesNotDefine(t *testing.T) {
	t.Parallel()

	entry := manifest.AssetEntry{Index: 4, ObjectForm: true, Scope: "global"}

	scope, violation := manifest.ResolveScope(entry, manifest.AssetTypeSkill, []harness.ID{harness.ClaudeCode})
	require.NotNil(t, violation)
	assert.Empty(t, string(scope))
	assert.Equal(t, 4, violation.Index)
	assert.Equal(t, "assets[4].scope", violation.Field)
	assert.Contains(t, violation.String(), `"global"`)
	assert.Contains(t, violation.String(), `"project", "user"`)
}

// TestResolveScopeRejectsUserOnAnInstruction covers the definitional case: at
// user scope there is no committed file for the content to live in, so the type
// would mean nothing.
func TestResolveScopeRejectsUserOnAnInstruction(t *testing.T) {
	t.Parallel()

	entry := manifest.AssetEntry{Index: 5, ObjectForm: true, Scope: string(manifest.ScopeUser)}

	scope, violation := manifest.ResolveScope(entry, manifest.AssetTypeInstruction, []harness.ID{harness.ClaudeCode})
	require.NotNil(t, violation)
	assert.Empty(t, string(scope))
	assert.Equal(t, "assets[5].scope", violation.Field)
	assert.Contains(t, violation.String(), "instruction")
	assert.Contains(t, violation.String(), `"rule"`, "the remedy names the type for that intent")
}
