package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"

	"github.com/harnaas/harnaas/cmd/harnaas/cli"
	"github.com/harnaas/harnaas/internal/procsignal"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dieFromSignalEnvVar, when set on a re-executed test binary, makes TestMain
// call dieFromSignal with the named signal instead of running the suite. The
// parent then inspects how the child died — the only way to exercise real
// process death without killing the test runner.
const dieFromSignalEnvVar = "HARNAAS_TEST_DIE_FROM_SIGNAL"

func TestMain(m *testing.M) {
	if name := os.Getenv(dieFromSignalEnvVar); name != "" {
		sig := os.Signal(syscall.SIGINT)
		if name == "TERM" {
			sig = syscall.SIGTERM
		}
		// dieFromSignal returns only when the re-raise could not be delivered,
		// so the fallback here mirrors its own.
		dieFromSignal(sig)
		os.Exit(exitCodeForSignal(sig))
	}
	os.Exit(m.Run())
}

// nonNumericSignal is an os.Signal that is not a syscall.Signal, exercising
// exitCodeForSignal's fallback branch.
type nonNumericSignal struct{}

func (nonNumericSignal) String() string { return "non-numeric" }
func (nonNumericSignal) Signal()        {}

// TestDieFromSignalTerminatesBySignal is the regression guard for the headline
// behaviour: a shell aborts a `while true; do harnaas …; done` loop only when
// the child is killed by the signal, not when it exits normally with code 130.
// Simplifying dieFromSignal back to a plain os.Exit(130) would leave the exit
// code looking right while silently breaking loop escape; this catches exactly
// that by re-executing the test binary and asserting how it died.
func TestDieFromSignalTerminatesBySignal(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("signal-to-self and kill-by-signal reporting do not exist on Windows")
	}

	tests := []struct {
		name string
		env  string
		want syscall.Signal
	}{
		{"SIGINT", "INT", syscall.SIGINT},
		{"SIGTERM", "TERM", syscall.SIGTERM},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// -test.run=^$ matches no test; TestMain's child branch runs before
			// m.Run and ends the process, so no test actually executes.
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^$")
			cmd.Env = append(os.Environ(), dieFromSignalEnvVar+"="+tc.env)

			err := cmd.Run()

			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr, "child should have died by signal, got err=%v", err)
			ws, ok := exitErr.Sys().(syscall.WaitStatus)
			require.True(t, ok, "no syscall.WaitStatus available: %T", exitErr.Sys())
			require.True(t, ws.Signaled(),
				"child exited normally with code %d; dieFromSignal must re-raise, not os.Exit", ws.ExitStatus())
			assert.Equal(t, tc.want, ws.Signal())
		})
	}
}

// TestExitCodeForSignal locks the 128-plus-signal-number mapping, so a
// "simplification" back to a hardcoded 130 cannot silently regress SIGTERM.
func TestExitCodeForSignal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sig  os.Signal
		want int
	}{
		{"interrupt", os.Interrupt, 130},
		{"termination", syscall.SIGTERM, 143},
		{"a signal with no number falls back to the interrupt code", nonNumericSignal{}, 130},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, exitCodeForSignal(tc.sig))
		})
	}
}

// TestExitCodeTaxonomy pins the documented meanings so a later change cannot
// renumber them, and records that 2 is reserved rather than in use.
func TestExitCodeTaxonomy(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 0, exitSuccess)
	assert.Equal(t, 1, exitFailure)
	assert.Equal(t, 2, exitLintFindings)
}

func TestForceQuitNoticeDistinguishesTheSignal(t *testing.T) {
	t.Parallel()

	assert.Contains(t, forceQuitNotice(os.Interrupt), "Ctrl-C again")
	assert.Contains(t, forceQuitNotice(syscall.SIGTERM), "termination signal")
}

func TestTerminatingSignal(t *testing.T) {
	// Mutates the process-global recorded signal, so no t.Parallel here.
	tests := []struct {
		name     string
		recorded os.Signal
		err      error
		want     os.Signal
	}{
		{
			name:     "a signalled cancellation reports its signal",
			recorded: syscall.SIGTERM,
			err:      context.Canceled,
			want:     syscall.SIGTERM,
		},
		{
			name:     "a cancellation with no signal is an ordinary failure",
			recorded: nil,
			err:      fmt.Errorf("fetch source: %w", context.Canceled),
			want:     nil,
		},
		{
			name:     "an unrelated error is never signal-driven",
			recorded: os.Interrupt,
			err:      errors.New("manifest is missing"),
			want:     nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			procsignal.Reset()
			if tc.recorded != nil {
				procsignal.Store(tc.recorded)
			}
			t.Cleanup(procsignal.Reset)

			assert.Equal(t, tc.want, terminatingSignal(tc.err))
		})
	}
}

