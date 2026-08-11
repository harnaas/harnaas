package github

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

const (
	// commitID is the commit every archive in this file is of, and the one the
	// tag below resolves to.
	commitID = "9f2a1c4e8b7d6a5f4e3c2b1a0987654321fedcba"

	// otherCommitID is a second commit in the same repository, which is what
	// makes "at most once per repository *and commit*" testable as more than
	// "at most once per repository".
	otherCommitID = "3d5b7e9012a4c6e8f0b2d4a6c8e02468ace13579"

	// wrapper is the top-level directory a forge puts around a repository
	// archive. Its spelling is the forge's to choose, which is why extraction
	// takes it from the archive rather than reconstructing it.
	wrapper = "acme-assets-9f2a1c4/"
)

// tagListing is what `ls-remote` prints for a repository whose tag v1.2.0 is a
// lightweight tag on commitID.
var tagListing = commitID + "\trefs/tags/v1.2.0\n"

// resolveRequest builds a request for one asset at a path within the source.
func resolveRequest(id, ref, path string) source.Request {
	req := request(ref)
	req.Asset.ID = id
	req.Asset.Ref = manifest.AssetRef{SourceKey: "acme", Path: path}
	return req
}

// archiveOf builds the gzipped tar a forge serves, wrapping every named file in
// the archive's single top-level directory.
func archiveOf(tb testing.TB, files map[string]string) []byte {
	tb.Helper()

	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gz)

	for _, name := range slices.Sorted(maps.Keys(files)) {
		require.NoError(tb, writer.WriteHeader(&tar.Header{
			Name:     wrapper + name,
			Typeflag: tar.TypeReg,
			Size:     int64(len(files[name])),
			Mode:     0o644,
		}))
		_, err := writer.Write([]byte(files[name]))
		require.NoError(tb, err)
	}

	require.NoError(tb, writer.Close())
	require.NoError(tb, gz.Close())
	return buffer.Bytes()
}

// assetArchive is a plausible asset repository: two skills and a rule, so a
// declared subtree has both something to select and something to leave behind.
func assetArchive(tb testing.TB) []byte {
	tb.Helper()

	return archiveOf(tb, map[string]string{
		"README.md":                       "# assets",
		"skills/review/SKILL.md":          "review skill",
		"skills/review/reference/list.md": "checklist",
		"skills/release/SKILL.md":         "release skill",
		"rules/house-style.md":            "house style",
	})
}

// diagnostic is an error shaped the way the transport's own are: a problem, a
// blank line and a fix.
//
// It is a type rather than an errors.New value because revive refuses a message
// ending in punctuation — which is the same reason every diagnostic harnaas
// prints is a type with an Error method rather than a sentinel.
type diagnostic string

// Error renders the diagnostic as written.
func (d diagnostic) Error() string { return string(d) }

// countingFetcher answers every request with body and counts the URLs it was
// asked for, which is how "fetched at most once" is asserted as a number rather
// than inferred from a result that happens to be right.
type countingFetcher struct {
	mu          sync.Mutex
	urls        []string
	credentials []source.Credential
	body        []byte
	err         error
}

// fetch is the [archiveFetcher] the kind is built with.
func (f *countingFetcher) fetch(_ context.Context, url string, credential source.Credential) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.urls = append(f.urls, url)
	f.credentials = append(f.credentials, credential)
	if f.err != nil {
		return nil, f.err
	}
	return f.body, nil
}

// asked returns the URLs the kind fetched, in order.
func (f *countingFetcher) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.urls)
}

// presented returns the credentials the kind fetched with, in the same order.
func (f *countingFetcher) presented() []source.Credential {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.credentials)
}

func TestAnArchiveURLNamesTheCommitRatherThanTheRef(t *testing.T) {
	t.Parallel()

	got := archiveURL(repository, commitID)

	assert.Equal(t, "https://api.github.com/repos/acme/assets/tarball/"+commitID, got)
	assert.NotContains(t, got, "v1.2.0", "the ref has already been spent; the URL names an object that cannot move")
}

