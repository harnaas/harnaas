//go:build e2e

// Package e2e runs the built harnaas binary as a process.
//
// Everything asserted here is a property of the process rather than of a
// function: the status a shell sees when harnaas finishes, and whether an
// interrupt killed harnaas or harnaas exited with a number that merely looks
// like one. Neither is observable from inside the test binary — the exit code
// is the entrypoint's own os.Exit call, and re-raising a signal to self would
// kill the test run rather than the subject.
//
// So these tests build the binary and run it, which is slower than the unit
// suite by more than a feedback loop can afford. They sit behind the `e2e`
// build tag and are run on their own: `mise run test:e2e`.
package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/internal/testenv"
)

// envBinary names a harnaas binary to run instead of building one.
//
// It exists for the two cases where building from source here is the wrong
// thing: a runner that has already built the binary it means to ship and wants
// these tests run against that one, and a machine with no Go toolchain, which
// is the only way to exercise the signal behaviour where harnaas was
// cross-compiled for a platform the developer is not sitting on. Unset, the
// suite builds the binary itself, which is what keeps the default honest.
const envBinary = "HARNAAS_E2E_BIN"

// binPath is the binary under test, resolved once for the whole suite by
// TestMain.
var binPath string

func TestMain(m *testing.M) {
	os.Exit(runSuite(m))
}

// runSuite builds the binary and then runs the suite with the per-user
// directories redirected.
//
// The build happens before the redirect deliberately. It reads the real module
// and build caches — the same ones testenv pins the redirect to, for this exact
// reason — so building first keeps the toolchain out of the redirect entirely.
// A build failure is also reported before any test has started, rather than as
// the same message repeated by every test that wanted a binary.
func runSuite(m *testing.M) int {
	dir, err := os.MkdirTemp("", "harnaas-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: create the build directory:", err)
		return 1
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	binPath, err = resolveBinary(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}

	// The child inherits this process's environment, so the redirect installed
	// here is also what keeps the binary's own log file out of the developer's
	// real cache directory.
	return testenv.Main(m)
}

// resolveBinary returns the binary under test: the one envBinary names where it
// is set, and otherwise one built into dir from the sources as they are now.
//
// A named binary is made absolute here rather than at each use, because every
// run happens in a scratch project directory where a relative path would name
// nothing.
func resolveBinary(dir string) (string, error) {
	named := os.Getenv(envBinary)
	if named == "" {
		return buildBinary(dir)
	}

	path, err := filepath.Abs(named)
	if err != nil {
		return "", fmt.Errorf("resolve %s=%s to an absolute path: %w", envBinary, named, err)
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("the binary named by %s: %w", envBinary, err)
	}
	return path, nil
}

// buildBinary builds cmd/harnaas into dir and returns the executable's path.
//
// The build runs from the module root, which is this package's parent: `go
// test` makes the package directory the test binary's working directory, so the
// relative path needs no lookup and no environment variable.
func buildBinary(dir string) (string, error) {
	name := "harnaas"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)

	build := exec.Command("go", "build", "-o", path, "./cmd/harnaas")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build the harnaas binary: %w\n%s", err, out)
	}
	return path, nil
}

// result is one run of the binary: what a shell would have seen.
type result struct {
	// ExitCode is the status the process exited with, or -1 where it was
	// terminated by a signal rather than exiting at all.
	ExitCode int

	// Stdout and Stderr are the two streams kept apart, because which of them
	// a line arrived on is itself part of the contract.
	Stdout string
	Stderr string
}

// runHarnaas runs the binary in dir with args and waits for it to finish.
//
// A non-zero status is an outcome rather than a failure — most of what this
// package asserts is which status — so only a failure to run the process at all
// fails the test.
func runHarnaas(t *testing.T, dir string, args ...string) result {
	t.Helper()

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(binPath, args...)
	cmd.Dir = dir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	var exitErr *exec.ExitError
	if err := cmd.Run(); err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("run harnaas %v in %s: %v", args, dir, err)
	}

	return result{
		ExitCode: cmd.ProcessState.ExitCode(),
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
}

// newProject creates a scratch project and returns its root.
//
// The repository marker is created directly rather than by running `git init`,
// because the root is resolved by looking for that entry and nothing here needs
// a working tree. A test that shelled out to git would also fail on a machine
// without it, for a reason that has nothing to do with harnaas.
func newProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".git"), 0o750))
	return dir
}

// newDirectoryOutsideARepository returns a directory with no repository above
// it, which is the state a command needing a project root has to refuse.
func newDirectoryOutsideARepository(t *testing.T) string {
	t.Helper()

	return t.TempDir()
}
