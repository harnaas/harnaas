package source

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// frontmatterDelimiter opens and closes a frontmatter block. It is the same
// three characters at both ends, which is why splitting looks for the first line
// and then the next occurrence rather than for two different markers.
const frontmatterDelimiter = "---"

// Frontmatter is the YAML block at the head of a harness document, kept as the
// bytes it arrived as.
//
// The type exists to make pass-through structural rather than a rule somebody
// has to remember. harnaas reads a skill's `name` and refuses a mismatch instead
// of correcting it, and the reason is not politeness: a `rule` carries a `paths:`
// list whose meaning a re-serialization can change — quoting, ordering and
// folding are all a YAML writer's choice — so a document that went through a
// parser and back is not the document its author reviewed. This type therefore
// offers no way to produce YAML at all. It holds the block and the body it was
// split from, and [Frontmatter.Decode] reads values *out* of it; there is no
// encoder to reach for, so no later phase can rewrite frontmatter by accident.
type Frontmatter struct {
	// Raw is the YAML block exactly as it appeared, without its delimiter lines
	// and without the newline that ends the block.
	Raw []byte

	// Body is everything after the closing delimiter, exactly as it appeared. It
	// is kept beside Raw so a caller that needs the document's prose does not
	// have to split it a second time and reach a different answer.
	Body []byte
}

// SplitFrontmatter separates a document's frontmatter block from its body.
//
// The split is textual: a document has frontmatter when its very first line is
// the delimiter and a later line is the delimiter again. Nothing is parsed here,
// so a block whose YAML is invalid still splits — which is what lets a caller
// tell "this file has no frontmatter" from "this file's frontmatter is broken",
// two problems with two different edits.
//
// The second return reports whether a block was found at all. A document with no
// frontmatter is not an error at this level: only some asset types require one.
func SplitFrontmatter(content []byte) (Frontmatter, bool) {
	opening := []byte(frontmatterDelimiter + "\n")
	// A file written on Windows opens with the same three characters followed by
	// a carriage return, and refusing it would make the check depend on which
	// machine committed the skill.
	openingCRLF := []byte(frontmatterDelimiter + "\r\n")

	var rest []byte
	switch {
	case bytes.HasPrefix(content, opening):
		rest = content[len(opening):]
	case bytes.HasPrefix(content, openingCRLF):
		rest = content[len(openingCRLF):]
	default:
		return Frontmatter{}, false
	}

	block, body, closed := cutClosingDelimiter(rest)
	if !closed {
		return Frontmatter{}, false
	}

	return Frontmatter{Raw: block, Body: body}, true
}

// cutClosingDelimiter finds the line that closes the block and returns what came
// before it and what came after.
//
// The delimiter has to be a whole line: a body line reading `---` mid-sentence
// is not a close, and a `----` rule below the block is not one either. Matching
// on the line rather than on the prefix is what keeps a document whose prose
// begins with a horizontal rule from having its frontmatter silently extended.
func cutClosingDelimiter(rest []byte) (block, body []byte, closed bool) {
	for offset := 0; offset < len(rest); {
		end := bytes.IndexByte(rest[offset:], '\n')
		var line []byte
		var next int
		if end < 0 {
			line, next = rest[offset:], len(rest)
		} else {
			line, next = rest[offset:offset+end], offset+end+1
		}

		if string(bytes.TrimRight(line, "\r")) == frontmatterDelimiter {
			return rest[:offset], rest[next:], true
		}
		offset = next
	}

	return nil, nil, false
}

// Decode reads the block's values into v, which is the caller's own struct with
// the fields it needs.
//
// Decoding into a caller-declared type rather than into a map is deliberate: a
// map would be one value away from being handed to a YAML encoder, and the whole
// reason this type has no encoder is that harnaas must not rewrite frontmatter.
// Unknown fields are ignored, because a skill's frontmatter belongs to its author
// and carries whatever the harnesses reading it want — harnaas is one reader of
// that block, not its owner.
func (f Frontmatter) Decode(v any) error {
	if err := yaml.Unmarshal(f.Raw, v); err != nil {
		return fmt.Errorf("parsing frontmatter: %w", err)
	}
	return nil
}
