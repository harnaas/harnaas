package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/interactive"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/jsonutil"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/logging"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/paths"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/uiform"
)

const initLong = `Create harnaas.json at the project root, declaring which harnesses this
project targets.

init writes that one file and nothing else. The harness directories, the
.harnaas directory and any ignore-file entries belong to harnaas install,
which records what it created; anything init created would be unmanaged, and
the next install would report a conflict against init's own output.

The harnesses a project already uses are detected from what is in the
repository and pre-filled. Pass --harness to name them yourself instead.`

// manifestPerm is the mode a scaffolded manifest is created with: a committed,
// hand-edited file, so the ordinary non-executable default.
const manifestPerm fs.FileMode = 0o644

// initOptions holds the flags `harnaas init` accepts. Each is registered
// locally on the command, because the root carries no persistent flags.
type initOptions struct {
	// force allows replacing an existing manifest.
	force bool

	// assumeYes accepts the pre-filled selection without prompting, so a user
	// on a terminal can take the non-interactive path deliberately.
	assumeYes bool

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
	flags.BoolVarP(&opts.assumeYes, "yes", "y", false,
		"accept the harness selection without prompting")
	// A repeated flag rather than a comma-separated list: a harness id is one
	// token, and splitting on commas would turn `--harness "a, b"` into a name
	// with a leading space and a diagnostic about whitespace nobody typed
	// deliberately.
	flags.StringArrayVar(&opts.harnesses, "harness", nil,
		"harness to target; repeat for each one, overriding detection")

	return cmd
}

// runInit scaffolds the manifest.
//
// The order of the steps is the contract: everything that can refuse happens
// before anything is written, and before the prompt too — asking a question
// whose answer cannot change the outcome wastes the one moment the user is
// paying attention.
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

	selection, err := selectHarnesses(root, opts.harnesses)
	if err != nil {
		return err
	}
	if err := refuseExistingManifest(path, opts.force); err != nil {
		return err
	}

	explainSelection(cmd, selection)

	confirmed, err := confirmSelection(cmd, selection, opts.assumeYes)
	if err != nil {
		return err
	}
	if !confirmed {
		// A decline is an answer, not a failure: the user was asked and said
		// no, and harnaas did exactly that. Only a cancelled prompt — the user
		// walking away from a question they never chose to answer — exits
		// non-zero.
		advisef(cmd, "Nothing was created. Re-run with --harness to name the harnesses yourself.\n")
		return nil
	}

	if err := writeScaffold(path, selection.Harnesses); err != nil {
		return err
	}

	logging.Info(ctx, "manifest created",
		slog.String("path", path),
		slog.Int("harness_count", len(selection.Harnesses)),
	)
	reportCreated(cmd, path)

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

// explainSelection says where the selection came from, on stderr, before the
// user is asked to confirm it.
//
// A flag-supplied selection gets no sentence: the user typed it a moment ago,
// and repeating it back is noise. The other two are worth stating — detection
// can be wrong in both directions, and the default is the one case where harnaas
// picked a harness the project shows no sign of using.
func explainSelection(cmd *cobra.Command, selection harnessSelection) {
	switch selection.Origin {
	case originFlag:
		// Nothing to say: the selection is the user's own words.
	case originDetected:
		advisef(cmd, "Detected %s in this project.\n", displayNames(selection.Harnesses))
	case originDefault:
		advisef(cmd, "No supported harness detected in this project; targeting %s.\n",
			displayNames(selection.Harnesses))
	}
}

// confirmSelection asks the user to confirm the selection, where asking is
// possible and was not waived.
//
// Not asking is the normal case, not a degraded one: a run with no terminal, a
// run inside a coding agent and a run with --yes all proceed from the detected
// and flag-supplied values, because every workflow here has to be completable
// without a prompt. The prompt renders on stderr so a confirmed run's stdout
// still carries the result alone.
func confirmSelection(cmd *cobra.Command, selection harnessSelection, assumeYes bool) (bool, error) {
	in, out := cmd.InOrStdin(), cmd.ErrOrStderr()
	if assumeYes || !interactive.CanPrompt(in, out) {
		return true, nil
	}

	question := fmt.Sprintf("Create %s targeting %s?",
		manifest.FileName, displayNames(selection.Harnesses))

	confirmed, err := uiform.Confirm(cmd.Context(), in, out, question, true)
	if errors.Is(err, uiform.ErrCancelled) {
		// Cancelling is not declining. The user answered nothing, so harnaas
		// does nothing and says so with a non-zero exit, rather than taking the
		// default answer on behalf of someone who walked away.
		return false, fmt.Errorf("%w: no %s was created", err, manifest.FileName)
	}
	if err != nil {
		return false, fmt.Errorf("confirm harness selection: %w", err)
	}

	return confirmed, nil
}

// reportCreated prints what init did and what to do next, on stdout.
//
// The remaining setup is described rather than performed, and the description
// names the command that performs it. There is deliberately no flag that makes
// init do any of it.
func reportCreated(cmd *cobra.Command, path string) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Created %s\n", path)
	fmt.Fprintf(out, "\nNext: declare the assets you want in %s, then run `harnaas install`.\n",
		manifest.FileName)
	fmt.Fprintf(out, "`harnaas install` creates %s/, writes into the harness directories and\n",
		manifest.LocalRoot)
	fmt.Fprint(out, "maintains the ignore-file entries for what it installed. init wrote none of them.\n")
}
