package source

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// digestPrefix names the algorithm inside the value itself, so a digest recorded
// in the lockfile today stays readable by a binary that hashes differently
// tomorrow: it can tell "computed another way" from "the content changed", which
// a bare hex string could not.
const digestPrefix = "sha256:"

// Digest is a content digest, rendered as its algorithm and hexadecimal sum.
//
// It is a named type because harnaas stores digests beside paths, ids and refs
// in the lockfile, and a function taking one should not silently accept any of
// those. Comparison is the only operation: harnaas asks whether two digests are
// equal and never what they mean.
type Digest string

// DigestContent returns the digest of one file's bytes.
func DigestContent(content []byte) Digest {
	sum := sha256.Sum256(content)
	return Digest(digestPrefix + hex.EncodeToString(sum[:]))
}

// digestFiles returns the digest of a resolved source as a whole.
//
// Paths participate alongside content, so renaming a file changes the value even
// though no byte of it did — a source whose skill moved to a different file is a
// different source, and a digest that said otherwise would let `lint` report an
// unchanged install for content that no longer lands anywhere.
//
// File modes are not hashed at all. Harness assets are documents, an executable
// bit carries no meaning worth preserving, and hashing one would make the same
// content digest differently on platforms that report modes differently — which
// would turn a Windows machine and a Linux machine into a permanent disagreement
// about whether upstream had moved.
//
// The files must already be sorted by path, which [NewResolved] guarantees: the
// value has to be independent of archive order and of filesystem iteration
// order, and sorting at the one place a [Resolved] is built is what makes that a
// property of the type rather than a step each kind remembers.
func digestFiles(files []File) Digest {
	var canonical bytes.Buffer
	for _, file := range files {
		// The path is length-prefixed because it is untrusted input: an archive
		// entry may contain any byte, including the separators. Without a frame,
		// two different sets of files could serialize identically and hash the
		// same, which is the one failure a content digest may never have.
		fmt.Fprintf(&canonical, "%d %s %s\n", len(file.Path), file.Path, file.Digest)
	}
	return DigestContent(canonical.Bytes())
}
