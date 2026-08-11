package cli

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// The limits harnaas refuses to cross, and the one it only warns about.
//
// The first three are the harness's limits, not harnaas's. A SKILL.md past a
// megabyte, a memory file past four mebibytes and an import chain deeper than
// five are each *skipped* by the harnesses that define them — silently, with no
// message to the user — so installing into one of those conditions produces a
// green run and a file nobody reads. That is the single worst outcome for a
// tool whose purpose is telling a team what is in effect, which is why these
// fail the asset rather than warning about it.
//
// The fourth is deliberately not in that list. Assembled always-on content past
// roughly forty thousand characters is a degradation point rather than a rule
// any harness enforces: the content is still read, and the model simply attends
// to it less well. Nothing is skipped, so nothing is refused.
const (
	skillFileLimit        = 1 << 20 // 1 MB, per SKILL.md
	memoryFileLimit       = 4 << 20 // 4 MiB, per memory file
	maxImportDepth        = 5
	alwaysOnWarnThreshold = 40_000 // characters of assembled always-on content, per scope
)

// limitError reports rendered output the target harness would skip.
//
// It names the limit, the measured value and the asset, because a reader who is
// told only "too large" has to discover all three for themselves — and the
// measurement is of the *rendered* bytes rather than the source, since what the
// harness meets is what landed.
type limitError struct {
	// AssetID is the asset that produced the output, and Subject what was
	// measured, spelled as it appears to the reader.
	AssetID string
	Subject string

	// Measured and Limit are the value and the ceiling, in the unit Subject is
	// counted in.
	Measured int
	Limit    int

	// Unit is what the numbers count, and Fix the sentence that resolves it.
	Unit string
	Fix  string
}

func (e *limitError) Error() string {
	return fmt.Sprintf(
		"%q renders to %s of %d %s, past the %d %s the harness reads\n\n"+
			"Nothing was written for it: past that point the harness skips the file without saying so, "+
			"so installing it would report success for content nothing reads. %s",
		e.AssetID, e.Subject, e.Measured, e.Unit, e.Limit, e.Unit, e.Fix,
	)
}

// checkSkillSize refuses a rendered SKILL.md the harness would skip.
func checkSkillSize(assetID string, content []byte) error {
	if len(content) <= skillFileLimit {
		return nil
	}
	return &limitError{
		AssetID: assetID, Subject: "a " + skillFileName, Unit: "bytes",
		Measured: len(content), Limit: skillFileLimit,
		Fix: "Split the skill, or move its reference material into separate files beside it.",
	}
}

// checkMemoryFileSize refuses an assembled memory file the harness would skip.
//
// It is checked during planning, against the file the install *would* write,
// because the alternative is discovering it after the write — at which point
// the project's committed memory file has already been replaced by one no
// harness reads.
func checkMemoryFileSize(content []byte) error {
	if len(content) <= memoryFileLimit {
		return nil
	}
	return &limitError{
		AssetID: memoryFileName, Subject: "a memory file", Unit: "bytes",
		Measured: len(content), Limit: memoryFileLimit,
		Fix: "Declare fewer instruction assets, or move the longer ones to rules, which are separate files.",
	}
}

// importLine reports the path an import line names, and whether the line is one.
//
// The syntax is a line whose whole content is `@` followed by a path, which is
// the form the bridge line itself uses. An indented one is inside somebody's
// list or code fence and is prose about an import rather than an import, which
// is the rule [matchesBridgeLine] applies for the same reason.
func importLine(line string) (string, bool) {
	text := strings.TrimRight(line, " \t\r\n")
	if !strings.HasPrefix(text, "@") || len(text) == 1 || strings.ContainsAny(text, " \t") {
		return "", false
	}
	return text[1:], true
}

// checkImportDepth refuses an import chain deeper than the harness follows.
//
// `read` returns a file's content and whether it exists, so the walk is a pure
// function of a lookup rather than of a filesystem — the same shape every
// renderer has, and what lets the limit be exercised without building a tree of
// files five levels deep.
//
// A file that does not resolve ends that branch rather than failing: a memory
// file importing something not checked out is the author's arrangement, and it
// is not what this limit is about. A cycle is bounded by the depth ceiling
// itself, which is why there is no separate visited set — the chain is refused
// at the same point whether it is deep or circular.
func checkImportDepth(entry string, read func(path string) ([]byte, bool)) error {
	var walk func(path string, depth int) error
	walk = func(path string, depth int) error {
		if depth > maxImportDepth {
			return &limitError{
				AssetID: path, Subject: "an import chain", Unit: "levels",
				Measured: depth, Limit: maxImportDepth,
				Fix: "Flatten the imports, or inline the deepest file into the one that imports it.",
			}
		}

		content, found := read(path)
		if !found {
			return nil
		}
		for _, line := range strings.Split(string(content), "\n") {
			imported, isImport := importLine(line)
			if !isImport {
				continue
			}
			if err := walk(imported, depth+1); err != nil {
				return err
			}
		}
		return nil
	}

	return walk(entry, 1)
}

// contribution is one asset's share of the always-on content in a scope.
type contribution struct {
	// AssetID names the asset, and Characters is what it contributes.
	AssetID    string
	Characters int
}

// alwaysOnWarning reports that a scope's assembled always-on content has passed
// the threshold where a model attends to it less well.
//
// It returns the empty string below the threshold, so a caller writes whatever
// comes back and never has to ask whether there was anything to say.
//
// The largest contributors are named because the total on its own is not
// actionable: a team told they have fifty thousand characters of always-on
// content still has to work out which asset to look at, and the answer is
// almost always two or three of them.
func alwaysOnWarning(scope string, contributions []contribution) string {
	total := 0
	for _, entry := range contributions {
		total += entry.Characters
	}
	if total <= alwaysOnWarnThreshold {
		return ""
	}

	ranked := slices.Clone(contributions)
	slices.SortStableFunc(ranked, func(a, b contribution) int {
		if bySize := cmp.Compare(b.Characters, a.Characters); bySize != 0 {
			return bySize
		}
		// Ties break by id so two runs over one project say the same thing.
		return cmp.Compare(a.AssetID, b.AssetID)
	})
	if len(ranked) > 3 {
		ranked = ranked[:3]
	}

	named := make([]string, 0, len(ranked))
	for _, entry := range ranked {
		named = append(named, fmt.Sprintf("%q (%d)", entry.AssetID, entry.Characters))
	}

	return fmt.Sprintf(
		"%s scope now carries %d characters of always-on content, past the %d where models "+
			"attend to it less well. The largest are %s. Nothing was changed by this warning.\n",
		scope, total, alwaysOnWarnThreshold, strings.Join(named, ", "),
	)
}
