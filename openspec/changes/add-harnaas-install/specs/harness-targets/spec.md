## Purpose

Defines where an asset lands. Most harnesses read a shared skills directory and a shared memory file,
so those are written first; a named adapter is required only to reach a harness's own rule, command
and persona surfaces. Covers the adapter contract and registry, scope resolution, support tiers,
destination containment, and the managed instruction block in `AGENTS.md`.

## ADDED Requirements

### Requirement: Shared Target Precedence

`skill` and `instruction` assets SHALL install to the shared targets first: `.agents/skills/<id>/SKILL.md`
for a skill, and the managed block in the project's `AGENTS.md` for an instruction. These types SHALL
be written to a harness's own directory only where that harness is known not to read the shared
target, and a harness that does read it MUST NOT receive a duplicate copy.

#### Scenario: One write serves many harnesses

- **WHEN** a manifest targets several harnesses that all read the shared skills directory
- **THEN** a single `.agents/skills/<id>/SKILL.md` is written and no per-harness copy is made

#### Scenario: Fallback only where the shared target is not read

- **WHEN** a target harness is known not to read the shared skills directory
- **THEN** the skill is additionally written to that harness's own skills directory

#### Scenario: Shared targets need no named adapter

- **WHEN** a manifest targets a harness that has no named adapter registered
- **THEN** its `skill` and `instruction` assets still install through the shared targets

### Requirement: Named Adapter Surfaces

`rule`, `command` and `persona` assets have no shared equivalent and SHALL install only through a
named adapter for the target harness. Version 1 SHALL ship exactly one named adapter, `claude-code`.
Targeting a harness that has no named adapter with one of these types MUST be reported as unsupported
rather than resolved to a guessed path.

#### Scenario: The one named adapter is registered

- **WHEN** the registry is consulted in version 1
- **THEN** `claude-code` is the only named adapter present

#### Scenario: Harness-specific type without an adapter

- **WHEN** a `command` asset targets a harness with no named adapter
- **THEN** that pairing is reported unsupported and no file is written for it

### Requirement: Harness Adapter Contract

A named adapter SHALL expose a stable registry identity, presence detection, a root directory for
each scope it supports, and a destination for a given asset type and scope. An asset type the harness
has no surface for MUST be reported as having no destination rather than mapped to an invented path.
The contract SHALL be uniform, so the install flow computes a destination without knowing which
harness it is addressing.

#### Scenario: Destination comes from the adapter

- **WHEN** the install flow needs the destination for an asset on a named harness
- **THEN** it obtains the destination from that harness's adapter with no harness-specific branching

#### Scenario: Missing surface is reported, not invented

- **WHEN** an adapter has no surface for an asset type
- **THEN** it reports the absence and the caller takes the documented unsupported path

### Requirement: Harness Registry

Named adapters SHALL register themselves with a central registry at package initialization, and the
registry SHALL expose lookup by identity and a listing in a deterministic order independent of
registration order. Two adapters registering the same identity is a programming error and MUST fail
loudly at startup. A lookup for an unregistered identity SHALL name it and list those registered.

#### Scenario: Listing is deterministic

- **WHEN** the registry is listed
- **THEN** the same order is produced on every run, independent of registration order

#### Scenario: Duplicate registration fails loudly

- **WHEN** two adapters register under the same identity
- **THEN** startup fails immediately rather than silently keeping one of them

#### Scenario: Unknown harness is actionable

- **WHEN** a manifest names a harness that is not registered
- **THEN** the error names the unknown harness and lists the registered ones

### Requirement: Harness Detection

Each named adapter SHALL report whether its harness is present, from observable evidence such as its
configuration directory existing. Detection MUST create nothing on disk, and MUST report absence
rather than an error when the harness is simply not installed.

#### Scenario: Present harness is detected

- **WHEN** a project contains the Claude Code configuration directory
- **THEN** the `claude-code` adapter reports the harness as present

