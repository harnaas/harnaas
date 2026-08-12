package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// devinProject is a project root holding one asset of every type under
// `.harnaas`, with the manifest left to the caller.
//
// Local sources are what make these tests exercise the whole flow without a
// network, exactly as the Claude Code tests beside them do.
func devinProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	write := func(name, content string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	}

	write(".harnaas/rules/house-style.md", "---\nname: house-style\n---\nTwo spaces.\n")
	write(".harnaas/agents/reviewer.md", "---\nname: reviewer\n---\nReview carefully.\n")
	write(".harnaas/commands/deploy.md", "---\nname: deploy\n---\nShip it.\n")
	write(".harnaas/instructions/tone.md", "Be brief.\n")
	write(".harnaas/skills/review/SKILL.md", "---\nname: review\ndescription: Review code\n---\nReview carefully.\n")

	return root
}

func TestDevinCLIReceivesEachTypeAtItsOwnKindOfDestination(t *testing.T) {
	t.Parallel()

	root := devinProject(t)
	writeManifest(t, root, `{
  "version": 1,
  "harnesses": ["devin-cli"],
  "sources": {},
  "assets": [
    ".harnaas/rules/house-style.md",
    ".harnaas/agents/reviewer.md",
    ".harnaas/instructions/tone.md",
    ".harnaas/skills/review"
  ]
}`)

	run := runInstallIn(t, root)

	require.NoError(t, run.err)
	assert.True(t, exists(t, root, ".devin/rules/house-style.md"), "a rule goes through the adapter's surface")
	assert.True(t, exists(t, root, ".devin/agents/reviewer.md"), "a persona is one file, not a directory")
	assert.False(t, exists(t, root, ".devin/agents/reviewer/AGENT.md"), "the directory spelling is not used")
	assert.True(t, exists(t, root, ".agents/skills/review/SKILL.md"), "a skill goes to the shared directory")
	assert.False(t, exists(t, root, ".devin/skills"), "and is not copied into the harness's own directory")
	assert.Contains(t, readFile(t, root, "AGENTS.md"), "Be brief.",
		"an instruction reaches this harness through the memory file it reads natively")
}

// TestARuleInstallsForDevinCLIByteForByte is the identity rule at the level a user
// meets it. This harness spells its rule frontmatter differently from the one
// beside it, and harnaas neither translates the keys nor warns about them — the
// manifest names a source, and choosing one that suits the targets is the author's
// decision.
func TestARuleInstallsForDevinCLIByteForByte(t *testing.T) {
	t.Parallel()

	root := devinProject(t)
	source := readFile(t, root, ".harnaas/rules/house-style.md")
	writeManifest(t, root, `{
  "version": 1,
  "harnesses": ["devin-cli"],
  "sources": {},
  "assets": [".harnaas/rules/house-style.md"]
}`)

	require.NoError(t, runInstallIn(t, root).err)

	assert.Equal(t, source, readFile(t, root, ".devin/rules/house-style.md"))
}

// TestACommandTargetingDevinCLIIsUnsupported is ADR 0005 at the level a user meets
// it. The harness has a skill surface and cannot be told to leave a skill alone,
// so the command is refused rather than installed as one that the harness stays
// free to start unprompted.
func TestACommandTargetingDevinCLIIsUnsupported(t *testing.T) {
	t.Parallel()

	root := devinProject(t)
	writeManifest(t, root, `{
  "version": 1,
  "harnesses": ["devin-cli"],
  "sources": {},
  "assets": [".harnaas/commands/deploy.md"]
}`)

	run := runInstallIn(t, root)

	require.NoError(t, run.err, "one refused pairing is an outcome, not a failed run")
	assert.Contains(t, run.stdout, "unsupported")
	assert.Contains(t, run.stdout, "deploy")
	assert.False(t, exists(t, root, ".agents/skills/deploy"), "nothing is written for a refused pairing")
	assert.False(t, exists(t, root, ".devin/commands/deploy.md"), "and no path is invented for it")
	assert.NotContains(t, run.stdout, "deploy (devin-cli) -> ",
		"a pairing that wrote nothing names no destination, and least of all the memory file")
}

