package source_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// subject is the asset every archive in this file is extracted for. It carries
// all three fields because what a diagnostic has to name is the whole point of
// the type.
var subject = source.ArchiveSubject{
	AssetID: "review",
	Source:  "github:acme/assets",
	Commit:  "9f2a1c4e8b7d6a5f4e3c2b1a0987654321fedcba",
}

// archiveEntry is one entry to put in a test archive.
type archiveEntry struct {
	// name is the entry's name exactly as the archive should spell it, including
	// the top-level directory a repository archive wraps its content in.
	name string

	// typeflag is the tar entry type, zero meaning a regular file.
	typeflag byte

	// content is a regular file's bytes.
	content string

	// linkname is the target of a link entry.
	linkname string

	// pax holds the records of a pax global header, which is the only payload
	// tar allows that entry type to carry.
	pax map[string]string
}

// archiveOf builds a gzipped tar of the given entries, which is the shape a
// forge serves a repository archive in.
func archiveOf(tb testing.TB, entries ...archiveEntry) []byte {
	tb.Helper()

	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gz)

	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		// A pax global header carries records and nothing else; tar refuses to
		// encode one with any other field set.
		if typeflag == tar.TypeXGlobalHeader {
			require.NoError(tb, writer.WriteHeader(&tar.Header{
				Name:       entry.name,
				Typeflag:   typeflag,
				PAXRecords: entry.pax,
			}))
			continue
		}

		require.NoError(tb, writer.WriteHeader(&tar.Header{
			Name:     entry.name,
			Typeflag: typeflag,
			Linkname: entry.linkname,
			Size:     int64(len(entry.content)),
			Mode:     0o644,
		}))
		if entry.content != "" {
			_, err := writer.Write([]byte(entry.content))
			require.NoError(tb, err)
		}
	}

	require.NoError(tb, writer.Close())
	require.NoError(tb, gz.Close())
	return buffer.Bytes()
}

// repositoryArchive is a plausible asset repository: a pax global header, the
// wrapper directory every forge adds, one skill, one rule and content the
// declared subtree must not pick up.
func repositoryArchive(tb testing.TB) []byte {
	tb.Helper()

	return archiveOf(tb,
		archiveEntry{name: "pax_global_header", typeflag: tar.TypeXGlobalHeader, pax: map[string]string{"comment": "9f2a1c4"}},
		archiveEntry{name: "acme-assets-9f2a1c4/", typeflag: tar.TypeDir},
		archiveEntry{name: "acme-assets-9f2a1c4/README.md", content: "# assets"},
		archiveEntry{name: "acme-assets-9f2a1c4/skills/", typeflag: tar.TypeDir},
		archiveEntry{name: "acme-assets-9f2a1c4/skills/review/", typeflag: tar.TypeDir},
		archiveEntry{name: "acme-assets-9f2a1c4/skills/review/SKILL.md", content: "review skill"},
		archiveEntry{name: "acme-assets-9f2a1c4/skills/review/reference/checklist.md", content: "checklist"},
		archiveEntry{name: "acme-assets-9f2a1c4/skills/release/SKILL.md", content: "release skill"},
		archiveEntry{name: "acme-assets-9f2a1c4/rules/house-style.md", content: "house style"},
	)
}

func TestExtractSubtreeTakesOnlyTheDeclaredSubtree(t *testing.T) {
	t.Parallel()

	files, err := source.ExtractSubtree(repositoryArchive(t), "skills/review", subject)
	require.NoError(t, err)

	assert.Equal(t, map[string][]byte{
		"SKILL.md":               []byte("review skill"),
		"reference/checklist.md": []byte("checklist"),
	}, files, "the subtree's own files, named relative to it")
}

func TestExtractSubtreeOfASingleFileNamesItByItsLeaf(t *testing.T) {
	t.Parallel()

	// A rule, an instruction, a command and a persona are each one file rather
	// than a directory, so the path relative to the subtree is the file itself —
	// and an empty path is one [source.NewResolved] refuses outright.
	files, err := source.ExtractSubtree(repositoryArchive(t), "rules/house-style.md", subject)
	require.NoError(t, err)

	assert.Equal(t, map[string][]byte{"house-style.md": []byte("house style")}, files)
}

func TestExtractSubtreeStripsTheArchivesOwnWrapperDirectory(t *testing.T) {
	t.Parallel()

	// The wrapper is named for the repository and the commit, neither of which
	// appears in any path harnaas records.
	files, err := source.ExtractSubtree(repositoryArchive(t), "", subject)
	require.NoError(t, err)

	for path := range files {
		assert.NotContains(t, path, "acme-assets-9f2a1c4", "no path carries the wrapper directory")
	}
	assert.Contains(t, files, "README.md")
	assert.Contains(t, files, "skills/review/SKILL.md")
}

