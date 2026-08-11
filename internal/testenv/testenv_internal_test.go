package testenv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunMainRedirectsTheSuiteAndRemovesItsDirectory exercises the half of Main
// a test package cannot observe about itself: by the time TestMain's own
// deferred cleanup runs, the suite is over and there is nobody left to assert
// that the directory went away.
//
// Main redirects the process rather than one test, so the variables it sets are
// snapshotted with t.Setenv first — that is what puts them back for the rest of
// this suite when the test ends.
func TestRunMainRedirectsTheSuiteAndRemovesItsDirectory(t *testing.T) {
	for _, v := range append(toolchainVars(), userDirVars("scratch")...) {
		t.Setenv(v.key, os.Getenv(v.key))
	}

	var home string
	code := runMain(func() int {
		resolved, err := os.UserHomeDir()
		require.NoError(t, err)
		home = resolved
		return 7
	})

	assert.Equal(t, 7, code, "the suite's own exit code is what TestMain hands to os.Exit")
	require.NotEmpty(t, home)

	root := filepath.Dir(home)
	assert.Equal(t, "home", filepath.Base(home), "the suite ran against a redirected home directory")

	_, err := os.Stat(root)
	assert.ErrorIs(t, err, os.ErrNotExist,
		"the redirect directory %s outlived the suite that owned it", root)
}

func TestWithinDistinguishesASiblingFromAChild(t *testing.T) {
	t.Parallel()

	root := filepath.Join("a", "root")

	assert.True(t, within(root, root))
	assert.True(t, within(root, filepath.Join(root, "cache")))
	assert.False(t, within(root, filepath.Join("a", "root-next-door")))
	assert.False(t, within(root, filepath.Join("a", "elsewhere")))
	assert.False(t, within(root, filepath.Dir(root)))
}
