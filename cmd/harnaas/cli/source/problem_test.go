package source_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// diagnostic is an error shaped the way harnaas's own are: a problem, a blank
// line and a fix.
//
// It is a type rather than an errors.New value because revive refuses a message
// ending in punctuation — which is the same reason every diagnostic harnaas
// prints is a type with an Error method rather than a sentinel.
type diagnostic string

// Error renders the diagnostic as written.
func (d diagnostic) Error() string { return string(d) }

// TestProblemTakesTheProblemAndLeavesTheFix covers the one thing a wrapper needs
// from a finished diagnostic, including the shapes an error harnaas did not
// write arrives in.
func TestProblemTakesTheProblemAndLeavesTheFix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "problem then fix",
			err:  diagnostic("the host refused\n\nCheck that this machine can reach it."),
			want: "the host refused",
		},
		{
			name: "no fix at all",
			err:  errors.New("the host refused"),
			want: "the host refused",
		},
		{
			name: "a fix of several paragraphs",
			err:  diagnostic("the host refused\n\nCheck the host.\n\nThen run harnaas install again."),
			want: "the host refused",
		},
		{
			name: "a single newline is not a paragraph break",
			err:  errors.New("the host refused\nand said so twice"),
			want: "the host refused\nand said so twice",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, source.Problem(tc.err))
		})
	}
}

// TestProblemCarriesTheRedactionTheTypeApplied holds the composition the
// transport diagnostics depend on: a caller quoting a cause's problem gets the
// message the type built, credential already removed, rather than the URL it
// holds.
func TestProblemCarriesTheRedactionTheTypeApplied(t *testing.T) {
	t.Parallel()

	err := &source.FetchError{
		URL: "https://harnaas:hunter2@api.github.com/repos/acme/assets/tarball/abc?token=signed",
		Err: errors.New("no such host"),
	}

	problem := source.Problem(err)

	assert.Contains(t, problem, "no such host")
	assert.NotContains(t, problem, "hunter2")
	assert.NotContains(t, problem, "token=")
}
