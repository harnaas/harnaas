package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/interactive"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/paths"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/uiform"
)

// initRun is one execution of `harnaas init` against a scratch project.
type initRun struct {
	// root is the project the command ran in.
	root string

	// stdout and stderr are the command's own streams, captured.
	stdout, stderr string

	// err is what the command returned, which is what the entrypoint would
	// render and turn into an exit code.
	err error
}

// manifestPath is where the run's manifest would be.
func (r initRun) manifestPath() string {
	return filepath.Join(r.root, manifest.FileName)
}

// runInitIn executes `harnaas init` in root with args, through the same
// constructor the root command attaches, so the flags under test are the ones a
// user gets.
func runInitIn(t *testing.T, root string, args ...string) initRun {
	t.Helper()
	return runInitWith(t, initCase{root: root, args: args})
}

// runInitFor executes `harnaas init` naming its harnesses, which is what a run
// with no terminal has to do: the prompting gate answers no under `go test`, and
// a run that can neither prompt nor read the flag is refused.
func runInitFor(t *testing.T, root string, targets ...harness.ID) initRun {
	t.Helper()

	args := make([]string, 0, len(targets)*2)
	for _, target := range targets {
		args = append(args, "--harness", string(target))
	}
	return runInitWith(t, initCase{root: root, args: args})
}

// initCase is the setup one run needs. It is a struct rather than a parameter
// list because the two cases that vary — a cancelled context, and a prompt with
// something on stdin — vary in different fields, and every other call would
// otherwise pass two zero values to reach the third.
type initCase struct {
	// root is the project root the command resolves everything from.
	root string

	// ctx is the run's context; the zero value takes the test's own.
	ctx context.Context //nolint:containedctx // this is a test fixture, not a struct the code under test holds

	// stdin is what a prompt would read; the zero value is an empty reader.
	stdin io.Reader

	// args are the command-line arguments after `init`.
	args []string
}

// runInitWith executes `harnaas init` for one case.
func runInitWith(t *testing.T, tc initCase) initRun {
	t.Helper()

	ctx := tc.ctx
	if ctx == nil {
		ctx = t.Context()
	}
	stdin := tc.stdin
	if stdin == nil {
		stdin = strings.NewReader("")
	}

	var stdout, stderr bytes.Buffer
	cmd := newInitCmd()
	cmd.SetContext(paths.WithProjectRoot(ctx, tc.root))
	cmd.SetArgs(tc.args)
	cmd.SetIn(stdin)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	err := cmd.Execute()

	return initRun{root: tc.root, stdout: stdout.String(), stderr: stderr.String(), err: err}
}

// scratchProject is an empty directory standing in for a project root. The root
// is passed in explicitly rather than discovered, so no test has to create a
// repository or move the process.
func scratchProject(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return root
}

// writeFile creates a file under root, creating parent directories.
func writeFile(t *testing.T, root, name, content string) string {
	t.Helper()
	full := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	return full
}

// entries lists a directory's entry names, sorted, so a test can assert on
// everything that is there rather than on the one file it thought to check.
func entries(t *testing.T, dir string) []string {
	t.Helper()
	found, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(found))
	for _, entry := range found {
		names = append(names, entry.Name())
	}
	return names
}

// loadManifest decodes and interprets the manifest a run created, through the
// real loader — the only thing that can say the scaffold is usable.
func loadManifest(t *testing.T, path string) (*manifest.Document, *manifest.Interpretation) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	doc, err := manifest.Decode(data)
	require.NoError(t, err, "the scaffolded manifest must decode under strict decoding")

	interpretation, err := manifest.Interpret(doc)
	require.NoError(t, err, "the scaffolded manifest must validate unmodified")

	return doc, interpretation
}

func TestInitCreatesAManifestThatLoadsCleanly(t *testing.T) {
	t.Parallel()

	run := runInitFor(t, scratchProject(t), harness.ClaudeCode)
	require.NoError(t, run.err)

	doc, interpretation := loadManifest(t, run.manifestPath())
	assert.Equal(t, manifest.SupportedVersion, doc.Version)
	assert.Equal(t, []string{string(harness.ClaudeCode)}, doc.Harnesses)
	assert.Empty(t, doc.Sources, "a scaffolded project declares no sources")
	assert.Empty(t, doc.Assets, "a scaffolded project declares no assets")
	assert.Empty(t, interpretation.Assets)
}

