package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrintJSONWritesOneDocumentToStdoutAlone is the property that makes
// `harnaas … --json | jq` safe: the document is the only thing on stdout, and
// the advisory text a command emits alongside it lands on stderr.
func TestPrintJSONWritesOneDocumentToStdoutAlone(t *testing.T) {
	t.Parallel()

	cmd := newLeafCmd("renders")
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	advisef(cmd, "Resolving %d assets…\n", 2)
	require.NoError(t, printJSON(cmd, map[string]any{"installed": 2}))
	advisef(cmd, "Done.\n")

	var doc map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc),
		"stdout must parse as a single JSON document, got %q", out.String())
	assert.Equal(t, map[string]any{"installed": float64(2)}, doc)
	assert.Equal(t, "Resolving 2 assets…\nDone.\n", errOut.String())
}

// TestPrintJSONLeavesHTMLUnescaped guards the encoder setting: Go escapes <, >
// and & by default, which would rewrite an asset id, a path or a source ref
// into something no consumer asked for.
func TestPrintJSONLeavesHTMLUnescaped(t *testing.T) {
	t.Parallel()

	cmd := newLeafCmd("renders")
	var out bytes.Buffer
	cmd.SetOut(&out)

	require.NoError(t, printJSON(cmd, map[string]string{"source": "github:acme/assets?ref=a&tag=b"}))

	// htmlEscapedAmpersand is what Go's default encoder would have written.
	const htmlEscapedAmpersand = "\\u0026"

	assert.Contains(t, out.String(), "a&tag=b")
	assert.NotContains(t, out.String(), htmlEscapedAmpersand,
		"the ampersand must survive verbatim, not as Go's default HTML escape")
}

// TestPrintJSONEndsWithExactlyOneNewline keeps the document a clean unit for a
// line-oriented reader.
func TestPrintJSONEndsWithExactlyOneNewline(t *testing.T) {
	t.Parallel()

	cmd := newLeafCmd("renders")
	var out bytes.Buffer
	cmd.SetOut(&out)

	require.NoError(t, printJSON(cmd, []string{"skills/review"}))

	assert.Equal(t, "[\n  \"skills/review\"\n]\n", out.String())
}

// TestPrintJSONReportsAnEncodingFailure proves an unencodable value surfaces as
// an error the entrypoint can render, rather than as silently truncated stdout.
func TestPrintJSONReportsAnEncodingFailure(t *testing.T) {
	t.Parallel()

	cmd := newLeafCmd("renders")
	cmd.SetOut(&bytes.Buffer{})

	err := printJSON(cmd, func() {})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "encode json output")
}
