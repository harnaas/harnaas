// Package source turns a declared source into content harnaas can install.
//
// The manifest layer parses a source and resolves nothing: `github:acme/assets@v1`
// is checked for shape, and whether that repository exists, whether the ref
// points anywhere and what is at the path are all questions it deliberately does
// not ask. This package answers them, and it is the only place in harnaas that
// reaches the network or reads content out of a repository.
//
// Two things shape it. Every source kind answers the same question — given a
// parsed source and a path within it, produce the files and the provenance
// needed to record what landed — so the kinds sit behind one contract and a
// [Registry] dispatches to them by name. And a resolved source is worthless
// unless it can be re-identified later: `lint` has to answer "did somebody edit
// this?" and "has upstream moved?" independently, months after the install, from
// nothing but what was recorded. That is why a [Resolved] carries a digest per
// file and one over the source as a whole, and why the only way to obtain one is
// a constructor that computes both.
package source

import (
	"context"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// Kind resolves the sources of one kind for one install run.
//
// It is created per run rather than registered as a value, because resolution is
// not stateless: a repository archive is fetched at most once per repository and
// commit per run, and the memory of what has already been fetched belongs to the
// run that did it. A kind shared across runs would either leak that memory
// between them or need a global cache to hold it.
type Kind interface {
	// Resolve produces the content and provenance for one asset's source.
	//
	// It is handed one asset rather than one `sources` entry because the unit
	// being resolved is the subtree an asset names, and because every diagnostic
	// raised here has to name the asset the author would go and edit.
	Resolve(ctx context.Context, req Request) (*Resolved, error)
}

// NewKind constructs a kind's resolver for one install run. It is what a kind
// registers, for the reason [Kind] gives.
type NewKind func(RunOptions) Kind

// RunOptions are the caller's choices for one resolution run.
//
// They are handed to every kind at construction rather than to [Kind.Resolve]
// per asset, because they describe the run and not the request: two assets of
// one install cannot disagree about whether the cache may be read.
type RunOptions struct {
	// Cache is where fetched content is reused from and kept for later runs. A
	// nil cache is the caller-facing bypass, and is deliberately the zero value:
	// a caller that has not said which cache to use has not said one may be
	// read, which is the answer that cannot surprise anybody.
	Cache *ArchiveCache
}

// Request is one asset's resolution: the asset being resolved and the `sources`
// entry it references.
//
// The asset travels whole rather than as the two or three fields a kind reads,
// because the fields a diagnostic needs are not the fields resolution needs — a
// message names the asset's id and its position in the manifest, and narrowing
// the request would make every kind's errors worse to keep the struct smaller.
type Request struct {
	// Asset is the manifest entry being resolved, already interpreted: its
	// reference, type and id are settled before anything is fetched.
	Asset manifest.Asset

	// Source is the parsed `sources` entry the asset references. It is the zero
	// value for the project-local form, which references no entry.
	Source manifest.Source
}

// Kind reports which source kind must resolve this request.
//
// The project-local form declares no `sources` entry and is local by definition
// of the manifest grammar, not by anything a caller decides. Answering it here,
// once, is what keeps the install flow above and the kinds below from each
// restating that rule — and from restating it differently.
func (r Request) Kind() manifest.SourceKind {
	if r.Asset.Ref.Local() {
		return manifest.SourceKindLocal
	}
	return r.Source.Kind
}
