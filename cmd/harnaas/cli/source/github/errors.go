package github

import (
	"fmt"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// Every diagnostic here names the asset as well as the repository, because the
// person reading it is looking for the line to edit. A repository and a ref
// alone describe the failure accurately and say nothing about where it came
// from — one `sources` entry is referenced by any number of assets, and the
// manifest is what the reader has open.

// UnknownRefError reports a ref the repository does not have.
//
// It is a distinct type rather than a message because an unknown ref is the one
// ref failure that is always a manifest edit: the repository answered, harnaas
// reached it, and the name is simply not there.
type UnknownRefError struct {
	// AssetID is the asset whose source declared the ref.
	AssetID string

	// Repository is the `owner/repository` pair the manifest declared.
	Repository string

	// Ref is the ref as written, so a typo is visible in the message rather
	// than only in the file.
	Ref string
}

// Error states the problem and then the fix, as every user-facing diagnostic
// does.
func (e *UnknownRefError) Error() string {
	return fmt.Sprintf(
		"the asset %q asks for %s at %q, which that repository has no tag, branch or commit for\n\n"+
			"Change the ref after @ in %s to one that exists in %s, or pin the commit identifier directly.",
		e.AssetID, e.Repository, e.Ref, manifest.FileName, e.Repository,
	)
}

// RefLookupError reports a ref lookup that could not be made at all.
//
// A repository that is private, renamed, deleted or simply unreachable all
// arrive here, and they are one diagnostic rather than four because git's own
// message is the part that tells them apart and harnaas has nothing to add to
// it. What harnaas contributes is which asset was being resolved.
type RefLookupError struct {
	// AssetID is the asset whose source was being resolved.
	AssetID string

	// Repository is the `owner/repository` pair the manifest declared.
	Repository string

	// Ref is the ref as written. It is empty where the request declared none.
	Ref string

	// Remote is the Git remote harnaas asked, held raw and redacted where the
	// message is built — the same rule the transport diagnostics follow, so
	// redaction is a property of the type rather than a step a caller
	// constructing one has to remember.
	Remote string

	// Err is git's own failure, kept so a caller can still recognize a
	// cancelled run through errors.Is.
	Err error
}

// Error states the problem and then the fix.
func (e *RefLookupError) Error() string {
	return fmt.Sprintf(
		"could not resolve %s for the asset %q against %s: %s\n\n"+
			"Check that the repository exists and that this machine can read it — a private repository is resolved with the Git credentials already configured on this machine — then run harnaas install again.",
		e.describeRef(), e.AssetID, source.RedactURL(e.Remote), e.Err,
	)
}

// Unwrap keeps git's failure inspectable, which is what lets an interrupted run
// still be recognized as a cancellation rather than as a repository problem.
func (e *RefLookupError) Unwrap() error { return e.Err }

// describeRef names what was being resolved, which is a ref where the manifest
// declared one and the default branch where it did not.
func (e *RefLookupError) describeRef() string {
	if e.Ref == "" {
		return "the default branch of " + e.Repository
	}
	return fmt.Sprintf("%q in %s", e.Ref, e.Repository)
}

// GitUnavailableError reports that harnaas could not run git at all.
//
// It is separate from every other lookup failure because it is the one that is
// not about the manifest, the network or the repository: nothing the author
// could write in `harnaas.json` would fix it, so a message pointing at the file
// would send them to edit a file that is already correct.
type GitUnavailableError struct {
	// Err is the failure that reported git missing, kept for logs and for a
	// caller matching on exec.ErrNotFound.
	Err error
}

// Error states the problem and then the fix.
func (e *GitUnavailableError) Error() string {
	return fmt.Sprintf(
		"harnaas resolves a %s source's ref by running git, which is not installed on this machine\n\n"+
			"Install git and run harnaas install again, or pin each %s source to a full commit identifier, which needs no lookup.",
		manifest.SourceKindGitHub, manifest.SourceKindGitHub,
	)
}

// Unwrap keeps the underlying lookup failure inspectable.
func (e *GitUnavailableError) Unwrap() error { return e.Err }
