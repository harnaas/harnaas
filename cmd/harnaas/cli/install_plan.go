package cli

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// planInstall turns resolved assets into the whole set of changes the run will
// make, deciding one outcome per asset and target and touching nothing.
//
// Assets are processed in a stable order derived from their ids rather than
// from manifest order, so reordering harnaas.json changes neither the report
// nor the lockfile.
func planInstall(
	root string,
	resolutions []resolvedAsset,
	recorded *lockDocument,
	opts *installOptions,
) (*installPlan, error) {
	ordered := slices.Clone(resolutions)
	slices.SortStableFunc(ordered, func(a, b resolvedAsset) int {
		if byID := compareStrings(a.Asset.ID, b.Asset.ID); byID != 0 {
			return byID
		}
		return compareStrings(string(a.Asset.Type), string(b.Asset.Type))
	})

	plan := &installPlan{}
	claimed := recorded.claimants()
	previous := previousInstallations(recorded)
	previousByAsset := previousAssets(recorded)

	for _, resolution := range ordered {
		asset := resolution.Asset
		lock := lockAsset{
			ID: asset.ID, Type: asset.Type,
			Source:         recordedSource(resolution.Resolved.Provenance.Source),
			RequestedRef:   resolution.Resolved.Provenance.RequestedRef,
			ResolvedCommit: resolution.Resolved.Provenance.ResolvedCommit,
			SourceDigest:   resolution.Resolved.Digest,
			InstalledAt:    time.Now().UTC(),
		}

		for _, target := range asset.Targets {
			result, installation, err := planOne(root, asset, resolution.Resolved, target, recorded, previous, plan, opts)
			if err != nil {
				return nil, err
			}
			plan.results = append(plan.results, result)
			if installation != nil {
				lock.Installations = append(lock.Installations, *installation)
			}
		}

		if len(lock.Installations) > 0 {
			// Carry the previous timestamp forward where nothing about this
			// asset changed, so a run that installed nothing produces a
			// byte-identical lockfile.
			if unchanged, at := unchangedSince(previousByAsset, lock); unchanged {
				lock.InstalledAt = at
			}
			plan.assets = append(plan.assets, lock)
		}
	}

	plan.removes = planRemovals(root, plan, recorded, claimed)
	return plan, nil
}

// compareStrings orders two strings, extracted so the sort functions above read
// as the rules they encode rather than as calls into the standard library.
func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// installationKey identifies one recorded installation.
type installationKey struct {
	AssetID     string
	Type        manifest.AssetType
	Harness     harness.ID
	Scope       manifest.Scope
	Destination string
}

// previousInstallations indexes what the lockfile records, so planning can ask
// "is this destination mine, and did it change?" without scanning.
func previousInstallations(recorded *lockDocument) map[installationKey]lockInstallation {
	index := make(map[installationKey]lockInstallation)
	for _, asset := range recorded.Assets {
		for _, installation := range asset.Installations {
			index[installationKey{
				AssetID: asset.ID, Type: asset.Type, Harness: installation.Harness,
				Scope: installation.Scope, Destination: installation.Destination,
			}] = installation
		}
	}
	return index
}

// assetKey identifies one recorded asset.
type assetKey struct {
	ID   string
	Type manifest.AssetType
}

// previousAssets indexes the recorded assets, so planning can recover the
// timestamp an unchanged asset has to keep.
func previousAssets(recorded *lockDocument) map[assetKey]lockAsset {
	index := make(map[assetKey]lockAsset, len(recorded.Assets))
	for _, asset := range recorded.Assets {
		index[assetKey{ID: asset.ID, Type: asset.Type}] = asset
	}
	return index
}

// unchangedSince reports whether an asset's planned installations match what
// was recorded, and the recorded time to carry forward if so.
//
// Carrying the time forward is what makes a re-run that changed nothing produce
// a byte-identical lockfile. Stamping the current moment instead would put a
// new timestamp in a committed file on every run, so "nothing changed" could
// never be shown by the file itself.
//
// The comparison is over the whole asset rather than per installation: the
// timestamp is the asset's, so one target changing makes the asset changed.
func unchangedSince(previous map[assetKey]lockAsset, planned lockAsset) (bool, time.Time) {
	recorded, found := previous[assetKey{ID: planned.ID, Type: planned.Type}]
	if !found || len(recorded.Installations) != len(planned.Installations) {
		return false, time.Time{}
	}
	if recorded.SourceDigest != planned.SourceDigest {
		return false, time.Time{}
	}

	byDestination := make(map[destinationKey]lockInstallation, len(recorded.Installations))
	for _, installation := range recorded.Installations {
		byDestination[destinationKey{Scope: installation.Scope, Destination: installation.Destination}] = installation
	}

	for _, installation := range planned.Installations {
		was, present := byDestination[destinationKey{Scope: installation.Scope, Destination: installation.Destination}]
		if !present || was.InstalledDigest != installation.InstalledDigest {
			return false, time.Time{}
		}
	}
	return true, recorded.InstalledAt
}

