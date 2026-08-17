package cli

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// localEntries lists what is under the project's local asset directory.
func localEntries(t *testing.T, root string) []string {
	t.Helper()
	return entries(t, filepath.Join(root, manifest.LocalRoot))
}

// scaffoldedFor runs init for one selection and returns the project root.
func scaffoldedFor(t *testing.T, targets ...harness.ID) string {
	t.Helper()

	root := scratchProject(t)
	run := runInitFor(t, root, targets...)
	require.NoError(t, run.err)
	return root
}

// A harness with a surface for every type earns a directory for every type.
func TestScaffoldingEarnsEveryDirectoryForClaudeCode(t *testing.T) {
	t.Parallel()

	root := scaffoldedFor(t, harness.ClaudeCode)

	assert.Equal(t,
		[]string{"agents", "commands", "instructions", "rules", "skills"},
		localEntries(t, root))
}

// A type no selected harness can receive is not offered a directory. Devin CLI
// has no command surface and its skill format cannot be told to leave a skill
// alone, so a command cannot be delivered there at all — and a directory harnaas
// would refuse to install from is one an author should not be invited to fill.
// See ADR 0005.
func TestScaffoldingWithholdsCommandsFromDevinCLI(t *testing.T) {
	t.Parallel()

	root := scaffoldedFor(t, harness.DevinCLI)

	assert.Equal(t,
		[]string{"agents", "instructions", "rules", "skills"},
		localEntries(t, root))
	assert.NoDirExists(t, filepath.Join(root, manifest.LocalRoot, "commands"))
}

// The selection is a union: one harness that can receive a type is enough,
// because an asset declares its own targets and the project may narrow them.
func TestScaffoldingTakesTheUnionOfTheSelection(t *testing.T) {
	t.Parallel()

	root := scaffoldedFor(t, harness.ClaudeCode, harness.DevinCLI)

	assert.Equal(t,
		[]string{"agents", "commands", "instructions", "rules", "skills"},
		localEntries(t, root))
}

// Each created directory says what belongs in it, and shows the manifest entry
// that declares one. The file is also what makes the directory survive a clone:
// git tracks no empty directory.
func TestEachScaffoldedDirectoryExplainsItself(t *testing.T) {
	t.Parallel()

	root := scaffoldedFor(t, harness.ClaudeCode)

	for _, directory := range localEntries(t, root) {
		explanation := filepath.Join(root, manifest.LocalRoot, directory, scaffoldExplanation)
		content, err := os.ReadFile(explanation)
		require.NoError(t, err, "%s carries no explanation, so a clone would not have the directory", directory)

		text := string(content)
		assert.Contains(t, text, directory, "the explanation names the directory it is in")
		assert.Contains(t, text, manifest.FileName, "and the file an asset is declared in")
		assert.Contains(t, text, path.Join(manifest.LocalRoot, directory),
			"and shows an entry under this directory rather than a generic one")
	}
}

// The example in each explanation is one the manifest actually accepts. An
// example that did not parse would be worse than none: it is the line an author
// copies.
func TestTheExplanationsExampleEntryIsValid(t *testing.T) {
	t.Parallel()

	for _, assetType := range manifest.AssetTypes() {
		directory, known := manifest.DirectoryFor(assetType)
		require.True(t, known)

		entry := explanations[assetType]
		require.NotEmpty(t, entry.Example, "%s has no example id", assetType)

		source := path.Join(manifest.LocalRoot, directory, entry.Example)
		if assetType != manifest.AssetTypeSkill {
			source += ".md"
		}

		ref, violation := manifest.ParseAssetRef(0, source)
		require.Nil(t, violation, "%s: %q is not an entry the manifest accepts", assetType, source)

		inferred, typeViolation := manifest.InferType(0, ref)
		require.Nil(t, typeViolation)
		assert.Equal(t, assetType, inferred,
			"the example in the %s directory infers as %s", directory, inferred)
	}
}

