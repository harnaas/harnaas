package local_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/paths"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source/local"
)

// project builds a scratch repository holding the named files, each written
// relative to the project root, and returns a context carrying that root.
//
// The root is passed rather than discovered, so no test here depends on where
// the suite happens to be run from.
func project(tb testing.TB, files map[string]string) (context.Context, string) {
	tb.Helper()

	root := tb.TempDir()
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(tb, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(tb, os.WriteFile(full, []byte(content), 0o644))
	}

	return paths.WithProjectRoot(tb.Context(), root), root
}

// localRequest builds a request in the project-local form: the asset carries the
// whole path and references no `sources` entry.
func localRequest(id, path string) source.Request {
	return source.Request{
		Asset: manifest.Asset{ID: id, Ref: manifest.AssetRef{Path: path}},
	}
}

// resolve runs the kind the way the registry does, with the run's options
// settled once.
func resolve(ctx context.Context, opts source.RunOptions, req source.Request) (*source.Resolved, error) {
	return local.New(opts).Resolve(ctx, req)
}

// resolvedPaths returns every resolved path, which is what most assertions here
// are about.
func resolvedPaths(resolved *source.Resolved) []string {
	names := make([]string, 0, len(resolved.Files))
	for _, file := range resolved.Files {
		names = append(names, file.Path)
	}
	return names
}

// TestASingleFileResolvesUnderItsOwnLeafName holds the rule archive extraction
// already follows: a rule or a command is one file, and the path relative to a
// file is nothing.
func TestASingleFileResolvesUnderItsOwnLeafName(t *testing.T) {
	t.Parallel()

	ctx, _ := project(t, map[string]string{
		".harnaas/rules/house-style.md": "# House style\n",
	})

	resolved, err := resolve(ctx, source.RunOptions{}, localRequest("house-style", ".harnaas/rules/house-style.md"))
	require.NoError(t, err)

	require.Len(t, resolved.Files, 1)
	assert.Equal(t, "house-style.md", resolved.Files[0].Path)
	assert.Equal(t, "# House style\n", string(resolved.Files[0].Content))
	assert.NotEmpty(t, resolved.Digest)
}

// TestADirectoryResolvesEveryFileUnderItRelativeToItself covers the skill shape:
// a directory of documents, keyed by where each one sits inside it.
func TestADirectoryResolvesEveryFileUnderItRelativeToItself(t *testing.T) {
	t.Parallel()

	ctx, _ := project(t, map[string]string{
		".harnaas/skills/review/SKILL.md":             "---\nname: review\n---\n",
		".harnaas/skills/review/references/checks.md": "checks\n",
		".harnaas/skills/review/references/deep/x.md": "deep\n",
		".harnaas/skills/elsewhere/SKILL.md":          "not this one\n",
		".harnaas/rules/house-style.md":               "not this one either\n",
	})

	resolved, err := resolve(ctx, source.RunOptions{}, localRequest("review", ".harnaas/skills/review"))
	require.NoError(t, err)

	assert.Equal(t, []string{
		"SKILL.md",
		"references/checks.md",
		"references/deep/x.md",
	}, resolvedPaths(resolved))
}

// TestProvenanceRecordsALocalSourceAsPinningNothing holds the two halves of a
// local source's provenance apart: it is spelled the way the manifest declares
// it, and it never claims a ref, a commit or mutability it does not have.
func TestProvenanceRecordsALocalSourceAsPinningNothing(t *testing.T) {
	t.Parallel()

	ctx, _ := project(t, map[string]string{".harnaas/rules/tone.md": "tone\n"})

	resolved, err := resolve(ctx, source.RunOptions{}, localRequest("tone", ".harnaas/rules/tone.md"))
	require.NoError(t, err)

	assert.Equal(t, source.Provenance{
		Kind:   manifest.SourceKindLocal,
		Source: ".harnaas/rules/tone.md",
	}, resolved.Provenance)
}

// TestAKeyedLocalSourceIsReadRelativeToTheDirectoryItDeclares covers the second
// accepted spelling: a `sources` entry naming a directory under `.harnaas`, and
// an asset naming a path within it.
func TestAKeyedLocalSourceIsReadRelativeToTheDirectoryItDeclares(t *testing.T) {
	t.Parallel()

	ctx, _ := project(t, map[string]string{
		".harnaas/vendored/skills/review/SKILL.md": "---\nname: review\n---\n",
	})

	req := source.Request{
		Asset: manifest.Asset{
			ID:  "review",
			Ref: manifest.AssetRef{SourceKey: "vendored", Path: "skills/review"},
		},
		Source: manifest.Source{
			Key:        "vendored",
			Kind:       manifest.SourceKindLocal,
			Repository: ".harnaas/vendored",
		},
	}

	resolved, err := resolve(ctx, source.RunOptions{}, req)
	require.NoError(t, err)

	assert.Equal(t, []string{"SKILL.md"}, resolvedPaths(resolved))
	assert.Equal(t, "local:.harnaas/vendored", resolved.Provenance.Source,
		"the lockfile records the source the manifest declared, not the path it expanded to")
}

