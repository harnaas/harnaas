package cli

import (
	"cmp"
	"slices"
)

// severity is how much a finding matters, and it decides the exit status.
//
// There are two, deliberately. A third — "info" — would be a finding nobody has
// to act on, and a report that contains those teaches a reader to skim past the
// line that mattered.
type severity string

const (
	// severityError is every state that is not both pinned and current. It
	// exits 2.
	severityError severity = "error"

	// severityWarning is advisory and leaves the installation reproducible and
	// current. It exits 0 unless --strict.
	severityWarning severity = "warning"
)

// finding is one discrepancy lint found.
//
// Every field but Asset and Path is required, including Remedy: a finding with
// no available remedy says so rather than omitting the field, because a reader
// who meets a blank there cannot tell "nothing to do" from "harnaas forgot".
type finding struct {
	// Asset is the asset the finding concerns, empty for a project-level one
	// such as a managed block.
	Asset string `json:"asset,omitempty"`

	// Path is the specific file, where one file of an installation is at fault
	// rather than the whole of it.
	Path string `json:"path,omitempty"`

	Severity severity `json:"severity"`
	Problem  string   `json:"problem"`
	Remedy   string   `json:"remedy"`
}

// lintReport is everything one run found.
type lintReport struct {
	Findings []finding `json:"findings"`

	// Unchecked names the assets update detection could not reach, and Skipped
	// the checks that did not run at all.
	//
	// Both exist so a clean report is never mistaken for a complete one. A run
	// that could not reach the network and said nothing would be reporting
	// "everything is current" on the strength of not having looked.
	Unchecked []string `json:"unchecked,omitempty"`
	Skipped   []string `json:"skipped,omitempty"`
}

// sort puts findings in their stable order: by asset, then path, then problem.
//
// Derived from the finding rather than from the order the checks ran, so
// reordering the manifest or the lockfile cannot reorder the report and two
// runs over identical state are identical line for line.
func (r *lintReport) sort() {
	slices.SortStableFunc(r.Findings, func(a, b finding) int {
		if byAsset := cmp.Compare(a.Asset, b.Asset); byAsset != 0 {
			return byAsset
		}
		if byPath := cmp.Compare(a.Path, b.Path); byPath != 0 {
			return byPath
		}
		return cmp.Compare(a.Problem, b.Problem)
	})
	slices.Sort(r.Unchecked)
	slices.Sort(r.Skipped)
}

// counts returns how many findings carried each severity.
func (r *lintReport) counts() (errors, warnings int) {
	for _, found := range r.Findings {
		switch found.Severity {
		case severityError:
			errors++
		case severityWarning:
			warnings++
		}
	}
	return errors, warnings
}

// LintFindingsError reports that lint ran its checks to completion and found at
// least one error-severity finding.
//
// It is a type of its own so the entrypoint can tell it from every other
// failure and exit 2 rather than 1. That distinction is the whole reason the
// status is reserved: a CI job has to be able to tell "lint crashed" from "lint
// worked and your harness has drifted", and a lint run that fails partway
// through is the former.
type LintFindingsError struct {
	// Errors is how many error-severity findings were reported, so the
	// entrypoint's log record says how bad it was without re-reading the
	// report.
	Errors int
}

func (e *LintFindingsError) Error() string {
	return "lint found problems"
}

// AlreadyPrinted reports that the findings have been written, so the entrypoint
// adds nothing. The report *is* the message.
func (e *LintFindingsError) AlreadyPrinted() bool { return true }