// An asset placed in a scaffolded directory and declared by its path is the type
// that directory is for, with no `type` field written. That round trip is the
// whole reason the directory names are the inference table's own.
func TestAnAssetInAScaffoldedDirectoryInfersItsType(t *testing.T) {
	t.Parallel()

	root := scaffoldedFor(t, harness.ClaudeCode)
	writeFile(t, root, filepath.Join(manifest.LocalRoot, "rules", "house-style.md"), "# House style\n")

	document := `{
	  "version": 1,
	  "harnesses": ["claude-code"],
	  "sources": {},
	  "assets": [".harnaas/rules/house-style.md"]
	}`
	doc, err := manifest.Decode([]byte(document))
	require.NoError(t, err)

	interpretation, err := manifest.Interpret(doc)
	require.NoError(t, err)

	require.Len(t, interpretation.Assets, 1)
	assert.Equal(t, manifest.AssetTypeRule, interpretation.Assets[0].Type)
	assert.Equal(t, "house-style", interpretation.Assets[0].ID)
}

// Scaffolding only ever adds. What is already under the local asset directory is
// the author's — harnaas reads it and never writes to it — so a second run, force
// included, changes nothing that is there.
func TestScaffoldingNeverOverwritesWhatIsThere(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	const mine = "# rules\n\nMy own note about this directory.\n"
	explanation := writeFile(t, root, filepath.Join(manifest.LocalRoot, "rules", scaffoldExplanation), mine)
	asset := writeFile(t, root, filepath.Join(manifest.LocalRoot, "rules", "house-style.md"), "# House style\n")

	run := runInitFor(t, root, harness.ClaudeCode)
	require.NoError(t, run.err)
	run = runInitWith(t, initCase{
		root: root,
		args: []string{"--force", "--harness", string(harness.ClaudeCode)},
	})
	require.NoError(t, run.err)

	after, err := os.ReadFile(explanation)
	require.NoError(t, err)
	assert.Equal(t, mine, string(after), "an explanation that was there is never rewritten")

	assert.FileExists(t, asset, "and nothing under the directory is removed")
	assert.Equal(t, []string{scaffoldExplanation, "house-style.md"},
		entries(t, filepath.Join(root, manifest.LocalRoot, "rules")))
}

// A later run with a narrower selection removes nothing. Those directories may
// hold content harnaas never wrote, and an empty one costs nothing.
func TestANarrowerSelectionRemovesNothing(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	run := runInitFor(t, root, harness.ClaudeCode)
	require.NoError(t, run.err)

	run = runInitWith(t, initCase{
		root: root,
		args: []string{"--force", "--harness", string(harness.DevinCLI)},
	})
	require.NoError(t, run.err)

	assert.DirExists(t, filepath.Join(root, manifest.LocalRoot, "commands"),
		"the directory an earlier selection earned stays, content or not")
}

// A partial layout is completed rather than recreated, and a directory that was
// already there is left exactly as it is — explanation or no explanation.
func TestScaffoldingCompletesAPartialLayout(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, manifest.LocalRoot, "rules"), 0o755))

	run := runInitFor(t, root, harness.ClaudeCode)
	require.NoError(t, run.err)

	assert.Equal(t,
		[]string{"agents", "commands", "instructions", "rules", "skills"},
		localEntries(t, root))
	assert.Empty(t, entries(t, filepath.Join(root, manifest.LocalRoot, "rules")),
		"a directory that was already there is left as it is")
	assert.FileExists(t, filepath.Join(root, manifest.LocalRoot, "skills", scaffoldExplanation),
		"and the ones this run created carry their explanation")
}

// The report names what this run created and never claims what was already
// there: the author may have put something in it, and a run that says it made it
// is a run they will not look at again.
func TestTheReportClaimsOnlyWhatItCreated(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, manifest.LocalRoot, "rules"), 0o755))

	run := runInitFor(t, root, harness.ClaudeCode)
	require.NoError(t, run.err)

	assert.Contains(t, run.stdout, path.Join(manifest.LocalRoot, "skills"))
	assert.NotContains(t, run.stdout, path.Join(manifest.LocalRoot, "rules"),
		"the directory that was already there is not reported as created")
}

// A selection naming a harness with no per-harness mapping earns the two types
// that reach every harness through shared locations, and no others: a rule, a
// command and a persona reach a harness only through a mapping.
//
// It is asked of the derivation directly because no such harness can be named in
// a manifest today — every roster entry has an adapter — and "a harness with no
// adapter" is a supported state that has to keep working before it is reachable.
func TestASelectionWithNoMappingEarnsOnlyTheSharedTypes(t *testing.T) {
	t.Parallel()

	directories := scaffoldDirectories([]harness.ID{harness.ClaudeCode}, &adapter.Registry{})

	paths := make([]string, 0, len(directories))
	for _, directory := range directories {
		paths = append(paths, directory.Path)
	}
	assert.Equal(t, []string{
		path.Join(manifest.LocalRoot, "skills"),
		path.Join(manifest.LocalRoot, "instructions"),
	}, paths)
}

