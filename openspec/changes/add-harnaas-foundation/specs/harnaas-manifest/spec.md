## Purpose

Defines `harnaas.json`, the committed, hand-edited file in which a project declares which harness
assets it wants and where they come from. It covers the file's location, its strict decoding, the
document shape, the `sources` block, how each asset's type and id are inferred from its path, target
and scope defaulting, and the validation that must pass before any later phase reads it.

## ADDED Requirements

### Requirement: Manifest Location And Root-Only Rule

The manifest SHALL be a single file named `harnaas.json` at the repository root, resolved from the
project root carried in the request context so it is found from any subdirectory. A `harnaas.json`
discovered in a subdirectory of the project SHALL be a validation error naming the root manifest as
the only one harnaas reads. When the root has no manifest, a command requiring one SHALL say so and
name `harnaas init`.

#### Scenario: Manifest found from a subdirectory

- **WHEN** a command runs in a nested directory of a project whose root contains `harnaas.json`
- **THEN** that manifest is loaded and its relative paths resolve against the project root

#### Scenario: Manifest in a subdirectory is an error

- **WHEN** a `harnaas.json` exists in a subdirectory of the project
- **THEN** the run fails naming that file and stating that only the manifest at the repository root
  is read

#### Scenario: Missing manifest is reported actionably

- **WHEN** a command requiring the manifest runs in a project with no root `harnaas.json`
- **THEN** the CLI reports that no manifest was found, names `harnaas init` as the way to create one,
  and exits non-zero

### Requirement: Manifest Is Never Written By harnaas

Apart from `harnaas init` creating the file once, harnaas SHALL never write, rewrite, reformat or
normalize `harnaas.json`. `install` and `lint` SHALL treat it as read-only input: adding, removing or
repinning an asset is a hand edit. Every remedy harnaas reports for a manifest problem MUST be
phrased as an edit for a person to make, never as a command that edits the file.

#### Scenario: Install leaves the manifest untouched

- **WHEN** `harnaas install` runs against any manifest, including one it reports problems with
- **THEN** `harnaas.json` is byte-identical afterwards

#### Scenario: Remedy is an edit, not a write

- **WHEN** a command reports that an asset entry needs to change
- **THEN** the remedy states the exact edit to make in `harnaas.json` and no command that would apply
  it automatically is offered

### Requirement: Strict Manifest Decoding

Because the manifest is committed and hand-edited, decoding SHALL reject unknown fields and malformed
JSON, naming the offending field or the parse location. A decoding failure MUST NOT fall back to
partial or default values and MUST leave every asset in the document unprocessed.

#### Scenario: Misspelled field is rejected

- **WHEN** the manifest contains a top-level key spelled `assests` instead of `assets`
- **THEN** loading fails naming the unknown field, and no asset is processed

#### Scenario: Malformed JSON is rejected

- **WHEN** the manifest is not well-formed JSON
- **THEN** loading fails with a parse error identifying the location, and no partial manifest is used

### Requirement: Manifest Document Shape

The manifest SHALL be a JSON object carrying an integer `version`, a `harnesses` array naming the
harnesses assets target by default, a `sources` object mapping a source key to a repository and ref,
and an `assets` array whose entries are source strings or override objects. A field whose value has
the wrong shape SHALL be rejected naming that field.

#### Scenario: Minimal manifest loads

- **WHEN** a manifest declares version `1`, harnesses `["claude-code"]`, one source key, and an
  `assets` array of source strings
- **THEN** it loads successfully and its assets are available to later phases

#### Scenario: Assets must be an array

- **WHEN** `assets` is present but is not an array
- **THEN** loading fails with an error naming the `assets` field

### Requirement: Declared Sources Block

Every repository and ref SHALL be declared once in `sources` under a short key, and assets SHALL
reference that key rather than repeating a repository or ref. A source value SHALL name its kind,
repository and ref, as in `github:acme/assets@v1.2.0`. An asset referencing an undeclared key MUST
fail naming the key and the `sources` block, and an unrecognized source kind MUST fail naming it.

#### Scenario: Several assets share one declared source

