package github

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// A ref is not a commit. `v1.2.0` is a name in somebody else's repository, and
// the file harnaas installs has to be attributable to the object that name stood
// for on the day it ran — otherwise "did upstream move?" has no answer, and a
// lockfile recording the name records nothing.
//
// Resolution therefore happens before anything is fetched, and produces two
// facts rather than one: the commit, and whether the name that produced it can
// name a different commit tomorrow. The second is what separates a pin from a
// moving target for the lockfile and for `lint`.

// objectIDLengths are the lengths of a full Git object identifier, in the two
// hash algorithms Git has: SHA-1 and SHA-256.
//
// Only a full identifier is taken as one. An abbreviation is ambiguous by
// construction — it names whichever object it happens to be unique against
// today — and the manifest exists to pin content, so an abbreviation that grows
// a second match upstream would silently become a different install.
var objectIDLengths = []int{40, 64}

// RefResolution is what a declared ref resolved to.
type RefResolution struct {
	// Commit is the immutable commit identifier the ref named, lowercased,
	// which is the spelling Git itself produces and the lockfile records.
	Commit string

	// Mutable reports whether the ref that produced Commit can name a
	// different commit tomorrow. A branch can; a tag or a commit identifier
	// cannot.
	Mutable bool
}

// gitRunner runs git with the given arguments and returns its standard output.
//
// It is a parameter rather than a call, so the failures that matter here —
// git absent from the machine, a remote answering something that is not a
// ref listing — are exercisable without arranging for a real git to produce
// them. The same seam [source.Fetcher] uses for its size ceiling.
type gitRunner func(ctx context.Context, args ...string) ([]byte, error)

// ResolveRef resolves the ref one request's source declares to the commit it
// names, before any content is fetched.
func ResolveRef(ctx context.Context, req source.Request) (RefResolution, error) {
	return resolveRef(ctx, runGit, remoteURL(req.Source.Repository), req)
}

// resolveRef is [ResolveRef] with the git invocation and the remote as
// parameters.
func resolveRef(ctx context.Context, run gitRunner, remote string, req source.Request) (RefResolution, error) {
	ref := req.Source.Ref

	// A full commit identifier is already the answer. Asking the remote would
	// trade a round trip for a result that cannot differ from what the manifest
	// wrote, and would make a repository that is unreachable today fail a
	// resolution that needs nothing from it.
	if commit, ok := objectID(ref); ok {
		return RefResolution{Commit: commit}, nil
	}

	if ref == "" {
		return resolveDefaultBranch(ctx, run, remote, req)
	}

	// Every pattern is built with a `refs/` prefix, so no argument harnaas
	// passes can be read by git as an option — the ref arrives from a manifest
	// and is untrusted input in the same class as an archive entry's name.
	tag, peeled, branch := "refs/tags/"+ref, "refs/tags/"+ref+"^{}", "refs/heads/"+ref

	out, err := run(ctx, "ls-remote", remote, tag, peeled, branch)
	if err != nil {
		return RefResolution{}, lookupFailure(req, remote, err)
	}

	listed := listedRefs(out)

	// A tag beats a branch of the same name, which is Git's own precedence for
	// a bare name, and the peeled entry beats the tag it was peeled from: an
	// annotated tag's own object is a tag, and the commit is what an archive is
	// taken from.
	for _, candidate := range []struct {
		name    string
		mutable bool
	}{
		{name: peeled},
		{name: tag},
		{name: branch, mutable: true},
	} {
		id, listedRef := listed[candidate.name]
		if !listedRef {
			continue
		}

		commit, ok := objectID(id)
		if !ok {
			return RefResolution{}, lookupFailure(req, remote, fmt.Errorf(
				"the remote listed %q as %q, which is not a commit identifier", candidate.name, id,
			))
		}

		return RefResolution{Commit: commit, Mutable: candidate.mutable}, nil
	}

	return RefResolution{}, &UnknownRefError{
		AssetID:    req.Asset.ID,
		Repository: req.Source.Repository,
		Ref:        ref,
	}
}