func TestScaffoldIsFormattedForHandEditing(t *testing.T) {
	t.Parallel()

	run := runInitFor(t, scratchProject(t), harness.ClaudeCode)
	require.NoError(t, run.err)

	data, err := os.ReadFile(run.manifestPath())
	require.NoError(t, err)

	// The whole file, line for line. Indentation and the empty containers are
	// the subject: an author opening this file has to see where a source and an
	// asset go, and a comparison that only parsed the JSON would pass on a
	// single unreadable line.
	assert.Equal(t, []string{
		`{`,
		`  "version": 1,`,
		`  "harnesses": [`,
		`    "claude-code"`,
		`  ],`,
		`  "sources": {},`,
		`  "assets": []`,
		`}`,
		``,
	}, strings.Split(string(data), "\n"))
}

func TestInitReportsThePathItCreatedAndTheNextCommand(t *testing.T) {
	t.Parallel()

	run := runInitFor(t, scratchProject(t), harness.ClaudeCode)
	require.NoError(t, run.err)

	assert.Contains(t, run.stdout, run.manifestPath(),
		"the created path is the result, so it belongs on stdout")
	assert.Contains(t, run.stdout, "Claude Code", "the report names what the manifest targets")
	assert.Contains(t, run.stdout, "harnaas install",
		"the next command to run must be named")
}

func TestInitPrintsRemainingSetupAsAdviceOnly(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	ignore := writeFile(t, root, ".gitignore", "node_modules/\n")

	run := runInitFor(t, root, harness.ClaudeCode)
	require.NoError(t, run.err)

	// The ignore file is named as somebody else's work, and nothing was done to
	// it.
	assert.Contains(t, run.stdout, "ignore-file")
	assert.Contains(t, run.stdout, "harnaas install")

	unchanged, err := os.ReadFile(ignore)
	require.NoError(t, err)
	assert.Equal(t, "node_modules/\n", string(unchanged))
}

// The guidance may not attribute work to a command that does not do it. It used
// to say `harnaas install` creates the local asset directory; install has never
// created it, and init now does.
func TestGuidanceDoesNotAttributeTheScaffoldingToInstall(t *testing.T) {
	t.Parallel()

	run := runInitFor(t, scratchProject(t), harness.ClaudeCode)
	require.NoError(t, run.err)

	created, next, found := strings.Cut(run.stdout, "Next:")
	require.True(t, found, "the report ends with what to do next")

	assert.Contains(t, created, manifest.LocalRoot,
		"the local asset directory is reported as something init created")
	assert.NotContains(t, next, manifest.LocalRoot,
		"and never as work left for another command")
}

// The `harnesses` list is a guarantee a team publishes, so what happens to be in
// the working tree does not decide it — in either direction.
func TestInitIgnoresWhatTheProjectContains(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	require.NoError(t, os.Mkdir(filepath.Join(root, ".claude"), 0o755))
	writeFile(t, root, "AGENTS.md", "# Agents\n")

	run := runInitFor(t, root, harness.DevinCLI)
	require.NoError(t, run.err)

	doc, _ := loadManifest(t, run.manifestPath())
	assert.Equal(t, []string{string(harness.DevinCLI)}, doc.Harnesses,
		"the selection is the whole answer; the .claude directory is not a second one")
	assert.NotContains(t, run.stdout, "Detected")
	assert.NotContains(t, run.stderr, "Detected")
}

// Every recognized harness is offered, in the roster's order, showing both the
// name a person knows it by and the id they would have to type.
func TestTheSelectionOffersTheWholeRoster(t *testing.T) {
	t.Parallel()

	choices := rosterChoices()

	require.Len(t, choices, len(harness.All()))
	for i, h := range harness.All() {
		assert.Equal(t, h.ID, choices[i].Value, "the roster's order is the offered order")
		assert.Contains(t, choices[i].Label, h.DisplayName)
		assert.Contains(t, choices[i].Label, string(h.ID),
			"the id is shown too: it is what --harness and the manifest take")
	}
}

func TestSelectionFromFlagsKeepsEachNameOnce(t *testing.T) {
	t.Parallel()

	targets, err := selectHarnesses(t.Context(), strings.NewReader(""), &bytes.Buffer{},
		[]string{"claude-code", "claude-code"})
	require.NoError(t, err)

	assert.Equal(t, []harness.ID{"claude-code"}, targets,
		"a name repeated across flag occurrences is written once")
}

func TestUnsupportedHarnessNameWritesNothing(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)

	run := runInitIn(t, root, "--harness", "claud-code")

	var unknown *harness.UnknownError
	require.ErrorAs(t, run.err, &unknown)
	assert.Equal(t, harness.ID("claud-code"), unknown.ID)
	assert.Contains(t, run.err.Error(), "claude-code", "the supported harnesses must be listed")
	assert.Empty(t, entries(t, root), "nothing is written when the selection is refused")
}

