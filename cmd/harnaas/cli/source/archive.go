package source

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// An archive arrives as bytes a forge produced from content harnaas did not
// write, so every name in it is untrusted input in the same class as an asset id
// or a manifest path. What extraction has to guarantee is that the files it hands
// on are files: named relative to the subtree the manifest asked for, staying
// inside it, holding content rather than a pointer at something else on the
// machine, and bounded.
//
// Nothing here writes to disk. A resolved source is held in memory, so "no
// partially extracted content is left behind" is not a cleanup step that could be
// skipped on a path somebody forgot — a failed extraction returns no map at all,
// and there is nowhere else the bytes could have gone.

// maxArchiveBytes is the ceiling on what one archive may expand to.
//
// It is separate from the ceiling on the compressed body because neither of the
// other two limits catches the failure it exists for: a gzip stream well under
// the transport ceiling can expand to hundreds of times its size, and an archive
// whose selected subtree is one small file is still read to its end to find that
// out. Bounding the decompressed stream is the only limit that sees those bytes.
const maxArchiveBytes int64 = 256 << 20

// maxAssetBytes is the ceiling on everything one asset's subtree extracts to.
//
// A skill is a directory of documents. A subtree past this is nearly always a
// path that names a directory containing the asset rather than the asset, which
// is a manifest edit rather than a limit to raise.
const maxAssetBytes int64 = 16 << 20

// maxAssetFileBytes is the ceiling on a single file within an asset.
//
// It is much smaller than the subtree ceiling on purpose: the subtree limit is
// about how much an asset may be, and this one is about what a harness asset is.
// A single document past this is not one, and reporting the file by name is a far
// better diagnostic than reporting that the total was exceeded somewhere.
const maxAssetFileBytes int64 = 4 << 20

// errArchiveTooLarge reports a decompressed stream past its ceiling. It is a
// sentinel because it surfaces from inside the tar reader, which is the only
// place that reads the stream at all.
var errArchiveTooLarge = errors.New("archive expands past the limit")

// extractionLimits are the ceilings one extraction runs under.
//
// They are a value rather than the constants read directly, so the failure past
// each one can be exercised without building an archive that meets it — the same
// seam [Fetcher.fetch] uses for the response ceiling.
type extractionLimits struct {
	// archive bounds the decompressed stream.
	archive int64

	// asset bounds everything the selected subtree extracts to.
	asset int64

	// file bounds one file within the subtree.
	file int64
}

// productionLimits are the ceilings a real install runs under.
func productionLimits() extractionLimits {
	return extractionLimits{archive: maxArchiveBytes, asset: maxAssetBytes, file: maxAssetFileBytes}
}

// ArchiveSubject names what an extraction is for.
//
// It travels with the archive because every diagnostic extraction raises is about
// a manifest entry rather than about an archive: the person reading it is looking
// for the line to edit, and "entry 4 climbs above the root" without the asset it
// came from names nothing they can act on. The commit is carried for the same
// reason a lockfile keeps it — the answer to "the path is not there" is different
// depending on which commit it was not there at.
type ArchiveSubject struct {
	// AssetID is the asset being resolved, as the manifest names it.
	AssetID string

	// Source is the normalized source the archive came from, spelled the way the
	// manifest declares it.
	Source string

	// Commit is the commit the archive is of. It is empty for a kind whose
	// content has no commit.
	Commit string
}

// describe renders the subject as the grammatical subject of a sentence, so one
// wording serves every diagnostic in this file.
func (s ArchiveSubject) describe() string {
	described := "this asset"
	if s.AssetID != "" {
		described = fmt.Sprintf("the asset %q", s.AssetID)
	}
	if s.Source != "" {
		described += " from " + s.Source
	}
	if s.Commit != "" {
		described += " at commit " + s.Commit
	}
	return described
}

// ExtractSubtree takes one asset's subtree out of a repository archive and
// returns its files, keyed by path relative to that subtree.
//
// The archive is a gzipped tar whose content is wrapped in a single top-level
// directory, which is how every forge that serves repository archives produces
// one. The wrapper is taken from the archive rather than reconstructed from the
// repository and commit: its spelling is the forge's to choose, and a harnaas
// that predicted it would be one forge release away from selecting nothing and
// reporting the asset's path as missing. An archive with more than one top-level
// directory is refused instead, because "strip the first component" would
// otherwise discard content in silence.
func ExtractSubtree(archive []byte, subtree string, subject ArchiveSubject) (map[string][]byte, error) {
	return extractSubtree(archive, subtree, subject, productionLimits())
}

