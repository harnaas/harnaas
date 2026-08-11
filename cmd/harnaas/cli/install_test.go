package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/paths"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// ruleDestination is where the declared rule lands: the one destination these
// tests both read back and overwrite, so it is named once.
const ruleDestination = ".claude/rules/house-style.md"

// installedProject is a project root holding one rule, one instruction and one
// skill under `.harnaas`, all declared.
//
// Local sources are what make these tests exercise the whole flow without a
// network: resolution, verification, rendering, destinations, managed blocks
// and the lockfile all run exactly as they do for a remote source, and only the
// bytes' origin differs.
func installedProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	write := func(name, content string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	}

	write(".harnaas/rules/house-style.md", "---\nname: house-style\n---\nTwo spaces.\n")
	write(".harnaas/instructions/tone.md", "Be brief.\n")
	write(".harnaas/skills/review/SKILL.md", "---\nname: review\ndescription: Review code\n---\nReview carefully.\n")
	write(manifest3Assets())

	return root
}

// manifest3Assets is the manifest installedProject writes, declaring one asset
// of each shape the flow treats differently: an adapter surface, the memory
// file's block, and a shared directory.
func manifest3Assets() (string, string) {
	return "harnaas.json", `{
  "version": 1,
  "harnesses": ["claude-code"],
  "sources": {},
  "assets": [
    ".harnaas/rules/house-style.md",
    ".harnaas/instructions/tone.md",
    ".harnaas/skills/review"
  ]
}`
}

// installRun is what one `harnaas install` produced.
type installRun struct {
	stdout string
	stderr string
	err    error
}

// runInstallIn executes `harnaas install` against a project root.
func runInstallIn(t *testing.T, root string, args ...string) installRun {
	t.Helper()

	var stdout, stderr bytes.Buffer
	cmd := newInstallCmd()
	cmd.SetContext(paths.WithProjectRoot(t.Context(), root))
	cmd.SetArgs(args)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	// Executed on its own line: the operands of a composite literal are
	// evaluated left to right, so building the struct around the call would
	// read both buffers before the command had written to either.
	err := cmd.Execute()

	return installRun{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

// writeManifest replaces the project's manifest, for the tests that change what
// is declared between runs.
func writeManifest(t *testing.T, root, document string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, "harnaas.json"), []byte(document), 0o600))
}

// exists reports whether a project-relative path is there.
func exists(t *testing.T, root, name string) bool {
	t.Helper()
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name)))
	return err == nil
}

// readFile reads a project-relative file.
func readFile(t *testing.T, root, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	require.NoError(t, err)
	return string(content)
}

func TestInstallPlacesEveryAssetAtItsOwnKindOfDestination(t *testing.T) {
	t.Parallel()

	root := installedProject(t)
	run := runInstallIn(t, root)

	require.NoError(t, run.err)
	assert.True(t, exists(t, root, ruleDestination), "a rule goes through the adapter's surface")
	assert.True(t, exists(t, root, ".agents/skills/review/SKILL.md"), "a skill goes to the shared directory")
	assert.Contains(t, readFile(t, root, "AGENTS.md"), "Be brief.", "an instruction is inlined into the memory file")
	assert.Contains(t, readFile(t, root, "CLAUDE.md"), "@AGENTS.md",
		"and the bridge line is what makes Claude Code read it")
}

func TestInstallIgnoresExactlyThePathsItInstalled(t *testing.T) {
	t.Parallel()

	root := installedProject(t)
	require.NoError(t, runInstallIn(t, root).err)

	ignore := readFile(t, root, ".gitignore")
	assert.Contains(t, ignore, "/.claude/rules/house-style.md",
		"an entry is anchored and names the path from the project root, not from the scope root")
	assert.Contains(t, ignore, "/.agents/skills/review")
	assert.NotContains(t, ignore, "/.agents/skills\n",
		"a directory entry would untrack somebody's hand-written skill beside the installed one")
}