#### Scenario: Detection has no side effects

- **WHEN** detection runs against a project with no harness directories
- **THEN** it reports absence and creates nothing on disk

### Requirement: Scope Resolution

Scope SHALL default to `project`, with `user` opt-in. An adapter SHALL offer `user` scope only where
that harness's per-user location is unambiguous. Declaring `user` scope for a harness that does not
offer it MUST be a validation error naming the harness and the asset, refused before any write, and
MUST NOT fall back silently to `project`.

#### Scenario: Project is the default root

- **WHEN** an asset declares no scope
- **THEN** it resolves beneath the harness's project root

#### Scenario: User scope resolves where it is unambiguous

- **WHEN** an asset declares `user` scope on an adapter that offers it
- **THEN** the same relative destination resolves beneath the per-user root instead of the project root

#### Scenario: User scope on an adapter that does not offer it

- **WHEN** an asset declares `user` scope on an adapter with no unambiguous per-user location
- **THEN** validation fails naming the asset and the harness, and nothing is written

### Requirement: Claude Code Destination Mapping

Beneath the resolved scope root, the `claude-code` adapter SHALL map a `rule` to `rules/<id>.md`, a
`command` to `commands/<id>.md` and a `persona` to `agents/<id>.md`, using the asset id as the file
stem. Rule files SHALL be standalone files the harness discovers on its own, and MUST NOT be
referenced from `CLAUDE.md`, `AGENTS.md` or any other file.

#### Scenario: Rule maps to the rules directory

- **WHEN** a `rule` asset with id `house-style` targets Claude Code at project scope
- **THEN** its destination is `house-style.md` in the project's Claude Code rules directory

#### Scenario: Persona maps to the agents directory

- **WHEN** a `persona` asset with id `reviewer` targets Claude Code
- **THEN** its destination is `reviewer.md` under the Claude Code agents directory

#### Scenario: Rules are never referenced

- **WHEN** rule assets are installed for Claude Code
- **THEN** no reference to them is added to any memory file or managed block

### Requirement: Support Tier Gates Writes

Every adapter surface SHALL carry a support tier that decides whether harnaas writes to it. A `live`
surface SHALL be written with no note. A `removed` surface MUST be refused with an error naming its
replacement, because writing there is a guaranteed silent no-op. A `gated` surface SHALL be written
with a setup note and a `legacy` surface with a deprecation note; both notes MUST appear in the
install report.

#### Scenario: Removed surface is refused

- **WHEN** an asset targets a surface whose tier is `removed`
- **THEN** the write is refused with an error naming the replacement surface and nothing is written

#### Scenario: Gated surface writes with a setup note

- **WHEN** an asset targets a surface whose tier is `gated`
- **THEN** the file is written and the install report carries the setup note for that surface

#### Scenario: Legacy surface writes with a deprecation note

- **WHEN** an asset targets a surface whose tier is `legacy`
- **THEN** the file is written and the install report carries the deprecation note

#### Scenario: Live surface writes quietly

- **WHEN** an asset targets a surface whose tier is `live`
- **THEN** the file is written and no tier note is attached to the outcome

### Requirement: Destination Containment

Every computed destination MUST resolve inside its scope root, and a mapping or asset id that would
escape it MUST be rejected before any write. Writes SHALL be performed through a handle anchored at
the scope root, so containment holds even if a path component is replaced between validation and use.

#### Scenario: Escaping destination is rejected

- **WHEN** a computed destination would resolve outside the scope root
- **THEN** the install is refused for that asset and nothing is written

#### Scenario: Containment survives a swapped path component

- **WHEN** a directory component beneath the scope root is replaced with a link pointing elsewhere
- **THEN** the write fails rather than following it outside the root

### Requirement: Unsupported Pairing Outcome

Where an asset type has no destination on a target harness, harnaas SHALL report `unsupported` for
that asset and target as a first-class outcome, rather than inventing a path or failing the run. The
report MUST name the asset, its type and the harness, and the asset's other targets SHALL still
install.

