package cli

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/harnaas/harnaas/cmd/harnaas/cli/adapter"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/harness"
	"github.com/harnaas/harnaas/cmd/harnaas/cli/manifest"
)

// skillless is an adapter for a harness with no skill surface, which no shipped
// adapter is and which the emulation path still has to answer for.
type skillless struct{ id harness.ID }

func (a skillless) Harness() harness.ID              { return a.id }
func (skillless) Detect(fs.FS) bool                  { return false }
func (skillless) Root(manifest.Scope) (string, bool) { return ".elsewhere", true }
func (skillless) Destination(manifest.Asset) (adapter.Destination, bool) {
	return adapter.Destination{}, false
}

// ownSkills is an adapter for a harness that keeps skills in its own directory.
type ownSkills struct{ id harness.ID }

func (a ownSkills) Harness() harness.ID              { return a.id }
func (ownSkills) Detect(fs.FS) bool                  { return false }
func (ownSkills) Root(manifest.Scope) (string, bool) { return ".elsewhere", true }
func (ownSkills) Destination(asset manifest.Asset) (adapter.Destination, bool) {
	if asset.Type != manifest.AssetTypeSkill {
		return adapter.Destination{}, false
	}
	return adapter.Destination{Path: "skills/" + asset.ID, Tier: adapter.TierLive}, true
}

// TestSuppressionIsRecordedPerHarness pins the table ADR 0005 rests on. The
// default is that a harness honours the key harnaas writes, and an entry is what
// records one that does not.
func TestSuppressionIsRecordedPerHarness(t *testing.T) {
	t.Parallel()

	assert.False(t, suppressesSkillAutoInvocation(harness.DevinCLI),
		"its skill format documents no such key and both invocation modes are on by default")
	assert.True(t, suppressesSkillAutoInvocation(harness.ClaudeCode))
	assert.True(t, suppressesSkillAutoInvocation("unrecorded"),
		"the default is honours-unless-recorded, the same shape as the shared-skills table")
}

// TestEveryHarnessOnTheRosterReadsTheSharedSkillsDirectory states the emptiness of
// the fallback table as a fact rather than leaving it as an absence. ADR 0002
// rests on it, and the day it stops being true this is what says so.
func TestEveryHarnessOnTheRosterReadsTheSharedSkillsDirectory(t *testing.T) {
	t.Parallel()

	for _, id := range harness.IDs() {
		assert.True(t, readsSharedSkills(id), "%s is recorded as not reading the shared skills directory", id)
	}
}

// TestAnEmulatedCommandLandsWhereTheHarnessReadsSkills covers the branch no
// harness on today's roster reaches. A harness that does not read the shared
// directory needs the copy in its own, and one that reads neither cannot take a
// command at all — which is the refusal rather than a path written into nothing.
func TestAnEmulatedCommandLandsWhereTheHarnessReadsSkills(t *testing.T) {
	t.Parallel()

	command := manifest.Asset{Type: manifest.AssetTypeCommand, ID: "deploy", Scope: manifest.ScopeProject}

	t.Run("reads the shared directory", func(t *testing.T) {
		t.Parallel()

		destination, offered := skillDestinationFor(
			func(harness.ID) bool { return true }, "shared", command, skillless{id: "shared"})

		assert.True(t, offered)
		assert.Equal(t, ".agents/skills/deploy", destination)
	})

	t.Run("keeps skills in its own directory", func(t *testing.T) {
		t.Parallel()

		destination, offered := skillDestinationFor(
			func(harness.ID) bool { return false }, "own", command, ownSkills{id: "own"})

		assert.True(t, offered)
		assert.Equal(t, "skills/deploy", destination,
			"the copy goes where that harness reads, not into a shared directory it ignores")
	})

	t.Run("reads neither", func(t *testing.T) {
		t.Parallel()

		destination, offered := skillDestinationFor(
			func(harness.ID) bool { return false }, "neither", command, skillless{id: "neither"})

		assert.False(t, offered)
		assert.Empty(t, destination)
	})
}

// TestAnAdapterWithNoSkillSurfaceStillTakesAnEmulatedCommand is the correction
// this change makes, stated as the behaviour rather than as the diagnostic. The
// previous condition asked the adapter for a skill surface, which no adapter
// offers by design, so every emulation was refused for a reason that was never
// the real one.
func TestAnAdapterWithNoSkillSurfaceStillTakesAnEmulatedCommand(t *testing.T) {
	t.Parallel()

	registry := &adapter.Registry{}
	registry.Register(skillless{id: harness.ClaudeCode})

	command := manifest.Asset{
		Type: manifest.AssetTypeCommand, ID: "deploy",
		Targets: []harness.ID{harness.ClaudeCode}, Scope: manifest.ScopeProject,
	}

	// The shared directory is what makes every rostered harness able to take a
	// skill, so the only harness that reads neither is one recorded as skipping
	// it — which none is. Registering an adapter with no skill surface under a
	// harness that does read the shared one therefore still resolves, and the
	// refusal this test wants is the one the table above cannot produce yet.
	plan := planTarget(command, nil, harness.ClaudeCode, registry)

	assert.True(t, plan.supported(),
		"a harness reading the shared directory can always take an emulated command")
	assert.Equal(t, ".agents/skills/deploy", plan.Destination)
	assert.Equal(t, adapter.RendererAsSkill, plan.Renderer)
}

// TestACommandIsRefusedWhereTheHarnessCannotBeSilenced is ADR 0005 at the layer
// that builds the sentence, asserted on the reason rather than only on the
// refusal: the message must name the obstacle, and must not claim a missing skill
// surface for a harness that has one.
func TestACommandIsRefusedWhereTheHarnessCannotBeSilenced(t *testing.T) {
	t.Parallel()

	registry := &adapter.Registry{}
	registry.Register(skillless{id: harness.DevinCLI})

	command := manifest.Asset{
		Type: manifest.AssetTypeCommand, ID: "deploy",
		Targets: []harness.ID{harness.DevinCLI}, Scope: manifest.ScopeProject,
	}

	plan := planTarget(command, nil, harness.DevinCLI, registry)

	assert.False(t, plan.supported())
	assert.Empty(t, plan.Destination, "nothing a caller could write is handed back")
	assert.Contains(t, plan.Unsupported, string(harness.DevinCLI))
	assert.Contains(t, plan.Unsupported, autoInvokeKey)
	assert.Contains(t, plan.Unsupported, "unprompted")
	assert.NotContains(t, plan.Unsupported, "no skill surface",
		"this harness has one; the obstacle is that it cannot be told to leave a skill alone")
}
