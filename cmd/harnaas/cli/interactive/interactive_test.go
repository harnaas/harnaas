package interactive

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEnv returns a getenv that answers from vars and reports every other name
// as unset, so a case only has to state the variables it is about.
func fakeEnv(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

func TestCanPromptGates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                string
		vars                map[string]string
		underTest           bool
		streamsAreTerminals bool
		want                bool
	}{
		{
			name:                "terminals on both streams with a clean environment",
			streamsAreTerminals: true,
			want:                true,
		},
		{
			name:                "piped streams",
			streamsAreTerminals: false,
			want:                false,
		},
		{
			name:                "the override forces prompting on despite piped streams",
			vars:                map[string]string{EnvTestTTY: "1"},
			streamsAreTerminals: false,
			want:                true,
		},
		{
			name:                "the override forces prompting off despite terminals",
			vars:                map[string]string{EnvTestTTY: "0"},
			streamsAreTerminals: true,
			want:                false,
		},
		{
			name:                "any other override value forces prompting off",
			vars:                map[string]string{EnvTestTTY: "yes"},
			streamsAreTerminals: true,
			want:                false,
		},
		{
			name:                "the override outranks every other gate",
			vars:                map[string]string{EnvTestTTY: "1", "CI": "true", "CLAUDECODE": "1"},
			underTest:           true,
			streamsAreTerminals: false,
			want:                true,
		},
		{
			name:                "running under go test",
			underTest:           true,
			streamsAreTerminals: true,
			want:                false,
		},
		{
			name:                "CI set",
			vars:                map[string]string{"CI": "true"},
			streamsAreTerminals: true,
			want:                false,
		},
		{
			name:                "CI set to false is the developer escape hatch",
			vars:                map[string]string{"CI": "false"},
			streamsAreTerminals: true,
			want:                true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := canPrompt(fakeEnv(c.vars), c.underTest, c.streamsAreTerminals)
			assert.Equal(t, c.want, got)
		})
	}
}

// Every recognized agent sentinel suppresses prompting on its own, on streams
// that would otherwise allow it.
func TestCanPromptIsFalseForEveryAgentSentinel(t *testing.T) {
	t.Parallel()

	for _, name := range agentEnvVars {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			env := fakeEnv(map[string]string{name: "1"})
			assert.False(t, canPrompt(env, false, true))
		})
	}
}

// A pipe is what a command gets when its output is redirected or when a script
// drives it, and it is the case the non-interactive contract turns on.
func TestCanPromptIsFalseForPipedStreams(t *testing.T) {
	t.Parallel()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, r.Close())
		assert.NoError(t, w.Close())
	})

	assert.False(t, IsTerminalReader(r), "a pipe read end is not a terminal")
	assert.False(t, IsTerminalWriter(w), "a pipe write end is not a terminal")
	assert.False(t, CanPrompt(r, w))
}

// A buffer is what a test hands a command, and it must answer the same way a
// pipe does rather than falling through to the process's own streams.
func TestCanPromptIsFalseForBuffers(t *testing.T) {
	t.Parallel()

	assert.False(t, IsTerminalReader(&bytes.Buffer{}))
	assert.False(t, IsTerminalWriter(&bytes.Buffer{}))
	assert.False(t, CanPrompt(&bytes.Buffer{}, &bytes.Buffer{}))
}

func TestShouldStyleGates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		noColor          string
		term             string
		isTerminalWriter bool
		want             bool
	}{
		{
			name:             "a terminal with an ANSI-capable TERM",
			term:             "xterm-256color",
			isTerminalWriter: true,
			want:             true,
		},
		{
			name:             "a writer that is not a terminal",
			term:             "xterm-256color",
			isTerminalWriter: false,
			want:             false,
		},
		{
			name:             "NO_COLOR set",
			noColor:          "1",
			term:             "xterm-256color",
			isTerminalWriter: true,
			want:             false,
		},
		{
			name:             "TERM=cygwin renders escapes as literal glyphs",
			term:             "cygwin",
			isTerminalWriter: true,
			want:             false,
		},
		{
			name:             "an unset TERM defers to the writer",
			isTerminalWriter: true,
			want:             true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := shouldStyle(c.noColor, c.term, c.isTerminalWriter)
			assert.Equal(t, c.want, got)
		})
	}
}

// The exported entry point must read the process environment, not just accept
// it as an argument: NO_COLOR is the first gate, so it decides even where the
// writer would have qualified.
func TestShouldStyleReadsTheEnvironment(t *testing.T) {
	t.Setenv(EnvNoColor, "1")

	assert.False(t, ShouldStyle(os.Stdout))
}
