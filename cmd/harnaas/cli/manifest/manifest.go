// Package manifest decodes `harnaas.json`, the committed, hand-edited file in
// which a project declares which harness assets it wants and where they come
// from.
//
// The package covers the manifest end to end in two layers that meet at
// [Document]. Decoding turns bytes into that document and interprets none of
// it: a field it accepts is one it could read, not one harnaas has agreed to.
// Interpretation then reads the document — parsing a source value, parsing an
// asset's source string, inferring a type and an id from a path, defaulting
// targets and scope, and checking the result against the harness roster.
//
// The two share one package because harnaas extracts a package only to break an
// import cycle, and there is none to break here: both layers are imported by
// the same callers, and splitting them would put the roster on one side of a
// boundary that exists for no other reason. What separates them instead is when
// they stop. Decoding stops at the first failure, because a document that will
// not parse has no second problem to find. Interpretation reports every
// violation at once as a [Violation] list, because every asset entry is
// independent and fixing three mistakes should not take three runs.
//
// # Strict decoding
//
// The manifest is written by a person and reviewed by a team, so an unknown
// field is nearly always a typo and surfacing it immediately is worth the cost
// of refusing to run. That is the whole test: strictness follows who writes the
// file, not what the file is called. `harnaas.lock.json` is machine-written and
// therefore decodes leniently, because a newer binary introduces fields an
// older binary would otherwise reject, bricking the file for everyone still on
// the older version.
//
// The version field is what bounds that trade. It is read on its own, ahead of
// the strict pass, so a manifest written by a newer harnaas is refused with an
// explicit "upgrade harnaas" message rather than a confusing complaint about
// the first field this binary happens not to know.
//
// A decode failure returns no document at all. Half a manifest is not a smaller
// manifest — it is a different one, and installing from it would place a subset
// of what the project declared while reporting success.
//
// # Read-only
//
// Nothing here writes. Apart from `harnaas init` creating the file once, harnaas
// never writes, reformats or normalizes `harnaas.json`: it is the artifact a
// team reviews, and a tool that rewrites it makes its diffs untrustworthy. Every
// remedy this package reports is therefore phrased as the exact edit for a
// person to make, never as a command that would make it for them. A test over
// this package's own sources asserts no write call has crept in.
package manifest

// FileName is the manifest's only accepted name, at the repository root.
//
// It is a constant rather than a flag because the file is a project-level
// declaration: a `--manifest` pointing somewhere else would mean two developers
// in the same repository could install different asset sets and both be right.
const FileName = "harnaas.json"

// SupportedVersion is the manifest schema version this binary understands.
//
// A manifest declaring a greater version is refused with an upgrade message
// rather than read partially, because the fields this binary would silently
// ignore are exactly the ones the newer harnaas added to change what the
// document means.
const SupportedVersion = 1

// Document is a decoded manifest, field for field as written.
//
// Nothing in it has been interpreted: `Harnesses` and each entry's `Targets`
// are the strings the author typed rather than roster ids, and an asset's
// `Type`, `ID` and `Scope` are populated only where the object form declared
// them. An empty field here means "not declared", never "declared as the
// default" — the interpretation layer is what turns the first into the second,
// and it can only do that if decoding kept them distinguishable.
type Document struct {
	// Path is the file the document was decoded from, so a later diagnostic can
	// name it. It is empty for a document decoded from bytes.
	Path string

	// Version is the declared schema version, always a version this binary
	// understands — a greater one fails decoding rather than reaching a caller.
	Version int

	// Harnesses names the harnesses assets target by default. The list states a
	// guarantee ("we guarantee the declared assets work for these"), not
	// exclusivity: assets installed for one harness are routinely visible to
	// another.
	Harnesses []string

	// Sources maps a short key to a source string such as
	// `github:acme/assets@v1.2.0`. The key is what asset entries reference, so a
	// repository and a ref appear exactly once each.
	Sources map[string]string

	// Assets is the declared asset list in document order. Order is preserved
	// because it is the order the author reads in review, and every diagnostic
	// naming an entry names its index in this slice.
	Assets []AssetEntry
}

// AssetEntry is one entry of the `assets` array, in whichever form it was
// written.
//
// The string form carries only a source; the object form may additionally
// declare a type, an id, a target list or a scope, each suppressing what would
// otherwise be inferred or defaulted for that one field.
type AssetEntry struct {
	// Index is the entry's position in the `assets` array. It is not decoded
	// from the document — it is where the entry was found, and it is how every
	// diagnostic points the author at the line to edit, since an entry has no
	// other stable name until its id is known.
	Index int

	// ObjectForm records whether the entry was written as an object rather than
	// a bare source string. Some remedies are only available in the object form,
	// so a message can direct an author there only if it knows they are not
	// already using it.
	ObjectForm bool

	// Source is the entry's source string: the `<sourceKey>:<path>` or
	// `.harnaas/…` form, exactly as written and not yet parsed.
	Source string

	// Type overrides the type the path would imply. Empty means undeclared.
	Type string

	// ID overrides the id the path's leaf would imply. Empty means undeclared.
	ID string

	// Targets overrides the manifest's `harnesses` list for this asset. A nil
	// slice means undeclared; an empty non-nil slice means the author declared
	// an empty list, which is a validation error rather than a default, because
	// the asset could never be installed anywhere.
	Targets []string

	// Scope overrides the default `project` scope. Empty means undeclared.
	Scope string
}
