package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// requestFor builds a render request over one file's content.
func requestFor(t *testing.T, name adapter.Renderer, assetType manifest.AssetType, content string) renderRequest {
	t.Helper()
	return renderRequest{
		Asset:    manifest.Asset{ID: "ship", Type: assetType},
		Files:    []source.File{{Path: "ship.md", Content: []byte(content), Digest: source.DigestContent([]byte(content))}},
		Renderer: name,
		Target:   harness.ClaudeCode,
	}
}

func TestIdentityReproducesEveryByte(t *testing.T) {
	t.Parallel()

	// CRLF endings and trailing whitespace are exactly what a normalizing
	// renderer would quietly rewrite.
	const content = "---\r\nname: ship\r\npaths:\r\n  - \"src/**\"\r\n---\r\nBody.   \r\n\r\n"
	out, err := render(requestFor(t, adapter.RendererIdentity, manifest.AssetTypeRule, content))

	require.NoError(t, err)
	require.Len(t, out.Files, 1)
	assert.Equal(t, content, string(out.Files[0].Content),
		"a rewritten paths: list changes what the rule applies to, so nothing may be normalized")
	assert.False(t, out.Emulated)
}

func TestAnUndeclaredRendererCopies(t *testing.T) {
	t.Parallel()

	out, err := render(requestFor(t, "", manifest.AssetTypeRule, "body"))

	require.NoError(t, err)
	assert.Equal(t, "body", string(out.Files[0].Content),
		"a surface that declared no renderer gets copying, because copying is the default rather than the rule")
}

func TestAsSkillDisablesModelInvocation(t *testing.T) {
	t.Parallel()

	out, err := render(requestFor(t, adapter.RendererAsSkill, manifest.AssetTypeCommand,
		"---\nname: ship\ndescription: Ship it\n---\nRun the deploy.\n"))

	require.NoError(t, err)
	require.Len(t, out.Files, 1)
	assert.Equal(t, source.SkillFileName, out.Files[0].Path)
	assert.Contains(t, string(out.Files[0].Content), autoInvokeKey+": false",
		"a command delivered as a skill the harness may start on its own is a different asset")
}

func TestAsSkillLeavesEveryOtherFrontmatterKeyByteIdentical(t *testing.T) {
	t.Parallel()

	const content = "---\nname: ship\ndescription: \"Ship  it\"\npaths:\n  - 'src/**'\n---\nBody.\n"
	out, err := render(requestFor(t, adapter.RendererAsSkill, manifest.AssetTypeCommand, content))

	require.NoError(t, err)
	rendered := string(out.Files[0].Content)
	for _, line := range []string{"name: ship", `description: "Ship  it"`, "paths:", "  - 'src/**'"} {
		assert.Contains(t, rendered, line,
			"the block is edited textually, never re-encoded, so quoting and spacing survive")
	}
	assert.Contains(t, rendered, "Body.\n")
}

func TestAsSkillReplacesAnExistingInvocationKey(t *testing.T) {
	t.Parallel()

	out, err := render(requestFor(t, adapter.RendererAsSkill, manifest.AssetTypeCommand,
		"---\n"+autoInvokeKey+": true\nname: ship\n---\nBody.\n"))

	require.NoError(t, err)
	rendered := string(out.Files[0].Content)
	assert.Equal(t, 1, strings.Count(rendered, autoInvokeKey+":"))
	assert.Contains(t, rendered, autoInvokeKey+": false")
	assert.NotContains(t, rendered, autoInvokeKey+": true")
}

func TestAsSkillIgnoresAnIndentedKeyOfTheSameName(t *testing.T) {
	t.Parallel()

	// An indented key belongs to the mapping above it; replacing one would
	// move a nested setting to the top level.
	out, err := render(requestFor(t, adapter.RendererAsSkill, manifest.AssetTypeCommand,
		"---\nnested:\n  "+autoInvokeKey+": true\n---\nBody.\n"))

	require.NoError(t, err)
	rendered := string(out.Files[0].Content)
	assert.Contains(t, rendered, "  "+autoInvokeKey+": true", "the nested setting is untouched")
	assert.Contains(t, rendered, "\n"+autoInvokeKey+": false", "and a top-level one is added")
}

func TestAsSkillAddsFrontmatterWhenThereIsNone(t *testing.T) {
	t.Parallel()

	out, err := render(requestFor(t, adapter.RendererAsSkill, manifest.AssetTypeCommand, "Just a body.\n"))

	require.NoError(t, err)
	assert.Equal(t, "---\n"+autoInvokeKey+": false\n---\nJust a body.\n", string(out.Files[0].Content))
}

func TestAsSkillIsReportedAsAnEmulation(t *testing.T) {
	t.Parallel()

	out, err := render(requestFor(t, adapter.RendererAsSkill, manifest.AssetTypeCommand, "---\nname: ship\n---\nBody.\n"))

	require.NoError(t, err)
	assert.True(t, out.Emulated, "an emulation must never be reported as plain success")
	assert.Contains(t, out.Note, "will not invoke it on its own initiative",
		"the report has to say how the installed form differs from native support")
}

func TestAsInstructionIsReportedAsAnEmulation(t *testing.T) {
	t.Parallel()

	out, err := render(requestFor(t, adapter.RendererAsInstruction, manifest.AssetTypeRule, "Be brief.\n"))

	require.NoError(t, err)
	assert.True(t, out.Emulated)
	assert.Contains(t, out.Note, "always on")
	assert.Equal(t, "Be brief.\n", string(out.Files[0].Content), "the content itself is unchanged")
}

func TestADeclaredButUnimplementedRendererIsRefusedRatherThanApproximated(t *testing.T) {
	t.Parallel()

	out, err := render(requestFor(t, adapter.RendererMDC, manifest.AssetTypeRule, "Be brief.\n"))

	assert.Nil(t, out, "falling back to copying would write a file the harness cannot read")
	var unimplemented *unimplementedRendererError
	require.ErrorAs(t, err, &unimplemented)
	assert.Contains(t, err.Error(), string(adapter.RendererMDC), "the refusal names the renderer")
	assert.Contains(t, err.Error(), string(harness.ClaudeCode), "and the pairing it met")
}

func TestARendererTheContractDoesNotNameIsAWiringMistake(t *testing.T) {
	t.Parallel()

	_, err := render(requestFor(t, adapter.Renderer("invented"), manifest.AssetTypeRule, "x"))

	var unknown *unknownRendererError
	require.ErrorAs(t, err, &unknown)
}

func TestEveryImplementedRendererIsNamedByTheContract(t *testing.T) {
	t.Parallel()

	for name := range renderers {
		assert.Contains(t, adapter.Renderers(), name,
			"a renderer no adapter can declare is one nothing can select")
	}
}
