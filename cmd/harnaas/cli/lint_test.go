package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source/github"
)

func TestNothingInstalledCollapsesToOneFinding(t *testing.T) {
	t.Parallel()

	declared := make([]manifest.Asset, 12)
	for i := range declared {
		declared[i] = manifest.Asset{ID: "asset", Type: manifest.AssetTypeSkill}
	}

	found, collapsed := checkNothingInstalled(declared, &lockDocument{})

	require.True(t, collapsed)
	assert.Contains(t, found.Problem, "12",
		"a fresh clone declaring twelve assets gets one finding naming the count, not twelve findings")
	assert.Contains(t, found.Remedy, "harnaas install")
}

func TestNothingInstalledSaysNothingWhenSomethingIs(t *testing.T) {
	t.Parallel()

	recorded := &lockDocument{Assets: []lockAsset{{ID: "review", Type: manifest.AssetTypeSkill}}}
	_, collapsed := checkNothingInstalled([]manifest.Asset{{ID: "review"}}, recorded)

	assert.False(t, collapsed)
}

func TestATrackedBranchIsNotReproducible(t *testing.T) {
	t.Parallel()

	declared := []manifest.Asset{{ID: "review", Ref: manifest.AssetRef{SourceKey: "acme"}}}
	sources := map[string]manifest.Source{
		"acme": {Key: "acme", Kind: manifest.SourceKindGitHub, Repository: "acme/assets", Ref: "main"},
	}

	findings := checkNotReproducible(declared, sources)

	require.Len(t, findings, 1)
	assert.Equal(t, severityError, findings[0].Severity,
		"a branch is an error whether or not it moved, or CI is permanently red with no achievable fix")
	assert.Contains(t, findings[0].Remedy, "Pin it")
}

func TestAPinnedRefIsNotFlagged(t *testing.T) {
	t.Parallel()

	declared := []manifest.Asset{{ID: "review", Ref: manifest.AssetRef{SourceKey: "acme"}}}

	for _, ref := range []string{"v1.2.0", "1.2.0", "0f52986e2ec5d9a761b33ce7be2fbf039aeec3fe"} {
		sources := map[string]manifest.Source{
			"acme": {Key: "acme", Kind: manifest.SourceKindGitHub, Repository: "acme/assets", Ref: ref},
		}
		assert.Empty(t, checkNotReproducible(declared, sources), "%q is pinned", ref)
	}
}

func TestAnAbbreviatedCommitIsNotAFullOne(t *testing.T) {
	t.Parallel()

	assert.True(t, looksLikeCommit("0f52986e2ec5d9a761b33ce7be2fbf039aeec3fe"))
	assert.False(t, looksLikeCommit("0f52986"),
		"an abbreviation names whichever object it is unique against today")
	assert.False(t, looksLikeCommit("0f52986e2ec5d9a761b33ce7be2fbf039aeec3fz"))
}

func TestVersionsOrderByTheirNumericComponents(t *testing.T) {
	t.Parallel()

	assert.Negative(t, compareVersions("v1.2.0", "v1.3.0"))
	assert.Negative(t, compareVersions("v1.9.0", "v1.10.0"), "components are numbers, not text")
	assert.Positive(t, compareVersions("v2.0.0", "v1.99.99"))
	assert.Zero(t, compareVersions("v1.2.0", "1.2.0"), "the leading v is not part of the version")
	assert.Negative(t, compareVersions("v1.2", "v1.2.1"), "a missing component is zero")
}

func TestAPreReleaseSortsBelowTheReleaseItQualifies(t *testing.T) {
	t.Parallel()

	assert.Negative(t, compareVersions("v1.2.0-rc.1", "v1.2.0"))
	assert.Positive(t, compareVersions("v1.2.0", "v1.2.0-rc.1"))
}

func TestOnlyTheHighestNewerStableTagIsOffered(t *testing.T) {
	t.Parallel()

	best := highestNewerStable("v1.2.0", []string{"v1.3.0", "v1.4.0", "v1.2.1", "v1.1.0"})

	assert.Equal(t, "v1.4.0", best,
		"the reader upgrades to the newest, and the intermediate tags are not decisions they have to make")
}

func TestAPreReleaseIsNeverOfferedOverAStableInstall(t *testing.T) {
	t.Parallel()

	assert.Empty(t, highestNewerStable("v1.2.0", []string{"v1.3.0-rc.1", "v2.0.0-beta"}),
		"`brew install` must never resolve to a release candidate, and neither must a lint remedy")
}

func TestNonVersionTagsAreIgnoredRatherThanCompared(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "v1.3.0", highestNewerStable("v1.2.0", []string{"latest", "nightly", "v1.3.0"}))
	assert.Empty(t, highestNewerStable("v1.2.0", []string{"latest", "nightly"}))
}

