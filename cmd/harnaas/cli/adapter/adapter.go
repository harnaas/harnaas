// Package adapter is where an asset lands on one named harness.
//
// Most harnesses need no adapter at all. Of the harnesses surveyed for this
// design, 17 read `.agents/skills/<id>/SKILL.md` and 21 read `AGENTS.md`, so a
// skill and an instruction reach them through shared locations that no
// per-harness code computes. What has no shared equivalent anywhere is a rule, a
// command and a persona, and those reach a harness only through an adapter that
// knows its directories. There are two, `claude-code` and `devin-cli`, and "a
// harness with no adapter" is a supported state rather than a gap.
//
// An adapter answering for fewer than all three types is equally supported, and
// the second one is the case: `devin-cli` has no commands directory, so it maps a
// rule and a persona and reports a command as having no destination here.
//
// This package is the contract and the registry; each adapter is its own package
// beneath it. The split is the extraction rule doing its work rather than a layer
// for its own sake: the install flow lives in the flat cli package and has to
// import the adapters, so an adapter importing a contract that lived beside the
// flow would be an import cycle one file later. Keeping the contract here is also
// what makes the import boundary checkable — an adapter may reach this package
// and general-purpose utilities, and nothing else.
//
// An adapter answers questions and performs no I/O of its own. It is handed a
// filesystem to detect through and it returns paths for the caller to write, for
// the same reason the harness roster holds no behaviour: two things that can each
// compute a destination eventually compute two different ones.
package adapter

