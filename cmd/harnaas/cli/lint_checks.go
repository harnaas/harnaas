package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source/github"
)

// checkNothingInstalled collapses "nothing has been installed yet" into one
// finding.
//
// A fresh clone declaring twelve assets would otherwise report twelve identical
// "not installed" findings, which buries the one thing the reader needs to know
// and the one command that fixes all of it. The absent lockfile is deliberately
// not a finding of its own: it is not a second problem, it is the same one.
func checkNothingInstalled(declared []manifest.Asset, recorded *lockDocument) (finding, bool) {
	if len(declared) == 0 || len(recorded.Assets) > 0 {
		return finding{}, false
	}
	return finding{
		Severity: severityError,
		Problem: fmt.Sprintf("%d declared %s not been installed in this project yet",
			len(declared), plural(len(declared), "asset has", "assets have")),
		Remedy: "Run `harnaas install`.",
	}, true
}

// checkDeclaredButNotInstalled reports an asset the manifest declares and the
// lockfile does not record.
func checkDeclaredButNotInstalled(declared []manifest.Asset, recorded *lockDocument) []finding {
	known := previousAssets(recorded)

	var findings []finding
	for _, asset := range declared {
		if _, found := known[assetKey{ID: asset.ID, Type: asset.Type}]; found {
			continue
		}
		findings = append(findings, finding{
			Asset:    asset.ID,
			Severity: severityError,
			Problem:  fmt.Sprintf("%q is declared in %s and has never been installed", asset.ID, manifest.FileName),
			Remedy:   "Run `harnaas install`.",
		})
	}
	return findings
}

// checkRecordedButUndeclared reports a lockfile entry the manifest no longer
// declares.
//
// The remedy is the install command rather than an edit, because install is
// what converges: removing the entry by hand would leave the installed files
// behind and unmanaged.
func checkRecordedButUndeclared(declared []manifest.Asset, recorded *lockDocument) []finding {
	wanted := make(map[assetKey]bool, len(declared))
	for _, asset := range declared {
		wanted[assetKey{ID: asset.ID, Type: asset.Type}] = true
	}

	var findings []finding
	for _, asset := range recorded.Assets {
		if wanted[assetKey{ID: asset.ID, Type: asset.Type}] {
			continue
		}
		findings = append(findings, finding{
			Asset:    asset.ID,
			Severity: severityError,
			Problem: fmt.Sprintf("%q is recorded as installed and %s no longer declares it",
				asset.ID, manifest.FileName),
			Remedy: "Run `harnaas install`, which removes what is no longer declared.",
		})
	}
	return findings
}

// checkRefDisagreement reports a lockfile whose recorded ref differs from what
// the manifest now asks for.
//
// This is the check frozen mode exists for: it needs no installed files and no
// network, and it catches the common case of somebody editing a ref and not
// reinstalling before opening a pull request.
func checkRefDisagreement(declared []manifest.Asset, sources map[string]manifest.Source, recorded *lockDocument) []finding {
	known := previousAssets(recorded)

	var findings []finding
	for _, asset := range declared {
		was, found := known[assetKey{ID: asset.ID, Type: asset.Type}]
		if !found {
			continue
		}
		wanted := sources[asset.Ref.SourceKey].Ref
		if wanted == "" || wanted == was.RequestedRef {
			continue
		}
		findings = append(findings, finding{
			Asset:    asset.ID,
			Severity: severityError,
			Problem: fmt.Sprintf("%s asks for %q and the lockfile records %q as installed",
				manifest.FileName, wanted, was.RequestedRef),
			Remedy: "Run `harnaas install` to install the ref the manifest asks for.",
		})
	}
	return findings
}

