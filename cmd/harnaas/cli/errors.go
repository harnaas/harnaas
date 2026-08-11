package cli

// AlreadyPrintedError marks a failure whose user-facing explanation the command
// has already written to the terminal. The entrypoint — the only component that
// prints an error — recognises the mark and prints nothing further, so a
// friendly, formatted diagnostic is never followed by a raw restatement of the
// same failure.
//
// The wrapped cause stays reachable through errors.As and errors.Is: marking an
// error as printed is a statement about output, not a decision to discard what
// went wrong.
type AlreadyPrintedError struct {
	Err error
}

// Error returns the wrapped cause's message. The entrypoint does not print it,
// but tests, logs and errors.Is comparisons still need it to be truthful.
func (e *AlreadyPrintedError) Error() string {
	return e.Err.Error()
}

// Unwrap exposes the cause so callers can inspect it.
func (e *AlreadyPrintedError) Unwrap() error {
	return e.Err
}

// AlreadyPrinted reports that the user-facing message has already been written.
func (e *AlreadyPrintedError) AlreadyPrinted() bool {
	return true
}

// NewAlreadyPrintedError marks err as already explained to the user. Call it
// only after writing that explanation; returning an unmarked error the command
// also printed is what produces duplicate output.
func NewAlreadyPrintedError(err error) *AlreadyPrintedError {
	return &AlreadyPrintedError{Err: err}
}