// extractSubtree is [ExtractSubtree] with the ceilings as a parameter.
func extractSubtree(
	archive []byte,
	subtree string,
	subject ArchiveSubject,
	limits extractionLimits,
) (map[string][]byte, error) {
	// The manifest grammar already cleaned this path and refused one that leaves
	// the source's root, so the only work here is spelling the whole-archive case
	// the same way the selection test expects it.
	prefix := path.Clean(subtree)
	if prefix == "." {
		prefix = ""
	}

	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, &ArchiveFormatError{Subject: subject, Reason: "it is not the gzip stream harnaas expected", Err: err}
	}
	defer gz.Close()

	reader := tar.NewReader(&boundedStream{reader: gz, limit: limits.archive})
	run := &extraction{subject: subject, prefix: prefix, limits: limits, files: map[string][]byte{}}

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, run.readFailure(err)
		}
		if err := run.consider(header, reader); err != nil {
			return nil, err
		}
	}

	if len(run.files) == 0 {
		return nil, &PathNotFoundError{Subject: subject, Path: subtree}
	}
	return run.files, nil
}

// extraction is the state one pass over an archive accumulates: what has been
// selected, how much of the budget it has spent, and which top-level directory
// every entry has to agree on.
type extraction struct {
	subject ArchiveSubject
	prefix  string
	limits  extractionLimits

	files map[string][]byte
	root  string
	// rootKnown distinguishes "no entry has named the root yet" from an archive
	// whose root is legitimately the empty string, which no archive's is.
	rootKnown bool
	total     int64
}

// consider takes one archive entry: checks it, decides whether the subtree
// selected it, and reads it if so.
func (e *extraction) consider(header *tar.Header, reader io.Reader) error {
	// A repository archive begins with a pax global header carrying the commit,
	// which is an entry in the tar sense and a file in no other. Skipping it here,
	// before anything looks at names, is what stops it from being mistaken for the
	// archive's top-level directory.
	if header.Typeflag == tar.TypeXGlobalHeader {
		return nil
	}

	name, err := safeEntryPath(header.Name)
	if err != nil {
		return &UnsafeEntryError{Subject: e.subject, Entry: header.Name, Reason: err.Error()}
	}
	if name == "" {
		return nil
	}

	// Containment is checked for every entry and not only for the selected ones.
	// An entry that climbs above the root cannot be classified as inside or
	// outside the subtree in the first place, and an archive containing one is not
	// an archive harnaas will take anything from.
	root, rest, _ := strings.Cut(name, "/")
	if !e.rootKnown {
		e.root, e.rootKnown = root, true
	}
	if root != e.root {
		return &ArchiveFormatError{
			Subject: e.subject,
			Reason:  "it holds more than one top-level directory, so harnaas cannot tell which one is the repository",
		}
	}
	if rest == "" {
		return nil
	}

	relative, selected := e.selection(rest)
	if !selected {
		return nil
	}

	switch header.Typeflag {
	case tar.TypeDir:
		// A directory is implied by the files under it, and an empty one carries
		// nothing to install.
		return nil
	case tar.TypeReg:
	default:
		// Only entries the subtree selected are rejected for their kind. A link
		// elsewhere in the repository is somebody else's arrangement of their own
		// files, and refusing to install an asset because of one would make an
		// unrelated part of the repository harnaas's business.
		return &UnsupportedEntryError{Subject: e.subject, Entry: header.Name, EntryKind: entryKind(header.Typeflag)}
	}

	return e.take(header, reader, relative)
}

// selection reports whether the subtree takes an entry, and under what name.
//
// A subtree naming a single file — a rule or a command is one file, not a
// directory — resolves to that file under its own leaf name, because the path
// relative to a file is nothing and a resolved file with no path could not be
// told from one that was not there.
func (e *extraction) selection(repoPath string) (string, bool) {
	switch {
	case e.prefix == "":
		return repoPath, true
	case repoPath == e.prefix:
		return path.Base(repoPath), true
	case strings.HasPrefix(repoPath, e.prefix+"/"):
		return strings.TrimPrefix(repoPath, e.prefix+"/"), true
	default:
		return "", false
	}
}

// take reads one selected file, under the per-file and per-asset ceilings.
func (e *extraction) take(header *tar.Header, reader io.Reader, relative string) error {
	// Checked from the header rather than after the read, so an entry declaring
	// more than harnaas will hold is refused before any of it is allocated.
	if header.Size > e.limits.file {
		return &FileTooLargeError{Subject: e.subject, Path: relative, Size: header.Size, Limit: e.limits.file}
	}
	e.total += header.Size
	if e.total > e.limits.asset {
		return &AssetTooLargeError{Subject: e.subject, Limit: e.limits.asset}
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		return e.readFailure(err)
	}
	e.files[relative] = content
	return nil
}

