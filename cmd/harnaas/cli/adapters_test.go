package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
)

// TestBinaryRegistersTheDeclaredAdapters pins the set of harnesses this binary
// has a named adapter for.
//
// The assertion is whole rather than a set of "contains" checks, for the reason
// the source-kind set is: an adapter that stopped registering is a manifest that
// suddenly reports a rule unsupported, and one that started registering is a
// harness harnaas claims to install rules for before anybody wrote down where
// they land. The test lives in the package that links the adapters, so what it
// observes is the binary's own registry rather than one a fixture assembled.
//
// The order is the registry's own, which is sorted rather than the import graph's,
// so this list reads the same however the blank imports happen to be arranged.
func TestBinaryRegistersTheDeclaredAdapters(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []harness.ID{harness.ClaudeCode, harness.DevinCLI}, adapter.Default.Harnesses())
}

// TestEveryRegisteredAdapterAnswersForItsOwnHarness proves registration filed
// each adapter under the harness it actually addresses.
//
// The registry keys itself on what the adapter reports, so the two cannot
// disagree by construction — but an adapter registered as a nil interface value
// reads identically in the listing above and only fails once the install flow
// asks it anything.
func TestEveryRegisteredAdapterAnswersForItsOwnHarness(t *testing.T) {
	t.Parallel()

	adapters := adapter.Default.Adapters()
	require.NotEmpty(t, adapters)

	for _, a := range adapters {
		require.NotNil(t, a)

		found, err := adapter.Default.Lookup(a.Harness())
		require.NoError(t, err)
		assert.Equal(t, a.Harness(), found.Harness())
	}
}
