package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// installedAt is a fixed instant, so a test comparing two encodings is
// comparing the ordering rules rather than the clock.
var installedAt = time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

// twoLockAssets is the fixture the ordering assertions run against, declared
// out of order at every level so a test that read them as given would fail.
func twoLockAssets() []lockAsset {
	return []lockAsset{
		{
			ID:           "review",
			Type:         manifest.AssetTypeSkill,
			Source:       "github:acme/assets@v1.2.0",
			RequestedRef: "v1.2.0", ResolvedCommit: "abc123",
			SourceDigest: source.DigestContent([]byte("review")),
			InstalledAt:  installedAt,
			Installations: []lockInstallation{
				{
					Harness: harness.ClaudeCode, Scope: manifest.ScopeProject,
					Destination:     ".agents/skills/review",
					InstalledDigest: source.DigestContent([]byte("review")),
					Files: []lockFile{
						{Path: "reference.md", Digest: source.DigestContent([]byte("b"))},
						{Path: "SKILL.md", Digest: source.DigestContent([]byte("a"))},
					},
				},
			},
		},
		{
			ID:           "house-style",
			Type:         manifest.AssetTypeRule,
			Source:       ".harnaas/rules/house-style.md",
			SourceDigest: source.DigestContent([]byte("house")),
			InstalledAt:  installedAt,
			Installations: []lockInstallation{
				{
					Harness: harness.ClaudeCode, Scope: manifest.ScopeProject,
					Destination:     ".claude/rules/house-style.md",
					InstalledDigest: source.DigestContent([]byte("house")),
					Files:           []lockFile{{Path: "house-style.md", Digest: source.DigestContent([]byte("house"))}},
				},
			},
		},
	}
}

func TestLockDocumentOrdersAssetsFilesAndInstallations(t *testing.T) {
	t.Parallel()

	document := newLockDocument(twoLockAssets())

	require.Len(t, document.Assets, 2)
	assert.Equal(t, "house-style", document.Assets[0].ID, "assets are ordered by id, not by the order install processed them")
	assert.Equal(t, []string{"SKILL.md", "reference.md"},
		[]string{document.Assets[1].Installations[0].Files[0].Path, document.Assets[1].Installations[0].Files[1].Path},
		"files are ordered by path, which is also the order lint reports in")
}

func TestLockFileIsByteIdenticalForReorderedInput(t *testing.T) {
	t.Parallel()

	forward := twoLockAssets()
	reversed := []lockAsset{forward[1], forward[0]}

	forwardRoot, reversedRoot := t.TempDir(), t.TempDir()
	require.NoError(t, saveLock(forwardRoot, newLockDocument(forward)))
	require.NoError(t, saveLock(reversedRoot, newLockDocument(reversed)))

	forwardBytes, err := os.ReadFile(lockPath(forwardRoot))
	require.NoError(t, err)
	reversedBytes, err := os.ReadFile(lockPath(reversedRoot))
	require.NoError(t, err)

	assert.Equal(t, string(forwardBytes), string(reversedBytes),
		"identical state must produce a byte-identical file, or every install is a diff")
}

func TestLockFileRoundTrips(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, saveLock(root, newLockDocument(twoLockAssets())))

	loaded, err := loadLock(root)
	require.NoError(t, err)
	assert.Equal(t, SupportedLockVersion, loaded.Version)
	require.Len(t, loaded.Assets, 2)
	assert.Equal(t, "v1.2.0", loaded.Assets[1].RequestedRef,
		"the requested ref survives resolution, because lint asks about it separately from the commit")
	assert.Equal(t, "abc123", loaded.Assets[1].ResolvedCommit)
}

func TestAbsentLockFileMeansNothingIsManaged(t *testing.T) {
	t.Parallel()

	document, err := loadLock(t.TempDir())

	require.NoError(t, err, "an absent lockfile is the first-install case, not a failure")
	assert.Empty(t, document.Assets,
		"nothing recorded means every destination on disk is unmanaged, which is the protective reading")
}

