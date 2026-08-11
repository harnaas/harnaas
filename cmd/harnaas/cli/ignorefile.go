package cli

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
)

// ignoreFileName is the project's version-control ignore file.
//
// Installed content is fully reproducible from the manifest plus the lockfile,
// so committing it duplicates state and turns every upstream bump into a large,
// meaningless diff. What is committed is the declaration and the record of what
// it resolved to; what is ignored is the result.
const ignoreFileName = ".gitignore"

// installedIgnoreBlock is the region harnaas owns inside the ignore file.
//
// The markers are `#` comments because that is the ignore file's only comment
// form, and the name is `installed` rather than a bare `harnaas` for the reason
// the memory file's block names itself `instructions`: harnaas owns a block in
// two different files, and a reader meeting one marker should not have to work
// out which.
var installedIgnoreBlock = managedBlock{
	file:  ignoreFileName,
	begin: "# harnaas:begin installed",
	end:   "# harnaas:end installed",
}

// ignoreEntry renders one installed path as an ignore-file entry.
//
// Every entry is anchored with a leading slash, which is what makes it name the
// one path harnaas installed rather than every path that ends the same way: an
// unanchored `rules/house-style.md` matches at any depth, and a repository with
// a `docs/rules/house-style.md` would find it untracked by an entry that was
// never about it.
//
// Anchoring also settles escaping without any. `#` and `!` are only special at
// the start of a line, and a line that starts with `/` cannot start with
// either.
func ignoreEntry(path string) string {
	return "/" + strings.TrimPrefix(path, "/")
}

// coarseIgnoreError refuses a block in which one entry would ignore another.
//
// This is the failure requirement 7.7 exists to prevent, caught where the block
// is built rather than trusted not to happen. An entry that is an ancestor of
// another is by definition a directory holding more than the path harnaas
// installed, and the shared skills directory is exactly where that costs
// somebody their hand-written skill: `.agents/skills/` as one entry untracks
// every skill in it, installed or not.
type coarseIgnoreError struct {
	// Ancestor is the entry that would swallow the other.
	Ancestor string

	// Descendant is the entry it would swallow.
	Descendant string
}

func (e *coarseIgnoreError) Error() string {
	return fmt.Sprintf(
		"the ignore entry %q covers %q, so it would untrack paths harnaas did not install\n\n"+
			"This is a defect in harnaas rather than anything to change in the project: "+
			"every entry in the block must name one installed path exactly.",
		e.Ancestor, e.Descendant,
	)
}

// renderIgnoreBlock is the body of the ignore file's managed block: one entry
// per installed path, sorted.
//
// Sorting is by the rendered entry rather than by asset, so the block is a
// function of the installed set alone: two projects that installed the same
// paths write the same block, and reordering the manifest changes nothing.
// Duplicates are collapsed because one path may be installed for several
// harnesses and the file is one file.
//
// The paths are project-scope destinations only. A user-scoped destination
// lives outside the repository, where this file has no reach and nothing to
// say.
func renderIgnoreBlock(paths []string) ([]byte, error) {
	entries := make([]string, 0, len(paths))
	for _, path := range paths {
		entries = append(entries, ignoreEntry(path))
	}
	slices.Sort(entries)
	entries = slices.Compact(entries)

	if err := refuseCoarseEntries(entries); err != nil {
		return nil, err
	}

	var body bytes.Buffer
	for _, entry := range entries {
		body.WriteString(entry)
		body.WriteByte('\n')
	}
	return body.Bytes(), nil
}

// refuseCoarseEntries reports an entry that would ignore another entry.
//
// The entries arrive sorted, so an ancestor is always immediately followed by
// the first path it covers and a single pass over neighbours is enough. The
// comparison is on a slash-terminated prefix rather than on the raw string, so
// `/a/skills` is not treated as an ancestor of `/a/skills-extra`.
func refuseCoarseEntries(entries []string) error {
	for i := 1; i < len(entries); i++ {
		previous := entries[i-1]
		if strings.HasPrefix(entries[i], strings.TrimSuffix(previous, "/")+"/") {
			return &coarseIgnoreError{Ancestor: previous, Descendant: entries[i]}
		}
	}
	return nil
}

// writeIgnoreBlock makes the ignore file's managed block list exactly these
// installed paths, and removes the block when there are none.
//
// Regenerating the whole block on every install is what prunes it: a
// destination convergence removed is simply not in the set the next block is
// built from, so there is no second code path that has to remember to take an
// entry out.
func writeIgnoreBlock(root string, paths []string) error {
	if len(paths) == 0 {
		return dropManagedBlock(root, installedIgnoreBlock)
	}

	body, err := renderIgnoreBlock(paths)
	if err != nil {
		return err
	}
	return writeManagedBlock(root, installedIgnoreBlock, body)
}
