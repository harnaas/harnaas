package github_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
