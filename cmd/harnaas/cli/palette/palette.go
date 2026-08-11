// Package palette is the single source of truth for the colours harnaas puts on
// a terminal.
//
// Every colour here is a base16 slot — an ANSI number from 0 to 15 — and never
// a hex value or a 256-colour code. A slot is a request the terminal answers
// with its own theme, so harnaas's output inherits whatever the user already
// chose. A hex value overrides that choice, and the override is only ever right
// on the machine it was picked on: the shade of grey that reads as "secondary"
// against a dark background is invisible against a light one.
//
// The constants are plain strings rather than any styling library's colour
// type, which is what keeps this package dependency-free and importable from
// anywhere without an import cycle. A caller wraps as its own renderer requires.
//
// Reach for the bright slots (8–15) and for the renderer's faint attribute to
// build hierarchy. Adding a colour outside this file is how a CLI ends up with
// four different greys, none of which agree.
package palette

// The base16 slots, named after the ANSI colour each number selects.
const (
	Black         = "0"
	Red           = "1"
	Green         = "2"
	Yellow        = "3"
	Blue          = "4"
	Magenta       = "5"
	Cyan          = "6"
	White         = "7"
	BrightBlack   = "8"
	BrightRed     = "9"
	BrightGreen   = "10"
	BrightYellow  = "11"
	BrightBlue    = "12"
	BrightMagenta = "13"
	BrightCyan    = "14"
	BrightWhite   = "15"
)

// The semantic aliases. Style against these, and keep the raw slot names for
// the rare case where a colour carries no meaning at all — a slot name in a
// style declaration says what the colour is, where an alias says why it is
// there, and only the second survives a change of palette.
//
// There is deliberately no alias for body text, and adding one is the mistake
// this comment exists to prevent. Body text is left unstyled so it renders in
// the terminal's own default foreground, which inverts with the background.
// Pinning it to White would make every line of harnaas's output invisible on a
// light terminal, and pinning it to Black would do the same on a dark one.
// Unstyled is the only setting that is correct on both.
const (
	// Accent marks the one thing on screen the user is meant to look at first.
	Accent = Magenta
	// Accent2 frames detail that belongs to the accented thing without
	// competing with it.
	Accent2 = BrightMagenta
	// Muted is the single colour for secondary text, hints and borders. One
	// muted slot rather than a range of greys is what keeps de-emphasis
	// reading as one level instead of four.
	Muted = BrightBlack
	// Success, Error, Warning and Info carry outcome. These four map onto the
	// colours a terminal user already reads as good, bad, careful and
	// incidental, so they are not re-pointed to fit a brand.
	Success = Green
	Error   = Red
	Warning = Yellow
	Info    = Cyan
)
