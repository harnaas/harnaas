package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// rawDocument is the shape the strict pass decodes into.
//
// It differs from [Document] in two deliberate ways. `version` is a pointer, so
// a missing field is distinguishable from a declared `0`. `assets` is a slice of
// raw messages, so each entry is decoded separately with its index in hand: an
// entry has no other name, and encoding/json attaches no array index to the
// errors it returns.
type rawDocument struct {
	Version   *int              `json:"version"`
	Harnesses []string          `json:"harnesses"`
	Sources   map[string]string `json:"sources"`
	Assets    []json.RawMessage `json:"assets"`
}

// rawAsset is the object form of an asset entry. The accepted field set is
// exactly this struct, and decoding rejects anything else.
type rawAsset struct {
	Source  string   `json:"source"`
	Type    string   `json:"type"`
	ID      string   `json:"id"`
	Targets []string `json:"targets"`
	Scope   string   `json:"scope"`
}

// fieldShapes says what each field must be, in the words the author wrote it
// in rather than in Go's.
//
// Deriving these from the struct types by reflection would produce messages
// naming Go types — `[]string`, `map[string]string` — which describe harnaas's
// implementation rather than the JSON the author is looking at. Field names do
// not collide across the two structs, so one table serves both.
var fieldShapes = map[string]string{
	"version":   "an integer",
	"harnesses": "an array of harness names",
	"sources":   "an object mapping a source key to a source string",
	"assets":    "an array of asset entries",
	"source":    "a string",
	"type":      "a string",
	"id":        "a string",
	"targets":   "an array of harness names",
	"scope":     "a string",
}

// unknownFieldPrefix is how encoding/json spells a field the document declares
// and the struct does not.
//
// The standard library returns a bare error for this case rather than a type,
// so recognising it means matching its text. The match is a best effort:
// [locator.translate] falls through to reporting the error as written if the
// wording ever changes, which is a worse message but never a wrong one.
const unknownFieldPrefix = "json: unknown field "

// unknownOffset says a failure has no byte offset to report, so the message
// names the file without pretending to a location.
const unknownOffset = int64(-1)

// Decode decodes a manifest from bytes, without touching the filesystem.
//
// It is the whole of decoding; [Load] is this function plus finding the file.
func Decode(data []byte) (*Document, error) {
	return decode(data, "")
}

// decode reads the version, then decodes the rest strictly, then decodes each
// asset entry. Every failure returns a nil document: a partially decoded
// manifest declares a different asset set than the one in the file.
func decode(data []byte, path string) (*Document, error) {
	loc := locator{data: data, path: path}

	version, err := decodeVersion(data, loc)
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var raw rawDocument
	if decodeErr := decoder.Decode(&raw); decodeErr != nil {
		return nil, loc.translate(decodeErr, decoder.InputOffset())
	}

	document := &Document{
		Path:      path,
		Version:   version,
		Harnesses: raw.Harnesses,
		Sources:   raw.Sources,
		Assets:    make([]AssetEntry, 0, len(raw.Assets)),
	}
	for index, rawEntry := range raw.Assets {
		entry, entryErr := decodeAssetEntry(index, rawEntry, loc)
		if entryErr != nil {
			return nil, entryErr
		}
		document.Assets = append(document.Assets, entry)
	}

	return document, nil
}