import (
	"io/fs"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// Adapter maps assets onto the files one harness reads.
//
// The contract is uniform on purpose: the install flow obtains a destination
// without knowing which harness it is addressing, so adding the second adapter
// is additive rather than another branch in the flow.
type Adapter interface {
	// Harness is the identity this adapter registers under, and the exact
	// string a manifest writes in `harnesses` or in an asset's `targets`.
	Harness() harness.ID

	// Detect reports whether the project already shows evidence of this
	// harness — its configuration directory existing, or a file only it
	// writes.
	//
	// The parameter is a filesystem rather than a directory path because
	// detection must create nothing: an fs.FS offers no way to write, so
	// "detection has no side effects" is a property of the signature rather
	// than a rule each adapter has to honour. A harness that is simply not
	// installed is an absence and never a failure — there is nothing for a
	// caller to do about it and nothing to report.
	Detect(project fs.FS) bool

	// Root returns the harness's own directory for one scope, relative to that
	// scope's root: the project root at `project` scope, and the user's home
	// directory at `user` scope. The second result is false where the harness
	// has no unambiguous location for that scope, which is the whole of the
	// adapter's answer to whether an asset may declare it.
	//
	// It is relative, and joined by the caller, because an adapter that read
	// the environment for a home directory would be a second place in harnaas
	// deciding where a user's files live.
	Root(scope manifest.Scope) (string, bool)

	// Destination returns where one asset installs on this harness, relative to
	// the resolved scope root — the harness's own directory for the asset's
	// scope, which is [Adapter.Root] joined to the project root or to the
	// user's home. A rule resolves to `rules/house-style.md` there, never to a
	// path counted from the project root, so the same mapping serves both
	// scopes and the lockfile can record a destination that means the same
	// thing on another machine.
	//
	// The second result is false where this harness has no surface for the
	// asset's type. That absence is a value rather than an invented path
	// precisely because a guessed destination is indistinguishable from a real
	// one once it is written: the caller reports the pairing as unsupported and
	// installs the asset's other targets.
	//
	// The asset travels whole rather than as the type, id and scope a mapping
	// reads, for the reason a source request does: the fields a diagnostic needs
	// are not the fields the mapping needs, and narrowing the parameter would
	// make every caller's messages worse to keep a signature shorter.
	Destination(asset manifest.Asset) (Destination, bool)
}

// Destination is where one asset installs on one harness, together with the
// terms it installs on.
//
// The terms travel with the path because they are decided by the same lookup:
// which surface a type maps to is what settles how current that surface is and
// what has to happen to the bytes on their way into it. Splitting them would let
// a caller obtain a path and write it without ever asking whether the harness
// still reads there.
type Destination struct {
	// Path is the installed file relative to the resolved scope root,
	// slash-separated. Whether it stays inside that root is not this value's
	// promise: the write goes through a handle anchored there, so an asset id
	// that would escape is refused by the kernel at the moment of the write
	// rather than by a comparison at the moment it was computed.
	Path string

	// Tier is how current the surface is, which decides whether harnaas writes
	// here at all.
	Tier Tier

	// Replacement names the surface that superseded a removed one, and is set
	// only where Tier is [TierRemoved]. It is what makes the refusal actionable:
	// the reader is being told the harness stopped reading this path, and the
	// only useful next sentence is where it reads instead.
	Replacement string

	// Note is the sentence the install report carries for a gated or a legacy
	// surface — the setup the user still has to perform, or the deprecation
	// they should plan for. A live surface carries none, because a note nobody
	// has to act on teaches a reader to skip them all.
	Note string

	// Renderer names the transformation from the source's bytes to the ones
	// that land here. It is a name rather than a function because an adapter
	// declares what a surface needs while the rendering layer owns how it is
	// done: an adapter carrying an implementation would import the layer the
	// import boundary exists to keep out.
	Renderer Renderer
}

// Tier is how current a harness surface is, which decides whether harnaas writes
// to it.
//
// It exists because a harness that has moved on from a path does not say so: the
// write succeeds, the file appears, and nothing ever reads it. That is the exact
// outcome a tool whose purpose is telling a team what is in effect must not
// produce, so the state of a surface is data harnaas carries rather than
// something a user discovers.
type Tier string

const (
	// TierLive is a surface the harness reads today. It is written with no
	// note, which is what keeps the notes on the other tiers worth reading.
	TierLive Tier = "live"

	// TierGated is a surface the harness reads only once the user has enabled
	// something. It is written, with the setup note carried into the install
	// report, because the file is correct and the remaining step is theirs.
	TierGated Tier = "gated"

	// TierLegacy is a surface the harness still reads but has deprecated. It is
	// written, with the deprecation note carried into the report, so a team
	// learns before the surface becomes removed rather than after.
	TierLegacy Tier = "legacy"

	// TierRemoved is a surface the harness no longer reads. Writing there is a
	// guaranteed silent no-op, so it is refused rather than written, naming the
	// surface that replaced it.
	TierRemoved Tier = "removed"
)

// Renderer names the transformation between a source's bytes and the bytes that
// land at a destination.
//
// A name rather than a function keeps the adapter layer and the rendering layer
// apart, and it is also what makes a surface able to declare a renderer nobody
// has written yet: selecting one with no implementation reports the pairing
// unsupported and names the renderer, which is a truthful outcome, where falling
// back to copying the bytes would write a file the harness cannot read.
type Renderer string

const (
	// RendererIdentity reproduces the source bytes exactly, including
	// frontmatter, line endings and trailing whitespace. It is what almost
	// every surface declares, and it is deliberately a renderer rather than the
	// absence of one: copying is the default, not the rule.
	RendererIdentity Renderer = "identity"

	// RendererAsSkill delivers an asset through a harness's skill surface when
	// its own surface is absent — a command as a SKILL.md the model is told not
	// to invoke on its own initiative. It is an emulation, and is reported as
	// one rather than as plain success.
	RendererAsSkill Renderer = "as-skill"

	// RendererAsInstruction delivers a rule through the memory file's managed
	// block on a harness with no rules directory. It is an emulation, and it is
	// only ever selected for a rule that declares no path scoping: a scoped
	// rule delivered this way would silently become always-on, which is a
	// change of meaning rather than of delivery.
	RendererAsInstruction Renderer = "as-instruction"

	// RendererMDC converts an asset into Cursor's `.mdc` rule format.
	//
	// It is named and deliberately not implemented in v1. Naming it is the
	// point: a surface added later may declare it before anybody writes it, and
	// the pairing is then reported unsupported and names this renderer, rather
	// than falling back to copying bytes into a file the harness cannot read.
	// This is the constant that keeps that path exercised while v1 ships one
	// adapter whose every surface renders identically.
	RendererMDC Renderer = "mdc"
)

// Renderers returns every renderer the contract names, implemented or not, in a
// fixed order.
//
// The set is declared here rather than derived from what the rendering layer
// happens to implement, because the two answer different questions: this is
// what an adapter may declare, and the rendering layer decides what harnaas can
// actually produce. A name missing from here is a surface no adapter can point
// at; a name here with no implementation is the supported "unsupported" case.
func Renderers() []Renderer {
	return []Renderer{RendererIdentity, RendererAsSkill, RendererAsInstruction, RendererMDC}
}
