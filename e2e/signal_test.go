//go:build e2e

package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// interruptWait is how long a step of the interrupt sequence may take. It is
// generous because every value it bounds is a process start or a signal
// delivery, both of which take milliseconds and neither of which gets faster
// for being hurried on a loaded CI runner.
const interruptWait = 30 * time.Second

// TestInterruptTerminatesTheProcessBySignal proves harnaas is killed by the
// interrupt it received rather than exiting with a status that merely encodes
// one.
//
// The difference is invisible in the exit status a shell reports and decides
// whether a user's Ctrl-C escapes a `while true; do harnaas …; done` loop: the
// shell aborts the loop only when the child was terminated *by* the signal. A
// plain exit with status 130 is an ordinary exit, so the loop respawns harnaas
// and the interrupt never gets out.
//
// It cannot be asserted from inside the test binary, because the re-raised
// signal would kill the test run instead of the subject — which is the reason
// this package runs the built binary at all.
func TestInterruptTerminatesTheProcessBySignal(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("windows supports no signal-to-self, so dieFromSignal takes its documented 128-plus-signum fallback there — an ordinary exit, which is what the fallback is for")
	}

	project := newProject(t)

	cmd := exec.Command(binPath, "init")
	cmd.Dir = project
	// The two variables put the process where a user interrupts one: prompting
	// is forced on, and the accessible prompt is a plain question read from
	// stdin — so with nothing written to stdin the process blocks on an answer
	// that never comes, with its signal handler already installed.
	cmd.Env = append(os.Environ(), "HARNAAS_TEST_TTY=1", "ACCESSIBLE=1")

	// Held open for the lifetime of the test: closing it would send EOF, which
	// the prompt reads as an answer and stops waiting for one.
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	defer stdin.Close()

	var stderr syncBuffer
	cmd.Stderr = &stderr

	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
	})

	// The prompt is written before the read that blocks, so its arrival is the
	// only reliable evidence that the process is waiting rather than still
	// starting up. Interrupting earlier would test a different thing: the
	// cancellation would be noticed by the prompt as a cancelled context and
	// reported as an ordinary failure, which is correct behaviour and not this
	// behaviour.
	//
	// The sentinel is the accessible selection's own question, which is the last
	// thing written before it reads: init asks which harnesses this project
	// targets and waits for a number.
	requireOutput(t, &stderr, "Enter a number between")

	require.NoError(t, cmd.Process.Signal(os.Interrupt))
	requireOutput(t, &stderr, "press Ctrl-C again")

	// The first signal cancelled the root context, but the prompt is blocked on
	// stdin and cannot unwind, which is exactly the stuck shutdown the second
	// stage exists for.
	require.NoError(t, cmd.Process.Signal(os.Interrupt))

	err = cmd.Wait()
	require.Error(t, err, "harnaas finished successfully after two interrupts")

	state := cmd.ProcessState
	status, ok := state.Sys().(syscall.WaitStatus)
	require.True(t, ok, "no wait status for the terminated process")
	require.True(t, status.Signaled(),
		"harnaas exited with status %d rather than being terminated by a signal", state.ExitCode())
	assert.Equal(t, syscall.SIGINT, status.Signal(),
		"harnaas was terminated by a signal other than the one it received")

	assert.NoFileExists(t, filepath.Join(project, "harnaas.json"),
		"an interrupted run wrote the manifest")
}

// syncBuffer collects a stream the child process is writing while the test
// reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// requireOutput waits for want to appear in the stream, and fails with what did
// arrive instead — which is the whole diagnostic when a step of the sequence
// never happened.
func requireOutput(t *testing.T, stream *syncBuffer, want string) {
	t.Helper()

	deadline := time.Now().Add(interruptWait)
	for time.Now().Before(deadline) {
		if strings.Contains(stream.String(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q on stderr; stderr held:\n%s", want, stream.String())
}
