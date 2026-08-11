package source_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

func TestRedactURLRemovesEverySecretPart(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "nothing to remove",
			raw:  "https://codeload.github.com/acme/assets/tar.gz/abc123",
			want: "https://codeload.github.com/acme/assets/tar.gz/abc123",
		},
		{
			name: "userinfo",
			raw:  "https://harnaas:hunter2@codeload.github.com/acme/assets",
			want: "https://codeload.github.com/acme/assets",
		},
		{
			name: "bare username",
			raw:  "https://hunter2@codeload.github.com/acme/assets",
			want: "https://codeload.github.com/acme/assets",
		},
		{
			// This is the redaction that matters in practice: an archive
			// download redirects to a signed URL whose query grants the same
			// access the request had.
			name: "signed query",
			raw:  "https://cdn.example.com/a.tar.gz?token=abc&X-Amz-Signature=def",
			want: "https://cdn.example.com/a.tar.gz",
		},
		{
			name: "fragment",
			raw:  "https://cdn.example.com/a.tar.gz#anything",
			want: "https://cdn.example.com/a.tar.gz",
		},
		{
			name: "port survives",
			raw:  "https://forge.example.com:8443/acme/assets?token=abc",
			want: "https://forge.example.com:8443/acme/assets",
		},
		{
			name: "unparseable says so rather than echoing",
			raw:  "https://harnaas:hunter2@example.com/%zz",
			want: "<unparseable url>",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, source.RedactURL(tc.raw))
		})
	}
}

// TestEveryTransportFailureIsShapedProblemThenFix asserts the diagnostic
// contract over the whole transport failure surface at once: the shape, and that
// nothing secret survives into the message.
//
// The list is written out rather than derived, for the reason the command
// surface is: a test that asked the package which errors it declares would agree
// with any set. Adding a transport diagnostic is therefore two edits, and the
// second one is where somebody confirms it redacts what it prints.
func TestEveryTransportFailureIsShapedProblemThenFix(t *testing.T) {
	t.Parallel()

	// Every one is built with a URL carrying a credential, because the shape and
	// the redaction are the two things a transport diagnostic has to get right.
	const raw = "https://harnaas:hunter2@assets.example.com/skills.tar.gz?token=abc"

	failures := map[string]error{
		"insecure destination": &source.InsecureDestinationError{URL: raw, Reason: "its scheme is \"http\" rather than https"},
		"too many redirects":   &source.TooManyRedirectsError{URL: raw, Limit: 5},
		"response too large":   &source.ResponseTooLargeError{URL: raw, Limit: 1024},
		"status":               &source.StatusError{URL: raw, StatusCode: 404, Status: "404 Not Found"},
		"fetch":                &source.FetchError{URL: raw, Err: errors.New("connection refused")},
	}

	for name, err := range failures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			message := err.Error()

			problem, fix, split := strings.Cut(message, "\n\n")
			require.True(t, split, "the problem and the fix must be separated by a blank line: %s", message)
			assert.NotContains(t, problem, "\n", "the problem is one line")
			assert.NotEmpty(t, fix)

			assert.Contains(t, problem, "assets.example.com",
				"a diagnostic names the destination it is about")
			assert.NotContains(t, message, "hunter2", "no credential reaches a message")
			assert.NotContains(t, message, "token=", "no signed query reaches a message")
		})
	}
}
