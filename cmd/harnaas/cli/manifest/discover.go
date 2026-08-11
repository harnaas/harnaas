package manifest

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/paths"
)

// skippedDirs are directory names the search for a stray manifest does not
// descend into.
//
// A `harnaas.json` inside a dependency tree belongs to that dependency, not to
// this project: a vendored library that manages its own harness assets would
// otherwise make every harnaas command in the parent repository fail with a
// diagnostic about a file the author cannot edit. Dot-directories are skipped
// for the same reason plus a plainer one — `.git` is large and holds nothing a
// person declared.
var skippedDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
}

// Path returns where the manifest lives for the project carried in ctx.
//
// It is the one place the file's location is assembled, so `init` writing the
// manifest and `install` reading it can never disagree about where it is.
func Path(ctx context.Context) (string, error) {
	root, err := paths.ProjectRoot(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, FileName), nil
}

// Load finds the project's manifest and decodes it.
//
// The manifest is resolved from the project root carried in ctx rather than
// from where the command was run, so every command reads the same declarations
// from every directory in the repository.
func Load(ctx context.Context) (*Document, error) {
	root, err := paths.ProjectRoot(ctx)
	if err != nil {
		return nil, err
	}
	return loadFrom(root)
}

// loadFrom is Load once the project root is known.
//
// The search for a stray manifest runs before the root manifest is read, and
// therefore before the "no manifest here" message too: a project whose only
// `harnaas.json` sits in a subdirectory is not a project without a manifest,
// and telling its author to run `harnaas init` would answer a question they did
// not ask.
func loadFrom(root string) (*Document, error) {
	stray, err := findSubdirectoryManifest(root)
	if err != nil {
		return nil, err
	}
	if stray != "" {
		return nil, &SubdirectoryError{Path: stray, Root: root}
	}

	path := filepath.Join(root, FileName)

	//nolint:gosec // G304: the path is the project root harnaas resolved plus a constant file name, and it is opened for reading only.
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &NotFoundError{Root: root}
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return decode(data, path)
}

// findSubdirectoryManifest returns the path of the first `harnaas.json` below
// root, or an empty string if there is none.
//
// The walk stops at the first match because one is enough to fail the run, and
// it skips the directories a project's own declarations never live in. A
// deliberate consequence is that harnaas refuses to run rather than quietly
// reading the root manifest: a file with this name that harnaas ignores is
// worse than one it rejects, because its author believes it is declaring
// assets.
func findSubdirectoryManifest(root string) (string, error) {
	found := ""

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// An unreadable directory is not evidence of a stray manifest, and
			// it is not this function's failure to report: whatever the command
			// actually needs from that directory will fail with a message about
			// the thing it needed.
			return nil //nolint:nilerr // Deliberate: an unreadable subtree cannot answer the question, so the search moves on.
		}

		if entry.IsDir() {
			if path == root {
				return nil
			}
			if skippedDirs[entry.Name()] || strings.HasPrefix(entry.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}

		if entry.Name() == FileName && filepath.Dir(path) != root {
			found = path
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("search %s for a second %s: %w", root, FileName, err)
	}

	return found, nil
}
