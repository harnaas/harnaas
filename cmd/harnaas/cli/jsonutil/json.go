// Package jsonutil holds harnaas's one JSON encoding and its one file write.
//
// Both exist because the defaults are wrong for what harnaas produces. Go's
// encoder escapes HTML by default, which silently rewrites the ampersand in a
// source ref or the angle brackets in a placeholder into escapes nobody asked
// for; and an ordinary file write truncates the destination before it has the
// replacement bytes, so an interrupted `harnaas init` would leave a manifest
// that no longer parses.
//
// Every JSON document harnaas emits — the --json output of a command and the
// manifest that init scaffolds — goes through Marshal here, so a document read
// from a terminal and a document read from disk are formatted identically.
package jsonutil

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// indent is the one indentation harnaas uses. It is a constant rather than a
// parameter because the point of this package is that there is a single answer:
// a manifest a person hand-edits and a --json document a person reads over
// should not be indented differently for want of a shared default.
const indent = "  "

// Marshal encodes v as harnaas's canonical JSON document: indented, with HTML
// left unescaped, and terminated by exactly one newline.
//
// The newline is part of the format, not a courtesy. A file without a trailing
// newline shows up in a diff as a modification to its last line the next time
// anything appends, and a document without one runs into the next thing a
// line-oriented reader prints.
//
// It is json.Encoder rather than json.MarshalIndent because only the encoder
// can be told to leave HTML alone. Encoder already writes the trailing newline.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetIndent("", indent)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encode json document: %w", err)
	}

	return buf.Bytes(), nil
}
