package uiform

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/palette"
)

func TestIsAccessibleModeFollowsTheEnvironment(t *testing.T) {
	t.Setenv(EnvAccessible, "")
	assert.False(t, IsAccessibleMode())

	t.Setenv(EnvAccessible, "1")
	assert.True(t, IsAccessibleMode())
}

// Accessible mode is honoured: with it requested, a prompt completes against
// streams that are not a terminal at all. The full-screen path cannot — it
// needs a terminal to redraw — so a prompt that answers here answered in its
// accessible form.
func TestAccessibleModeIsHonoured(t *testing.T) {
	t.Setenv(EnvAccessible, "1")

	var out bytes.Buffer
	answer, err := Confirm(t.Context(), strings.NewReader("y\n"), &out, "Use claude-code?", false)

	require.NoError(t, err)
	assert.True(t, answer, "the answer read from the input stream")
	assert.Contains(t, out.String(), "Use claude-code?", "the question is written as plain text")
}

// The accessible prompt reports the answer the user gave, not the default it
// was handed.
func TestAccessibleConfirmReadsTheAnswerFromInput(t *testing.T) {
	cases := []struct {
		name  string
		input string
		def   bool
		want  bool
	}{
		{name: "yes against a no default", input: "y\n", def: false, want: true},
		{name: "no against a yes default", input: "n\n", def: true, want: false},
		{name: "an empty line takes the default", input: "\n", def: true, want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(EnvAccessible, "1")

			var out bytes.Buffer
			answer, err := Confirm(t.Context(), strings.NewReader(c.input), &out, "Proceed?", c.def)

			require.NoError(t, err)
			assert.Equal(t, c.want, answer)
		})
	}
}

// A prompt renders on the writer it was given. Nothing may reach stdout, which
// under --json carries the document and nothing else.
func TestConfirmRendersOnTheGivenWriter(t *testing.T) {
	t.Setenv(EnvAccessible, "1")

	var out bytes.Buffer
	_, err := Confirm(t.Context(), strings.NewReader("y\n"), &out, "Proceed?", false)

	require.NoError(t, err)
	assert.NotEmpty(t, out.String(), "the prompt went to the writer the caller passed")
}

// A cancelled prompt is not a "no". A caller has to be able to tell the two
// apart to write nothing and exit non-zero on one and not the other.
func TestConfirmReportsCancellationSeparatelyFromDeclining(t *testing.T) {
	t.Setenv(EnvAccessible, "")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	answer, err := Confirm(ctx, strings.NewReader(""), &bytes.Buffer{}, "Proceed?", true)

	require.ErrorIs(t, err, ErrCancelled)
	assert.False(t, answer)
}

// The cancellation has to carry why it happened, not only that it happened. The
// entrypoint decides whether to terminate by signal from the cause alone, so a
// prompt that reported a bare ErrCancelled would turn a user's Ctrl-C into an
// ordinary exit — and an enclosing `while true` loop would keep respawning
// harnaas. The message stays the caller's, because what happened is unchanged.
func TestConfirmCancelledByContextCarriesTheContextsCause(t *testing.T) {
	t.Setenv(EnvAccessible, "")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := Confirm(ctx, strings.NewReader(""), &bytes.Buffer{}, "Proceed?", true)

	require.ErrorIs(t, err, ErrCancelled)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, ErrCancelled.Error(), err.Error())
}

// Ctrl-C typed at a full-screen prompt never becomes a signal: the form puts
// the terminal in raw mode, which disables the line discipline's signal
// characters, so harnaas receives the byte and the form consumes it. Reporting
// that as an ordinary cancellation is what would leave a user's Ctrl-C trapped
// inside a `while true` loop, so it is reported as the interrupt it was — and
// still as a cancellation, because the question still went unanswered.
func TestConfirmReportsCtrlCAsAnInterrupt(t *testing.T) {
	t.Setenv(EnvAccessible, "")

	// 0x03 is the byte a terminal in raw mode delivers for Ctrl-C.
	_, err := Confirm(t.Context(), strings.NewReader("\x03"), &bytes.Buffer{}, "Proceed?", true)

	require.ErrorIs(t, err, ErrInterrupted)
	require.ErrorIs(t, err, ErrCancelled)
	assert.NotErrorIs(t, err, context.Canceled,
		"no context was cancelled; the entrypoint must not read this as a delivered signal")
}

// The ids the fixture below offers, named because the assertions compare
// against them.
const (
	firstChoice  = "claude-code"
	secondChoice = "devin-cli"
)

// harnessChoices is the shape the init command offers: a display name to read
// and an id to write.
func harnessChoices() []Choice[string] {
	return []Choice[string]{
		{Label: "Claude Code (" + firstChoice + ")", Value: firstChoice},
		{Label: "Devin CLI (" + secondChoice + ")", Value: secondChoice},
	}
}

// The accessible selection reads its answer from the input stream: each number
// toggles that option, and zero submits.
func TestAccessibleMultiSelectReadsTheChoiceFromInput(t *testing.T) {
	t.Setenv(EnvAccessible, "1")

	var out bytes.Buffer
	chosen, err := MultiSelect(t.Context(), strings.NewReader("2\n0\n"), &out,
		"Which harnesses?", harnessChoices())

	require.NoError(t, err)
	assert.Equal(t, []string{secondChoice}, chosen)
	assert.Contains(t, out.String(), "Which harnesses?", "the question is written as plain text")
	assert.Contains(t, out.String(), "Claude Code ("+firstChoice+")", "every option is offered")
}

