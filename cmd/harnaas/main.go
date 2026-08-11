// Command harnaas manages a project's AI-harness assets as a declared,
// versioned dependency of the repository.
//
// This file owns the whole process contract: it is the only component that
// prints an error, the only one that chooses an exit code, and the only one
// that decides how a signal terminates the process. Commands return errors;
// what those errors look like on the terminal is decided here, once.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/harnaas/harnaas/cmd/harnaas/cli"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/logging"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/paths"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/uiform"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/versioninfo"
	"github.com/harnaas/harnaas/internal/procsignal"
	"github.com/spf13/cobra"
)

// The exit-code contract. Every status harnaas can produce is listed here, and
// a new one must not be introduced without a documented meaning.
//
//   - exitSuccess       the command completed and found nothing to report.
//   - exitFailure       any runtime failure: bad configuration, I/O, network.
//   - exitLintFindings  reserved: `lint` ran its checks to completion and
//     reported at least one error-severity finding.
//
// exitLintFindings exists so a CI job can tell "lint crashed" from "lint worked
// and your harness has drifted" — the distinction lint exists to make. It is
// reserved for that single meaning: no command other than `lint` may return it,
// and a lint run that fails partway through exits exitFailure like any other
// runtime failure. Nothing in this binary emits it yet; `lint` itself arrives
// with add-harnaas-lint, and reportError below never produces it.
//
// Termination by signal is not in this list because it is not an exit at all:
// see dieFromSignal, which re-raises the signal and only falls back to
// 128-plus-signal-number where re-raising is unsupported.
const (
	exitSuccess      = 0
	exitFailure      = 1
	exitLintFindings = 2
)

// forceQuitGrace is how long dieFromSignal waits for a re-raised signal to be
// delivered. Delivery ends the process well before this elapses; the wait only
// exists so a platform that silently drops the re-raise still reaches the
// fallback exit instead of hanging.
const forceQuitGrace = 500 * time.Millisecond

func main() {
	// Resolve version and commit before anything reads them — the root command
	// copies Version into its own field as it is built.
	versioninfo.Load()

	os.Exit(run())
}

// run executes the command tree and returns the process exit code. It exists
// as a separate function so the deferred cancel actually runs; main's os.Exit
// would skip it.
func run() int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Establish the project root once, here, so every command resolves paths
	// against the repository rather than against wherever the user was standing.
	// A failure to find one is carried in the context, not reported: `--version`
	// and `--help` are legitimate outside a repository, and only a command that
	// asks for a root should have to care that there is none.
	ctx = paths.WithDiscoveredRoot(ctx)

	// Diagnostics go to a log file for the rest of the run. Open reports no
	// failure because there is nowhere to report one to: the terminal belongs to
	// the command's own output, and a warning printed there is the interleaving
	// the file exists to avoid. A run whose log could not be opened just has no
	// log.
	closeLog := logging.Open()
	defer closeLog()

	logging.Info(ctx, "harnaas started", slog.String("version", versioninfo.Version))

	watchSignals(cancel, os.Stderr)

	root := cli.NewRootCmd()

	// ExecuteContextC returns the deepest command cobra matched, which is the
	// one whose usage a positional-argument error should show.
	executed, err := root.ExecuteContextC(ctx)
	if err == nil {
		logging.Info(ctx, "harnaas finished", slog.Int("exit_code", exitSuccess))
		return exitSuccess
	}

	if sig := terminatingSignal(err); sig != nil {
		logging.Info(ctx, "harnaas terminated by signal", slog.String("signal", sig.String()))
		cancel()
		dieFromSignal(sig)
	}

	code := reportError(root.ErrOrStderr(), root, executed, err)

	// The failure is recorded by type, never by message. Errors from later
	// phases quote the manifest that produced them, and a team's instructions
	// and rules are the content that must never reach a log file.
	logging.Error(ctx, "harnaas failed",
		slog.Int("exit_code", code),
		slog.String("error_type", fmt.Sprintf("%T", err)),
	)

	return code
}

// watchSignals installs the two-stage handler. The first signal cancels the
// root context so in-flight work unwinds cleanly and prints a notice; because
// signal.Notify has disabled Go's default "signal terminates the process"
// behaviour, a second read is what keeps a further Ctrl-C from being swallowed
// during a slow or stuck shutdown. The caught signal is recorded so the
// eventual termination re-raises that same signal rather than assuming an
// interrupt.
func watchSignals(cancel context.CancelFunc, notice io.Writer) {
	sigChan := make(chan os.Signal, 1)
	signals := []os.Signal{os.Interrupt}
	if runtime.GOOS != "windows" {
		signals = append(signals, syscall.SIGTERM)
	}
	signal.Notify(sigChan, signals...)

	go func() {
		sig := <-sigChan
		procsignal.Store(sig)
		fmt.Fprintln(notice, forceQuitNotice(sig))
		cancel()
		<-sigChan
		dieFromSignal(sig)
	}()
}

