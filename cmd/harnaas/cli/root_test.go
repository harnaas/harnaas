package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newLeafCmd builds a subcommand that does nothing, for tests that care about
// how the tree is wired rather than about what a command does.
func newLeafCmd(use string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: "a subcommand used only by the wiring tests",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
}

// TestRootDeclaresNoPersistentFlags guards the rule that gives --json its
// meaning: nothing is inherited, so a flag a command accepts is a flag that
// command honours. A persistent flag added here — however convenient — would be
// accepted by every future verb whether or not it did anything with it.
func TestRootDeclaresNoPersistentFlags(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()
	// Cobra registers --help and --version lazily, and on Flags() rather than
	// PersistentFlags(); force that to happen so the assertion is made against
	// a fully initialized command rather than an empty one.
	root.InitDefaultHelpFlag()
	root.InitDefaultVersionFlag()

	assert.False(t, root.PersistentFlags().HasFlags(),
		"the root must declare no persistent flags; register a flag locally on each command that honours it:\n%s",
		root.PersistentFlags().FlagUsages())

	child := newLeafCmd("leaf")
	addCommand(root, child, groupSetup)
	assert.False(t, child.InheritedFlags().HasFlags(),
		"a subcommand must inherit nothing from the root:\n%s", child.InheritedFlags().FlagUsages())
}

// TestJSONFlagIsUnknownOnACommandThatDoesNotDeclareIt is the same rule observed
// from the outside: --json is accepted where it is registered and fails loudly
// everywhere else, rather than being accepted and ignored.
func TestJSONFlagIsUnknownOnACommandThatDoesNotDeclareIt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"the command that declares it accepts it", []string{"renders", "--json"}, ""},
		{"a sibling that does not declare it rejects it", []string{"acts", "--json"}, "unknown flag: --json"},
		{"the root rejects it too", []string{"--json"}, "unknown flag: --json"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := NewRootCmd()
			renders := newLeafCmd("renders")
			addJSONFlag(renders)
			addCommand(root, newLeafCmd("acts"), groupSetup)
			addCommand(root, renders, groupAssets)
			root.SetArgs(tc.args)
			root.SetOut(&strings.Builder{})
			root.SetErr(&strings.Builder{})

			err := root.Execute()

			if tc.wantErr == "" {
				require.NoError(t, err)
				assert.True(t, jsonRequested(renders), "the flag the command declared must read back as requested")
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// TestJSONRequestedTreatsAnUndeclaredFlagAsNotRequested pins the read helper's
// contract: asking a command that has no JSON view is a truthful no, not a
// panic, so shared rendering helpers can ask unconditionally.
func TestJSONRequestedTreatsAnUndeclaredFlagAsNotRequested(t *testing.T) {
	t.Parallel()

	declared := newLeafCmd("declared")
	addJSONFlag(declared)
	require.False(t, jsonRequested(declared), "an unset --json is not a request")
	require.NoError(t, declared.Flags().Set("json", "true"))
	assert.True(t, jsonRequested(declared), "a set --json is a request")

	assert.False(t, jsonRequested(newLeafCmd("undeclared")),
		"a command with no --json flag must answer no rather than panicking")
}

// TestHelpGroupsAreDeclaredInDisplayOrder holds the coupling addCommand relies
// on: a group is declared the first time a command uses it, so attaching
// commands out of helpGroups order would silently reorder the help output.
func TestHelpGroupsAreDeclaredInDisplayOrder(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()
	addCommand(root, newLeafCmd("setup-verb"), groupSetup)
	addCommand(root, newLeafCmd("asset-verb"), groupAssets)
	addCommand(root, newLeafCmd("another-asset-verb"), groupAssets)

	got := make([]string, 0, len(root.Groups()))
	for _, g := range root.Groups() {
		got = append(got, g.ID)
	}
	assert.Equal(t, []string{groupSetup, groupAssets}, got,
		"each group is declared once, on first use, in the order commands are attached")

	usage := root.UsageString()
	assert.Less(t, strings.Index(usage, "Setup Commands:"), strings.Index(usage, "Asset Commands:"))
}

// TestNewRootCmdDeclaresNoEmptyHelpGroup keeps the help output honest while the
// command set is still being built: cobra prints a header for every declared
// group, so a group whose commands do not exist yet must stay undeclared rather
// than render as an empty heading.
func TestNewRootCmdDeclaresNoEmptyHelpGroup(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()
	for _, g := range root.Groups() {
		assert.True(t, groupHasMember(root, g.ID), "help group %q is declared but no command is filed under it", g.ID)
	}
}

func groupHasMember(root *cobra.Command, groupID string) bool {
	for _, c := range root.Commands() {
		if c.GroupID == groupID {
			return true
		}
	}
	return false
}

// TestEveryCommandIsFiledUnderADeclaredGroup checks at construction time the
// invariant cobra only panics over at execute time.
func TestEveryCommandIsFiledUnderADeclaredGroup(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()
	for _, c := range root.Commands() {
		if c.GroupID == "" {
			continue // cobra's own generated help and completion commands
		}
		assert.True(t, root.ContainsGroup(c.GroupID), "command %q is filed under undeclared group %q", c.Name(), c.GroupID)
	}
}

func TestAddCommandRejectsAnUnknownHelpGroup(t *testing.T) {
	t.Parallel()

	root := NewRootCmd()
	assert.PanicsWithValue(t, `command "stray" filed under unknown help group "nowhere"`, func() {
		addCommand(root, newLeafCmd("stray"), "nowhere")
	})
}

// TestPackageDeclaresNoInit asserts by construction that no command registers
// itself from a package init, so NewRootCmd is the whole tree and reading it is
// enough to know what harnaas accepts. The gochecknoinits linter says the same,
// but a lint rule can be silenced by a comment and this cannot.
func TestPackageDeclaresNoInit(t *testing.T) {
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
				"%s declares a package init; attach commands from NewRootCmd instead", name)
		}
	}
}
