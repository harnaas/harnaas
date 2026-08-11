package source

import "strings"

// Problem returns a diagnostic's problem: what is wrong, without the edit or
// command that fixes it.
//
// Every user-facing message harnaas writes is a problem, a blank line and a fix.
// Where one error is wrapped by another with a better fix to offer — a transport
// failure attributed to the asset whose source produced it, which is the only
// thing that knows which manifest line to edit — the wrapper needs the cause's
// problem and not its remedy: two remedies in one message leave the reader
// deciding which of them is theirs.
//
// An error that is not shaped that way is its own problem entire, which is the
// right answer for a cause harnaas did not write.
func Problem(err error) string {
	problem, _, _ := strings.Cut(err.Error(), "\n\n")
	return problem
}