func TestResolveProducesTheSubtreeAndItsProvenance(t *testing.T) {
	t.Parallel()

	fetcher := &countingFetcher{body: assetArchive(t)}
	kind := newKind(answeringRunner(tagListing, nil), fetcher.fetch, source.Credential{})

	resolved, err := kind.Resolve(t.Context(), resolveRequest("review", "v1.2.0", "skills/review"))
	require.NoError(t, err)

	paths := make([]string, 0, len(resolved.Files))
	for _, file := range resolved.Files {
		paths = append(paths, file.Path)
	}
	assert.Equal(t, []string{"SKILL.md", "reference/list.md"}, paths)

	assert.Equal(t, source.Provenance{
		Kind:           manifest.SourceKindGitHub,
		Source:         "github:acme/assets@v1.2.0",
		RequestedRef:   "v1.2.0",
		ResolvedCommit: commitID,
	}, resolved.Provenance, "the ref asked for is recorded beside the commit it resolved to, never collapsed into it")
	assert.NotEmpty(t, resolved.Digest)
}

func TestABranchResolvesToAMutableSource(t *testing.T) {
	t.Parallel()

	fetcher := &countingFetcher{body: assetArchive(t)}
	kind := newKind(answeringRunner(commitID+"\trefs/heads/main\n", nil), fetcher.fetch, source.Credential{})

	resolved, err := kind.Resolve(t.Context(), resolveRequest("review", "main", "skills/review"))
	require.NoError(t, err)

	assert.True(t, resolved.Provenance.Mutable, "a branch can name something else tomorrow, and the lockfile has to say so")
}

func TestOneRepositoryAndCommitIsFetchedOncePerRun(t *testing.T) {
	t.Parallel()

	fetcher := &countingFetcher{body: assetArchive(t)}
	kind := newKind(answeringRunner(tagListing, nil), fetcher.fetch, source.Credential{})

	for _, asset := range []struct{ id, path string }{
		{id: "review", path: "skills/review"},
		{id: "release", path: "skills/release"},
		{id: "house-style", path: "rules/house-style.md"},
	} {
		resolved, err := kind.Resolve(t.Context(), resolveRequest(asset.id, "v1.2.0", asset.path))
		require.NoError(t, err)
		assert.NotEmpty(t, resolved.Files, "every asset is extracted from the one archive")
	}

	assert.Len(t, fetcher.asked(), 1, "three assets in one repository at one commit share a single retrieval")
}

func TestASecondCommitIsItsOwnFetch(t *testing.T) {
	t.Parallel()

	archive := assetArchive(t)
	fetcher := &countingFetcher{body: archive}
	kind := newKind(func(_ context.Context, args ...string) ([]byte, error) {
		// The commit a ref resolves to is what keys the memo, so the two refs
		// here have to answer differently for the test to mean anything.
		if slices.Contains(args, "refs/tags/v1.3.0") {
			return []byte(otherCommitID + "\trefs/tags/v1.3.0\n"), nil
		}
		return []byte(tagListing), nil
	}, fetcher.fetch, source.Credential{})

	for _, ref := range []string{"v1.2.0", "v1.2.0", "v1.3.0"} {
		_, err := kind.Resolve(t.Context(), resolveRequest("review", ref, "skills/review"))
		require.NoError(t, err)
	}

	assert.Equal(t, []string{
		archiveURL(repository, commitID),
		archiveURL(repository, otherCommitID),
	}, fetcher.asked())
}

func TestConcurrentResolutionsStillFetchOnce(t *testing.T) {
	t.Parallel()

	fetcher := &countingFetcher{body: assetArchive(t)}
	kind := newKind(answeringRunner(tagListing, nil), fetcher.fetch, source.Credential{})

	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()

			_, err := kind.Resolve(t.Context(), resolveRequest("review", "v1.2.0", "skills/review"))
			assert.NoError(t, err)
		}()
	}
	group.Wait()

	assert.Len(t, fetcher.asked(), 1)
}