// planOne decides the outcome for one asset and one target.
//
// The order of the checks is the protection model. Whether harnaas may write
// here at all is settled before what it would write: an unmanaged destination
// is refused before the content is rendered, so no amount of rendering can talk
// the flow into overwriting somebody's file.
func planOne(
	root string,
	asset manifest.Asset,
	resolved *source.Resolved,
	target harness.ID,
	recorded *lockDocument,
	previous map[installationKey]lockInstallation,
	plan *installPlan,
	opts *installOptions,
) (installResult, *lockInstallation, error) {
	targetPlan := planTarget(asset, resolved.Files, target, adapter.Default)

	result := installResult{
		AssetID: asset.ID, Type: asset.Type, Harness: target,
		Scope: targetPlan.Scope, Destination: targetPlan.Destination, Note: targetPlan.Note,
	}

	if !targetPlan.supported() {
		return result.withOutcome(outcomeUnsupported, targetPlan.Unsupported), nil, nil
	}

	rendered, err := render(renderRequest{
		Asset: asset, Files: resolved.Files, Renderer: targetPlan.Renderer, Target: target,
	})
	if err != nil {
		// A renderer that cannot produce this pairing is an outcome, not a
		// failure of the run: the other targets and the other assets still
		// install, and the report carries the reason.

		return result.withOutcome(outcomeUnsupported, err.Error()), nil, nil
	}

	if asset.Type == manifest.AssetTypeSkill || targetPlan.Renderer == adapter.RendererAsSkill {
		for _, file := range rendered.Files {
			if file.Path == skillFileName {
				if err := checkSkillSize(asset.ID, file.Content); err != nil {
					return result.withOutcome(outcomeUnsupported, err.Error()), nil, nil
				}
			}
		}
	}

	// An instruction contributes to the memory file's block rather than to a
	// destination of its own: the block as a whole is what the next phase
	// writes, so there is no path here to protect from an overwrite.
	//
	// It is still recorded, against the memory file and digested over its own
	// contribution rather than over the whole block. Two things need that. A
	// re-run has to be able to say `unchanged` — without a record every
	// instruction reports `created` forever, and the report stops meaning
	// anything — and `lint` has to be able to name *which* instruction the
	// block no longer matches rather than only that it drifted.
	if targetPlan.Instruction {
		plan.instructions = append(plan.instructions, instruction{
			ID: asset.ID, Source: declaredSource(asset.Ref), Content: rendered.Files[0].Content,
		})

		contribution := source.DigestContent(rendered.Files[0].Content)
		installation := &lockInstallation{
			Harness: target, Scope: targetPlan.Scope, Destination: memoryFileName,
			Renderer: string(targetPlan.Renderer), Emulated: rendered.Emulated,
			Files:           []lockFile{{Path: asset.ID, Digest: contribution}},
			InstalledDigest: contribution,
		}

		recordedInstruction, was := previous[installationKey{
			AssetID: asset.ID, Type: asset.Type, Harness: target,
			Scope: targetPlan.Scope, Destination: memoryFileName,
		}]

		switch {
		case rendered.Emulated:
			return result.withOutcome(outcomeEmulated, rendered.Note), installation, nil
		case !was:
			return result.withOutcome(outcomeCreated, ""), installation, nil
		case recordedInstruction.InstalledDigest == contribution && memoryBlockHolds(root, asset.ID, rendered.Files[0].Content):
			return result.withOutcome(outcomeUnchanged, ""), installation, nil
		default:
			return result.withOutcome(outcomeUpdated, ""), installation, nil
		}
	}

	scopeRoot, err := scopeRootFor(root, target, asset)
	if err != nil {
		return result.withOutcome(outcomeUnsupported, err.Error()), nil, nil
	}

	// Set before any outcome returns. The lockfile records the path from the
	// scope root, but every sentence the reader is about to be shown has to name
	// the path they can actually go and look at — telling somebody to delete
	// `rules/house-style.md` when the file is at `.claude/rules/house-style.md`
	// is a remedy that does not work.
	projectPath := projectRelative(root, scopeRoot, targetPlan.Destination)
	if targetPlan.Scope == manifest.ScopeProject {
		result.Destination = projectPath
	}

	state, err := readInstalled(scopeRoot, targetPlan.Destination)
	if err != nil {
		return result, nil, err
	}

	key := installationKey{
		AssetID: asset.ID, Type: asset.Type, Harness: target,
		Scope: targetPlan.Scope, Destination: targetPlan.Destination,
	}
	recordedInstallation, managed := previous[key]

	switch {
	case state.Present && !managed && !claimedByAnother(recorded, targetPlan, asset):
		// Nothing harnaas installed is here, so it is somebody's file. This is
		// refused on every flag: --force restores what harnaas put there and has
		// no business destroying what it did not.
		return result.withOutcome(outcomeConflictUnmanaged, ""), nil, nil

	case state.Present && managed && state.Digest != recordedInstallation.InstalledDigest && !opts.force:
		// The record is carried forward unchanged. harnaas still owns this
		// destination — it declined to rewrite it, which is not the same as
		// giving it up — and dropping the entry would make the next run meet
		// its own file as unmanaged and refuse it on every flag, including the
		// --force this outcome just told the reader to use.
		kept := recordedInstallation
		return result.withOutcome(outcomeConflictDrift, ""), &kept, nil
	}

	digest, files, err := digestRendered(rendered.Files)
	if err != nil {
		return result, nil, err
	}

	switch {
	case rendered.Emulated:
		result = result.withOutcome(outcomeEmulated, rendered.Note)
	case !state.Present:
		result = result.withOutcome(outcomeCreated, "")
	case state.Digest == digest:
		result = result.withOutcome(outcomeUnchanged, "")
	default:
		result = result.withOutcome(outcomeUpdated, "")
	}

	if result.Outcome != outcomeUnchanged {
		plan.writes = append(plan.writes, plannedWrite{
			Asset: asset, Plan: targetPlan, Files: rendered.Files, Root: scopeRoot,
		})
	}

	if targetPlan.Scope == manifest.ScopeProject {
		plan.installedPaths = append(plan.installedPaths, projectPath)
	}

	installation := &lockInstallation{
		Harness: target, Scope: targetPlan.Scope, Destination: targetPlan.Destination,
		Renderer: string(targetPlan.Renderer), Emulated: rendered.Emulated,
		Files: files, InstalledDigest: digest,
	}
	return result, installation, nil
}

