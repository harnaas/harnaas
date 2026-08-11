package cli

import (
	"fmt"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/versioninfo"
	"github.com/spf13/cobra"
)

const rootLong = `harnaas manages a project's AI-harness assets as a declared, versioned
dependency: harnaas.json declares them, harnaas install places them, and
harnaas lint verifies them.`

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

	return cmd
}
