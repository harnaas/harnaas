## Purpose

Defines Devin CLI as a harness harnaas recognizes: the id a manifest writes, how a project is
detected as already using it, where a rule and a persona land beneath the harness's own directory,
what happens to the type it has no surface for, and the shared routes a skill and an instruction
already take to reach it. It also covers Devin CLI's split per-user layout and how the roster and the
named adapter answer it differently, the adapter's own shape, and what lint reports about a
destination beneath the harness's directory.

## ADDED Requirements

### Requirement: Devin CLI Is A Recognized Harness

The roster SHALL recognize Devin CLI under the id `devin-cli`, with the display name `Devin CLI`.
That id SHALL be accepted wherever a harness is named — the manifest's `harnesses` list, an asset's
`targets`, and the `init` flag that replaces detection — and SHALL appear in the list of recognized
ids that every unknown-harness diagnostic prints. The display name MUST NOT be accepted where an id
is expected. Adding this harness MUST NOT change which harness `init` falls back to when a project
shows evidence of none.

#### Scenario: A manifest targets Devin CLI

- **WHEN** a manifest names `devin-cli` in its `harnesses` list or in an asset's `targets`
- **THEN** the manifest validates and the asset is planned for that harness

#### Scenario: An unknown name lists Devin CLI among the recognized ids

- **WHEN** a manifest names a harness harnaas does not recognize
- **THEN** the diagnostic names the entry that is wrong and lists both `claude-code` and `devin-cli`
  as ids it would accept

#### Scenario: A display name is still not an id

- **WHEN** a manifest writes `Devin CLI` where a harness id belongs
- **THEN** it is reported as an unrecognized harness rather than resolved to `devin-cli`

### Requirement: Devin CLI Project Detection

The presence of Devin CLI's own configuration directory at the project root SHALL be the whole of the
evidence that a project already uses this harness, and `harnaas init` SHALL pre-fill a manifest
targeting it when that evidence is present. The project's memory file MUST NOT count as evidence,
because most recognized harnesses read it and it therefore proves nothing about this one. Where a
project shows evidence of more than one harness, the scaffolded list SHALL be ordered the same way on
every run and every machine. Detection SHALL NOT follow a symbolic link when deciding whether the
evidence is present, because the question is whether the harness left something behind rather than
whether it resolves.

#### Scenario: A project holding a Devin CLI directory

- **WHEN** `harnaas init` runs in a project whose root holds Devin CLI's configuration directory
- **THEN** the scaffolded manifest targets `devin-cli` and says the harness was detected

#### Scenario: A project holding only a memory file

- **WHEN** `harnaas init` runs in a project whose root holds the shared memory file and no harness
  directory
- **THEN** Devin CLI is not reported as detected and the scaffolded manifest does not target it

#### Scenario: A project using both harnesses

- **WHEN** `harnaas init` runs in a project showing evidence of both recognized harnesses
- **THEN** the scaffolded list holds both ids in the roster's own order, identically on every run

### Requirement: Devin CLI Rule And Persona Destinations

A `rule` targeting Devin CLI SHALL install at `.devin/rules/<id>.md` and a `persona` at
`.devin/agents/<id>.md`, each relative to the resolved scope root and each taking the asset id as its
file stem, so a destination is predictable from the manifest alone. A persona SHALL install as a
single file rather than as a directory holding one, because the harness reads both spellings and the
flat one is the destination a reader can find from the report. The installed bytes SHALL equal the
resolved source bytes exactly, including frontmatter, line endings and trailing whitespace: harnaas
does not translate one harness's frontmatter vocabulary into another's, and a document whose quoting
carries meaning MUST NOT be re-encoded.

#### Scenario: A rule installs into the harness's rules directory

- **WHEN** a `rule` asset targets `devin-cli` at project scope
- **THEN** it is written to `.devin/rules/<id>.md` beneath the project root and reported at that path

#### Scenario: A persona installs as a flat file

- **WHEN** a `persona` asset targets `devin-cli`
- **THEN** it is written to `.devin/agents/<id>.md` rather than to a directory named for the asset

#### Scenario: The installed bytes equal the source bytes

- **WHEN** a rule authored with frontmatter keys Devin CLI does not read is installed for it
- **THEN** the file lands byte for byte as resolved, with no key added, removed or rewritten

### Requirement: Devin CLI Has No Command Surface

Devin CLI exposes no directory for commands — what a user invokes by name there is a skill — so a
`command` targeting `devin-cli` MUST be reported unsupported and nothing SHALL be written for that
pairing. The asset's other targets SHALL still install, and the run MUST NOT fail because one pairing
had nowhere to go. The reported reason SHALL name the missing surface and MUST NOT state that the
harness has no skill surface, which is not true of this harness.

#### Scenario: A command targeting only Devin CLI

- **WHEN** a `command` asset targets `devin-cli` and no other harness
- **THEN** the pairing is reported unsupported with its reason, no file is written, and the run
  succeeds

#### Scenario: A command targeting both harnesses

- **WHEN** a `command` asset targets both `claude-code` and `devin-cli`
- **THEN** it installs natively for `claude-code` and is reported unsupported for `devin-cli`

### Requirement: Skills Reach Devin CLI Through The Shared Directory

