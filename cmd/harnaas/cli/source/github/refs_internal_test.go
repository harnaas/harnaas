package github

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
	"github.com/harnaas/harnaas/internal/testenv"
)

// TestMain redirects the per-user directories, which for this package is more
// than the module's rule requiring it: git reads the user's global
// configuration out of the home directory, so a suite run against the real one
// would resolve refs under whatever that machine's owner happens to have
// configured and would pass or fail for reasons the test never stated.
func TestMain(m *testing.M) { testenv.Main(m) }

const (
	// assetID is the asset every request in this file resolves for. The
	// diagnostics name it, which is the point.
	assetID = "review"

	// repository is the `owner/repository` pair the manifest declared. Nothing
	// here reaches github.com — the remote is passed separately — but the
	// diagnostics quote it, so it has to read like one.
	repository = "acme/assets"
)

// request builds the request for a source declaring ref.
func request(ref string) source.Request {
	return source.Request{
		Asset:  manifest.Asset{ID: assetID},
		Source: manifest.Source{Key: "acme", Kind: manifest.SourceKindGitHub, Repository: repository, Ref: ref},
	}
}

// refusingRunner fails the test if it is called, which is how "no remote lookup
// is required" is asserted as an absence rather than inferred from a result that
// happens to be right.
func refusingRunner(tb testing.TB) gitRunner {
	tb.Helper()

	return func(_ context.Context, args ...string) ([]byte, error) {
		tb.Errorf("git was run when no lookup should have been needed: git %s", strings.Join(args, " "))
		return nil, errors.New("unreachable")
	}
}

// answeringRunner replies with fixed `ls-remote` output and records the
// arguments it was given.
func answeringRunner(out string, args *[]string) gitRunner {
	return func(_ context.Context, given ...string) ([]byte, error) {
		if args != nil {
			*args = given
		}
		return []byte(out), nil
	}
}

