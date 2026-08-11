package source_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// provenance is a filled-in provenance for tests whose subject is the content
// rather than where it came from.
func provenance() source.Provenance {
	return source.Provenance{
		Kind:           manifest.SourceKindGitHub,
		Source:         "github:acme/assets@v1.2.0",
		RequestedRef:   "v1.2.0",
		ResolvedCommit: "9f2c1b4d5e6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c",
	}
}

func TestNewResolvedSortsFilesByPath(t *testing.T) {
	t.Parallel()

	resolved, err := source.NewResolved(provenance(), map[string][]byte{
		"reference/two.md": []byte("two"),
		"SKILL.md":         []byte("skill"),
		"reference/one.md": []byte("one"),
	})
	require.NoError(t, err)

	paths := make([]string, 0, len(resolved.Files))
	for _, file := range resolved.Files {
		paths = append(paths, file.Path)
	}
	assert.Equal(t, []string{"SKILL.md", "reference/one.md", "reference/two.md"}, paths)
}

func TestNewResolvedDigestsEveryFile(t *testing.T) {
	t.Parallel()

	resolved, err := source.NewResolved(provenance(), map[string][]byte{
		"SKILL.md": []byte("skill"),
		"extra.md": []byte("extra"),
	})
	require.NoError(t, err)

	for _, file := range resolved.Files {
		assert.Equal(t, source.DigestContent(file.Content), file.Digest,
			"file %q carries the digest of its own content", file.Path)
	}
	assert.NotEqual(t, resolved.Files[0].Digest, resolved.Files[1].Digest)
}

func TestNewResolvedKeepsProvenanceAndContentVerbatim(t *testing.T) {
	t.Parallel()

	content := []byte("---\nname: review\n---\n\nBody.\n")
	resolved, err := source.NewResolved(provenance(), map[string][]byte{"SKILL.md": content})
	require.NoError(t, err)

	assert.Equal(t, provenance(), resolved.Provenance)
	require.Len(t, resolved.Files, 1)
	assert.Equal(t, content, resolved.Files[0].Content)
}

// TestNewResolvedRefusesEmptyContent pins the rule that keeps a half-finished
// fetch from converging to a deletion: a source that resolved to nothing is not
// a source that resolved.
func TestNewResolvedRefusesEmptyContent(t *testing.T) {
	t.Parallel()

	for name, content := range map[string]map[string][]byte{
		"nil map":   nil,
		"empty map": {},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			resolved, err := source.NewResolved(provenance(), content)
			require.Error(t, err)
			assert.Nil(t, resolved)
		})
	}
}

func TestNewResolvedRefusesAFileWithNoPath(t *testing.T) {
	t.Parallel()

	resolved, err := source.NewResolved(provenance(), map[string][]byte{"": []byte("content")})
	require.Error(t, err)
	assert.Nil(t, resolved)
}

// TestWholeSourceDigestIsIndependentOfOrdering is the platform-independence
// requirement in the form a single-process test can state it: the digest is a
// function of the sorted paths and their content, and map iteration order —
// which Go randomizes on every range — cannot reach it.
func TestWholeSourceDigestIsIndependentOfOrdering(t *testing.T) {
	t.Parallel()

	content := map[string][]byte{
		"SKILL.md":         []byte("skill"),
		"reference/one.md": []byte("one"),
		"reference/two.md": []byte("two"),
		"scripts/run.sh":   []byte("#!/bin/sh\n"),
	}

	first, err := source.NewResolved(provenance(), content)
	require.NoError(t, err)

	for range 20 {
		again, err := source.NewResolved(provenance(), content)
		require.NoError(t, err)
		require.Equal(t, first.Digest, again.Digest)
	}
}

// TestWholeSourceDigestIgnoresProvenance keeps the two questions apart: the
// digest answers "is this the same content", and the commit it arrived from is
// recorded beside it rather than folded into it.
func TestWholeSourceDigestIgnoresProvenance(t *testing.T) {
	t.Parallel()

	content := map[string][]byte{"SKILL.md": []byte("skill")}

	first, err := source.NewResolved(provenance(), content)
	require.NoError(t, err)

	other := provenance()
	other.ResolvedCommit = "0000000000000000000000000000000000000000"
	second, err := source.NewResolved(other, content)
	require.NoError(t, err)

	assert.Equal(t, first.Digest, second.Digest)
}

func TestWholeSourceDigestChangesWithContent(t *testing.T) {
	t.Parallel()

	before, err := source.NewResolved(provenance(), map[string][]byte{
		"SKILL.md":         []byte("skill"),
		"reference/one.md": []byte("one"),
	})
	require.NoError(t, err)

	after, err := source.NewResolved(provenance(), map[string][]byte{
		"SKILL.md":         []byte("skill"),
		"reference/one.md": []byte("one edited"),
	})
	require.NoError(t, err)

	assert.NotEqual(t, before.Digest, after.Digest)
}

// TestWholeSourceDigestChangesWithARename is why paths participate at all: the
// bytes are identical and the source is not.
func TestWholeSourceDigestChangesWithARename(t *testing.T) {
	t.Parallel()

	before, err := source.NewResolved(provenance(), map[string][]byte{
		"SKILL.md":         []byte("skill"),
		"reference/one.md": []byte("one"),
	})
	require.NoError(t, err)

	after, err := source.NewResolved(provenance(), map[string][]byte{
		"SKILL.md":         []byte("skill"),
		"reference/uno.md": []byte("one"),
	})
	require.NoError(t, err)

	assert.NotEqual(t, before.Digest, after.Digest)
}

// TestWholeSourceDigestFramesPaths guards the one failure a content digest may
// not have: two different sets of files whose serializations collide. An archive
// entry may be named anything, so a source of two empty files can be forged by a
// source of one whose path spells out the first file's line — which is exactly
// what the length prefix in front of each path prevents.
func TestWholeSourceDigestFramesPaths(t *testing.T) {
	t.Parallel()

	empty := source.DigestContent(nil)

	first, err := source.NewResolved(provenance(), map[string][]byte{
		"a": nil,
		"b": nil,
	})
	require.NoError(t, err)

	second, err := source.NewResolved(provenance(), map[string][]byte{
		"a " + string(empty) + "\nb": nil,
	})
	require.NoError(t, err)

	assert.NotEqual(t, first.Digest, second.Digest)
}

func TestDigestContentNamesItsAlgorithm(t *testing.T) {
	t.Parallel()

	// The empty string's SHA-256, which is what pins the algorithm rather than
	// only the shape of the value.
	assert.Equal(t,
		source.Digest("sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"),
		source.DigestContent(nil),
	)
	assert.Equal(t, source.DigestContent(nil), source.DigestContent([]byte{}))
	assert.NotEqual(t, source.DigestContent(nil), source.DigestContent([]byte("x")))
}

// TestAResolvedFileCarriesNoFileMode keeps the mode normalization structural.
// The digest cannot cover a mode that the type does not hold, so a later change
// that wants to preserve an executable bit has to edit this rule rather than
// quietly reintroduce a value that hashes differently on Windows and Linux.
func TestAResolvedFileCarriesNoFileMode(t *testing.T) {
	t.Parallel()

	fields := reflect.VisibleFields(reflect.TypeFor[source.File]())
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.Name)
	}

	assert.Equal(t, []string{"Path", "Content", "Digest"}, names)
}
