package manifest

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Interpretation is a decoded manifest with every question about it answered.
//
// It exists only where the document has no violations at all. That is the whole
// point of the type: [Interpret] is the only way to obtain an [Asset], and it
// returns nothing on failure, so a document harnaas has objected to cannot reach
// a phase that would install from it while the objection went unread.
type Interpretation struct {
	// Document is the decoded manifest the interpretation came from, kept so a
	// later phase can read the version and the declared harness list without a
	// second decode.
	Document *Document

	// Sources is every `sources` entry parsed, keyed as the document keyed it.
	// Every key an asset references is present, because an asset referencing a
	// key that is not is a violation.
	Sources map[string]Source

	// Assets is every entry interpreted, in document order — the order the
	// author reads in review, and the order a later phase should act in.
	Assets []Asset
}

// Interpret reads a decoded document and answers every question its entries
// leave open: which source, what type, what id, which harnesses, which scope.
//
// It does not stop at the first problem. Decoding stops, because a document that
// will not parse has no second problem to find; interpretation does not, because
// every asset entry is independent and a manifest with three mistakes should not
// take three runs of the same command to fix. Every violation found is collected
// into one [ValidationError], ordered by asset index and then by field so two
// runs over one file produce byte-identical output.
//
// On any violation the interpretation is nil. A partially interpreted manifest
// is not a smaller manifest — it is a different one, and installing from it
// would place a subset of what the project declared while reporting success.
func Interpret(doc *Document) (*Interpretation, error) {
	var violations []*Violation

	sources := make(map[string]Source, len(doc.Sources))
	// Sorted rather than in map order so the parsed set is built the same way on
	// every run. The violations sort themselves afterwards regardless, but a
	// deterministic build order is one less thing to reason about.
	for _, key := range slices.Sorted(maps.Keys(doc.Sources)) {
		source, violation := ParseSource(key, doc.Sources[key])
		if violation != nil {
			violations = append(violations, violation)
			// The key is registered even though its value is unusable, because
			// the key is declared: an asset referencing it has made no mistake of
			// its own, and reporting one against that asset would name an edit
			// that does not fix anything.
			sources[key] = Source{Key: key}
			continue
		}
		sources[key] = source
	}

	// The manifest's own harness list is checked once, here, rather than once per
	// asset that inherited it: one misspelling in one list is one mistake.
	violations = append(violations, CheckHarnesses(doc.Harnesses)...)

	assets := make([]Asset, 0, len(doc.Assets))
	for _, entry := range doc.Assets {
		asset, entryViolations := interpretEntry(entry, doc.Harnesses, sources)
		if len(entryViolations) > 0 {
			violations = append(violations, entryViolations...)
			continue
		}
		assets = append(assets, asset)
	}

	// Uniqueness is the one question no single entry can answer, so it is asked
	// last, over the entries that had nothing else wrong with them. An entry that
	// failed above has no settled id to collide with.
	violations = append(violations, checkDuplicateIDs(assets)...)

	if len(violations) > 0 {
		sortViolations(violations)
		return nil, &ValidationError{Path: doc.Path, Violations: violations}
	}

	return &Interpretation{Document: doc, Sources: sources, Assets: assets}, nil
}

