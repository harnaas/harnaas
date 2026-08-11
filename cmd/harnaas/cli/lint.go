package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/jsonutil"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/paths"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source/github"
)

const lintLong = `Check that what is installed still agrees with harnaas.json and
harnaas.lock.json, and that nothing has moved on upstream.

lint changes nothing. It reports what is wrong and the command or edit that
resolves it, leaving harnaas install as the single repair path.

The only state that passes is one where every source is pinned to a tag or a
commit and is current. An available update is an error, not a warning, and no
flag downgrades one — a warning CI tolerates indefinitely does not keep a
team's configuration current.

Exit status is 0 when nothing is wrong, 2 when there are error-severity
findings, and 1 when lint itself could not run.`

// lintOptions holds the flags `harnaas lint` accepts, each registered locally
// on the command because the root carries no persistent flags.
type lintOptions struct {
	// frozen checks the manifest against the lockfile alone: no installed file
	// is read and no request is made.
	frozen bool

	// offline runs every local check and skips every network one.
	offline bool

	// strict promotes warnings to errors for the exit status only.
	strict bool

	// refresh bypasses cached resolution results.
	refresh bool

	// asJSON emits the findings as a single document on stdout.
	asJSON bool
}

// newLintCmd builds `harnaas lint`.
func newLintCmd() *cobra.Command {
	opts := &lintOptions{}

	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Check the installed assets against " + manifest.FileName,
		Long:  lintLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLint(cmd, opts)
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&opts.frozen, "frozen", false,
		"check the manifest against the lockfile alone, reading no installed file and making no request")
	flags.BoolVar(&opts.offline, "offline", false, "run every local check and skip every network one")
	flags.BoolVar(&opts.strict, "strict", false, "promote warnings to errors for the exit status")
	flags.BoolVar(&opts.refresh, "refresh", false, "ignore cached resolution results")
	flags.BoolVar(&opts.asJSON, "json", false, "emit the findings as a single JSON document")

	return cmd
}

// runLint performs every check the flags allow and reports what it found.
//
// Nothing here writes. That is the command's whole contract, and it is why the
// remedy on every finding is a command to run rather than something lint offers
// to do: install is the single repair path, so there is exactly one place where
// a mistake can destroy work.
func runLint(cmd *cobra.Command, opts *lintOptions) error {
	ctx := cmd.Context()

	root, err := paths.ProjectRoot(ctx)
	if err != nil {
		return err
	}

	report := &lintReport{}

	document, err := manifest.Load(ctx)
	if err != nil {
		// A manifest that will not load is one finding and suppresses every
		// check that depends on it. Reporting it and then reporting every asset
		// as uninstalled would bury the one problem under its own consequences.
		report.Findings = append(report.Findings, finding{
			Severity: severityError,
			Problem:  err.Error(),
			Remedy:   fmt.Sprintf("Correct %s, then run `harnaas lint` again.", manifest.FileName),
		})
		return finishLint(cmd, report, opts)
	}

	interpretation, err := manifest.Interpret(document)
	if err != nil {
		report.Findings = append(report.Findings, finding{
			Severity: severityError,
			Problem:  err.Error(),
			Remedy:   fmt.Sprintf("Correct %s, then run `harnaas lint` again.", manifest.FileName),
		})
		return finishLint(cmd, report, opts)
	}

	recorded, err := loadLock(root)
	if err != nil {
		// An unreadable lockfile is lint failing to run rather than a finding:
		// there is nothing to check against, and reporting "nothing is
		// installed" would be a conclusion drawn from a file harnaas could not
		// read.
		return err
	}

	report.Findings = append(report.Findings, lintChecks(ctx, root, interpretation, recorded, opts, report)...)

	// Whatever did not run is named, including on a clean report. A run that
	// checked less than it could and said nothing would let a green CI job mean
	// "current" when it only means "nothing local was wrong".
	switch {
	case opts.frozen:
		report.Skipped = append(report.Skipped, "installed file integrity", "update detection")
	case opts.offline:
		report.Skipped = append(report.Skipped, "update detection over the network")
	}

	return finishLint(cmd, report, opts)
}

