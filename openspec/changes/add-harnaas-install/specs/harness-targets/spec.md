## Purpose

Defines how `harnaas` knows where an asset belongs: the adapter contract every supported harness
implements, the registry that discovers adapters, how project and user scopes resolve to concrete
roots, and the Claude Code adapter's mapping from asset type to destination — including the managed
block it maintains for rule assets, which Claude Code has no dedicated directory for.

## ADDED Requirements

### Requirement: Harness Adapter Contract

A harness SHALL be represented by an adapter exposing a stable registry name, a human-readable
display name, presence detection, the root directory for a given scope, and the destination for a
given asset type. Behaviour that only some harnesses provide SHALL be expressed as an optional
capability an adapter opts into, so the core contract stays small as harnesses are added.

#### Scenario: Adapter answers the core contract

- **WHEN** the install flow needs the destination for an asset on a given harness
- **THEN** it obtains it from that harness's adapter without knowing which harness it is

#### Scenario: Optional capability is detected, not assumed

- **WHEN** a harness does not implement an optional capability
- **THEN** the caller detects its absence and takes the documented fallback rather than failing

### Requirement: Harness Registry

Adapters SHALL register themselves with a central registry at package initialization, and the
registry SHALL expose lookup by name and a listing in a deterministic order. Registering two adapters
under the same name is a programming error and MUST fail loudly at startup rather than silently
replacing one. A lookup for an unregistered name SHALL report the name and list those available.

#### Scenario: Listing is deterministic

- **WHEN** the registry is listed
- **THEN** the same order is produced on every run, independent of registration order

#### Scenario: Unknown harness lookup is actionable

- **WHEN** a manifest targets a harness name that is not registered
- **THEN** the error names the unknown harness and lists the registered ones

#### Scenario: Duplicate registration fails loudly

- **WHEN** two adapters register under the same name
- **THEN** startup fails immediately rather than silently keeping one of them

### Requirement: Harness Detection

Each adapter SHALL report whether its harness is present for a given project, based on observable
evidence such as its configuration directory existing. Detection MUST NOT create anything, and MUST
report absence rather than an error when the harness is simply not installed.

#### Scenario: Present harness is detected

- **WHEN** a project contains the Claude Code configuration directory
- **THEN** the Claude Code adapter reports the harness as present

#### Scenario: Detection has no side effects

- **WHEN** detection runs against a project with no harness directories
- **THEN** it reports absence and creates nothing on disk

### Requirement: Scope Root Resolution

An adapter SHALL resolve a scope to a concrete root directory: `project` scope resolves to the
harness's directory within the project root, and `user` scope resolves to the harness's per-user home
directory. Resolution of the user scope MUST come from a single place in the codebase so the path is
never derived independently elsewhere.

#### Scenario: Project scope resolves inside the project

- **WHEN** an asset is scoped to the project on the Claude Code harness
- **THEN** its root is the Claude Code configuration directory inside the project root

#### Scenario: User scope resolves to the user home

- **WHEN** an asset is scoped to the user on the Claude Code harness
- **THEN** its root is the Claude Code per-user configuration directory

#### Scenario: User home is unavailable

- **WHEN** the per-user directory cannot be determined
- **THEN** resolving a user-scoped destination fails naming the asset and the reason, and no
  project-scoped fallback is silently substituted

### Requirement: Claude Code Destination Mapping

The Claude Code adapter SHALL map each asset type to a destination beneath the resolved scope root,
using the asset's id as the final path segment: a `skill` to a directory under the skills directory,
a `command` to a markdown file under the commands directory, a `subagent` to a markdown file under
the agents directory, and a `rule` to a markdown file under a harnaas-owned rules directory.

#### Scenario: Skill maps to a directory

- **WHEN** a `skill` asset with id `code-review` targets Claude Code at project scope
- **THEN** its destination is the `code-review` directory under the project's Claude Code skills
  directory

#### Scenario: Command maps to a file named for the asset

- **WHEN** a `command` asset with id `deploy` targets Claude Code
- **THEN** its destination is a markdown file named for that id under the commands directory

#### Scenario: User-scoped skill maps under the user root

- **WHEN** a `skill` asset is scoped to the user
- **THEN** its destination is the same relative path beneath the per-user root rather than the
  project root

### Requirement: Managed Rule Reference Block

Because Claude Code has no rules directory it reads automatically, the adapter SHALL install `rule`
assets as files and additionally maintain a marker-delimited block in the scope's memory file that
references them. The block SHALL be the only part of that file `harnaas` writes, SHALL be regenerated
in a deterministic order, and SHALL be removed entirely when no rule assets remain.

#### Scenario: Block is created with a reference per rule

- **WHEN** rule assets are installed for Claude Code
- **THEN** a marker-delimited block appears in the scope's memory file containing one reference per
  installed rule, in a deterministic order

#### Scenario: Content outside the block is preserved

- **WHEN** the memory file already contains hand-written content
- **THEN** that content is byte-for-byte unchanged and only the marker-delimited block is rewritten

#### Scenario: Block is removed when no rules remain

- **WHEN** the last rule asset is removed from the manifest and install runs
- **THEN** the block and its markers are removed and the rest of the file is left intact

#### Scenario: Memory file is created only when needed

- **WHEN** rule assets are installed and no memory file exists
- **THEN** one is created containing only the managed block

### Requirement: Unsupported Asset Type Handling

When an asset type has no destination on a target harness, the adapter SHALL report the combination
as unsupported rather than inventing a path or failing the run. The install flow SHALL surface it as
a distinct outcome so the user learns that asset was skipped for that harness and why.

#### Scenario: Unsupported combination is skipped, not failed

- **WHEN** an asset type has no mapping on one of its target harnesses
- **THEN** that target is reported as unsupported, the run continues, and other targets still install

#### Scenario: Unsupported outcome names the harness

- **WHEN** an unsupported combination is reported
- **THEN** the message names the asset, its type and the harness that does not support it

### Requirement: Destination Containment

Every computed destination MUST resolve inside its harness's scope root. An asset id or mapping that
would place a file outside that root MUST be rejected before any write. Writes SHALL be performed
through a handle confined to the scope root so containment holds even if a path component is replaced
between validation and use.

#### Scenario: Escaping destination is rejected

- **WHEN** a computed destination would resolve outside the scope root
- **THEN** the install is refused for that asset and nothing is written

#### Scenario: Containment survives a symlinked path component

- **WHEN** a directory component beneath the scope root is replaced with a symbolic link pointing
  elsewhere
- **THEN** the write fails rather than following the link outside the root

### Requirement: Adapter Import Boundary

Harness adapters SHALL depend only on the adapter contract package and general-purpose utility
packages, and MUST NOT import the install, lockfile, or command-layer packages. This boundary SHALL be
enforced by an automated check that fails the build, and widening it MUST require an explicit edit to
that check's allowlist.

#### Scenario: Forbidden import fails the build

- **WHEN** an adapter imports the install flow
- **THEN** the automated boundary check fails naming the offending import

#### Scenario: Widening the allowlist is explicit

- **WHEN** an adapter legitimately needs a package outside the current allowlist
- **THEN** the check fails with a message directing the author to add it to the allowlist
  deliberately

#### Scenario: Adapters register without being referenced

- **WHEN** the boundary check runs
- **THEN** it also confirms every adapter package registers itself, so no adapter is silently absent
  from the registry
