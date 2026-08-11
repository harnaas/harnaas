package jsonutil

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// entriesIn names everything in dir, which is how every test here checks that
// no staging file survived: the guarantee is about the directory, not about a
// name the test could guess.
func entriesIn(t *testing.T, dir string) []string {
	t.Helper()

	found, err := os.ReadDir(dir)
	require.NoError(t, err)

	names := make([]string, 0, len(found))
	for _, entry := range found {
		names = append(names, entry.Name())
	}
	return names
}

func TestWriteFileAtomicCreatesTheFileAndLeavesNothingElseBehind(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "harnaas.json")

	require.NoError(t, WriteFileAtomic(target, []byte("{\n  \"version\": 1\n}\n"), 0o644))

	contents, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "{\n  \"version\": 1\n}\n", string(contents))
	assert.Equal(t, []string{"harnaas.json"}, entriesIn(t, dir))
}

func TestWriteFileAtomicAppliesThePermission(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("windows maps a file mode onto a read-only bit, so the requested bits are not readable back")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "harnaas.json")

	require.NoError(t, WriteFileAtomic(target, []byte("{}\n"), 0o600))

	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// The replacement is shorter than what it replaces, which is the case an
// in-place write gets wrong by leaving the tail of the previous contents.
func TestWriteFileAtomicReplacesAnExistingFileInFull(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "harnaas.json")
	require.NoError(t, os.WriteFile(target, []byte("{\n  \"version\": 1,\n  \"assets\": []\n}\n"), 0o644))

	require.NoError(t, WriteFileAtomic(target, []byte("{}\n"), 0o644))

	contents, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "{}\n", string(contents))
	assert.Equal(t, []string{"harnaas.json"}, entriesIn(t, dir))
}

// This is the guarantee the whole function exists for: a failure after the
// staging file has been created and written must leave the destination exactly
// as it was, and must not leave the staging file behind.
func TestWriteFileAtomicLeavesThePreviousFileIntactWhenTheWriteFailsMidway(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "harnaas.json")
	previous := "{\n  \"version\": 1\n}\n"
	require.NoError(t, os.WriteFile(target, []byte(previous), 0o644))

	injected := errors.New("injected flush failure")
	err := writeFileAtomic(target, []byte("{\n  \"version\": 2\n}\n"), 0o644, func(*os.File) error {
		return injected
	})

	require.ErrorIs(t, err, injected)

	contents, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.Equal(t, previous, string(contents), "the destination still holds the previous document")
	assert.Equal(t, []string{"harnaas.json"}, entriesIn(t, dir), "no staging file survived the failure")
}

// The same failure with no previous file must not conjure a destination out of
// a partial write either.
func TestWriteFileAtomicCreatesNothingWhenTheWriteFailsMidway(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "harnaas.json")

	injected := errors.New("injected flush failure")
	err := writeFileAtomic(target, []byte("{}\n"), 0o644, func(*os.File) error { return injected })

	require.ErrorIs(t, err, injected)
	assert.NoFileExists(t, target)
	assert.Empty(t, entriesIn(t, dir), "no staging file survived the failure")
}

func TestWriteFileAtomicReportsAnUnusableDestinationDirectory(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "absent", "harnaas.json")

	err := WriteFileAtomic(target, []byte("{}\n"), 0o644)

	require.Error(t, err)
	assert.Contains(t, err.Error(), target, "the diagnostic names the file that could not be written")
}