// The answer reads in the order the options were offered, not the order they
// were ticked. Two users who chose the same harnesses must produce the same
// `harnesses` list.
func TestMultiSelectReturnsChoicesInTheOfferedOrder(t *testing.T) {
	t.Setenv(EnvAccessible, "1")

	chosen, err := MultiSelect(t.Context(), strings.NewReader("2\n1\n0\n"), &bytes.Buffer{},
		"Which harnesses?", harnessChoices())

	require.NoError(t, err)
	assert.Equal(t, []string{firstChoice, secondChoice}, chosen,
		"ticked second-then-first, returned first-then-second")
}

// Submitting nothing is an answer, and an empty one is refused rather than
// re-asked. Re-asking is what the form library does, and in the accessible
// rendering it re-asks the same reader — so an answer that has ended is asked
// and answers nothing, forever.
func TestMultiSelectRefusesAnEmptySelection(t *testing.T) {
	t.Setenv(EnvAccessible, "1")

	chosen, err := MultiSelect(t.Context(), strings.NewReader("0\n"), &bytes.Buffer{},
		"Which harnesses?", harnessChoices())

	require.ErrorIs(t, err, ErrNothingSelected)
	assert.Nil(t, chosen)
	assert.NotErrorIs(t, err, ErrCancelled,
		"the question was answered; only the answer was empty")
}

// An input that ends before the question is answered ends the prompt, rather
// than being asked again in a loop nothing can break.
func TestMultiSelectDoesNotSpinOnExhaustedInput(t *testing.T) {
	t.Setenv(EnvAccessible, "1")

	_, err := MultiSelect(t.Context(), strings.NewReader(""), &bytes.Buffer{},
		"Which harnesses?", harnessChoices())

	require.ErrorIs(t, err, ErrNothingSelected)
}

// A cancelled selection is not an empty one, for the reason a cancelled confirm
// is not a "no": the question went unanswered, and the caller writes nothing.
func TestMultiSelectReportsCancellation(t *testing.T) {
	t.Setenv(EnvAccessible, "")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	chosen, err := MultiSelect(ctx, strings.NewReader(""), &bytes.Buffer{},
		"Which harnesses?", harnessChoices())

	require.ErrorIs(t, err, ErrCancelled)
	require.ErrorIs(t, err, context.Canceled,
		"the cause travels so the entrypoint can still tell a signalled run from a failed one")
	assert.Nil(t, chosen)
}

// Ctrl-C at a full-screen selection never became a signal, for the reason it
// never does at a confirm: the form put the terminal in raw mode and consumed
// the keystroke.
func TestMultiSelectReportsCtrlCAsAnInterrupt(t *testing.T) {
	t.Setenv(EnvAccessible, "")

	_, err := MultiSelect(t.Context(), strings.NewReader("\x03"), &bytes.Buffer{},
		"Which harnesses?", harnessChoices())

	require.ErrorIs(t, err, ErrInterrupted)
	require.ErrorIs(t, err, ErrCancelled)
	assert.NotErrorIs(t, err, context.Canceled,
		"no context was cancelled; the entrypoint must not read this as a delivered signal")
}

// The theme must not pin a foreground on anything that is body text. A base16
// slot always maps to that slot, so a title pinned to black is invisible on a
// dark terminal and one pinned to white is invisible on a light one — the same
// reason the palette package declares no body-text alias.
func TestThemeLeavesTitlesAndUnselectedOptionsUnstyled(t *testing.T) {
	t.Parallel()

	for _, isDark := range []bool{true, false} {
		styles := Theme().Theme(isDark)

		unstyled := map[string]lipgloss.Style{
			"Focused.Title":            styles.Focused.Title,
			"Focused.UnselectedOption": styles.Focused.UnselectedOption,
			"Blurred.Title":            styles.Blurred.Title,
			"Blurred.UnselectedOption": styles.Blurred.UnselectedOption,
			"Group.Title":              styles.Group.Title,
		}
		for name, style := range unstyled {
			assert.IsType(t, lipgloss.NoColor{}, style.GetForeground(),
				"isDark=%v: %s pins a foreground; it must inherit the terminal's default", isDark, name)
		}
	}
}

// Selection is the accent, and the accent is the palette's — not a colour
// picked here.
func TestThemeDrawsSelectionFromThePalette(t *testing.T) {
	t.Parallel()

	for _, isDark := range []bool{true, false} {
		styles := Theme().Theme(isDark)

		selection := map[string]lipgloss.Style{
			"Focused.SelectSelector":      styles.Focused.SelectSelector,
			"Focused.MultiSelectSelector": styles.Focused.MultiSelectSelector,
			"Focused.SelectedOption":      styles.Focused.SelectedOption,
			"Focused.SelectedPrefix":      styles.Focused.SelectedPrefix,
		}
		for name, style := range selection {
			assert.Equal(t, lipgloss.Color(palette.Accent), style.GetForeground(),
				"isDark=%v: %s does not use the palette's accent", isDark, name)
		}
	}
}
