package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/jsonutil"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// installedFilePerm and installedDirPerm are the modes installed content is
// created with.
//
// Ordinary non-executable defaults: harness assets are documents, and an
// executable bit on one carries no meaning — which is also why file modes are
// not part of any digest.
const (
	installedFilePerm fs.FileMode = 0o644
	installedDirPerm  fs.FileMode = 0o755
)

// stagingSuffix and supersededSuffix name the two temporary neighbours a
// directory replacement uses. Both are removed on success and on failure, so a
// failed install leaves nothing behind for the next one to trip over.
const (
	stagingSuffix    = ".harnaas-staging"
	supersededSuffix = ".harnaas-superseded"
)

// installedState is what is on disk at a destination right now.
type installedState struct {
	// Present reports whether anything is there at all.
	Present bool

	// Files is a digest per file, relative to the destination and sorted by
	// path, in the shape the lockfile records.
	Files []lockFile

	// Digest covers the whole destination, computed the same way a resolved
	// source's is so the two are comparable at all.
	Digest source.Digest
}

// readInstalled reports what is at a destination, treating an absent one as
// simply not present.
//
// A file destination is read as a single entry named after its own leaf, which
// is how a resolved single-file source is shaped too — that symmetry is what
// lets one digest computation serve both sides of the comparison.
func readInstalled(root, destination string) (installedState, error) {
	full := filepath.Join(root, filepath.FromSlash(destination))

	info, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) {
		return installedState{}, nil
	}
	if err != nil {
		return installedState{}, fmt.Errorf("read %s: %w", destination, err)
	}

	content := make(map[string][]byte)
	if info.IsDir() {
		if err := collectFiles(full, "", content); err != nil {
			return installedState{}, err
		}
	} else {
		//nolint:gosec // G304: the path is the resolved scope root plus a destination harnaas itself computed.
		data, readErr := os.ReadFile(full)
		if readErr != nil {
			return installedState{}, fmt.Errorf("read %s: %w", destination, readErr)
		}
		content[path.Base(destination)] = data
	}

	if len(content) == 0 {
		// A directory with nothing in it is not an installation. Treating it as
		// absent lets the next install create the content rather than compare
		// against a digest over no files.
		return installedState{}, nil
	}

	resolved, err := source.NewResolved(source.Provenance{}, content)
	if err != nil {
		return installedState{}, fmt.Errorf("digest %s: %w", destination, err)
	}

	files := make([]lockFile, 0, len(resolved.Files))
	for _, file := range resolved.Files {
		files = append(files, lockFile{Path: file.Path, Digest: file.Digest})
	}
	return installedState{Present: true, Files: files, Digest: resolved.Digest}, nil
}

// collectFiles reads every regular file beneath dir into content, keyed by its
// slash-separated path relative to the destination.
//
// A entry that is not a regular file is skipped rather than followed: a symlink
// inside a managed destination is not something harnaas put there, and reading
// through one would digest a file outside the destination as though it were in
// it.
func collectFiles(dir, prefix string, content map[string][]byte) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	for _, entry := range entries {
		name := path.Join(prefix, entry.Name())
		full := filepath.Join(dir, entry.Name())
		switch {
		case entry.IsDir():
			if err := collectFiles(full, name, content); err != nil {
				return err
			}
		case entry.Type().IsRegular():
			//nolint:gosec // G304: the path is beneath a destination harnaas computed and is walked, not taken from input.
			data, readErr := os.ReadFile(full)
			if readErr != nil {
				return fmt.Errorf("read %s: %w", full, readErr)
			}
			content[name] = data
		}
	}
	return nil
}

// writeDestination puts files at the destination, replacing whatever is there
// as a unit.
//
// A single file goes through the module's one atomic writer. A directory is
// staged beside its final location and moved into place, so the destination is
// never observed holding a mix of old and new files — the failure mode a
// file-by-file update has whenever a skill gains or loses a file.
func writeDestination(root, destination string, files []source.File) (err error) {
	full := filepath.Join(root, filepath.FromSlash(destination))

	if len(files) == 1 && files[0].Path == path.Base(destination) {
		if err := os.MkdirAll(filepath.Dir(full), installedDirPerm); err != nil {
			return fmt.Errorf("create the directory for %s: %w", destination, err)
		}
		if err := jsonutil.WriteFileAtomic(full, files[0].Content, installedFilePerm); err != nil {
			return fmt.Errorf("write %s: %w", destination, err)
		}
		return nil
	}

	staging := full + stagingSuffix
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("clear the staging directory for %s: %w", destination, err)
	}
	defer func() {
		// Removed on both paths: a staging directory that outlived a failure is
		// the next run's mystery.
		if removeErr := os.RemoveAll(staging); err == nil && removeErr != nil {
			err = fmt.Errorf("remove the staging directory for %s: %w", destination, removeErr)
		}
	}()

	for _, file := range files {
		target := filepath.Join(staging, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), installedDirPerm); err != nil {
			return fmt.Errorf("stage %s: %w", file.Path, err)
		}
		if err := os.WriteFile(target, file.Content, installedFilePerm); err != nil {
			return fmt.Errorf("stage %s: %w", file.Path, err)
		}
	}

	return swapIntoPlace(full, staging, destination)
}

