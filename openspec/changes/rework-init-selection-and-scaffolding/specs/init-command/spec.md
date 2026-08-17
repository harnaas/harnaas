## ADDED Requirements

### Requirement: Roster Presentation In The Selection

Where `harnaas init` presents a selection, it SHALL list every harness harnaas recognizes, in the
roster's own order, showing each one's display name and the id the manifest will hold. No harness
SHALL be pre-selected, omitted, or reordered on the basis of what the project contains. The selection
SHALL accept more than one harness, and SHALL refuse to complete with none chosen, because a manifest
with an empty `harnesses` list declares assets and guarantees them for nothing.

#### Scenario: Every recognized harness is offered

- **WHEN** `harnaas init` presents its harness selection
- **THEN** every harness on the roster appears in the list, in the roster's order, each with its
  display name and the id that will be written to the manifest

#### Scenario: A project's contents do not change the list

- **WHEN** `harnaas init` presents its harness selection in a project that already contains one
  recognized harness's own directory
- **THEN** the list, its order and its selected entries are identical to those presented in a project
  containing no harness at all

#### Scenario: Several harnesses can be chosen at once

- **WHEN** the user selects more than one harness
- **THEN** the scaffolded manifest's `harnesses` list holds every selected harness, in the roster's
  order and with no duplicates

#### Scenario: An empty selection cannot be submitted

- **WHEN** the user attempts to complete the selection with no harness chosen
- **THEN** the selection is refused with a message saying at least one harness is required, and no
  manifest is written

### Requirement: Bounded Side Effects

`harnaas init` SHALL write the manifest and the project's local asset scaffolding, and nothing else.
It MUST NOT create or modify any harness directory, the project's `AGENTS.md`, its `CLAUDE.md`, or its
version-control ignore file. Every destination a harness reads SHALL be left for `harnaas install` to
create and record.

#### Scenario: Ignore file is left untouched

- **WHEN** the user runs `harnaas init` in a project containing a version-control ignore file
- **THEN** that ignore file is byte-for-byte unchanged after the command completes, and no ignore file
  is created if none existed

#### Scenario: Memory and bridge files are left untouched

- **WHEN** the user runs `harnaas init` in a project containing `AGENTS.md` or `CLAUDE.md`
- **THEN** both files are byte-for-byte unchanged, and neither is created if absent

#### Scenario: No harness directories are created

- **WHEN** `harnaas init` scaffolds a manifest naming a harness that has no directory in the project
- **THEN** no harness directory is created, and the only paths that appear are the manifest and the
  local asset scaffolding

## MODIFIED Requirements

### Requirement: Interactive And Non-Interactive Selection

`harnaas init` SHALL take the manifest's `harnesses` list from the user, and never from the project's
own contents. Where a prompt may be shown, it SHALL present the recognized harnesses through an
accessible interactive selection. Where a prompt may not be shown, it SHALL take the selection from
the harness flag, so the whole command stays completable without a terminal. A run that can neither
prompt nor read a flag-supplied selection SHALL write nothing and exit non-zero, naming the flag and
every recognized harness id. If the user cancels the selection, it SHALL write nothing and exit
non-zero. The command MUST NOT offer a flag that accepts a selection the user never made.

#### Scenario: Terminal run presents the selection

- **WHEN** `harnaas init` is run with a terminal attached and no harness flag
- **THEN** it presents the harness selection and scaffolds from what the user chose

#### Scenario: Harness flag supplies the selection without prompting

- **WHEN** the user names target harnesses with the harness flag
- **THEN** exactly those harnesses fill the manifest's `harnesses` list, no selection is presented,
  and the run completes without reading from the terminal

#### Scenario: No terminal and no flag is refused

- **WHEN** `harnaas init` is run with output piped and no terminal attached, and no harness flag is
  given
- **THEN** nothing is written, the process exits non-zero, and the message names the harness flag and
  every recognized harness id

#### Scenario: Unknown harness name is rejected

- **WHEN** the user names a harness the CLI does not support
- **THEN** the command fails naming the unsupported harness and the supported ones, and no manifest is
  written

#### Scenario: Cancelled selection writes nothing