func TestLockDecodingIgnoresUnknownFields(t *testing.T) {
	t.Parallel()

	document, err := decodeLock([]byte(`{"version":1,"assets":[],"writtenByANewerHarnaas":true}`), "x")

	require.NoError(t, err, "a newer harnaas must not brick the lockfile for a teammate on an older one")
	assert.Equal(t, SupportedLockVersion, document.Version)
}

func TestLockDecodingRefusesAVersionItCannotInterpret(t *testing.T) {
	t.Parallel()

	_, err := decodeLock([]byte(`{"version":99}`), "harnaas.lock.json")

	var versionErr *lockVersionError
	require.ErrorAs(t, err, &versionErr)
	assert.Contains(t, err.Error(), "Upgrade harnaas")
	assert.Contains(t, err.Error(), "Do not delete the lockfile",
		"deleting it makes every installed file unmanaged, which is the trap the message exists to close")
}

func TestLockDecodingReportsAFileThatWillNotParse(t *testing.T) {
	t.Parallel()

	_, err := decodeLock([]byte(`{"version":`), "harnaas.lock.json")

	var decodeErr *lockDecodeError
	require.ErrorAs(t, err, &decodeErr)
	assert.Contains(t, err.Error(), "Nothing was installed, removed or overwritten")
}

func TestLoadLockReportsAnUnreadableFileWithoutTouchingAnything(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(lockPath(root), []byte("not json"), 0o600))

	_, err := loadLock(root)

	require.Error(t, err)
	content, readErr := os.ReadFile(filepath.Join(root, LockFileName))
	require.NoError(t, readErr)
	assert.Equal(t, "not json", string(content), "a parse failure must not rewrite the file it could not read")
}

func TestClaimantsGroupsADestinationSeveralHarnessesRead(t *testing.T) {
	t.Parallel()

	shared := lockInstallation{
		Scope: manifest.ScopeProject, Destination: ".agents/skills/review",
		InstalledDigest: source.DigestContent([]byte("x")),
	}
	first, second := shared, shared
	first.Harness = harness.ClaudeCode
	second.Harness = harness.ID("another-harness")

	document := newLockDocument([]lockAsset{{
		ID: "review", Type: manifest.AssetTypeSkill,
		Installations: []lockInstallation{first, second},
	}})

	claims := document.claimants()
	key := destinationKey{Scope: manifest.ScopeProject, Destination: ".agents/skills/review"}
	assert.Equal(t, []harness.ID{harness.ID("another-harness"), harness.ClaudeCode}, claims[key],
		"the harness field is attribution: one shared file is claimed by every harness that reads it")
}

func TestRecordedDestinationRefusesAnAbsolutePath(t *testing.T) {
	t.Parallel()

	_, err := recordedDestination(filepath.Join(t.TempDir(), "skills", "review"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "must mean the same thing on every machine")
}

func TestRecordedDestinationRefusesAPathLeavingTheScopeRoot(t *testing.T) {
	t.Parallel()

	_, err := recordedDestination("../elsewhere")

	require.Error(t, err)
}

func TestRecordedDestinationNormalizesSeparators(t *testing.T) {
	t.Parallel()

	recorded, err := recordedDestination(filepath.FromSlash(".claude/rules/house-style.md"))

	require.NoError(t, err)
	assert.Equal(t, ".claude/rules/house-style.md", recorded,
		"a lockfile written on Windows and one written on Linux must record the same install")
}

func TestRecordedSourceLeavesAnOrdinarySourceAlone(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "github:acme/assets@v1.2.0", recordedSource("github:acme/assets@v1.2.0"))
}

func TestRecordedSourceRedactsACredential(t *testing.T) {
	t.Parallel()

	recorded := recordedSource("https://user:secret@example.com/assets?token=grant")

	assert.NotContains(t, recorded, "secret")
	assert.NotContains(t, recorded, "grant", "a committed file must not carry a query string that grants access")
}