// readFailure turns a failure to read the stream into the diagnostic it is: the
// expansion ceiling surfaces from inside the tar reader, and everything else is
// an archive harnaas could not read to the end.
func (e *extraction) readFailure(err error) error {
	if errors.Is(err, errArchiveTooLarge) {
		return &ArchiveTooLargeError{Subject: e.subject, Limit: e.limits.archive}
	}
	return &ArchiveFormatError{Subject: e.subject, Reason: "harnaas could not read it to the end", Err: err}
}

// safeEntryPath returns an archive entry's name as a relative slash-separated
// path, or reports why harnaas will not treat it as one.
//
// Both spellings of an absolute path are refused on every platform, and so is a
// backslash, for the reason the manifest grammar refuses them: the same archive
// is extracted on Windows and on Linux, and a name that means a different file on
// each is one harnaas cannot install the same way twice.
func safeEntryPath(name string) (string, error) {
	switch {
	case name == "":
		return "", errors.New("names nothing at all")
	case strings.ContainsRune(name, '\\'):
		return "", errors.New("separates its path with a backslash, which names a different file on Windows than on Linux")
	case isAbsoluteEntry(name):
		return "", errors.New("names an absolute location on the machine rather than a path inside the archive")
	}

	cleaned := path.Clean(name)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", errors.New("climbs above the root of the archive")
	}
	if cleaned == "." {
		return "", nil
	}
	return cleaned, nil
}

// isAbsoluteEntry reports whether an archive entry names a location on the
// machine. It recognizes the POSIX, UNC and drive-letter spellings on every
// platform, because the archive that carries one is read on all of them.
func isAbsoluteEntry(name string) bool {
	if strings.HasPrefix(name, "/") {
		return true
	}
	if len(name) < 3 || name[1] != ':' {
		return false
	}
	letter := name[0] | 0x20
	return letter >= 'a' && letter <= 'z' && name[2] == '/'
}

// entryKind names an archive entry's type in the words a person would use, so a
// refusal says what was found rather than quoting a tar type flag.
func entryKind(typeflag byte) string {
	switch typeflag {
	case tar.TypeSymlink:
		return "a symbolic link"
	case tar.TypeLink:
		return "a hard link"
	case tar.TypeChar, tar.TypeBlock:
		return "a device"
	case tar.TypeFifo:
		return "a named pipe"
	default:
		return "an entry that is not a file"
	}
}

// boundedStream fails past a total, rather than stopping at it.
//
// The distinction is the same one [readWithinLimit] makes and matters more here:
// a reader that reported EOF at the ceiling would hand the tar reader a truncated
// archive, which reads as an archive that legitimately ends there — and an asset
// resolved from it would record a digest over content that was silently cut in
// half.
type boundedStream struct {
	reader io.Reader
	limit  int64
	read   int64
}

// Read fills p, failing once the stream has produced more than the limit. The
// comparison is on the total after the read rather than before it, so a stream of
// exactly the limit is read to its end and succeeds.
func (s *boundedStream) Read(p []byte) (int, error) {
	n, err := s.reader.Read(p)
	s.read += int64(n)
	if s.read > s.limit {
		return n, errArchiveTooLarge
	}
	return n, err //nolint:wrapcheck // the caller's own reader's error, passed through unchanged.
}

// UnsafeEntryError reports an archive entry whose name would put content
// somewhere other than inside the asset.
type UnsafeEntryError struct {
	// Subject is the asset the archive was fetched for.
	Subject ArchiveSubject

	// Entry is the entry's name exactly as the archive spells it, which is the
	// only spelling that helps somebody look for it.
	Entry string

	// Reason completes "the entry ...", naming which rule the name broke.
	Reason string
}

// Error states the problem and then the fix.
func (e *UnsafeEntryError) Error() string {
	return fmt.Sprintf(
		"the archive for %s holds the entry %q, which %s\n\n"+
			"harnaas extracts only entries that stay inside the archive. "+
			"Declare a source whose archive does, or put the content in a directory under %s and declare it as a local source.",
		e.Subject.describe(), e.Entry, e.Reason, manifest.LocalRoot,
	)
}

// UnsupportedEntryError reports an entry within the selected subtree that is not
// a file harnaas can install.
type UnsupportedEntryError struct {
	// Subject is the asset the archive was fetched for.
	Subject ArchiveSubject

	// Entry is the entry's name exactly as the archive spells it.
	Entry string

	// EntryKind is what was found there, in the words the message uses.
	EntryKind string
}

