package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/interactive"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/uiform"
)

// selectionQuestion is what the prompt asks. It names the file the answer ends
// up in, because that file is the thing being decided: the list is a guarantee
// a team publishes about itself, not a preference for this run.
const selectionQuestion = "Which harnesses does this project target?"

// selectHarnesses resolves the manifest's `harnesses` list from the user, and
// only from the user.
//
// A flag-supplied list is the whole answer and suppresses the prompt: the user
// typed it a moment ago, and asking them to confirm their own words wastes the
// one moment they are paying attention. Otherwise the roster is presented and
// they choose.
//
// What this deliberately never does is look at the project. The `harnesses` list
// means "we guarantee the declared assets work for these harnesses", and the
// contents of a working tree answer a different question — which harnesses have
// left a file here — that disagrees with the first in both directions. See ADR
// 0006 and the change that removed detection.
func selectHarnesses(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	requested []string,
) ([]harness.ID, error) {
	if len(requested) > 0 {
		return recognizedHarnesses(requested)
	}

	if !interactive.CanPrompt(in, out) {
		return nil, &noSelectionError{}
	}

	chosen, err := uiform.MultiSelect(ctx, in, out, selectionQuestion, rosterChoices())
	if errors.Is(err, uiform.ErrNothingSelected) {
		// The question was answered, and the answer was empty. That is not a
		// cancellation — nobody walked away — and it is not a manifest either:
		// an empty `harnesses` list declares assets and guarantees them for
		// nothing.
		return nil, &noSelectionError{answered: true}
	}
	if errors.Is(err, uiform.ErrCancelled) {
		// Cancelling is not declining. The user answered nothing, so harnaas
		// does nothing and says so with a non-zero exit, rather than taking an
		// answer on behalf of someone who walked away.
		return nil, fmt.Errorf("%w: no %s was created", err, manifest.FileName)
	}
	if err != nil {
		return nil, fmt.Errorf("select harnesses: %w", err)
	}

	return chosen, nil
}

// rosterChoices is the roster as a list a person reads: the display name they
// recognize, and the id the manifest will hold, in the roster's own order.
//
// Both strings are shown because both are needed and neither substitutes for the
// other — the name is what a user knows the harness by, and the id is what they
// would have to type into `--harness` or into the file afterwards. Nothing is
// pre-selected: a pre-ticked box is a guess about the project in a form that is
// harder to notice than a sentence.
func rosterChoices() []uiform.Choice[harness.ID] {
	roster := harness.All()
	choices := make([]uiform.Choice[harness.ID], 0, len(roster))
	for _, h := range roster {
		choices = append(choices, uiform.Choice[harness.ID]{
			Label: fmt.Sprintf("%s (%s)", h.DisplayName, h.ID),
			Value: h.ID,
		})
	}
	return choices
}

// noSelectionError refuses a run that has no way to obtain a selection.
//
// It is one type with two problems rather than two types, because the fix is the
// same sentence in both cases and the reader's next action is identical: name the
// harnesses. What differs is what happened — nobody could be asked, or somebody
// was asked and chose nothing — and stating the wrong one sends a reader looking
// for a terminal they do have.
type noSelectionError struct {
	// answered records that a selection was presented and came back empty,
	// rather than never being possible at all.
	answered bool
}

func (e *noSelectionError) Error() string {
	problem := "harnaas cannot ask which harnesses this project targets, and none were named"
	if e.answered {
		problem = "no harness was selected, and a project must target at least one"
	}

	return fmt.Sprintf(
		"%s\n\n"+
			"Name them with --harness, repeating the flag for each one, as in `harnaas init --harness %s`.\n"+
			"Recognized harnesses: %s.",
		problem, harness.Default, strings.Join(recognizedIDs(), ", "),
	)
}

// recognizedIDs is every id a manifest may name, in the roster's order, for a
// message that has to list them.
func recognizedIDs() []string {
	roster := harness.All()
	ids := make([]string, 0, len(roster))
	for _, h := range roster {
		ids = append(ids, string(h.ID))
	}
	return ids
}

// recognizedHarnesses turns the flag's raw strings into roster ids, failing on
// the first name harnaas does not recognize.
//
// The roster's own error is returned unwrapped: it already names the input and
// lists what harnaas does recognize, which is the whole diagnostic, and the user
// is looking at the flag they just typed.
//
// A name repeated across flag occurrences is kept once. The manifest is a
// declaration rather than a log of what was typed, and a `harnesses` list naming
// one harness twice would be a file harnaas asked its author to hand-edit and
// then wrote badly itself.
func recognizedHarnesses(names []string) ([]harness.ID, error) {
	ids := make([]harness.ID, 0, len(names))
	for _, name := range names {
		id := harness.ID(name)
		if _, err := harness.Lookup(id); err != nil {
			return nil, err
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// displayNames renders a selection the way it is said to a person, using the
// roster's display names rather than the ids the manifest will hold.
//
// An id that is somehow not on the roster falls back to itself rather than
// dropping out of the sentence: a selection is only ever built from recognized
// ids, and a message that silently listed fewer harnesses than it was about
// would be worse than one naming a raw id.
func displayNames(ids []harness.ID) string {
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		h, err := harness.Lookup(id)
		if err != nil {
			names = append(names, string(id))
			continue
		}
		names = append(names, h.DisplayName)
	}
	return strings.Join(names, ", ")
}