// checkNotReproducible reports an asset tracking a ref that can move.
//
// It is an error whether or not the ref has moved, and that is the point. A
// branch moves whenever upstream commits, so reporting a moved branch as
// "outdated" would leave CI permanently red with no achievable fix; reporting
// it as "not pinned" gives a fix the team can actually apply once. See ADR
// 0004.
func checkNotReproducible(declared []manifest.Asset, sources map[string]manifest.Source) []finding {
	var findings []finding
	for _, asset := range declared {
		source, found := sources[asset.Ref.SourceKey]
		if !found || source.Kind != manifest.SourceKindGitHub {
			continue
		}
		if looksLikeCommit(source.Ref) || looksLikeVersionTag(source.Ref) {
			continue
		}
		findings = append(findings, finding{
			Asset:    asset.ID,
			Severity: severityError,
			Problem: fmt.Sprintf("%q tracks %q, which can point somewhere else tomorrow, so this install is not reproducible",
				asset.ID, source.Ref),
			Remedy: fmt.Sprintf(
				"Pin it in %s: change the %q source to a version tag or a full commit identifier, then run `harnaas install`.",
				manifest.FileName, asset.Ref.SourceKey),
		})
	}
	return findings
}

// looksLikeCommit reports whether a ref is a full commit identifier, which is
// the one ref that cannot move.
//
// Only a full one counts: an abbreviation names whichever object it is unique
// against today, so one that grew a second match upstream would silently become
// a different install.
func looksLikeCommit(ref string) bool {
	if len(ref) != 40 {
		return false
	}
	for _, char := range ref {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return false
		}
	}
	return true
}

// looksLikeVersionTag reports whether a ref is a version tag.
//
// The test is deliberately loose — a leading `v` and a digit — because the
// question here is only "did somebody pin this deliberately", and a project
// whose tags are `release-3` is not tracking a branch.
func looksLikeVersionTag(ref string) bool {
	trimmed := strings.TrimPrefix(ref, "v")
	return trimmed != "" && trimmed[0] >= '0' && trimmed[0] <= '9'
}

// checkInstalledIntegrity re-computes every installed file's digest and reports
// what no longer matches.
//
// An installation whose destination is gone entirely collapses to one finding
// rather than one per recorded file, for the reason "nothing installed"
// collapses: the reader has one problem and one command, and a finding per file
// would hide that.
func checkInstalledIntegrity(root string, recorded *lockDocument) []finding {
	var findings []finding

	for _, asset := range recorded.Assets {
		for _, installation := range asset.Installations {
			if installation.Destination == memoryFileName {
				// The memory file is checked as a managed block, not as a
				// destination: several assets share one file, so a per-asset
				// digest over it would report every instruction whenever any
				// one of them changed.
				continue
			}

			scopeRoot, err := scopeRootFor(root, installation.Harness, manifest.Asset{
				ID: asset.ID, Type: asset.Type, Scope: installation.Scope,
			})
			if err != nil {
				continue
			}

			state, err := readInstalled(scopeRoot, installation.Destination)
			if err != nil {
				continue
			}

			reported := projectRelative(root, scopeRoot, installation.Destination)
			if !state.Present {
				findings = append(findings, finding{
					Asset: asset.ID, Path: reported, Severity: severityError,
					Problem: reported + " is recorded as installed and is not there",
					Remedy:  "Run `harnaas install`.",
				})
				continue
			}
			if state.Digest == installation.InstalledDigest {
				continue
			}
			findings = append(findings, compareFiles(asset.ID, reported, installation, state)...)
		}
	}
	return findings
}

// compareFiles names which file of an installation changed, went missing, or
// was added.
//
// Naming the file is the whole reason per-file digests are recorded. "Something
// under this skill changed" sends the reader to diff a directory; "this file
// changed" does not.
func compareFiles(assetID, destination string, installation lockInstallation, state installedState) []finding {
	recorded := make(map[string]string, len(installation.Files))
	for _, file := range installation.Files {
		recorded[file.Path] = string(file.Digest)
	}
	present := make(map[string]string, len(state.Files))
	for _, file := range state.Files {
		present[file.Path] = string(file.Digest)
	}

	var findings []finding
	for path, digest := range recorded {
		found, exists := present[path]
		switch {
		case !exists:
			findings = append(findings, finding{
				Asset: assetID, Path: destination + "/" + path, Severity: severityError,
				Problem: path + " is recorded as installed and is missing",
				Remedy:  "Run `harnaas install`.",
			})
		case found != digest:
			findings = append(findings, finding{
				Asset: assetID, Path: destination + "/" + path, Severity: severityError,
				Problem: path + " was modified outside harnaas",
				Remedy:  "Run `harnaas install --force` to restore it, or keep the edit and stop declaring the asset.",
			})
		}
	}
	for path := range present {
		if _, exists := recorded[path]; exists {
			continue
		}
		findings = append(findings, finding{
			Asset: assetID, Path: destination + "/" + path, Severity: severityError,
			Problem: path + " is inside a destination harnaas manages and is not part of the installation",
			Remedy:  "Move it elsewhere if it is yours, or run `harnaas install --force` to restore the destination.",
		})
	}
	return findings
}

