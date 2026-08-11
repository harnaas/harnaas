package manifest

import (
	"fmt"
	"path/filepath"
	"strings"
)

// DecodeError reports a manifest harnaas could not decode.
//
// One type covers every decoding failure — malformed JSON, an unknown field, a
// wrongly shaped field, a version this binary cannot read — because they all
// end in the same place: a person opening `harnaas.json` and editing one line.
// What differs between them is the sentence, not the shape, and the shape is
// what a caller would branch on.
type DecodeError struct {
	// Path is the manifest the failure came from. It is empty for a document
	// decoded from bytes, where the message falls back to naming the file.
	Path string

	// Problem states what is wrong, in one line.
	Problem string

	// Fix states the edit that resolves it. It is always an edit for a person to
	// make: harnaas does not write this file.
	Fix string

	// Line and Column locate the problem in the file, both 1-based, and are zero
	// where the failure has no single location — an entry named by its index, or
	// a version that is wrong wherever it was written.
	Line, Column int
}

// Error renders the location, the problem, a blank line, and the fix.
func (e *DecodeError) Error() string {
	return fmt.Sprintf("%s: %s\n\n%s", e.where(), e.Problem, e.Fix)
}

// where names the file and, where one is known, the line and column inside it,
// in the `file:line:column` form an editor and a terminal both make clickable.
func (e *DecodeError) where() string {
	name := e.Path
	if name == "" {
		name = FileName
	}
	if e.Line <= 0 {
		return name
	}
	return fmt.Sprintf("%s:%d:%d", name, e.Line, e.Column)
}

// ValidationError is every violation one manifest contains, in one error.
//
// The aggregate is the type rather than a convenience over it, because reporting
// violations one at a time would turn a manifest with three mistakes into three
// runs of the same command, each ending in a different sentence. A caller that
// wants to render them itself reads [ValidationError.Violations], where each one
// still carries its index and field as data.
type ValidationError struct {
	// Path is the manifest the violations came from. It is empty for a document
	// decoded from bytes, where the message falls back to naming the file.
	Path string

	// Violations is every violation found, already ordered by asset index and
	// then by field. The order is fixed at construction rather than at render
	// time so a caller reading the slice sees what the message would have shown.
	Violations []*Violation
}

// Error names the file and how many problems it has, then renders each one.
func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Violations)+1)
	parts = append(parts, fmt.Sprintf("%s has %s", e.where(), problemCount(len(e.Violations))))
	for _, violation := range e.Violations {
		parts = append(parts, violation.String())
	}
	return strings.Join(parts, "\n\n")
}

// where names the manifest the violations belong to. Unlike a decode failure
// there is no single line to point at: the violations each name their own place
// in the document.
func (e *ValidationError) where() string {
	if e.Path == "" {
		return FileName
	}
	return e.Path
}

// problemCount renders the count the way the heading reads it, so a manifest
// with one problem is not told it has "1 problems".
func problemCount(count int) string {
	if count == 1 {
		return "1 problem"
	}
	return fmt.Sprintf("%d problems", count)
}

// NotFoundError reports a project with no manifest at its root.
//
// It is a distinct type because "there is no manifest yet" is not a broken
// manifest: it is the ordinary state of a project before `harnaas init`, and a
// caller may want to say something different about it.
type NotFoundError struct {
	// Root is the project root that was searched, so the message can show which
	// project harnaas resolved — the usual cause of surprise here is being in a
	// different repository than expected.
	Root string
}

// Error names the project searched and the command that creates the file.
func (e *NotFoundError) Error() string {
	return fmt.Sprintf(
		"no %s found in %s\n\n"+
			"Run `harnaas init` in that project to create one.",
		FileName, e.Root,
	)
}

// SubdirectoryError reports a `harnaas.json` below the project root.
//
// Such a file is an error rather than something harnaas quietly skips. A file
// with this name that harnaas ignores is worse than one it rejects: its author
// believes it is declaring assets, and every command silently disagrees.
// Merging it is not on the table either, since the effective asset set would
// then depend on which directory a command ran from, which is exactly the
// variation harnaas exists to remove.
type SubdirectoryError struct {
	// Path is the offending manifest's absolute path.
	Path string

	// Root is the project root, whose manifest is the only one harnaas reads.
	Root string
}

// Error names the file that will never be read and states the only manifest
// harnaas does read.
func (e *SubdirectoryError) Error() string {
	shown := e.Path
	if relative, err := filepath.Rel(e.Root, e.Path); err == nil {
		shown = filepath.ToSlash(relative)
	}

	return fmt.Sprintf(
		"found a second manifest at %s, which harnaas never reads\n\n"+
			"harnaas reads only the %s at the repository root (%s). "+
			"Move any entries you need into that file and delete %s.",
		shown, FileName, e.Root, shown,
	)
}
