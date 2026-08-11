package local

import (
	"fmt"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// Every diagnostic here names the asset and the path relative to the project
// root. The path is the manifest's own spelling — `.harnaas/skills/review`, not
// `skills/review` and not an absolute location on this machine — because the
// reader is holding the file that wrote it, and a message in any other spelling
// asks them to translate before they can act.

// MissingSourceError reports an asset whose content is not in the repository.
//
// A local source is the one kind whose failure is never about anybody else:
// nothing was unreachable and no credential was refused, so the whole message is
// the path that is not there and the two edits that fix it.
type MissingSourceError struct {
	// AssetID is the asset whose source was being resolved.
	AssetID string

	// Path is where the content was expected, relative to the project root.
	Path string
}

// Error states the problem and then the fix, as every user-facing diagnostic
// does.
func (e *MissingSourceError) Error() string {
	return fmt.Sprintf(
		"the asset %q reads from %s, and this repository has nothing there\n\n"+
			"Add the content at %s, or correct the asset's source in %s.",
		e.AssetID, e.Path, e.Path, manifest.FileName,
	)
}

// EmptySourceError reports a local source that exists and holds no files.
//
// It is separate from [MissingSourceError] because the two are different
// mistakes: one is a path nobody created, and this is a directory somebody
// created and left empty. Reporting it as missing would send an author to look
// for a directory they are looking at.
type EmptySourceError struct {
	// AssetID is the asset whose source was being resolved.
	AssetID string

	// Path is the directory, relative to the project root.
	Path string
}

// Error states the problem and then the fix.
func (e *EmptySourceError) Error() string {
	return fmt.Sprintf(
		"the asset %q reads from %s, which holds no files\n\n"+
			"Put the asset's content in %s, or remove the entry from %s.",
		e.AssetID, e.Path, e.Path, manifest.FileName,
	)
}

// ContainmentError reports a read the anchored handle refused because the path
// led out of `.harnaas`.
//
// The manifest grammar already refused every path that leaves `.harnaas` as
// text, so reaching this means the filesystem said something the manifest could
// not: a component of the path is a symbolic link to somewhere else. That is
// exactly the case an anchored handle exists for, and why this is a distinct
// diagnostic — an author reading "could not read" would go and check
// permissions on a file that is perfectly readable.
type ContainmentError struct {
	// AssetID is the asset whose source was being resolved.
	AssetID string

	// Path is the refused path, relative to the project root.
	Path string

	// Err is the refusal from the anchored handle, kept so the reason is still
	// inspectable.
	Err error
}

// Error states the problem and then the fix.
func (e *ContainmentError) Error() string {
	return fmt.Sprintf(
		"the asset %q reads from %s, which leads outside %s\n\n"+
			"Put the content itself under %s/ rather than a link to it, or declare the repository it comes from in the %q block of %s.",
		e.AssetID, e.Path, manifest.LocalRoot,
		manifest.LocalRoot, "sources", manifest.FileName,
	)
}

// Unwrap keeps the handle's refusal inspectable.
func (e *ContainmentError) Unwrap() error { return e.Err }

// UnsupportedEntryError reports something under a local source that is not a
// file harnaas can install.
//
// It reads like the archive's counterpart on purpose. A reader who has met one
// has met the same problem, and two wordings for one situation would suggest two
// situations.
type UnsupportedEntryError struct {
	// AssetID is the asset whose source was being resolved.
	AssetID string

	// Path is the entry, relative to the project root.
	Path string

	// EntryKind is what was found there, in the words the message uses.
	EntryKind string
}

// Error states the problem and then the fix.
func (e *UnsupportedEntryError) Error() string {
	return fmt.Sprintf(
		"the asset %q reads %s at %s, and harnaas installs only regular files\n\n"+
			"A harness asset is a document. Put the file itself where the link is, or point the asset at the file it leads to.",
		e.AssetID, e.EntryKind, e.Path,
	)
}

// ReadError reports a local source this machine would not let harnaas read.
//
// It is the residue: not missing, not an escape, not a link — a permission, a
// device that went away, a name the platform will not open. harnaas has nothing
// to add to what the system said, so the message quotes it and names the asset
// it was reading for.
type ReadError struct {
	// AssetID is the asset whose source was being resolved.
	AssetID string

	// Path is the path being read, relative to the project root.
	Path string

	// Err is the failure, kept whole so a cancelled or interrupted run is still
	// recognizable through errors.Is.
	Err error
}

// Error states the problem and then the fix.
func (e *ReadError) Error() string {
	return fmt.Sprintf(
		"could not read %s for the asset %q: %s\n\n"+
			"Check that this user can read %s, then run harnaas install again.",
		e.Path, e.AssetID, e.Err, e.Path,
	)
}

// Unwrap keeps the underlying failure inspectable.
func (e *ReadError) Unwrap() error { return e.Err }
