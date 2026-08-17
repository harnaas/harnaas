// Package uiform is the only place harnaas builds a form.
//
// It exists so that two properties hold for every prompt without anyone having
// to remember them: the prompt renders in an accessible, screen-reader-friendly
// form when the environment asks for it, and its colours come from the
// terminal's own base palette rather than from a hex value picked on somebody
// else's machine. Both are one call each on the form library, and both are the
// kind of call that is omitted exactly once and then copied forever.
//
// Callers pass the command's own streams. A prompt is not the command's output:
// under --json the document on stdout must be the only thing there, so a form
// rendered on stdout would corrupt it. Prompts render on stderr.
//
// Whether a prompt may be shown at all is not this package's question — see
// the interactive package. Nothing here checks; a caller asks first.
package uiform

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/palette"
)

// EnvAccessible requests accessible mode. Set it to any non-empty value and
// prompts render as plain sequential questions and answers rather than as a
// redrawn screen, which is what a screen reader can follow.
const EnvAccessible = "ACCESSIBLE"

// ErrCancelled reports that the user dismissed a prompt rather than answering
// it. It is deliberately not folded into the negative answer: declining and
// walking away are different acts, and a command that treats a cancelled prompt
// as a "no" would go on to do the "no" thing to a user who asked for nothing at
// all. A caller maps this to writing nothing and exiting non-zero.
var ErrCancelled = errors.New("prompt cancelled by the user")

// ErrInterrupted reports that the user interrupted a prompt with Ctrl-C. It is
// also ErrCancelled — the question went unanswered either way — and callers
// deciding what to do need no more than that.
//
// It exists for the entrypoint, which needs to know that no signal is coming.
// A full-screen form puts the terminal in raw mode, and raw mode disables the
// line discipline's signal characters, so Ctrl-C is delivered to harnaas as a
// keystroke the form consumed rather than as SIGINT. Exiting normally on it is
// what would leave the user's Ctrl-C trapped inside a `while true` loop, so the
// entrypoint terminates as if the signal the terminal never sent had arrived.
var ErrInterrupted = errors.New("prompt interrupted by the user")

// cancelledError is ErrCancelled with the reason the prompt ended still
// attached. A prompt dismissed because the root context was cancelled was
// dismissed by a signal, and the entrypoint decides whether a run was
// signal-driven — and so whether to terminate by that signal rather than exit —
// from that cause alone. Returning a bare ErrCancelled would drop it, and the
// user's Ctrl-C would never escape an enclosing shell loop.
//
// It reads as ErrCancelled to a caller asking what happened, because what
// happened is the same either way: the question went unanswered.
type cancelledError struct{ cause error }

func (e *cancelledError) Error() string { return ErrCancelled.Error() }

func (e *cancelledError) Is(target error) bool { return target == ErrCancelled }

func (e *cancelledError) Unwrap() error { return e.cause }

// IsAccessibleMode reports whether accessible mode is requested.
func IsAccessibleMode() bool {
	return os.Getenv(EnvAccessible) != ""
}