func TestNoNewerTagOffersNothing(t *testing.T) {
	t.Parallel()

	assert.Empty(t, highestNewerStable("v2.0.0", []string{"v1.9.0", "v2.0.0"}))
}

func TestLintReportOrdersFindingsByAssetThenPath(t *testing.T) {
	t.Parallel()

	report := &lintReport{Findings: []finding{
		{Asset: "review", Path: "b.md", Problem: "x"},
		{Asset: "house-style", Problem: "y"},
		{Asset: "review", Path: "a.md", Problem: "z"},
	}}
	report.sort()

	assert.Equal(t, []string{"house-style", "review", "review"},
		[]string{report.Findings[0].Asset, report.Findings[1].Asset, report.Findings[2].Asset})
	assert.Equal(t, "a.md", report.Findings[1].Path,
		"two runs over identical state must be identical line for line")
}

func TestStrictPromotesWarningsForTheExitStatusOnly(t *testing.T) {
	t.Parallel()

	report := &lintReport{Findings: []finding{{Severity: severityWarning, Problem: "x", Remedy: "y"}}}
	errors, warnings := report.counts()

	assert.Equal(t, 0, errors)
	assert.Equal(t, 1, warnings,
		"strict changes the status derived from these counts, never the severity a finding was reported with")
}

func TestLintFindingsErrorIsAlreadyPrinted(t *testing.T) {
	t.Parallel()

	err := &LintFindingsError{Errors: 3}

	assert.True(t, err.AlreadyPrinted(), "the report is the message; the entrypoint must not restate it")
}

// upstreamFixture builds an interpretation and a lockfile for one github asset
// installed from ref at commit.
func upstreamFixture(ref, commit string) (*manifest.Interpretation, *lockDocument) {
	asset := manifest.Asset{ID: "review", Type: manifest.AssetTypeSkill, Ref: manifest.AssetRef{SourceKey: "acme"}}
	return &manifest.Interpretation{
			Assets: []manifest.Asset{asset},
			Sources: map[string]manifest.Source{
				"acme": {Key: "acme", Kind: manifest.SourceKindGitHub, Repository: "acme/assets", Ref: ref},
			},
		}, &lockDocument{Assets: []lockAsset{{
			ID: "review", Type: manifest.AssetTypeSkill, RequestedRef: ref, ResolvedCommit: commit,
		}}}
}

// resolvesTo is a refResolver answering one commit for every request.
func resolvesTo(commit string) refResolver {
	return func(context.Context, source.Request) (github.RefResolution, error) {
		return github.RefResolution{Commit: commit, Mutable: true}, nil
	}
}

func TestAMovedRefIsReportedWithBothCommits(t *testing.T) {
	t.Parallel()

	interpretation, recorded := upstreamFixture("main", "aaaaaaaaaaaa1111")
	findings, unchecked := checkUpstream(t.Context(), interpretation, recorded, resolvesTo("bbbbbbbbbbbb2222"), nil)

	require.Len(t, findings, 1)
	assert.Equal(t, severityError, findings[0].Severity)
	assert.Contains(t, findings[0].Problem, "aaaaaaaaaaaa", "the commit that was installed")
	assert.Contains(t, findings[0].Problem, "bbbbbbbbbbbb", "and the one the ref points at now")
	assert.Empty(t, unchecked)
}

func TestAnUnmovedRefIsNotReported(t *testing.T) {
	t.Parallel()

	interpretation, recorded := upstreamFixture("main", "aaaaaaaaaaaa1111")
	findings, _ := checkUpstream(t.Context(), interpretation, recorded, resolvesTo("aaaaaaaaaaaa1111"), nil)

	assert.Empty(t, findings)
}

func TestACommitPinnedAssetIsNeverLookedUp(t *testing.T) {
	t.Parallel()

	const pinned = "0f52986e2ec5d9a761b33ce7be2fbf039aeec3fe"
	interpretation, recorded := upstreamFixture(pinned, pinned)

	asked := false
	findings, _ := checkUpstream(t.Context(), interpretation, recorded,
		func(context.Context, source.Request) (github.RefResolution, error) {
			asked = true
			return github.RefResolution{}, nil
		}, nil)

	assert.Empty(t, findings)
	assert.False(t, asked,
		"the user pinned it deliberately, so there is no newer commit to find and asking would learn nothing")
}

