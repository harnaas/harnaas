package manifest

import (
	"fmt"
	"strconv"
)

// DocumentIndex marks a violation belonging to the document as a whole rather
// than to one asset entry — a malformed `sources` value, an unrecognized name
// in `harnesses`.
//
// It is negative so it sorts ahead of every asset entry without a special case
// in the comparison: a problem with the whole document is the one an author
// should read first, because it is the one that can explain the entries below
// it.
const DocumentIndex = -1

// Violation is one thing a decoded manifest says that harnaas cannot accept.
//
// Decoding stops at the first failure because a document that will not parse
// has no second problem to find. Interpretation does not stop: every asset entry
// is independent, so reporting one at a time would make a manifest with three
// mistakes three runs of the same command, each ending in a different sentence.
//
// A violation therefore carries where it is as data rather than only inside its
// sentence. That is what lets the aggregate order itself identically on every
// run — by [Violation.Index], then by [Violation.Field] — instead of inheriting
// the order a map happened to iterate in.
type Violation struct {
	// Index is the asset entry's position in `assets`, or [DocumentIndex] for a
	// violation about the document itself. It is the primary sort key.
	Index int

	// Field names the part of the document at fault in the author's own
	// vocabulary — `sources.acme`, `assets[2].scope` — so a reader knows which
	// line to open, and so two violations on one entry order deterministically.
	Field string

	// Problem states what is wrong, in one line.
	Problem string

	// Fix states the exact edit that resolves it. It is always an edit for a
	// person to make: harnaas does not write this file.
	Fix string
}

// String renders the location, the problem, a blank line, and the fix — the
// shape every user-facing diagnostic in harnaas takes.
//
// A violation is deliberately not an error. One of them is never what a command
// returns: interpretation collects them all and returns the aggregate, and a
// type that satisfied `error` would invite a caller to return the first one it
// saw, which is the behaviour accumulating exists to prevent.
func (v *Violation) String() string {
	return fmt.Sprintf("%s: %s\n\n%s", v.Field, v.Problem, v.Fix)
}

// sourceField names a `sources` entry, or the block itself where the entry has
// no key to be named by.
func sourceField(key string) string {
	if key == "" {
		return "sources"
	}
	return "sources." + key
}

// assetField names an asset entry.
func assetField(index int) string {
	return "assets[" + strconv.Itoa(index) + "]"
}

// assetSubject is how every message refers to an asset entry.
//
// An entry has no other stable name: its id may be the thing that is wrong, and
// its source string may be absent. The index is the one thing an author can
// always count to in the file, so it is worth spelling identically everywhere,
// which is why both decoding and interpretation phrase it here.
func assetSubject(index int) string {
	return fmt.Sprintf("the asset entry at index %d", index)
}

// sourceViolation reports a problem with a `sources` entry.
func sourceViolation(key, problem, fix string) *Violation {
	return &Violation{Index: DocumentIndex, Field: sourceField(key), Problem: problem, Fix: fix}
}

// assetSourceViolation reports a problem with an asset entry's source string.
//
// The source is attributed as a field even for an entry written in the string
// form, which declares no field name at all, because it is the field the object
// form would have carried and it is what a reader has to change either way.
func assetSourceViolation(index int, problem, fix string) *Violation {
	return &Violation{Index: index, Field: assetField(index) + ".source", Problem: problem, Fix: fix}
}
