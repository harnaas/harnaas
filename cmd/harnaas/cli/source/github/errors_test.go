package github_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source/github"
)

// TestEveryRefFailureIsShapedProblemThenFix asserts the diagnostic contract over
// the whole ref-resolution failure surface at once.
//
// The list is written out rather than derived, for the reason the command
// surface is: a test that asked the package which errors it declares would agree
// with any set. Adding one is therefore two edits, and the second one is where
// somebody confirms the new message names the asset and leaks nothing.
func TestEveryRefFailureIsShapedProblemThenFix(t *testing.T) {
	t.Parallel()

	// The remote is built with a credential in it, which harnaas never does
	// itself — the point is that a diagnostic printing one would be the type's
	// mistake and not the caller's.
	const remote = "https://harnaas:hunter2@github.com/acme/assets.git?token=abc"

	failures := map[string]error{
		"unknown ref": &github.UnknownRefError{AssetID: "review", Repository: "acme/assets", Ref: "v9.9.9"},
		"lookup refused": &github.RefLookupError{
			AssetID:    "review",
			Repository: "acme/assets",
			Ref:        "v1.2.0",
			Remote:     remote,
			Err:        errors.New("fatal: repository not found"),
		},
		"git unavailable": &github.GitUnavailableError{Err: errors.New(`exec: "git": executable file not found in $PATH`)},
		// The cause is a real transport diagnostic rather than a plain error,
		// because the credential is redacted by the type that holds the URL —
		// so what this case proves is that quoting a cause's problem carries
		// that redaction with it rather than reaching around it.
		"archive retrieval": &github.ArchiveRetrievalError{
			AssetID:    "review",
			Repository: "acme/assets",
			Commit:     "9f2a1c4e8b7d6a5f4e3c2b1a0987654321fedcba",
			Err:        &source.FetchError{URL: remote, Err: errors.New("no such host")},
		},
		"access denied": &github.AuthorizationError{
			AssetID:     "review",
			Repository:  "acme/assets",
			Commit:      "9f2a1c4e8b7d6a5f4e3c2b1a0987654321fedcba",
			TokenOrigin: "HARNAAS_GITHUB_TOKEN",
			Err:         &source.StatusError{URL: remote, StatusCode: 403, Status: "403 Forbidden"},
		},
		"access denied with no token": &github.AuthorizationError{
			AssetID:    "review",
			Repository: "acme/assets",
			Commit:     "9f2a1c4e8b7d6a5f4e3c2b1a0987654321fedcba",
			Err:        &source.StatusError{URL: remote, StatusCode: 404, Status: "404 Not Found"},
		},
		"offline ref": &github.OfflineRefError{AssetID: "review", Repository: "acme/assets", Ref: "v1.2.0"},
		"offline archive": &github.OfflineArchiveError{
			AssetID:    "review",
			Repository: "acme/assets",
			Commit:     "9f2a1c4e8b7d6a5f4e3c2b1a0987654321fedcba",
		},
	}

	for name, err := range failures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			message := err.Error()

			problem, fix, split := strings.Cut(message, "\n\n")
			require.True(t, split, "the problem and the fix must be separated by a blank line: %s", message)
			assert.NotContains(t, problem, "\n", "the problem is one line")
			assert.NotEmpty(t, fix)

			assert.NotContains(t, message, "hunter2", "no credential reaches a message")
			assert.NotContains(t, message, "token=", "no signed query reaches a message")
		})
	}
}

// TestAManifestFailureNamesTheAssetAndTheRepository holds the two diagnostics
// an author can act on to naming both — a `sources` entry is referenced by any
// number of assets, so the repository alone does not say which line to edit.
func TestAManifestFailureNamesTheAssetAndTheRepository(t *testing.T) {
	t.Parallel()

	failures := map[string]error{
		"unknown ref": &github.UnknownRefError{AssetID: "review", Repository: "acme/assets", Ref: "v9.9.9"},
		"lookup refused": &github.RefLookupError{
			AssetID:    "review",
			Repository: "acme/assets",
			Ref:        "v1.2.0",
			Remote:     "https://github.com/acme/assets.git",
			Err:        errors.New("fatal: repository not found"),
		},
		"archive retrieval": &github.ArchiveRetrievalError{
			AssetID:    "review",
			Repository: "acme/assets",
			Commit:     "9f2a1c4e8b7d6a5f4e3c2b1a0987654321fedcba",
			Err:        errors.New("the host refused"),
		},
		"access denied": &github.AuthorizationError{
			AssetID:     "review",
			Repository:  "acme/assets",
			Commit:      "9f2a1c4e8b7d6a5f4e3c2b1a0987654321fedcba",
			TokenOrigin: "GH_TOKEN",
			Err:         errors.New("403 Forbidden"),
		},
		"offline ref": &github.OfflineRefError{AssetID: "review", Repository: "acme/assets", Ref: "v1.2.0"},
		"offline archive": &github.OfflineArchiveError{
			AssetID:    "review",
			Repository: "acme/assets",
			Commit:     "9f2a1c4e8b7d6a5f4e3c2b1a0987654321fedcba",
		},
	}

	for name, err := range failures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			message := err.Error()
			assert.Contains(t, message, "review")
			assert.Contains(t, message, "acme/assets")
		})
	}
}

