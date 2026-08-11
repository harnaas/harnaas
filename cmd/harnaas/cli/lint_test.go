package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
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
