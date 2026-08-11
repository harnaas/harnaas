// Package interactive answers one question: may harnaas stop and ask?
//
// The answer is resolved from the streams a command was handed and from the
// environment it is running in, and it is deliberately biased towards "no". A
// prompt shown to something that cannot answer it does not degrade — it hangs,
// and a hung CI job or a hung coding agent is the failure mode this package
// exists to prevent. Every workflow is completable without a terminal, so
// answering "no" always leaves a working path; answering "yes" wrongly does not.
//
// The stream test asks whether the command's own input and output are terminals
// rather than probing a controlling terminal directly. That keeps the question
// answerable on every platform harnaas ships to, and keeps it honest under
// redirection: `harnaas init > out.txt` has a terminal attached to the process
// and still must not prompt.
package interactive

import (
	"io"
	"os"
	"testing"

	"golang.org/x/term"
)

// EnvTestTTY forces the prompting decision, so a test can exercise both answers
// without a terminal. Set it to "1" to force prompting on; any other non-empty
// value forces it off. It is read before every other gate, including the stream
// test, and unset it is as if it were never there.
const EnvTestTTY = "HARNAAS_TEST_TTY"

// EnvNoColor and EnvTerm gate styling. NO_COLOR is honoured per
// https://no-color.org: set to anything non-empty, harnaas emits no escape
// sequences at all.
const (
	EnvNoColor = "NO_COLOR"
	EnvTerm    = "TERM"
)

// CanPrompt reports whether harnaas may show an interactive prompt reading from
// in and rendering on out. Pass the command's own streams — cmd.InOrStdin() and
// cmd.ErrOrStderr() — not the process ones, so a test driving a command through
// buffers gets the same answer the buffers deserve.
//
// The gates, first match wins:
//
//  1. EnvTestTTY, when set, is the whole answer.
//  2. Running under `go test`, the answer is no. A developer's terminal is a
//     real terminal, and an in-process test that reached a prompt would block
//     there rather than fail.
//  3. An agent sentinel in the environment, the answer is no. A coding agent
//     runs harnaas as a subprocess and often hands it the agent's own terminal,
//     which no human is watching.
//  4. CI set to anything but "false", the answer is no. Self-hosted runners do
//     attach terminals, so the stream test alone would let a prompt through and
//     hang the job.
//  5. Otherwise: both streams must be terminals.
func CanPrompt(in io.Reader, out io.Writer) bool {
	return canPrompt(os.Getenv, testing.Testing(), IsTerminalReader(in) && IsTerminalWriter(out))
}

// canPrompt is the pure decision behind CanPrompt, split out because `go test`
// can never present a real terminal: driving the environment gates through the
// exported function would short-circuit at the testing.Testing() gate and every
// assertion after it would pass without being reached.
func canPrompt(getenv func(string) string, underTest, streamsAreTerminals bool) bool {
	if v := getenv(EnvTestTTY); v != "" {
		return v == "1"
	}
	if underTest {
		return false
	}
	if isAgentEnv(getenv) {
		return false
	}
	// CI=<non-empty> is the convention every provider follows. CI=false is the
	// escape hatch a developer uses to override a value they inherited.
	if v := getenv("CI"); v != "" && v != "false" {
		return false
	}
	return streamsAreTerminals
}

// agentEnvVars are the environment variables coding agents set on the
// subprocesses they spawn. Each is a vendor's own sentinel, and a non-empty
// value is the whole signal — the values themselves are not standardised.
//
// This list is expected to grow and is expected to be incomplete. It is a
// refinement over the stream test, not a replacement for it: an agent harnaas
// does not recognise still has to fail the stream test or pass a flag, which is
// why no workflow may depend on a prompt in the first place.
var agentEnvVars = []string{
	"CLAUDECODE",      // Claude Code
	"CURSOR_AGENT",    // Cursor's agent CLI
	"GEMINI_CLI",      // Gemini CLI's shell tool
	"COPILOT_CLI",     // GitHub Copilot CLI
	"PI_CODING_AGENT", // Pi Coding Agent's shell tool
}

// isAgentEnv reports whether the environment names a coding agent that spawned
// this process.
func isAgentEnv(getenv func(string) string) bool {
	for _, name := range agentEnvVars {
		if getenv(name) != "" {
			return true
		}
	}
	return false
}

// IsTerminalReader reports whether r is an *os.File backed by a terminal. A
// reader that is not a file — a buffer, a pipe, a strings.Reader — is not one,
// which is what makes a piped run detectable.
func IsTerminalReader(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd())) //nolint:gosec // G115: a file descriptor always fits in an int
}

// IsTerminalWriter reports whether w is an *os.File backed by a terminal. Use
// it for writer-scoped presentation decisions; for "may I ask the user?" use
// CanPrompt, which also weighs the environment.
func IsTerminalWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd())) //nolint:gosec // G115: a file descriptor always fits in an int
}

// ShouldStyle reports whether styled output may be written to w. It is the
// single gate in front of the palette: nothing else decides for itself whether
// colour is appropriate.
//
// This is a separate question from CanPrompt. A run inside a coding agent must
// not be prompted and may still be styled, because the agent renders the
// escape sequences it captures.
func ShouldStyle(w io.Writer) bool {
	return shouldStyle(os.Getenv(EnvNoColor), os.Getenv(EnvTerm), IsTerminalWriter(w))
}

// shouldStyle is the pure decision behind ShouldStyle, split out for the same
// reason as canPrompt: the terminal test can never be true under `go test`, so
// the gates in front of it are only reachable with the answer supplied.
func shouldStyle(noColor, termName string, isTerminalWriter bool) bool {
	if noColor != "" {
		return false
	}
	if termLacksANSI(termName) {
		return false
	}
	return isTerminalWriter
}

// termLacksANSI reports whether termName names a console that does not handle
// ANSI escape sequences. TERM=cygwin is the case worth naming: it renders the
// escape byte as a literal glyph, so styled output arrives as visible garbage
// like "←[32m" rather than as colour.
func termLacksANSI(termName string) bool {
	return termName == "cygwin"
}
