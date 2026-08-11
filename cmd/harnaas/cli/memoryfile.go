package cli

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// memoryFileName is the project's committed memory file — the file a harness
// reads for always-on guidance without anybody having run install.
//
// `AGENTS.md` rather than any harness's own file: it is the one memory file
// several harnesses already read, and the harness whose file it is not reaches
// it through a bridge line. That is also what makes an instruction distinct
// from a rule at all — an instruction survives a fresh clone because it lives
// in a committed file, and a rule does not because it lives in an installed
// one.
const memoryFileName = "AGENTS.md"

// instructionBlock is the region harnaas owns inside the memory file.
//
// The markers are HTML comments because the memory file is Markdown, where they
// render as nothing. The name in them is `instructions` rather than a bare
// `harnaas`, because the ignore file's block is harnaas's too and a reader
// meeting one marker should not have to know which of harnaas's blocks it
// delimits.
var instructionBlock = managedBlock{
	file:  memoryFileName,
	begin: "<!-- harnaas:begin instructions -->",
	end:   "<!-- harnaas:end instructions -->",
}

// instruction is one instruction asset's contribution to the memory file.
//
// It carries the source alongside the content because the block is committed:
// a reader opening `AGENTS.md` months later meets text nobody on the team
// wrote, and the comment above it is the only thing in the file that says where
// it came from.
type instruction struct {
	// ID is the asset's id, which is what the block is ordered by.
	ID string

	// Source is the asset's source spelled the way the manifest declares it.
	Source string

	// Content is the asset's bytes, inlined verbatim.
	Content []byte
}

// renderInstructionBlock is the body of the memory file's managed block.
//
// The ordering is by asset id and not by manifest position, so moving an entry
// in `harnaas.json` regenerates a byte-identical block: the manifest's order
// says nothing about what the assets mean, and a diff that moved a thousand
// lines because somebody sorted their manifest would be a diff nobody reads.
//
// Content is inlined verbatim. The one byte that may be added is a newline
// after content that does not end in one, because the next asset's comment has
// to begin a line — every other byte of every asset arrives in the file exactly
// as it resolved.
func renderInstructionBlock(instructions []instruction) []byte {
	ordered := slices.Clone(instructions)
	slices.SortStableFunc(ordered, func(a, b instruction) int {
		return cmp.Compare(a.ID, b.ID)
	})

	var body bytes.Buffer
	for i, entry := range ordered {
		if i > 0 {
			body.WriteByte('\n')
		}
		fmt.Fprintf(&body, "<!-- harnaas instruction %q from %s -->\n", entry.ID, entry.Source)
		body.Write(entry.Content)
		if len(entry.Content) > 0 && entry.Content[len(entry.Content)-1] != '\n' {
			body.WriteByte('\n')
		}
	}

	return body.Bytes()
}

// writeInstructionBlock makes the memory file's managed block hold exactly
// these instruction assets, and removes the block when there are none.
//
// The empty case is a removal rather than an empty block on purpose: a block
// with nothing in it is a claim that harnaas has something to say here, and the
// state after the last instruction asset goes is that it does not.
func writeInstructionBlock(root string, instructions []instruction) error {
	if len(instructions) == 0 {
		return dropManagedBlock(root, instructionBlock)
	}
	return writeManagedBlock(root, instructionBlock, renderInstructionBlock(instructions))
}

// bridgeFileName is the file the bridge line lives in, and bridgeLine is the
// line itself.
//
// Claude Code does not read `AGENTS.md` at all, so the instruction block would
// be invisible to it without one line importing the file. Every other harness
// on the roster reads `AGENTS.md` directly and needs nothing.
//
// This is a single line rather than a managed block because it is a single
// line: a pair of markers around one import would be three lines of harnaas in
// a file the team writes, to own what a whole-line match already identifies.
const (
	bridgeFileName = "CLAUDE.md"
	bridgeLine     = "@" + memoryFileName
)

