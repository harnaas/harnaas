## Purpose

Defines `harnaas init`, the command that brings a project under `harnaas` management by creating its
`harnaas.json`. It covers what the scaffolded manifest contains, how target harnesses are detected
and confirmed, how an existing manifest is protected from being overwritten, and the strict limits on
what else the command is allowed to touch.

## ADDED Requirements

### Requirement: Manifest Scaffolding

`harnaas init` SHALL create a `harnaas.json` at the project root declaring the current schema version,
the selected target harnesses, and an empty asset list. The generated file SHALL be formatted for
human editing, and MUST be valid input to the manifest loader without further modification.

#### Scenario: Manifest is created in an uninitialized project

- **WHEN** the user runs `harnaas init` in a project with no `harnaas.json`
- **THEN** a `harnaas.json` is written at the project root declaring version `1`, the selected
  harnesses, and an empty `assets` array

#### Scenario: Generated manifest loads cleanly

- **WHEN** a manifest produced by `harnaas init` is subsequently loaded
- **THEN** it decodes and validates without error under strict decoding

#### Scenario: Init reports what it created

- **WHEN** `harnaas init` completes successfully
- **THEN** it prints the path of the created manifest and the next command to run

### Requirement: Restricted Side Effects

`harnaas init` SHALL write exactly one file, `harnaas.json`, and nothing else. It MUST NOT create or
modify any harness directory, MUST NOT create the project's `.harnaas` directory, and MUST NOT edit
version-control ignore files. Any guidance about ignoring installed paths SHALL be printed as advice
rather than applied.

#### Scenario: Ignore file is left untouched

- **WHEN** the user runs `harnaas init` in a project containing a version-control ignore file
- **THEN** that ignore file is byte-for-byte unchanged after the command completes

#### Scenario: No harness directories are created

- **WHEN** `harnaas init` selects a harness that has no directory in the project yet
- **THEN** no harness directory is created and only `harnaas.json` appears on disk

### Requirement: Overwrite Protection

When a `harnaas.json` already exists, `harnaas init` SHALL refuse to replace it and exit non-zero,
unless the user explicitly passes a force flag. The refusal message MUST name the flag that would
allow the overwrite. When forced, the existing file SHALL be replaced in full.

#### Scenario: Existing manifest is preserved by default

- **WHEN** the user runs `harnaas init` in a project that already has a `harnaas.json`
- **THEN** the existing file is unchanged, the CLI reports that a manifest already exists and names
  the force flag, and the process exits non-zero

#### Scenario: Force replaces the manifest

- **WHEN** the user runs `harnaas init` with the force flag in a project that already has a manifest
- **THEN** the existing manifest is replaced with a freshly scaffolded one

### Requirement: Harness Detection

`harnaas init` SHALL detect which supported harnesses are already present in the project and offer
them as the default selection. Detection SHALL be based on observable evidence in the project rather
than on configuration the user has not yet written. When no harness is detected, the command SHALL
fall back to the CLI's default harness rather than producing a manifest with no targets.

#### Scenario: Present harness is preselected

- **WHEN** the project already contains a recognized harness directory
- **THEN** that harness is preselected as a target in the scaffolded manifest

#### Scenario: No harness detected falls back to the default

- **WHEN** no recognized harness is present in the project
- **THEN** the scaffolded manifest targets the CLI's default harness and the command says which one
  it chose and why

### Requirement: Interactive And Non-Interactive Selection

`harnaas init` SHALL confirm the harness selection through an accessible interactive prompt when a
terminal is available. It SHALL accept the same selection through command-line flags, and when no
terminal is available or the user requests non-interactive operation, it SHALL proceed with the
flag-supplied or detected values without blocking on input.

#### Scenario: Non-interactive run completes without prompting

- **WHEN** `harnaas init` is run with output piped and no terminal attached
- **THEN** it scaffolds the manifest from detected and flag-supplied values and exits without
  prompting

#### Scenario: Assume-yes skips the prompt

- **WHEN** the user runs `harnaas init` with the assume-yes flag on a terminal
- **THEN** the detected selection is accepted without a prompt

#### Scenario: Explicit harness flag overrides detection

- **WHEN** the user names target harnesses with a flag
- **THEN** those harnesses are used regardless of what detection found

#### Scenario: Cancelled prompt writes nothing

- **WHEN** the user aborts the interactive prompt
- **THEN** no `harnaas.json` is written and the process exits non-zero

### Requirement: Manifest Write Durability

The scaffolded manifest SHALL be written atomically, so an interrupted run leaves either the previous
state or the complete new file, never a partially written manifest. A failure during writing MUST NOT
leave a truncated `harnaas.json` in place.

#### Scenario: Interrupted write leaves no partial file

- **WHEN** the process is interrupted while `harnaas init` is writing the manifest
- **THEN** the project contains either no manifest or a complete, loadable one, and never a truncated
  file
