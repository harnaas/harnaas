package harness_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
)

func TestDefaultIsItselfRecognized(t *testing.T) {
	t.Parallel()

	// init falls back to Default when it detects nothing, and manifest
	// validation rejects a harness the roster does not recognize. A Default
	// outside the roster would make init scaffold a manifest that harnaas
	// itself refuses to load.
	assert.True(t, harness.Recognized(harness.Default),
		"the default harness %q is not in the roster", harness.Default)

	found, err := harness.Lookup(harness.Default)
	require.NoError(t, err)
	assert.Equal(t, harness.Default, found.ID)
}

func TestRosterIsOrderedByID(t *testing.T) {
	t.Parallel()

	ids := harness.IDs()
	require.NotEmpty(t, ids)
	assert.True(t, slices.IsSorted(ids),
		"the roster must be declared in id order so every list harnaas prints reads the same: %v", ids)
}

func TestIDOrderIsStableAcrossCalls(t *testing.T) {
	t.Parallel()

	// The roster is a slice rather than a map precisely so this holds. A map
	// would give a different order on every run, and the order is what an
	// "unknown harness" message and a scaffolded manifest are both built from.
	first := harness.IDs()
	for range 8 {
		assert.Equal(t, first, harness.IDs())
	}
}

func TestEveryEntryIsUsable(t *testing.T) {
	t.Parallel()

	seen := map[harness.ID]bool{}
	for _, h := range harness.All() {
		assert.NotEmpty(t, h.ID, "a roster entry has no id")
		assert.NotEmpty(t, h.DisplayName, "%s has no display name", h.ID)
		assert.NotEmpty(t, h.ProjectEvidence,
			"%s declares no project evidence, so init could never detect it", h.ID)

		assert.False(t, seen[h.ID], "%s is declared twice", h.ID)
		seen[h.ID] = true

		for _, evidence := range h.ProjectEvidence {
			assert.False(t, filepath.IsAbs(evidence),
				"%s: evidence %q must be relative to the project root", h.ID, evidence)
			assert.NotContains(t, evidence, "..",
				"%s: evidence %q reaches outside the project", h.ID, evidence)
		}
	}
}

func TestUnknownIDErrorNamesTheInputAndListsTheRecognizedOnes(t *testing.T) {
	t.Parallel()

	_, err := harness.Lookup("claud-code")

	var unknown *harness.UnknownError
	require.ErrorAs(t, err, &unknown)
	assert.Equal(t, harness.ID("claud-code"), unknown.ID)
	assert.Equal(t, harness.IDs(), unknown.Recognized)

	message := err.Error()
	assert.Contains(t, message, `"claud-code"`, "the message must quote the id as written")
	for _, id := range harness.IDs() {
		assert.Contains(t, message, string(id))
	}

	// Problem, blank line, then the fix — the shape every harnaas diagnostic has.
	problem, fix, split := strings.Cut(message, "\n\n")
	require.True(t, split, "the message must separate the problem from the fix with a blank line")
	assert.NotContains(t, problem, "\n")
	assert.NotEmpty(t, fix)
}

func TestUnknownIDErrorMessageIsIdenticalAcrossRuns(t *testing.T) {
	t.Parallel()

	_, first := harness.Lookup("nope")
	for range 8 {
		_, again := harness.Lookup("nope")
		assert.Equal(t, first.Error(), again.Error())
	}
}

func TestRecognizedAnswersFalseForAnUnknownID(t *testing.T) {
	t.Parallel()

	assert.True(t, harness.Recognized(harness.ClaudeCode))
	assert.False(t, harness.Recognized("cursor"))
	assert.False(t, harness.Recognized(""))
	assert.False(t, harness.Recognized("Claude Code"),
		"a display name is never accepted where an id is expected")
}

func TestHasPerUserLocationAnswersForARecognizedHarness(t *testing.T) {
	t.Parallel()

	perUser, err := harness.HasPerUserLocation(harness.ClaudeCode)
	require.NoError(t, err)
	assert.True(t, perUser, "Claude Code reads assets from an unambiguous per-user location")
}

func TestHasPerUserLocationRejectsAnUnknownIDRatherThanAnsweringFalse(t *testing.T) {
	t.Parallel()

	// Scope validation calls this. Folding "harnaas does not know this harness"
	// into "this harness has no per-user location" would report a
	// scope-not-supported error for what is really a misspelling, and send the
	// author to the wrong edit.
	perUser, err := harness.HasPerUserLocation("cursor")

	var unknown *harness.UnknownError
	require.ErrorAs(t, err, &unknown)
	assert.False(t, perUser)
}

func TestCallersCannotMutateTheRoster(t *testing.T) {
	t.Parallel()

	const clobbered = "clobbered"

	all := harness.All()
	require.NotEmpty(t, all)
	all[0].DisplayName = clobbered
	all[0].ProjectEvidence[0] = clobbered

	fresh := harness.All()
	assert.NotEqual(t, clobbered, fresh[0].DisplayName)
	assert.NotEqual(t, clobbered, fresh[0].ProjectEvidence[0])

	ids := harness.IDs()
	ids[0] = clobbered
	assert.NotEqual(t, harness.ID(clobbered), harness.IDs()[0])
}

// TestRosterStaysDataOnly asserts the package imports nothing that reaches the
// filesystem, the network or the environment.
//
// The roster ships one change ahead of the adapters that give its ids meaning,
// and the only thing keeping the two from drifting into conflict is that the
// roster holds no behaviour to disagree with. Evidence is a list of paths so
// that `init` does the stat calls; the moment this package does them itself,
// detection has two homes.
func TestRosterStaysDataOnly(t *testing.T) {
	t.Parallel()

	banned := []string{"os", "os/exec", "net", "net/http", "io/ioutil", "path/filepath"}

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.SkipObjectResolution)
		require.NoError(t, err)

		for _, imported := range file.Imports {
			path := strings.Trim(imported.Path.Value, `"`)
			assert.NotContains(t, banned, path,
				"%s imports %q; the roster is data only, so the caller does the I/O", name, path)
		}
	}
}

// TestRosterDeclaresNoInit keeps the roster a literal a reader can take in at a
// glance, rather than a registry assembled from wherever an init happens to run.
func TestRosterDeclaresNoInit(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.SkipObjectResolution)
		require.NoError(t, err)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue
			}
			assert.NotEqual(t, "init", fn.Name.Name,
				"%s declares a package init; declare roster entries in the literal instead", name)
		}
	}
}
