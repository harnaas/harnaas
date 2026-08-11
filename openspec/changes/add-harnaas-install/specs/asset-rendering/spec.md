## Purpose

Defines how `harnaas install` produces the bytes it writes: rendering is the general case and copying
verbatim is merely the default renderer, so this capability covers the render hook on the adapter
contract, the renderers v1 implements and those it only declares, the rule-to-instruction emulation
rule, frontmatter pass-through, the two independent digests that separate "upstream moved" from
"someone edited it", and the harness limits checked before anything lands.

## ADDED Requirements

### Requirement: Rendering Is The General Case

Install SHALL treat every installed file as the output of a renderer applied to source content for a
given target, with copying bytes verbatim being the default renderer rather than an invariant. A
renderer MUST be a deterministic function of source content and target, so the same source rendered
for the same target always produces byte-identical output.

#### Scenario: Default renderer copies verbatim

- **WHEN** an asset installs on a target with no declared renderer for its type
- **THEN** the installed bytes are identical to the source bytes

#### Scenario: Rendered output is deterministic

- **WHEN** the same source is rendered twice for the same target
- **THEN** the two outputs are byte-identical, so a re-run reports `unchanged`

### Requirement: Two Independent Digests

Each installation SHALL record a source digest over the upstream content and an installed digest over
the bytes actually written. The source digest answers whether upstream moved; the installed digest
answers whether the destination was edited outside harnaas. Neither digest MUST be derived from the
other, and the two SHALL be compared independently.

#### Scenario: Upstream moved

- **WHEN** the upstream content changes and the destination is untouched
- **THEN** the source digest differs while the installed digest still matches, and the state is
  reported as an available update rather than drift

#### Scenario: Someone edited the installed file

- **WHEN** an installed file is edited in place and upstream is unchanged
- **THEN** the installed digest differs while the source digest still matches, and the state is
  reported as drift rather than an available update

#### Scenario: Digests differ under a transforming renderer

- **WHEN** a renderer other than identity produced the installed file
- **THEN** the source and installed digests differ from each other by design, and neither comparison
  treats that difference as drift or as an update

### Requirement: Per-Type Render Hook

The adapter contract SHALL declare a render hook keyed by asset type, so a harness can state how each
type is rendered without any change to the install flow. An adapter that declares no hook for a type
SHALL receive the identity renderer. The install flow MUST select the renderer through this hook
without knowing which harness it is serving.

#### Scenario: No hook means identity

- **WHEN** an adapter declares no render hook for an asset type
- **THEN** that type installs through the identity renderer

#### Scenario: Hook applies only to its own type

- **WHEN** an adapter declares a render hook for one asset type
- **THEN** only that type is rendered through it and every other type still uses identity

#### Scenario: Install flow stays harness-agnostic

- **WHEN** install renders an asset for a target
- **THEN** it obtains the renderer from the adapter rather than branching on the harness name

### Requirement: Identity Renderer

The identity renderer SHALL reproduce the source bytes unchanged, including frontmatter, line endings
and trailing whitespace, and MUST NOT normalize, reformat or re-encode content. It SHALL be the
renderer used for every asset type on every harness in v1 except where the as-skill renderer applies.

#### Scenario: Skill directory is copied file for file

- **WHEN** a `skill` asset installs
- **THEN** every file beneath it is written byte-for-byte as authored upstream

#### Scenario: No normalization is applied

- **WHEN** source content uses line endings or trailing whitespace that differ from the local
  convention
- **THEN** the installed file preserves them exactly

### Requirement: As-Skill Renderer For Commands

Where a target harness has no command surface, a `command` asset SHALL be rendered as a `SKILL.md`
with model invocation disabled, so the harness will not load it on its own initiative and the user
must still invoke it deliberately. The rendered skill SHALL keep the command's id and body, and the
installation SHALL be reported as emulated.

#### Scenario: Command becomes a non-self-invoking skill

- **WHEN** a `command` asset targets a harness with no command surface
- **THEN** it is installed as a `SKILL.md` whose frontmatter disables model invocation, under a
  destination named for the command's id

#### Scenario: Native command surface is preferred

- **WHEN** a `command` asset targets a harness that has a command surface
- **THEN** the identity renderer is used and the outcome is a plain install, not an emulation

### Requirement: Declared But Unimplemented Renderers

Renderers that convert an asset to TOML, to YAML, to TypeScript, or into a shared document SHALL be
named by the render contract but MUST NOT be implemented in v1. Selecting one SHALL report that
asset-target pair as `unsupported`, naming the renderer, and MUST NOT fall back to identity, which
would write a file the harness cannot read.

#### Scenario: Unimplemented renderer is refused

