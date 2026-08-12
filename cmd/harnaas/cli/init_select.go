package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
)

// harnessOrigin records how init arrived at a harness selection.
//
// The three origins are kept apart because each owes the user a different
// sentence: a flag-supplied list needs no explanation, a detected one is worth
// stating so a wrong guess is visible, and the default is the only case where
// harnaas chose a harness the project shows no sign of using.
type harnessOrigin int

const (
	// originFlag means the user named the harnesses explicitly.
	originFlag harnessOrigin = iota
	// originDetected means the project's own contents named them.
	originDetected
	// originDefault means nothing was detected and the roster's default was
	// used, because a manifest with an empty `harnesses` list would declare
	// assets and guarantee them for nothing.
	originDefault
)

// harnessSelection is the target list a scaffolded manifest will declare,
// together with how it was arrived at.
type harnessSelection struct {
	// Harnesses is the selection in the order it will be written, with
	// duplicates removed.
	Harnesses []harness.ID

	// Origin is how the list was arrived at.
	Origin harnessOrigin
}

// selectHarnesses resolves the manifest's `harnesses` list from what the user
// asked for and, failing that, from what the project shows.
//
// A flag-supplied list replaces detection entirely rather than adding to it.
// Merging the two would make the manifest depend on what happened to be in the
// working tree at the moment init ran, which is the opposite of what naming the
// harnesses explicitly asks for — and it would leave no way to scaffold a
// manifest that omits a harness the project already contains.
func selectHarnesses(root string, requested []string) (harnessSelection, error) {
	if len(requested) > 0 {
		ids, err := recognizedHarnesses(requested)
		if err != nil {
			return harnessSelection{}, err
		}
		return harnessSelection{Harnesses: ids, Origin: originFlag}, nil
	}

	if detected := detectHarnesses(root, harness.All()); len(detected) > 0 {
		return harnessSelection{Harnesses: detected, Origin: originDetected}, nil
	}

	return harnessSelection{Harnesses: []harness.ID{harness.Default}, Origin: originDefault}, nil
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

// detectHarnesses reports which recognized harnesses the project already shows
// evidence of, in the roster's order.
//
// Detection reads and creates nothing: it stats the paths the roster declares as
// evidence and nothing else. That matters more here than it looks, because init
// runs before the project is under harnaas management at all — a detection pass
// that created a directory to look inside would be the one file init is
// forbidden to write.
//
// Order is the roster's rather than the filesystem's, so two projects containing
// the same harnesses scaffold the same list. The roster is a parameter for that
// clause alone: it was taken as one while claude-code was the only entry, so
// "every detected harness appears, in a deterministic order" could be exercised
// before a second harness existed to exercise it rather than after.
func detectHarnesses(root string, roster []harness.Harness) []harness.ID {
	var detected []harness.ID
	for _, h := range roster {
		if usesHarness(root, h) {
			detected = append(detected, h.ID)
		}
	}
	return detected
}

// usesHarness reports whether any one of a harness's evidence paths exists in
// the project. Any single piece of evidence is enough — a project with a
// `CLAUDE.md` and no `.claude/` is still a project using Claude Code.
//
// The check is an Lstat rather than a Stat because the question is whether the
// harness left something behind, not whether that something resolves: a symlink
// named `.claude` pointing at a directory that is not checked out is still
// evidence, and following it would answer "no" for a project that plainly says
// yes.
func usesHarness(root string, h harness.Harness) bool {
	for _, evidence := range h.ProjectEvidence {
		if _, err := os.Lstat(filepath.Join(root, evidence)); err == nil {
			return true
		}
	}
	return false
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
