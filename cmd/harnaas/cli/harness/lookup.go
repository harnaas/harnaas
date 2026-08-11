package harness

import (
	"fmt"
	"slices"
	"strings"
)

// UnknownError reports a harness id the roster does not recognize.
//
// It is a type rather than a formatted string so manifest validation can
// recognise the condition — and attribute it to the asset entry or the
// `harnesses` list that produced it — without matching on message text.
type UnknownError struct {
	// ID is the unrecognized id exactly as it was written, so the message can
	// quote the manifest back to its author rather than paraphrase it.
	ID ID

	// Recognized is every id the roster does recognize, in the roster's order.
	// It is captured rather than looked up at render time so the error stays a
	// complete diagnostic wherever it is finally printed.
	Recognized []ID
}

// Error states the problem and the edit that fixes it, in that order, as every
// user-facing diagnostic does.
//
// Listing the recognized ids is most of the fix: the failure is almost always a
// misspelling or a harness harnaas does not support yet, and those two need
// different responses from the author.
func (e *UnknownError) Error() string {
	names := make([]string, 0, len(e.Recognized))
	for _, id := range e.Recognized {
		names = append(names, string(id))
	}

	return fmt.Sprintf(
		"unknown harness %q\n\n"+
			"Use a harness harnaas recognizes: %s.",
		e.ID, strings.Join(names, ", "),
	)
}

// IDs returns every recognized harness id in the roster's order.
//
// Callers get a fresh slice, because this list is rendered into user-facing
// errors and sorted into manifests; a caller that reorders it in place would
// change what every later message says.
func IDs() []ID {
	ids := make([]ID, 0, len(roster))
	for _, h := range roster {
		ids = append(ids, h.ID)
	}
	return ids
}

// All returns every recognized harness in the roster's order.
//
// The returned entries are copies down to their evidence slices, so the roster
// cannot be edited through a value a caller was handed.
func All() []Harness {
	all := make([]Harness, 0, len(roster))
	for _, h := range roster {
		all = append(all, clone(h))
	}
	return all
}

// Lookup returns the harness with the given id, or an *UnknownError naming the
// input and listing what harnaas does recognize.
func Lookup(id ID) (Harness, error) {
	for _, h := range roster {
		if h.ID == id {
			return clone(h), nil
		}
	}
	return Harness{}, &UnknownError{ID: id, Recognized: IDs()}
}

// Recognized reports whether id names a harness in the roster. It is the
// question to ask where the answer is a branch rather than a diagnostic; where
// it is a diagnostic, call Lookup and return its error, which already names the
// input and the alternatives.
func Recognized(id ID) bool {
	return slices.ContainsFunc(roster, func(h Harness) bool { return h.ID == id })
}

// HasPerUserLocation reports whether the harness reads assets from an
// unambiguous per-user location. This is the query manifest scope validation
// calls to decide whether an asset may declare `user` scope for this target.
//
// An unrecognized id is an error and never a false: "harnaas does not know this
// harness" and "this harness has no per-user location" call for different edits
// from the author, and collapsing them would report the second for a typo.
func HasPerUserLocation(id ID) (bool, error) {
	h, err := Lookup(id)
	if err != nil {
		return false, err
	}
	return h.PerUserLocation, nil
}

// clone copies a roster entry and its evidence slice, so a caller holding the
// result shares no backing array with the roster.
func clone(h Harness) Harness {
	h.ProjectEvidence = slices.Clone(h.ProjectEvidence)
	return h
}