Devin CLI reads the shared skills directory, so a `skill` targeting it SHALL install there once and
MUST NOT be copied into the harness's own directory as well. A skill targeting Devin CLI and another
harness that also reads the shared directory SHALL produce exactly one written destination, recorded
as claimed by both. Removing one of those harnesses from the asset's targets MUST NOT remove the
shared file while another recorded harness still claims it.

#### Scenario: A skill targeting both harnesses is written once

- **WHEN** a `skill` asset targets both `claude-code` and `devin-cli`
- **THEN** one shared skill destination is written and no per-harness copy is made

#### Scenario: A skill targeting only Devin CLI

- **WHEN** a `skill` asset targets `devin-cli` alone
- **THEN** it installs to the shared skills directory rather than beneath `.devin`

#### Scenario: Dropping one harness leaves the shared skill

- **WHEN** `devin-cli` is removed from a skill's targets while `claude-code` still claims the same
  destination
- **THEN** the shared file stays and only the attribution to `devin-cli` is dropped

### Requirement: Instructions Reach Devin CLI Through The Memory File

Devin CLI reads the project's shared memory file directly, so an `instruction` targeting it SHALL
reach it through the managed block that already exists in that file, and harnaas MUST NOT add an
import line, a bridge file or a second copy for this harness. A project that targets Devin CLI and no
other harness SHALL still receive the managed block, and the existing bridge line that another
harness needs MUST behave exactly as it did before this harness was recognized.

#### Scenario: An instruction targeting only Devin CLI

- **WHEN** an `instruction` asset targets `devin-cli` alone
- **THEN** its content appears in the memory file's managed block and no file is created beneath
  `.devin`

#### Scenario: The bridge line is unaffected

- **WHEN** a project's instructions are installed for `devin-cli` alongside the harness that needs
  the bridge line
- **THEN** exactly one bridge line exists, as it did before `devin-cli` was recognized

### Requirement: Devin CLI Per-User Layout

Devin CLI keeps its per-user rules under one directory and its remaining per-user content under
another, and spells the second differently per operating system, so it has no single per-user root a
destination could be counted from. The roster SHALL nevertheless record that this harness has a
per-user location, because a skill installed at `user` scope reaches it through the shared per-user
skills directory; `user` scope SHALL therefore be accepted for it during manifest validation. The
named adapter SHALL offer no per-user root, so a user-scoped `rule` or `persona` targeting Devin CLI
MUST be reported unsupported naming the asset, the harness and the scope, and MUST NOT be installed
beneath a guessed directory or quietly demoted to project scope.

#### Scenario: A user-scoped skill installs

- **WHEN** a `skill` asset declares `user` scope and targets `devin-cli`
- **THEN** it installs to the shared per-user skills directory beneath the user's home

#### Scenario: A user-scoped rule is refused by name

- **WHEN** a `rule` asset declares `user` scope and targets `devin-cli`
- **THEN** the pairing is reported unsupported naming the asset, the harness and the scope, and
  nothing is written anywhere

#### Scenario: A user-scoped instruction is still refused for what it is

- **WHEN** an `instruction` asset declares `user` scope and targets `devin-cli`
- **THEN** it is refused during manifest validation for being an instruction outside a committed
  file, not for anything about this harness

### Requirement: The Devin CLI Adapter Answers And Does Nothing Else

The named adapter for Devin CLI SHALL register itself under `devin-cli`, answer only the questions
the adapter contract asks, and perform no filesystem, network or environment access of its own. Its
detection SHALL read the roster's declared evidence rather than a second list kept beside it, so that
`init` scaffolding a manifest and an install reporting a harness can never disagree about one
project. A `skill` and an `instruction` MUST be absent from its surface table, because both reach
this harness through shared locations and an adapter answering for them would be a second place
harnaas decides where they land. Registering this adapter MUST NOT change the destination of any
asset that does not target `devin-cli`.

#### Scenario: Detection reads the roster's evidence

- **WHEN** the roster's declared evidence for Devin CLI changes
- **THEN** both `init` detection and the adapter's own detection change with it, with no second list
  to update

#### Scenario: Detection reports absence rather than failure

- **WHEN** the adapter is asked whether a project uses Devin CLI and no evidence is present
- **THEN** it reports absence, and a harness that is simply not installed is never an error

#### Scenario: The shared types have no surface here

- **WHEN** the adapter is asked for the destination of a `skill` or an `instruction`
- **THEN** it reports having no surface for that type rather than returning a path beneath `.devin`

### Requirement: Devin CLI Destinations Are Checked Like Any Other

`harnaas lint` SHALL report drift, a missing destination, an unaccounted file and an unmanaged
conflict beneath Devin CLI's directory exactly as it does for any other harness, deriving the scope
root the same way install did so the two can never disagree about where an asset landed. Lint MUST
NOT enumerate the harness roster to do this, and recognizing a second harness MUST NOT change any
finding for a project that targets only the first.

#### Scenario: A hand-written file at a Devin CLI destination

- **WHEN** a file harnaas did not install already occupies a destination a Devin CLI asset would take
- **THEN** lint reports it as an unmanaged conflict and install refuses to overwrite it on any flag

#### Scenario: An edited installed rule

- **WHEN** an installed rule beneath `.devin` no longer matches the digest recorded for it
- **THEN** lint reports drift naming that file and the command that repairs it

#### Scenario: A project targeting only the first harness

- **WHEN** lint runs against a project whose manifest never names `devin-cli`
- **THEN** it reports exactly what it reported before this harness was recognized
