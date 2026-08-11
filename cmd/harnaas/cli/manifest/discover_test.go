package manifest_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/paths"
)

// scratchRepo returns a temporary directory marked as a repository root, with
// every symlink resolved so a path this test creates is spelled the way the
// package will report it.
func scratchRepo(t *testing.T) string {
	t.Helper()

	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(root, paths.RepositoryMarker), 0o750))
	return root
}

// writeFile creates a file and every directory above it.
func writeFile(t *testing.T, path, contents string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
}

// projectContext carries root as the project root, as the entrypoint does once
// it has discovered one.
func projectContext(t *testing.T, root string) context.Context {
	t.Helper()

	return paths.WithProjectRoot(t.Context(), root)
}

func TestLoadReadsTheManifestAtTheProjectRoot(t *testing.T) {
	t.Parallel()

	root := scratchRepo(t)
	path := filepath.Join(root, manifest.FileName)
	writeFile(t, path, minimal)

	document, err := manifest.Load(projectContext(t, root))
	require.NoError(t, err)

	assert.Equal(t, path, document.Path, "a loaded document names the file it came from")
	assert.Len(t, document.Assets, 2)
}

// TestLoadFindsTheRootManifestFromANestedDirectory is the whole point of
// resolving the manifest from the context-carried project root: a command run
// three directories down reads the same declarations as one run at the root.
func TestLoadFindsTheRootManifestFromANestedDirectory(t *testing.T) {
	root := scratchRepo(t)
	writeFile(t, filepath.Join(root, manifest.FileName), minimal)

	nested := filepath.Join(root, "packages", "api", "internal")
	require.NoError(t, os.MkdirAll(nested, 0o750))
	t.Chdir(nested)

	document, err := manifest.Load(paths.WithDiscoveredRoot(t.Context()))
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(root, manifest.FileName), document.Path)
}

func TestPathIsTheManifestAtTheProjectRoot(t *testing.T) {
	t.Parallel()

	root := scratchRepo(t)

	path, err := manifest.Path(projectContext(t, root))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, manifest.FileName), path)
}

func TestMissingManifestNamesInit(t *testing.T) {
	t.Parallel()

	root := scratchRepo(t)

	document, err := manifest.Load(projectContext(t, root))

	require.Nil(t, document)
	var notFound *manifest.NotFoundError
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, root, notFound.Root)
	assert.Contains(t, err.Error(), manifest.FileName)
	assert.Contains(t, err.Error(), "harnaas init")
}

func TestManifestInASubdirectoryIsAnError(t *testing.T) {
	t.Parallel()

	root := scratchRepo(t)
	writeFile(t, filepath.Join(root, manifest.FileName), minimal)
	stray := filepath.Join(root, "packages", "api", manifest.FileName)
	writeFile(t, stray, minimal)

	document, err := manifest.Load(projectContext(t, root))

	require.Nil(t, document)
	var subdirectory *manifest.SubdirectoryError
	require.ErrorAs(t, err, &subdirectory)
	assert.Equal(t, stray, subdirectory.Path)
	assert.Contains(t, err.Error(), "packages/api/"+manifest.FileName,
		"the message names the file that will never be read")
	assert.Contains(t, err.Error(), root, "and the manifest harnaas does read")
}

// TestManifestInASubdirectoryIsReportedWithNoRootManifest keeps the more useful
// of two true statements. A project whose only manifest sits in a subdirectory
// is not a project without a manifest, and answering "run harnaas init" would
// tell its author to create a file they believe they already wrote.
func TestManifestInASubdirectoryIsReportedWithNoRootManifest(t *testing.T) {
	t.Parallel()

	root := scratchRepo(t)
	writeFile(t, filepath.Join(root, "packages", "api", manifest.FileName), minimal)

	_, err := manifest.Load(projectContext(t, root))

	var subdirectory *manifest.SubdirectoryError
	require.ErrorAs(t, err, &subdirectory)
}

// TestDependencyDirectoriesAreNotSearched keeps a vendored library's own
// manifest from failing every command in the project that vendors it: that file
// declares the dependency's assets, and its author is not the person harnaas
// would be talking to.
func TestDependencyDirectoriesAreNotSearched(t *testing.T) {
	t.Parallel()

	root := scratchRepo(t)
	writeFile(t, filepath.Join(root, manifest.FileName), minimal)

	for _, dir := range []string{"node_modules/some-package", "vendor/example.com/lib", ".git/hooks", ".harnaas"} {
		writeFile(t, filepath.Join(root, filepath.FromSlash(dir), manifest.FileName), minimal)
	}

	document, err := manifest.Load(projectContext(t, root))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, manifest.FileName), document.Path)
}

func TestLoadSurfacesADecodeFailureFromTheFile(t *testing.T) {
	t.Parallel()

	root := scratchRepo(t)
	path := filepath.Join(root, manifest.FileName)
	writeFile(t, path, `{"version": 1, "assests": []}`)

	_, err := manifest.Load(projectContext(t, root))

	var decodeErr *manifest.DecodeError
	require.ErrorAs(t, err, &decodeErr)
	assert.Equal(t, path, decodeErr.Path)
	assert.Contains(t, err.Error(), path, "a diagnostic about a file names the file")
}

// TestLoadReportsAnUnestablishedProjectRoot proves Load asks for the root
// rather than falling back to the working directory, which is what keeps the
// working-directory ban meaningful.
func TestLoadReportsAnUnestablishedProjectRoot(t *testing.T) {
	t.Parallel()

	_, err := manifest.Load(t.Context())
	require.ErrorIs(t, err, paths.ErrRootNotEstablished)

	_, err = manifest.Path(t.Context())
	require.ErrorIs(t, err, paths.ErrRootNotEstablished)
}

// TestPackageNeverWrites asserts by construction what the manifest's read-only
// rule states: apart from `init` creating the file once, harnaas never writes,
// reformats or normalizes `harnaas.json`.
//
// The rule is what makes the manifest's diffs trustworthy — a reviewer can tell
// an intentional pin bump from a normalization pass — and it is the reason
// every remedy here is phrased as an edit. A write added to this package would
// break that quietly, in a change whose diff would look like a convenience.
func TestPackageNeverWrites(t *testing.T) {
	t.Parallel()

	banned := map[string]bool{
		"WriteFile": true, "Create": true, "CreateTemp": true, "OpenFile": true,
		"Remove": true, "RemoveAll": true, "Rename": true, "Truncate": true,
		"Mkdir": true, "MkdirAll": true, "Chmod": true, "WriteString": true,
	}

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, parseErr := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		require.NoError(t, parseErr)

		for _, imported := range file.Imports {
			assert.NotContains(t, imported.Path.Value, "jsonutil",
				"%s imports the atomic writer; nothing here may write the manifest", name)
		}

		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			assert.False(t, banned[selector.Sel.Name],
				"%s calls %s; the manifest is read-only outside `harnaas init`", name, selector.Sel.Name)
			return true
		})
	}
}
