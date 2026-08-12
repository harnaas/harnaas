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

	run := runInitIn(t, scratchProject(t))
	require.NoError(t, run.err)

	doc, interpretation := loadManifest(t, run.manifestPath())
	assert.Equal(t, manifest.SupportedVersion, doc.Version)
	assert.Equal(t, []string{string(harness.Default)}, doc.Harnesses)
	assert.Empty(t, doc.Sources, "a scaffolded project declares no sources")
	assert.Empty(t, doc.Assets, "a scaffolded project declares no assets")
	assert.Empty(t, interpretation.Assets)
}

func TestScaffoldIsFormattedForHandEditing(t *testing.T) {
	t.Parallel()

	run := runInitIn(t, scratchProject(t))
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

	run := runInitIn(t, scratchProject(t))
	require.NoError(t, run.err)

	assert.Contains(t, run.stdout, run.manifestPath(),
		"the created path is the result, so it belongs on stdout")
	assert.Contains(t, run.stdout, "harnaas install",
		"the next command to run must be named")
}

func TestInitPrintsRemainingSetupAsAdviceOnly(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	ignore := writeFile(t, root, ".gitignore", "node_modules/\n")

	run := runInitIn(t, root)
	require.NoError(t, run.err)

	// Guidance about the ignore file and the local asset directory names the
	// command that does it, and nothing was done.
	assert.Contains(t, run.stdout, manifest.LocalRoot)
	assert.Contains(t, run.stdout, "ignore-file")
	assert.Contains(t, run.stdout, "harnaas install")

	unchanged, err := os.ReadFile(ignore)
	require.NoError(t, err)
	assert.Equal(t, "node_modules/\n", string(unchanged))
}

func TestInitDetectionPreFillsTheHarnessList(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	require.NoError(t, os.Mkdir(filepath.Join(root, ".claude"), 0o755))

	run := runInitIn(t, root)
	require.NoError(t, run.err)

	doc, _ := loadManifest(t, run.manifestPath())
	assert.Equal(t, []string{"claude-code"}, doc.Harnesses)
	assert.Contains(t, run.stderr, "Detected Claude Code")
	assert.NotContains(t, run.stderr, "No supported harness detected")
}

func TestInitDetectsDevinCLIFromItsOwnDirectoryAlone(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	require.NoError(t, os.Mkdir(filepath.Join(root, ".devin"), 0o755))

	run := runInitIn(t, root)
	require.NoError(t, run.err)

	doc, _ := loadManifest(t, run.manifestPath())
	assert.Equal(t, []string{"devin-cli"}, doc.Harnesses)
	assert.Contains(t, run.stderr, "Detected Devin CLI")
}

// TestTheSharedMemoryFileDetectsNoHarnessOnItsOwn is the exclusion at the level a
// user meets it. Most recognized harnesses read `AGENTS.md`, so a project that has
// written one is not thereby using any particular harness — and scaffolding a
// manifest that says otherwise would have init invent a guarantee.
func TestTheSharedMemoryFileDetectsNoHarnessOnItsOwn(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	writeFile(t, root, "AGENTS.md", "# Agents\n")

	detected := detectHarnesses(root, harness.All())

	assert.Empty(t, detected)
}

func TestEveryDetectedHarnessAppearsInRosterOrder(t *testing.T) {
	t.Parallel()

	// The synthetic roster stays after the second real harness landed, because it
	// is what proves the rule for entries the real roster does not have: a harness
	// detected on the second of two evidence paths, and one detected on neither.
	root := scratchProject(t)
	writeFile(t, root, "beta-dir/config", "")
	writeFile(t, root, "GAMMA.md", "")

	detected := detectHarnesses(root, []harness.Harness{
		{ID: "alpha", DisplayName: "Alpha", ProjectEvidence: []string{".alpha"}},
		{ID: "beta", DisplayName: "Beta", ProjectEvidence: []string{"beta-dir"}},
		{ID: "gamma", DisplayName: "Gamma", ProjectEvidence: []string{".gamma", "GAMMA.md"}},
	})

	assert.Equal(t, []harness.ID{"beta", "gamma"}, detected,
		"every detected harness appears, in the roster's order, and an undetected one does not")
}

func TestDetectionCreatesNothing(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)

	detected := detectHarnesses(root, harness.All())

	assert.Empty(t, detected)
	assert.Empty(t, entries(t, root),
		"detection stats the evidence and nothing more; it must not create a directory to look in")
}

func TestInitFallsBackToTheDefaultHarnessAndSaysSo(t *testing.T) {
	t.Parallel()

	run := runInitIn(t, scratchProject(t))
	require.NoError(t, run.err)

	doc, _ := loadManifest(t, run.manifestPath())
	assert.Equal(t, []string{string(harness.Default)}, doc.Harnesses,
		"an empty harnesses list would declare assets and guarantee them for nothing")

	assert.Contains(t, run.stderr, "No supported harness detected")
	assert.Contains(t, run.stderr, "Claude Code",
		"the message must say which harness was chosen instead")
}