func TestExistingManifestIsRefusedAndLeftUntouched(t *testing.T) {
	t.Parallel()

	const existing = "{ \"version\": 1, \"harnesses\": [\"claude-code\"], \"sources\": {}, \"assets\": [] }\n"
	root := scratchProject(t)
	path := writeFile(t, root, manifest.FileName, existing)

	// No harness named, and no terminal — which is also refused. The manifest is
	// what this run is told about, because that is the fact that makes naming a
	// harness pointless rather than the other way round.
	run := runInitIn(t, root)

	var exists *manifestExistsError
	require.ErrorAs(t, run.err, &exists)
	assert.Equal(t, path, exists.Path)
	assert.Contains(t, run.err.Error(), "--force", "the refusal must name the flag that allows it")

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	// Byte for byte, not JSON-equal: a refused run must not reformat the file
	// either, and comparing the decoded documents would pass if it had.
	assert.Equal(t, strings.Split(existing, "\n"), strings.Split(string(after), "\n"),
		"the existing manifest is untouched")
	assert.Equal(t, []string{manifest.FileName}, entries(t, root),
		"a refused run leaves no staging file behind, and no scaffolding either")
}

func TestForceReplacesTheManifestInFull(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	// Longer than the scaffold, so a partial overwrite would leave a tail.
	path := writeFile(t, root, manifest.FileName,
		strings.Repeat("{\"version\": 1, \"harnesses\": [], \"sources\": {}, \"assets\": []}\n", 40))

	run := runInitFor(t, root, harness.ClaudeCode)
	require.Error(t, run.err, "an existing manifest is refused before anything else")

	run = runInitWith(t, initCase{
		root: root,
		args: []string{"--force", "--harness", string(harness.ClaudeCode)},
	})
	require.NoError(t, run.err)

	doc, _ := loadManifest(t, path)
	assert.Equal(t, []string{string(harness.ClaudeCode)}, doc.Harnesses)
}

// A run that can neither prompt nor read a selection is refused, and the refusal
// carries the whole of what the reader needs: the flag, and the ids it takes.
// Scaffolding a manifest from a guess would put a guarantee nobody chose into
// the file a team reviews, in the one environment where nobody reads the
// sentence saying so.
func TestPipedRunWithNoHarnessIsRefused(t *testing.T) {
	t.Parallel()

	// Buffers are what a piped run hands the command, and the prompting gate
	// answers no for them. The gate's own reasoning is interactive's to test.
	require.False(t, interactive.CanPrompt(strings.NewReader(""), &bytes.Buffer{}))

	root := scratchProject(t)
	run := runInitIn(t, root)

	var refused *noSelectionError
	require.ErrorAs(t, run.err, &refused)
	assert.Contains(t, run.err.Error(), "--harness")
	for _, h := range harness.All() {
		assert.Contains(t, run.err.Error(), string(h.ID), "every recognized id is named")
	}
	assert.Empty(t, entries(t, root), "a refused run writes nothing at all")
}

func TestPipedRunWithTheHarnessFlagCompletes(t *testing.T) {
	t.Parallel()

	run := runInitFor(t, scratchProject(t), harness.ClaudeCode)

	require.NoError(t, run.err)
	assert.FileExists(t, run.manifestPath())
}

func TestHarnessFlagSkipsThePromptOnATerminal(t *testing.T) {
	// Prompting is possible here, and naming the harnesses is what makes the run
	// take the non-interactive path deliberately. Forcing the gate on is also
	// why this test cannot run in parallel: t.Setenv is process-wide.
	t.Setenv(interactive.EnvTestTTY, "1")
	t.Setenv(uiform.EnvAccessible, "1")

	root := scratchProject(t)
	run := runInitFor(t, root, harness.ClaudeCode)

	require.NoError(t, run.err)
	assert.FileExists(t, run.manifestPath())
	assert.Empty(t, run.stderr, "no question is asked, so nothing is written to the prompt's stream")
}

// The selection is what fills the manifest, read from the answer the user gave.
func TestSelectionOnATerminalFillsTheManifest(t *testing.T) {
	t.Setenv(interactive.EnvTestTTY, "1")
	t.Setenv(uiform.EnvAccessible, "1")

	root := scratchProject(t)
	// Toggle both options, then submit. The accessible rendering numbers the
	// options in the order they were offered, which is the roster's.
	run := runInitWith(t, initCase{root: root, stdin: strings.NewReader("2\n1\n0\n")})

	require.NoError(t, run.err)
	doc, _ := loadManifest(t, run.manifestPath())
	assert.Equal(t, []string{string(harness.ClaudeCode), string(harness.DevinCLI)}, doc.Harnesses,
		"the answer is written in the roster's order, not the order the boxes were ticked")
	assert.Contains(t, run.stderr, "Which harnesses",
		"the prompt renders on stderr, so stdout carries the result alone")
}