// checkBridgeLine verifies that CLAUDE.md imports the memory file exactly once,
// and only where instruction content is installed.
//
// Without the line Claude Code never reads the block, so the content is
// installed, reported as installed, and has no effect — which is the silent
// no-op every rule in this design exists to prevent.
func checkBridgeLine(root string, recorded *lockDocument) []finding {
	instructions := 0
	for _, asset := range recorded.Assets {
		for _, installation := range asset.Installations {
			if installation.Destination == memoryFileName {
				instructions++
			}
		}
	}
	if instructions == 0 {
		return nil
	}

	content, err := readManagedFile(root, bridgeFileName)
	if err != nil {
		return nil
	}

	count := 0
	for _, line := range splitLinesKeepingEndings(content) {
		if matchesBridgeLine(string(line)) {
			count++
		}
	}

	switch count {
	case 1:
		return nil
	case 0:
		return []finding{{
			Severity: severityError,
			Problem: fmt.Sprintf("%s does not import %s, so Claude Code never reads the installed instruction content",
				bridgeFileName, memoryFileName),
			Remedy: "Run `harnaas install`.",
		}}
	default:
		return []finding{{
			Severity: severityError,
			Problem:  fmt.Sprintf("%s imports %s %d times", bridgeFileName, memoryFileName, count),
			Remedy:   "Run `harnaas install`, which collapses the duplicates.",
		}}
	}
}

// checkManagedBlocks reports a block whose markers do not pair up, and a block
// whose content no longer matches what the recorded installations imply.
//
// Content outside the markers is never reported. The team owns those bytes, and
// a tool that complained about them would be a tool nobody keeps installed.
func checkManagedBlocks(root string, recorded *lockDocument) []finding {
	var findings []finding

	for _, block := range []managedBlock{instructionBlock, installedIgnoreBlock} {
		content, err := readManagedFile(root, block.file)
		if err != nil {
			continue
		}
		if _, err := locateManagedBlock(content, block); err != nil {
			findings = append(findings, finding{
				Severity: severityError,
				Problem:  err.Error(),
				Remedy:   fmt.Sprintf("Repair the markers in %s by hand, then run `harnaas install`.", block.file),
			})
		}
	}

	_ = recorded
	return findings
}

// plural picks a form, so a one-asset project does not read as though harnaas
// cannot count.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// checkLocalSources re-reads every asset installed from a local source and
// compares it to what was recorded.
//
// This is the one half of update detection that needs no network, so it runs
// even when lint is told to skip every request: a file under `.harnaas` that
// somebody edited is an available update in exactly the sense a moved tag is,
// and refusing to look for it offline would make the offline report quieter
// than it has any right to be.
//
// A source that is no longer there is reported separately from one that
// changed. There is nothing to reinstall from, so "run install" would be a
// remedy that fails.
func checkLocalSources(ctx context.Context, interpretation *manifest.Interpretation, recorded *lockDocument) []finding {
	known := previousAssets(recorded)
	resolver := source.Default.NewResolver(source.RunOptions{})

	var findings []finding
	for _, asset := range interpretation.Assets {
		request := source.Request{Asset: asset, Source: interpretation.Sources[asset.Ref.SourceKey]}
		if request.Kind() != manifest.SourceKindLocal {
			continue
		}
		was, found := known[assetKey{ID: asset.ID, Type: asset.Type}]
		if !found {
			// Never installed, which checkDeclaredButNotInstalled already says.
			continue
		}

		resolved, err := resolver.Resolve(ctx, request)
		if err != nil {
			findings = append(findings, finding{
				Asset: asset.ID, Path: asset.Ref.Path, Severity: severityError,
				Problem: fmt.Sprintf("%q was installed from %s, which is no longer readable there",
					asset.ID, asset.Ref.Path),
				Remedy: fmt.Sprintf("Restore %s, or remove the entry from %s and run `harnaas install`.",
					asset.Ref.Path, manifest.FileName),
			})
			continue
		}

		if resolved.Digest == was.SourceDigest {
			continue
		}
		findings = append(findings, finding{
			Asset: asset.ID, Path: asset.Ref.Path, Severity: severityError,
			Problem: fmt.Sprintf("%s has changed since %q was installed from it", asset.Ref.Path, asset.ID),
			Remedy:  "Run `harnaas install`.",
		})
	}
	return findings
}

