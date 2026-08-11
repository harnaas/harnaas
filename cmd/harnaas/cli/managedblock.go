package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/jsonutil"
)

// managedFilePerm is the mode a managed-block file is created with. Both files
// harnaas contributes a block to — the project's memory file and its
// version-control ignore file — are committed, hand-edited text, so the
// ordinary non-executable default.
const managedFilePerm fs.FileMode = 0o644

// managedBlock is a marker-delimited region harnaas owns inside a file the team
// also writes.
//
// The two markers are whole lines. Matching a whole line rather than a prefix
// is what stops a marker quoted inside a fenced code block, or an indented one
// inside a list item, from being read as the start of harnaas's region — the
// same rule the frontmatter reader applies to its closing delimiter, and for
// the same reason: the file's author is allowed to write about harnaas.
//
// The comment syntax is a field rather than a constant because the two files
// that carry a block do not share one: a memory file is Markdown and takes an
// HTML comment, an ignore file takes a `#` line. Everything else about editing
// a block is identical, which is why there is one editor and not two.
//
// The file the region lives in is a field for the same reason it is not a
// parameter of every edit: a marker pair and the file it delimits are one fact,
// and a caller that could supply them separately could ask for the memory
// file's block to be written into the ignore file.
type managedBlock struct {
	// file is the file the region lives in, named the way the reader sees it:
	// relative to the project root.
	file string

	// begin is the line that opens the region.
	begin string

	// end is the line that closes it.
	end string
}

// blockSpan is where a managed block sits in a file's bytes: from the first
// byte of the begin marker's line to one past the newline that ends the end
// marker's line, so replacing the span replaces the whole region and nothing
// around it.
type blockSpan struct {
	start int
	end   int
	found bool
}

// locateManagedBlock finds b in content.
//
// A file with no block at all is not a failure — it is the ordinary case on a
// first install, and the state a removal converges on. What is a failure is a
// file whose markers do not pair up: harnaas would have to guess where its
// region ends, and every guess either swallows somebody's writing or leaves a
// marker behind that the next run reads differently again.
func locateManagedBlock(content []byte, b managedBlock) (blockSpan, error) {
	var span blockSpan
	open := -1

	for offset := 0; offset < len(content); {
		line, next := lineAt(content, offset)

		switch {
		case matchesMarker(line, b.begin):
			if open >= 0 || span.found {
				return blockSpan{}, &managedBlockError{File: b.file, Reason: blockDuplicated, Marker: b.begin}
			}
			open = offset
		case matchesMarker(line, b.end):
			if open < 0 {
				return blockSpan{}, &managedBlockError{File: b.file, Reason: blockUnopened, Marker: b.end}
			}
			span = blockSpan{start: open, end: next, found: true}
			open = -1
		}

		offset = next
	}

	if open >= 0 {
		return blockSpan{}, &managedBlockError{File: b.file, Reason: blockUnclosed, Marker: b.begin}
	}

	return span, nil
}

// lineAt returns the line beginning at offset and the offset the next line
// begins at, which is one past the newline or the end of the content for a
// final line that has none.
func lineAt(content []byte, offset int) (line string, next int) {
	if end := bytes.IndexByte(content[offset:], '\n'); end >= 0 {
		return string(content[offset : offset+end]), offset + end + 1
	}
	return string(content[offset:]), len(content)
}

// matchesMarker reports whether line is the marker.
//
// Trailing horizontal whitespace and a carriage return are ignored, so a file
// with CRLF endings and one an editor trimmed differently are both recognized.
// Leading whitespace is not: an indented marker is inside somebody's list or
// code block, not harnaas's.
func matchesMarker(line, marker string) bool {
	return strings.TrimRight(line, " \t\r") == marker
}

// setManagedBlock returns content with b's body replaced by body, appending the
// whole block where content does not have one.
//
// Everything outside the markers is carried through byte for byte. Where the
// block is appended, the only addition to the previous content is the newline a
// file not ending in one needs before a marker can begin a line — and the block
// is appended with no blank line before it deliberately, so that removing it
// again restores the previous bytes exactly rather than leaving a blank line
// behind for the next install to append a second one after.
func setManagedBlock(content []byte, b managedBlock, body []byte) ([]byte, error) {
	span, err := locateManagedBlock(content, b)
	if err != nil {
		return nil, err
	}

	rendered := renderManagedBlock(b, body)

	if !span.found {
		if len(content) == 0 {
			return rendered, nil
		}
		updated := make([]byte, 0, len(content)+len(rendered)+1)
		updated = append(updated, content...)
		if content[len(content)-1] != '\n' {
			updated = append(updated, '\n')
		}
		return append(updated, rendered...), nil
	}

	updated := make([]byte, 0, span.start+len(rendered)+len(content)-span.end)
	updated = append(updated, content[:span.start]...)
	updated = append(updated, rendered...)
	return append(updated, content[span.end:]...), nil
}

