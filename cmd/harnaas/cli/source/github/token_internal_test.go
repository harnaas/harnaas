package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// environment answers a variable lookup from a map, which is what makes the
// chain's order testable without the process environment the suite runs under
// having a say in it.
func environment(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestTheFirstVariableSetWins(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		env    map[string]string
		origin string
		token  string
	}{
		{
			name:   "harnaas beats the ambient ones",
			env:    map[string]string{"HARNAAS_GITHUB_TOKEN": "harnaas", "GH_TOKEN": "gh", "GITHUB_TOKEN": "actions"},
			origin: "HARNAAS_GITHUB_TOKEN",
			token:  "harnaas",
		},
		{
			name:   "gh beats the actions one",
			env:    map[string]string{"GH_TOKEN": "gh", "GITHUB_TOKEN": "actions"},
			origin: "GH_TOKEN",
			token:  "gh",
		},
		{
			name:   "the last of the chain is still read",
			env:    map[string]string{"GITHUB_TOKEN": "actions"},
			origin: "GITHUB_TOKEN",
			token:  "actions",
		},
		{
			name: "none set is unauthenticated",
			env:  map[string]string{},
		},
		{
			// A job that exports a token conditionally leaves the variable
			// behind with nothing in it, and an empty bearer token fails a
			// request the same one without a header would have satisfied.
			name: "set to nothing is not set",
			env:  map[string]string{"HARNAAS_GITHUB_TOKEN": "", "GH_TOKEN": "  \n"},
		},
		{
			name:   "an empty one does not stop the chain",
			env:    map[string]string{"HARNAAS_GITHUB_TOKEN": "", "GITHUB_TOKEN": "actions"},
			origin: "GITHUB_TOKEN",
			token:  "actions",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := resolveCredential(environment(tc.env))

			assert.Equal(t, tc.token, got.Token)
			assert.Equal(t, tc.origin, got.Origin)
			assert.Equal(t, tc.token != "", got.Present())
		})
	}
}

func TestTheChainIsReadFromTheProcessEnvironment(t *testing.T) {
	// t.Setenv forbids t.Parallel, which is the whole cost of the one test that
	// proves the ambient reader is wired to the same chain the table exercises.
	t.Setenv("HARNAAS_GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "from-the-environment")

	got := ambientCredential()

	assert.Equal(t, source.Credential{Token: "from-the-environment", Origin: "GITHUB_TOKEN"}, got)
}

func TestTheTokenChainIsNamedInTheOrderItIsConsulted(t *testing.T) {
	t.Parallel()

	// The diagnostic raised when no token was set has to list every variable
	// that would have supplied one, so the sentence is built from the chain
	// itself rather than written out beside it.
	assert.Equal(t, []string{"HARNAAS_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"}, tokenEnvVars)
	assert.Equal(t, "HARNAAS_GITHUB_TOKEN, GH_TOKEN or GITHUB_TOKEN", describeTokenChain())
}

func TestARefusedRetrievalIsAnAuthorizationFailureAndNothingElseIs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
		denied bool
	}{
		{name: "unauthorized", status: 401, denied: true},
		{name: "forbidden", status: 403, denied: true},
		// Not found is an access decision here: ref resolution has already
		// proved over Git that the repository exists and holds this commit, so
		// the forge declining to admit it to this request is about the token.
		{name: "not found", status: 404, denied: true},
		{name: "gone", status: 410, denied: false},
		{name: "server error", status: 500, denied: false},
		{name: "rate limited", status: 429, denied: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.denied, deniesAccess(tc.status))

			fetcher := &countingFetcher{err: &source.StatusError{
				URL:        "https://api.github.com/repos/acme/assets/tarball/" + commitID,
				StatusCode: tc.status,
				Status:     "the forge said so",
			}}
			kind := newKind(answeringRunner(tagListing, nil), fetcher.fetch,
				source.Credential{Token: "s3cr3t-token", Origin: "GH_TOKEN"})

			resolved, err := kind.Resolve(t.Context(), resolveRequest("review", "v1.2.0", "skills/review"))
			require.Error(t, err)
			assert.Nil(t, resolved)

			var denied *AuthorizationError
			if !tc.denied {
				assert.NotErrorAs(t, err, &denied, "only a refusal is reported as one; anything else is a retrieval failure")
				var retrieval *ArchiveRetrievalError
				assert.ErrorAs(t, err, &retrieval)
				return
			}

			require.ErrorAs(t, err, &denied)
			assert.Equal(t, "review", denied.AssetID)
			assert.Equal(t, repository, denied.Repository)
			assert.Equal(t, commitID, denied.Commit)
			assert.Equal(t, "GH_TOKEN", denied.TokenOrigin)
			assert.NotContains(t, err.Error(), "s3cr3t-token",
				"the token is named by where it came from and never by what it is")
		})
	}
}

func TestTheRunsCredentialIsWhatEveryRetrievalPresents(t *testing.T) {
	t.Parallel()

	credential := source.Credential{Token: "s3cr3t-token", Origin: "GH_TOKEN"}
	fetcher := &countingFetcher{body: assetArchive(t)}
	kind := newKind(answeringRunner(tagListing, nil), fetcher.fetch, credential)

	_, err := kind.Resolve(t.Context(), resolveRequest("review", "v1.2.0", "skills/review"))
	require.NoError(t, err)

	assert.Equal(t, []source.Credential{credential}, fetcher.presented())
}