// compareVersions orders two version tags the way a release sequence runs,
// returning -1, 0 or 1.
//
// Only the numeric components are compared, and a pre-release sorts below the
// release it qualifies — so `v1.2.0-rc.1` is older than `v1.2.0`, which is what
// keeps a release candidate from ever being offered as an update to a stable
// install. A tag that is not a version at all is not passed here: the caller
// filters those out rather than inventing an ordering for them.
func compareVersions(a, b string) int {
	left, right := versionParts(a), versionParts(b)
	for i := 0; i < len(left) || i < len(right); i++ {
		var l, r int
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		if l != r {
			return cmp.Compare(l, r)
		}
	}

	// Equal numerically: the one carrying a pre-release suffix is the earlier.
	leftPre, rightPre := isPreRelease(a), isPreRelease(b)
	switch {
	case leftPre && !rightPre:
		return -1
	case !leftPre && rightPre:
		return 1
	}
	return 0
}

// versionParts pulls the numeric components out of a version tag, stopping at
// the first pre-release or build separator.
func versionParts(tag string) []int {
	trimmed := strings.TrimPrefix(tag, "v")
	if cut := strings.IndexAny(trimmed, "-+"); cut >= 0 {
		trimmed = trimmed[:cut]
	}

	var parts []int
	for _, field := range strings.Split(trimmed, ".") {
		value, err := strconv.Atoi(field)
		if err != nil {
			break
		}
		parts = append(parts, value)
	}
	return parts
}

// isPreRelease reports whether a version tag carries a pre-release suffix.
func isPreRelease(tag string) bool {
	trimmed := strings.TrimPrefix(tag, "v")
	dash := strings.Index(trimmed, "-")
	return dash >= 0
}

// highestNewerStable returns the highest stable tag above installed, or the
// empty string when there is none.
//
// Exactly one finding is produced for an asset however many tags are newer,
// because the reader upgrades to the newest and the intermediate ones are not
// decisions they have to make. A pre-release is never offered over a stable
// install, and a tag that is not a version is ignored rather than producing a
// spurious comparison.
func highestNewerStable(installed string, tags []string) string {
	best := ""
	for _, tag := range tags {
		if !looksLikeVersionTag(tag) || isPreRelease(tag) {
			continue
		}
		if compareVersions(tag, installed) <= 0 {
			continue
		}
		if best == "" || compareVersions(tag, best) > 0 {
			best = tag
		}
	}
	return best
}

// refResolver resolves a source's ref to the commit it currently names.
//
// A parameter rather than a direct call so update detection is exercisable
// without a remote: the failures that matter here — a ref that has vanished, a
// host that cannot be reached — are the ones hardest to arrange for real, and
// they are exactly the ones whose handling has to be right.
type refResolver func(ctx context.Context, req source.Request) (github.RefResolution, error)

// tagLister returns the tags a repository publishes. A parameter for the reason
// refResolver is one: the interesting cases are the ones a real remote will not
// produce on demand.
type tagLister func(ctx context.Context, repository string) ([]string, error)

