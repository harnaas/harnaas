package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/paths"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

const installLong = `Make the filesystem match harnaas.json.

install resolves every declared source, computes the whole plan, and only then
writes anything — so a source that fails to resolve leaves every harness
directory untouched.

It never overwrites a path it did not create. A destination the lockfile does
not record is reported as a conflict and left alone, on every flag, --force
included; --force restores destinations harnaas installed and somebody has
since edited.

Removing an asset from harnaas.json removes its installed files on the next
install, so emptying the assets array and installing is the full uninstall.`

// installOptions holds the flags `harnaas install` accepts, each registered
// locally on the command because the root carries no persistent flags.
type installOptions struct {
	// dryRun computes the whole plan and writes nothing.
	dryRun bool

	// force restores drifted managed destinations, and only those.
	force bool

	// offline resolves from the cache and makes no network request.
	offline bool

	// noCache bypasses the archive cache for this run.
	noCache bool

	// asJSON emits the report as a single document on stdout.
	asJSON bool
}

// newInstallCmd builds `harnaas install`.
func newInstallCmd() *cobra.Command {
	opts := &installOptions{}

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the assets declared in " + manifest.FileName,
		Long:  installLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInstall(cmd, opts)
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&opts.dryRun, "dry-run", false, "compute the plan and write nothing")
	flags.BoolVar(&opts.force, "force", false, "restore destinations harnaas installed that have since been edited")
	flags.BoolVar(&opts.offline, "offline", false, "resolve from the cache and make no network request")
	flags.BoolVar(&opts.noCache, "no-cache", false, "ignore the archive cache for this run")
	flags.BoolVar(&opts.asJSON, "json", false, "emit the report as a single JSON document")

	return cmd
}

// runInstall is the whole flow: resolve, plan, apply, record.
//
// The phases are ordered so that everything which can fail without touching the
// project happens first. Resolution reaches the network and the filesystem but
// writes to neither destination nor lockfile, so a source that fails leaves the
// harness exactly as it was — which is what makes a failed install safe to
// re-run rather than something to clean up after.
func runInstall(cmd *cobra.Command, opts *installOptions) error {
	ctx := cmd.Context()

	root, err := paths.ProjectRoot(ctx)
	if err != nil {
		return err
	}

	document, err := manifest.Load(ctx)
	if err != nil {
		return err
	}
	interpretation, err := manifest.Interpret(document)
	if err != nil {
		return err
	}

	// The lock covers the lockfile's read-modify-write. A dry run takes it too:
	// it reads the lockfile to decide what is managed, and a concurrent install
	// rewriting it underneath would make the plan describe a state that never
	// existed.
	release, err := acquireInstallLock(root)
	if err != nil {
		return err
	}
	defer release()

	recorded, err := loadLock(root)
	if err != nil {
		return err
	}

	resolutions, report := resolveAssets(ctx, interpretation, opts)

	plan, err := planInstall(root, resolutions, recorded, opts)
	if err != nil {
		return err
	}
	report.Results = append(report.Results, plan.results...)
	for _, removal := range plan.removes {
		report.Removed = append(report.Removed, removal.Reported)
	}
	report.DryRun = opts.dryRun
	report.sort()

	if !opts.dryRun {
		if err := plan.apply(root, recorded); err != nil {
			return err
		}
	}

	if err := writeInstallReport(cmd, report, opts); err != nil {
		return err
	}

	if len(report.Failures) > 0 {
		// The assets that resolved are installed and recorded; the run still
		// fails, because a partial install reported as success is a project
		// whose harness is missing something nobody was told about.
		return NewAlreadyPrintedError(fmt.Errorf("%d of %d assets could not be installed",
			len(report.Failures), len(interpretation.Assets)))
	}
	return nil
}

// resolvedAsset is one asset that resolved, with the content it resolved to.
type resolvedAsset struct {
	Asset    manifest.Asset
	Resolved *source.Resolved
}

// resolveAssets resolves every declared asset, collecting failures rather than
// stopping at the first.
//
// A failure affecting one asset must not abort the others: a manifest with a
// typo in one entry should still install the eleven that are correct, and the
// reader should meet every problem in one run rather than one per run.
func resolveAssets(ctx context.Context, interpretation *manifest.Interpretation, opts *installOptions) ([]resolvedAsset, *installReport) {
	// A nil cache is the bypass, which is why --no-cache simply does not build
	// one: there is no flag on the cache saying "do not use me", so a run that
	// was told not to read it cannot accidentally be handed one.
	runOptions := source.RunOptions{Offline: opts.offline}
	if !opts.noCache {
		runOptions.Cache = source.NewArchiveCache()
	}
	resolver := source.Default.NewResolver(runOptions)

	report := &installReport{}
	resolutions := make([]resolvedAsset, 0, len(interpretation.Assets))

	for _, asset := range interpretation.Assets {
		request := source.Request{Asset: asset, Source: interpretation.Sources[asset.Ref.SourceKey]}

		resolved, err := resolver.Resolve(ctx, request)
		if err != nil {
			report.Failures = append(report.Failures, installFailure{AssetID: asset.ID, Problem: err.Error()})
			continue
		}
		if err := source.Verify(request, resolved); err != nil {
			report.Failures = append(report.Failures, installFailure{AssetID: asset.ID, Problem: err.Error()})
			continue
		}

		resolutions = append(resolutions, resolvedAsset{Asset: asset, Resolved: resolved})
	}

	return resolutions, report
}

// plannedWrite is one destination the plan will write, remove or leave alone.
type plannedWrite struct {
	Asset       manifest.Asset
	Plan        targetPlan
	Files       []source.File
	Digest      source.Digest
	Root        string
	Outcome     outcome
	Emulated    bool
	Renderer    adapter.Renderer
	SourceRef   source.Provenance
	InstalledAt time.Time
}

// installPlan is everything the run will do, computed before anything is
// written.
type installPlan struct {
	writes  []plannedWrite
	removes []plannedRemoval
	results []installResult

	// instructions is the memory file's assembled block, and instructionIDs the
	// assets that contributed, so the always-on threshold can name them.
	instructions []instruction

	// installedPaths is every project-scope destination, for the ignore block.
	installedPaths []string

	// assets is the lockfile the run will write.
	assets []lockAsset
}

// plannedRemoval is a managed destination convergence will delete.
type plannedRemoval struct {
	Scope       manifest.Scope
	Root        string
	Destination string

	// Reported is the path from the project root, which is the one the reader
	// can go and look at. Destination stays relative to the scope root because
	// that is what the lockfile recorded it as.
	Reported string
}
