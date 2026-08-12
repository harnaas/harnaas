// Package claudecode maps assets onto the files Claude Code reads.
//
// It exists for the three asset types that have no shared equivalent anywhere:
// a rule, a command and a persona. It is the only adapter mapping all three —
// devin-cli has no command surface at all, which is refused rather than
// emulated; see ADR 0005. A
// skill and an instruction reach Claude Code the way they reach every other
// harness — through `.agents/skills/` and the project's memory file — so they
// are deliberately absent from the mapping here rather than mapped a second
// time into `.claude/`. An adapter that answered for them would be a second
// place harnaas decides where a skill lands, and the two answers would be one
// release from disagreeing.
//
// Everything this package knows is a question the contract asks: which harness
// it registers under, whether the project already uses it, its own directory per
// scope, and the destination for one asset. It performs no I/O, holds no state
// and reads no environment, so linking it changes what harnaas can install and
// nothing else.
package claudecode

import (
	"io/fs"
	"path"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// init registers this adapter with [adapter.Default], so a binary that links
// this package is a binary that can install a rule, a command or a persona for
// Claude Code. Registration lives here rather than in the install flow for the
// reason a source kind's does: a list maintained elsewhere is a list a new
// adapter has to remember to join, and the failure that produces is an adapter
// which compiles, passes its own tests and is missing from the registry.
//
//nolint:gochecknoinits // Self-registration is the adapter contract; see [adapter.Registry].
func init() {
	adapter.Register(New())
}

// directory is Claude Code's own directory, relative to the root of whichever
// scope an asset installs at: `<project>/.claude` and `<home>/.claude`.
//
// That both scopes spell it the same way is a fact about this harness and not a
// rule — the contract returns a root per scope precisely because another harness
// will not — so [Adapter.Root] answers per scope rather than returning one
// constant to both callers.
const directory = ".claude"

// extension is the file extension every Claude Code surface takes. The asset id
// is the stem, which is what makes a destination predictable from the manifest
// alone: an author reading `rules/house-style.md` in the report can find the
// entry that produced it without consulting the lockfile.
const extension = ".md"

// surfaces is the whole of Claude Code's mapping: the directory beneath the
// resolved scope root for each asset type this harness has a surface for,
// together with the terms that surface installs on.
//
// A type absent from this table is a type Claude Code has no surface for, which
// is the answer [Adapter.Destination] returns rather than an invented path. Both
// absences here are deliberate and neither is a gap: a skill and an instruction
// install through shared locations, and the install flow consults a named
// adapter only for the types that have none.
//
// Every surface is live today. The tier travels anyway, because a harness that
// stops reading a path does not announce it — the day one of these becomes
// legacy or removed, the edit is one field in this table and the install report
// changes with it.
var surfaces = map[manifest.AssetType]adapter.Destination{
	manifest.AssetTypeRule: {
		Path:     "rules",
		Tier:     adapter.TierLive,
		Renderer: adapter.RendererIdentity,
	},
	manifest.AssetTypeCommand: {
		Path:     "commands",
		Tier:     adapter.TierLive,
		Renderer: adapter.RendererIdentity,
	},
	manifest.AssetTypePersona: {
		Path:     "agents",
		Tier:     adapter.TierLive,
		Renderer: adapter.RendererIdentity,
	},
}

// Adapter installs assets for Claude Code.
//
// It holds nothing. An adapter is a mapping from an asset to a path, and this
// one is registered as a value for that reason: there is no per-run state to
// give each run its own copy of.
type Adapter struct{}

var _ adapter.Adapter = Adapter{}

// New returns the claude-code adapter.
func New() Adapter { return Adapter{} }

// Harness is the identity this adapter registers under, and the exact string a
// manifest writes.
func (Adapter) Harness() harness.ID { return harness.ClaudeCode }

// Detect reports whether the project already shows evidence of Claude Code.
//
// The evidence is the roster's rather than a second list kept here. `init`
// detects harnesses to scaffold a manifest and this adapter detects one to
// report on an install, and the two answering differently for one project is
// exactly the drift that keeps the roster behaviourless — so there is one
// declaration of what counts as evidence, and both readers read it.
func (Adapter) Detect(project fs.FS) bool {
	h, err := harness.Lookup(harness.ClaudeCode)
	if err != nil {
		// Unreachable from the shipped roster, which is what registration is
		// keyed on. A roster that had lost this harness would leave harnaas
		// unable to accept `claude-code` in a manifest at all, so reporting
		// absence is the only answer detection could usefully give.
		return false
	}

	return detect(project, h.ProjectEvidence)
}

// detect reports whether any one of the evidence paths is present. Any single
// piece is enough: a project with a `CLAUDE.md` and no `.claude/` is still a
// project using Claude Code.
//
// The check is an Lstat rather than a Stat, matching init's rule for the same
// reason — the question is whether the harness left something behind, not
// whether it resolves. A `.claude` symbolic link into a directory that is not
// checked out is still evidence, and following it would answer "no" for a
// project that plainly says yes.
func detect(project fs.FS, evidence []string) bool {
	for _, name := range evidence {
		if _, err := fs.Lstat(project, name); err == nil {
			return true
		}
	}
	return false
}

// Root returns Claude Code's own directory for one scope, relative to that
// scope's root.
//
// Both scopes are offered, and that is the harness's own property rather than a
// convenience: Claude Code reads `<home>/.claude` unambiguously, which is what
// lets an asset declare `user` scope for this target at all. A harness whose
// per-user location were ambiguous would return false here, and
// [adapter.ResolveRoot] would refuse the asset rather than quietly installing it
// at project scope.
func (Adapter) Root(scope manifest.Scope) (string, bool) {
	switch scope {
	case manifest.ScopeProject, manifest.ScopeUser:
		return directory, true
	default:
		return "", false
	}
}

// Destination returns where one asset installs on Claude Code, relative to the
// resolved scope root: a rule at `rules/<id>.md`, a command at
// `commands/<id>.md` and a persona at `agents/<id>.md`.
//
// The scope does not enter the mapping. It selects the root the path is joined
// beneath, and whether the harness has a root for it is [Adapter.Root]'s answer,
// asked once by [adapter.ResolveRoot] before any destination is wanted — so the
// same relative path serves both scopes and the lockfile records something that
// means the same thing on another machine.
//
// A rule installs as a standalone file the harness discovers on its own.
// Nothing references it from `CLAUDE.md`, from `AGENTS.md` or from any managed
// block, which is a property of this mapping being the whole of what a rule
// needs: adding a reference would put the same fact in two files and leave the
// second one to rot.
func (Adapter) Destination(asset manifest.Asset) (adapter.Destination, bool) {
	surface, supported := surfaces[asset.Type]
	if !supported {
		return adapter.Destination{}, false
	}

	surface.Path = path.Join(surface.Path, asset.ID+extension)
	return surface, true
}
