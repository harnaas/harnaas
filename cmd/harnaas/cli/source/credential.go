package source

import "strings"

// A credential is the one value harnaas handles that must never be printed, and
// the type is built around that rather than the callers being asked to remember
// it: the token travels together with where it was read from, and every way a
// credential can be rendered names the origin and never the secret. A diagnostic
// that has to tell the reader which token was used therefore has something safe
// to name, which is what stops a message from reaching for the value itself.

// Credential is what harnaas presents on a request, together with the name of
// where it came from.
//
// The zero value is the unauthenticated request, which is a legitimate way to
// fetch a public source rather than a missing configuration — so a fetch is
// always asked for a credential, and "none" is one of the answers rather than a
// second method to call.
type Credential struct {
	// Token is the secret itself. It is never rendered by this package, and
	// nothing in harnaas may log it, print it or write it to a file.
	Token string

	// Origin names where the token was read from — an environment variable, in
	// the only chain harnaas has today. It exists so a diagnostic can tell a
	// reader which token was used without quoting one, which is the difference
	// between an actionable message and a leaked credential.
	Origin string
}

// Present reports whether harnaas holds a token to send.
func (c Credential) Present() bool { return strings.TrimSpace(c.Token) != "" }

// String renders the credential as its origin.
//
// It exists so that a credential caught by a `%v` or a `%s` in a message, a log
// record or a test failure prints where the token came from instead of the
// token. Without it, fmt would render the struct field by field and the one
// value that must never appear in output would be the first thing in it.
func (c Credential) String() string {
	if !c.Present() {
		return "no credential"
	}
	if c.Origin == "" {
		return "a credential"
	}
	return "the credential from " + c.Origin
}

// GoString covers `%#v`, which ignores [Credential.String] and would otherwise
// print the struct literal — token included.
func (c Credential) GoString() string {
	return "source.Credential(" + c.String() + ")"
}
