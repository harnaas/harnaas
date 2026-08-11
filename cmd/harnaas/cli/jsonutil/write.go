package jsonutil

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// WriteFileAtomic replaces the file at filePath with data, or leaves it exactly
// as it was.
//
// The bytes are staged in a sibling file in the destination directory, flushed
// to disk, given perm, and then renamed over the destination. Rename is the
// step that makes this atomic: on every platform harnaas targets a reader sees
// either the whole old file or the whole new one, never a truncated file that
// no longer parses. Staging beside the destination rather than in the system
// temporary directory is what keeps the rename a rename — across filesystems it
// degrades into a copy, which is the non-atomic write this function exists to
// avoid.
//
// The flush before the rename matters as much as the rename. Without it some
// filesystems can make the rename visible after a crash while the staged file's
// contents are still buffered, which presents an empty file as a completed
// write — worse than either the old or the new file, because it looks valid.
//
// The staging file is removed on both paths. On failure that is the whole point:
// a failed write leaves the directory as it found it, with no `.tmp` litter for
// the user to wonder about. On success the rename has already consumed the name
// and the removal finds nothing, which is why its error is dropped.
func WriteFileAtomic(filePath string, data []byte, perm fs.FileMode) error {
	return writeFileAtomic(filePath, data, perm, (*os.File).Sync)
}

// writeFileAtomic is WriteFileAtomic with the flush step supplied by the
// caller.
//
// The seam exists for one test: that a failure occurring after the staging file
// has been created and written leaves the previous file intact and no staging
// file behind. That guarantee is the reason this function is not a plain
// os.WriteFile, and a guarantee about a failure path that is never exercised on
// a failure path is not a guarantee.
func writeFileAtomic(filePath string, data []byte, perm fs.FileMode, sync func(*os.File) error) (err error) {
	dir := filepath.Dir(filePath)

	staging, err := os.CreateTemp(dir, filepath.Base(filePath)+".*.tmp")
	if err != nil {
		return fmt.Errorf("stage a replacement for %s: %w", filePath, err)
	}
	stagingPath := staging.Name()

	// Unconditional, so no early return can skip it. os.Remove on the name the
	// rename already consumed reports "not exist", which is the outcome asked
	// for.
	defer func() { _ = os.Remove(stagingPath) }()

	if err := writeStaging(staging, data, sync, perm); err != nil {
		return fmt.Errorf("stage a replacement for %s: %w", filePath, err)
	}

	if err := os.Rename(stagingPath, filePath); err != nil {
		return fmt.Errorf("move the staged replacement into place at %s: %w", filePath, err)
	}

	syncDir(dir)

	return nil
}

// writeStaging fills the staging file and closes it, so that every failure
// before the rename leaves a closed file for the caller's deferred removal to
// delete. An open handle would keep the file undeletable on Windows.
func writeStaging(staging *os.File, data []byte, sync func(*os.File) error, perm fs.FileMode) error {
	if err := writeAndSync(staging, data, sync); err != nil {
		return err
	}

	// After the close, not before: perm describes the file the reader will
	// find, and chmod by name is the same operation either way.
	if err := os.Chmod(staging.Name(), perm); err != nil {
		return fmt.Errorf("set permissions on the staged file: %w", err)
	}

	return nil
}

// writeAndSync writes, flushes and closes, closing on every path including the
// ones where the write or the flush failed.
func writeAndSync(staging *os.File, data []byte, sync func(*os.File) error) (err error) {
	defer func() {
		closeErr := staging.Close()
		if err == nil && closeErr != nil {
			err = fmt.Errorf("close the staged file: %w", closeErr)
		}
	}()

	if _, err := staging.Write(data); err != nil {
		return fmt.Errorf("write the staged file: %w", err)
	}

	if err := sync(staging); err != nil {
		return fmt.Errorf("flush the staged file to disk: %w", err)
	}

	return nil
}

// syncDir flushes the directory entry the rename created, so that a crash
// cannot leave the new contents on disk while the directory still points at the
// old file.
//
// Best-effort, and deliberately so. Windows does not support flushing a
// directory handle at all, and on a system that does, the rename has already
// succeeded by the time this runs — the caller has the file it asked for, and
// reporting a failure here would tell them otherwise.
func syncDir(dir string) {
	//nolint:gosec // G304: dir is filepath.Dir of the caller's own destination path, opened read-only to flush it.
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync() //nolint:errcheck // Best-effort: the rename already succeeded, so a failure here must not be reported as a failed write.
	_ = d.Close()
}