- **WHEN** two asset strings both begin with the key `acme:` declared as
  `github:acme/assets@v1.2.0`
- **THEN** both assets resolve against that one repository and ref, declared in exactly one place

#### Scenario: Undeclared source key is rejected

- **WHEN** an asset string uses a key that `sources` does not declare
- **THEN** loading fails naming the key and pointing at the `sources` block

#### Scenario: Unrecognized source kind is rejected

- **WHEN** a `sources` entry names a kind other than `github` or `local`
- **THEN** loading fails naming the unsupported kind and the source key

### Requirement: Asset Source String Grammar

An asset entry given as a string SHALL take one of two forms: `<sourceKey>:<path>`, where the key is
declared in `sources` and the path is relative to that source's root; or a project-local path
beginning with `.harnaas/`. A string in neither form — an unprefixed relative path, an absolute path,
or a URL — MUST be rejected, naming the entry and both accepted forms.

#### Scenario: Keyed remote form is accepted

- **WHEN** an asset string is `acme:skills/review` and `acme` is declared in `sources`
- **THEN** it resolves to the path `skills/review` within that source

#### Scenario: Local form is accepted

- **WHEN** an asset string is `.harnaas/rules/house-style.md`
- **THEN** it resolves to that path beneath the project's `.harnaas` directory

#### Scenario: Unprefixed path is rejected

- **WHEN** an asset string is `skills/review` with no source key and no `.harnaas/` prefix
- **THEN** loading fails naming the entry and stating both accepted forms

### Requirement: Local Source Containment Under .harnaas

A local asset path SHALL be relative to the project root and SHALL resolve inside `.harnaas`. A path
that is absolute, or that escapes `.harnaas` by traversing upward with parent-directory segments,
MUST be rejected at load time reporting that local assets live under `.harnaas`, and its content MUST
NOT be read.

#### Scenario: Upward traversal is rejected

- **WHEN** a local asset path escapes `.harnaas` using parent-directory segments
- **THEN** loading fails reporting that local assets must live under `.harnaas`

#### Scenario: Absolute local path is rejected

- **WHEN** a local asset entry names an absolute filesystem path
- **THEN** loading fails reporting that local asset paths must be relative to the project root

### Requirement: Type And Id Inferred From The Asset Path

An asset's type and id SHALL be inferred from its path: the containing directory names the type —
`skills/` → skill, `rules/` → rule, `instructions/` → instruction, `commands/` → command, `agents/` →
persona — and the leaf names the id, with any file extension removed. A path whose containing
directory is none of those names MUST be rejected, naming the entry and directing the author to the
object form.

#### Scenario: Skill inferred from a keyed path

- **WHEN** an asset string is `acme:skills/review`
- **THEN** the asset has type `skill` and id `review`

#### Scenario: Persona inferred from an agents directory

- **WHEN** an asset string is `acme:agents/reviewer`
- **THEN** the asset has type `persona` and id `reviewer`

#### Scenario: Extension stripped from a local file asset

- **WHEN** an asset string is `.harnaas/instructions/tone.md`
- **THEN** the asset has type `instruction` and id `tone`

#### Scenario: Unrecognized containing directory is rejected

- **WHEN** an asset string is `acme:prompts/review`, whose containing directory names no type
- **THEN** loading fails naming the entry and stating that the object form must declare the type

### Requirement: Asset Object Override Form

An asset entry given as an object SHALL carry its source string plus any of `type`, `id`, `targets`
and `scope`, each overriding what the path would otherwise imply or the manifest would otherwise
default to. The object form SHALL be the only way to state a per-asset type, id, target list or
scope, and it MUST be decoded as strictly as the rest of the document.

#### Scenario: Object supplies type and id for an unconventional layout

- **WHEN** an asset object names a source path whose directory does not follow the convention and
  declares `type` and `id` explicitly
- **THEN** the declared type and id are used and no inference from the path is attempted

#### Scenario: Object narrows the target list

- **WHEN** an asset object declares `targets` while the manifest also declares `harnesses`
- **THEN** the asset's effective targets are those declared on the object

