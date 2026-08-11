## Purpose

Defines `harnaas.json`, the committed, human-edited file in which a project declares which harness
assets it wants and where each one comes from. It covers the document's location, schema, decoding
strictness, the shorthand-to-canonical normalization of source references, and the validation rules
that make a manifest usable — but not how sources are fetched or installed.

## ADDED Requirements

### Requirement: Manifest Location And Discovery

The manifest SHALL be a single file named `harnaas.json` at the project root. Commands that need it
SHALL resolve it from the project root carried in the request context, so the same manifest is found
from any subdirectory. When the file is absent, a command that requires it SHALL say so and name the
command that creates one, rather than proceeding with empty defaults.

#### Scenario: Manifest found from a subdirectory

- **WHEN** a command is run from a nested directory of a project whose root contains `harnaas.json`
- **THEN** that manifest is loaded and its relative paths resolve against the project root

#### Scenario: Missing manifest is reported actionably

- **WHEN** a command requiring the manifest runs in a project with no `harnaas.json`
- **THEN** the CLI reports that no manifest was found, names `harnaas init` as the way to create one,
  and exits non-zero

### Requirement: Manifest Document Schema

The manifest SHALL be a JSON object carrying an integer `version`, an optional `harnesses` array of
harness names that assets target by default, and an `assets` array. Each asset entry SHALL carry a
unique string `id`, a `type`, a `source`, and optional `targets` and `scope` fields. Unrecognized
top-level shapes MUST be rejected rather than ignored.

#### Scenario: Minimal valid manifest is accepted

- **WHEN** a manifest declares a version, one harness, and one asset with an id, type and source
- **THEN** it loads successfully and the asset is available to later phases

#### Scenario: Assets must be an array

- **WHEN** `assets` is present but is not an array
- **THEN** loading fails with an error naming the `assets` field

### Requirement: Strict Manifest Decoding

Because `harnaas.json` is committed and hand-edited, decoding SHALL reject unknown fields, so a
misspelled key surfaces immediately instead of being silently dropped. A decoding failure SHALL name
the offending field and MUST NOT fall back to partial or default values.

#### Scenario: Misspelled field is rejected

- **WHEN** the manifest contains an asset field spelled `targetss` instead of `targets`
- **THEN** loading fails with an error naming the unknown field, and no asset is processed

#### Scenario: Malformed JSON is rejected

- **WHEN** the manifest is not well-formed JSON
- **THEN** loading fails with a parse error identifying the location, and no partial manifest is used

### Requirement: Asset Source Forms

An asset `source` SHALL be expressible either as a canonical object carrying a `kind` discriminator
and its kind-specific fields, or as an equivalent string shorthand. A shorthand SHALL be normalized
into the canonical object at load time so every later phase sees one representation. Version 1
recognizes the source kinds `github` and `local`.

#### Scenario: GitHub shorthand is normalized

- **WHEN** an asset declares the source string `github:owner/repo/skills/review@v1.2.0`
- **THEN** it normalizes to a `github` source with repository `owner/repo`, path `skills/review`, and
  ref `v1.2.0`

#### Scenario: Local shorthand is normalized

- **WHEN** an asset declares the source string `local:.harnaas/rules/house.md`
- **THEN** it normalizes to a `local` source with path `.harnaas/rules/house.md`

#### Scenario: Canonical object form is accepted unchanged

- **WHEN** an asset declares its source as an object with kind `github` and explicit repository, path
  and ref fields
- **THEN** it is used as-is and produces the same normalized source as the equivalent shorthand

#### Scenario: Unknown source kind is rejected

- **WHEN** an asset declares a source whose kind is neither `github` nor `local`
- **THEN** loading fails naming the unsupported kind and the asset id

### Requirement: Source Reference Fields

A `github` source SHALL carry an owner-and-repository identifier, an optional path within that
repository defaulting to its root, and an optional `ref` naming a tag, branch or commit. When `ref` is
omitted the repository's default branch SHALL be used. A `local` source SHALL carry a path that MUST
be relative to the project root and MUST resolve inside the project's `.harnaas` directory.