// interpretEntry answers every open question about one asset entry, collecting
// the violations rather than returning at the first.
//
// A question whose answer would have to be invented is not asked. Where the
// source string did not parse there is no path to infer a type or an id from, so
// inference is skipped unless the entry declared the field itself — asking anyway
// would report a second problem the author never wrote, and send them looking for
// it.
func interpretEntry(entry AssetEntry, harnesses []string, sources map[string]Source) (Asset, []*Violation) {
	var violations []*Violation

	ref, refViolation := ParseAssetRef(entry.Index, entry.Source)
	if refViolation != nil {
		violations = append(violations, refViolation)
	} else if keyViolation := CheckSourceKey(entry.Index, ref, sources); keyViolation != nil {
		violations = append(violations, keyViolation)
	}
	inferable := refViolation == nil

	var assetType AssetType
	if inferable || entry.Type != "" {
		resolved, violation := ResolveType(entry, ref)
		if violation != nil {
			violations = append(violations, violation)
		}
		assetType = resolved
	}

	var id string
	if inferable || entry.ID != "" {
		resolved, violation := ResolveID(entry, ref)
		if violation != nil {
			violations = append(violations, violation)
		}
		id = resolved
	}
	if violation := CheckID(entry.Index, id); violation != nil {
		violations = append(violations, violation)
	}

	targets, targetViolations := ResolveTargets(entry, harnesses)
	violations = append(violations, targetViolations...)

	scope, scopeViolation := ResolveScope(entry, assetType, targets)
	if scopeViolation != nil {
		violations = append(violations, scopeViolation)
	}

	if len(violations) > 0 {
		return Asset{}, violations
	}

	return Asset{
		Index:      entry.Index,
		ObjectForm: entry.ObjectForm,
		Ref:        ref,
		Type:       assetType,
		ID:         id,
		Targets:    targets,
		Scope:      scope,
	}, nil
}

// CheckID reports an id that is not a single safe path segment.
//
// An id names a file or directory the installer creates inside a harness's own
// directory, so a separator or a parent-directory segment in one would place
// content outside the tree harnaas manages — and would do it under a name a
// reviewer reading the manifest would not read as a path at all.
//
// An empty id passes, because it means an earlier step already reported why
// there is none, and a second sentence about the same missing thing is noise.
func CheckID(index int, id string) *Violation {
	if id == "" || (id != "." && id != ".." && !strings.ContainsAny(id, `/\`)) {
		return nil
	}

	return assetViolation(index, "id",
		fmt.Sprintf("%s has the id %q, which is not a single path segment", assetSubject(index), id),
		fmt.Sprintf(
			"Set %q on the entry in %s to a plain name with no %q, %q or %q segment, as in %q.",
			"id", FileName, "/", `\`, "..", "house-style",
		),
	)
}

// checkDuplicateIDs reports two assets of one type resolving to the same id.
//
// Uniqueness is per type rather than across the manifest, because the id is what
// a harness addresses the installed asset by and each type is addressed in its
// own namespace: a skill and a command may both be called `review` without either
// one shadowing the other.
//
// Each collision is attributed to the later entry and names the earlier one, so a
// duplicate is reported once rather than twice, and always against the entry an
// author is most likely to have just added.
func checkDuplicateIDs(assets []Asset) []*Violation {
	type identity struct {
		assetType AssetType
		id        string
	}

	firstUse := make(map[identity]int, len(assets))
	var violations []*Violation

	for _, asset := range assets {
		key := identity{assetType: asset.Type, id: asset.ID}
		first, seen := firstUse[key]
		if !seen {
			firstUse[key] = asset.Index
			continue
		}

		violations = append(violations, assetViolation(asset.Index, "id",
			fmt.Sprintf(
				"%s and %s are both the %s %q, and one would overwrite the other",
				assetSubject(first), assetSubject(asset.Index), asset.Type, asset.ID,
			),
			fmt.Sprintf(
				"Declare a different %q on one of the two entries in %s, or remove the entry you no longer want.",
				"id", FileName,
			),
		))
	}

	return violations
}

// sortViolations orders violations by asset index and then by field.
//
// The order is what makes the aggregate readable rather than merely complete: an
// author fixes a file top to bottom, and two runs over one manifest have to
// produce the same message or a diff of two runs is unreadable. Document-level
// violations sort first because [DocumentIndex] is negative — a problem with the
// whole document is the one that can explain the entries below it.
func sortViolations(violations []*Violation) {
	slices.SortStableFunc(violations, func(a, b *Violation) int {
		if byIndex := cmp.Compare(a.Index, b.Index); byIndex != 0 {
			return byIndex
		}
		return cmp.Compare(a.Field, b.Field)
	})
}
