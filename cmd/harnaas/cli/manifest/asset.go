package manifest

import (
	"fmt"
	"slices"
	"strings"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
)

// Scope is where an asset's installed copy lives: beside the project, or in the
// target harness's own per-user location.
//
// It is declared per asset rather than chosen per run because it is a property
// of the content — a team's house-style rule belongs to the repository, a
// personal shortcut belongs to the person — and a flag would make one manifest
// install to two different places depending on who ran the command.
type Scope string

const (
	// ScopeProject installs beneath the project root of each target harness. It
	// is the default, and the only scope an instruction may take.
	ScopeProject Scope = "project"

	// ScopeUser installs into the target harness's per-user location, and is
	// accepted only for a harness the roster records as having an unambiguous
	// one.
	ScopeUser Scope = "user"
)

// scopes is every accepted scope, in the order every diagnostic lists them —
// the default first, because it is the one an author reading the list most
// often wants.
var scopes = []Scope{ScopeProject, ScopeUser}

// Asset is one manifest entry with every question about it answered: what it
// is, what it is called, which harnesses it is guaranteed for, and where it
// installs.
//
// Nothing here is optional. Every field an entry may leave undeclared has been
// inferred from its path or defaulted from the document by the time an Asset
// exists, which is what makes it the shape later phases read: `install` never
// has to know whether the author wrote `type` or relied on the directory name,
// and so cannot treat the two differently.
type Asset struct {
	// Index is the entry's position in the `assets` array, carried through
	// because it stays the way every diagnostic points at the line to edit.
	Index int

	// ObjectForm records whether the entry was written as an object, so a
	// remedy can direct an author to the object form only when they are not
	// already using it.
	ObjectForm bool

	// Ref is the parsed source string: which source, and the path within it.
	Ref AssetRef

	// Type is the asset's type, declared or inferred from the containing
	// directory.
	Type AssetType

	// ID is the asset's id, declared or inferred from the path's leaf.
	ID string

	// Targets is the effective target list: the entry's own `targets` where it
	// declared them, and the manifest's `harnesses` otherwise. It is never
	// empty — an asset with no target could never be installed anywhere, which
	// is a violation rather than a state an Asset can reach.
	Targets []harness.ID

	// Scope is where the asset installs, `project` unless the entry declared
	// otherwise.
	Scope Scope
}

// ResolveType returns the entry's type: the one it declares, or the one its
// path implies.
//
// A declared type suppresses inference entirely rather than being checked
// against it. The object form exists precisely for a source whose layout does
// not follow the convention, so comparing the two would refuse the case it was
// added for.
func ResolveType(entry AssetEntry, ref AssetRef) (AssetType, *Violation) {
	if entry.Type == "" {
		return InferType(entry.Index, ref)
	}

	declared := AssetType(entry.Type)
	if !slices.Contains(AssetTypes(), declared) {
		return "", assetViolation(entry.Index, "type",
			fmt.Sprintf("%s declares the type %q, which is not a type harnaas installs", assetSubject(entry.Index), entry.Type),
			fmt.Sprintf("Set %q on the entry in %s to one of %s.", "type", FileName, typeList()),
		)
	}

	return declared, nil
}

// ResolveID returns the entry's id: the one it declares, or the one its path's
// leaf implies.
//
// Inference is suppressed one field at a time, which is why this is a separate
// decision from [ResolveType]: an entry declaring `type` for an unconventional
// layout still wants its id inferred, and an entry declaring `id` because two
// sources use the same leaf still wants its type inferred.
//
// Whether the resulting id is usable — a single safe path segment, unique among
// assets of its type — is a question about the document rather than the entry,
// and is answered where every entry's id is in hand.
func ResolveID(entry AssetEntry, ref AssetRef) (string, *Violation) {
	if entry.ID == "" {
		return InferID(entry.Index, ref)
	}
	return entry.ID, nil
}

