package cli

import "github.com/spf13/cobra"

// addJSONFlag registers --json locally on a command that can render its result
// as a machine-readable document.
//
// Local, never persistent. A persistent --json would be inherited — and
// silently ignored — by every side-effecting verb, and cobra offers no way to
// hide a persistent flag from a subset of children. Accepting a flag and
// honouring it have to be the same act, so the one-line registration sits on
// exactly the commands that honour it, and `harnaas <verb> --json` fails as an
// unknown flag everywhere else. Read it back with jsonRequested.
func addJSONFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("json", false, "Write the result to stdout as a JSON document")
}

// jsonRequested reports whether --json was requested on cmd.
//
// A command that never registered the flag answers "not requested" rather than
// panicking: the lookup failing means the flag is undeclared on this command
// tree, which is a truthful no. That lets shared rendering helpers ask
// unconditionally instead of every caller tracking whether its command has a
// JSON view.
func jsonRequested(cmd *cobra.Command) bool {
	requested, err := cmd.Flags().GetBool("json")
	return err == nil && requested
}