func TestARetrievalFailureNamesTheAssetTheRepositoryAndTheCommit(t *testing.T) {
	t.Parallel()

	fetcher := &countingFetcher{err: diagnostic("could not fetch https://api.github.com/x: no such host\n\nCheck that this machine can reach the host.")}
	kind := newKind(answeringRunner(tagListing, nil), fetcher.fetch, source.Credential{})

	resolved, err := kind.Resolve(t.Context(), resolveRequest("review", "v1.2.0", "skills/review"))
	require.Nil(t, resolved, "a failed retrieval never resolves to content, empty or otherwise")

	var retrieval *ArchiveRetrievalError
	require.ErrorAs(t, err, &retrieval)
	assert.Equal(t, "review", retrieval.AssetID)
	assert.Equal(t, repository, retrieval.Repository)
	assert.Equal(t, commitID, retrieval.Commit)
}

func TestARememberedFailureNamesTheAssetThatMetIt(t *testing.T) {
	t.Parallel()

	fetcher := &countingFetcher{err: errors.New("the host refused")}
	kind := newKind(answeringRunner(tagListing, nil), fetcher.fetch, source.Credential{})

	first, firstErr := kind.Resolve(t.Context(), resolveRequest("review", "v1.2.0", "skills/review"))
	second, secondErr := kind.Resolve(t.Context(), resolveRequest("release", "v1.2.0", "skills/release"))

	require.Nil(t, first)
	require.Nil(t, second)
	assert.Len(t, fetcher.asked(), 1, "a repository that could not be fetched is not asked again for the next asset")
	assert.Contains(t, firstErr.Error(), `"review"`)
	assert.Contains(t, secondErr.Error(), `"release"`, "the remembered failure is attributed to the asset reading it")
}

func TestNoFailurePathResolvesToContent(t *testing.T) {
	t.Parallel()

	archive := assetArchive(t)

	cases := []struct {
		name    string
		listing string
		fetcher *countingFetcher
		path    string
	}{
		{
			name:    "unknown ref",
			fetcher: &countingFetcher{body: archive},
			path:    "skills/review",
		},
		{
			name:    "retrieval refused",
			listing: tagListing,
			fetcher: &countingFetcher{err: errors.New("the host refused")},
			path:    "skills/review",
		},
		{
			name:    "archive is not an archive",
			listing: tagListing,
			fetcher: &countingFetcher{body: []byte("<!doctype html>")},
			path:    "skills/review",
		},
		{
			name:    "path is not at the commit",
			listing: tagListing,
			fetcher: &countingFetcher{body: archive},
			path:    "skills/missing",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			kind := newKind(answeringRunner(tc.listing, nil), tc.fetcher.fetch, source.Credential{})

			resolved, err := kind.Resolve(t.Context(), resolveRequest("review", "v1.2.0", tc.path))
			require.Error(t, err)
			assert.Nil(t, resolved, "no failure hands on a source, so none can converge to deleting what the asset installed")
			assert.Contains(t, err.Error(), "review", "every failure names the asset the author would go and edit")
		})
	}
}

func TestARetrievalFailureQuotesTheCausesProblemAndNotItsFix(t *testing.T) {
	t.Parallel()

	cause := diagnostic("example.invalid answered 404 Not Found\n\nCheck that the source the manifest declares exists.")
	err := &ArchiveRetrievalError{AssetID: "review", Repository: repository, Commit: commitID, Err: cause}

	message := err.Error()
	assert.Contains(t, message, "answered 404 Not Found")
	assert.NotContains(t, message, "Check that the source the manifest declares exists.",
		"one message offers one fix, or the reader has to decide which of two is theirs")
	assert.Equal(t, 1, strings.Count(message, "\n\n"), "problem, blank line, fix")
	require.ErrorIs(t, err, cause)
}
