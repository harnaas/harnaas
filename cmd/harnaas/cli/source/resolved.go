package source

import (
	"errors"
	"maps"
	"slices"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// Provenance records where a resolved source came from, in the terms the
// lockfile stores and `lint` later re-checks.
//
// The requested ref is kept alongside the commit it resolved to, and the two are
// never collapsed. A lockfile holding only the commit could say whether the
// installed files still match it, but not whether the tag or branch the manifest
// asked for now points somewhere else — which is the whole of update detection.
type Provenance struct {
	// Kind is the source kind that produced the content.
	Kind manifest.SourceKind

	// Source is the normalized source, spelled the way the lockfile records it,
	// so a recorded installation names its origin without the reader consulting
	// the manifest that was current when it ran.
	Source string

	// RequestedRef is the ref the manifest asked for, exactly as written. It is
	// empty for a kind that has no refs.
	RequestedRef string

	// ResolvedCommit is the immutable commit the request resolved to, and the
	// commit the content was taken from. It is empty for a kind whose content
	// has no commit.
	ResolvedCommit string

	// Mutable reports whether the requested ref can point somewhere else
	// tomorrow — a branch can, a tag or a commit identifier cannot.
	//
	// It is about the ref and not about the content: a local source declares no
	// ref and so is never mutable, even though the file it names is edited
	// freely. Whether the bytes changed is a question the digests answer.
	Mutable bool
}

// File is one file of a resolved source.
type File struct {
	// Path is the file's location relative to the source's own root, cleaned
	// and slash-separated on every platform, so a lockfile written on Windows
	// and one written on Linux record the same install.
	Path string

	// Content is the file's bytes, exactly as they arrived. Resolution never
	// rewrites content — not even a skill's frontmatter, whose `name` it checks
	// and refuses to correct.
	Content []byte

	// Digest is the digest of Content, computed once here so no later phase has
	// to re-read the file to say which of several files changed.
	Digest Digest
}

// Resolved is one asset's source, fetched and verified: every file it consists
// of, the digests that re-identify them, and where they came from.
//
// The content is held in memory rather than left in a directory a caller must
// clean up. Harness assets are documents and resolution bounds their total size
// before anything is read, so the bytes cost little; what it buys is that
// rendering and digesting are pure functions of a value, with no temporary
// directory whose lifetime has to outlive the phase that created it.
type Resolved struct {
	// Provenance is where the content came from.
	Provenance Provenance

	// Files is every resolved file, sorted by path. Sorted rather than mapped
	// because this order is what the digest is computed over, what the lockfile
	// records and what a report lists — three places that must not disagree.
	Files []File

	// Digest covers the whole source: every path and every file's digest. It is
	// what says "upstream moved", independently of the installed digest that
	// says "somebody edited this".
	Digest Digest
}

// errEmptySource reports a resolved source with no files in it.
//
// It is a distinct error because the condition it prevents is specific: a
// retrieval that failed halfway must never be handed on as a source that
// legitimately resolved to nothing, which would converge to deleting every file
// the asset had installed. Making the constructor refuse it is what turns that
// from a rule each kind follows into one none of them can break.
var errEmptySource = errors.New("resolved source contains no files")

// errEmptyPath reports a file with no path, which would leave the whole-source
// digest unable to distinguish it from a file that was not there at all.
var errEmptyPath = errors.New("resolved source names a file with an empty path")

// NewResolved builds a resolved source from its provenance and its files, given
// as the path-to-content mapping extraction produces.
//
// It is the only way to obtain a [Resolved], so every value of that type has
// sorted files, a digest per file and a whole-source digest computed the same
// way — a kind cannot produce one whose digest disagrees with its content, and
// no later phase has to check that it did.
func NewResolved(provenance Provenance, content map[string][]byte) (*Resolved, error) {
	if len(content) == 0 {
		return nil, errEmptySource
	}

	files := make([]File, 0, len(content))
	for _, path := range slices.Sorted(maps.Keys(content)) {
		if path == "" {
			return nil, errEmptyPath
		}
		files = append(files, File{
			Path:    path,
			Content: content[path],
			Digest:  DigestContent(content[path]),
		})
	}

	return &Resolved{Provenance: provenance, Files: files, Digest: digestFiles(files)}, nil
}