// Theme returns harnaas's form theme: base16 slots throughout, so the prompt
// sits in the palette the user already chose for their terminal.
//
// It starts from the form library's own base16 theme and makes two corrections.
// Titles and unselected options drop their pinned foreground so they inherit
// the terminal's default text colour, which is the only colour that inverts
// with the background — the same reason the palette package declares no
// body-text alias. Selection is re-pointed at the accent slot so the one thing
// the user is meant to look at is the one thing the palette says it is.
func Theme() huh.Theme {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		t := huh.ThemeBase16(isDark)

		accent := lipgloss.Color(palette.Accent)

		// The base theme copies its focused styles into the blurred ones
		// wholesale, and copies the focused title into the group title, so
		// clearing only the focused variants leaves an inactive field in a
		// multi-field form still pinned to a slot that cannot invert.
		t.Focused.Title = t.Focused.Title.UnsetForeground()
		t.Focused.UnselectedOption = t.Focused.UnselectedOption.UnsetForeground()
		t.Blurred.Title = t.Blurred.Title.UnsetForeground()
		t.Blurred.UnselectedOption = t.Blurred.UnselectedOption.UnsetForeground()
		t.Group.Title = t.Group.Title.UnsetForeground()

		// The pointer and the chosen option are the accent, in both the single
		// and multiple selection forms.
		t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(accent)
		t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(accent)
		t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(accent)
		t.Focused.SelectedPrefix = t.Focused.SelectedPrefix.Foreground(accent)

		// An unfocused button reads as plain text — no chip, no pinned colour —
		// so it inverts with the terminal like everything else that is not
		// carrying meaning.
		t.Focused.BlurredButton = t.Focused.BlurredButton.UnsetForeground().UnsetBackground()
		t.Blurred.BlurredButton = t.Blurred.BlurredButton.UnsetForeground().UnsetBackground()

		// The focused button keeps its accent chip, with its label in the
		// terminal's background colour so it reads as reverse video against the
		// chip on a light terminal and on a dark one.
		label := lipgloss.LightDark(isDark)(lipgloss.Color(palette.BrightWhite), lipgloss.Color(palette.Black))
		t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(label)
		t.Blurred.FocusedButton = t.Blurred.FocusedButton.Foreground(label)

		return t
	})
}

// New builds a form on harnaas's theme, reading from in, rendering on out, and
// switched to accessible mode when the environment asks for it.
//
// Accessible mode is a property of the whole form rather than of a field, which
// is why even a single yes/no question is wrapped in one: a bare field cannot
// be made accessible, so a prompt built outside this package silently is not.
func New(in io.Reader, out io.Writer, groups ...*huh.Group) *huh.Form {
	if IsAccessibleMode() {
		// One line per read, for the whole form. The accessible path builds a
		// new buffered scanner for every question it asks and throws it away
		// with whatever it read ahead still in it — so a reader handed
		// "1\n0\n" loses the second line answering the first question, and
		// every question after that is answered by end-of-input. Reading a
		// line at a time is what makes a form of more than one question, or a
		// question asked more than once, answerable at all.
		in = newLineReader(in)
	}

	form := huh.NewForm(groups...).
		WithTheme(Theme()).
		WithInput(in).
		WithOutput(out)
	if IsAccessibleMode() {
		form = form.WithAccessible(true)
	}
	return form
}

// lineReader hands its caller at most one line per read, buffering the rest for
// the next one.
//
// The buffer belongs to the form rather than to the question, which is the whole
// point: the accessible path's own buffering does not survive between questions.
type lineReader struct{ buffered *bufio.Reader }

func newLineReader(r io.Reader) *lineReader {
	return &lineReader{buffered: bufio.NewReader(r)}
}

func (l *lineReader) Read(p []byte) (int, error) {
	var n int
	for n < len(p) {
		b, err := l.buffered.ReadByte()
		if err != nil {
			if n > 0 {
				// What was read is a whole answer as far as the caller is
				// concerned; the error is theirs to meet on the next read.
				return n, nil
			}
			return 0, err //nolint:wrapcheck // An io.Reader returns its source's error, io.EOF included.
		}
		p[n] = b
		n++
		if b == '\n' {
			break
		}
	}
	return n, nil
}

// Confirm asks a yes/no question and returns the answer, with def pre-selected.
// A cancelled prompt — Ctrl-C, or the root context cancelled by a signal —
// returns ErrCancelled and no answer. Where a cancelled context is what ended
// the prompt, the returned error also unwraps to that context's error, so the
// entrypoint can still tell a signal-driven run from an ordinary failure.
//
// The context is consulted directly rather than only through the error the form
// library returns, for two reasons. It reports a cancelled context as a timeout,
// because a timeout is the only reason it cancels one itself, and harnaas sets
// none. And in accessible mode it runs the prompt synchronously without
// watching the context at all, so a cancellation that arrives mid-question is
// only visible afterwards.
func Confirm(ctx context.Context, in io.Reader, out io.Writer, question string, def bool) (bool, error) {
	if cause := ctx.Err(); cause != nil {
		return false, &cancelledError{cause: cause}
	}

	answer := def
	form := New(in, out, huh.NewGroup(
		huh.NewConfirm().
			Title(question).
			Value(&answer),
	))
	err := form.RunWithContext(ctx)

	if cause := ctx.Err(); cause != nil {
		return false, &cancelledError{cause: cause}
	}
	if errors.Is(err, huh.ErrUserAborted) {
		// The form library binds this to Ctrl-C alone, so the user interrupted
		// the prompt — the terminal was just in raw mode and never turned the
		// keystroke into a signal.
		return false, &cancelledError{cause: ErrInterrupted}
	}
	if err != nil {
		return false, fmt.Errorf("confirm prompt: %w", err)
	}
	return answer, nil
}

