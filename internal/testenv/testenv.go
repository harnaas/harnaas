// Package testenv gives a test its own home, cache and config directories, so
// no test reads or writes the real ones.
//
// # Why
//
// Everything harnaas keeps that is not the manifest lives under the user's own
// directories: the log file sits under the cache directory, and user-scoped
// assets will sit under the config or home directory. A test exercising those
// paths against the real ones writes into the machine it happens to be running
// on — a developer's own log, or a developer's own harness configuration — and
// reads state that differs between that machine and CI. Neither failure is
// loud: the test passes, and what it proved is that the developer's machine was
// already set up the way the test assumed.
//
// So a package whose tests can reach per-user state redirects it first, and the
// redirect is verified rather than assumed. This package sets the environment
// variables the standard library derives each directory from, and then asks the
// standard library where the directories are. A platform whose mapping it does
// not cover fails the suite naming the directory it could not move, instead of
// quietly letting the tests write to the real one.
//
// # What is deliberately left alone
//
// The Go toolchain's own directories — the module cache, the build cache and
// the env file — are pinned to where they resolve today before anything moves.
// They are derived from the same home and cache directories, and a test that
// shells out to `go build` with them redirected would re-download the module
// graph into a directory that is deleted when the suite ends.
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// envVar is one variable and the value to redirect it to.
type envVar struct {
	key   string
	value string
}

// Redirect points the per-user directories at directories of this test's own
// and returns the root holding them.
//
// It sets environment variables, so a test calling it cannot call t.Parallel.
// Prefer Main, which costs a test nothing, cannot be forgotten and leaves the
// suite parallel; Redirect is for the test that needs to know where the
// directories are, because what it asserts is that something landed in one.
func Redirect(tb testing.TB) string {
	tb.Helper()

	root := tb.TempDir()
	if err := redirect(root, func(key, value string) error {
		tb.Setenv(key, value)
		return nil
	}); err != nil {
		tb.Fatal(err)
	}
	return root
}

// Main runs a package's tests with the per-user directories redirected, and
// returns the code TestMain hands to os.Exit:
//
//	func TestMain(m *testing.M) { os.Exit(testenv.Main(m)) }
//
// The redirect is process-wide and installed before the first test runs, which
// is what makes it a property of the package rather than something every test
// has to remember — and, unlike Redirect, it leaves every test free to run in
// parallel.
func Main(m *testing.M) int {
	return runMain(m.Run)
}

// runMain is Main with the suite as a parameter, so the creation and removal of
// the directory can be tested without re-executing the test binary.
func runMain(run func() int) int {
	root, err := os.MkdirTemp("", "harnaas-testenv-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "testenv: create the redirect directory:", err)
		return 1
	}
	defer func() {
		_ = os.RemoveAll(root)
	}()

	if err := redirect(root, os.Setenv); err != nil {
		fmt.Fprintln(os.Stderr, "testenv:", err)
		return 1
	}
	return run()
}

// redirect points the variables the standard library derives the per-user
// directories from at subdirectories of root, and then verifies that the
// standard library agrees the directories moved.
func redirect(root string, setenv func(key, value string) error) error {
	// Resolved before anything moves, because it reads the real directories.
	vars := append(toolchainVars(), userDirVars(root)...)

	for _, v := range vars {
		if err := setenv(v.key, v.value); err != nil {
			return fmt.Errorf("set %s: %w", v.key, err)
		}
	}

	for _, dir := range []struct {
		name    string
		resolve func() (string, error)
	}{
		{"home", os.UserHomeDir},
		{"cache", os.UserCacheDir},
		{"config", os.UserConfigDir},
	} {
		path, err := dir.resolve()
		if err != nil {
			return fmt.Errorf("resolve the %s directory after redirecting it: %w", dir.name, err)
		}
		if !within(root, path) {
			return fmt.Errorf(
				"the %s directory still resolves to %s, outside %s: this platform derives it from a variable testenv does not set",
				dir.name, path, root)
		}
		if err := os.MkdirAll(path, 0o750); err != nil {
			return fmt.Errorf("create the %s directory %s: %w", dir.name, path, err)
		}
	}
	return nil
}

// userDirVars are the variables the standard library derives os.UserHomeDir,
// os.UserCacheDir and os.UserConfigDir from, across the platforms harnaas
// builds for.
//
// Every one is set on every platform. The ones that do not apply here are
// simply not read, and a list that varied by build tag would be three files
// that have to agree with each other about a mapping the standard library
// already decides.
func userDirVars(root string) []envVar {
	home := filepath.Join(root, "home")
	cache := filepath.Join(root, "cache")
	config := filepath.Join(root, "config")

	return []envVar{
		{"HOME", home},              // unix home; darwin derives cache and config from it
		{"USERPROFILE", home},       // windows home
		{"XDG_CACHE_HOME", cache},   // linux cache
		{"LOCALAPPDATA", cache},     // windows cache
		{"XDG_CONFIG_HOME", config}, // linux config
		{"APPDATA", config},         // windows config
	}
}

// toolchainVars pins the Go toolchain's directories to where they resolve right
// now, so redirecting home and cache does not move them too.
//
// A variable already set keeps its value; the rest are spelled out with the
// same defaults `go env` would compute. Where a real directory cannot be
// resolved at all the variable is left alone, because a guess would be worse
// than the toolchain's own failure.
func toolchainVars() []envVar {
	var vars []envVar
	add := func(key, fallback string) {
		if value := os.Getenv(key); value != "" {
			vars = append(vars, envVar{key, value})
			return
		}
		if fallback != "" {
			vars = append(vars, envVar{key, fallback})
		}
	}

	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			gopath = filepath.Join(home, "go")
		}
	}
	add("GOPATH", gopath)
	if gopath != "" {
		add("GOMODCACHE", filepath.Join(gopath, "pkg", "mod"))
	}

	if cache, err := os.UserCacheDir(); err == nil {
		add("GOCACHE", filepath.Join(cache, "go-build"))
	}
	if config, err := os.UserConfigDir(); err == nil {
		add("GOENV", filepath.Join(config, "go", "env"))
	}
	return vars
}

// within reports whether path is root or sits beneath it.
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