// swapIntoPlace moves staging to full, stepping the previous content aside
// first.
//
// Renaming onto an existing directory fails on Windows and silently merges on
// none of the platforms harnaas ships to, so the previous content is renamed
// away and removed only once the new content is in place. A failure between the
// two puts the previous content back, which is what keeps the destination
// either wholly old or wholly new.
func swapIntoPlace(full, staging, destination string) error {
	if err := os.MkdirAll(filepath.Dir(full), installedDirPerm); err != nil {
		return fmt.Errorf("create the directory for %s: %w", destination, err)
	}

	superseded := full + supersededSuffix
	if err := os.RemoveAll(superseded); err != nil {
		return fmt.Errorf("clear the superseded directory for %s: %w", destination, err)
	}

	stepped := false
	if _, err := os.Lstat(full); err == nil {
		if err := os.Rename(full, superseded); err != nil {
			return fmt.Errorf("move the previous %s aside: %w", destination, err)
		}
		stepped = true
	}

	if err := os.Rename(staging, full); err != nil {
		if stepped {
			// Best effort, and the only correct one: the previous content is
			// the state the user had, and leaving the destination missing would
			// be worse than the failure being reported.
			//nolint:errcheck // Best effort by definition: the rename that failed is already being reported, and there is nothing useful to do with a second failure while putting the previous content back.
			_ = os.Rename(superseded, full)
		}
		return fmt.Errorf("move %s into place: %w", destination, err)
	}

	if stepped {
		if err := os.RemoveAll(superseded); err != nil {
			return fmt.Errorf("remove the previous %s: %w", destination, err)
		}
	}
	return nil
}

// removeDestination deletes a managed destination and any directories it leaves
// empty beneath the scope root.
//
// Pruning the empty parents matters because the shared skills directory is
// created by harnaas and would otherwise linger after a full uninstall, looking
// like something a harness still reads. It stops at the scope root and at the
// first directory that still holds anything.
func removeDestination(root, destination string) error {
	full := filepath.Join(root, filepath.FromSlash(destination))
	if err := os.RemoveAll(full); err != nil {
		return fmt.Errorf("remove %s: %w", destination, err)
	}

	for dir := filepath.Dir(full); strings.HasPrefix(dir, root) && dir != root; dir = filepath.Dir(dir) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			//nolint:nilerr // Pruning empty parents is tidying, not part of the removal: a directory that cannot be read or is not empty simply ends the walk.
			return nil
		}
		if err := os.Remove(dir); err != nil {
			//nolint:nilerr // The destination is already gone, which is what the caller asked for; a parent that will not delete is left in place rather than failing a successful removal.
			return nil
		}
	}
	return nil
}

// lockFileName is the exclusive lock covering the lockfile's read-modify-write.
const lockLockFileName = ".harnaas.lock"

// lockWait and lockPoll bound how long a run waits for another to finish.
//
// Bounded rather than indefinite because the primary consumers are CI jobs and
// coding agents: a run that blocks forever behind a crashed one is a hung job
// nobody is watching, and a clear message naming the holder is something a
// reader can act on.
const (
	lockWait = 10 * time.Second
	lockPoll = 100 * time.Millisecond
)

// lockContentionError reports another run holding the install lock.
type lockContentionError struct {
	Path string
}

func (e *lockContentionError) Error() string {
	return fmt.Sprintf(
		"another harnaas install is running in this project (its lock is %s)\n\n"+
			"Wait for it to finish and run again. If no install is running, that file is left over "+
			"from one that was killed, and deleting it is safe.",
		e.Path,
	)
}

// acquireInstallLock takes the exclusive lock, waiting a bounded time for it.
//
// The lock is a file created with O_EXCL, which is atomic on every platform
// harnaas ships to and needs no dependency. What it guards is the lockfile's
// read-modify-write: two installs interleaving there would write a lockfile
// recording one run's assets and the other's installations.
func acquireInstallLock(root string) (func(), error) {
	full := filepath.Join(root, lockLockFileName)

	deadline := time.Now().Add(lockWait)
	for {
		//nolint:gosec // G304: the path is the project root harnaas resolved plus this package's own fixed lock name.
		file, err := os.OpenFile(full, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = file.Close()
			return func() { _ = os.Remove(full) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("take the install lock: %w", err)
		}
		if time.Now().After(deadline) {
			return nil, &lockContentionError{Path: full}
		}
		time.Sleep(lockPoll)
	}
}