// TestTheRefusalNamesTheObstacleRatherThanAMissingSkillSurface pins the correction
// this change makes to an existing diagnostic. This harness plainly has a skill
// surface, and a message claiming otherwise would send its reader to check
// something that is not wrong.
func TestTheRefusalNamesTheObstacleRatherThanAMissingSkillSurface(t *testing.T) {
	t.Parallel()

	root := devinProject(t)
	writeManifest(t, root, `{
  "version": 1,
  "harnesses": ["devin-cli"],
  "sources": {},
  "assets": [".harnaas/commands/deploy.md"]
}`)

	run := runInstallIn(t, root)
	require.NoError(t, run.err)

	assert.Contains(t, run.stdout, "unprompted")
	assert.NotContains(t, run.stdout, "no skill surface",
		"this harness has one; the obstacle is that it cannot be silenced")
}

// TestACommandTargetingBothInstallsWhereItHasASurface is the per-pairing rule: one
// refusal never costs the asset its other targets.
func TestACommandTargetingBothInstallsWhereItHasASurface(t *testing.T) {
	t.Parallel()

	root := devinProject(t)
	writeManifest(t, root, `{
  "version": 1,
  "harnesses": ["claude-code", "devin-cli"],
  "sources": {},
  "assets": [".harnaas/commands/deploy.md"]
}`)

	run := runInstallIn(t, root)

	require.NoError(t, run.err)
	assert.True(t, exists(t, root, ".claude/commands/deploy.md"), "the harness with a command surface takes it")
	assert.Contains(t, run.stdout, "unsupported", "and the other pairing is reported")
}

// TestOneSharedSkillServesBothHarnesses is ADR 0002's bet, checkable for the first
// time now that two harnesses read the same directory.
func TestOneSharedSkillServesBothHarnesses(t *testing.T) {
	t.Parallel()

	root := devinProject(t)
	writeManifest(t, root, `{
  "version": 1,
  "harnesses": ["claude-code", "devin-cli"],
  "sources": {},
  "assets": [".harnaas/skills/review"]
}`)

	require.NoError(t, runInstallIn(t, root).err)

	assert.True(t, exists(t, root, ".agents/skills/review/SKILL.md"))
	assert.False(t, exists(t, root, ".claude/skills"), "no per-harness copy is made")
	assert.False(t, exists(t, root, ".devin/skills"), "for either harness")

	recorded := readFile(t, root, "harnaas.lock.json")
	assert.Contains(t, recorded, `"claude-code"`, "both harnesses claim the one destination")
	assert.Contains(t, recorded, `"devin-cli"`)
}

// TestDroppingOneHarnessLeavesTheSharedSkill is the other half of the same bet.
// The harness field is attribution, the file is one file, and removing it because
// one claimant went away would break the other.
func TestDroppingOneHarnessLeavesTheSharedSkill(t *testing.T) {
	t.Parallel()

	root := devinProject(t)
	writeManifest(t, root, `{
  "version": 1,
  "harnesses": ["claude-code", "devin-cli"],
  "sources": {},
  "assets": [".harnaas/skills/review"]
}`)
	require.NoError(t, runInstallIn(t, root).err)

	writeManifest(t, root, `{
  "version": 1,
  "harnesses": ["claude-code"],
  "sources": {},
  "assets": [".harnaas/skills/review"]
}`)
	require.NoError(t, runInstallIn(t, root).err)

	assert.True(t, exists(t, root, ".agents/skills/review/SKILL.md"),
		"the remaining harness still reads it")
	assert.NotContains(t, readFile(t, root, "harnaas.lock.json"), `"devin-cli"`,
		"only the attribution goes")
}

