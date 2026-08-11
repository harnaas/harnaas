package cli

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// outcome is what happened to one asset for one target.
//
// The set is closed and every pairing produces exactly one of them, including
// the pairings nothing was written for. A pairing that simply vanished from the
// report would be the silent no-op the whole design refuses: the reader would
// have no way to tell "installed" from "harnaas decided not to".
type outcome string

const (
	// outcomeCreated and outcomeUpdated are the two ways bytes landed.
	outcomeCreated outcome = "created"
	outcomeUpdated outcome = "updated"

	// outcomeUnchanged is reported rather than omitted, because "nothing to do"
	// is the answer a re-run exists to give.
	outcomeUnchanged outcome = "unchanged"

	// outcomeEmulated is an asset that reached the harness through another
	// type's surface. It is never reported as created or updated, because the
	// installed form behaves differently from a native one and the reader has
	// to know which they have.
	outcomeEmulated outcome = "emulated"

	// outcomeConflictUnmanaged is a destination that exists and no lockfile
	// entry claims. Nothing is written, on any flag.
	outcomeConflictUnmanaged outcome = "conflict-unmanaged"

	// outcomeConflictDrift is a managed destination edited since it was
	// installed. Nothing is written without --force.
	outcomeConflictDrift outcome = "conflict-drift"

	// outcomeUnsupported is a pairing harnaas will not write: a removed
	// surface, a renderer nobody implemented, or an emulation that would change
	// what the asset means.
	outcomeUnsupported outcome = "unsupported"
)

// outcomes is every outcome in report order, which runs from the ones that
// changed something to the ones that need a decision.
func outcomes() []outcome {
	return []outcome{
		outcomeCreated, outcomeUpdated, outcomeEmulated, outcomeUnchanged,
		outcomeConflictUnmanaged, outcomeConflictDrift, outcomeUnsupported,
	}
}

// blocked reports whether an outcome stopped or altered an install, which is
// exactly the set that must carry a remedy.
func (o outcome) blocked() bool {
	switch o {
	case outcomeConflictUnmanaged, outcomeConflictDrift, outcomeUnsupported, outcomeEmulated:
		return true
	case outcomeCreated, outcomeUpdated, outcomeUnchanged:
		return false
	}
	return false
}

// withOutcome settles a result: the outcome, the sentence explaining it, and
// the remedy where one is owed.
//
// Attaching the remedy here rather than at each call site is what makes "every
// outcome that blocked or altered an install carries a runnable remedy" true by
// construction — a new blocked outcome added to [outcome.blocked] gets one
// without anybody remembering to ask for it.
func (r installResult) withOutcome(o outcome, note string) installResult {
	r.Outcome = o
	if note != "" {
		r.Note = note
	}
	if o.blocked() {
		r.Remedy = remedyFor(r)
	}
	return r
}

// installResult is one asset-and-target pairing's line in the report.
type installResult struct {
	AssetID string             `json:"asset"`
	Type    manifest.AssetType `json:"type"`
	Harness harness.ID         `json:"harness"`
	Scope   manifest.Scope     `json:"scope"`

	// Destination is relative to the scope root, and is empty for an
	// instruction, which lands inside the memory file's block.
	Destination string `json:"destination,omitempty"`

	Outcome outcome `json:"outcome"`

	// Remedy is the command or manifest edit that resolves a blocked outcome,
	// and Note the sentence a gated surface or an emulation carries.
	Remedy string `json:"remedy,omitempty"`
	Note   string `json:"note,omitempty"`
}

// installReport is the whole run, in the order it is printed and encoded.
type installReport struct {
	Results []installResult `json:"results"`

	// Failures are the assets that could not be resolved at all, which is a
	// different thing from a pairing harnaas declined to write.
	Failures []installFailure `json:"failures,omitempty"`

	// Removed is every managed destination convergence deleted.
	//
	// Not one of the seven outcomes, because it is not something that happened
	// to a declared asset — it happened to one that is no longer declared, and
	// there is no manifest entry to report it against. It is still reported:
	// deleting files and saying nothing is the one thing convergence must not
	// do.
	Removed []string `json:"removed,omitempty"`

	// DryRun records that nothing was written, so a JSON consumer cannot
	// mistake a plan for a run.
	DryRun bool `json:"dryRun"`
}

// installFailure is one asset that never reached the planning phase.
type installFailure struct {
	AssetID string `json:"asset"`
	Problem string `json:"problem"`
}

// sort puts the report in its stable order: by asset id, then type, then
// harness, then destination.
//
// Derived from identifiers rather than from manifest order or filesystem
// iteration order, so reordering the manifest changes neither the report nor
// the lockfile, and two runs over one project print the same thing.
func (r *installReport) sort() {
	slices.SortStableFunc(r.Results, func(a, b installResult) int {
		if byID := cmp.Compare(a.AssetID, b.AssetID); byID != 0 {
			return byID
		}
		if byType := cmp.Compare(string(a.Type), string(b.Type)); byType != 0 {
			return byType
		}
		if byHarness := cmp.Compare(string(a.Harness), string(b.Harness)); byHarness != 0 {
			return byHarness
		}
		return cmp.Compare(a.Destination, b.Destination)
	})
	slices.SortStableFunc(r.Failures, func(a, b installFailure) int {
		return cmp.Compare(a.AssetID, b.AssetID)
	})
	slices.Sort(r.Removed)
}

// counts returns how many pairings each outcome covered, for the summary.
func (r *installReport) counts() map[outcome]int {
	counted := make(map[outcome]int, len(outcomes()))
	for _, result := range r.Results {
		counted[result.Outcome]++
	}
	return counted
}

// changed reports whether anything was written, which is what decides between
// a summary and "nothing to do".
func (r *installReport) changed() bool {
	for _, result := range r.Results {
		switch result.Outcome {
		case outcomeCreated, outcomeUpdated, outcomeEmulated:
			return true
		case outcomeUnchanged, outcomeConflictUnmanaged, outcomeConflictDrift, outcomeUnsupported:
		}
	}
	return false
}

// remedyFor is the runnable next step for a blocked outcome.
//
// Every one of them is a command to run or an edit to make, never a
// restatement of the problem: a reader who has just been told harnaas refused
// to do something needs the sentence that gets them unblocked.
func remedyFor(result installResult) string {
	switch result.Outcome {
	case outcomeConflictUnmanaged:
		return fmt.Sprintf(
			"harnaas never overwrites a path it did not create, --force included. "+
				"Move or delete %s yourself, then run `harnaas install` again.",
			result.Destination,
		)
	case outcomeConflictDrift:
		return fmt.Sprintf(
			"%s was edited since harnaas installed it. Run `harnaas install --force` to restore "+
				"the source content, or keep the edit and remove the asset from %s.",
			result.Destination, manifest.FileName,
		)
	case outcomeUnsupported:
		return fmt.Sprintf(
			"Nothing was written for this target. Narrow the entry's %q in %s so it does not name %s.",
			"targets", manifest.FileName, result.Harness,
		)
	case outcomeEmulated:
		return "No action is needed. This is reported separately from a native install because it behaves differently."
	case outcomeCreated, outcomeUpdated, outcomeUnchanged:
		return ""
	}
	return ""
}