// Nothing scaffolded is managed. `.harnaas` is content harnaas reads and never a
// destination it writes, so no directory and no explanation appears in the
// lockfile, and none is covered by a managed ignore-file entry — the scaffolding
// is source content a team commits. See ADR 0006 and ADR 0001.
//
// The explanations are also proved to be nobody's dependency: one is deleted and
// one is rewritten before the install, and the run is expected to be identical to
// one where neither happened.
func TestAnInstallRecordsNothingAboutTheScaffolding(t *testing.T) {
	t.Parallel()

	root := scaffoldedFor(t, harness.ClaudeCode)
	writeFile(t, root, filepath.Join(manifest.LocalRoot, "rules", "house-style.md"),
		"---\nname: house-style\n---\nTwo spaces.\n")
	writeManifest(t, root, `{
  "version": 1,
  "harnesses": ["claude-code"],
  "sources": {},
  "assets": [".harnaas/rules/house-style.md"]
}`)

	require.NoError(t, os.Remove(filepath.Join(root, manifest.LocalRoot, "skills", scaffoldExplanation)))
	writeFile(t, root, filepath.Join(manifest.LocalRoot, "rules", scaffoldExplanation), "mine now\n")

	require.NoError(t, runInstallIn(t, root).err)
	assert.True(t, exists(t, root, ruleDestination), "the asset installed as it would have anyway")

	lock := readFile(t, root, LockFileName)
	assert.NotContains(t, lock, manifest.LocalRoot+"/skills",
		"a scaffolded directory is not a destination, so nothing records ownership of it")
	assert.NotContains(t, lock, scaffoldExplanation,
		"and the explanation harnaas wrote once is not managed content either")

	ignore := readFile(t, root, ".gitignore")
	assert.NotContains(t, ignore, manifest.LocalRoot,
		"the scaffolding is source a team commits; ignoring it would hide the assets it holds")

	// And lint has nothing to say about directories nobody declared anything
	// from: it reports on the assets a manifest declares.
	report, err := runLintIn(t, root)
	require.NoError(t, err)
	assert.NotContains(t, report, manifest.LocalRoot+"/skills")
	assert.NotContains(t, report, scaffoldExplanation)
}

// A scaffolding failure after the manifest was written says so: the project is
// initialized, the path that could not be created is named, and a re-run finishes
// the job. A message that only named the failure would leave its reader unsure
// whether to start over.
func TestAScaffoldingFailureKeepsAndNamesTheManifest(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	// A regular file where the directory goes: `.harnaas` is "already there" as
	// far as creating it goes, and nothing can be created beneath it.
	writeFile(t, root, manifest.LocalRoot, "not a directory\n")

	run := runInitFor(t, root, harness.ClaudeCode)

	var failed *scaffoldError
	require.ErrorAs(t, run.err, &failed)
	assert.Contains(t, failed.Path, manifest.LocalRoot, "the path that could not be created is named")
	assert.Contains(t, run.err.Error(), manifest.FileName)
	assert.Contains(t, run.err.Error(), "The project is initialized")

	assert.FileExists(t, run.manifestPath(), "the manifest that was written stays")
	doc, _ := loadManifest(t, run.manifestPath())
	assert.Equal(t, []string{string(harness.ClaudeCode)}, doc.Harnesses)
}

// The scaffolding is created through a handle anchored at the project root, so a
// local asset directory that leads somewhere else on this machine cannot be
// written through. The paths are constants; the filesystem is what is untrusted.
func TestScaffoldingIsNotWrittenThroughALinkOutOfTheProject(t *testing.T) {
	t.Parallel()

	root := scratchProject(t)
	outside := scratchProject(t)

	if err := os.Symlink(outside, filepath.Join(root, manifest.LocalRoot)); err != nil {
		t.Skipf("this platform does not allow the test to create a symbolic link: %v", err)
	}

	run := runInitFor(t, root, harness.ClaudeCode)

	// Whichever answer the handle gives, nothing lands outside the project.
	if run.err == nil {
		assert.Empty(t, entries(t, outside),
			"the scaffolding must never be written through a link out of the project")
	}
}