// forceQuitNotice is the advisory text for a first signal. It distinguishes an
// interrupt the user typed from a termination signal a supervisor sent, because
// the two want different explanations, and it leads with a blank line so the
// notice does not land on top of a half-written progress line.
func forceQuitNotice(sig os.Signal) string {
	if sig == os.Interrupt {
		return "\nInterrupting… press Ctrl-C again to force quit."
	}
	return "\nReceived termination signal, shutting down… signal again to force quit."
}

// terminatingSignal reports the signal that ended the run, or nil if it was not
// signal-driven.
//
// A prompt the user interrupted reports one even though no signal was ever
// delivered. A full-screen form puts the terminal in raw mode, and raw mode
// disables the line discipline's signal characters, so Ctrl-C reaches harnaas
// as a keystroke the form consumed instead of as SIGINT. The user pressed
// Ctrl-C; that the terminal was in a mode which does not translate it is not
// something they should have to know, and exiting normally would leave their
// Ctrl-C trapped inside a `while true` loop — the one outcome the re-raise in
// dieFromSignal exists to prevent.
//
// Otherwise the gate is deliberately on a signal having been recorded rather
// than on the error type alone: a context.Canceled that arose without a signal
// — an internally cancelled sub-context, say — is a genuine failure and must be
// reported as one, not dressed up as a user abort that would also wrongly break
// an enclosing shell loop.
func terminatingSignal(err error) os.Signal {
	if errors.Is(err, uiform.ErrInterrupted) {
		return os.Interrupt
	}
	if !errors.Is(err, context.Canceled) {
		return nil
	}
	return procsignal.Load()
}

// dieFromSignal terminates the process as if it had been killed by sig, rather
// than exiting normally.
//
// The distinction matters to an interactive shell: it aborts a
// `while true; do harnaas …; done` loop only when the child is *killed by*
// SIGINT. A plain exit with status 130 is an ordinary exit, so the loop keeps
// respawning harnaas and the user's Ctrl-C never escapes it. Re-raising the
// actual signal also keeps a SIGTERM shutdown reporting the conventional 143.
//
// The signal is reset to its default disposition, re-raised at this process,
// and given a moment to arrive. Where the re-raise cannot be delivered — on
// Windows, where signal-to-self is unsupported — the conventional
// 128-plus-signal-number exit is the fallback, so the process never hangs.
func dieFromSignal(sig os.Signal) {
	signal.Reset(sig)
	if p, err := os.FindProcess(os.Getpid()); err == nil {
		if err := p.Signal(sig); err == nil {
			time.Sleep(forceQuitGrace)
		}
	}
	os.Exit(exitCodeForSignal(sig))
}

// exitCodeForSignal maps a signal to the conventional 128-plus-signal-number
// exit code — 130 for SIGINT, 143 for SIGTERM — falling back to an interrupt's
// 130 for a signal carrying no numeric value on this platform.
func exitCodeForSignal(sig os.Signal) int {
	if s, ok := sig.(syscall.Signal); ok {
		return 128 + int(s)
	}
	return 128 + int(syscall.SIGINT)
}

// reportError renders err according to the error contract and returns the exit
// code the process should use. It is the single place that decides what a
// failure looks like on the terminal.
//
// root is the command tree's root and executed is the deepest command cobra
// matched; the two differ exactly when a subcommand's own validator failed,
// which is when the subcommand's usage — not the root's — is the useful one.
func reportError(out io.Writer, root, executed *cobra.Command, err error) int {
	var printed *cli.AlreadyPrintedError

	switch {
	case errors.As(err, &printed):
		// The command already wrote a friendly explanation. Printing anything
		// here would restate it in a less useful form.
	case isUnknownCommandOrFlagError(err):
		writeUsageAndError(out, root, err)
	case isPositionalArgError(err):
		writeUsageAndError(out, executed, err)
	default:
		fmt.Fprintln(out, err)
	}

	return exitFailure
}

// isUnknownCommandOrFlagError reports whether err is cobra's complaint about a
// verb or flag it does not recognise, which is a usage problem rather than a
// runtime one.
func isUnknownCommandOrFlagError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "unknown command") || strings.Contains(msg, "unknown flag")
}

// isPositionalArgError reports whether err looks like a cobra positional-
// argument validator failure. Cobra's stock validators — ExactArgs, NoArgs,
// MinimumNArgs, MaximumNArgs, RangeArgs — all surface messages containing
// "arg(s)", and that substring appears in none of cobra's other error paths nor
// in harnaas's own errors, so it is a stable discriminator that does not reach
// into cobra internals.
func isPositionalArgError(err error) bool {
	return strings.Contains(err.Error(), "arg(s)")
}

// writeUsageAndError prints the command's usage before the error, so the reader
// sees what was expected and only then what went wrong.
func writeUsageAndError(out io.Writer, cmd *cobra.Command, err error) {
	if cmd == nil {
		fmt.Fprintln(out, err)
		return
	}
	fmt.Fprint(out, cmd.UsageString())
	fmt.Fprintf(out, "\nError: invalid usage: %v\n", err)
}
