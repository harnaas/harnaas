package local

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The escape classification is tested here rather than through a resolution
// because the resolution needs a symbolic link, and creating one is a privilege
// an unprivileged Windows user does not have — which would leave the rule
// covered on some machines and unasserted on the ones most likely to break it.
//
// A `..` component produces the same refusal from the same code in the standard
// library, and needs no privilege at all. Reaching for it here is deliberate: it
// is unreachable through a manifest, because the grammar removes `..` textually
// long before a read, so it is only ever a way to obtain the error the handle
// raises.

// anchoredRoot opens a handle on a scratch directory with a sibling outside it.
func anchoredRoot(tb testing.TB) *os.Root {
	tb.Helper()

	base := tb.TempDir()
	inside := filepath.Join(base, "inside")
	require.NoError(tb, os.Mkdir(inside, 0o755))
	require.NoError(tb, os.WriteFile(filepath.Join(base, "outside.md"), []byte("elsewhere"), 0o644))

	anchor, err := os.OpenRoot(inside)
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = anchor.Close() })

	return anchor
}

// TestARefusalForLeavingTheAnchorIsRecognized pins the one classification that
// cannot be made with errors.Is, because the standard library keeps the sentinel
// behind it unexported.
func TestARefusalForLeavingTheAnchorIsRecognized(t *testing.T) {
	t.Parallel()

	_, err := anchoredRoot(t).Stat("../outside.md")

	require.Error(t, err)
	require.NotErrorIs(t, err, fs.ErrNotExist,
		"a path that leaves the anchor is refused rather than reported absent, which is why the two need telling apart")
	assert.True(t, escapesAnchor(err), "the refusal harnaas reports as a containment violation: %v", err)
}

// TestAnAbsentPathIsNotMistakenForAnEscape holds the commonest failure apart
// from the rarest: reporting a file nobody created as a containment violation
// would accuse an author of an escape they never wrote.
func TestAnAbsentPathIsNotMistakenForAnEscape(t *testing.T) {
	t.Parallel()

	_, err := anchoredRoot(t).Stat("nothing-here.md")

	require.ErrorIs(t, err, fs.ErrNotExist)
	assert.False(t, escapesAnchor(err))
}

// TestEveryRefusalIsClassifiedAsTheEditItNeeds drives the classifier over all
// three readings at once, because what separates them is which edit the author
// has to make.
func TestEveryRefusalIsClassifiedAsTheEditItNeeds(t *testing.T) {
	t.Parallel()

	anchor := anchoredRoot(t)
	_, escape := anchor.Stat("../outside.md")
	_, absent := anchor.Stat("nothing-here.md")

	read := &reader{assetID: "review", base: ".harnaas/skills/review"}

	var missing *MissingSourceError
	require.ErrorAs(t, read.locationFailure(read.base, absent), &missing)
	assert.Equal(t, ".harnaas/skills/review", missing.Path)

	var containment *ContainmentError
	require.ErrorAs(t, read.locationFailure(read.base, escape), &containment)
	assert.Equal(t, ".harnaas/skills/review", containment.Path)

	var unreadable *ReadError
	require.ErrorAs(t, read.locationFailure(read.base, errors.New("permission denied")), &unreadable)
	assert.Equal(t, ".harnaas/skills/review", unreadable.Path)
}

// TestAnUnrecognizedWordingFallsBackToAReadFailure states the cost of matching
// a message: a standard library that rewords its refusal makes harnaas less
// specific, and never wrong. A containment violation reported as a read that
// failed still refuses the read.
func TestAnUnrecognizedWordingFallsBackToAReadFailure(t *testing.T) {
	t.Parallel()

	reworded := &fs.PathError{Op: "statat", Path: "x", Err: errors.New("path leaves the root")}

	assert.False(t, escapesAnchor(reworded))

	read := &reader{assetID: "review", base: ".harnaas/skills/review"}

	var unreadable *ReadError
	require.ErrorAs(t, read.locationFailure(read.base, reworded), &unreadable)
}

// TestPathsWithinTheSourceAreNamedRelativeToTheProjectRoot holds the spelling
// every diagnostic uses: the manifest wrote `.harnaas/skills/review`, and a
// message naming `references/checks.md` alone names a file in no particular
// place.
func TestPathsWithinTheSourceAreNamedRelativeToTheProjectRoot(t *testing.T) {
	t.Parallel()

	read := &reader{base: ".harnaas/skills/review"}

	assert.Equal(t, ".harnaas/skills/review", read.projectPath(""))
	assert.Equal(t, ".harnaas/skills/review/references/checks.md", read.projectPath("references/checks.md"))
}
