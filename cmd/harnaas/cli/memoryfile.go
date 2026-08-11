package cli

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"
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