// Choice is one option a [MultiSelect] offers: the line the user reads, and the
// value a chosen line stands for.
//
// The two are separate because they are read by different audiences. A person
// picks a harness by its display name, and the manifest holds its id — so a
// caller that had to render one string would either show the user a value they
// never type or write a display name into a file that only accepts ids.
type Choice[T comparable] struct {
	// Label is what the option reads as.
	Label string

	// Value is what choosing it means.
	Value T
}

// MultiSelect asks the user to choose from a list and returns what they chose,
// in the order the choices were offered rather than the order they were ticked
// — a list rendered from a selection has to read the same way twice.
//
// Choosing nothing returns [ErrNothingSelected] and no answer. The rule is
// enforced here rather than through the form library's own validation, which
// re-asks the question instead: in the accessible path it re-asks from the same
// reader, so an answer that has ended — a closed pipe, a user's Ctrl-D — is asked
// and answers nothing, forever. A prompt that cannot be answered must fail rather
// than spin, and refusing here behaves identically in both renderings.
//
// A cancelled prompt — Ctrl-C, or the root context cancelled by a signal —
// returns ErrCancelled and no answer, exactly as [Confirm] does, and for the same
// reason: declining and walking away are different acts. The context is consulted
// directly for the reasons named there.
func MultiSelect[T comparable](
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	question string,
	choices []Choice[T],
) ([]T, error) {
	if cause := ctx.Err(); cause != nil {
		return nil, &cancelledError{cause: cause}
	}

	options := make([]huh.Option[T], 0, len(choices))
	for _, choice := range choices {
		options = append(options, huh.NewOption(choice.Label, choice.Value))
	}

	var chosen []T
	form := New(in, out, huh.NewGroup(
		huh.NewMultiSelect[T]().
			Title(question).
			Options(options...).
			Value(&chosen),
	))
	err := form.RunWithContext(ctx)

	if cause := ctx.Err(); cause != nil {
		return nil, &cancelledError{cause: cause}
	}
	if errors.Is(err, huh.ErrUserAborted) {
		return nil, &cancelledError{cause: ErrInterrupted}
	}
	if err != nil {
		return nil, fmt.Errorf("selection prompt: %w", err)
	}
	if len(chosen) == 0 {
		return nil, ErrNothingSelected
	}

	return inOfferedOrder(choices, chosen), nil
}

// ErrNothingSelected reports a selection submitted with nothing chosen.
//
// It is deliberately not an ErrCancelled: the user answered the question, and
// the answer was empty. A caller decides what an empty answer means to it, which
// for a list that has to name at least one thing is a refusal naming the flag
// that supplies the same list without a prompt.
var ErrNothingSelected = errors.New("nothing was selected")

// inOfferedOrder re-orders an answer to match the list it was chosen from.
//
// The form returns values in the order they were ticked, which is a record of
// how somebody used a keyboard rather than a fact about the answer. Ordering by
// the offered list is what makes two users who chose the same things produce the
// same file.
func inOfferedOrder[T comparable](choices []Choice[T], chosen []T) []T {
	ordered := make([]T, 0, len(chosen))
	for _, choice := range choices {
		if slices.Contains(chosen, choice.Value) {
			ordered = append(ordered, choice.Value)
		}
	}
	return ordered
}