- **WHEN** the user aborts the interactive selection
- **THEN** no `harnaas.json` is written, no local asset scaffolding is created, and the process exits
  non-zero

#### Scenario: A flag that confirms an unmade selection is not accepted

- **WHEN** the user passes an assume-yes flag to `harnaas init`
- **THEN** the command fails reporting an unknown flag, and no manifest is written

### Requirement: Setup Guidance Is Advice Only

`harnaas init` SHALL communicate any remaining setup it does not perform — ignoring installed paths,
populating the manifest, installing the declared assets — as printed guidance naming the command that
performs it. It MUST NOT perform that setup itself, and MUST NOT offer a flag that makes init perform
it. Guidance MUST name only commands that actually perform the step described.

#### Scenario: Ignore guidance is printed, not applied

- **WHEN** `harnaas init` finishes scaffolding a manifest
- **THEN** any advice about ignoring installed paths is printed as text naming `harnaas install`, and
  the ignore file on disk is unchanged

#### Scenario: Guidance does not attribute work to a command that does not do it

- **WHEN** `harnaas init` prints its closing guidance
- **THEN** every step it attributes to another command is a step that command performs, and the local
  asset directories init has just created are not described as work left for `harnaas install`

### Requirement: Overwrite Protection

When a `harnaas.json` already exists at the project root, `harnaas init` SHALL refuse to replace it
and exit non-zero, unless the user explicitly passes a force flag. The refusal message MUST name the
flag that would allow the overwrite. When forced, the existing manifest SHALL be replaced in full. The
force flag SHALL apply to the manifest alone and MUST NOT authorize replacing, rewriting or removing
anything under the project's local asset directory.

#### Scenario: Existing manifest is preserved by default

- **WHEN** the user runs `harnaas init` in a project that already has a `harnaas.json`
- **THEN** the existing file is unchanged, the CLI reports that a manifest already exists and names
  the force flag, and the process exits non-zero

#### Scenario: Force replaces the manifest

- **WHEN** the user runs `harnaas init` with the force flag in a project that already has a manifest
- **THEN** the existing manifest is replaced in full with a freshly scaffolded one

#### Scenario: Force does not reach the local asset directory

- **WHEN** the user runs `harnaas init` with the force flag in a project whose local asset directory
  already holds content
- **THEN** every existing file under that directory is byte-for-byte unchanged and none is removed

## REMOVED Requirements

### Requirement: Harness Detection Pre-Fills The Target List

**Reason**: The manifest's `harnesses` list is a guarantee about which harnesses the declared assets
are supported on, and detection answers a different question — which harnesses have left a file in
this working tree. The two disagree in both directions with nothing to show for it: a `.claude`
directory exists in repositories that guarantee nothing, a team adopting a harness has left no
evidence of it yet and is handed the roster's default instead, and a shallow checkout or an ignored
configuration directory changes the answer for one project depending on when init ran. The user was
shown a pre-filled sentence to confirm rather than the roster to choose from, so the alternatives were
never visible at the one moment they mattered.

**Migration**: Choose the harnesses in the selection `harnaas init` now presents, or name them with
`--harness <id>`, repeating the flag per harness. A non-interactive run that relied on detection —
including one that relied on the default when detection found nothing — must now pass `--harness`;
the refusal it receives otherwise names the flag and every recognized id. Detection itself is not
gone from harnaas: the roster still records each harness's project evidence, and the adapters still
detect through it when reporting an install.

### Requirement: Single-File Side Effects

**Reason**: The rule protected a real property — init must not create the destinations `harnaas
install` records ownership of, because anything init created there would be unmanaged and the next
install would report a conflict against init's own output — but it stated that property as a file
count. The project's local asset directory is an input harnaas reads and never a destination it
writes, so it can never be part of the managed set, and scaffolding it claims nothing. Stated as "one
file", the rule also forbade the one piece of setup init is uniquely placed to get right, and left
init printing guidance that attributed the work to a command which does not do it.

**Migration**: The narrower rule replacing it is `Bounded Side Effects`, which forbids the same
destinations by naming them: no harness directory, no `AGENTS.md`, no `CLAUDE.md` and no ignore file.
A project that wants no local asset scaffolding can delete the created directories; init does not
recreate them unless it is run again.
