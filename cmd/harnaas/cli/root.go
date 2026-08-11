package cli

import (
	"fmt"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/versioninfo"
	"github.com/spf13/cobra"
)

const rootLong = `harnaas manages a project's AI-harness assets as a declared, versioned
dependency: harnaas.json declares them, harnaas install places them, and
harnaas lint verifies them.`

// The help group ids. A group exists to answer "where do I start?" before
// "what else is there?", which is why setup comes first.
const (
	groupSetup  = "setup"
	groupAssets = "assets"
)

// helpGroups declares every help group in display order: getting a project
// declared, then the verbs that act on what it declares. addCommand declares a
// group on the root the first time a command files itself under it, so the
// order commands are attached in NewRootCmd must follow this list — cobra
// renders group headers in declaration order, and it prints a header for a
// declared group whether or not the group has any members.
var helpGroups = []cobra.Group{
	{ID: groupSetup, Title: "Setup Commands:"},
	{ID: groupAssets, Title: "Asset Commands:"},
}

// NewRootCmd builds the harnaas command tree.
//
// Cobra's own error and usage printing is silenced here so the process
// entrypoint is the only component that renders an error or chooses an exit
// code. The root deliberately declares no persistent flags: a flag that applies
// to some commands is registered locally on each command that honours it, so
// accepting a flag and honouring it are the same act.
//
// Subcommands are attached by explicit constructor calls from this function.
// Nothing registers itself from a package init, so the tree is readable in one
// place.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "harnaas",
		Short: "Manage a project's AI-harness assets as a versioned dependency",
		Long:  rootLong,
		// Version must be read after versioninfo.Load(), which the entrypoint
		// calls before building the tree.
		Version:       versioninfo.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
		// The root itself takes no positional arguments, so an unrecognized
		// verb is reported as an unknown command rather than swallowed. Cobra
		// only infers that once subcommands exist; stating it keeps the
		// behaviour true of an empty tree too.
		Args: cobra.NoArgs,
		// The generated completion command stays functional but out of help.
		CompletionOptions: cobra.CompletionOptions{HiddenDefaultCmd: true},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Help(); err != nil {
				return fmt.Errorf("render help: %w", err)
			}
			return nil
		},
	}

	cmd.SetVersionTemplate(fmt.Sprintf("harnaas %s (%s)\n", versioninfo.Version, versioninfo.Commit))

	// Every subcommand is attached here, by an explicit constructor call, in
	// helpGroups order. Nothing registers itself from a package init — the
	// gochecknoinits linter forbids one, and TestPackageDeclaresNoInit asserts
	// it over the sources — so this function is the entire command tree, and
	// reading it is enough to know what harnaas accepts.
	//
	// The commented lines are the commands their own changes attach here, kept
	// in place so the order stays visible: a group's header is rendered where
	// its first command was attached, so install and lint must land after init.
	addCommand(cmd, newInitCmd(), groupSetup)
	addCommand(cmd, newInstallCmd(), groupAssets)
	//	addCommand(cmd, newLintCmd(), groupAssets)     // add-harnaas-lint

	return cmd
}

// addCommand attaches child to parent under the named help group, declaring
// the group on parent the first time a command uses it. It is the only way a
// command joins the tree.
//
// An unknown group id is a wiring mistake in this file, not a runtime
// condition, and cobra itself panics at execute time for a command filed under
// an undeclared group. Panicking here instead surfaces the typo at the point
// that made it, naming both the command and the group.
func addCommand(parent, child *cobra.Command, groupID string) {
	title, ok := helpGroupTitle(groupID)
	if !ok {
		panic(fmt.Sprintf("command %q filed under unknown help group %q", child.Name(), groupID))
	}
	if !parent.ContainsGroup(groupID) {
		parent.AddGroup(&cobra.Group{ID: groupID, Title: title})
	}
	child.GroupID = groupID
	parent.AddCommand(child)
}

// helpGroupTitle returns the display title declared for a help group id.
func helpGroupTitle(id string) (string, bool) {
	for _, g := range helpGroups {
		if g.ID == id {
			return g.Title, true
		}
	}
	return "", false
}
