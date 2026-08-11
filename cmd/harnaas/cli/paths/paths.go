// Package paths establishes which project harnaas is acting on and carries it
// in the request context.
//
// Every path a command touches resolves against the project root, never against
// the process working directory: `harnaas install` run from `src/api/` has to
// install into the same places as the same command run from the repository
// root, and `harnaas lint` has to report the same findings from either. Reading
// the working directory at a use site quietly makes the answer depend on where
// the user happened to stand.
//
// So the root is resolved once, at the entrypoint, and read from the context
// everywhere else. forbidigo bans os.Getwd across the repository with a message
// naming ProjectRoot(ctx) as the replacement; WithDiscoveredRoot below is the
// single sanctioned reader of the working directory, and it exists only to feed
// that carrier. Nothing here resolves a root a second time.
package paths

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// RepositoryMarker is the entry whose presence makes a directory the root of a
// repository. Git writes it as a directory in an ordinary clone and as a file in
// a worktree or a submodule, so the search below tests for existence and not for
// a directory — refusing to run inside a worktree would be an arbitrary
// restriction, not a safety property.
const RepositoryMarker = ".git"

// ErrRootNotEstablished reports that the context never passed through
// WithDiscoveredRoot or WithProjectRoot. It is a wiring failure rather than
// anything the user did: some caller built a context by hand and gave it to a
// command. The message still reads as a diagnostic because an error that can
// reach a terminal must, and it names the fix a developer needs.
var ErrRootNotEstablished error = rootNotEstablishedError{}

// rootNotEstablishedError carries ErrRootNotEstablished's message. It is a type
// rather than an errors.New value so the message can be a problem and a fix
// separated by a blank line, like every other diagnostic here, instead of the
// single unpunctuated clause a bare sentinel is held to.
type rootNotEstablishedError struct{}

func (rootNotEstablishedError) Error() string {
	return "no project root was established for this command\n\n" +
		"Build the command's context with paths.WithDiscoveredRoot before executing it."
}

// NoProjectRootError reports that Dir is not inside a repository, so there is no
// project for a command to act on.
//
// It is a type rather than a formatted string so a command can recognise the
// condition — `init` may want to say something about starting a repository
// where `lint` only wants to fail — without matching on message text.
type NoProjectRootError struct {
	// Dir is the absolute directory the upward search started from. Naming it
	// is most of the diagnostic: "not inside a repository" is puzzling until
	// the user sees which directory harnaas was actually looking at.
	Dir string
}

// Error states the problem and the edit that fixes it, in that order, as every
// user-facing diagnostic does.
func (e *NoProjectRootError) Error() string {
	return fmt.Sprintf(
		"no project root found: %s is not inside a repository\n\n"+
			"Run harnaas from inside your project's repository, or create one there with `git init`.",
		e.Dir,
	)
}

// rootKey is the context key for the resolved root. An unexported struct type
// cannot collide with a key from any other package.
type rootKey struct{}

// resolution is what the context carries: the root that was found, or the
// failure to find one.
//
// The failure travels rather than aborting at the entrypoint because most of
// what harnaas can do needs no project at all — `--version`, `--help`, an
// unknown-command diagnostic — and refusing to start outside a repository would
// break every one of them. A command that needs a root asks for one and gets the
// failure then, at the moment it is relevant.
type resolution struct {
	root string
	err  error
}

// Resolve walks up from startDir and returns the first enclosing directory
// holding a RepositoryMarker, or a *NoProjectRootError if the walk reaches the
// filesystem root without finding one.
//
// The nearest enclosing repository wins, so a command run inside a submodule
// acts on the submodule. That is the same answer git itself gives, and a rule
// that agrees with git is one users already know.
func Resolve(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolve %s to an absolute path: %w", startDir, err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, RepositoryMarker)); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// filepath.Dir is its own fixed point at the filesystem root, and
			// on Windows at a volume root too, so this is the end of the walk
			// on every platform without special-casing either.
			return "", &NoProjectRootError{Dir: filepath.Clean(startDir)}
		}
		dir = parent
	}
}

// WithProjectRoot returns a context carrying root as the project root, for a
// caller that already knows it — a test over a scratch project, or a future
// command that resolves a project some other way.
func WithProjectRoot(ctx context.Context, root string) context.Context {
	return context.WithValue(ctx, rootKey{}, resolution{root: root})
}

// WithDiscoveredRoot resolves the project root from the process working
// directory and returns a context carrying either the root or the failure.
//
// This is the one place in harnaas that reads the working directory. It runs
// once, at the entrypoint, before any command executes; every other site calls
// ProjectRoot. Neither failure — an unreadable working directory or a directory
// outside any repository — is reported here, because at this point nobody knows
// yet whether the command about to run needs a project.
func WithDiscoveredRoot(ctx context.Context) context.Context {
	//nolint:forbidigo // The ban names ProjectRoot(ctx) as the replacement; this is the function that fills it, and the only sanctioned reader of the working directory.
	wd, err := os.Getwd()
	if err != nil {
		return context.WithValue(ctx, rootKey{}, resolution{
			err: fmt.Errorf("determine the working directory to resolve the project root from: %w", err),
		})
	}

	root, err := Resolve(wd)
	return context.WithValue(ctx, rootKey{}, resolution{root: root, err: err})
}

// ProjectRoot returns the project root carried by ctx.
//
// This is the replacement forbidigo's message names, and the only supported way
// to learn where the project is. A command that calls it is declaring that it
// needs a project, which is why the discovery failure surfaces here rather than
// at the entrypoint.
func ProjectRoot(ctx context.Context) (string, error) {
	res, ok := ctx.Value(rootKey{}).(resolution)
	if !ok {
		return "", ErrRootNotEstablished
	}
	if res.err != nil {
		return "", res.err
	}
	return res.root, nil
}
