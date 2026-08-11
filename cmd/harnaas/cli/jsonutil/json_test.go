package jsonutil_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/jsonutil"
)

func TestMarshalIndentsNestedStructure(t *testing.T) {
	t.Parallel()

	doc, err := jsonutil.Marshal(map[string]any{"assets": []string{"acme:skills/review.md"}})
	require.NoError(t, err)

	// Line by line rather than as one document string: the indentation is what
	// is under test, and a whole-document comparison hides which line moved.
	assert.Equal(t, []string{
		`{`,
		`  "assets": [`,
		`    "acme:skills/review.md"`,
		`  ]`,
		`}`,
		``,
	}, strings.Split(string(doc), "\n"))
}

// The escapes are written as interpreted strings so the assertion cannot be
// satisfied by its own input: a raw literal here would contain the bare
// character and the "does not contain" check would pass trivially.
func TestMarshalLeavesHTMLCharactersVerbatim(t *testing.T) {
	t.Parallel()

	doc, err := jsonutil.Marshal(map[string]string{"ref": "github:acme/assets?a=1&b=2<v>"})
	require.NoError(t, err)

	assert.Contains(t, string(doc), "?a=1&b=2<v>")
	assert.NotContains(t, string(doc), "\\u0026")
	assert.NotContains(t, string(doc), "\\u003c")
	assert.NotContains(t, string(doc), "\\u003e")
}

func TestMarshalEndsInExactlyOneNewline(t *testing.T) {
	t.Parallel()

	doc, err := jsonutil.Marshal(map[string]int{"version": 1})
	require.NoError(t, err)

	assert.True(t, strings.HasSuffix(string(doc), "}\n"), "document ends in a single newline: %q", string(doc))
}

func TestMarshalRoundTrips(t *testing.T) {
	t.Parallel()

	original := map[string]any{"version": float64(1), "harnesses": []any{"claude-code"}}

	doc, err := jsonutil.Marshal(original)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(doc, &decoded))
	assert.Equal(t, original, decoded)
}

func TestMarshalReportsAnUnencodableValue(t *testing.T) {
	t.Parallel()

	doc, err := jsonutil.Marshal(map[string]any{"fn": func() {}})

	require.Error(t, err)
	assert.Nil(t, doc, "no partial document is returned alongside the error")
}
