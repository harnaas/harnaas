package cli

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	// ADR 0005's second half: this is the refusal no manifest edit resolves, so
	// it carries its own remedy rather than inheriting the one that sends a
	// reader to narrow `targets` in a file that is already correct.
	assert.NotEmpty(t, plan.Remedy, "the refusal states its own remedy")
	assert.Contains(t, plan.Remedy, manifest.FileName)
	assert.Contains(t, plan.Remedy, "already correct")
}

// TestARefusalTheManifestCanResolveStatesNoRemedyOfItsOwn keeps the override from
// quietly becoming the answer for every refusal.
//
// A pairing whose harness simply has no surface for the type is one narrowing
// `targets` genuinely settles, so the plan leaves the remedy empty and the
// outcome's default — the edit that settles it — is what the reader gets.
//
// A persona is the type that asks this cleanly. A rule with no rules surface is
// emulated into the memory file rather than refused, and a command is the ADR
// 0005 case itself, so neither would be testing the default at all.
func TestARefusalTheManifestCanResolveStatesNoRemedyOfItsOwn(t *testing.T) {
	t.Parallel()

	registry := &adapter.Registry{}
	registry.Register(skillless{id: harness.DevinCLI})

	persona := manifest.Asset{
		Type: manifest.AssetTypePersona, ID: "reviewer",
		Targets: []harness.ID{harness.DevinCLI}, Scope: manifest.ScopeProject,
	}

	plan := planTarget(persona, nil, harness.DevinCLI, registry)

	require.False(t, plan.supported(), "the double offers no surface for a persona")
	assert.Empty(t, plan.Remedy,
		"nothing here beats the default, and a plan that answered anyway would be a second place to keep it right")
}

// TestTypeReachesHarnessAgreesWithThePlan is the pairing that keeps `init`'s
// scaffolding honest. The directories a selection earns are exactly the types an
// install would find somewhere to put, so the two answers are asserted equal over
// every pairing the registry can produce rather than written down twice.
//
// The probe asset carries no content, which is the one place the two questions
// genuinely differ: a rule declaring path scoping is refused where an unscoped one
// is emulated into the memory file, and a directory that does not exist yet has no
// content to ask about. Asking with none is asking the question a directory is
// for.
func TestTypeReachesHarnessAgreesWithThePlan(t *testing.T) {
	t.Parallel()

	// The binary's own registry, for the reason the registered-adapter test uses
	// it: what has to agree is what this binary installs with, not a fixture
	// assembled to agree.
	registry := adapter.Default

	// The harnesses with no adapter at all are as much part of the contract as
	// the two with one: "a harness with no adapter" is a supported state.
	targets := append(harnessIDs(), harness.ID("unmapped"))

	for _, target := range targets {
		for _, assetType := range manifest.AssetTypes() {
			probe := manifest.Asset{
				Type: assetType, ID: "example",
				Targets: []harness.ID{target}, Scope: manifest.ScopeProject,
			}

			assert.Equal(t,
				planTarget(probe, nil, target, registry).supported(),
				typeReachesHarness(target, assetType, registry),
				"%s / %s: the scaffolding and the install flow disagree about this pairing",
				target, assetType)
		}
	}
}

// TestTypeReachesHarnessOnTodaysRoster pins the answers themselves, so a change
// on either side of the agreement above is visible as the behaviour it changes
// rather than only as two functions still matching each other.
func TestTypeReachesHarnessOnTodaysRoster(t *testing.T) {
	t.Parallel()

	registry := adapter.Default

	for _, assetType := range manifest.AssetTypes() {
		assert.True(t, typeReachesHarness(harness.ClaudeCode, assetType, registry),
			"claude-code has a surface for every type, %s included", assetType)
	}

	for _, assetType := range []manifest.AssetType{
		manifest.AssetTypeSkill,
		manifest.AssetTypeRule,
		manifest.AssetTypeInstruction,
		manifest.AssetTypePersona,
	} {
		assert.True(t, typeReachesHarness(harness.DevinCLI, assetType, registry),
			"devin-cli takes a %s", assetType)
	}

	assert.False(t, typeReachesHarness(harness.DevinCLI, manifest.AssetTypeCommand, registry),
		"devin-cli has no command surface and cannot be told to leave a skill alone; see ADR 0005")
}

// harnessIDs is the roster's ids, in the roster's order.
func harnessIDs() []harness.ID {
	ids := make([]harness.ID, 0, len(harness.All()))
	for _, h := range harness.All() {
		ids = append(ids, h.ID)
	}
	return ids
}