func TestObjectIDRecognizesOnlyAFullIdentifier(t *testing.T) {
	t.Parallel()

	const sha1 = "0b8e5a1d6a3f6c2e9d4b7a0c1f2e3d4c5b6a7980"

	cases := []struct {
		name      string
		candidate string
		want      string
	}{
		{name: "sha-1", candidate: sha1, want: sha1},
		{name: "sha-256", candidate: strings.Repeat("ab", 32), want: strings.Repeat("ab", 32)},
		{name: "uppercase is lowered", candidate: strings.ToUpper(sha1), want: sha1},
		{name: "abbreviation", candidate: sha1[:12]},
		{name: "too long", candidate: sha1 + "0"},
		{name: "tag name", candidate: "v1.2.0"},
		{name: "branch name", candidate: "main"},
		{name: "right length, not hex", candidate: strings.Repeat("g", 40)},
		{name: "empty", candidate: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := objectID(tc.candidate)
			assert.Equal(t, tc.want != "", ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestACommitIdentifierResolvesWithoutAskingTheRemote(t *testing.T) {
	t.Parallel()

	const commit = "0b8e5a1d6a3f6c2e9d4b7a0c1f2e3d4c5b6a7980"

	got, err := resolveRef(t.Context(), refusingRunner(t), "https://example.invalid/x.git", request(commit))
	require.NoError(t, err)

	assert.Equal(t, RefResolution{Commit: commit}, got)
	assert.False(t, got.Mutable, "a commit identifier cannot name something else tomorrow")
}

func TestARefLookupAsksOnlyForNamesUnderRefs(t *testing.T) {
	t.Parallel()

	var given []string
	_, err := resolveRef(t.Context(), answeringRunner("", &given), "remote", request("--upload-pack=touch /tmp/pwned"))
	require.Error(t, err)

	require.Equal(t, "ls-remote", given[0])
	for _, pattern := range given[2:] {
		assert.True(t, strings.HasPrefix(pattern, "refs/"),
			"every pattern is prefixed so git cannot read one as an option: %q", pattern)
	}
}

func TestAnAnnotatedTagResolvesToTheCommitItPeelsTo(t *testing.T) {
	t.Parallel()

	const (
		tagObject = "b64066577109e2006eeb5fac973132056e3ebdfe"
		commit    = "122b225e2dc03b97b9d08d8678a737fdb8f2b393"
	)

	// The tag's own object is listed first and the peeled commit second, which
	// is the order git prints them; taking the first line would record the tag
	// object as the commit and fetch an archive that does not exist.
	out := tagObject + "\trefs/tags/v1.2.0\n" + commit + "\trefs/tags/v1.2.0^{}\n"

	got, err := resolveRef(t.Context(), answeringRunner(out, nil), "remote", request("v1.2.0"))
	require.NoError(t, err)

	assert.Equal(t, RefResolution{Commit: commit}, got)
}

func TestATagBeatsABranchOfTheSameName(t *testing.T) {
	t.Parallel()

	const (
		branchTip = "1111111111111111111111111111111111111111"
		tagged    = "2222222222222222222222222222222222222222"
	)

	out := branchTip + "\trefs/heads/same\n" + tagged + "\trefs/tags/same\n"

	got, err := resolveRef(t.Context(), answeringRunner(out, nil), "remote", request("same"))
	require.NoError(t, err)

	assert.Equal(t, RefResolution{Commit: tagged}, got,
		"a bare name means the tag, which is git's own precedence")
}

func TestARemoteAnsweringSomethingOtherThanACommitIsRefused(t *testing.T) {
	t.Parallel()

	out := "$(rm -rf /)\trefs/heads/main\n"

	_, err := resolveRef(t.Context(), answeringRunner(out, nil), "remote", request("main"))

	var lookup *RefLookupError
	require.ErrorAs(t, err, &lookup)
	assert.Contains(t, err.Error(), "not a commit identifier")
}

func TestGitMissingFromTheMachineIsItsOwnFailure(t *testing.T) {
	t.Parallel()

	failing := func(_ context.Context, _ ...string) ([]byte, error) {
		return nil, &exec.Error{Name: "git", Err: exec.ErrNotFound}
	}

	_, err := resolveRef(t.Context(), failing, "remote", request("v1.2.0"))

	var unavailable *GitUnavailableError
	require.ErrorAs(t, err, &unavailable)
	require.ErrorIs(t, err, exec.ErrNotFound)

	var lookup *RefLookupError
	assert.NotErrorAs(t, err, &lookup,
		"a missing git is not a problem with the repository the manifest names")
}

func TestARefusedLookupNamesTheAssetAndKeepsItsCause(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("fatal: repository not found")
	failing := func(_ context.Context, _ ...string) ([]byte, error) { return nil, sentinel }

	_, err := resolveRef(t.Context(), failing, "https://github.com/acme/assets.git", request("v1.2.0"))

	var lookup *RefLookupError
	require.ErrorAs(t, err, &lookup)
	require.ErrorIs(t, err, sentinel, "a cancelled or refused run stays recognizable through the wrapper")

	message := err.Error()
	assert.Contains(t, message, assetID)
	assert.Contains(t, message, repository)
	assert.Contains(t, message, sentinel.Error())
}

func TestAnAbsentRefResolvesTheDefaultBranch(t *testing.T) {
	t.Parallel()

	const commit = "122b225e2dc03b97b9d08d8678a737fdb8f2b393"

	var given []string
	got, err := resolveRef(t.Context(), answeringRunner(commit+"\tHEAD\n", &given), "remote", request(""))
	require.NoError(t, err)

	assert.Equal(t, RefResolution{Commit: commit, Mutable: true}, got)
	assert.Equal(t, []string{"ls-remote", "remote", "HEAD"}, given)
}

func TestAnAbsentRefAgainstARepositoryWithNoHeadIsUnknown(t *testing.T) {
	t.Parallel()

	_, err := resolveRef(t.Context(), answeringRunner("", nil), "remote", request(""))

	var unknown *UnknownRefError
	require.ErrorAs(t, err, &unknown)
	assert.Equal(t, "HEAD", unknown.Ref)
}

func TestTheRemoteIsBuiltFromTheRepositoryAlone(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "https://github.com/acme/assets.git", remoteURL(repository))
}

// The tests below run the real git against a repository on this machine, which
// is the only way the parts a stub cannot reach are exercised at all: the
// argument order `ls-remote` actually accepts, that a ref the remote does not
// have comes back as a successful run printing nothing rather than as a
// failure, and that an annotated tag really is listed both peeled and unpeeled.
// Every one of those is a property of git rather than of this package, and a
// stub asserting them would only be asserting what the author believed.

// gitRepository builds a repository with the ref shapes resolution has to tell
// apart: an annotated tag, a lightweight tag, and a branch ahead of the default
// one so a branch tip is distinguishable from HEAD.
func gitRepository(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on this machine's PATH")
	}

	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()

		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	}

	run("init", "--quiet", "--initial-branch=main")

	// The suite has its own home directory, so there is no global identity to
	// inherit and every repository states one.
	run("config", "user.name", "harnaas test")
	run("config", "user.email", "test@harnaas.invalid")

	run("commit", "--quiet", "--allow-empty", "--message", "first")
	run("tag", "--annotate", "v1.2.0", "--message", "release")
	run("tag", "lightweight")
	run("checkout", "--quiet", "-b", "feature")
	run("commit", "--quiet", "--allow-empty", "--message", "second")
	run("checkout", "--quiet", "main")

	return dir
}

// commitOf is what a ref resolves to according to git itself, which is the only
// expectation worth comparing against.
func commitOf(t *testing.T, dir, ref string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", "rev-parse", ref+"^{commit}")
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)

	return strings.TrimSpace(string(out))
}

