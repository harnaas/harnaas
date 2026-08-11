package adapter_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// modulePath is this module's import path, which is how a module-internal import
// is told from a standard-library one: the standard library has no dot in its
// first path element and nothing else in the tree spells this prefix.
const modulePath = "github.com/harnaas/harnaas"

// allowedModuleImports is the whole of what an adapter package may import from
// inside harnaas.
//
// It is an explicit list rather than a rule about package names because the
// point is that widening it is an edit somebody makes on purpose. An adapter is
// a mapping from an asset to a path: it needs the contract it implements and the
// two vocabularies that contract is written in, and an adapter reaching past
// those is either doing I/O it should be handing to its caller or growing a
// dependency on the flow that calls it — which is the import cycle this package
// exists on its own to avoid.
var allowedModuleImports = []string{
	modulePath + "/cmd/harnaas/cli/adapter",
	modulePath + "/cmd/harnaas/cli/harness",
	modulePath + "/cmd/harnaas/cli/manifest",
}

// TestAdapterPackagesStayWithinTheImportBoundary parses every adapter package's
// non-test sources and fails on an import outside the allowlist.
//
// A test rather than a linter rule because the boundary is a property of this
// directory rather than of the module: the flat cli package imports the adapters
// and the adapters must not import it back, and the day one does the failure is
// not a compile error — Go permits it right up until the cycle closes, and by
// then the fix is a redesign rather than an import removed.
//
// Third-party imports are outside the allowlist too. An adapter that needs a
// YAML writer or an HTTP client is an adapter that has stopped answering
// questions and started doing work, so the test failing is the conversation
// happening at the right time.
func TestAdapterPackagesStayWithinTheImportBoundary(t *testing.T) {
	t.Parallel()

	packages := adapterPackages(t)
	require.NotEmpty(t, packages, "there is at least one adapter package to check")

	for _, pkg := range packages {
		t.Run(pkg, func(t *testing.T) {
			t.Parallel()

			for name, file := range parseAdapterPackage(t, pkg) {
				for _, imported := range file.Imports {
					path := strings.Trim(imported.Path.Value, `"`)
					if !strings.Contains(strings.SplitN(path, "/", 2)[0], ".") {
						continue // Standard library.
					}

					assert.Contains(t, allowedModuleImports, path,
						"the adapter package %q imports %q in %s, which is outside the boundary; "+
							"widen allowedModuleImports deliberately if that is the intent",
						pkg, path, name)
				}
			}
		})
	}
}

// TestEveryAdapterPackageRegistersItself is the other half of the same test, and
// it is here rather than beside the registry because it is the same failure seen
// from the other side.
//
// Self-registration's own failure mode is an adapter that compiles, passes its
// own tests and never reaches [adapter.Default] — which reads exactly like a
// harness harnaas simply has no adapter for. Asserting it from the syntax tree
// catches the package that was written and never wired, where a test over the
// registry alone would only ever see the adapters that did register.
func TestEveryAdapterPackageRegistersItself(t *testing.T) {
	t.Parallel()

	for _, pkg := range adapterPackages(t) {
		t.Run(pkg, func(t *testing.T) {
			t.Parallel()

			registers := false
			for _, file := range parseAdapterPackage(t, pkg) {
				ast.Inspect(file, func(node ast.Node) bool {
					call, isCall := node.(*ast.CallExpr)
					if !isCall {
						return true
					}
					selector, isSelector := call.Fun.(*ast.SelectorExpr)
					if !isSelector {
						return true
					}
					pkgName, isIdent := selector.X.(*ast.Ident)
					if isIdent && pkgName.Name == "adapter" && selector.Sel.Name == "Register" {
						registers = true
					}
					return true
				})
			}

			assert.True(t, registers,
				"the adapter package %q never calls adapter.Register, so a binary linking it still has no adapter for its harness",
				pkg)
		})
	}
}

// adapterPackages lists the directories beneath this one, each of which is one
// adapter. The set is discovered rather than declared so a package added without
// a line in a list is still checked — the failure this pair of tests exists for
// is precisely an adapter nobody remembered to wire.
func adapterPackages(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	var packages []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") && entry.Name() != "testdata" {
			packages = append(packages, entry.Name())
		}
	}
	return packages
}

// parseAdapterPackage returns the package's non-test source files by name. Test
// files are excluded because the boundary is about what a shipped binary links:
// a test importing a helper says nothing about what the adapter reaches for at
// run time.
func parseAdapterPackage(t *testing.T, pkg string) map[string]*ast.File {
	t.Helper()

	entries, err := os.ReadDir(pkg)
	require.NoError(t, err)

	fset := token.NewFileSet()
	files := make(map[string]*ast.File)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, filepath.Join(pkg, name), nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		files[name] = file
	}

	require.NotEmpty(t, files, "the adapter package %q has no source files", pkg)
	return files
}