// checkUpstream reports refs that have moved and tags that have been superseded.
//
// A commit-pinned asset is never checked and never causes a request: the user
// pinned it deliberately, so there is no newer commit to find on its behalf and
// asking would be a request made to learn nothing.
//
// A host that cannot be reached does not fail the run and does not mask a local
// finding. The affected assets are marked unchecked and the summary says how
// many, so a clean report is never mistaken for a complete one — and repeated
// failures against one host are reported once for that host rather than once
// per asset, because it is one outage and one thing to fix.
func checkUpstream(
	ctx context.Context,
	interpretation *manifest.Interpretation,
	recorded *lockDocument,
	resolve refResolver,
	listTags tagLister,
) (findings []finding, unchecked []string) {
	known := previousAssets(recorded)
	failedHosts := make(map[string]bool)

	for _, asset := range interpretation.Assets {
		declared, found := interpretation.Sources[asset.Ref.SourceKey]
		if !found || declared.Kind != manifest.SourceKindGitHub {
			continue
		}
		was, installed := known[assetKey{ID: asset.ID, Type: asset.Type}]
		if !installed {
			continue
		}
		if looksLikeCommit(declared.Ref) {
			continue
		}

		resolution, err := resolve(ctx, source.Request{Asset: asset, Source: declared})
		if err != nil {
			// A ref that is gone is not an outage and not an update: there is
			// no newer content on offer, and a reinstall would fail outright.
			// Folding it into the host summary would hide a problem only a
			// manifest edit fixes behind one that fixes itself.
			var unknown *github.UnknownRefError
			if errors.As(err, &unknown) {
				findings = append(findings, finding{
					Asset: asset.ID, Severity: severityError,
					Problem: fmt.Sprintf("%q no longer exists in %s, and %q was installed from it",
						declared.Ref, declared.Repository, asset.ID),
					Remedy: fmt.Sprintf(
						"Point the %q source in %s at a ref that exists, then run `harnaas install`.",
						declared.Key, manifest.FileName),
				})
				continue
			}

			// One outage is one thing to fix. The first asset that meets it
			// reports it; the rest are counted as unchecked.
			if !failedHosts[declared.Repository] {
				failedHosts[declared.Repository] = true
				findings = append(findings, finding{
					Asset: asset.ID, Severity: severityWarning,
					Problem: fmt.Sprintf("%s could not be checked for updates: %v", declared.Repository, err),
					Remedy:  "Run `harnaas lint` again when the remote is reachable, or use --offline to skip update checks deliberately.",
				})
			}
			unchecked = append(unchecked, asset.ID)
			continue
		}

		if looksLikeVersionTag(declared.Ref) && listTags != nil {
			// A tag that has not moved can still have been superseded, which
			// is a different question from the one resolution answers.
			if superseded := checkSupersededTag(ctx, asset.ID, declared, listTags); superseded != nil {
				findings = append(findings, *superseded)
			}
		}

		if resolution.Commit == was.ResolvedCommit {
			continue
		}
		findings = append(findings, finding{
			Asset: asset.ID, Severity: severityError,
			Problem: fmt.Sprintf("%q now points at %s and %q was installed from %s",
				declared.Ref, short(resolution.Commit), asset.ID, short(was.ResolvedCommit)),
			Remedy: "Run `harnaas install`.",
		})
	}

	return findings, unchecked
}

// short abbreviates a commit for a message, where the full identifier would
// crowd out the sentence around it.
func short(commit string) string {
	if len(commit) <= 12 {
		return commit
	}
	return commit[:12]
}

