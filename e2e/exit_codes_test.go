//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The exit-code contract, restated here as the numbers a shell reads. They are
// deliberately spelled out rather than imported: cmd/harnaas declares them for
// the entrypoint's own use, and a test that imported them would agree with a
// renumbering instead of catching one.
const (
	exitSuccess      = 0
	exitFailure      = 1
	exitLintFindings = 2
)

// TestExitCodes runs every way this binary can succeed and every way it can
// fail, and pins the status each one exits with.
//
// The negative half is why the table is a table. Exit `2` is reserved for a
// `lint` run that completed and reported error-severity findings, and `lint`
// does not exist yet — so nothing this binary can be asked to do may produce
// it. Covering every failure path is how that stays true as commands are added:
// a verb that starts returning `2` for an ordinary failure fails here, rather
// than in a CI job that reads the status as "your harness has drifted" and
// carries on.
func TestExitCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// setup returns the directory the command runs in, so a case can
		// arrange the project state its failure needs.
		setup      func(t *testing.T) string
		args       []string
		want       int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "reporting the version succeeds",
			setup:      newProject,
			args:       []string{"--version"},
			want:       exitSuccess,
			wantStdout: "harnaas ",
		},
		{
			name:       "printing help succeeds",
			setup:      newProject,
			args:       []string{"--help"},
			want:       exitSuccess,
			wantStdout: "Setup Commands:",
		},
		{
			name:       "creating a manifest succeeds",
			setup:      newProject,
			args:       []string{"init", "--yes"},
			want:       exitSuccess,
			wantStdout: "Created ",
		},
		{
			name:       "refusing to replace a manifest is a runtime failure",
			setup:      projectWithManifest,
			args:       []string{"init"},
			want:       exitFailure,
			wantStderr: "--force",
		},
		{
			name:       "running outside a repository is a runtime failure",
			setup:      newDirectoryOutsideARepository,
			args:       []string{"init", "--yes"},
			want:       exitFailure,
			wantStderr: "no project root found",
		},
		{
			name:       "an unrecognized command is a runtime failure",
			setup:      newProject,
			args:       []string{"bogus"},
			want:       exitFailure,
			wantStderr: "unknown command",
		},
		{
			name:       "an unrecognized flag is a runtime failure",
			setup:      newProject,
			args:       []string{"init", "--bogus"},
			want:       exitFailure,
			wantStderr: "unknown flag",
		},
		{
			name:       "an argument init does not take is a runtime failure",
			setup:      newProject,
			args:       []string{"init", "extra"},
			want:       exitFailure,
			wantStderr: "unknown command",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res := runHarnaas(t, tc.setup(t), tc.args...)

			assert.Equal(t, tc.want, res.ExitCode,
				"stdout:\n%s\nstderr:\n%s", res.Stdout, res.Stderr)

			// Asserted separately from the expected status, and not folded into
			// it: this is the rule the whole table exists to defend, and it has
			// to fail even for a case somebody has updated to expect `2`.
			assert.NotEqual(t, exitLintFindings, res.ExitCode,
				"exit 2 is reserved for a completed lint run reporting findings")

			if tc.wantStdout != "" {
				assert.Contains(t, res.Stdout, tc.wantStdout)
			}
			if tc.wantStderr != "" {
				assert.Contains(t, res.Stderr, tc.wantStderr)
			}
			if tc.want != exitSuccess {
				// A failure explains itself on stderr and nowhere else, so a
				// pipeline reading stdout gets nothing rather than half a
				// result followed by a diagnostic.
				assert.Empty(t, res.Stdout, "a failing run wrote to stdout")
			}
		})
	}
}

// projectWithManifest is a scratch project that has already been initialized,
// created by running the binary rather than by writing the file, so the refusal
// is tested against a manifest harnaas itself produced.
func projectWithManifest(t *testing.T) string {
	t.Helper()

	dir := newProject(t)
	res := runHarnaas(t, dir, "init", "--yes")
	require.Equal(t, exitSuccess, res.ExitCode, "arrange an initialized project: %s", res.Stderr)
	return dir
}

// TestRefusingToReplaceAManifestLeavesItUntouched pairs with the exit-code
// table: the status says the run failed, and this says the file it refused to
// replace is byte-for-byte what it was.
func TestRefusingToReplaceAManifestLeavesItUntouched(t *testing.T) {
	t.Parallel()

	dir := projectWithManifest(t)
	path := filepath.Join(dir, "harnaas.json")

	before, err := os.ReadFile(path)
	require.NoError(t, err)

	res := runHarnaas(t, dir, "init")
	require.Equal(t, exitFailure, res.ExitCode)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	// Compared line by line rather than as one string: a whole-document compare
	// of two JSON files is rewritten by testifylint into a semantic one, which
	// would pass on a manifest harnaas had silently reformatted.
	assert.Equal(t, strings.Split(string(before), "\n"), strings.Split(string(after), "\n"))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	assert.ElementsMatch(t, []string{".git", "harnaas.json"}, names,
		"a refused run left something behind")
}
