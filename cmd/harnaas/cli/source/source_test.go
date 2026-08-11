package source_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// TestRequestKindComesFromTheReferencedSource covers the keyed form, where the
// kind is a property of the `sources` entry rather than of the asset.
func TestRequestKindComesFromTheReferencedSource(t *testing.T) {
	t.Parallel()

	assert.Equal(t, manifest.SourceKindGitHub, githubRequest().Kind())
}

// TestRequestKindOfTheProjectLocalFormIsLocal pins the rule that would otherwise
// be restated by every caller: an asset naming a path under `.harnaas` declares
// no source, and is local by definition of the grammar.
func TestRequestKindOfTheProjectLocalFormIsLocal(t *testing.T) {
	t.Parallel()

	request := localRequest("house-style")

	assert.Equal(t, manifest.Source{}, request.Source, "the local form references no sources entry")
	assert.Equal(t, manifest.SourceKindLocal, request.Kind())
}

// TestRequestKindOfAParsedManifestMatchesItsSource walks the real path from a
// manifest to a request, so the two forms are exercised as the interpreter
// produces them rather than as a test hand-builds them.
func TestRequestKindOfAParsedManifestMatchesItsSource(t *testing.T) {
	t.Parallel()

	document, err := manifest.Decode([]byte(`{
  "version": 1,
  "harnesses": ["claude-code"],
  "sources": {"acme": "github:acme/assets@v1.2.0"},
  "assets": ["acme:skills/review", ".harnaas/skills/house-style"]
}`))
	if err != nil {
		t.Fatalf("decoding the manifest: %v", err)
	}

	interpretation, err := manifest.Interpret(document)
	if err != nil {
		t.Fatalf("interpreting the manifest: %v", err)
	}

	kinds := make([]manifest.SourceKind, 0, len(interpretation.Assets))
	for _, asset := range interpretation.Assets {
		kinds = append(kinds, source.Request{
			Asset:  asset,
			Source: interpretation.Sources[asset.Ref.SourceKey],
		}.Kind())
	}

	assert.Equal(t, []manifest.SourceKind{manifest.SourceKindGitHub, manifest.SourceKindLocal}, kinds)
}
