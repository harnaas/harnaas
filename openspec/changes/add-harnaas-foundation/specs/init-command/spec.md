## Purpose

Defines `harnaas init`, the command that brings a project under harnaas management by scaffolding its
manifest. It covers the shape of the generated `harnaas.json`, how present harnesses are detected and
pre-filled, how an existing manifest is protected, and the strict limit that init touches nothing else
on disk — every other file is `harnaas install`'s to write.

## ADDED Requirements

### Requirement: Manifest Scaffolding

`harnaas init` SHALL create a `harnaas.json` at the project root containing the current schema
version, a `harnesses` list holding the selected target harnesses, an empty `sources` map, and an
empty `assets` array. The generated manifest SHALL be formatted for hand editing, and MUST decode and
validate without further modification under the manifest loader's strict decoding.

#### Scenario: Manifest is created in an uninitialized project

- **WHEN** the user runs `harnaas init` in a project with no `harnaas.json`
- **THEN** a `harnaas.json` is written at the project root declaring version `1`, the selected
  harnesses, an empty `sources` map, and an empty `assets` array

#### Scenario: Generated manifest loads cleanly

- **WHEN** a manifest produced by `harnaas init` is subsequently loaded
- **THEN** it decodes and validates without error under strict decoding, and describes a project with
  no declared sources and no declared assets

#### Scenario: Init reports what it created

- **WHEN** `harnaas init` completes successfully
- **THEN** it prints the path of the created manifest and the next command to run

### Requirement: Harness Detection Pre-Fills The Target List

`harnaas init` SHALL detect which supported harnesses are already present in the project and pre-fill
the manifest's `harnesses` list with them. Detection SHALL rest on observable evidence in the project
rather than on configuration the user has not yet written. When no harness is detected, the command
SHALL fall back to the CLI's default harness and state which one it chose and why, rather than writing
a manifest with an empty `harnesses` list.

#### Scenario: Present harness pre-fills the list

- **WHEN** the project already contains a recognized harness's own directory
- **THEN** that harness appears in the `harnesses` list of the scaffolded manifest

#### Scenario: Several present harnesses are all pre-filled

- **WHEN** the project contains directories for more than one recognized harness
- **THEN** every detected harness appears in the `harnesses` list, in a deterministic order

#### Scenario: No harness detected falls back to the default

- **WHEN** no recognized harness is present in the project
- **THEN** the scaffolded manifest lists the CLI's default harness, and the command says which one it
  chose and that nothing was detected

### Requirement: Single-File Side Effects

`harnaas init` SHALL write exactly one file, `harnaas.json`, and nothing else. It MUST NOT create or
modify any harness directory, the project's `.harnaas` directory, the project's `AGENTS.md`, its
`CLAUDE.md`, or its version-control ignore file. Every destination beyond the manifest SHALL be left
for `harnaas install` to create.

#### Scenario: Ignore file is left untouched

- **WHEN** the user runs `harnaas init` in a project containing a version-control ignore file
- **THEN** that ignore file is byte-for-byte unchanged after the command completes, and no ignore file
  is created if none existed

#### Scenario: Memory and bridge files are left untouched

- **WHEN** the user runs `harnaas init` in a project containing `AGENTS.md` or `CLAUDE.md`
- **THEN** both files are byte-for-byte unchanged, and neither is created if absent

#### Scenario: No harness directories are created

- **WHEN** `harnaas init` pre-fills a harness that has no directory in the project yet
- **THEN** no harness directory is created and only `harnaas.json` appears on disk

#### Scenario: The local asset directory is not created

- **WHEN** `harnaas init` completes in a project with no `.harnaas` directory
- **THEN** no `.harnaas` directory exists afterwards

### Requirement: Setup Guidance Is Advice Only

`harnaas init` SHALL communicate any remaining setup — ignoring installed paths, creating local asset
directories, populating the manifest — as printed guidance naming the command that performs it. It
MUST NOT perform that setup itself, and MUST NOT offer a flag that makes init perform it.

#### Scenario: Ignore guidance is printed, not applied

- **WHEN** `harnaas init` finishes scaffolding a manifest
- **THEN** any advice about ignoring installed paths is printed as text naming `harnaas install`, and
  the ignore file on disk is unchanged

### Requirement: Overwrite Protection

When a `harnaas.json` already exists at the project root, `harnaas init` SHALL refuse to replace it
and exit non-zero, unless the user explicitly passes a force flag. The refusal message MUST name the
flag that would allow the overwrite. When forced, the existing manifest SHALL be replaced in full.

#### Scenario: Existing manifest is preserved by default

- **WHEN** the user runs `harnaas init` in a project that already has a `harnaas.json`
- **THEN** the existing file is unchanged, the CLI reports that a manifest already exists and names
  the force flag, and the process exits non-zero

#### Scenario: Force replaces the manifest

- **WHEN** the user runs `harnaas init` with the force flag in a project that already has a manifest
- **THEN** the existing manifest is replaced in full with a freshly scaffolded one

### Requirement: Interactive And Non-Interactive Selection

`harnaas init` SHALL confirm the harness selection through an accessible interactive prompt when a
terminal is available, and SHALL accept the same selection through command-line flags so the whole
command is completable without a prompt. With no terminal attached, or when the user asks for
non-interactive operation, it SHALL proceed from the flag-supplied and detected values without
blocking on input. If the user cancels the prompt, it SHALL write nothing and exit non-zero.

#### Scenario: Non-interactive run completes without prompting

- **WHEN** `harnaas init` is run with output piped and no terminal attached
- **THEN** it scaffolds the manifest from the detected and flag-supplied harnesses and exits without
  prompting

#### Scenario: Assume-yes skips the prompt

- **WHEN** the user runs `harnaas init` with the assume-yes flag on a terminal
- **THEN** the pre-filled selection is accepted without a prompt

#### Scenario: Explicit harness flag overrides detection

- **WHEN** the user names target harnesses with a flag
- **THEN** exactly those harnesses fill the manifest's `harnesses` list, regardless of what detection
  found

#### Scenario: Unknown harness name is rejected

- **WHEN** the user names a harness the CLI does not support
- **THEN** the command fails naming the unsupported harness and the supported ones, and no manifest is
  written

#### Scenario: Cancelled prompt writes nothing

- **WHEN** the user aborts the interactive prompt
- **THEN** no `harnaas.json` is written and the process exits non-zero

### Requirement: Manifest Write Durability

The scaffolded manifest SHALL be written atomically, so an interrupted run leaves either the previous
state or the complete new file. A failure while writing MUST NOT leave a truncated or partially
written `harnaas.json` in place, and MUST NOT leave staging files behind.

#### Scenario: Interrupted write leaves no partial file

- **WHEN** the process is interrupted while `harnaas init` is writing the manifest
- **THEN** the project contains either no manifest or a complete, loadable one, and never a truncated
  file

#### Scenario: Failed forced overwrite preserves the old manifest

- **WHEN** a forced `harnaas init` fails partway through writing over an existing manifest
- **THEN** the previous `harnaas.json` is still intact and no staging file remains
