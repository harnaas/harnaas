package github

import (
	"context"
	"errors"
	"net/http"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// init registers this kind with [source.Default], so a binary that links this
// package is a binary that resolves `github` sources. Registration lives here
// rather than in the install flow because the alternative is a list of kinds
// maintained somewhere else that a new kind has to remember to join — and the
// failure that produces is a kind which compiles, passes its own tests and is
// missing from the registry.
//
//nolint:gochecknoinits // Self-registration is the source-kind contract; see [source.Registry].
func init() {
	source.Register(manifest.SourceKindGitHub, New)
}

// Kind resolves every `github` source of one install run.
//
// It is built per run rather than registered as a value because it holds the
// run's archives: several assets naming different subtrees of one repository at
// one commit share a single retrieval, and that memory is only correct for as
// long as the run that made it.
type Kind struct {
	run      gitRunner
	archives *archives

	// credential is the run's token, read from the environment chain once at
	// construction rather than per retrieval. A run is one identity: reading the
	// chain per asset would let a variable changed mid-run split one install
	// into two identities, and would make "which token was this?" a question
	// with a different answer for each diagnostic.
	credential source.Credential

	// offline is the run's answer to whether the network may be reached, held
	// here as well as in the archive memo because a resolution makes two kinds
	// of request and this is where the first one is decided.
	offline bool
}

// New returns the kind for one install run.
//
// It is what the source registry registers, and the fetcher it builds is the
// only way harnaas makes an HTTP request — so every transport rule applies here
// by there being no second route to a request.
func New(opts source.RunOptions) source.Kind {
	return newKind(runGit, source.NewFetcher().Fetch, ambientCredential(), opts)
}

// newKind is [New] with the git invocation, the retrieval and the credential as
// parameters, so a whole resolution is exercisable with neither a network, a
// repository nor a token in the environment the suite runs under. The run's own
// choices arrive as the options they arrive as everywhere else, so a field added
// to them reaches this kind without another parameter.
func newKind(run gitRunner, fetch archiveFetcher, credential source.Credential, opts source.RunOptions) *Kind {
	return &Kind{
		run:        run,
		archives:   newArchives(fetch, opts),
		credential: credential,
		offline:    opts.Offline,
	}
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
	resolution, err := k.resolution(ctx, req)
	if err != nil {
		return nil, err
	}

	archive, err := k.archives.get(ctx, req.Source.Repository, resolution.Commit, k.credential)
	if err != nil {
		return nil, k.retrievalFailure(req, resolution.Commit, err)
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
		return nil, k.retrievalFailure(req, resolution.Commit, err)
	}

	return resolved, nil
}

// resolution turns the declared ref into the commit it names, by the one route
// the run has: the remote where the network is available, and the ref itself
// where it is not.
//
// The branch is here rather than inside the lookup so that an offline run makes
// no request at all — a lookup that decided to refuse would still be a lookup,
// and "no ref lookup is attempted" is the property offline mode is asked for.
func (k *Kind) resolution(ctx context.Context, req source.Request) (RefResolution, error) {
	if k.offline {
		return resolveRefOffline(req)
	}

	return resolveRef(ctx, k.run, remoteURL(req.Source.Repository), req)
}

// retrievalFailure attributes a retrieval that produced no usable archive to the
// asset that asked for it.
//
// A refusal is separated from every other failure here rather than in the memo,
// because the memo holds what the transport answered and nothing about which
// asset read it — and because the credential is the run's, not the archive's. A
// second asset meeting the remembered refusal therefore gets a message naming
// itself and the same token.
func (k *Kind) retrievalFailure(req source.Request, commit string, err error) error {
	// An offline miss is checked first because it is not a failure of the
	// retrieval: nothing was asked, so there is no host, status or cause to
	// report, and the only fix is a run with the network available.
	if errors.Is(err, errNotCached) {
		return &OfflineArchiveError{
			AssetID:    req.Asset.ID,
			Repository: req.Source.Repository,
			Commit:     commit,
		}
	}

	var status *source.StatusError
	if errors.As(err, &status) && deniesAccess(status.StatusCode) {
		return &AuthorizationError{
			AssetID:     req.Asset.ID,
			Repository:  req.Source.Repository,
			Commit:      commit,
			TokenOrigin: k.credential.Origin,
			Err:         err,
		}
	}

	return &ArchiveRetrievalError{
		AssetID:    req.Asset.ID,
		Repository: req.Source.Repository,
		Commit:     commit,
		Err:        err,
	}
}

// deniesAccess reports whether a status is the forge declining to serve this
// request rather than a failure of any other kind.
//
// Not found is one of them, which is not the obvious reading. GitHub answers 404
// rather than 403 for a repository the request may not see, so that a stranger
// cannot learn a private repository exists by asking — and by the time an
// archive is fetched, ref resolution has already proved that the repository
// exists and holds this commit. It proved it over Git, with the credentials the
// user's Git configuration supplies, which are not the token this request
// carries. A "not found" here is therefore the forge declining to admit to
// harnaas's *HTTP* identity what it just admitted to its Git one.
func deniesAccess(status int) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return true
	default:
		return false
	}
}
