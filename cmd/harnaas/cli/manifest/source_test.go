package manifest_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// TestParseSourceAcceptsTheDocumentedForms covers the two registered kinds in
// the spelling the manifest's own documentation uses.
func TestParseSourceAcceptsTheDocumentedForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value string
		want  manifest.Source
	}{
		{
			name:  "github source with an owner, a repository and a tag",
			key:   "acme",
			value: "github:acme/assets@v1.2.0",
			want: manifest.Source{
				Key:        "acme",
				Kind:       manifest.SourceKindGitHub,
				Repository: "acme/assets",
				Ref:        "v1.2.0",
			},
		},
		{
			name:  "github source pinned to a commit",
			key:   "acme",
			value: "github:acme/assets@0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c",
			want: manifest.Source{
				Key:        "acme",
				Kind:       manifest.SourceKindGitHub,
				Repository: "acme/assets",
				Ref:        "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c",
			},
		},
		{
			name:  "local source naming the .harnaas root itself",
			key:   "house",
			value: "local:.harnaas",
			want: manifest.Source{
				Key:        "house",
				Kind:       manifest.SourceKindLocal,
				Repository: ".harnaas",
			},
		},
		{
			name:  "local source naming a directory beneath .harnaas",
			key:   "house",
			value: "local:.harnaas/shared/",
			want: manifest.Source{
				Key:        "house",
				Kind:       manifest.SourceKindLocal,
				Repository: ".harnaas/shared",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			source, violation := manifest.ParseSource(test.key, test.value)
			require.Nil(t, violation)
			assert.Equal(t, test.want, source)
		})
	}
}

// TestSourceStringRoundTripsWhatWasWritten proves a parsed source can be quoted
// back at its author, which is what lets a downstream diagnostic name the
// declaration without the caller reassembling it.
func TestSourceStringRoundTripsWhatWasWritten(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"github:acme/assets@v1.2.0", "local:.harnaas"} {
		source, violation := manifest.ParseSource("acme", value)
		require.Nil(t, violation)
		assert.Equal(t, value, source.String())
	}
}

// TestParseSourceRejects covers every way a `sources` entry can be wrong. Each
// case asserts on the fragments the author needs to see rather than the whole
// sentence, so rewording a diagnostic does not break the test that guards it.
func TestParseSourceRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		key       string
		value     string
		wantField string
		wantParts []string
	}{
		{
			name:      "a value with no kind",
			key:       "acme",
			value:     "acme/assets@v1.2.0",
			wantField: "sources.acme",
			wantParts: []string{"acme", "does not name a source kind", "github:acme/assets@v1.2.0"},
		},
		{
			name:      "an unregistered kind names the kind and the key",
			key:       "acme",
			value:     "gitlab:acme/assets@v1.2.0",
			wantField: "sources.acme",
			wantParts: []string{`"acme"`, `"gitlab"`, "does not recognize", `"github"`, `"local"`},
		},
		{
			name:      "a github repository that is not owner and repository",
			key:       "acme",
			value:     "github:assets@v1.2.0",
			wantField: "sources.acme",
			wantParts: []string{`"assets"`, "owner and a repository"},
		},
		{
			name:      "a github source with no ref",
			key:       "acme",
			value:     "github:acme/assets",
			wantField: "sources.acme",
			wantParts: []string{"declares no ref", "no particular version"},
		},
		{
			name:      "a local source carrying a ref",
			key:       "house",
			value:     "local:.harnaas@v1",
			wantField: "sources.house",
			wantParts: []string{"local", `"v1"`, "no ref to be at"},
		},
		{
			name:      "a local source outside .harnaas",
			key:       "house",
			value:     "local:assets",
			wantField: "sources.house",
			wantParts: []string{`"assets"`, "not inside .harnaas"},
		},
		{
			name:      "a local source escaping .harnaas upward",
			key:       "house",
			value:     "local:.harnaas/../../elsewhere",
			wantField: "sources.house",
			wantParts: []string{"not inside .harnaas"},
		},
		{
			name:      "an absolute local source",
			key:       "house",
			value:     "local:/etc/harnaas",
			wantField: "sources.house",
			wantParts: []string{"not inside .harnaas"},
		},
		{
			name:      "an empty key",
			key:       "",
			value:     "github:acme/assets@v1.2.0",
			wantField: "sources",
			wantParts: []string{"empty key"},
		},
		{
			name:      "a key an asset entry could not reference",
			key:       "acme:assets",
			value:     "github:acme/assets@v1.2.0",
			wantField: "sources.acme:assets",
			wantParts: []string{"could not reference it by"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			source, violation := manifest.ParseSource(test.key, test.value)
			require.NotNil(t, violation)
			assert.Equal(t, manifest.Source{}, source, "a rejected source must not be half-parsed")
			assert.Equal(t, manifest.DocumentIndex, violation.Index)
			assert.Equal(t, test.wantField, violation.Field)

			for _, part := range test.wantParts {
				assert.Contains(t, violation.String(), part)
			}
		})
	}
}

// TestSourceRefIsTakenFromTheLastAtSign pins the split, because a ref is the
// one part of a source string a person edits regularly and getting the boundary
// wrong would silently pin the wrong thing.
func TestSourceRefIsTakenFromTheLastAtSign(t *testing.T) {
	t.Parallel()

	source, violation := manifest.ParseSource("acme", "github:acme/as@sets@v1.2.0")
	require.Nil(t, violation)
	assert.Equal(t, "acme/as@sets", source.Repository)
	assert.Equal(t, "v1.2.0", source.Ref)
}

// TestSourceViolationsAreProblemThenFix asserts the diagnostic shape every
// user-facing message in harnaas takes: where, what is wrong, a blank line, and
// the edit that resolves it — never a command that would make the edit.
func TestSourceViolationsAreProblemThenFix(t *testing.T) {
	t.Parallel()

	_, violation := manifest.ParseSource("acme", "gitlab:acme/assets@v1.2.0")
	require.NotNil(t, violation)

	parts := strings.Split(violation.String(), "\n\n")
	require.Len(t, parts, 2, "a violation is a problem, a blank line, and a fix")
	assert.True(t, strings.HasPrefix(parts[0], violation.Field+": "), "the problem opens with where it is")
	assert.NotEmpty(t, parts[1])
	assert.NotContains(t, parts[1], "harnaas fix", "no remedy offers a command that edits the manifest")
}