- **WHEN** an adapter selects a renderer that v1 does not implement
- **THEN** the target is reported `unsupported` naming the renderer, nothing is written, and the run
  continues for other targets

#### Scenario: No silent fallback to identity

- **WHEN** a declared renderer has no implementation
- **THEN** the source bytes are not copied verbatim in its place

### Requirement: Rule To Instruction Emulation

A `rule` asset that declares no path scoping SHALL be eligible for emulation through the instruction
surface on a harness with no rules directory, because its always-on semantics are then unchanged. A
`rule` that declares `paths:` MUST NOT be emulated; it SHALL be reported `unsupported` for that
harness rather than silently installed as always-on guidance.

#### Scenario: Unscoped rule is emulated

- **WHEN** a `rule` asset with no path scoping targets a harness with no rules directory
- **THEN** it is installed through the instruction surface and reported as emulated

#### Scenario: Path-scoped rule is refused, not widened

- **WHEN** a `rule` asset declaring `paths:` targets a harness with no rules directory
- **THEN** the target is reported `unsupported`, the message names the path scoping as the reason,
  and nothing is written

#### Scenario: Native rules directory wins

- **WHEN** the target harness has its own rules directory
- **THEN** the rule installs there natively and no emulation occurs, whether or not it declares
  `paths:`

### Requirement: Emulation Is Reported As Its Own Outcome

An emulated installation SHALL be reported with the `emulated` outcome together with a statement of
how it differs from native support, and MUST NOT be reported as `created`, `updated` or `unchanged`.
The lockfile SHALL record that the installation was emulated so later runs and lint can tell it apart
from a native one.

#### Scenario: Emulation is never plain success

- **WHEN** an asset installs through emulation
- **THEN** the report shows the `emulated` outcome and explains the behavioural difference, in both
  the text and `--json` reports

#### Scenario: Re-running keeps the emulated marking

- **WHEN** an already-emulated installation is re-run with nothing changed
- **THEN** the report still identifies it as emulated rather than reporting plain success

### Requirement: Frontmatter Pass-Through

harnaas SHALL pass asset frontmatter through untouched and MUST NOT add, remove, reorder or rewrite
keys — in particular it never rewrites `paths:`. Where a renderer's contract requires setting a key,
as as-skill does to disable model invocation, it SHALL change only that key and leave every other key
exactly as authored.

#### Scenario: Unknown keys survive

- **WHEN** an asset's frontmatter contains keys harnaas does not interpret
- **THEN** they appear in the installed file unchanged and in their original order

#### Scenario: Path scoping is never rewritten

- **WHEN** a `rule` declaring `paths:` installs on a harness that supports rules
- **THEN** the `paths:` value is written exactly as authored

#### Scenario: A renderer touches only what it must

- **WHEN** the as-skill renderer disables model invocation
- **THEN** only that key differs from the source frontmatter

### Requirement: Hard Harness Limits Fail Installation

Install SHALL fail an asset whose rendered output exceeds a limit the target harness provably
enforces — 4 MiB per memory file, 1 MB per `SKILL.md`, and an import depth of 5 — because a file past
the limit is silently skipped by the harness. The failure MUST name the limit, the measured value and
the asset, and nothing MUST be written for that asset.

#### Scenario: Oversized skill fails

- **WHEN** a rendered `SKILL.md` exceeds 1 MB
- **THEN** install fails for that asset naming the 1 MB limit and the measured size, and writes
  nothing for it

#### Scenario: Oversized memory file fails before writing

- **WHEN** the assembled memory file would exceed 4 MiB
- **THEN** install fails during planning and the existing memory file is left untouched

#### Scenario: Excessive import depth fails

- **WHEN** an asset's imports nest more than five levels deep
- **THEN** install fails naming the depth limit and the import chain that exceeded it

#### Scenario: Limits are measured on rendered output

- **WHEN** a renderer changes the size of the content
- **THEN** the limits are checked against the bytes that would be written, not against the source

### Requirement: Assembled Always-On Content Warning

Install SHALL warn when the assembled always-on content for a scope reaches the soft threshold of
roughly 40,000 characters, naming the total and the largest contributors. This threshold is a
degradation point rather than an enforced limit, so the warning MUST NOT fail the run, block any
write, or change the bytes installed.

#### Scenario: Warning is advisory

- **WHEN** assembled always-on content passes the soft threshold
- **THEN** a warning is reported, the install still completes, and the exit code is unaffected

#### Scenario: Warning names the contributors

- **WHEN** the threshold warning is reported
- **THEN** it states the assembled character count and the assets contributing the most to it

#### Scenario: Below the threshold nothing is said

- **WHEN** assembled always-on content stays under the soft threshold
- **THEN** no warning is emitted