func TestExtractSubtreeSkipsDirectoryEntries(t *testing.T) {
	t.Parallel()

	files, err := source.ExtractSubtree(repositoryArchive(t), "skills", subject)
	require.NoError(t, err)

	assert.NotContains(t, files, "review", "a directory is implied by the files under it")
	assert.Contains(t, files, "review/SKILL.md")
}

func TestExtractSubtreeSkipsThePaxGlobalHeader(t *testing.T) {
	t.Parallel()

	// A repository archive puts the header first, so an extraction that treated
	// it as an ordinary entry would take it for the wrapper directory and find
	// every real entry disagreeing with it.
	archive := archiveOf(t,
		archiveEntry{name: "pax_global_header", typeflag: tar.TypeXGlobalHeader, pax: map[string]string{"comment": "9f2a1c4"}},
		archiveEntry{name: "acme-assets-9f2a1c4/rules/house-style.md", content: "house style"},
	)

	files, err := source.ExtractSubtree(archive, "rules", subject)
	require.NoError(t, err)
	assert.Equal(t, map[string][]byte{"house-style.md": []byte("house style")}, files)
}

func TestExtractSubtreeRefusesAnEntryThatEscapesTheRoot(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		entry string
	}{
		{name: "upward traversal", entry: "../outside.md"},
		{name: "traversal through a directory", entry: "acme-assets-9f2a1c4/skills/../../../outside.md"},
		{name: "posix absolute", entry: "/etc/passwd"},
		{name: "drive letter", entry: "C:/Windows/System32/drivers/etc/hosts"},
		{name: "unc", entry: `\\share\assets\SKILL.md`},
		{name: "backslash separator", entry: `acme-assets-9f2a1c4\skills\review\SKILL.md`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			archive := archiveOf(t,
				archiveEntry{name: "acme-assets-9f2a1c4/skills/review/SKILL.md", content: "review skill"},
				archiveEntry{name: tc.entry, content: "not this"},
			)

			files, err := source.ExtractSubtree(archive, "skills/review", subject)

			var unsafe *source.UnsafeEntryError
			require.ErrorAs(t, err, &unsafe)
			assert.Nil(t, files, "a refused archive yields no files at all, not the ones read before it")
			assert.Equal(t, tc.entry, unsafe.Entry, "the offending entry is held as the archive spells it")
			assert.Contains(t, unsafe.Error(), fmt.Sprintf("%q", tc.entry), "and named in the message")
		})
	}
}

func TestExtractSubtreeRefusesALinkWithinTheSubtree(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		typeflag byte
		linkname string
		want     string
	}{
		{name: "symbolic link", typeflag: tar.TypeSymlink, linkname: "../../../../etc/passwd", want: "a symbolic link"},
		{name: "hard link", typeflag: tar.TypeLink, linkname: "acme-assets-9f2a1c4/README.md", want: "a hard link"},
		{name: "character device", typeflag: tar.TypeChar, want: "a device"},
		{name: "block device", typeflag: tar.TypeBlock, want: "a device"},
		{name: "named pipe", typeflag: tar.TypeFifo, want: "a named pipe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			archive := archiveOf(t,
				archiveEntry{name: "acme-assets-9f2a1c4/skills/review/SKILL.md", content: "review skill"},
				archiveEntry{
					name:     "acme-assets-9f2a1c4/skills/review/secrets",
					typeflag: tc.typeflag,
					linkname: tc.linkname,
				},
			)

			files, err := source.ExtractSubtree(archive, "skills/review", subject)

			var unsupported *source.UnsupportedEntryError
			require.ErrorAs(t, err, &unsupported)
			assert.Nil(t, files)
			assert.Equal(t, tc.want, unsupported.EntryKind)
			assert.Contains(t, unsupported.Error(), "skills/review/secrets")
		})
	}
}

func TestExtractSubtreeIgnoresALinkOutsideTheSubtree(t *testing.T) {
	t.Parallel()

	// A link elsewhere in the repository is the repository's own arrangement of
	// its own files. Refusing the asset because of one would make an unrelated
	// part of somebody else's tree harnaas's business.
	archive := archiveOf(t,
		archiveEntry{name: "acme-assets-9f2a1c4/skills/review/SKILL.md", content: "review skill"},
		archiveEntry{name: "acme-assets-9f2a1c4/scripts/current", typeflag: tar.TypeSymlink, linkname: "../README.md"},
	)

	files, err := source.ExtractSubtree(archive, "skills/review", subject)
	require.NoError(t, err)
	assert.Equal(t, map[string][]byte{"SKILL.md": []byte("review skill")}, files)
}