func TestRealGitResolvesEveryRefShape(t *testing.T) {
	t.Parallel()

	dir := gitRepository(t)

	cases := []struct {
		name    string
		ref     string
		want    string
		mutable bool
	}{
		{name: "annotated tag", ref: "v1.2.0", want: "v1.2.0"},
		{name: "lightweight tag", ref: "lightweight", want: "lightweight"},
		{name: "branch", ref: "feature", want: "feature", mutable: true},
		{name: "default branch", ref: "", want: "HEAD", mutable: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveRef(t.Context(), runGit, dir, request(tc.ref))
			require.NoError(t, err)

			assert.Equal(t, commitOf(t, dir, tc.want), got.Commit)
			assert.Equal(t, tc.mutable, got.Mutable, "only a ref that can move is reported mutable")
		})
	}
}

func TestRealGitReportsARefTheRepositoryDoesNotHave(t *testing.T) {
	t.Parallel()

	dir := gitRepository(t)

	// git exits successfully having printed nothing, so an unknown ref is only
	// ever a property of the output — a run that checked the status would
	// report every unknown ref as a resolution to nothing.
	_, err := resolveRef(t.Context(), runGit, dir, request("v9.9.9"))

	var unknown *UnknownRefError
	require.ErrorAs(t, err, &unknown)

	assert.Equal(t, assetID, unknown.AssetID)
	assert.Equal(t, repository, unknown.Repository)
	assert.Equal(t, "v9.9.9", unknown.Ref)
}

func TestRealGitFailingCarriesItsOwnExplanation(t *testing.T) {
	t.Parallel()

	dir := gitRepository(t)

	_, err := resolveRef(t.Context(), runGit, filepath.Join(dir, "absent"), request("v1.2.0"))

	var lookup *RefLookupError
	require.ErrorAs(t, err, &lookup)

	assert.Contains(t, err.Error(), "does not appear to be a git repository",
		"git's own stderr is what tells a private repository from a missing one")
}