#### Scenario: Omitted ref defaults to the default branch

- **WHEN** a GitHub source declares no ref
- **THEN** the normalized source records that the repository's default branch is to be used

#### Scenario: Local source outside .harnaas is rejected

- **WHEN** a local source names a path outside the `.harnaas` directory, including by traversing
  upward with parent-directory segments
- **THEN** loading fails reporting that local sources must live under `.harnaas`

#### Scenario: Absolute local path is rejected

- **WHEN** a local source names an absolute filesystem path
- **THEN** loading fails reporting that local source paths must be relative to the project root

### Requirement: Asset Type Declaration

Each asset SHALL declare a `type` of `skill`, `rule`, `command`, or `subagent`. The type determines
where the asset is installed and whether its source is expected to be a single file or a directory
tree: a `skill` SHALL resolve to a directory, and the other types SHALL resolve to a single file.
An unrecognized type MUST be rejected at load time.

#### Scenario: Recognized type is accepted

- **WHEN** an asset declares type `skill`
- **THEN** it loads and is recorded as requiring a directory source

#### Scenario: Unknown type is rejected

- **WHEN** an asset declares a type outside the recognized set
- **THEN** loading fails naming the asset id and listing the recognized types

### Requirement: Target And Scope Defaults

An asset's `targets` SHALL default to the manifest's top-level `harnesses` list when omitted, and its
`scope` SHALL default to `project`. A `scope` of `user` SHALL mark the asset for installation into the
harness's per-user location instead of the project's. An asset whose effective target list is empty
MUST be rejected, since it could never be installed anywhere.

#### Scenario: Targets inherit the top-level harness list

- **WHEN** the manifest lists harnesses `["claude-code"]` and an asset omits `targets`
- **THEN** that asset's effective targets are `["claude-code"]`

#### Scenario: Explicit targets override the default

- **WHEN** an asset declares its own `targets`
- **THEN** the manifest's top-level harness list is not applied to that asset

#### Scenario: Empty effective target list is rejected

- **WHEN** an asset omits `targets` and the manifest declares no top-level harnesses
- **THEN** loading fails reporting that the asset has no target harness

#### Scenario: User scope is recorded

- **WHEN** an asset declares scope `user`
- **THEN** the loaded asset is marked for the per-user location of each of its target harnesses

### Requirement: Manifest Semantic Validation

Loading SHALL validate the whole document after decoding and report every violation found, not only
the first. Asset ids MUST be unique and MUST be usable as a filesystem path segment. A declared
harness name MUST be one the CLI recognizes. Validation failures MUST prevent the manifest from being
used.

#### Scenario: Duplicate asset ids are rejected

- **WHEN** two assets declare the same `id`
- **THEN** validation fails naming the duplicated id

#### Scenario: Unsafe asset id is rejected

- **WHEN** an asset id contains a path separator or a parent-directory segment
- **THEN** validation fails reporting that the id must be a single safe path segment

#### Scenario: Unknown harness name is rejected

- **WHEN** the manifest targets a harness the CLI does not recognize
- **THEN** validation fails naming the unknown harness and listing the recognized ones

#### Scenario: All violations are reported together

- **WHEN** a manifest contains several independent validation violations
- **THEN** the reported error describes every violation rather than stopping at the first

### Requirement: Manifest Version Handling

The manifest SHALL declare a schema `version`, and the CLI SHALL reject a version it does not
understand with a message telling the user to upgrade, rather than attempting a partial read. Version
`1` is defined by this specification.

#### Scenario: Known version loads

- **WHEN** the manifest declares version `1`
- **THEN** it is decoded according to this specification

#### Scenario: Newer version is refused with guidance

- **WHEN** the manifest declares a version greater than the CLI understands
- **THEN** loading fails telling the user the manifest was written by a newer `harnaas` and to
  upgrade, and no assets are processed

#### Scenario: Missing version is rejected

- **WHEN** the manifest omits `version`
- **THEN** loading fails reporting that the version field is required
