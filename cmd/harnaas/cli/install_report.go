package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/jsonutil"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// apply carries out the plan: writes, then removals, then the managed blocks,
// then the lockfile.
//
// The lockfile is written last and records only what actually landed. A
// lockfile written first would claim ownership of destinations a later failure
// never created, and the next run would then converge by deleting files it had
// never installed.
func (p *installPlan) apply(root string, recorded *lockDocument) error {
	for _, write := range p.writes {
		if err := writeDestination(write.Root, write.Plan.Destination, write.Files); err != nil {
			return err
		}
	}

	for _, removal := range p.removes {
		if err := removeDestination(removal.Root, removal.Destination); err != nil {
			return err
		}
	}

	if err := p.writeBlocks(root); err != nil {
		return err
	}

	_ = recorded
	return saveLock(root, newLockDocument(p.assets))
}

// writeBlocks maintains the three files harnaas shares with the team.
//
// The bridge line follows the instruction block rather than being maintained
// independently: it exists only to make that block reachable from Claude Code,
// so when the block goes the line has nothing left to point at.
func (p *installPlan) writeBlocks(root string) error {
	if err := writeInstructionBlock(root, p.instructions); err != nil {
		return err
	}

	if len(p.instructions) == 0 {
		if err := dropBridgeLine(root); err != nil {
			return err
		}
	} else if err := writeBridgeLine(root); err != nil {
		return err
	}

	return writeIgnoreBlock(root, p.installedPaths)
}

// writeInstallReport prints the run, as one JSON document or as text.
//
// Under --json the document is the only thing on stdout and every advisory
// sentence goes to stderr, so a CI job can pipe stdout into a parser without
// the text harnaas wanted a human to read corrupting it.
func writeInstallReport(cmd *cobra.Command, report *installReport, opts *installOptions) error {
	if opts.asJSON {
		encoded, err := jsonutil.Marshal(report)
		if err != nil {
			return fmt.Errorf("encode the install report: %w", err)
		}
		if _, err := cmd.OutOrStdout().Write(encoded); err != nil {
			return fmt.Errorf("write the install report: %w", err)
		}
		if opts.dryRun {
			advisef(cmd, "This was a dry run. Nothing was written.\n")
		}
		return nil
	}

	out := cmd.OutOrStdout()

	if opts.dryRun {
		advisef(cmd, "Dry run: nothing was written.\n")
	}

	for _, result := range report.Results {
		// An empty destination means two different things, and only one of them
		// is a place. An instruction has no destination of its own because it
		// lands inside the memory file's block, so naming that file is the
		// useful answer; an unsupported pairing has none because nothing was
		// written, and borrowing the instruction's answer would print an arrow
		// at a file harnaas did not touch.
		if result.Outcome == outcomeUnsupported {
			fmt.Fprintf(out, "%-18s %s (%s)\n", result.Outcome, result.AssetID, result.Harness)
		} else {
			where := result.Destination
			if where == "" {
				where = memoryFileName
			}
			fmt.Fprintf(out, "%-18s %s (%s) -> %s\n", result.Outcome, result.AssetID, result.Harness, where)
		}
		if result.Note != "" {
			fmt.Fprintf(out, "  %s\n", result.Note)
		}
		if result.Remedy != "" {
			fmt.Fprintf(out, "  %s\n", result.Remedy)
		}
	}

	for _, failure := range report.Failures {
		fmt.Fprintf(out, "%-18s %s\n", "failed", failure.AssetID)
		fmt.Fprintf(out, "  %s\n", failure.Problem)
	}

	writeInstallSummary(cmd, report)
	return nil
}

// writeInstallSummary ends the report with a count per outcome.
//
// Only the outcomes that occurred are listed: a summary padded with zeroes
// teaches a reader to skim past the line that mattered.
func writeInstallSummary(cmd *cobra.Command, report *installReport) {
	out := cmd.OutOrStdout()
	counted := report.counts()

	if len(report.Results) == 0 && len(report.Failures) == 0 && len(report.Removed) == 0 {
		fmt.Fprintf(out, "Nothing is declared in %s.\n", manifest.FileName)
		return
	}

	fmt.Fprintln(out)
	for _, name := range outcomes() {
		if count := counted[name]; count > 0 {
			fmt.Fprintf(out, "%d %s\n", count, name)
		}
	}

	// Removals are listed by path rather than only counted. Convergence is the
	// one part of an install that deletes, and a reader who emptied the assets
	// array has to be able to see exactly what went — a bare number is the
	// least reassuring possible way to report a deletion.
	if len(report.Removed) > 0 {
		fmt.Fprintf(out, "%d no longer declared, removed\n", len(report.Removed))
		for _, removed := range report.Removed {
			fmt.Fprintf(out, "  %s\n", removed)
		}
	}

	if len(report.Failures) > 0 {
		fmt.Fprintf(out, "%d failed to resolve\n", len(report.Failures))
	}
	if !report.changed() && len(report.Failures) == 0 && len(report.Removed) == 0 {
		fmt.Fprintf(out, "\nEverything already matches %s.\n", manifest.FileName)
	}
}
