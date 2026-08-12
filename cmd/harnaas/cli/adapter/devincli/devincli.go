// Package devincli maps assets onto the files Devin CLI reads.
//
// It exists for the two asset types this harness has a surface of its own for: a
// rule and a persona. A skill and an instruction reach Devin CLI the way they
// reach most harnesses — through `.agents/skills/` and the project's memory file,
// both of which it documents reading — so they are deliberately absent from the
// mapping here rather than mapped a second time into `.devin/`. A command is
// absent for a different reason: this harness has no command surface at all,
// because what a user invokes by name there is a skill, and the install flow
// reports that pairing rather than inventing a path for it.
//
// Everything this package knows is a question the contract asks: which harness it
// registers under, whether the project already uses it, its own directory per
// scope, and the destination for one asset. It performs no I/O, holds no state
// and reads no environment, so linking it changes what harnaas can install and
// nothing else.
package devincli

import (
	"io/fs"
	"path"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// init registers this adapter with [adapter.Default], so a binary that links this
// package is a binary that can install a rule or a persona for Devin CLI.
// Registration lives here rather than in the install flow for the reason a source
// kind's does: a list maintained elsewhere is a list a new adapter has to remember
// to join, and the failure that produces is an adapter which compiles, passes its
// own tests and is missing from the registry.
//
//nolint:gochecknoinits // Self-registration is the adapter contract; see [adapter.Registry].
func init() {
	adapter.Register(New())
}

// directory is Devin CLI's own directory beneath the project root.
//
// Unlike Claude Code's, it is not also the answer at user scope. This harness
// keeps its per-user rules under one home directory and its per-user skills,
// personas and memory file under another, and spells the second differently per
// operating system — so [Adapter.Root] offers this one for `project` and offers
// nothing for anything else.
const directory = ".devin"

// extension is the file extension every Devin CLI surface takes. The asset id is
// the stem, which is what makes a destination predictable from the manifest
// alone: an author reading `rules/house-style.md` in the report can find the
// entry that produced it without consulting the lockfile.
const extension = ".md"

// surfaces is the whole of Devin CLI's mapping: the directory beneath the
// resolved scope root for each asset type this harness has a surface for,
// together with the terms that surface installs on.
//
// A type absent from this table is a type harnaas will not write for this
// harness, and the three absences here are three different facts. A skill and an
// instruction install through shared locations, so answering for them would be a
// second place harnaas decides where they land. A command has no surface to
// answer with: this harness's slash commands are its skills, and delivering one
// through that surface is refused upstream because there is no setting harnaas
// can write to stop the harness starting it.
//
// The persona surface takes the flat spelling rather than the directory form the
// harness also reads. Where a harness offers two, harnaas picks the one the
// report can name usefully: a path to a file is something a reader can open, and
// a path to a directory is something they have to go looking inside. It also
// keeps every single-file asset the same shape — one file whose stem is the asset
// id — rather than making a persona the exception.
//
// Every surface is live today. The tier travels anyway, because a harness that
// stops reading a path does not announce it.
var surfaces = map[manifest.AssetType]adapter.Destination{
	manifest.AssetTypeRule: {
		Path:     "rules",
		Tier:     adapter.TierLive,
		Renderer: adapter.RendererIdentity,
	},
	manifest.AssetTypePersona: {
		Path:     "agents",
		Tier:     adapter.TierLive,
		Renderer: adapter.RendererIdentity,
	},
}

// Adapter installs assets for Devin CLI.
//
// It holds nothing. An adapter is a mapping from an asset to a path, and this one
// is registered as a value for that reason: there is no per-run state to give each
// run its own copy of.
type Adapter struct{}

var _ adapter.Adapter = Adapter{}

// New returns the devin-cli adapter.
func New() Adapter { return Adapter{} }

// Harness is the identity this adapter registers under, and the exact string a
// manifest writes.
func (Adapter) Harness() harness.ID { return harness.DevinCLI }

// Detect reports whether the project already shows evidence of Devin CLI.
//
// The evidence is the roster's rather than a second list kept here. `init`
// detects harnesses to scaffold a manifest and this adapter detects one to report
// on an install, and the two answering differently for one project is exactly the
// drift that keeps the roster behaviourless.
func (Adapter) Detect(project fs.FS) bool {
	h, err := harness.Lookup(harness.DevinCLI)
	if err != nil {
		// Unreachable from the shipped roster, which is what registration is
		// keyed on. A roster that had lost this harness would leave harnaas
		// unable to accept `devin-cli` in a manifest at all, so reporting
		// absence is the only answer detection could usefully give.
		return false
	}

	return detect(project, h.ProjectEvidence)
}

// detect reports whether any one of the evidence paths is present.
//
// The check is an Lstat rather than a Stat, matching init's rule for the same
// reason — the question is whether the harness left something behind, not whether
// it resolves. A `.devin` symbolic link into a directory that is not checked out
// is still evidence, and following it would answer "no" for a project that
// plainly says yes.
func detect(project fs.FS, evidence []string) bool {
	for _, name := range evidence {
		if _, err := fs.Lstat(project, name); err == nil {
			return true
		}
	}
	return false
}

// Root returns Devin CLI's own directory for one scope, relative to that scope's
// root.
//
// Only `project` is offered, and that is this harness's own property rather than
// a simplification. Its per-user rules sit under one home directory and its
// per-user skills, personas and memory file under another, which is spelled
// differently per operating system — so there is no single root a destination
// could be counted from, and a lockfile recording one would hold a path that
// means something different on the next machine.
//
// Refusing here is what makes a user-scoped rule or persona for this harness an
// unsupported pairing naming the asset, the harness and the scope, rather than a
// file installed somewhere its author has no reason to look. A user-scoped skill
// is unaffected: it reaches this harness through the shared per-user skills
// directory, which no adapter computes.
func (Adapter) Root(scope manifest.Scope) (string, bool) {
	if scope == manifest.ScopeProject {
		return directory, true
	}
	return "", false
}

// Destination returns where one asset installs on Devin CLI, relative to the
// resolved scope root: a rule at `rules/<id>.md` and a persona at
// `agents/<id>.md`.
//
// The scope does not enter the mapping. It selects the root the path is joined
// beneath, and whether the harness has a root for it is [Adapter.Root]'s answer,
// asked once by [adapter.ResolveRoot] before any destination is wanted.
//
// A rule installs as a standalone file the harness discovers on its own. Nothing
// references it from the memory file or from any managed block, which is a
// property of this mapping being the whole of what a rule needs.
func (Adapter) Destination(asset manifest.Asset) (adapter.Destination, bool) {
	surface, supported := surfaces[asset.Type]
	if !supported {
		return adapter.Destination{}, false
	}

	surface.Path = path.Join(surface.Path, asset.ID+extension)
	return surface, true
}