// decodeVersion reads `version` on its own, leniently, before anything else.
//
// Reading it inside the strict pass would report a manifest from a newer
// harnaas as an unknown field — the first field this binary does not know,
// which is an arbitrary one — and send the author looking for a typo in a file
// that has none. Read first, the same manifest produces the one message that
// helps: upgrade.
//
// The pass reads the whole document rather than streaming, which also settles
// trailing data: json.Unmarshal rejects a second top-level value, where a
// decoder would stop at the end of the first and silently drop the rest.
func decodeVersion(data []byte, loc locator) (int, error) {
	var probe struct {
		Version *int `json:"version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return 0, loc.translate(err, unknownOffset)
	}

	switch {
	case probe.Version == nil:
		return 0, loc.fail(
			fmt.Sprintf("the manifest declares no %q field", "version"),
			fmt.Sprintf(`Add "version": %d as the first field of %s.`, SupportedVersion, FileName),
		)
	case *probe.Version > SupportedVersion:
		return 0, loc.fail(
			fmt.Sprintf(
				"the manifest declares version %d and this harnaas understands version %d, so it was written by a newer harnaas",
				*probe.Version, SupportedVersion,
			),
			"Upgrade harnaas to a version that understands this manifest.",
		)
	case *probe.Version < SupportedVersion:
		return 0, loc.fail(
			fmt.Sprintf("the manifest declares version %d, which is not a manifest version harnaas defines", *probe.Version),
			fmt.Sprintf(`Set "version": %d in %s.`, SupportedVersion, FileName),
		)
	}

	return *probe.Version, nil
}

// decodeAssetEntry decodes one entry in whichever form it was written.
//
// The form is chosen from the first non-space byte rather than by attempting a
// string decode and falling back to an object one, so an entry that is neither
// is reported as what it is instead of as a failed decode of a form its author
// never used.
func decodeAssetEntry(index int, raw json.RawMessage, loc locator) (AssetEntry, error) {
	entryLoc := loc.forAsset(index)

	trimmed := bytes.TrimLeft(raw, " \t\r\n")
	if len(trimmed) == 0 {
		return AssetEntry{}, entryLoc.fail(entryLoc.subject()+" is empty", assetFormFix)
	}

	switch trimmed[0] {
	case '"':
		var source string
		if err := json.Unmarshal(raw, &source); err != nil {
			return AssetEntry{}, entryLoc.translate(err, unknownOffset)
		}
		return AssetEntry{Index: index, Source: source}, nil

	case '{':
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()

		var object rawAsset
		if err := decoder.Decode(&object); err != nil {
			return AssetEntry{}, entryLoc.translate(err, unknownOffset)
		}
		return AssetEntry{
			Index:      index,
			ObjectForm: true,
			Source:     object.Source,
			Type:       object.Type,
			ID:         object.ID,
			Targets:    object.Targets,
			Scope:      object.Scope,
		}, nil

	default:
		return AssetEntry{}, entryLoc.fail(
			entryLoc.subject()+" is neither a source string nor an object",
			assetFormFix,
		)
	}
}

// assetFormFix states both accepted entry forms, which is the fix for every
// entry that is neither.
var assetFormFix = fmt.Sprintf(
	"Write the entry in %s either as a source string, such as \"acme:skills/review\", "+
		"or as an object with a \"source\" field.",
	FileName,
)

// locator turns an encoding/json failure into a harnaas diagnostic, and a byte
// offset into the line and column an author can navigate to.
//
// A locator is scoped either to the whole document or, through [forAsset], to
// one asset entry, which is what decides whether a message says "the manifest"
// or names the entry.
type locator struct {
	data []byte
	path string

	// entry names the asset entry this locator's bytes came from, and is nil
	// for the document as a whole.
	entry *int
}

// forAsset returns a locator scoped to one asset entry.
func (l locator) forAsset(index int) locator {
	l.entry = &index
	return l
}

// subject names what a message is about: the manifest, or one entry in it.
func (l locator) subject() string {
	if l.entry == nil {
		return "the manifest"
	}
	return assetSubject(*l.entry)
}

// qualifier attributes a field to its asset entry, and is empty at document
// scope, where a field name is already unambiguous.
func (l locator) qualifier() string {
	if l.entry == nil {
		return ""
	}
	return " of " + l.subject()
}

// fail reports a problem with no single location in the file.
func (l locator) fail(problem, fix string) *DecodeError {
	return l.failAt(unknownOffset, problem, fix)
}

// failAt reports a problem at a byte offset into the document.
func (l locator) failAt(offset int64, problem, fix string) *DecodeError {
	line, column := l.locate(offset)
	return &DecodeError{Path: l.path, Problem: problem, Fix: fix, Line: line, Column: column}
}

// translate converts an encoding/json failure into a diagnostic naming what is
// wrong and the edit that fixes it.
//
// fallback supplies a byte offset for the failures the standard library reports
// without one; it is [unknownOffset] where the caller has none either.
func (l locator) translate(err error, fallback int64) *DecodeError {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return l.failAt(syntaxErr.Offset,
			fmt.Sprintf("%s is not valid JSON: %s", l.subject(), syntaxErr.Error()),
			fmt.Sprintf("Correct the JSON at that position in %s.", FileName),
		)
	}

	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return l.failAt(fallback,
			l.subject()+" ends in the middle of a JSON value",
			fmt.Sprintf("Complete the JSON in %s — a bracket or brace is likely unclosed.", FileName),
		)
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return l.wrongShape(typeErr)
	}

	if field, ok := unknownField(err); ok {
		return l.failAt(fallback,
			fmt.Sprintf("%s has the unknown field %q", l.subject(), field),
			fmt.Sprintf(
				"Remove or correct %q in %s. harnaas rejects a field it does not know because in a hand-edited file it is almost always a misspelling.",
				field, FileName,
			),
		)
	}

	return l.failAt(fallback,
		fmt.Sprintf("%s could not be decoded: %s", l.subject(), err.Error()),
		fmt.Sprintf("Correct %s and run the command again.", FileName),
	)
}

// wrongShape reports a field whose value is not the shape the manifest format
// gives it.
func (l locator) wrongShape(err *json.UnmarshalTypeError) *DecodeError {
	if err.Field == "" {
		return l.failAt(err.Offset,
			fmt.Sprintf("%s must be a JSON object, and this one is %s", l.subject(), article(err.Value)),
			fmt.Sprintf("Make the top level of %s an object with a \"version\" field.", FileName),
		)
	}

	shape, known := fieldShapes[err.Field]
	if !known {
		return l.failAt(err.Offset,
			fmt.Sprintf("the %q field%s is %s, which is not the shape harnaas expects there",
				err.Field, l.qualifier(), article(err.Value)),
			fmt.Sprintf("Correct the %q field in %s.", err.Field, FileName),
		)
	}

	return l.failAt(err.Offset,
		fmt.Sprintf("the %q field%s must be %s, and this one is %s",
			err.Field, l.qualifier(), shape, article(err.Value)),
		fmt.Sprintf("Edit %s so %q is %s.", FileName, err.Field, shape),
	)
}

// locate converts a byte offset into a 1-based line and column, or two zeroes
// where there is no offset to convert.
//
// An entry-scoped locator never reports a location: its entry was decoded from
// its own bytes, so every offset in the failure counts from the start of the
// entry and would name a line elsewhere in the file. Naming the entry's index
// is the honest answer, and a plausible line:column pointing at the wrong line
// is worse than none.
//
// encoding/json reports the offset *after* the byte it stopped on, so the
// reported position steps back one to name that byte rather than the one
// following it.
func (l locator) locate(offset int64) (line, column int) {
	if l.entry != nil || offset <= 0 || offset > int64(len(l.data)) {
		return 0, 0
	}

	line, column = 1, 1
	for _, b := range l.data[:offset-1] {
		if b == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return line, column
}

// unknownField extracts the field name from encoding/json's unknown-field
// error, reporting whether the error was one.
func unknownField(err error) (string, bool) {
	message := err.Error()
	if !strings.HasPrefix(message, unknownFieldPrefix) {
		return "", false
	}

	name, unquoteErr := strconv.Unquote(strings.TrimPrefix(message, unknownFieldPrefix))
	if unquoteErr != nil {
		return "", false
	}
	return name, true
}

// article prefixes a JSON type name as encoding/json spells it — "string",
// "number", "array" — with the article that makes it read as a sentence.
func article(jsonType string) string {
	if jsonType == "" {
		return "a value of another type"
	}
	if strings.ContainsRune("aeiou", rune(jsonType[0])) {
		return "an " + jsonType
	}
	return "a " + jsonType
}
