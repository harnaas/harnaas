package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/jsonutil"
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
// The document's shape — indented, HTML unescaped, newline-terminated — is
// jsonutil.Marshal's, and is shared with the manifest init writes so that the
// same document read from a pipe and from disk looks the same.
//
// Encoding fully before writing anything is what makes the failure case safe: a
// value that cannot be encoded returns an error with stdout still empty, where
// streaming an encoder straight at the terminal would leave a half-written
// document ahead of the entrypoint's error message.
func printJSON(cmd *cobra.Command, v any) error {
	doc, err := jsonutil.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode json output: %w", err)
	}

	if _, err := cmd.OutOrStdout().Write(doc); err != nil {
		return fmt.Errorf("write json output: %w", err)
	}

	return nil
}

// advisef writes progress, warning or explanatory text to stderr. None of it is
// part of the result, so it stays off stdout whether or not --json was
// requested. The caller supplies the trailing newline.
func advisef(cmd *cobra.Command, format string, args ...any) {
	fmt.Fprintf(cmd.ErrOrStderr(), format, args...)
}