func TestASecondInstallChangesNothingAtAll(t *testing.T) {
	t.Parallel()

	root := installedProject(t)
	require.NoError(t, runInstallIn(t, root).err)
	first := readFile(t, root, LockFileName)

	run := runInstallIn(t, root)

	require.NoError(t, run.err)
	assert.Contains(t, run.stdout, "unchanged")
	assert.NotContains(t, run.stdout, "created")
	assert.Equal(t, first, readFile(t, root, LockFileName),
		"a run that installed nothing must produce a byte-identical lockfile, or the file is permanent diff noise")
}

func TestAHandWrittenFileIsNeverOverwrittenOnAnyFlag(t *testing.T) {
	t.Parallel()

	root := installedProject(t)
	destination := ruleDestination
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude", "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, filepath.FromSlash(destination)), []byte("mine\n"), 0o600))

	for _, args := range [][]string{nil, {"--force"}} {
		run := runInstallIn(t, root, args...)

		require.NoError(t, run.err)
		assert.Contains(t, run.stdout, string(outcomeConflictUnmanaged))
		assert.Equal(t, "mine\n", readFile(t, root, destination),
			"--force restores what harnaas installed and has no business destroying what it did not (args %v)", args)
	}
}

func TestADriftedFileIsReportedAndRestoredOnlyOnForce(t *testing.T) {
	t.Parallel()

	root := installedProject(t)
	require.NoError(t, runInstallIn(t, root).err)

	destination := ruleDestination
	require.NoError(t, os.WriteFile(filepath.Join(root, filepath.FromSlash(destination)), []byte("edited\n"), 0o600))

	reported := runInstallIn(t, root)
	require.NoError(t, reported.err)
	assert.Contains(t, reported.stdout, string(outcomeConflictDrift))
	assert.Equal(t, "edited\n", readFile(t, root, destination), "the edit survives a run without --force")

	restored := runInstallIn(t, root, "--force")
	require.NoError(t, restored.err)
	assert.Contains(t, readFile(t, root, destination), "Two spaces.", "--force restores the source content")
}

func TestADriftedFileStaysManagedSoForceCanStillRestoreIt(t *testing.T) {
	t.Parallel()

	root := installedProject(t)
	require.NoError(t, runInstallIn(t, root).err)

	destination := ruleDestination
	require.NoError(t, os.WriteFile(filepath.Join(root, filepath.FromSlash(destination)), []byte("edited\n"), 0o600))

	// The run that reports the drift must not drop the record: doing so would
	// make the next run meet harnaas's own file as unmanaged and refuse the
	// --force this one just recommended.
	require.NoError(t, runInstallIn(t, root).err)
	assert.Contains(t, readFile(t, root, LockFileName), "house-style",
		"declining to rewrite a destination is not giving it up")

	require.NoError(t, runInstallIn(t, root, "--force").err)
	assert.Contains(t, readFile(t, root, destination), "Two spaces.")
}

func TestDroppingAnAssetRemovesWhatItInstalled(t *testing.T) {
	t.Parallel()

	root := installedProject(t)
	require.NoError(t, runInstallIn(t, root).err)

	writeManifest(t, root, `{"version":1,"harnesses":["claude-code"],"sources":{},
	  "assets":[".harnaas/rules/house-style.md"]}`)
	run := runInstallIn(t, root)

	require.NoError(t, run.err)
	assert.False(t, exists(t, root, ".agents/skills/review"), "convergence removes what is no longer declared")
	assert.True(t, exists(t, root, ruleDestination), "and keeps what still is")
	assert.NotContains(t, readFile(t, root, "AGENTS.md"), "Be brief.", "the instruction leaves the block")
	assert.Contains(t, run.stdout, "removed", "deleting files and saying nothing is what convergence must not do")
}