func TestExtractSubtreeRefusesMoreThanOneTopLevelDirectory(t *testing.T) {
	t.Parallel()

	// Stripping the first component of an archive with two roots would discard
	// one of them in silence, and pick which by archive order.
	archive := archiveOf(t,
		archiveEntry{name: "acme-assets-9f2a1c4/skills/review/SKILL.md", content: "review skill"},
		archiveEntry{name: "somewhere-else/skills/review/SKILL.md", content: "not this"},
	)

	files, err := source.ExtractSubtree(archive, "skills/review", subject)

	var format *source.ArchiveFormatError
	require.ErrorAs(t, err, &format)
	assert.Nil(t, files)
	assert.Contains(t, format.Error(), "more than one top-level directory")
}

func TestExtractSubtreeReportsAPathThatIsNotThere(t *testing.T) {
	t.Parallel()

	files, err := source.ExtractSubtree(repositoryArchive(t), "skills/triage", subject)

	var missing *source.PathNotFoundError
	require.ErrorAs(t, err, &missing)
	assert.Nil(t, files)

	message := missing.Error()
	assert.Contains(t, message, `"review"`, "the asset the author would edit")
	assert.Contains(t, message, "skills/triage", "the path that is not there")
	assert.Contains(t, message, subject.Commit, "the commit it was looked for at")
}

func TestExtractSubtreeReportsSomethingThatIsNotAnArchive(t *testing.T) {
	t.Parallel()

	files, err := source.ExtractSubtree([]byte("<html>404</html>"), "skills/review", subject)

	var format *source.ArchiveFormatError
	require.ErrorAs(t, err, &format)
	assert.Nil(t, files)
}

func TestExtractSubtreeReportsATruncatedArchive(t *testing.T) {
	t.Parallel()

	whole := repositoryArchive(t)

	files, err := source.ExtractSubtree(whole[:len(whole)/2], "skills/review", subject)

	var format *source.ArchiveFormatError
	require.ErrorAs(t, err, &format)
	assert.Nil(t, files, "half an archive resolves to nothing rather than to the files it happened to reach")
}

func TestExtractedFilesAreAResolvableSource(t *testing.T) {
	t.Parallel()

	// Extraction produces exactly the mapping [source.NewResolved] consumes, which
	// is the whole of the contract between this phase and the next.
	files, err := source.ExtractSubtree(repositoryArchive(t), "skills/review", subject)
	require.NoError(t, err)

	resolved, err := source.NewResolved(source.Provenance{Source: subject.Source, ResolvedCommit: subject.Commit}, files)
	require.NoError(t, err)

	paths := make([]string, 0, len(resolved.Files))
	for _, file := range resolved.Files {
		paths = append(paths, file.Path)
	}
	assert.Equal(t, []string{"SKILL.md", "reference/checklist.md"}, paths)
}

// TestEveryExtractionFailureIsShapedProblemThenFix asserts the diagnostic
// contract over the whole extraction failure surface at once.
//
// The list is written out rather than derived, for the reason the transport list
// is: a test that asked the package which errors it declares would agree with any
// set. Adding an extraction diagnostic is two edits, and the second one is where
// somebody confirms it names the asset somebody has to go and edit.
func TestEveryExtractionFailureIsShapedProblemThenFix(t *testing.T) {
	t.Parallel()

	failures := map[string]error{
		"unsafe entry":      &source.UnsafeEntryError{Subject: subject, Entry: "../outside.md", Reason: "climbs above the root of the archive"},
		"unsupported entry": &source.UnsupportedEntryError{Subject: subject, Entry: "skills/review/link", EntryKind: "a symbolic link"},
		"file too large":    &source.FileTooLargeError{Subject: subject, Path: "reference/corpus.bin", Size: 9 << 20, Limit: 4 << 20},
		"asset too large":   &source.AssetTooLargeError{Subject: subject, Limit: 16 << 20},
		"archive too large": &source.ArchiveTooLargeError{Subject: subject, Limit: 256 << 20},
		"path not found":    &source.PathNotFoundError{Subject: subject, Path: "skills/triage"},
		"archive format":    &source.ArchiveFormatError{Subject: subject, Reason: "it is not the gzip stream harnaas expected", Err: gzip.ErrHeader},
	}

	for name, err := range failures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			message := err.Error()

			problem, fix, split := strings.Cut(message, "\n\n")
			require.True(t, split, "the problem and the fix must be separated by a blank line: %s", message)
			assert.NotContains(t, problem, "\n", "the problem is one line")
			assert.NotEmpty(t, fix)

			assert.Contains(t, problem, `"review"`, "a diagnostic names the asset it is about")
			assert.Contains(t, problem, "github:acme/assets", "and the source it came from")
		})
	}
}

func TestArchiveSubjectDescribesWhatItKnows(t *testing.T) {
	t.Parallel()

	// A local source has no commit, so a diagnostic must not claim one.
	err := &source.PathNotFoundError{
		Subject: source.ArchiveSubject{AssetID: "house-style", Source: ".harnaas/rules"},
		Path:    "rules/house-style.md",
	}

	assert.Contains(t, err.Error(), `the asset "house-style" from .harnaas/rules names`)
	assert.NotContains(t, err.Error(), "at commit")
}
