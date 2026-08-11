package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The two rules forbidigo encodes, asserted again over the module's own syntax.
//
// A lint rule is silenceable: `//nolint:forbidigo` with a plausible-looking
// reason passes review more easily than it should, and both of these rules fail
// quietly rather than loudly — a command that read the process working
// directory would work perfectly until somebody ran it from a subdirectory, and
// one that used a Print* helper would put its result on the wrong stream and
// only break the pipeline of whoever was parsing it. So the rules are checked
// twice: golangci-lint catches them while the code is being written, and this
// catches a suppression that got past review.
//
// These tests live in the cli package because that is where the rules are
// documented, but they walk the whole module: internal/ and the entrypoint are
// bound by them too.

// forbiddenPrintNames are the method names that write to a stream the caller
// did not choose. The rule is stated for cobra's helpers — they write to
// OutOrStderr, so a command's result would land on stderr — but it is asserted
// against every receiver, because fmt's and log's package-level printers write
// to a process global for the same reason and are just as invisible to a test
// capturing the command's own writers.
var forbiddenPrintNames = map[string]bool{
	"Print":   true,
	"Printf":  true,
	"Println": true,
}

// workingDirectoryReader is the one sanctioned call to os.Getwd, expressed as a
// module-relative path. paths.WithDiscoveredRoot is the abstraction forbidigo's
// message names, so the suppression belongs inside it and nowhere else.
var workingDirectoryReader = filepath.Join("cmd", "harnaas", "cli", "paths", "paths.go")

func TestOnlyTheProjectRootResolverReadsTheWorkingDirectory(t *testing.T) {
	t.Parallel()

	var found []string
	forEachSourceFile(t, func(rel string, file *ast.File) {
		osName, ok := importedAs(file, "os")
		if !ok {
			return
		}
		if callsSelector(file, osName, "Getwd") {
			found = append(found, rel)
		}
	})

	assert.Equal(t, []string{workingDirectoryReader}, found,
		"os.Getwd() breaks when a command runs from a subdirectory; resolve the project root once and read it from the context with paths.ProjectRoot(ctx)")
}

func TestNoSourceWritesThroughAPrintHelper(t *testing.T) {
	t.Parallel()

	var found []string
	forEachSourceFile(t, func(rel string, file *ast.File) {
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || !forbiddenPrintNames[sel.Sel.Name] {
				return true
			}
			receiver, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			found = append(found, rel+": "+receiver.Name+"."+sel.Sel.Name)
			return true
		})
	})

	assert.Empty(t, found,
		"a Print* helper writes to a stream the caller did not choose; write to the command's own writers with fmt.Fprint*(cmd.OutOrStdout(), …) or advisef")
}

// forEachSourceFile parses every non-test Go file in the module and hands it to
// visit with its module-relative path.
//
// Test files are excluded on purpose: a test may legitimately name anything it
// is asserting about, including the very calls these rules forbid.
func forEachSourceFile(t *testing.T, visit func(rel string, file *ast.File)) {
	t.Helper()

	forEachGoFile(t, func(name string) bool { return !strings.HasSuffix(name, "_test.go") }, visit)
}

// forEachGoFile parses every Go file in the module whose base name include
// accepts, and hands each to visit with its module-relative path.
func forEachGoFile(t *testing.T, include func(name string) bool, visit func(rel string, file *ast.File)) {
	t.Helper()

	root := moduleRoot(t)
	fset := token.NewFileSet()

	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipDir(entry.Name()) && path != root {
				return fs.SkipDir
			}
			return nil
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || !include(name) {
			return nil
		}

		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return parseErr
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		visit(rel, file)
		return nil
	}))
}

// skipDir reports whether a directory holds nothing this rule applies to:
// dot-directories, test fixtures, and the dependency trees a module may vendor.
// Their contents are somebody else's code, and a rule harnaas states about
// itself is not one it gets to state about them.
func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor" || name == "node_modules"
}

// moduleRoot walks up from the test's own directory to the directory holding
// go.mod, so the walk covers the module however deep the test sits.
func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := filepath.Abs(".")
	require.NoError(t, err)

	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "no go.mod above %s", dir)
		dir = parent
	}
}

// importedAs returns the name a file refers to an import path by, honouring an
// alias and the last path segment otherwise. A file that does not import the
// path cannot call into it, whatever an identifier happens to be spelled.
func importedAs(file *ast.File, importPath string) (string, bool) {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != importPath {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name, spec.Name.Name != "_"
		}
		return path[strings.LastIndex(path, "/")+1:], true
	}
	return "", false
}

// callsSelector reports whether file contains a reference to receiver.name.
func callsSelector(file *ast.File, receiver, name string) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != name {
			return true
		}
		if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == receiver {
			found = true
		}
		return true
	})
	return found
}