// TestAnOfflineRunResolvesALocalSourceIdentically is the whole of this kind's
// answer to offline mode: the content is already here, so a run with no network
// and no cache does exactly what a networked one does.
func TestAnOfflineRunResolvesALocalSourceIdentically(t *testing.T) {
	t.Parallel()

	ctx, _ := project(t, map[string]string{".harnaas/skills/review/SKILL.md": "---\nname: review\n---\n"})
	req := localRequest("review", ".harnaas/skills/review")

	online, err := resolve(ctx, source.RunOptions{}, req)
	require.NoError(t, err)

	offline, err := resolve(ctx, source.RunOptions{Offline: true}, req)
	require.NoError(t, err)

	assert.Equal(t, online, offline)
}

// TestAMissingSourceNamesTheAssetAndTheProjectRelativePath holds the diagnostic
// to the manifest's own spelling of the path, which is what the author has open.
func TestAMissingSourceNamesTheAssetAndTheProjectRelativePath(t *testing.T) {
	t.Parallel()

	ctx, root := project(t, map[string]string{".harnaas/rules/tone.md": "tone\n"})

	_, err := resolve(ctx, source.RunOptions{}, localRequest("house-style", ".harnaas/rules/house-style.md"))

	var missing *local.MissingSourceError
	require.ErrorAs(t, err, &missing)
	assert.Equal(t, "house-style", missing.AssetID)
	assert.Equal(t, ".harnaas/rules/house-style.md", missing.Path)
	assert.NotContains(t, err.Error(), root, "the path is named relative to the project root, not as a location on this machine")
}

// TestAProjectWithNoHarnaasDirectoryReportsTheAssetsOwnPath holds the one case
// where the failure is not where the read was: there is no `.harnaas` to anchor
// at, and naming that directory would name a path the author never wrote.
func TestAProjectWithNoHarnaasDirectoryReportsTheAssetsOwnPath(t *testing.T) {
	t.Parallel()

	ctx, _ := project(t, nil)

	_, err := resolve(ctx, source.RunOptions{}, localRequest("review", ".harnaas/skills/review"))

	var missing *local.MissingSourceError
	require.ErrorAs(t, err, &missing)
	assert.Equal(t, ".harnaas/skills/review", missing.Path)
}

// TestAnEmptyDirectoryIsRefusedRatherThanResolvedToNothing keeps the constructor's
// refusal from surfacing as an internal error: a source with no files would
// converge to deleting everything the asset had installed, and the author needs
// to be told which directory is empty.
func TestAnEmptyDirectoryIsRefusedRatherThanResolvedToNothing(t *testing.T) {
	t.Parallel()

	ctx, root := project(t, nil)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".harnaas", "skills", "review"), 0o755))

	_, err := resolve(ctx, source.RunOptions{}, localRequest("review", ".harnaas/skills/review"))

	var empty *local.EmptySourceError
	require.ErrorAs(t, err, &empty)
	assert.Equal(t, ".harnaas/skills/review", empty.Path)
}

// TestASymbolicLinkOutOfHarnaasIsRefused is the scenario the anchored handle
// exists for: the manifest's path stayed inside `.harnaas` when it was
// validated, and the filesystem says otherwise at the moment of the read.
func TestASymbolicLinkOutOfHarnaasIsRefused(t *testing.T) {
	t.Parallel()

	ctx, root := project(t, map[string]string{"secrets/private.md": "not harnaas's to read\n"})
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".harnaas", "rules"), 0o755))

	link := filepath.Join(root, ".harnaas", "rules", "house-style.md")
	if err := os.Symlink(filepath.Join(root, "secrets", "private.md"), link); err != nil {
		t.Skipf("this platform will not create a symbolic link for an unprivileged user: %v", err)
	}

	_, err := resolve(ctx, source.RunOptions{}, localRequest("house-style", ".harnaas/rules/house-style.md"))

	var containment *local.ContainmentError
	require.ErrorAs(t, err, &containment)
	assert.Equal(t, ".harnaas/rules/house-style.md", containment.Path)
	assert.NotContains(t, err.Error(), "not harnaas's to read", "the refused content never reaches the message")
}

