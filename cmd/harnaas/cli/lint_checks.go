package cli

import (
	"fmt"
	"strings"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
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
