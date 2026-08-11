package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// commandSurface is everything harnaas accepts, and whether each command
// renders its result as a JSON document.
//
// It is written out here rather than derived from the tree, because a test that
// asked the tree what the tree contains would agree with any tree. Declared, it
// makes adding a command a two-line change — the constructor call in
// NewRootCmd, and a line here — and the second line is the moment somebody
// decides whether the new verb has a machine-readable view. A command whose
// result a CI job or a coding agent would want to read and which answers false
// here is a decision, not an oversight.
//
// Nothing declares --json yet: `init` writes a file and prints a path, and a
// document restating the path it just printed would be a JSON view invented to
// have one. `install` and `lint` arrive with their own changes and their own
// lines here.
var commandSurface = map[string]bool{
	// `lint` is the CI gate: its whole purpose is a machine deciding whether a
	// harness is in the state the manifest declares, so a document is the
	// primary output rather than a convenience.
	"harnaas lint": true,
	"harnaas":      false,
	"harnaas init": false,
	// `install` reports an outcome per asset and target, and the primary
	// consumers are CI jobs and coding agents deciding whether a harness is in
	// the state the manifest declares. That is exactly the result worth reading
	// as a document rather than scraping out of a table.
	"harnaas install": true,
}

// cobraGenerated names the commands cobra adds on its own behalf. They are not
// harnaas's surface — nobody here decides what they accept — so the walk skips
// them rather than the surface map having to list them.
var cobraGenerated = map[string]bool{
	"help":       true,
	"completion": true,
}

// walkCommands visits cmd and every harnaas command beneath it, passing the
// full command path a user would type.
func walkCommands(cmd *cobra.Command, path string, visit func(path string, cmd *cobra.Command)) {
	visit(path, cmd)
	for _, child := range cmd.Commands() {
		if cobraGenerated[child.Name()] {
			continue
		}
		walkCommands(child, path+" "+child.Name(), visit)
	}
}

// TestCommandTreeMatchesTheDeclaredSurface is the wiring guardrail: the tree
// NewRootCmd builds and the surface declared above are the same set, and each
// command's --json availability is the one declared for it.
//
// Comparing the whole map in one assertion rather than looping is deliberate: a
// command added without a line here fails as a diff naming it, and so does a
// command that quietly grew or lost a --json flag.
func TestCommandTreeMatchesTheDeclaredSurface(t *testing.T) {
	t.Parallel()

	got := map[string]bool{}
	walkCommands(NewRootCmd(), "harnaas", func(path string, cmd *cobra.Command) {
		got[path] = cmd.Flags().Lookup(jsonFlagName) != nil
	})

	assert.Equal(t, commandSurface, got,
		"the command tree and the declared surface disagree; update commandSurface when adding a command or a --json view")
}

// TestEveryCommandIsReachableFromTheRoot checks the other half of wiring: a
// command that exists in the tree is one a user can actually run. A command
// constructed and attached under a name cobra resolves to something else, or
// attached with neither a RunE nor subcommands, is unreachable in the only
// sense that matters.
func TestEveryCommandIsReachableFromTheRoot(t *testing.T) {
	t.Parallel()

	for path := range commandSurface {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			root := NewRootCmd()
			args := strings.Fields(path)[1:] // drop the binary name

			found, remaining, err := root.Find(args)
			require.NoError(t, err)
			assert.Equal(t, path, found.CommandPath(), "%q resolved to a different command", path)
			assert.Empty(t, remaining, "part of %q was not matched as a command", path)
			assert.True(t, found.Runnable() || found.HasSubCommands(),
				"%q can be typed but does nothing: it has neither a RunE nor subcommands", path)
		})
	}
}

// TestNoCommandInheritsAFlag is the rule that gives --json its meaning, checked
// over the real tree rather than over a synthetic child: nothing is inherited,
// so a flag a command accepts is a flag that command honours.
func TestNoCommandInheritsAFlag(t *testing.T) {
	t.Parallel()

	walkCommands(NewRootCmd(), "harnaas", func(path string, cmd *cobra.Command) {
		assert.False(t, cmd.InheritedFlags().HasFlags(),
			"%q inherits flags it did not declare and may not honour:\n%s", path, cmd.InheritedFlags().FlagUsages())
	})
}

// TestJSONResultIsAloneOnStdoutThroughTheWholeCommandPath is the output
// contract observed from outside: a command registers --json with the shared
// helper, reads it back with the shared helper, and renders through the shared
// helper — and the result of all three is a stdout carrying one parseable
// document with every advisory line on stderr.
//
// output_test.go tests the helpers directly; this drives them through cobra's
// own execution, which is the path a user's `harnaas … --json | jq` takes.
func TestJSONResultIsAloneOnStdoutThroughTheWholeCommandPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantStdout string
	}{
		{
			name:       "without the flag the result is the human text",
			args:       []string{"renders"},
			wantStdout: "installed 2 assets\n",
		},
		{
			name:       "with the flag the result is the document alone",
			args:       []string{"renders", "--json"},
			wantStdout: "{\n  \"installed\": 2\n}\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			root := NewRootCmd()
			addCommand(root, newRenderingCmd(), groupAssets)
			root.SetArgs(tc.args)
			root.SetOut(&stdout)
			root.SetErr(&stderr)

			require.NoError(t, root.Execute())

			assert.Equal(t, tc.wantStdout, stdout.String())
			assert.Equal(t, "Resolving 2 assets…\n", stderr.String(),
				"every advisory line belongs on stderr whether or not --json was requested")
		})
	}

	t.Run("the document parses on its own", func(t *testing.T) {
		t.Parallel()

		var stdout bytes.Buffer
		root := NewRootCmd()
		addCommand(root, newRenderingCmd(), groupAssets)
		root.SetArgs([]string{"renders", "--json"})
		root.SetOut(&stdout)
		root.SetErr(&bytes.Buffer{})

		require.NoError(t, root.Execute())

		var doc map[string]any
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc),
			"stdout must parse as a single JSON document, got %q", stdout.String())
		assert.Equal(t, map[string]any{"installed": float64(2)}, doc)
	})
}

// newRenderingCmd is a stand-in for a command with a JSON view, wired the way a
// real one would be: --json registered with addJSONFlag, read with
// jsonRequested, rendered with printJSON, and everything else through advisef.
// It exists because no command in this change has a JSON view yet, and the
// contract has to be guarded before the first one does.
func newRenderingCmd() *cobra.Command {
	cmd := newLeafCmd("renders")
	addJSONFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		advisef(cmd, "Resolving %d assets…\n", 2)

		if jsonRequested(cmd) {
			return printJSON(cmd, map[string]int{"installed": 2})
		}

		fmt.Fprintf(cmd.OutOrStdout(), "installed %d assets\n", 2)
		return nil
	}
	return cmd
}