#### Scenario: Unsupported does not stop the run

- **WHEN** one of an asset's targets does not support its type
- **THEN** that pairing is reported unsupported and the asset still installs on its other targets

#### Scenario: Unsupported outcome is specific

- **WHEN** an unsupported pairing is reported
- **THEN** the message names the asset, its type and the harness that has no surface for it

### Requirement: Managed Instruction Block In AGENTS.md

harnaas SHALL maintain a marker-delimited managed block in the project's `AGENTS.md` containing every
`instruction` asset inlined verbatim, sorted by asset id, each preceded by an HTML comment naming the
asset and its source. Everything outside the markers MUST be preserved byte-for-byte. The block and
its markers SHALL be removed when the last instruction asset goes.

#### Scenario: Block carries content and provenance

- **WHEN** instruction assets are installed
- **THEN** the block holds each asset's content verbatim, preceded by an HTML comment naming the asset
  and the source it came from

#### Scenario: Ordering is deterministic

- **WHEN** the manifest's asset order is changed but its instruction set is not
- **THEN** the regenerated block is byte-identical, ordered by asset id

#### Scenario: Surrounding content is untouched

- **WHEN** `AGENTS.md` already contains hand-written content around the markers
- **THEN** that content is byte-for-byte unchanged and only the block between the markers is rewritten

#### Scenario: AGENTS.md is created when absent

- **WHEN** instruction assets are installed and no `AGENTS.md` exists
- **THEN** one is created containing only the managed block

#### Scenario: Block disappears with the last instruction

- **WHEN** the last instruction asset is removed from the manifest and install runs
- **THEN** the block and both markers are removed and the rest of `AGENTS.md` is left intact

### Requirement: CLAUDE.md Bridge Line

While any instruction asset is installed, harnaas SHALL ensure exactly one `@AGENTS.md` line exists in
`CLAUDE.md`, appended at the end of the file, creating `CLAUDE.md` containing only that line when it
is absent. When no instruction assets remain, harnaas SHALL remove that line, and MUST delete
`CLAUDE.md` only if the file then contains nothing else.

#### Scenario: Line is appended to an existing file

- **WHEN** instruction assets are installed and `CLAUDE.md` exists without the bridge line
- **THEN** the line is appended at the end and the existing content is preserved byte-for-byte

#### Scenario: File is created when absent

- **WHEN** instruction assets are installed and no `CLAUDE.md` exists
- **THEN** `CLAUDE.md` is created containing only the `@AGENTS.md` line

#### Scenario: Line is not duplicated

- **WHEN** install runs again and the bridge line is already present
- **THEN** `CLAUDE.md` is left unchanged and no second line is added

#### Scenario: Empty file is deleted on removal

- **WHEN** the last instruction asset goes and `CLAUDE.md` contains only the bridge line
- **THEN** the line is removed and the file is deleted

#### Scenario: File with other content survives removal

- **WHEN** the last instruction asset goes and `CLAUDE.md` contains other content
- **THEN** only the bridge line is removed and the file is kept

### Requirement: Adapter Import Boundary

Adapter packages SHALL import only the adapter contract package and general-purpose utilities, and
MUST NOT import the install, lockfile or command-layer packages. A hand-written test SHALL parse each
adapter package's syntax tree and fail on any import outside an explicit allowlist. That same test
MUST assert every adapter package registers itself, so no adapter is silently missing from the
registry.

#### Scenario: Forbidden import fails the test

- **WHEN** an adapter package imports the install flow
- **THEN** the boundary test fails naming the offending package and import

#### Scenario: Widening the boundary is deliberate

- **WHEN** an adapter needs a package outside the current allowlist
- **THEN** the test fails until the allowlist is edited explicitly to admit it

#### Scenario: Missing self-registration fails the test

- **WHEN** an adapter package exists but does not register itself
- **THEN** the same test fails naming that package
