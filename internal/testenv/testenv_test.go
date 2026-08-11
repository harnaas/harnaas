package testenv_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harnaas/harnaas/internal/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// userDirs are the three questions the standard library answers about where a
// user's own files live. Redirecting one and leaving another behind is the
// failure this package exists to prevent, so every test asks all three.
var userDirs = map[string]func() (string, error){
	"home":   os.UserHomeDir,
	"cache":  os.UserCacheDir,
	"config": os.UserConfigDir,
}

func TestRedirectMovesEveryPerUserDirectoryUnderTheTestsOwn(t *testing.T) {
	root := testenv.Redirect(t)

	for name, resolve := range userDirs {
		path, err := resolve()
		require.NoError(t, err, "the %s directory", name)
		assert.True(t, within(root, path),
			"the %s directory resolves to %s, outside the test's own %s", name, path, root)

		info, statErr := os.Stat(path)
		require.NoError(t, statErr, "the %s directory exists, so a test writing to it need not create it", name)
		assert.True(t, info.IsDir())
	}
}

func TestRedirectIsUndoneWhenTheTestEnds(t *testing.T) {
	before, err := os.UserHomeDir()
	require.NoError(t, err)

	var during string
	t.Run("redirected", func(t *testing.T) {
		testenv.Redirect(t)

		home, homeErr := os.UserHomeDir()
		require.NoError(t, homeErr)
		during = home
	})
	require.NotEqual(t, before, during, "the subtest was redirected")

	after, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, before, after,
		"a redirect installed by one test must not follow the suite into the next one")
}

// TestTheGoToolchainKeepsItsOwnDirectories is what makes a test that shells out
// to `go build` usable: the module cache and the build cache are derived from
// the same home and cache directories, and moving them with the rest would make
// the first build in a redirected test re-download the module graph into a
// directory the suite deletes on the way out.
func TestTheGoToolchainKeepsItsOwnDirectories(t *testing.T) {
	root := testenv.Redirect(t)

	for _, key := range []string{"GOPATH", "GOMODCACHE", "GOCACHE", "GOENV"} {
		value := os.Getenv(key)
		require.NotEmpty(t, value, "%s is pinned rather than left to be derived from the redirected home", key)
		assert.False(t, within(root, value),
			"%s resolves to %s, inside the redirected %s", key, value, root)
	}
}

// within reports whether path is root or sits beneath it. A path Rel cannot
// relate to root — a different Windows volume — is outside it.
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