func TestHarnessFlagOverridesDetectionEntirely(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	require.NoError(t, os.Mkdir(filepath.Join(root, ".claude"), 0o755))

	run := runInitIn(t, root, "--harness", "claude-code")
	require.NoError(t, run.err)

	doc, _ := loadManifest(t, run.manifestPath())
	assert.Equal(t, []string{"claude-code"}, doc.Harnesses)
	assert.NotContains(t, run.stderr, "Detected",
		"detection is not consulted at all when the harnesses are named")
}

func TestSelectionFromFlagsIgnoresTheProjectAndKeepsEachNameOnce(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	require.NoError(t, os.Mkdir(filepath.Join(root, ".claude"), 0o755))

	selection, err := selectHarnesses(root, []string{"claude-code", "claude-code"})
	require.NoError(t, err)

	assert.Equal(t, originFlag, selection.Origin)
	assert.Equal(t, []harness.ID{"claude-code"}, selection.Harnesses,
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
		"a refused run leaves no staging file behind either")
}

func TestForceReplacesTheManifestInFull(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	// Longer than the scaffold, so a partial overwrite would leave a tail.
	path := writeFile(t, root, manifest.FileName,
		strings.Repeat("{\"version\": 1, \"harnesses\": [], \"sources\": {}, \"assets\": []}\n", 40))

	run := runInitIn(t, root, "--force")
	require.NoError(t, run.err)

	doc, _ := loadManifest(t, path)
	assert.Equal(t, []string{string(harness.Default)}, doc.Harnesses)
	assert.Equal(t, []string{manifest.FileName}, entries(t, root))
}

func TestPipedRunCompletesWithoutPrompting(t *testing.T) {
	t.Parallel()

	// Buffers are what a piped run hands the command, and the prompting gate
	// answers no for them. The gate's own reasoning is interactive's to test;
	// what this proves is that init takes the non-interactive path from there —
	// it scaffolds from the detected values and returns rather than blocking on
	// input nobody is going to type.
	assert.False(t, interactive.CanPrompt(strings.NewReader(""), &bytes.Buffer{}))

	run := runInitIn(t, scratchProject(t))

	require.NoError(t, run.err)
	assert.FileExists(t, run.manifestPath())
}

func TestAssumeYesSkipsThePromptOnATerminal(t *testing.T) {
	// Prompting is possible here, and --yes is what makes the run take the
	// non-interactive path deliberately. Forcing the gate on is also why this
	// test cannot run in parallel: t.Setenv is process-wide.
	t.Setenv(interactive.EnvTestTTY, "1")

	root := scratchProject(t)
	run := runInitWith(t, initCase{root: root, args: []string{"--yes"}})

	require.NoError(t, run.err)
	assert.FileExists(t, run.manifestPath())
	assert.NotContains(t, run.stderr, "?", "no question is asked")
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

func TestDecliningThePromptWritesNothingAndSucceeds(t *testing.T) {
	t.Setenv(interactive.EnvTestTTY, "1")
	t.Setenv(uiform.EnvAccessible, "1")

	root := scratchProject(t)
	run := runInitWith(t, initCase{root: root, stdin: strings.NewReader("n\n")})

	// Declining and cancelling are different acts. Both write nothing; only
	// cancelling is an error, because only cancelling leaves the question
	// unanswered.
	require.NoError(t, run.err)
	assert.Empty(t, entries(t, root))
	assert.Contains(t, run.stderr, "Nothing was created")
}

func TestInitCreatesTheManifestAndNothingElse(t *testing.T) {
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

	run := runInitIn(t, root)
	require.NoError(t, run.err)

	for name, content := range before {
		after, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		assert.Equal(t, content, string(after), "%s must be byte-for-byte unchanged", name)
	}

	assert.Equal(t, []string{".gitignore", "AGENTS.md", "CLAUDE.md", manifest.FileName},
		entries(t, root), "harnaas.json is the only file that appeared")
}

func TestInitCreatesNoneOfTheFilesItLeavesAlone(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)

	run := runInitIn(t, root)
	require.NoError(t, run.err)

	// The harness the manifest declares has no directory here, and init did not
	// make one: a destination is managed only once the lockfile records it, and
	// init writes no lockfile, so anything else it created would be unmanaged
	// and the next install would conflict with init's own output.
	assert.Equal(t, []string{manifest.FileName}, entries(t, root))
	for _, absent := range []string{".claude", "CLAUDE.md", "AGENTS.md", ".gitignore", manifest.LocalRoot} {
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
	for _, name := range []string{"force", "yes", "harness"} {
		assert.NotNil(t, cmd.Flags().Lookup(name), "--%s must be registered on init", name)
	}
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