// newTestTree builds a root with one subcommand that takes exactly one
// argument, which is enough to reach every branch of the error switch.
func newTestTree() (root, child *cobra.Command) {
	root = cli.NewRootCmd()
	child = &cobra.Command{
		Use:   "demo",
		Short: "a subcommand used only by the entrypoint tests",
		Args:  cobra.ExactArgs(1),
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
	root.AddCommand(child)
	return root, child
}

func TestReportErrorPrintsAnAlreadyPrintedErrorNoFurther(t *testing.T) {
	t.Parallel()

	root, _ := newTestTree()
	cause := errors.New("harnaas.json not found")
	var out bytes.Buffer

	code := reportError(&out, root, root, cli.NewAlreadyPrintedError(cause))

	assert.Equal(t, exitFailure, code, "an already-printed failure still exits non-zero")
	assert.Empty(t, out.String(), "the command already explained this; the entrypoint must add nothing")
}

func TestReportErrorPrintsAPlainErrorExactlyOnce(t *testing.T) {
	t.Parallel()

	root, _ := newTestTree()
	var out bytes.Buffer

	code := reportError(&out, root, root, errors.New("read manifest: permission denied"))

	assert.Equal(t, exitFailure, code)
	assert.Equal(t, "read manifest: permission denied\n", out.String())
}

func TestReportErrorShowsRootUsageForAnUnknownCommandOrFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{"unknown command", errors.New(`unknown command "instal" for "harnaas"`)},
		{"unknown flag", errors.New("unknown flag: --jsn")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root, _ := newTestTree()
			var out bytes.Buffer

			code := reportError(&out, root, root, tc.err)

			assert.Equal(t, exitFailure, code)
			assert.Contains(t, out.String(), root.UsageString())
			assert.Contains(t, out.String(), tc.err.Error())
		})
	}
}

func TestReportErrorShowsTheDeepestSubcommandUsageForAnArgCountError(t *testing.T) {
	t.Parallel()

	root, child := newTestTree()
	var out bytes.Buffer

	code := reportError(&out, root, child, errors.New("accepts 1 arg(s), received 3"))

	assert.Equal(t, exitFailure, code)
	assert.Contains(t, out.String(), child.UsageString())
	assert.NotContains(t, out.String(), root.UsageString(),
		"an arg-count failure belongs to the subcommand, so the root's usage is not the useful one")
}

// TestReportErrorNeverReturnsTheLintFindingsCode holds the negative half of the
// exit-code contract: 2 means lint findings and nothing else, so no error path
// in the entrypoint may produce it.
func TestReportErrorNeverReturnsTheLintFindingsCode(t *testing.T) {
	t.Parallel()

	root, child := newTestTree()
	errs := []error{
		errors.New("read manifest: permission denied"),
		cli.NewAlreadyPrintedError(errors.New("harnaas.json not found")),
		errors.New(`unknown command "instal" for "harnaas"`),
		errors.New("unknown flag: --jsn"),
		errors.New("accepts 1 arg(s), received 3"),
		context.Canceled,
	}
	for _, err := range errs {
		var out bytes.Buffer
		assert.NotEqual(t, exitLintFindings, reportError(&out, root, child, err), "error: %v", err)
	}
}

// TestReportErrorKeepsTheCauseUnwrappable guards the other half of the
// already-printed contract: suppressing the message must not suppress the
// error, so a caller can still inspect what happened.
func TestReportErrorKeepsTheCauseUnwrappable(t *testing.T) {
	t.Parallel()

	cause := errors.New("harnaas.json not found")
	err := error(cli.NewAlreadyPrintedError(cause))

	root, _ := newTestTree()
	var out bytes.Buffer
	reportError(&out, root, root, err)

	assert.ErrorIs(t, err, cause)
}

// TestErrorSwitchClassifiesRealCobraErrors runs the switch over errors cobra
// actually produces, rather than hand-written lookalikes, so a change to
// cobra's wording cannot leave the classifiers silently matching nothing.
func TestErrorSwitchClassifiesRealCobraErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		wantUsage func(root, child *cobra.Command) *cobra.Command
	}{
		{
			name:      "an unrecognized verb shows the root usage",
			args:      []string{"bogus"},
			wantUsage: func(root, _ *cobra.Command) *cobra.Command { return root },
		},
		{
			name:      "an unrecognized flag shows the root usage",
			args:      []string{"--jsn"},
			wantUsage: func(root, _ *cobra.Command) *cobra.Command { return root },
		},
		{
			name:      "a wrong argument count shows the subcommand usage",
			args:      []string{"demo", "one", "two"},
			wantUsage: func(_, child *cobra.Command) *cobra.Command { return child },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root, child := newTestTree()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(tc.args)

			executed, err := root.ExecuteContextC(t.Context())
			require.Error(t, err)

			var out bytes.Buffer
			code := reportError(&out, root, executed, err)

			assert.Equal(t, exitFailure, code)
			assert.Contains(t, out.String(), tc.wantUsage(root, child).UsageString())
			assert.Contains(t, out.String(), err.Error())
		})
	}
}

// TestRootCommandSuppressesCobrasOwnErrorPrinting proves the entrypoint really
// is the only component that renders an error: cobra must return the failure
// without writing anything itself.
func TestRootCommandSuppressesCobrasOwnErrorPrinting(t *testing.T) {
	t.Parallel()

	root, _ := newTestTree()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"demo"}) // ExactArgs(1) fails

	_, err := root.ExecuteContextC(t.Context())

	require.Error(t, err)
	assert.Empty(t, out.String())
	assert.Empty(t, errOut.String(), "cobra must print neither the error nor the usage block")
}