// TestALookupWithNoRefNamesTheDefaultBranch pins the one wording that changes
// with the request rather than with the failure: a source declaring no ref was
// resolving the default branch, and a message quoting an empty ref names
// nothing.
func TestALookupWithNoRefNamesTheDefaultBranch(t *testing.T) {
	t.Parallel()

	err := &github.RefLookupError{
		AssetID:    "review",
		Repository: "acme/assets",
		Remote:     "https://github.com/acme/assets.git",
		Err:        errors.New("fatal: repository not found"),
	}

	assert.Contains(t, err.Error(), "the default branch of acme/assets")
}

// TestAnAuthorizationFailureNamesTheTokenItWasMadeWith holds the two halves of
// the rule apart: a run that had a token is told which variable it came from,
// and a run that had none is told every variable one could be supplied through.
// Naming the wrong one of those sends the reader to check a variable they never
// set, or to set one they already have.
func TestAnAuthorizationFailureNamesTheTokenItWasMadeWith(t *testing.T) {
	t.Parallel()

	withToken := &github.AuthorizationError{
		AssetID:     "review",
		Repository:  "acme/assets",
		Commit:      "9f2a1c4e8b7d6a5f4e3c2b1a0987654321fedcba",
		TokenOrigin: "GH_TOKEN",
		Err:         &source.StatusError{URL: "https://api.github.com/x", StatusCode: 403, Status: "403 Forbidden"},
	}
	assert.Contains(t, withToken.Error(), "with the token in GH_TOKEN")
	assert.NotContains(t, withToken.Error(), "HARNAAS_GITHUB_TOKEN",
		"the chain is what a run with no token is offered; a run that had one is told about the one it had")

	withNone := &github.AuthorizationError{
		AssetID:    "review",
		Repository: "acme/assets",
		Commit:     "9f2a1c4e8b7d6a5f4e3c2b1a0987654321fedcba",
		Err:        &source.StatusError{URL: "https://api.github.com/x", StatusCode: 404, Status: "404 Not Found"},
	}
	message := withNone.Error()
	assert.Contains(t, message, "without a token")
	for _, name := range []string{"HARNAAS_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} {
		assert.Contains(t, message, name, "every way a token could have been supplied is named")
	}
	// The status is still quoted: "not found" is what the forge said, and a
	// message that only talked about tokens would hide it.
	assert.Contains(t, message, "404 Not Found")
}

// TestAnAuthorizationFailureIsNotAManifestEdit holds the one retrieval failure
// whose fix is a token rather than a line in harnaas.json to saying so.
func TestAnAuthorizationFailureIsNotAManifestEdit(t *testing.T) {
	t.Parallel()

	err := &github.AuthorizationError{
		AssetID:     "review",
		Repository:  "acme/assets",
		Commit:      "9f2a1c4e8b7d6a5f4e3c2b1a0987654321fedcba",
		TokenOrigin: "HARNAAS_GITHUB_TOKEN",
		Err:         errors.New("403 Forbidden"),
	}

	assert.NotContains(t, err.Error(), "harnaas.json")
}

// TestAnOfflineArchiveFailureIsNotAManifestEdit holds the second diagnostic
// whose fix is not in the file: the source is declared correctly and the commit
// resolved, and what is missing is one run with the network. Pointing at
// harnaas.json would send the reader to change a line that is already right.
func TestAnOfflineArchiveFailureIsNotAManifestEdit(t *testing.T) {
	t.Parallel()

	err := &github.OfflineArchiveError{
		AssetID:    "review",
		Repository: "acme/assets",
		Commit:     "9f2a1c4e8b7d6a5f4e3c2b1a0987654321fedcba",
	}

	assert.NotContains(t, err.Error(), "harnaas.json")
	assert.Contains(t, err.Error(), "9f2a1c4e8b7d6a5f4e3c2b1a0987654321fedcba",
		"the cache is keyed by the commit, so the commit is what has not been fetched")
}

// TestGitUnavailableSendsNobodyToEditTheManifest holds the one diagnostic here
// that is not about the manifest to saying so: nothing an author writes in
// harnaas.json would install git, and a message naming the file would send them
// to edit one that is already correct.
func TestGitUnavailableSendsNobodyToEditTheManifest(t *testing.T) {
	t.Parallel()

	err := &github.GitUnavailableError{Err: errors.New("not found")}

	assert.NotContains(t, err.Error(), "harnaas.json")
	assert.Contains(t, err.Error(), "Install git")
}