// claimedByAnother reports whether the lockfile records this destination for a
// different harness.
//
// A shared destination is one file several harnesses read, so meeting it while
// planning the second harness is not a conflict — harnaas installed it, for the
// first.
func claimedByAnother(recorded *lockDocument, plan targetPlan, asset manifest.Asset) bool {
	for _, recordedAsset := range recorded.Assets {
		if recordedAsset.ID != asset.ID || recordedAsset.Type != asset.Type {
			continue
		}
		for _, installation := range recordedAsset.Installations {
			if installation.Scope == plan.Scope && installation.Destination == plan.Destination {
				return true
			}
		}
	}
	return false
}

// digestRendered computes the installed digest and per-file digests the same
// way a resolved source's are, so the two sides of every later comparison were
// produced by one computation.
func digestRendered(files []source.File) (source.Digest, []lockFile, error) {
	content := make(map[string][]byte, len(files))
	for _, file := range files {
		content[file.Path] = file.Content
	}

	resolved, err := source.NewResolved(source.Provenance{}, content)
	if err != nil {
		return "", nil, fmt.Errorf("digest the rendered content: %w", err)
	}

	recordedFiles := make([]lockFile, 0, len(resolved.Files))
	for _, file := range resolved.Files {
		recordedFiles = append(recordedFiles, lockFile{Path: file.Path, Digest: file.Digest})
	}
	return resolved.Digest, recordedFiles, nil
}

// scopeRootFor resolves the directory a destination is relative to.
//
// A skill and an instruction reach every harness through shared locations, so
// they resolve against the project root without consulting an adapter — which
// is what lets a harness with no adapter still receive them.
func scopeRootFor(root string, target harness.ID, asset manifest.Asset) (string, error) {
	if asset.Scope == manifest.ScopeUser {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve the home directory for a user-scoped asset: %w", err)
		}
		if asset.Type == manifest.AssetTypeSkill {
			return home, nil
		}
		adapted, lookupErr := adapter.Default.Lookup(target)
		if lookupErr != nil {
			return "", lookupErr
		}
		relative, resolveErr := adapter.ResolveRoot(adapted, asset)
		if resolveErr != nil {
			return "", resolveErr
		}
		return filepath.Join(home, relative), nil
	}

	if asset.Type == manifest.AssetTypeSkill {
		return root, nil
	}

	adapted, err := adapter.Default.Lookup(target)
	if err != nil {
		return "", err
	}
	relative, err := adapter.ResolveRoot(adapted, asset)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, relative), nil
}