#### Scenario: Unknown field in an asset object is rejected

- **WHEN** an asset object carries a key outside the accepted set
- **THEN** loading fails naming the unknown field and the asset

### Requirement: Target Defaulting

An asset's effective target list SHALL be the `targets` declared on its object form when present, and
the manifest's `harnesses` list otherwise. An asset whose effective target list is empty MUST be
rejected, since it could never be installed anywhere. A target naming a harness harnaas does not
recognize MUST be rejected, naming it and listing the recognized ones.

#### Scenario: Targets inherit the top-level harness list

- **WHEN** the manifest lists harnesses `["claude-code"]` and an asset declares no targets
- **THEN** that asset's effective targets are `["claude-code"]`

#### Scenario: Empty effective target list is rejected

- **WHEN** an asset declares no targets and the manifest declares no harnesses
- **THEN** validation fails reporting that the asset has no target harness

#### Scenario: Unknown harness name is rejected

- **WHEN** the manifest targets a harness harnaas does not recognize
- **THEN** validation fails naming the unknown harness and listing the recognized ones

### Requirement: Scope Defaulting And User-Scope Validity

An asset's `scope` SHALL default to `project`. `user` scope SHALL be accepted only for a target
harness the harness roster records as having an unambiguous per-user location; declaring it for any
other target MUST be a validation error naming the harness, never a silent fall back to project
scope. An `instruction` asset SHALL be project scope only, and declaring `user` scope on one MUST be
rejected.

#### Scenario: Scope defaults to project

- **WHEN** an asset declares no scope
- **THEN** it installs beneath the project root of each of its target harnesses

#### Scenario: User scope accepted where the roster records a per-user location

- **WHEN** an asset declares scope `user` and targets a harness the roster records as having a
  per-user location
- **THEN** the asset is marked for that harness's per-user location

#### Scenario: User scope rejected where the roster records none

- **WHEN** an asset declares scope `user` for a target harness with no unambiguous per-user location
- **THEN** validation fails naming that harness, and the asset is not silently installed at project
  scope

#### Scenario: User-scoped instruction is refused

- **WHEN** the manifest declares an `instruction` asset at `user` scope
- **THEN** validation fails naming the asset and pointing to `rule` as the type for that intent,
  because at user scope there is no clone for the content to survive

#### Scenario: User scope on an instruction is rejected

- **WHEN** an asset of type `instruction` declares scope `user`
- **THEN** validation fails reporting that instructions are project scope only

### Requirement: Manifest Semantic Validation

Validation SHALL run over the whole decoded document and report every violation it finds rather than
stopping at the first. An id, whether inferred or declared, MUST be unique among assets of the same
type and MUST be a single safe path segment containing no path separator and no parent-directory
segment. A document with any violation MUST NOT be used by any later phase.

#### Scenario: Duplicate id within a type is rejected

- **WHEN** two assets of type `skill` resolve to the same id from different sources
- **THEN** validation fails naming the duplicated id and both entries

#### Scenario: Unsafe id is rejected

- **WHEN** a declared id contains a path separator or a parent-directory segment
- **THEN** validation fails reporting that an id must be a single safe path segment

#### Scenario: All violations are reported together

- **WHEN** a manifest contains several independent violations
- **THEN** the reported error describes every violation rather than stopping at the first

### Requirement: Manifest Version Handling

The manifest SHALL declare an integer `version`; version `1` is defined by this specification. A
version greater than harnaas understands MUST be refused with a message saying the manifest was
written by a newer harnaas and telling the user to upgrade, rather than attempting a partial read. A
missing or non-integer `version` MUST be an error.

#### Scenario: Known version loads

- **WHEN** the manifest declares version `1`
- **THEN** it is decoded according to this specification

#### Scenario: Newer version is refused with guidance

- **WHEN** the manifest declares a version greater than harnaas understands
- **THEN** loading fails saying the manifest was written by a newer harnaas and telling the user to
  upgrade, and no asset is processed

#### Scenario: Missing version is rejected

- **WHEN** the manifest omits `version`
- **THEN** loading fails reporting that the version field is required