func TestAnUnreachableHostIsReportedOnceAndDoesNotFailTheRun(t *testing.T) {
	t.Parallel()

	interpretation, recorded := upstreamFixture("main", "aaaaaaaaaaaa1111")
	// Two assets against the same repository, so the summarising rule has
	// something to summarise.
	second := interpretation.Assets[0]
	second.ID = "tone"
	interpretation.Assets = append(interpretation.Assets, second)
	recorded.Assets = append(recorded.Assets, lockAsset{
		ID: "tone", Type: manifest.AssetTypeSkill, RequestedRef: "main", ResolvedCommit: "aaaaaaaaaaaa1111",
	})

	findings, unchecked := checkUpstream(t.Context(), interpretation, recorded,
		func(context.Context, source.Request) (github.RefResolution, error) {
			return github.RefResolution{}, assert.AnError
		}, nil)

	require.Len(t, findings, 1, "several assets behind one outage are one thing to fix, reported once")
	assert.Equal(t, severityWarning, findings[0].Severity,
		"a host that cannot be reached must not be counted as an error, or an outage fails the build")
	assert.Len(t, unchecked, 2, "and the summary still says how many went unchecked")
}

// publishes is a tagLister answering one fixed set of tags.
func publishes(tags ...string) tagLister {
	return func(context.Context, string) ([]string, error) { return tags, nil }
}

func TestASupersededTagIsReportedWithAVerbatimEdit(t *testing.T) {
	t.Parallel()

	interpretation, recorded := upstreamFixture("v1.2.0", "aaaaaaaaaaaa1111")
	findings, _ := checkUpstream(t.Context(), interpretation, recorded,
		resolvesTo("aaaaaaaaaaaa1111"), publishes("v1.2.0", "v1.3.0", "v1.4.0"))

	require.Len(t, findings, 1, "the ref has not moved; what changed is that a newer tag exists")
	assert.Equal(t, severityError, findings[0].Severity, "an available update is an error, never a warning")
	assert.Contains(t, findings[0].Remedy, `"github:acme/assets@v1.2.0"`, "the exact current source line")
	assert.Contains(t, findings[0].Remedy, `"github:acme/assets@v1.4.0"`, "and the exact replacement")
	assert.Contains(t, findings[0].Remedy, "harnaas install", "followed by the command to run")
}

func TestACurrentTagIsNotReported(t *testing.T) {
	t.Parallel()

	interpretation, recorded := upstreamFixture("v1.4.0", "aaaaaaaaaaaa1111")
	findings, _ := checkUpstream(t.Context(), interpretation, recorded,
		resolvesTo("aaaaaaaaaaaa1111"), publishes("v1.2.0", "v1.3.0", "v1.4.0"))

	assert.Empty(t, findings, "pinned and current is the one state that passes")
}

func TestAPreReleaseUpstreamDoesNotSupersedeAStableInstall(t *testing.T) {
	t.Parallel()

	interpretation, recorded := upstreamFixture("v1.4.0", "aaaaaaaaaaaa1111")
	findings, _ := checkUpstream(t.Context(), interpretation, recorded,
		resolvesTo("aaaaaaaaaaaa1111"), publishes("v1.4.0", "v1.5.0-rc.1"))

	assert.Empty(t, findings)
}

func TestATagListingFailureIsNotASecondFindingAboutTheSameOutage(t *testing.T) {
	t.Parallel()

	interpretation, recorded := upstreamFixture("v1.2.0", "aaaaaaaaaaaa1111")
	findings, _ := checkUpstream(t.Context(), interpretation, recorded, resolvesTo("aaaaaaaaaaaa1111"),
		func(context.Context, string) ([]string, error) { return nil, assert.AnError })

	assert.Empty(t, findings,
		"the resolution beside it already met the same remote, and one outage is one finding")
}

func TestLintReportsAHandWrittenFileAtADeclaredDestination(t *testing.T) {
	t.Parallel()

	root := installedProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude", "rules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, filepath.FromSlash(ruleDestination)), []byte("mine\n"), 0o600))

	document, err := manifest.Decode([]byte(readFile(t, root, "harnaas.json")))
	require.NoError(t, err)
	interpretation, err := manifest.Interpret(document)
	require.NoError(t, err)

	findings := checkUnmanagedConflict(root, interpretation, &lockDocument{})

	require.NotEmpty(t, findings)
	assert.Contains(t, findings[0].Remedy, "--force included",
		"a reader assuming --force will deal with it later has to be told it will not")
}

func TestLintReportsAnInstalledPathTheIgnoreBlockLost(t *testing.T) {
	t.Parallel()

	root := installedProject(t)
	require.NoError(t, runInstallIn(t, root).err)

	// The team regenerated .gitignore by hand and lost an entry, which is the
	// case that quietly commits installed content.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"),
		[]byte("# harnaas:begin installed\n/.agents/skills/review\n# harnaas:end installed\n"), 0o600))

	recorded, err := loadLock(root)
	require.NoError(t, err)
	findings := checkIgnoreBlock(root, recorded)

	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Problem, ruleDestination)
	assert.Contains(t, findings[0].Remedy, "harnaas install")
}

