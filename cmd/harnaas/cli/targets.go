package cli

import (
	"fmt"
	"path"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/source"
)

// sharedSkillsDir is the directory 17 of 23 surveyed harnesses read skills
// from, and the reason most harnesses need no adapter at all.
//
// A skill installs here once and is visible to every harness that reads it,
// rather than being copied into one directory per declared harness — which is
// what ADR 0002 chose and what makes the `harnesses` list a guarantee rather
// than an exclusion.
const sharedSkillsDir = ".agents/skills"

// harnessesNotReadingSharedSkills is the whole of the fallback table: the
// harnesses known not to read the shared skills directory, which therefore need
// a copy in their own.
//
// It is empty in v1, and being empty is the finding rather than an omission —
// every harness on the roster reads `.agents/skills/`, which is what ADR 0002
// rests on. The table exists anyway, in exactly one place, because the survey
// behind that ADR will age: the day a harness is added that does not read the
// shared directory, this is the single edit, and a codebase that had encoded
// "everyone reads it" as an absence would need the rule discovered before it
// could be changed.
//
// A map rather than a slice so the lookup at the one call site reads as the
// question it asks.
var harnessesNotReadingSharedSkills = map[harness.ID]bool{}

// readsSharedSkills reports whether a harness finds a skill in the shared
// directory, which decides whether it also needs one of its own.
func readsSharedSkills(id harness.ID) bool {
	return !harnessesNotReadingSharedSkills[id]
}

// pathScopingKey is the frontmatter key by which a rule narrows what it applies
// to. Its presence is what makes a rule impossible to emulate as an
// instruction.
const pathScopingKey = "paths"

// targetPlan is where one asset lands for one harness, and on what terms.
//
// A plan is produced for every asset-and-target pairing, including the ones
// nothing will be written for: an unsupported pairing is an outcome the report
// has to carry, not a pairing to drop silently.
type targetPlan struct {
	// Harness is the target, and Scope the root Destination is relative to.
	Harness harness.ID
	Scope   manifest.Scope

	// Destination is relative to the scope root, slash-separated. It is empty
	// for an instruction, which lands inside the memory file's managed block
	// rather than at a path of its own.
	Destination string

	// Renderer is the transformation selected for this pairing, and Emulated
	// records that it reaches the harness through another type's surface.
	Renderer adapter.Renderer
	Emulated bool

	// Tier and Note carry the surface's terms into the report.
	Tier adapter.Tier
	Note string

	// Instruction marks a pairing that contributes to the memory file's block.
	Instruction bool

	// Unsupported is why nothing will be written, and is empty for a pairing
	// that will be. A reason rather than a bool, because every one of them
	// names a different next action for the reader.
	Unsupported string
}

// supported reports whether anything will be written for this pairing.
func (p targetPlan) supported() bool { return p.Unsupported == "" }

// declaresPathScoping reports whether a rule narrows itself to particular
// paths.
//
// It reads the frontmatter textually and decodes nothing, because the only
// question is whether the key is there — and a decoder would have to be handed
// a type, which would mean agreeing on a schema for a document harnaas
// deliberately never rewrites.
func declaresPathScoping(content []byte) bool {
	matter, found := source.SplitFrontmatter(content)
	if !found {
		return false
	}
	for _, line := range splitLinesKeepingEndings(matter.Raw) {
		if declaresKey(line, pathScopingKey) {
			return true
		}
	}
	return false
}

// planTarget decides where one asset lands for one harness.
//
// The order of the branches is the design's two-tier support: a skill and an
// instruction reach every harness through shared locations and never consult an
// adapter, and only the three types with no shared equivalent do. That is why
// "a harness with no adapter" is a supported state rather than a gap — it costs
// such a harness nothing for the two types most projects declare.
func planTarget(asset manifest.Asset, files []source.File, target harness.ID, registry *adapter.Registry) targetPlan {
	plan := targetPlan{Harness: target, Scope: asset.Scope, Renderer: adapter.RendererIdentity}
	if plan.Scope == "" {
		plan.Scope = manifest.ScopeProject
	}

	switch asset.Type {
	case manifest.AssetTypeSkill:
		if readsSharedSkills(target) {
			plan.Destination = path.Join(sharedSkillsDir, asset.ID)
			return plan
		}
		// Otherwise the skill needs a place in the harness's own directory,
		// which only a named adapter can say the location of, so the lookup
		// below runs as it does for the three per-harness types.

	case manifest.AssetTypeInstruction:
		// An instruction is inlined into the memory file's managed block, which
		// is one file for the whole project rather than one per asset, so it
		// has no destination of its own.
		plan.Instruction = true
		return plan

	case manifest.AssetTypeRule, manifest.AssetTypeCommand, manifest.AssetTypePersona:
	}

	adapted, err := registry.Lookup(target)
	if err != nil {
		plan.Unsupported = fmt.Sprintf(
			"%s has no harnaas adapter, and a %s reaches a harness only through one",
			target, asset.Type,
		)
		return plan
	}

	if destination, offered := adapted.Destination(asset); offered {
		return withSurface(plan, destination)
	}

	return emulate(plan, asset, files, adapted)
}