// Error states the problem and then the fix.
func (e *UnsupportedEntryError) Error() string {
	return fmt.Sprintf(
		"the archive for %s holds %s at %q, and harnaas installs only regular files\n\n"+
			"A harness asset is a document. Ask the repository to hold the file itself where the link is, "+
			"or put the content in a directory under %s and declare it as a local source.",
		e.Subject.describe(), e.EntryKind, e.Entry, manifest.LocalRoot,
	)
}

// FileTooLargeError reports one file past the per-file ceiling.
type FileTooLargeError struct {
	// Subject is the asset the archive was fetched for.
	Subject ArchiveSubject

	// Path is the file's path relative to the asset's subtree, which is where
	// somebody reading the manifest would go looking for it.
	Path string

	// Size is what the archive declares the file to be.
	Size int64

	// Limit is the ceiling, so the message states the number to come under.
	Limit int64
}

// Error states the problem and then the fix.
func (e *FileTooLargeError) Error() string {
	return fmt.Sprintf(
		"%s includes %q at %d bytes, past the %d bytes harnaas installs as one file\n\n"+
			"A harness asset is a document, so a file this large is content that came along with one rather than part of it. "+
			"Declare a source path that names the asset itself.",
		e.Subject.describe(), e.Path, e.Size, e.Limit,
	)
}

// AssetTooLargeError reports a selected subtree past the per-asset ceiling.
type AssetTooLargeError struct {
	// Subject is the asset the archive was fetched for.
	Subject ArchiveSubject

	// Limit is the ceiling in bytes.
	Limit int64
}

// Error states the problem and then the fix.
func (e *AssetTooLargeError) Error() string {
	return fmt.Sprintf(
		"%s extracts to more than the %d bytes harnaas installs for one asset\n\n"+
			"This is nearly always a path naming a directory that contains the asset rather than the asset. "+
			"Narrow the entry's source path in %s to the skill or file itself.",
		e.Subject.describe(), e.Limit, manifest.FileName,
	)
}

// ArchiveTooLargeError reports an archive that expands past the ceiling.
type ArchiveTooLargeError struct {
	// Subject is the asset the archive was fetched for.
	Subject ArchiveSubject

	// Limit is the ceiling in bytes.
	Limit int64
}

// Error states the problem and then the fix.
func (e *ArchiveTooLargeError) Error() string {
	return fmt.Sprintf(
		"the archive for %s expands to more than the %d bytes harnaas reads from one source\n\n"+
			"Harness assets are documents, so a repository this large is one that happens to contain them rather than the repository they live in. "+
			"Declare a source that holds the assets themselves.",
		e.Subject.describe(), e.Limit,
	)
}

// PathNotFoundError reports a source path that is not in the repository at the
// commit it resolved to.
type PathNotFoundError struct {
	// Subject is the asset the archive was fetched for, which carries the commit
	// the path was looked for at.
	Subject ArchiveSubject

	// Path is the path the manifest named, as it is written there.
	Path string
}

// Error states the problem and then the fix.
//
// The commit is part of the problem rather than incidental to it: a path that
// was there yesterday and is not there at a pinned commit is a different
// mistake from a path that was never right, and only the commit distinguishes
// them.
func (e *PathNotFoundError) Error() string {
	// A kind whose content has no commit records none, and a fix telling its
	// author to check "that commit" would send them looking for something the
	// problem never mentioned.
	fix := fmt.Sprintf("Check the path against the source and correct it in %s.", manifest.FileName)
	if e.Subject.Commit != "" {
		fix = fmt.Sprintf(
			"Check the path against the repository at that commit and correct it in %s, or pin a ref that has it.",
			manifest.FileName,
		)
	}

	return fmt.Sprintf("%s names the path %q, which is not there\n\n%s", e.Subject.describe(), e.Path, fix)
}

// ArchiveFormatError reports an archive harnaas could not take content out of.
type ArchiveFormatError struct {
	// Subject is the asset the archive was fetched for.
	Subject ArchiveSubject

	// Reason completes "the archive ...", naming what was wrong with it.
	Reason string

	// Err is the underlying failure where there was one, kept so a cancelled run
	// stays recognizable through errors.Is.
	Err error
}

// Error states the problem and then the fix.
func (e *ArchiveFormatError) Error() string {
	return fmt.Sprintf(
		"the archive for %s could not be extracted because %s\n\n"+
			"A partial download is the usual cause, so run harnaas install again. "+
			"If it fails the same way, the repository's archive is not one harnaas can read, and the content has to be declared as a local source under %s.",
		e.Subject.describe(), e.Reason, manifest.LocalRoot,
	)
}

// Unwrap keeps the cause reachable.
func (e *ArchiveFormatError) Unwrap() error { return e.Err }
