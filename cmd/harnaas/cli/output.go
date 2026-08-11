package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// The output contract, stated once for every command in this package.
//
// stdout carries the result and nothing else: the text a person reads, or —
// under --json — the document alone, so `harnaas … --json | jq` never chokes on
// a progress line that happened to land mid-stream. stderr carries everything
// advisory: progress, warnings, and any explanation of what the command chose
// to do. Both are reached through the command's own writers, never through the
// process globals, so a test can capture them.
//
// Cobra's Print* helpers write to OutOrStderr — stderr in production — and so
// would send a command's result to the wrong stream. forbidigo rejects them.

// printJSON writes v to stdout as the command's entire result.
//
// Indented, because a person reads this output as often as a script does.
// HTML left unescaped, because Go's default would rewrite an ampersand or an
// angle bracket in a path, an asset id or a source ref into an escape that no
// consumer asked for. Newline-terminated, so the document ends the stream
// cleanly for a line-oriented reader.
func printJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode json output: %w", err)
	}
	return nil
}

// advisef writes progress, warning or explanatory text to stderr. None of it is
// part of the result, so it stays off stdout whether or not --json was
// requested. The caller supplies the trailing newline.
func advisef(cmd *cobra.Command, format string, args ...any) {
	fmt.Fprintf(cmd.ErrOrStderr(), format, args...)
}