// removeManagedBlock returns content with b and both its markers gone.
//
// Content with no block comes back unchanged rather than as a failure: removal
// is what every run with nothing to contribute does, and most of those runs
// meet a file that never had a block.
func removeManagedBlock(content []byte, b managedBlock) ([]byte, error) {
	span, err := locateManagedBlock(content, b)
	if err != nil {
		return nil, err
	}
	if !span.found {
		return content, nil
	}

	updated := make([]byte, 0, len(content)-(span.end-span.start))
	updated = append(updated, content[:span.start]...)
	return append(updated, content[span.end:]...), nil
}

// renderManagedBlock is the block as it appears in a file: the begin marker,
// the body, the end marker, each on its own line.
//
// A body not ending in a newline is given one, because the end marker has to
// begin a line for the next run to recognize it. An empty body produces the two
// markers with nothing between them, which is a block harnaas owns and has
// nothing to put in — distinct from no block at all, and reached only by a
// caller that asked for it.
func renderManagedBlock(b managedBlock, body []byte) []byte {
	var out bytes.Buffer
	out.WriteString(b.begin)
	out.WriteByte('\n')
	if len(body) > 0 {
		out.Write(body)
		if body[len(body)-1] != '\n' {
			out.WriteByte('\n')
		}
	}
	out.WriteString(b.end)
	out.WriteByte('\n')
	return out.Bytes()
}

// writeManagedBlock puts body inside b's markers in b's file under the project
// root, creating the file when it is absent.
//
// A run that would write the bytes already there writes nothing at all. The
// content is what matters, so an unchanged file with an unchanged modification
// time is the honest record of a run that changed nothing — and it keeps a
// re-run out of every file watcher the team has pointed at their memory file.
func writeManagedBlock(root string, b managedBlock, body []byte) error {
	content, err := readManagedFile(root, b.file)
	if err != nil {
		return err
	}

	updated, err := setManagedBlock(content, b, body)
	if err != nil {
		return err
	}
	if bytes.Equal(updated, content) {
		return nil
	}

	return writeManagedFile(root, b.file, updated)
}

// dropManagedBlock removes b's block from b's file under the project root.
//
// An absent file, and a file that never carried the block, are both nothing to
// do. The file is left in place even where the block was the whole of it: what
// harnaas owns is the region between its markers, and the empty file that
// remains is a file the team can delete in one keystroke, where a file harnaas
// deleted is content nobody can get back from the manifest.
func dropManagedBlock(root string, b managedBlock) error {
	content, err := readManagedFile(root, b.file)
	if err != nil {
		return err
	}

	updated, err := removeManagedBlock(content, b)
	if err != nil {
		return err
	}
	if bytes.Equal(updated, content) {
		return nil
	}

	return writeManagedFile(root, b.file, updated)
}

// readManagedFile reads the file, reporting an absent one as empty content.
//
// "Not there yet" is the first-install case for every file that carries a
// block, and treating it as empty is what makes creating one the same code path
// as editing one.
func readManagedFile(root, name string) ([]byte, error) {
	//nolint:gosec // G304: the path is the project root harnaas resolved plus the block's own file name, and it is opened for reading only.
	content, err := os.ReadFile(filepath.Join(root, name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return content, nil
}

// writeManagedFile replaces the file's contents atomically.
//
// The write goes through jsonutil despite the package name and the file being
// Markdown or an ignore list: WriteFileAtomic takes arbitrary bytes and is the
// module's one atomic writer, and a second implementation would be a second
// place the stage-sync-rename sequence has to stay correct.
func writeManagedFile(root, name string, content []byte) error {
	if err := jsonutil.WriteFileAtomic(filepath.Join(root, name), content, managedFilePerm); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

// blockMarkerReason is why harnaas will not edit a file's managed block.
type blockMarkerReason string

const (
	// blockUnclosed is an opening marker with no closing one after it.
	blockUnclosed blockMarkerReason = "unclosed"

	// blockUnopened is a closing marker with no opening one before it.
	blockUnopened blockMarkerReason = "unopened"

	// blockDuplicated is a second opening marker in one file.
	blockDuplicated blockMarkerReason = "duplicated"
)

// managedBlockError refuses to edit a file whose markers do not pair up.
//
// The three reasons are one type because the reader's next action is the same
// in all three — open the file and look at the marker lines — and the message
// says which of them is there. What harnaas will not do is repair the markers
// itself: every repair is a guess about where the region the team can edit
// begins, and a wrong guess overwrites their writing with a block.
type managedBlockError struct {
	// File is the file, named the way the reader sees it: relative to the
	// project root.
	File string

	// Reason is which of the three shapes the markers are in.
	Reason blockMarkerReason

	// Marker is the marker line the reason is about.
	Marker string
}

func (e *managedBlockError) Error() string {
	var problem string
	switch e.Reason {
	case blockUnclosed:
		problem = fmt.Sprintf("%s opens a harnaas managed block with %s and never closes it", e.File, e.Marker)
	case blockUnopened:
		problem = fmt.Sprintf("%s closes a harnaas managed block with %s that was never opened", e.File, e.Marker)
	case blockDuplicated:
		problem = fmt.Sprintf("%s opens a harnaas managed block with %s more than once", e.File, e.Marker)
	}

	return problem + "\n\n" +
		fmt.Sprintf("Edit %s so the block appears once, opened and closed by its own marker line, "+
			"or delete the marker lines entirely and re-run to have harnaas write the block again.", e.File)
}