// TestALinkInsideADirectoryIsRefusedAsAnEntryHarnaasCannotInstall holds the
// walk's own rule: what a link points at is not what harnaas installs, and
// following one inside the tree is what would let a link back up it spin the
// walk forever.
func TestALinkInsideADirectoryIsRefusedAsAnEntryHarnaasCannotInstall(t *testing.T) {
	t.Parallel()

	ctx, root := project(t, map[string]string{
		".harnaas/skills/review/SKILL.md": "---\nname: review\n---\n",
		".harnaas/rules/tone.md":          "tone\n",
	})

	link := filepath.Join(root, ".harnaas", "skills", "review", "tone.md")
	if err := os.Symlink(filepath.Join(root, ".harnaas", "rules", "tone.md"), link); err != nil {
		t.Skipf("this platform will not create a symbolic link for an unprivileged user: %v", err)
	}

	_, err := resolve(ctx, source.RunOptions{}, localRequest("review", ".harnaas/skills/review"))

	var unsupported *local.UnsupportedEntryError
	require.ErrorAs(t, err, &unsupported)
	assert.Equal(t, ".harnaas/skills/review/tone.md", unsupported.Path)
	assert.Equal(t, "a symbolic link", unsupported.EntryKind)
}

// TestResolutionWithNoProjectRootReportsTheWiringFailure keeps the root the
// context's business: this kind never asks the process where it is standing.
func TestResolutionWithNoProjectRootReportsTheWiringFailure(t *testing.T) {
	t.Parallel()

	_, err := resolve(t.Context(), source.RunOptions{}, localRequest("tone", ".harnaas/rules/tone.md"))

	require.ErrorIs(t, err, paths.ErrRootNotEstablished)
}

// TestContentIsReproducedByteForByte holds the rule the whole install flow rests
// on: resolution reads, and never rewrites — not a line ending, not a
// frontmatter field, not a trailing newline it thinks is missing.
func TestContentIsReproducedByteForByte(t *testing.T) {
	t.Parallel()

	const content = "---\r\nname: review\r\n---\r\n\r\nCRLF, no trailing newline, and a \x00 byte"

	ctx, _ := project(t, map[string]string{".harnaas/skills/review/SKILL.md": content})

	resolved, err := resolve(ctx, source.RunOptions{}, localRequest("review", ".harnaas/skills/review"))
	require.NoError(t, err)

	require.Len(t, resolved.Files, 1)
	assert.Equal(t, content, string(resolved.Files[0].Content))
}

// TestEveryLocalFailureIsShapedProblemThenFix asserts the diagnostic contract
// over the whole failure surface at once.
//
// The list is written out rather than derived, for the reason the command
// surface is: a test that asked the package which errors it declares would agree
// with any set. Adding one is two edits, and the second is where somebody
// confirms the new message names the asset and points at an edit.
func TestEveryLocalFailureIsShapedProblemThenFix(t *testing.T) {
	t.Parallel()

	const assetPath = ".harnaas/skills/review"

	failures := map[string]error{
		"missing":     &local.MissingSourceError{AssetID: "review", Path: assetPath},
		"empty":       &local.EmptySourceError{AssetID: "review", Path: assetPath},
		"containment": &local.ContainmentError{AssetID: "review", Path: assetPath, Err: errors.New("path escapes from parent")},
		"unsupported": &local.UnsupportedEntryError{AssetID: "review", Path: assetPath + "/link.md", EntryKind: "a symbolic link"},
		"unreadable":  &local.ReadError{AssetID: "review", Path: assetPath, Err: errors.New("permission denied")},
	}

	for name, err := range failures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			message := err.Error()

			problem, fix, split := strings.Cut(message, "\n\n")
			require.True(t, split, "the problem and the fix must be separated by a blank line: %s", message)
			assert.NotContains(t, problem, "\n", "the problem is one line")
			assert.NotEmpty(t, fix)

			assert.Contains(t, message, "review", "every diagnostic names the asset")
			assert.Contains(t, message, assetPath, "every diagnostic names the path the manifest wrote")
		})
	}
}

// TestTheKindIsUsableOnEveryPlatformTheSuiteRunsOn is a guard against a rule
// written for one filesystem: the walk builds every path with forward slashes,
// which is what a lockfile records, while every read goes through the handle in
// the platform's own spelling.
func TestTheKindIsUsableOnEveryPlatformTheSuiteRunsOn(t *testing.T) {
	t.Parallel()

	ctx, _ := project(t, map[string]string{".harnaas/skills/review/references/checks.md": "checks\n"})

	resolved, err := resolve(ctx, source.RunOptions{}, localRequest("review", ".harnaas/skills/review"))
	require.NoError(t, err)

	for _, file := range resolved.Files {
		assert.NotContains(t, file.Path, `\`,
			"a resolved path is slash-separated on %s as well as everywhere else", runtime.GOOS)
	}
}
