package source_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// secret is the token every case here holds, chosen so that any rendering that
// leaked it would be unmistakable in a failure message.
const secret = "ghp-s3cr3t-token-value"

func TestACredentialIsPresentOnlyWhenItHoldsSomething(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		token string
		want  bool
	}{
		{name: "empty", token: "", want: false},
		// A variable exported with nothing in it is how a CI job leaves a token
		// it decided not to set, and an empty bearer header fails a request that
		// would have succeeded unauthenticated.
		{name: "blank", token: "   \n", want: false},
		{name: "token", token: secret, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, source.Credential{Token: tc.token}.Present())
		})
	}
}

func TestNoRenderingOfACredentialShowsTheToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		credential source.Credential
		want       string
	}{
		{name: "none", credential: source.Credential{}, want: "no credential"},
		{
			name:       "from a variable",
			credential: source.Credential{Token: secret, Origin: "HARNAAS_GITHUB_TOKEN"},
			want:       "the credential from HARNAAS_GITHUB_TOKEN",
		},
		{
			name:       "from nowhere named",
			credential: source.Credential{Token: secret},
			want:       "a credential",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Every verb fmt would reach for, including the two that ignore
			// String: %#v goes to GoString, and %+v does not.
			rendered := fmt.Sprintf("%v|%s|%+v|%#v|%q", tc.credential, tc.credential, tc.credential, tc.credential, tc.credential)

			assert.Contains(t, rendered, tc.want)
			assert.NotContains(t, rendered, secret, "the one value that must never be printed is the one the type holds")
		})
	}
}
