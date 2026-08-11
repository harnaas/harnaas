package github

import (
	"context"
	"sync"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// Content arrives as one archive per repository and commit, never as one request
// per file. A skill is a directory of documents, so a per-file route would spend
// a listing plus a request per file against a budget a CI job installing a
// handful of assets exhausts — and would give the cache a set of files to
// reconcile instead of one immutable object named by the commit it is of.
//
// The consequence is this file: several assets legitimately name different
// subtrees of one repository at one commit, so retrieval has to be asked for per
// asset and performed once. The memory of what has already been fetched is per
// run, which is why the source contract registers a constructor rather than a
// value — a memo shared between runs would either leak content into a later one
// or need a global to hold it.

// archiveHost is the endpoint archives are fetched from, and the only place its
// name is spelled.
//
// It is the API's tarball endpoint rather than the content host directly,
// because that is the documented route that takes an authorization header — the
// one a private repository needs, and the reason the fetcher follows a bounded
// redirect chain: the endpoint answers with a signed URL on the content host,
// whose query string carries the grant that [source.RedactURL] exists to drop.
const archiveHost = "api.github.com"

// archiveURL is the archive of one repository at one commit.
//
// The commit is used rather than the ref the manifest declared, so the URL names
// an object that cannot change under it — which is what makes the response
// content-addressable and the same archive reusable for every asset that
// resolved to it.
func archiveURL(repository, commit string) string {
	return "https://" + archiveHost + "/repos/" + repository + "/tarball/" + commit
}

// archiveFetcher retrieves the whole body at a URL, presenting a credential
// where the run has one.
//
// It is a parameter rather than a call for the reason [gitRunner] is: a
// resolution has to be exercisable without a network, and the fetcher's own
// rules — https only, no destination on this machine — mean a test server cannot
// stand in for the forge. The production value is [source.Fetcher.Fetch], which
// is the only way harnaas makes an HTTP request at all.
type archiveFetcher func(ctx context.Context, url string, credential source.Credential) ([]byte, error)

// archiveKey identifies one archive: a repository and the commit its content is
// of.
//
// The commit and not the ref, because two assets pinned to `v1.2.0` and to the
// commit that tag names are the same fetch, and the ref is what resolution has
// already spent. The credential is not part of the key because it is resolved
// once for the whole run: two retrievals within one run are made as the same
// identity by construction.
type archiveKey struct {
	repository string
	commit     string
}

// archives is one run's memory of the archives it has retrieved.
type archives struct {
	fetch archiveFetcher

	// mu guards fetched, and is held only while the slot for a key is found or
	// created — never across the retrieval itself, so one repository's fetch
	// does not queue behind another's.
	mu      sync.Mutex
	fetched map[archiveKey]*fetchedArchive
}

// newArchives returns an empty memo retrieving through fetch.
func newArchives(fetch archiveFetcher) *archives {
	return &archives{fetch: fetch, fetched: map[archiveKey]*fetchedArchive{}}
}

// fetchedArchive is one archive's slot in the memo: the retrieval, performed at
// most once, and whatever it produced.
type fetchedArchive struct {
	once sync.Once
	body []byte
	err  error
}

// get returns the archive for a repository at a commit, retrieving it the first
// time it is asked for and not again.
//
// A retrieval that failed is remembered as well as one that succeeded. The next
// asset referencing the same repository would meet the same host and the same
// answer, so asking again would multiply one outage by the number of assets
// declared against it — and the caller attributes the remembered failure to its
// own asset, so the second asset's diagnostic still names the second asset.
func (a *archives) get(ctx context.Context, repository, commit string, credential source.Credential) ([]byte, error) {
	key := archiveKey{repository: repository, commit: commit}

	a.mu.Lock()
	entry, known := a.fetched[key]
	if !known {
		entry = &fetchedArchive{}
		a.fetched[key] = entry
	}
	a.mu.Unlock()

	entry.once.Do(func() {
		entry.body, entry.err = a.fetch(ctx, archiveURL(repository, commit), credential)
	})

	return entry.body, entry.err
}