// withSurface fills a plan from the surface an adapter offered, refusing one the
// harness no longer reads.
//
// A removed surface is refused rather than written because writing there is a
// no-op that looks exactly like success — the file lands, the run is green, and
// the harness never opens it.
func withSurface(plan targetPlan, destination adapter.Destination) targetPlan {
	if destination.Tier == adapter.TierRemoved {
		plan.Unsupported = fmt.Sprintf(
			"%s no longer reads %s, so writing there would be a no-op that looks like success",
			plan.Harness, destination.Path,
		)
		if destination.Replacement != "" {
			plan.Unsupported += fmt.Sprintf(" (it reads %s instead)", destination.Replacement)
		}
		return plan
	}

	plan.Destination = destination.Path
	plan.Renderer = destination.Renderer
	plan.Tier = destination.Tier
	plan.Note = destination.Note
	return plan
}

// emulate decides whether an asset with no native surface can reach the harness
// through another type's, and refuses where doing so would change what the
// asset means.
//
// A native surface is always preferred, which is why this runs only after
// [adapter.Adapter.Destination] declined: emulation is the fallback, never a
// choice made while a real surface was available.
func emulate(plan targetPlan, asset manifest.Asset, files []source.File, adapted adapter.Adapter) targetPlan {
	switch asset.Type {
	case manifest.AssetTypeCommand:
		// A command delivered through the skill surface keeps its content and
		// loses only the harness's ability to start it unprompted, which the
		// renderer disables and the report states.
		if _, offered := adapted.Destination(skillAsset(asset)); !offered {
			plan.Unsupported = fmt.Sprintf("%s has no command surface and no skill surface to deliver one through", plan.Harness)
			return plan
		}
		plan.Destination = path.Join(sharedSkillsDir, asset.ID)
		plan.Renderer = adapter.RendererAsSkill
		plan.Emulated = true
		return plan

	case manifest.AssetTypeRule:
		if len(files) == 1 && declaresPathScoping(files[0].Content) {
			// Emulating this as an instruction would make a rule that applied
			// under some paths apply everywhere. That is a change of meaning
			// rather than of delivery, and it is invisible once written.
			plan.Unsupported = fmt.Sprintf(
				"%s has no rules directory, and this rule declares %q, "+
					"which the memory file cannot express: delivering it there would widen it from scoped to always-on",
				plan.Harness, pathScopingKey,
			)
			return plan
		}
		plan.Renderer = adapter.RendererAsInstruction
		plan.Emulated = true
		plan.Instruction = true
		return plan

	case manifest.AssetTypeSkill, manifest.AssetTypeInstruction, manifest.AssetTypePersona:
	}

	plan.Unsupported = fmt.Sprintf("%s has no surface for a %s and no way to deliver one without changing what it is",
		plan.Harness, asset.Type)
	return plan
}

// skillAsset is the asset as the skill surface would see it, used to ask an
// adapter whether it has one at all.
func skillAsset(asset manifest.Asset) manifest.Asset {
	asset.Type = manifest.AssetTypeSkill
	return asset
}

// declaredSource renders an asset reference the way the manifest spells it.
//
// The block in the memory file is committed, so a reader meeting text nobody on
// the team wrote needs the source in the form they would search harnaas.json
// for — not a normalized one that appears nowhere in the file they have open.
func declaredSource(ref manifest.AssetRef) string {
	if ref.Local() {
		return ref.Path
	}
	return ref.SourceKey + ":" + ref.Path
}
