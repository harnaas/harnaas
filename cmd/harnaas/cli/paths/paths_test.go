package paths_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/paths"
	"github.com/stretchr/testify/require"
)

// scratchDir returns a temporary directory with every symlink resolved.
// t.TempDir sits under /var on macOS, which is a symlink to /private/var, so a
// root discovered through the working directory comes back spelled differently
// from the path the test created. Resolving here keeps the comparisons honest
// without teaching the package to rewrite the paths it is given.
func scratchDir(t *testing.T) string {
	t.Helper()

	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

// scratchRepo returns a scratch directory marked as a repository root.
func scratchRepo(t *testing.T) string {
	t.Helper()

	root := scratchDir(t)
	require.NoError(t, os.Mkdir(filepath.Join(root, paths.RepositoryMarker), 0o750))
	return root
}

func TestResolveFindsTheRootFromANestedDirectory(t *testing.T) {
	t.Parallel()

	root := scratchRepo(t)
	nested := filepath.Join(root, "src", "api", "handlers")
	require.NoError(t, os.MkdirAll(nested, 0o750))

	fromNested, err := paths.Resolve(nested)
	require.NoError(t, err)

	fromRoot, err := paths.Resolve(root)
	require.NoError(t, err)

	require.Equal(t, root, fromNested)
	require.Equal(t, fromNested, fromRoot, "the depth a command is run from must not change the answer")
}

func TestResolveAcceptsAMarkerFileAsWellAsADirectory(t *testing.T) {
	t.Parallel()

	// Git writes .git as a file holding a gitdir: pointer in a worktree and in
	// a submodule. Both are ordinary places to run harnaas from.
	root := scratchDir(t)
	marker := filepath.Join(root, paths.RepositoryMarker)
	require.NoError(t, os.WriteFile(marker, []byte("gitdir: /elsewhere\n"), 0o600))

	resolved, err := paths.Resolve(root)
	require.NoError(t, err)
	require.Equal(t, root, resolved)
}

func TestResolvePrefersTheNearestEnclosingRepository(t *testing.T) {
	t.Parallel()

	outer := scratchRepo(t)
	inner := filepath.Join(outer, "vendor", "submodule")
	require.NoError(t, os.MkdirAll(filepath.Join(inner, paths.RepositoryMarker), 0o750))

	resolved, err := paths.Resolve(filepath.Join(inner, "skills"))
	require.NoError(t, err)
	require.Equal(t, inner, resolved, "a command run inside a submodule acts on the submodule, as git does")
}

func TestResolveOutsideARepositoryNamesTheDirectoryAndTheFix(t *testing.T) {
	t.Parallel()

	// A bare temporary directory: nothing above it up to the filesystem root
	// carries a repository marker.
	dir := scratchDir(t)

	resolved, err := paths.Resolve(dir)
	require.Empty(t, resolved)

	var noRoot *paths.NoProjectRootError
	require.ErrorAs(t, err, &noRoot)
	require.Equal(t, dir, noRoot.Dir)
	require.Contains(t, err.Error(), dir, "the diagnostic states which directory was searched")
	require.Contains(t, err.Error(), "git init", "the diagnostic states the fix")
}

func TestProjectRootReturnsTheCarriedRoot(t *testing.T) {
	t.Parallel()

	root := scratchRepo(t)

	got, err := paths.ProjectRoot(paths.WithProjectRoot(t.Context(), root))
	require.NoError(t, err)
	require.Equal(t, root, got)
}

func TestProjectRootReportsAContextThatNeverEstablishedARoot(t *testing.T) {
	t.Parallel()

	got, err := paths.ProjectRoot(t.Context())
	require.Empty(t, got)
	require.ErrorIs(t, err, paths.ErrRootNotEstablished)
}

func TestProjectRootIsUnaffectedByAnUnrelatedContextValue(t *testing.T) {
	t.Parallel()

	// The context key is an unexported struct type, so no other package can
	// name it, let alone overwrite the root with a value of its own.
	type otherKey struct{}

	ctx := context.WithValue(t.Context(), otherKey{}, "someone else's value")
	got, err := paths.ProjectRoot(ctx)
	require.Empty(t, got)
	require.ErrorIs(t, err, paths.ErrRootNotEstablished)
}

func TestWithDiscoveredRootResolvesFromTheWorkingDirectory(t *testing.T) {
	// t.Chdir forbids a parallel test, which is the whole reason the resolution
	// itself is a pure function taking a directory: only this one test has to
	// move the process.
	root := scratchRepo(t)
	nested := filepath.Join(root, "docs", "adr")
	require.NoError(t, os.MkdirAll(nested, 0o750))
	t.Chdir(nested)

	got, err := paths.ProjectRoot(paths.WithDiscoveredRoot(t.Context()))
	require.NoError(t, err)
	require.Equal(t, root, got)
}

func TestWithDiscoveredRootCarriesTheFailureRatherThanReportingIt(t *testing.T) {
	dir := scratchDir(t)
	t.Chdir(dir)

	// Discovery outside a repository must still hand back a usable context, so
	// that --version and --help keep working where there is no project.
	ctx := paths.WithDiscoveredRoot(t.Context())
	require.NotNil(t, ctx)

	got, err := paths.ProjectRoot(ctx)
	require.Empty(t, got)

	var noRoot *paths.NoProjectRootError
	require.ErrorAs(t, err, &noRoot)
	require.Equal(t, dir, noRoot.Dir)
}

func TestNoProjectRootErrorStatesTheProblemBeforeTheFix(t *testing.T) {
	t.Parallel()

	err := &paths.NoProjectRootError{Dir: filepath.Join("scratch", "project")}
	problem, fix, found := strings.Cut(err.Error(), "\n\n")

	require.True(t, found, "a diagnostic is a problem and a fix, separated by a blank line")
	require.Contains(t, problem, "no project root found")
	require.Contains(t, fix, "Run harnaas from inside your project's repository")
}
