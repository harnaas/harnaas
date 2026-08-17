package cli

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/jsonutil"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/logging"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/paths"
)

const initLong = `Create harnaas.json at the project root, declaring which harnesses this
project targets, and the .harnaas directory its own assets live in.

Which harnesses a project targets is a guarantee it publishes about itself, so
init asks rather than guesses: on a terminal it lists every harness harnaas
recognizes and you choose. Pass --harness to name them instead, repeating the
flag per harness — which is how a CI job or a coding agent runs init, and what
a run with no terminal requires.

Beneath .harnaas, init creates one directory per asset type your selection can
actually receive, each explaining what belongs in it. That directory is yours:
harnaas only ever reads it, and init only ever adds to it.

Nothing else is touched. The harness directories, the memory file and any
ignore-file entries belong to harnaas install, which records what it created;
anything init created there would be unmanaged, and the next install would
report a conflict against init's own output.`

// manifestPerm is the mode a scaffolded manifest is created with: a committed,
// hand-edited file, so the ordinary non-executable default.
const manifestPerm fs.FileMode = 0o644

// initOptions holds the flags `harnaas init` accepts. Each is registered
// locally on the command, because the root carries no persistent flags.
type initOptions struct {
	// force allows replacing an existing manifest. It reaches the manifest and
	// nothing else: the local asset scaffolding only ever adds, and no flag
	// makes it do otherwise.
	force bool

	// harnesses is the raw flag input, unvalidated: the strings the user typed,
	// so a name the roster rejects can be quoted back as written.
	harnesses []string
}

// newInitCmd builds `harnaas init`.
func newInitCmd() *cobra.Command {
	opts := &initOptions{}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create harnaas.json for this project",
		Long:  initLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInit(cmd, opts)
		},
	}

	flags := cmd.Flags()
	flags.BoolVar(&opts.force, "force", false,
		"replace an existing "+manifest.FileName)
	// A repeated flag rather than a comma-separated list: a harness id is one
	// token, and splitting on commas would turn `--harness "a, b"` into a name
	// with a leading space and a diagnostic about whitespace nobody typed
	// deliberately.
	//
	// There is deliberately no flag that accepts a selection nobody made. This
	// one is how a run without a terminal names its harnesses, and it is the
	// whole of that path.
	flags.StringArrayVar(&opts.harnesses, "harness", nil,
		"harness to target; repeat for each one")

	return cmd
}

// runInit scaffolds the manifest and the project's local asset directory.
//
// The order of the steps is the contract. Everything that can refuse happens
// before anything is written and before the prompt — asking a question whose
// answer cannot change the outcome wastes the one moment the user is paying
// attention — and the manifest is written before the scaffolding, because asset
// directories with no manifest declaring what they are for are scaffolding for
// nothing, while a manifest with no directories is a complete initialization.
func runInit(cmd *cobra.Command, opts *initOptions) error {
	ctx := cmd.Context()

	root, err := paths.ProjectRoot(ctx)
	if err != nil {
		return err
	}
	path, err := manifest.Path(ctx)
	if err != nil {
		return err
	}

	if err := refuseExistingManifest(path, opts.force); err != nil {
		return err
	}

	targets, err := selectHarnesses(ctx, cmd.InOrStdin(), cmd.ErrOrStderr(), opts.harnesses)
	if err != nil {
		return err
	}

	if err := writeScaffold(path, targets); err != nil {
		return err
	}
	logging.Info(ctx, "manifest created",
		slog.String("path", path),
		slog.Int("harness_count", len(targets)),
	)
	reportCreated(cmd, path, targets)

	scaffolded, err := scaffoldLocalAssets(root, targets, adapter.Default)
	// The report comes before the error is returned either way: a partial
	// scaffolding still created directories, and a reader told only that
	// something failed would not know which half of it happened.
	reportScaffolded(cmd, scaffolded)
	if err != nil {
		return err
	}
	logging.Info(ctx, "local asset scaffolding created",
		slog.Int("created_count", len(scaffolded.Created)),
		slog.Int("existing_count", len(scaffolded.Existing)),
	)

	reportNextSteps(cmd)

	return nil
}

// scaffold is the manifest init writes.
//
// It is a type of its own rather than [manifest.Document] because the two answer
// different questions: Document is what decoding produces and carries fields
// that are not part of the file, while this is the file. The field order here is
// the order they appear in the written manifest, which is the order a person
// reads them in — what this targets, where content comes from, what is wanted.
type scaffold struct {
	Version   int                `json:"version"`
	Harnesses []harness.ID       `json:"harnesses"`
	Sources   map[string]string  `json:"sources"`
	Assets    []assetPlaceholder `json:"assets"`
}