// planRemovals is convergence: the managed destinations no longer declared.
//
// Only destinations the lockfile records and whose content still matches are
// removed. A destination somebody edited is kept and reported, because deleting
// it would destroy work; and one another recorded harness still claims is kept,
// because attribution is per harness and the file is one file.
func planRemovals(root string, plan *installPlan, recorded *lockDocument, claimed map[destinationKey][]harness.ID) []plannedRemoval {
	planned := make(map[destinationKey]bool)
	for _, write := range plan.writes {
		planned[destinationKey{Scope: write.Plan.Scope, Destination: write.Plan.Destination}] = true
	}
	for _, asset := range plan.assets {
		for _, installation := range asset.Installations {
			planned[destinationKey{Scope: installation.Scope, Destination: installation.Destination}] = true
		}
	}

	var removals []plannedRemoval
	seen := make(map[destinationKey]bool)
	for _, asset := range recorded.Assets {
		for _, installation := range asset.Installations {
			key := destinationKey{Scope: installation.Scope, Destination: installation.Destination}
			if planned[key] || seen[key] {
				continue
			}
			seen[key] = true

			// A destination another recorded harness still claims stays: the
			// harness field is attribution, the file is one file, and removing
			// it because one claimant went away would break the others.
			if stillClaimed(claimed[key], recorded, plan) {
				continue
			}

			// The root has to be resolved the same way planning resolved it, or
			// an adapter-based destination is looked for beneath the project
			// root, found missing, and silently never removed — which is how a
			// full uninstall leaves .claude/ behind. The asset is reconstructed
			// from the record because it is, by definition, no longer declared.
			scopeRoot, err := scopeRootFor(root, installation.Harness, manifest.Asset{
				ID: asset.ID, Type: asset.Type, Scope: installation.Scope,
			})
			if err != nil {
				continue
			}

			state, err := readInstalled(scopeRoot, installation.Destination)
			if err != nil || !state.Present || state.Digest != installation.InstalledDigest {
				// Absent is nothing to do, and drifted is somebody's edit.
				continue
			}
			removals = append(removals, plannedRemoval{
				Scope: installation.Scope, Root: scopeRoot, Destination: installation.Destination,
				Reported: projectRelative(root, scopeRoot, installation.Destination),
			})
		}
	}
	return removals
}

// projectRelative renders a scope-root-relative destination as a path from the
// project root, for the ignore file and the report.
//
// A scope root outside the project — the user's home — has no such path, so the
// destination is returned as it stands rather than as a chain of `..` segments
// that would name a different file on every machine.
func projectRelative(root, scopeRoot, destination string) string {
	relative, err := filepath.Rel(root, scopeRoot)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return destination
	}
	if relative == "." {
		return destination
	}
	return path.Join(filepath.ToSlash(relative), destination)
}

// memoryBlockHolds reports whether the memory file's managed block already
// carries this instruction's content.
//
// The lockfile digest alone would say "this asset has not changed", which is
// not the same question: somebody may have edited the block itself, and an
// install that reported `unchanged` while the file no longer held the content
// would be exactly the silent no-op the design refuses. Reading the file is
// what makes the answer about the file.
func memoryBlockHolds(root, assetID string, content []byte) bool {
	existing, err := readManagedFile(root, memoryFileName)
	if err != nil {
		return false
	}
	span, err := locateManagedBlock(existing, instructionBlock)
	if err != nil || !span.found {
		return false
	}

	block := string(existing[span.start:span.end])
	return strings.Contains(block, string(content)) && strings.Contains(block, `"`+assetID+`"`)
}

// stillClaimed reports whether any harness the run is keeping still records
// this destination.
//
// Attribution is per harness and the file is one file, so a destination three
// harnesses read survives one of them being dropped from `targets`. It is
// removed only once nothing the run is keeping points at it.
func stillClaimed(claimants []harness.ID, recorded *lockDocument, plan *installPlan) bool {
	if len(claimants) < 2 {
		return false
	}
	_ = recorded
	for _, asset := range plan.assets {
		for _, installation := range asset.Installations {
			if slices.Contains(claimants, installation.Harness) {
				return true
			}
		}
	}
	return false
}