// TestAUserScopedRuleForDevinCLIIsUnsupported is the adapter's refusal reaching a
// user. The roster accepts `user` scope for this harness because a skill can take
// it; the adapter refuses a rule because there is no single per-user root to count
// one from, and a refusal is a refusal rather than a quieter install.
func TestAUserScopedRuleForDevinCLIIsUnsupported(t *testing.T) {
	t.Parallel()

	root := devinProject(t)
	writeManifest(t, root, `{
  "version": 1,
  "harnesses": ["devin-cli"],
  "sources": {},
  "assets": [
    { "source": ".harnaas/rules/house-style.md", "scope": "user" }
  ]
}`)

	run := runInstallIn(t, root)

	require.NoError(t, run.err)
	assert.Contains(t, run.stdout, "unsupported")
	assert.Contains(t, run.stdout, "user")
	assert.False(t, exists(t, root, ".devin/rules/house-style.md"),
		"a refused user scope never becomes a project-scope install")
}

// TestAUserScopedSkillForDevinCLIInstalls is why the roster records a per-user
// location at all: this asset reaches the harness through the shared per-user
// skills directory, which no adapter computes and which the adapter's refusal
// above therefore does not touch.
func TestAUserScopedSkillForDevinCLIInstalls(t *testing.T) {
	t.Parallel()

	root := devinProject(t)
	writeManifest(t, root, `{
  "version": 1,
  "harnesses": ["devin-cli"],
  "sources": {},
  "assets": [
    { "source": ".harnaas/skills/review", "scope": "user" }
  ]
}`)

	run := runInstallIn(t, root)
	require.NoError(t, run.err)

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(home, ".agents", "skills", "review", "SKILL.md"))
}

// TestLintReportsDriftAtADevinCLIDestination proves lint reads this harness's
// destinations the way it reads any other: it re-derives the scope root exactly as
// install did, and never enumerates the roster to do it.
func TestLintReportsDriftAtADevinCLIDestination(t *testing.T) {
	t.Parallel()

	root := devinProject(t)
	writeManifest(t, root, `{
  "version": 1,
  "harnesses": ["devin-cli"],
  "sources": {},
  "assets": [".harnaas/rules/house-style.md"]
}`)
	require.NoError(t, runInstallIn(t, root).err)

	installed := filepath.Join(root, filepath.FromSlash(".devin/rules/house-style.md"))
	require.NoError(t, os.WriteFile(installed, []byte("Edited by hand.\n"), 0o600))

	stdout, err := runLintIn(t, root, "--offline")

	require.Error(t, err, "drift is an error-severity finding")
	assert.Contains(t, stdout, "house-style")
	assert.Contains(t, stdout, "was modified outside harnaas")
	assert.NotContains(t, stdout, "is recorded as installed and is missing",
		"lint re-derived the same scope root install used; a different one would report the file absent")
}

// TestAHandWrittenFileAtADevinCLIDestinationIsNeverOverwritten is ADR 0001 at this
// harness's directory: a destination harnaas did not install is somebody's file,
// and no flag makes install take it.
func TestAHandWrittenFileAtADevinCLIDestinationIsNeverOverwritten(t *testing.T) {
	t.Parallel()

	root := devinProject(t)
	writeManifest(t, root, `{
  "version": 1,
  "harnesses": ["devin-cli"],
  "sources": {},
  "assets": [".harnaas/rules/house-style.md"]
}`)

	occupied := filepath.Join(root, filepath.FromSlash(".devin/rules/house-style.md"))
	require.NoError(t, os.MkdirAll(filepath.Dir(occupied), 0o755))
	require.NoError(t, os.WriteFile(occupied, []byte("Mine.\n"), 0o600))

	require.NoError(t, runInstallIn(t, root, "--force").err)

	assert.Equal(t, "Mine.\n", readFile(t, root, ".devin/rules/house-style.md"),
		"--force restores what harnaas put there and has no business destroying what it did not")
}