func TestCancelledPromptWritesNothingAndFails(t *testing.T) {
	t.Setenv(interactive.EnvTestTTY, "1")
	t.Setenv(uiform.EnvAccessible, "1")

	// A cancelled context is what a signal leaves behind: the question was put
	// and the user walked away from it.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	root := scratchProject(t)
	run := runInitWith(t, initCase{root: root, ctx: ctx})

	require.ErrorIs(t, run.err, uiform.ErrCancelled)
	assert.Contains(t, run.err.Error(), manifest.FileName,
		"the message says what was not created")
	assert.Empty(t, entries(t, root), "a cancelled prompt writes nothing")
}

// Submitting a selection with nothing chosen is an answer, and an empty
// `harnesses` list is not a manifest harnaas will write. It is refused as the
// same problem a flagless non-interactive run has — nobody named a harness — and
// with the same fix.
func TestEmptySelectionWritesNothingAndFails(t *testing.T) {
	t.Setenv(interactive.EnvTestTTY, "1")
	t.Setenv(uiform.EnvAccessible, "1")

	root := scratchProject(t)
	run := runInitWith(t, initCase{root: root, stdin: strings.NewReader("0\n")})

	var refused *noSelectionError
	require.ErrorAs(t, run.err, &refused)
	require.NotErrorIs(t, run.err, uiform.ErrCancelled,
		"the question was answered; only the answer was empty")
	assert.Contains(t, run.err.Error(), "--harness")
	assert.Empty(t, entries(t, root))
}

func TestInitLeavesEveryFileItDoesNotOwnAlone(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	before := map[string]string{
		".gitignore": "dist/\n",
		"AGENTS.md":  "# Agents\n\nProject instructions.\n",
		"CLAUDE.md":  "# Claude\n\nProject instructions.\n",
	}
	for name, content := range before {
		writeFile(t, root, name, content)
	}

	run := runInitFor(t, root, harness.ClaudeCode)
	require.NoError(t, run.err)

	for name, content := range before {
		after, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		assert.Equal(t, content, string(after), "%s must be byte-for-byte unchanged", name)
	}

	assert.Equal(t, []string{".gitignore", manifest.LocalRoot, "AGENTS.md", "CLAUDE.md", manifest.FileName},
		entries(t, root), "the manifest and the local asset directory are what appeared")
}

func TestInitCreatesNoDestinationAHarnessReads(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)

	run := runInitFor(t, root, harness.ClaudeCode)
	require.NoError(t, run.err)

	// The harness the manifest declares has no directory here, and init did not
	// make one: a destination is managed only once the lockfile records it, and
	// init writes no lockfile, so anything it created there would be unmanaged
	// and the next install would conflict with init's own output. `.harnaas` is
	// on the other side of that line — harnaas only ever reads it. See ADR 0006.
	assert.Equal(t, []string{manifest.LocalRoot, manifest.FileName}, entries(t, root))
	for _, absent := range []string{".claude", ".devin", "CLAUDE.md", "AGENTS.md", ".gitignore"} {
		assert.NoFileExists(t, filepath.Join(root, absent))
		assert.NoDirExists(t, filepath.Join(root, absent))
	}
}

func TestInitTakesNoPositionalArguments(t *testing.T) {
	t.Parallel()

	run := runInitIn(t, scratchProject(t), "claude-code")

	require.Error(t, run.err, "a harness named as an argument is a mistake worth reporting")
	assert.Empty(t, entries(t, run.root))
}

func TestInitFlagsAreRegisteredLocally(t *testing.T) {
	t.Parallel()

	cmd := newInitCmd()

	assert.False(t, cmd.HasPersistentFlags(),
		"init's flags apply to init, so they are registered on it and inherited by nothing")
	for _, name := range []string{"force", "harness"} {
		assert.NotNil(t, cmd.Flags().Lookup(name), "--%s must be registered on init", name)
	}
	assert.Nil(t, cmd.Flags().Lookup("yes"),
		"there is no selection to accept without prompting, so no flag accepts one")
	assert.Nil(t, cmd.Flags().Lookup("json"),
		"init's result is a file it created, so it declares no --json document")
}

func TestInitIsAttachedToTheRootUnderSetup(t *testing.T) {
	t.Parallel()

	var found *cobra.Command
	for _, child := range NewRootCmd().Commands() {
		if child.Name() == "init" {
			found = child
		}
	}

	require.NotNil(t, found, "init must be reachable from the root command")
	assert.Equal(t, groupSetup, found.GroupID)
}