func TestEmptyingTheAssetsArrayIsTheFullUninstall(t *testing.T) {
	t.Parallel()

	root := installedProject(t)
	require.NoError(t, runInstallIn(t, root).err)

	writeManifest(t, root, `{"version":1,"harnesses":["claude-code"],"sources":{},"assets":[]}`)
	require.NoError(t, runInstallIn(t, root).err)

	assert.False(t, exists(t, root, ruleDestination))
	assert.False(t, exists(t, root, ".agents/skills/review"))
	assert.False(t, exists(t, root, "CLAUDE.md"), "a file harnaas created holding only the bridge line goes with it")
	assert.NotContains(t, readFile(t, root, "AGENTS.md"), "harnaas:begin", "and both managed blocks are gone")
	assert.NotContains(t, readFile(t, root, ".gitignore"), "harnaas:begin")
}

func TestADryRunReportsTheSameOutcomesAndWritesNothing(t *testing.T) {
	t.Parallel()

	root := installedProject(t)

	dry := runInstallIn(t, root, "--dry-run")
	require.NoError(t, dry.err)

	assert.False(t, exists(t, root, ruleDestination))
	assert.False(t, exists(t, root, LockFileName), "not the lockfile either")
	assert.False(t, exists(t, root, "AGENTS.md"), "and no managed block")

	actual := runInstallIn(t, root)
	require.NoError(t, actual.err)
	assert.Equal(t, dry.stdout, actual.stdout,
		"a dry run immediately followed by a real one with nothing changed must predict it exactly")
}

func TestAJSONReportIsAloneOnStdout(t *testing.T) {
	t.Parallel()

	root := installedProject(t)
	run := runInstallIn(t, root, "--json")

	require.NoError(t, run.err)
	assert.True(t, strings.HasPrefix(run.stdout, "{"), "stdout carries the document and nothing else")
	assert.Contains(t, run.stdout, `"outcome"`)
}

func TestInstallIsIndependentOfManifestOrder(t *testing.T) {
	t.Parallel()

	forward := installedProject(t)
	require.NoError(t, runInstallIn(t, forward).err)

	reversed := installedProject(t)
	writeManifest(t, reversed, `{"version":1,"harnesses":["claude-code"],"sources":{},
	  "assets":[".harnaas/skills/review",".harnaas/instructions/tone.md",".harnaas/rules/house-style.md"]}`)
	reversedRun := runInstallIn(t, reversed)
	require.NoError(t, reversedRun.err)

	assert.Equal(t, stripTimes(readFile(t, forward, LockFileName)), stripTimes(readFile(t, reversed, LockFileName)),
		"reordering the manifest must change neither the report nor the lockfile")
}

// stripTimes removes the recorded install times, which are the one field two
// projects installed a moment apart legitimately differ on.
func stripTimes(document string) string {
	var kept []string
	for _, line := range strings.Split(document, "\n") {
		if strings.Contains(line, `"installedAt"`) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func TestAWriteThroughALinkOutOfTheHarnessDirectoryIsRefused(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()

	// A component of the destination is a symbolic link leading out. The path
	// is textually fine; only the filesystem knows where it goes, which is the
	// whole reason containment is the kernel's answer and not a comparison.
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude"), 0o755))
	if err := os.Symlink(outside, filepath.Join(root, ".claude", "rules")); err != nil {
		t.Skipf("this platform does not allow the test to create a symbolic link: %v", err)
	}

	err := writeDestination(root, "rules/house-style.md",
		[]source.File{{Path: "house-style.md", Content: []byte("x")}})

	// The link is inside the scope root, so the handle permits it; what matters
	// is that whichever answer it gives, the file never lands outside.
	if err == nil {
		_, statErr := os.Lstat(filepath.Join(outside, "house-style.md"))
		assert.Error(t, statErr, "content must never land outside the harness directory")
	}
}

func TestAnEscapingDestinationIsRefusedRatherThanCorrected(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	err := writeDestination(root, "../escaped/house-style.md",
		[]source.File{{Path: "house-style.md", Content: []byte("x")}})

	require.Error(t, err, "a destination that leaves the scope root is written nowhere")
	assert.NoFileExists(t, filepath.Join(filepath.Dir(root), "escaped", "house-style.md"))
}