// lintChecks runs the checks the flags allow, in a fixed order.
//
// Frozen mode stops after the checks that read only the manifest and the
// lockfile. That is what makes it usable on a fresh clone where nothing has
// been installed: it asks whether the lockfile still satisfies the manifest,
// which is a question about two committed files and nothing else.
func lintChecks(ctx context.Context, root string, interpretation *manifest.Interpretation, recorded *lockDocument, opts *lintOptions, report *lintReport) []finding {
	declared := interpretation.Assets

	var findings []finding
	findings = append(findings, checkNotReproducible(declared, interpretation.Sources)...)
	findings = append(findings, checkRefDisagreement(declared, interpretation.Sources, recorded)...)

	if opts.frozen {
		findings = append(findings, checkDeclaredButNotInstalled(declared, recorded)...)
		findings = append(findings, checkRecordedButUndeclared(declared, recorded)...)
		return findings
	}

	if collapsed, nothing := checkNothingInstalled(declared, recorded); nothing {
		// Everything below would report the same absence once per asset.
		return append(findings, collapsed)
	}

	findings = append(findings, checkDeclaredButNotInstalled(declared, recorded)...)
	findings = append(findings, checkRecordedButUndeclared(declared, recorded)...)
	findings = append(findings, checkInstalledIntegrity(root, recorded)...)

	// Local-source change detection runs whether or not the network is
	// allowed, because it needs none: the source is a file in this repository.
	findings = append(findings, checkLocalSources(ctx, interpretation, recorded)...)
	if !opts.offline {
		upstream, unchecked := checkUpstream(ctx, interpretation, recorded, github.ResolveRef, github.ListTags)
		findings = append(findings, upstream...)
		report.Unchecked = append(report.Unchecked, unchecked...)
	}

	findings = append(findings, checkUnmanagedConflict(root, interpretation, recorded)...)
	findings = append(findings, checkManagedBlocks(root, recorded)...)
	findings = append(findings, checkIgnoreBlock(root, recorded)...)
	findings = append(findings, checkBridgeLine(root, recorded)...)
	return findings
}

// finishLint writes the report and returns the error that decides the exit
// status.
func finishLint(cmd *cobra.Command, report *lintReport, opts *lintOptions) error {
	report.sort()

	if err := writeLintReport(cmd, report, opts); err != nil {
		return err
	}

	errors, warnings := report.counts()
	if opts.strict {
		errors += warnings
	}
	if errors > 0 {
		return &LintFindingsError{Errors: errors}
	}
	return nil
}

// writeLintReport prints the findings, grouped by asset, or emits them as one
// JSON document.
func writeLintReport(cmd *cobra.Command, report *lintReport, opts *lintOptions) error {
	if opts.asJSON {
		encoded, err := jsonutil.Marshal(report)
		if err != nil {
			return fmt.Errorf("encode the lint report: %w", err)
		}
		if _, err := cmd.OutOrStdout().Write(encoded); err != nil {
			return fmt.Errorf("write the lint report: %w", err)
		}
		return nil
	}

	out := cmd.OutOrStdout()

	group := ""
	for _, found := range report.Findings {
		subject := found.Asset
		if subject == "" {
			subject = "project"
		}
		if subject != group {
			fmt.Fprintf(out, "\n%s\n", subject)
			group = subject
		}
		fmt.Fprintf(out, "  %s: %s\n", found.Severity, found.Problem)
		fmt.Fprintf(out, "    %s\n", found.Remedy)
	}

	errors, warnings := report.counts()
	fmt.Fprintln(out)
	if errors == 0 && warnings == 0 {
		fmt.Fprintln(out, "Everything installed agrees with the manifest and the lockfile.")
	} else {
		fmt.Fprintf(out, "%d %s, %d %s\n",
			errors, plural(errors, "error", "errors"), warnings, plural(warnings, "warning", "warnings"))
	}

	// Stated even on a clean run. A report that checked less than it could and
	// said nothing would let a green CI job mean "current" when it only means
	// "nothing local was wrong".
	for _, skipped := range report.Skipped {
		advisef(cmd, "Skipped: %s.\n", skipped)
	}
	for _, unchecked := range report.Unchecked {
		advisef(cmd, "Unchecked: %s.\n", unchecked)
	}

	return nil
}
