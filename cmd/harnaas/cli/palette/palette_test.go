package palette_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/palette"
)

// declaredColours reads the package's own source rather than a hand-maintained
// list, so a colour added tomorrow is covered by these tests without anyone
// remembering to add it here. Reading the source is also the only way to assert
// something about what is *absent*.
func declaredColours(t *testing.T) map[string]string {
	t.Helper()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	colours := map[string]string{}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.SkipObjectResolution)
		require.NoError(t, err)

		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, ident := range value.Names {
					if ident.IsExported() {
						colours[ident.Name] = ""
					}
				}
			}
		}
	}

	require.NotEmpty(t, colours, "the package declares at least one colour")
	return colours
}

// Every colour is a base16 slot, so harnaas's output adopts the user's terminal
// theme instead of overriding it. A hex value or a 256-colour code here would
// look right only on the machine it was chosen on.
func TestEveryColourIsABase16Slot(t *testing.T) {
	t.Parallel()

	slots := map[string]string{
		"Black": palette.Black, "Red": palette.Red, "Green": palette.Green,
		"Yellow": palette.Yellow, "Blue": palette.Blue, "Magenta": palette.Magenta,
		"Cyan": palette.Cyan, "White": palette.White,
		"BrightBlack": palette.BrightBlack, "BrightRed": palette.BrightRed,
		"BrightGreen": palette.BrightGreen, "BrightYellow": palette.BrightYellow,
		"BrightBlue": palette.BrightBlue, "BrightMagenta": palette.BrightMagenta,
		"BrightCyan": palette.BrightCyan, "BrightWhite": palette.BrightWhite,
		"Accent": palette.Accent, "Accent2": palette.Accent2, "Muted": palette.Muted,
		"Success": palette.Success, "Error": palette.Error,
		"Warning": palette.Warning, "Info": palette.Info,
	}

	assert.Len(t, declaredColours(t), len(slots), "every declared colour is checked below")

	for name, value := range slots {
		slot, err := strconv.Atoi(value)
		require.NoErrorf(t, err, "%s is a slot number", name)
		assert.GreaterOrEqualf(t, slot, 0, "%s is within base16", name)
		assert.LessOrEqualf(t, slot, 15, "%s is within base16", name)
	}
}

// The deliberate absence: body text is left unstyled so it renders in the
// terminal's default foreground and inverts with the background. An alias for
// it would be used, and the moment it is used harnaas's output disappears on
// half the terminals it runs in.
func TestNoBodyTextColourIsDeclared(t *testing.T) {
	t.Parallel()

	forbidden := []string{"text", "body", "foreground", "primary", "default"}

	for name := range declaredColours(t) {
		lowered := strings.ToLower(name)
		for _, fragment := range forbidden {
			assert.NotContainsf(t, lowered, fragment,
				"%s reads as a body-text colour; body text stays unstyled so it inverts with the terminal's background", name)
		}
	}
}

// The semantic aliases exist so a style declaration says why a colour is there.
// An alias pointing at the same slot as another would quietly collapse two
// meanings into one on screen.
func TestSemanticAliasesDoNotCollide(t *testing.T) {
	t.Parallel()

	aliases := map[string]string{
		"Accent": palette.Accent, "Accent2": palette.Accent2, "Muted": palette.Muted,
		"Success": palette.Success, "Error": palette.Error,
		"Warning": palette.Warning, "Info": palette.Info,
	}

	seen := map[string]string{}
	for name, value := range aliases {
		previous, clash := seen[value]
		assert.Falsef(t, clash, "%s and %s both resolve to slot %s", name, previous, value)
		seen[value] = name
	}
}