// assetPlaceholder is the element type of an empty `assets` array. The array is
// always empty here — init declares no assets, because it has nothing to declare
// them from — so the type exists only to keep the field from being `any`.
type assetPlaceholder struct{}

// newScaffold builds the document for the selected harnesses.
//
// `sources` and `assets` are empty rather than absent, and non-nil rather than
// nil, so the written file shows the author both fields with the shape their
// content goes in. A manifest that omitted them would decode identically and
// teach nothing.
func newScaffold(harnesses []harness.ID) scaffold {
	return scaffold{
		Version:   manifest.SupportedVersion,
		Harnesses: harnesses,
		Sources:   map[string]string{},
		Assets:    []assetPlaceholder{},
	}
}

// writeScaffold encodes the manifest and puts it on disk in one step.
//
// The write goes through jsonutil so the file is staged, synced and renamed into
// place: a forced run over an existing manifest either replaces it completely or
// leaves the previous one intact, and never leaves a staging file behind. The
// document is encoded fully before the write starts, so an unencodable value
// fails with nothing touched.
func writeScaffold(path string, harnesses []harness.ID) error {
	document, err := jsonutil.Marshal(newScaffold(harnesses))
	if err != nil {
		return fmt.Errorf("encode %s: %w", manifest.FileName, err)
	}
	if err := jsonutil.WriteFileAtomic(path, document, manifestPerm); err != nil {
		return fmt.Errorf("write %s: %w", manifest.FileName, err)
	}
	return nil
}

// manifestExistsError refuses to replace a manifest that is already there.
//
// The manifest is the file a team reviews, and init is the one command allowed
// to write it. Replacing one silently would discard declarations nobody asked
// harnaas to touch, so the refusal is the default and the flag that lifts it is
// named in the message.
type manifestExistsError struct {
	// Path is the manifest left untouched.
	Path string
}

func (e *manifestExistsError) Error() string {
	return fmt.Sprintf(
		"a manifest already exists at %s\n\n"+
			"Edit it directly, or re-run with --force to replace it with a fresh one.",
		e.Path,
	)
}

// refuseExistingManifest checks for a manifest already at the project root.
//
// A stat that fails for any reason other than "not there" is treated as "not
// there" deliberately: the write is the authority on whether the file can be
// replaced, and guessing from a failed stat would refuse a run that would have
// succeeded, with a message about a file harnaas could not even see.
func refuseExistingManifest(path string, force bool) error {
	if force {
		return nil
	}
	if _, err := os.Lstat(path); err == nil {
		return &manifestExistsError{Path: path}
	}
	return nil
}

// reportCreated names the manifest and what it targets, on stdout.
func reportCreated(cmd *cobra.Command, path string, targets []harness.ID) {
	fmt.Fprintf(cmd.OutOrStdout(), "Created %s, targeting %s\n", path, displayNames(targets))
}

// reportScaffolded names the local asset directories this run created.
//
// Only the created ones are named. A directory that was already there is the
// author's, possibly with content in it, and a run claiming to have made it is a
// run they will not look at again. A run that created none says so, because
// silence there reads as a run that did nothing.
func reportScaffolded(cmd *cobra.Command, scaffolded scaffoldResult) {
	out := cmd.OutOrStdout()

	if len(scaffolded.Created) == 0 {
		if len(scaffolded.Existing) > 0 {
			fmt.Fprintf(out, "\n%s was already laid out; nothing was added to it.\n", manifest.LocalRoot)
		}
		return
	}

	fmt.Fprintf(out, "\nCreated %s for this project's own assets:\n", manifest.LocalRoot)
	for _, directory := range scaffolded.Created {
		if directory == manifest.LocalRoot {
			continue
		}
		fmt.Fprintf(out, "  %s\n", directory)
	}
	fmt.Fprintf(out, "Each one holds a %s saying what belongs in it. They are yours to edit.\n",
		scaffoldExplanation)
}

// reportNextSteps prints what to do next, on stdout.
//
// The remaining setup is described rather than performed, and the description
// names only what the named command actually does. There is deliberately no flag
// that makes init do any of it.
func reportNextSteps(cmd *cobra.Command) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "\nNext: declare the assets you want in %s, then run `harnaas install`.\n",
		manifest.FileName)
	fmt.Fprintf(out, "`harnaas install` writes into the harness directories and maintains the\n")
	fmt.Fprint(out, "ignore-file entries for what it installed. init wrote none of them.\n")
}