// checkSupersededTag reports the highest stable tag published above the one an
// asset is pinned to.
//
// A listing that fails is not reported here. The ref resolution beside it has
// already met the same remote, so a second finding about the same outage would
// be the per-asset noise the host summary exists to prevent.
func checkSupersededTag(ctx context.Context, assetID string, declared manifest.Source, listTags tagLister) *finding {
	tags, err := listTags(ctx, declared.Repository)
	if err != nil {
		return nil
	}

	newer := highestNewerStable(declared.Ref, tags)
	if newer == "" {
		return nil
	}

	// The remedy prints the exact strings so applying it is a literal
	// substitution rather than an interpretation of one.
	return &finding{
		Asset: assetID, Severity: severityError,
		Problem: fmt.Sprintf("%q is installed from %s and %s has been published",
			assetID, declared.Ref, newer),
		Remedy: fmt.Sprintf(
			"In %s, change the %q source from\n      %q\n    to\n      %q\n    then run `harnaas install`.",
			manifest.FileName, declared.Key,
			declared.String(),
			(manifest.Source{Kind: declared.Kind, Repository: declared.Repository, Ref: newer}).String(),
		),
	}
}

// checkUnmanagedConflict reports a declared asset whose destination exists on
// disk and no lockfile entry claims.
//
// Reported by lint because install will refuse it, and a team that only meets
// the refusal at install time meets it on the day they were trying to get
// something done. The message states the rule rather than only the fact: harnaas
// never claims a path it did not create, and a reader who assumes --force will
// deal with it later has been told twice that it will not.
func checkUnmanagedConflict(root string, interpretation *manifest.Interpretation, recorded *lockDocument) []finding {
	claimed := recorded.claimants()

	var findings []finding
	for _, asset := range interpretation.Assets {
		for _, target := range asset.Targets {
			plan := planTarget(asset, nil, target, adapter.Default)
			if !plan.supported() || plan.Instruction || plan.Destination == "" {
				continue
			}
			if _, mine := claimed[destinationKey{Scope: plan.Scope, Destination: plan.Destination}]; mine {
				continue
			}

			scopeRoot, err := scopeRootFor(root, target, asset)
			if err != nil {
				continue
			}
			state, err := readInstalled(scopeRoot, plan.Destination)
			if err != nil || !state.Present {
				continue
			}

			reported := projectRelative(root, scopeRoot, plan.Destination)
			findings = append(findings, finding{
				Asset: asset.ID, Path: reported, Severity: severityError,
				Problem: fmt.Sprintf("%s exists and harnaas did not install it, so install will not write %q there",
					reported, asset.ID),
				Remedy: fmt.Sprintf(
					"harnaas never claims a path it did not create, --force included. Move or delete %s yourself, then run `harnaas install`.",
					reported),
			})
		}
	}
	return findings
}

// checkIgnoreBlock reports an installed path the ignore block no longer lists.
//
// The block is what keeps installed content out of a team's commits, so a path
// missing from it is a file about to be committed by somebody who did not
// choose to. Content outside the markers is not read at all — those bytes are
// the team's, and a tool that complained about them is a tool nobody keeps.
func checkIgnoreBlock(root string, recorded *lockDocument) []finding {
	var installed []string
	for _, asset := range recorded.Assets {
		for _, installation := range asset.Installations {
			if installation.Scope != manifest.ScopeProject || installation.Destination == memoryFileName {
				continue
			}
			scopeRoot, err := scopeRootFor(root, installation.Harness, manifest.Asset{
				ID: asset.ID, Type: asset.Type, Scope: installation.Scope,
			})
			if err != nil {
				continue
			}
			installed = append(installed, projectRelative(root, scopeRoot, installation.Destination))
		}
	}
	if len(installed) == 0 {
		return nil
	}

	content, err := readManagedFile(root, ignoreFileName)
	if err != nil {
		return nil
	}
	span, err := locateManagedBlock(content, installedIgnoreBlock)
	if err != nil || !span.found {
		// A malformed or absent block is checkManagedBlocks's finding to make;
		// reporting every path here as well would be the same problem counted
		// once per asset.
		return nil
	}

	block := string(content[span.start:span.end])
	var findings []finding
	for _, path := range installed {
		if strings.Contains(block, ignoreEntry(path)+"\n") {
			continue
		}
		findings = append(findings, finding{
			Path: path, Severity: severityError,
			Problem: fmt.Sprintf("%s is installed and %s no longer ignores it", path, ignoreFileName),
			Remedy:  "Run `harnaas install`, which regenerates the block from what is installed.",
		})
	}
	return findings
}
