package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
)

// TestResolveScopeRejectsUserWhereTheRosterRecordsNoLocation covers the rule
// the real roster cannot exercise today, because every harness it lists has a
// per-user location.
//
// The rule still has to hold now: the moment a harness without one is added,
// the alternative to this error is installing the asset at project scope, which
// is somewhere the author did not ask for and would not notice.
func TestResolveScopeRejectsUserWhereTheRosterRecordsNoLocation(t *testing.T) {
	t.Parallel()

	const (
		withLocation    = harness.ID("with-location")
		withoutLocation = harness.ID("without-location")
	)
	perUserLocation := func(id harness.ID) (bool, error) {
		return id == withLocation, nil
	}

	entry := AssetEntry{Index: 6, ObjectForm: true, Scope: string(ScopeUser)}

	t.Run("one target without a location", func(t *testing.T) {
		t.Parallel()

		scope, violation := resolveScope(entry, AssetTypeSkill, []harness.ID{withoutLocation}, perUserLocation)
		require.NotNil(t, violation)
		assert.Empty(t, string(scope), "the asset is not silently installed at project scope")
		assert.Equal(t, "assets[6].scope", violation.Field)
		assert.Contains(t, violation.String(), `the target "without-location" has`)
		assert.Contains(t, violation.String(), "no unambiguous per-user location")
	})

	t.Run("one target of several", func(t *testing.T) {
		t.Parallel()

		targets := []harness.ID{withLocation, withoutLocation}
		scope, violation := resolveScope(entry, AssetTypeSkill, targets, perUserLocation)
		require.NotNil(t, violation)
		assert.Empty(t, string(scope), "user scope is refused for the whole entry, not for part of it")
		assert.Contains(t, violation.String(), `the target "without-location" has`)
	})

	t.Run("several targets without a location read as one sentence", func(t *testing.T) {
		t.Parallel()

		targets := []harness.ID{withoutLocation, harness.ID("also-without")}
		scope, violation := resolveScope(entry, AssetTypeSkill, targets, perUserLocation)
		require.NotNil(t, violation)
		assert.Empty(t, string(scope))
		assert.Contains(t, violation.String(), `the targets "without-location", "also-without" have`)
	})

	t.Run("every target has a location", func(t *testing.T) {
		t.Parallel()

		scope, violation := resolveScope(entry, AssetTypeSkill, []harness.ID{withLocation}, perUserLocation)
		require.Nil(t, violation)
		assert.Equal(t, ScopeUser, scope)
	})
}

// TestResolveScopeIgnoresATargetTheRosterDoesNotKnow keeps one mistake to one
// message: an unrecognized target has already been reported against the field
// that declared it, and reporting it again as a scope problem would name an
// edit that does not fix it.
func TestResolveScopeIgnoresATargetTheRosterDoesNotKnow(t *testing.T) {
	t.Parallel()

	entry := AssetEntry{Index: 0, ObjectForm: true, Scope: string(ScopeUser)}
	targets := []harness.ID{harness.ClaudeCode, "cursorr"}

	// The real roster is the point: its Lookup is what reports the unknown id,
	// and this asserts the scope rule leaves that report to the field that
	// declared the target.
	scope, violation := resolveScope(entry, AssetTypeSkill, targets, harness.HasPerUserLocation)
	require.Nil(t, violation)
	assert.Equal(t, ScopeUser, scope)
}
