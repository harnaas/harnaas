package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// TestBinaryRegistersExactlyTheV1SourceKinds pins the set of kinds this binary
// can resolve.
//
// The assertion is whole rather than a pair of "contains" checks, because both
// halves matter and for different reasons: a kind that stopped registering is a
// manifest that suddenly fails to install, and a kind that started registering
// is a source form harnaas accepts before anybody wrote down what it means. The
// test lives in the package that links the kinds, so what it observes is the
// binary's own registry rather than one a fixture assembled.
func TestBinaryRegistersExactlyTheV1SourceKinds(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		[]manifest.SourceKind{manifest.SourceKindGitHub, manifest.SourceKindLocal},
		source.Default.Kinds(),
	)
}

// TestEveryRegisteredKindConstructsForARun proves registration handed over a
// working constructor rather than merely a name.
//
// A registry entry is a constructor, so a name present in Kinds() is not by
// itself evidence that a run can resolve it: a kind registered under a nil
// constructor reads identically above and only fails once a run asks for a
// resolver, which is the moment every kind is built.
func TestEveryRegisteredKindConstructsForARun(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		require.NotNil(t, source.Default.NewResolver(source.RunOptions{}))
	})
}