// matchesBridgeLine reports whether a line is the bridge.
//
// The line arrives with its ending still attached, because the caller splits
// without discarding one, so the newline is trimmed here alongside the trailing
// whitespace and carriage return that [matchesMarker] ignores for the same
// reason: a file edited on another platform is the same file.
//
// Leading whitespace is not trimmed — an indented `@AGENTS.md` is inside
// somebody's list or code fence, where it is prose about the import rather than
// the import, and harnaas neither counts it nor removes it.
func matchesBridgeLine(line string) bool {
	return strings.TrimRight(line, " \t\r\n") == bridgeLine
}

// setBridgeLine makes content carry exactly one bridge line.
//
// Where there is none, it is appended at the end, after a newline if the file
// did not end in one — an import that shares a line with the last sentence of
// the file is not an import. Where there is exactly one, wherever it sits, the
// content is returned untouched: the line works from anywhere, and moving it to
// the end would be harnaas editing a file to satisfy nothing but its own idea
// of where the line belongs.
//
// Where there are several, the first is kept and the rest are dropped. The
// first is where the author put it; the duplicates are what a re-run of an
// older harnaas, or a merge, left behind.
func setBridgeLine(content []byte) []byte {
	lines := splitLinesKeepingEndings(content)

	seen := false
	kept := make([][]byte, 0, len(lines))
	for _, line := range lines {
		if matchesBridgeLine(string(line)) {
			if seen {
				continue
			}
			seen = true
		}
		kept = append(kept, line)
	}

	if seen {
		return bytes.Join(kept, nil)
	}

	updated := bytes.Join(kept, nil)
	if len(updated) > 0 && updated[len(updated)-1] != '\n' {
		updated = append(updated, '\n')
	}
	return append(updated, bridgeLine...)
}

// clearBridgeLine removes every bridge line from content, and reports whether
// what is left is worth keeping as a file.
//
// "Nothing else" is read as nothing but whitespace. A file harnaas created
// holds the bridge line and a newline, and a file somebody added a blank line
// to is still a file whose only content was the bridge — but a file with a
// sentence in it is theirs, and it stays even when the bridge is gone.
func clearBridgeLine(content []byte) (remainder []byte, empty bool) {
	lines := splitLinesKeepingEndings(content)

	kept := make([][]byte, 0, len(lines))
	for _, line := range lines {
		if matchesBridgeLine(string(line)) {
			continue
		}
		kept = append(kept, line)
	}

	remainder = bytes.Join(kept, nil)
	return remainder, len(bytes.TrimSpace(remainder)) == 0
}

// splitLinesKeepingEndings splits content into lines with their line endings
// still attached, so joining them reproduces the input byte for byte.
//
// bytes.Split would discard the information needed to tell a file that ends in
// a newline from one that does not, and rebuilding with a fixed separator would
// rewrite every CRLF in a file harnaas was asked to add one line to.
func splitLinesKeepingEndings(content []byte) [][]byte {
	var lines [][]byte
	for len(content) > 0 {
		end := bytes.IndexByte(content, '\n')
		if end < 0 {
			lines = append(lines, content)
			break
		}
		lines = append(lines, content[:end+1])
		content = content[end+1:]
	}
	return lines
}

// writeBridgeLine ensures CLAUDE.md imports the memory file, creating it
// holding only that line when it is absent.
func writeBridgeLine(root string) error {
	content, err := readManagedFile(root, bridgeFileName)
	if err != nil {
		return err
	}

	updated := setBridgeLine(content)
	if bytes.Equal(updated, content) {
		return nil
	}

	return writeManagedFile(root, bridgeFileName, updated)
}

// dropBridgeLine removes the bridge line, and removes CLAUDE.md itself when the
// line was all it held.
//
// Deleting the file is what keeps a full uninstall from leaving a file behind
// that harnaas created and nobody wrote. It is bounded to the case where
// nothing but whitespace remains, because every other case is somebody's file
// that happened to import the memory file.
func dropBridgeLine(root string) error {
	content, err := readManagedFile(root, bridgeFileName)
	if err != nil {
		return err
	}
	if len(content) == 0 {
		return nil
	}

	remainder, empty := clearBridgeLine(content)
	if bytes.Equal(remainder, content) {
		return nil
	}

	if empty {
		if err := os.Remove(filepath.Join(root, bridgeFileName)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", bridgeFileName, err)
		}
		return nil
	}

	return writeManagedFile(root, bridgeFileName, remainder)
}
