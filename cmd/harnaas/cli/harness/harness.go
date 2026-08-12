// Package harness is harnaas's roster of the harnesses it recognizes.
//
// The roster is data only: an id, a display name, whether the harness has an
// unambiguous per-user location, and the observable evidence that a project is
// already using it. It maps nothing to a destination and it writes nothing. The
// adapters that turn an asset into a file on disk arrive with `harnaas install`
// and attach themselves to these ids.
//
// That seam is deliberate. Three things in this change need to know a harness
// exists — manifest validation must reject an unrecognized target, scope
// validation must know whether `user` scope has anywhere to go, and `init` must
// detect which harnesses a project already uses — while none of them needs to
// know where a skill lands. Deferring the roster until the adapters exist would
// force the manifest to accept any string as a harness name and reject it two
// phases later, at a point where the error can no longer name the line that
// caused it.
//
// A harness in the manifest's `harnesses` list is a guarantee, not an
// observation: it means "we guarantee the declared assets work for this
// harness". Because skills and instructions land in shared locations that
// several harnesses read, a harness absent from the list will still see assets
// installed for one that is present. That is why an unrecognized name is an
// error — it is a guarantee harnaas cannot make — and why an asset being
// visible to a harness nobody listed is not.
package harness

// ID is a harness's canonical identifier: the exact string a manifest writes in
// its `harnesses` list or an asset's `targets`, and the key an adapter will
// later register itself under.
//
// It is a named type rather than a bare string so a function taking a harness
// cannot silently be handed an asset id, a source key or a display name.
type ID string

// ClaudeCode and DevinCLI are the harnesses harnaas recognizes. More arrive the
// same way; the roster is where they are declared, one entry each.
const ClaudeCode ID = "claude-code"

// DevinCLI is Cognition's terminal agent, and the id is deliberately not
// `devin`.
//
// Cognition ships an agent under the Devin name whose playbooks, knowledge and
// secrets are not files in anybody's repository, and which the terminal agent's
// own documentation says it does not read. A `harnesses` list states a
// guarantee, so the id has to name the thing the guarantee is about — and the
// broader spelling would claim harnaas manages something it cannot see.
const DevinCLI ID = "devin-cli"

// Default is the harness `harnaas init` scaffolds a manifest with when it
// detects none in the project. A default is what keeps init from ever writing
// an empty `harnesses` list, which would be a manifest that declares assets and
// guarantees them for nothing.
const Default = ClaudeCode

// Harness describes one recognized harness. Every field is data a caller reads;
// there is no behaviour here, and adding some is how the roster and the
// adapters start to disagree.
type Harness struct {
	// ID is the canonical identifier, and the only spelling a manifest may
	// use. Display names are never accepted as input.
	ID ID

	// DisplayName is what harnaas prints when it names the harness to a user.
	// It is never parsed, and never compared against manifest input.
	DisplayName string

	// PerUserLocation records whether the harness reads assets from an
	// unambiguous location in the user's home as well as from the project.
	//
	// This is what decides whether an asset may declare `user` scope for this
	// target. It is a property of the harness, not a preference: where a
	// harness has no single per-user root, or several that disagree, harnaas
	// has no correct answer and says so rather than guessing one.
	PerUserLocation bool

	// ProjectEvidence lists the project-root-relative paths whose presence
	// means the project is already using this harness. Any one of them is
	// enough.
	//
	// Evidence is observable state in the repository, never configuration the
	// user has not written yet: `init` runs before harnaas knows anything about
	// the project, so the only thing it can read is what the harness itself has
	// already left behind. The entries are paths rather than a predicate
	// because the roster stays data — `init` does the stat calls.
	ProjectEvidence []string
}

// roster is every recognized harness, ordered by ID.
//
// The order is the one callers see, so it has to be a property of the data
// rather than of how the literal happens to be written — a manifest error
// listing recognized harnesses must read identically on every run and every
// machine. TestRosterIsOrderedByID pins it, so an entry added in the wrong
// place fails a test instead of quietly reordering somebody's error message.
var roster = []Harness{
	{
		ID:              ClaudeCode,
		DisplayName:     "Claude Code",
		PerUserLocation: true,
		ProjectEvidence: []string{".claude", "CLAUDE.md"},
	},
	{
		ID:          DevinCLI,
		DisplayName: "Devin CLI",

		// True for the question this field asks, which is whether an asset may
		// declare `user` scope for this target at all. A skill installed at
		// user scope reaches this harness through the shared per-user skills
		// directory, which it documents reading — so recording `false` would
		// refuse something that demonstrably works.
		//
		// Its rules and its remaining per-user content sit under two different
		// home directories, and the second is spelled differently per platform,
		// so there is no single root a *destination* could be counted from. That
		// is the adapter's question rather than this one, and the adapter
		// answers it by offering no per-user root, which is why a user-scoped
		// rule for this harness is refused there rather than here.
		PerUserLocation: true,

		// The configuration directory alone. The memory file is read by 21 of
		// the 23 harnesses surveyed for ADR 0003, so treating it as evidence
		// would report this harness as detected in nearly every project that
		// has ever written one, and `init` would scaffold a guarantee nobody
		// asked for.
		ProjectEvidence: []string{".devin"},
	},
}