func TestLintSaysNothingAboutAnIgnoreBlockThatIsComplete(t *testing.T) {
	t.Parallel()

	root := installedProject(t)
	require.NoError(t, runInstallIn(t, root).err)

	recorded, err := loadLock(root)
	require.NoError(t, err)

	assert.Empty(t, checkIgnoreBlock(root, recorded),
		"install regenerates the block, so the state it leaves must lint clean")
}

func TestAVanishedRefIsNotReportedAsAnAvailableUpdate(t *testing.T) {
	t.Parallel()

	interpretation, recorded := upstreamFixture("v1.2.0", "aaaaaaaaaaaa1111")
	findings, unchecked := checkUpstream(t.Context(), interpretation, recorded,
		func(context.Context, source.Request) (github.RefResolution, error) {
			return github.RefResolution{}, &github.UnknownRefError{}
		}, nil)

	require.Len(t, findings, 1)
	assert.Equal(t, severityError, findings[0].Severity)
	assert.Contains(t, findings[0].Problem, "no longer exists",
		"there is no newer content on offer, so this must not read as an update")
	assert.Contains(t, findings[0].Remedy, manifest.FileName,
		"and unlike an outage, only a manifest edit fixes it")
	assert.Empty(t, unchecked, "the asset was checked; the answer was that its ref is gone")
}

// TestNoFlagPathDowngradesAnUpdateFinding is requirement 5.9 asserted in code:
// ADR 0004 makes an available update an error, and the only lever any flag has
// over severity is strict promoting warnings — never the reverse.
func TestNoFlagPathDowngradesAnUpdateFinding(t *testing.T) {
	t.Parallel()

	interpretation, recorded := upstreamFixture("v1.2.0", "aaaaaaaaaaaa1111")

	for _, opts := range []*lintOptions{{}, {strict: true}, {refresh: true}, {asJSON: true}} {
		findings, _ := checkUpstream(t.Context(), interpretation, recorded,
			resolvesTo("aaaaaaaaaaaa1111"), publishes("v1.2.0", "v1.4.0"))

		require.Len(t, findings, 1)
		assert.Equal(t, severityError, findings[0].Severity,
			"an available update is an error under every flag combination, including %+v", opts)
	}
}

func TestBridgeLineFindings(t *testing.T) {
	t.Parallel()

	withInstruction := &lockDocument{Assets: []lockAsset{{
		ID: "tone", Type: manifest.AssetTypeInstruction,
		Installations: []lockInstallation{{Destination: memoryFileName}},
	}}}

	tests := []struct {
		name    string
		content string
		lock    *lockDocument
		want    string
	}{
		{name: "missing line", content: "# House rules\n", lock: withInstruction, want: "does not import"},
		{name: "missing file", content: "", lock: withInstruction, want: "does not import"},
		{name: "duplicated line", content: bridgeLine + "\n" + bridgeLine + "\n", lock: withInstruction, want: "2 times"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			if tc.content != "" {
				require.NoError(t, os.WriteFile(filepath.Join(root, bridgeFileName), []byte(tc.content), 0o600))
			}

			findings := checkBridgeLine(root, tc.lock)

			require.Len(t, findings, 1)
			assert.Contains(t, findings[0].Problem, tc.want)
		})
	}
}

func TestNoBridgeLineFindingWhenNoInstructionIsInstalled(t *testing.T) {
	t.Parallel()

	// No CLAUDE.md at all, and nothing to say about it: the bridge exists only
	// to make instruction content reachable, so with none installed there is
	// nothing for a missing line to be missing from.
	assert.Empty(t, checkBridgeLine(t.TempDir(), &lockDocument{}))
}

func TestExitStatusFollowsSeverity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		findings []finding
		strict   bool
		wantErr  bool
	}{
		{name: "clean run"},
		{name: "warnings alone pass", findings: []finding{{Severity: severityWarning}}},
		{name: "any error fails", findings: []finding{{Severity: severityError}}, wantErr: true},
		{name: "strict promotes warnings", findings: []finding{{Severity: severityWarning}}, strict: true, wantErr: true},
		{
			name:     "strict leaves a clean run clean",
			strict:   true,
			findings: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cmd := newLintCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			err := finishLint(cmd, &lintReport{Findings: tc.findings}, &lintOptions{strict: tc.strict})

			if !tc.wantErr {
				assert.NoError(t, err, "exit 0 is every state with no error-severity finding")
				return
			}
			var findings *LintFindingsError
			require.ErrorAs(t, err, &findings, "exit 2 is reserved for lint completing and finding something")
		})
	}
}
