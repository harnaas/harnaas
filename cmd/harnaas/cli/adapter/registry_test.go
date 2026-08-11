package adapter_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// registryWith builds a registry holding one adapter per given harness, so a
// test can vary the set without touching the one the binary ships.
//
// The ids are arbitrary strings rather than roster entries because what is under
// test is the registry, and the roster has exactly one harness today — a listing
// asserted to be sorted needs more than one thing in it.
func registryWith(ids ...harness.ID) *adapter.Registry {
	registry := &adapter.Registry{}
	for _, id := range ids {
		registry.Register(&testAdapter{id: id})
	}
	return registry
}

func TestLookupReturnsTheAdapterForItsHarness(t *testing.T) {
	t.Parallel()

	registry := &adapter.Registry{}
	claude := claudeLike()
	registry.Register(claude)

	found, err := registry.Lookup(harness.ClaudeCode)

	require.NoError(t, err)
	assert.Same(t, claude, found)
}

// TestLookupOfAHarnessWithNoAdapterNamesItAndListsTheRegistered is the
// diagnostic half of task 5.4: a harness harnaas recognizes but cannot install a
// rule for has to be told apart from a harness harnaas has never heard of, which
// the manifest layer has already refused by this point.
func TestLookupOfAHarnessWithNoAdapterNamesItAndListsTheRegistered(t *testing.T) {
	t.Parallel()

	registry := registryWith(harness.ClaudeCode)

	found, err := registry.Lookup("codex")

	assert.Nil(t, found)
	var notRegistered *adapter.NotRegisteredError
	require.ErrorAs(t, err, &notRegistered)
	assert.Equal(t, harness.ID("codex"), notRegistered.Harness)
	assert.Equal(t, []harness.ID{harness.ClaudeCode}, notRegistered.Registered)
}

func TestNotRegisteredErrorStatesTheProblemThenTheFix(t *testing.T) {
	t.Parallel()

	err := &adapter.NotRegisteredError{
		Harness:    "codex",
		Registered: []harness.ID{harness.ClaudeCode, "cursor"},
	}

	problem, fix, separated := strings.Cut(err.Error(), "\n\n")

	require.True(t, separated, "the message separates the problem from the fix")
	assert.Contains(t, problem, `"codex"`)
	assert.NotContains(t, problem, "\n", "the problem is one line")
	assert.Contains(t, fix, `"claude-code", "cursor"`)
	assert.Contains(t, fix, manifest.FileName, "the remedy is an edit, and it names the file to edit")
	assert.NotContains(t, fix, "harnaas fix", "and never a command that makes the edit for the author")
}

// TestNotRegisteredErrorWithNoAdaptersStatesTheSituation covers a case a built
// binary cannot reach — it links at least one adapter — because the alternative
// is a remedy reading "a harness harnaas has an adapter for: ." if one ever does.
func TestNotRegisteredErrorWithNoAdaptersStatesTheSituation(t *testing.T) {
	t.Parallel()

	err := &adapter.NotRegisteredError{Harness: "codex"}

	_, fix, separated := strings.Cut(err.Error(), "\n\n")

	require.True(t, separated)
	assert.Contains(t, fix, "no adapters at all")
}

func TestRegisterPanicsOnASecondAdapterForOneHarness(t *testing.T) {
	t.Parallel()

	registry := registryWith(harness.ClaudeCode)

	assert.PanicsWithValue(t, `adapter: harness "claude-code" has two adapters registered`, func() {
		registry.Register(claudeLike())
	})
}

// TestHarnessesIsSortedRatherThanInRegistrationOrder matters because
// registration order is whatever the import graph produces, and this list is
// rendered into a message a user reads.
func TestHarnessesIsSortedRatherThanInRegistrationOrder(t *testing.T) {
	t.Parallel()

	registry := registryWith("cursor", harness.ClaudeCode, "codex")

	sorted := []harness.ID{harness.ClaudeCode, "codex", "cursor"}
	assert.Equal(t, sorted, registry.Harnesses())
	assert.Equal(t, sorted, registry.Harnesses(), "and the same list on every call")
}

func TestAdaptersAreListedInHarnessOrder(t *testing.T) {
	t.Parallel()

	registry := registryWith("cursor", harness.ClaudeCode, "codex")

	ids := make([]harness.ID, 0, 3)
	for _, listed := range registry.Adapters() {
		ids = append(ids, listed.Harness())
	}

	assert.Equal(t, registry.Harnesses(), ids)
}

func TestAnEmptyRegistryListsNothing(t *testing.T) {
	t.Parallel()

	var registry adapter.Registry

	assert.Empty(t, registry.Harnesses())
	assert.Empty(t, registry.Adapters())
	assert.False(t, registry.Registered(harness.ClaudeCode))
}

// TestRegisteredAnswersMembershipWithoutBuildingADiagnostic is why the predicate
// exists beside the lookup: the install flow asks whether a harness has an
// adapter long before it has an asset to attribute the answer to.
func TestRegisteredAnswersMembershipWithoutBuildingADiagnostic(t *testing.T) {
	t.Parallel()

	registry := registryWith(harness.ClaudeCode)

	assert.True(t, registry.Registered(harness.ClaudeCode))
	assert.False(t, registry.Registered("codex"))
}

// TestPackageLevelRegisterAddsToDefault pins the seam an adapter package's init
// reaches. A change that gave the package-level function a registry of its own
// would leave every adapter registered somewhere nothing consults, which is a
// binary that installs no rule, command or persona and says nothing about it.
//
// It is deliberately not parallel and deliberately not undone: the registry the
// binary itself uses is what is under test, and the set it holds is asserted
// whole where the adapters are linked, not here.
func TestPackageLevelRegisterAddsToDefault(t *testing.T) {
	const id harness.ID = "registered-by-this-test"

	registered := &testAdapter{id: id}
	adapter.Register(registered)

	found, err := adapter.Default.Lookup(id)

	require.NoError(t, err)
	assert.Same(t, registered, found)
	assert.Contains(t, adapter.Default.Harnesses(), id)
}