// ResolveTargets returns the harnesses an entry is guaranteed for: its own
// `targets` where it declared them, and the manifest's `harnesses` otherwise.
//
// A name inherited from `harnesses` is not checked here. It is checked once by
// [CheckHarnesses], because a misspelling in that one list is one mistake, and
// attributing it to every asset that inherited it would report the same edit
// once per entry and bury the entries with problems of their own.
//
// The result is a list of violations rather than one because `targets` is
// itself a list: two unrecognized names are two independent edits, and each is
// attributed to its own position so the aggregate orders them the same way on
// every run.
func ResolveTargets(entry AssetEntry, harnesses []string) ([]harness.ID, []*Violation) {
	// A nil slice is an undeclared field and an empty one is a declared empty
	// list. Both leave the asset with nowhere to install, but the edit that
	// fixes them is different, so decoding kept them distinguishable and this
	// is where the distinction is spent.
	if entry.Targets == nil {
		if len(harnesses) == 0 {
			return nil, []*Violation{assetViolation(entry.Index, "targets",
				fmt.Sprintf(
					"%s has no target harness: it declares no %q and the manifest declares no %q",
					assetSubject(entry.Index), "targets", "harnesses",
				),
				fmt.Sprintf(
					"Add a %q list to %s naming at least one of %s, or declare %q on this entry.",
					"harnesses", FileName, harnessList(), "targets",
				),
			)}
		}
		return toHarnessIDs(harnesses), nil
	}

	if len(entry.Targets) == 0 {
		return nil, []*Violation{assetViolation(entry.Index, "targets",
			fmt.Sprintf(
				"%s declares an empty %q list, so it could never be installed anywhere",
				assetSubject(entry.Index), "targets",
			),
			fmt.Sprintf(
				"Name at least one of %s in the entry's %q, or remove the field so the asset inherits the manifest's %q.",
				harnessList(), "targets", "harnesses",
			),
		)}
	}

	var violations []*Violation
	targets := make([]harness.ID, 0, len(entry.Targets))
	for position, name := range entry.Targets {
		id := harness.ID(name)
		if !harness.Recognized(id) {
			violations = append(violations, assetViolation(entry.Index, fmt.Sprintf("targets[%d]", position),
				fmt.Sprintf(
					"%s targets %q, which is not a harness harnaas recognizes",
					assetSubject(entry.Index), name,
				),
				fmt.Sprintf("Change it in %s to a harness harnaas recognizes: %s.", FileName, harnessList()),
			))
			continue
		}
		targets = append(targets, id)
	}
	if len(violations) > 0 {
		return nil, violations
	}

	return targets, nil
}

// CheckHarnesses reports every name in the manifest's `harnesses` list the
// roster does not recognize.
//
// An empty list is not reported here. It is only a problem for an asset that
// would have inherited it, and an asset declaring its own `targets` is
// perfectly well served by a manifest that declares no default — so the
// violation belongs to the entry that has nowhere to go, not to the document.
func CheckHarnesses(harnesses []string) []*Violation {
	var violations []*Violation
	for position, name := range harnesses {
		if harness.Recognized(harness.ID(name)) {
			continue
		}

		violations = append(violations, &Violation{
			Index: DocumentIndex,
			Field: fmt.Sprintf("harnesses[%d]", position),
			Problem: fmt.Sprintf(
				"the %q list names %q, which is not a harness harnaas recognizes",
				"harnesses", name,
			),
			Fix: fmt.Sprintf("Change it in %s to a harness harnaas recognizes: %s.", FileName, harnessList()),
		})
	}
	return violations
}

// ResolveScope returns where the entry installs: the scope it declares, or
// `project`.
func ResolveScope(entry AssetEntry, assetType AssetType, targets []harness.ID) (Scope, *Violation) {
	return resolveScope(entry, assetType, targets, harness.HasPerUserLocation)
}

