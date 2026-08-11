package cli

import (
	"go/ast"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The redirect rule, asserted over the module's own syntax.
//
// harnaas keeps everything that is not the manifest under the user's own
// directories, so a test can reach the machine it is running on without ever
// meaning to — and when it does, it passes. A test that appended to the
// developer's real log, or read a harness configuration that only exists on
// that one machine, reports nothing at all: it is green locally and green in CI
// for different reasons.
//
// So the rule is structural rather than remembered. A package whose files ask
// the standard library where the user's directories are must install
// internal/testenv in its tests, and this test is what notices when a new one
// does not.

// userDirFuncs are the standard library's answers to "where does this user keep
// their own files". Every one of them is a read of real machine state.
var userDirFuncs = []string{"UserHomeDir", "UserCacheDir", "UserConfigDir"}

// testenvImportPath is the redirect. Naming Main installs it for a whole
// package; naming Redirect installs it for one test that needs to know where
// the directories went.
const testenvImportPath = "github.com/harnaas/harnaas/internal/testenv"

func TestEveryPackageReachingPerUserDirectoriesRedirectsThemInTests(t *testing.T) {
	t.Parallel()

	reaching := map[string][]string{}
	redirected := map[string]bool{}

	forEachGoFile(t, func(string) bool { return true }, func(rel string, file *ast.File) {
		dir := filepath.Dir(rel)

		if osName, ok := importedAs(file, "os"); ok {
			for _, fn := range userDirFuncs {
				if callsSelector(file, osName, fn) {
					reaching[dir] = append(reaching[dir], rel)
					break
				}
			}
		}

		if !strings.HasSuffix(filepath.Base(rel), "_test.go") {
			return
		}
		if name, ok := importedAs(file, testenvImportPath); ok {
			if callsSelector(file, name, "Main") || callsSelector(file, name, "Redirect") {
				redirected[dir] = true
			}
		}
	})

	require.NotEmpty(t, reaching,
		"no file in the module reaches a per-user directory, so this walk is checking nothing")

	var missing []string
	for dir, files := range reaching {
		if redirected[dir] {
			continue
		}
		slices.Sort(files)
		missing = append(missing, dir+" ("+strings.Join(files, ", ")+")")
	}
	slices.Sort(missing)

	assert.Empty(t, missing,
		"these packages read the real home, cache or config directory in their tests; call testenv.Main(m) from a TestMain, or testenv.Redirect(t) in the test that needs the path")
}