// resolveDefaultBranch resolves the commit `HEAD` points at.
//
// The manifest grammar refuses a `github` source with no ref, so this is not a
// path a declared source reaches today. It is here because the source contract
// answers "what does an absent ref mean" for every kind, and the answer for a
// forge is the repository's default branch — leaving it unimplemented would make
// the first caller that has one invent a different answer.
//
// The result is mutable for the same reason a branch is: `HEAD` follows whatever
// branch the repository's owner points it at, and either can move.
func resolveDefaultBranch(ctx context.Context, run gitRunner, remote string, req source.Request) (RefResolution, error) {
	out, err := run(ctx, "ls-remote", remote, "HEAD")
	if err != nil {
		return RefResolution{}, lookupFailure(req, remote, err)
	}

	id, listed := listedRefs(out)["HEAD"]
	if !listed {
		return RefResolution{}, &UnknownRefError{
			AssetID:    req.Asset.ID,
			Repository: req.Source.Repository,
			Ref:        "HEAD",
		}
	}

	commit, ok := objectID(id)
	if !ok {
		return RefResolution{}, lookupFailure(req, remote, fmt.Errorf(
			"the remote listed HEAD as %q, which is not a commit identifier", id,
		))
	}

	return RefResolution{Commit: commit, Mutable: true}, nil
}

// listedRefs parses `git ls-remote` output into ref name to object identifier.
//
// A ref that the remote does not have is absent from the output rather than
// reported: `ls-remote` exits successfully having printed nothing, so "unknown
// ref" is a property of what came back and never of the exit status.
func listedRefs(out []byte) map[string]string {
	listed := make(map[string]string)

	for line := range strings.SplitSeq(string(out), "\n") {
		id, name, separated := strings.Cut(strings.TrimSpace(line), "\t")
		if !separated || id == "" || name == "" {
			continue
		}
		listed[name] = id
	}

	return listed
}

// objectID reports whether a string is a full Git object identifier, and
// returns it in the lowercase spelling Git itself writes.
//
// It guards two different things with one check. On the way in it decides
// whether a declared ref needs a remote lookup at all; on the way out it is what
// stops a remote's answer — untrusted text — from being pasted into an archive
// URL because it arrived on a line that looked like a ref listing.
func objectID(candidate string) (string, bool) {
	if !slices.Contains(objectIDLengths, len(candidate)) {
		return "", false
	}

	lowered := strings.ToLower(candidate)
	for _, r := range lowered {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return "", false
		}
	}

	return lowered, true
}

// lookupFailure turns a failed lookup into the diagnostic for it: git missing
// from the machine is a different problem with a different fix from a remote
// that refused, and only one of the two is about the manifest.
func lookupFailure(req source.Request, remote string, err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return &GitUnavailableError{Err: err}
	}

	return &RefLookupError{
		AssetID:    req.Asset.ID,
		Repository: req.Source.Repository,
		Ref:        req.Source.Ref,
		Remote:     remote,
		Err:        err,
	}
}

// runGit runs git and returns its standard output.
func runGit(ctx context.Context, args ...string) ([]byte, error) {
	// The executable is a constant, and every argument this package passes is
	// either a literal or a ref pattern built with a "refs/" prefix, so a ref
	// out of a manifest cannot arrive as something git reads as an option —
	// which TestARefLookupAsksOnlyForNamesUnderRefs is what holds it to.
	//nolint:gosec // G204: see above; the command is fixed and its arguments cannot be options.
	cmd := exec.CommandContext(ctx, "git", args...)

	// git must never stop to ask. harnaas's primary consumers are CI jobs and
	// coding agents, where a credential prompt is not a question anybody
	// answers — it is the run hanging until something kills it. Detaching stdin
	// closes the second route to the same outcome.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Stdin = nil

	// git puts everything a person needs on stderr, and an *exec.ExitError
	// reports nothing but a status number, so the two are folded together here
	// rather than at each call site.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}

	if reported := strings.Join(strings.Fields(stderr.String()), " "); reported != "" {
		return nil, fmt.Errorf("%w: %s", err, reported)
	}

	return nil, fmt.Errorf("running git: %w", err)
}