// resolveScope is [ResolveScope] with the roster query as a parameter.
//
// The seam exists because the roster is data this package cannot vary: today
// every recognized harness has a per-user location, so the rule that `user`
// scope is refused where one does not exist would have no test at all, and
// would be free to rot until the first harness that lacks one is added — which
// is exactly when a silent fall back to project scope would start installing
// assets somewhere nobody asked for.
func resolveScope(
	entry AssetEntry,
	assetType AssetType,
	targets []harness.ID,
	perUserLocation func(harness.ID) (bool, error),
) (Scope, *Violation) {
	if entry.Scope == "" {
		return ScopeProject, nil
	}

	declared := Scope(entry.Scope)
	if !slices.Contains(scopes, declared) {
		return "", assetViolation(entry.Index, "scope",
			fmt.Sprintf("%s declares the scope %q, which harnaas does not define", assetSubject(entry.Index), entry.Scope),
			fmt.Sprintf(
				"Set %q on the entry in %s to one of %s, or remove it to install at %q scope.",
				"scope", FileName, scopeList(), ScopeProject,
			),
		)
	}
	if declared == ScopeProject {
		return ScopeProject, nil
	}

	// An instruction is guidance concatenated into a committed memory file, and
	// what distinguishes it from a rule is surviving a fresh clone inside that
	// file. At user scope there is no clone and no committed file, so the type
	// would mean nothing — this is definitional, not a limitation of the
	// installer.
	if assetType == AssetTypeInstruction {
		return "", assetViolation(entry.Index, "scope",
			fmt.Sprintf(
				"%s is an %s at %q scope, and an %s exists only inside the project's committed memory file",
				assetSubject(entry.Index), AssetTypeInstruction, ScopeUser, AssetTypeInstruction,
			),
			fmt.Sprintf(
				"Remove %q from the entry in %s, or declare its %q as %q, which is the type for always-on guidance outside a committed file.",
				"scope", FileName, "type", AssetTypeRule,
			),
		)
	}

	var unsupported []string
	for _, target := range targets {
		hasLocation, err := perUserLocation(target)
		if err != nil {
			// The target is not in the roster, which [ResolveTargets] or
			// [CheckHarnesses] has already reported against the field that
			// declared it. Reporting it a second time as a scope problem would
			// name an edit that does not fix it.
			continue
		}
		if !hasLocation {
			unsupported = append(unsupported, quoted(string(target)))
		}
	}

	if len(unsupported) > 0 {
		return "", assetViolation(entry.Index, "scope",
			fmt.Sprintf(
				"%s declares %q scope, and %s no unambiguous per-user location",
				assetSubject(entry.Index), ScopeUser, hasNo(unsupported),
			),
			fmt.Sprintf(
				"Remove %q from the entry in %s so it installs at %q scope, or narrow the entry's %q to harnesses with a per-user location.",
				"scope", FileName, ScopeProject, "targets",
			),
		)
	}

	return ScopeUser, nil
}

// toHarnessIDs converts a declared name list into roster ids. It is only ever
// called with names already checked against the roster, or with the manifest's
// `harnesses` list, whose names [CheckHarnesses] reports on separately.
func toHarnessIDs(names []string) []harness.ID {
	ids := make([]harness.ID, 0, len(names))
	for _, name := range names {
		ids = append(ids, harness.ID(name))
	}
	return ids
}

// hasNo renders the offending targets as the subject of a sentence, so one
// target reads as naturally as several.
func hasNo(targets []string) string {
	if len(targets) == 1 {
		return "the target " + targets[0] + " has"
	}
	return "the targets " + strings.Join(targets, ", ") + " have"
}

// typeList renders every asset type for a diagnostic, in the fixed order
// [AssetTypes] gives.
func typeList() string {
	names := make([]string, 0, len(AssetTypes()))
	for _, assetType := range AssetTypes() {
		names = append(names, quoted(string(assetType)))
	}
	return strings.Join(names, ", ")
}

// scopeList renders every accepted scope for a diagnostic.
func scopeList() string {
	names := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		names = append(names, quoted(string(scope)))
	}
	return strings.Join(names, ", ")
}

// harnessList renders every recognized harness id for a diagnostic, in the
// roster's own order so two runs produce the same sentence.
func harnessList() string {
	ids := harness.IDs()
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, quoted(string(id)))
	}
	return strings.Join(names, ", ")
}
