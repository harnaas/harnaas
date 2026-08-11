package source

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The ceilings are exercised through the unexported seam rather than by building
// an archive that meets the real ones: a test that had to produce 256 MiB to
// prove the archive limit works would be one nobody runs.

// internalSubject is the asset these archives are extracted for.
var internalSubject = ArchiveSubject{AssetID: "review", Source: "github:acme/assets", Commit: "9f2a1c4"}

// tinyArchive builds a gzipped tar of name-to-content pairs under the wrapper
// directory a repository archive carries.
func tinyArchive(tb testing.TB, files map[string]string) []byte {
	tb.Helper()

	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gz)

	for name, content := range files {
		require.NoError(tb, writer.WriteHeader(&tar.Header{
			Name:     "acme-assets-9f2a1c4/" + name,
			Typeflag: tar.TypeReg,
			Size:     int64(len(content)),
			Mode:     0o644,
		}))
		_, err := writer.Write([]byte(content))
		require.NoError(tb, err)
	}

	require.NoError(tb, writer.Close())
	require.NoError(tb, gz.Close())
	return buffer.Bytes()
}

// generousLimits leaves every ceiling out of the way so a test can lower exactly
// the one it is about.
func generousLimits() extractionLimits {
	return extractionLimits{archive: 1 << 20, asset: 1 << 20, file: 1 << 20}
}

func TestExtractSubtreeRefusesAFilePastThePerFileCeiling(t *testing.T) {
	t.Parallel()

	archive := tinyArchive(t, map[string]string{
		"skills/review/SKILL.md":            "review skill",
		"skills/review/reference/corpus.md": strings.Repeat("x", 500),
	})

	limits := generousLimits()
	limits.file = 100

	files, err := extractSubtree(archive, "skills/review", internalSubject, limits)

	var tooLarge *FileTooLargeError
	require.ErrorAs(t, err, &tooLarge)
	assert.Nil(t, files, "no partially extracted content reaches a caller")
	assert.Equal(t, "reference/corpus.md", tooLarge.Path, "the file is named as the asset sees it")
	assert.Equal(t, int64(500), tooLarge.Size)
	assert.Equal(t, int64(100), tooLarge.Limit)
}

func TestExtractSubtreeRefusesASubtreePastTheAssetCeiling(t *testing.T) {
	t.Parallel()

	archive := tinyArchive(t, map[string]string{
		"skills/review/SKILL.md":  strings.Repeat("a", 400),
		"skills/review/second.md": strings.Repeat("b", 400),
	})

	limits := generousLimits()
	// Neither file meets the per-file ceiling; together they pass the asset one,
	// which is the case the second limit exists for.
	limits.file = 500
	limits.asset = 600

	files, err := extractSubtree(archive, "skills/review", internalSubject, limits)

	var tooLarge *AssetTooLargeError
	require.ErrorAs(t, err, &tooLarge)
	assert.Nil(t, files)
	assert.Equal(t, int64(600), tooLarge.Limit)
}

func TestExtractSubtreeRefusesAnArchiveThatExpandsPastTheCeiling(t *testing.T) {
	t.Parallel()

	// The selected subtree is one small file and everything past the ceiling is
	// outside it, which is exactly what the per-file and per-asset ceilings cannot
	// see: the stream is still read to its end to find out the subtree is complete.
	archive := tinyArchive(t, map[string]string{
		"skills/review/SKILL.md": "review skill",
		"corpus/zeros.bin":       strings.Repeat("\x00", 200_000),
	})

	limits := generousLimits()
	limits.archive = 50_000

	files, err := extractSubtree(archive, "skills/review", internalSubject, limits)

	var tooLarge *ArchiveTooLargeError
	require.ErrorAs(t, err, &tooLarge)
	assert.Nil(t, files)
	assert.Equal(t, int64(50_000), tooLarge.Limit)
	assert.Less(t, len(archive), 50_000, "the compressed archive is well under the ceiling its expansion passes")
}

func TestExtractSubtreeReadsAnArchiveExactlyAtTheCeiling(t *testing.T) {
	t.Parallel()

	// A stream ending exactly at the limit is a complete archive, and a reader
	// that failed on it would refuse content harnaas is willing to hold.
	archive := tinyArchive(t, map[string]string{"skills/review/SKILL.md": "review skill"})

	expanded := decompressedSize(t, archive)
	limits := generousLimits()
	limits.archive = expanded

	files, err := extractSubtree(archive, "skills/review", internalSubject, limits)
	require.NoError(t, err)
	assert.Equal(t, map[string][]byte{"SKILL.md": []byte("review skill")}, files)
}

func TestBoundedStreamFailsRatherThanReportingTheEnd(t *testing.T) {
	t.Parallel()

	// A reader that stopped at the limit would hand the tar reader a truncated
	// archive, which reads as one that legitimately ends there.
	stream := &boundedStream{reader: strings.NewReader(strings.Repeat("x", 100)), limit: 10}

	buffer := make([]byte, 64)
	_, err := stream.Read(buffer)
	require.ErrorIs(t, err, errArchiveTooLarge)
}

func TestSafeEntryPathAcceptsAnOrdinaryName(t *testing.T) {
	t.Parallel()

	name, err := safeEntryPath("acme-assets/skills/./review/SKILL.md")
	require.NoError(t, err)
	assert.Equal(t, "acme-assets/skills/review/SKILL.md", name)
}

func TestSafeEntryPathReportsTheArchiveRootAsNothing(t *testing.T) {
	t.Parallel()

	// A tar written with a leading "./" names its own root, which selects nothing
	// and is not an escape either.
	name, err := safeEntryPath("./")
	require.NoError(t, err)
	assert.Empty(t, name)
}

// decompressedSize reports how many bytes an archive expands to, so a test can
// set a ceiling exactly at it.
func decompressedSize(tb testing.TB, archive []byte) int64 {
	tb.Helper()

	gz, err := gzip.NewReader(bytes.NewReader(archive))
	require.NoError(tb, err)
	defer gz.Close()

	var counter countingWriter
	_, err = io.Copy(&counter, gz)
	require.NoError(tb, err)
	return counter.written
}

// countingWriter counts what is written to it and keeps none of it.
type countingWriter struct{ written int64 }

func (w *countingWriter) Write(p []byte) (int, error) {
	w.written += int64(len(p))
	return len(p), nil
}
