package github

import (
	"context"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// Kind resolves every `github` source of one install run.
//
// It is built per run rather than registered as a value because it holds the
// run's archives: several assets naming different subtrees of one repository at
// one commit share a single retrieval, and that memory is only correct for as
// long as the run that made it.
type Kind struct {
	run      gitRunner
	archives *archives
}

// New returns the kind for one install run.
//
// It is what the source registry registers, and the fetcher it builds is the
// only way harnaas makes an HTTP request — so every transport rule applies here
// by there being no second route to a request.
func New() source.Kind {
	return newKind(runGit, source.NewFetcher().Fetch)
}

// newKind is [New] with the git invocation and the retrieval as parameters, so a
// whole resolution is exercisable with neither a network nor a repository.
func newKind(run gitRunner, fetch archiveFetcher) *Kind {
	return &Kind{run: run, archives: newArchives(fetch)}
}

// Resolve produces one asset's content and provenance.
//
// The three steps are deliberately separate: the declared ref becomes a commit
// before anything is fetched, the commit's archive is retrieved at most once per
// run, and the subtree the asset names is taken out of it. Every failure is
// attributed to the asset, and none of them returns a resolved source — an empty
// one would converge to deleting every file the asset had installed, which is
// why [source.NewResolved] refuses one and why nothing here builds a [source.Resolved]
// by any other route.
func (k *Kind) Resolve(ctx context.Context, req source.Request) (*source.Resolved, error) {
	resolution, err := resolveRef(ctx, k.run, remoteURL(req.Source.Repository), req)
	if err != nil {
		return nil, err
	}

	archive, err := k.archives.get(ctx, req.Source.Repository, resolution.Commit)
	if err != nil {
		return nil, retrievalFailure(req, resolution.Commit, err)
	}

	content, err := source.ExtractSubtree(archive, req.Asset.Ref.Path, source.ArchiveSubject{
		AssetID: req.Asset.ID,
		Source:  req.Source.String(),
		Commit:  resolution.Commit,
	})
	if err != nil {
		return nil, err
	}

	resolved, err := source.NewResolved(source.Provenance{
		Kind:   manifest.SourceKindGitHub,
		Source: req.Source.String(),
		// The ref is recorded beside the commit and never collapsed into it:
		// "the installed files still match the commit" and "the tag now points
		// somewhere else" are the two questions lint asks separately.
		RequestedRef:   req.Source.Ref,
		ResolvedCommit: resolution.Commit,
		Mutable:        resolution.Mutable,
	}, content)
	if err != nil {
		return nil, retrievalFailure(req, resolution.Commit, err)
	}

	return resolved, nil
}

// retrievalFailure attributes a retrieval that produced no usable archive to the
// asset that asked for it.
func retrievalFailure(req source.Request, commit string, err error) error {
	return &ArchiveRetrievalError{
		AssetID:    req.Asset.ID,
		Repository: req.Source.Repository,
		Commit:     commit,
		Err:        err,
	}
}
