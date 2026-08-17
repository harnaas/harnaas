## Purpose

Defines the local asset scaffolding `harnaas init` creates: the project's `.harnaas` directory and the
asset-type directories beneath it that the selected harnesses can actually receive, each explaining
what belongs in it. It covers how the set of directories is derived from the selection, why the
scaffolding only ever adds, and the boundary that keeps it an input the author owns rather than part
of the managed set recorded in the lockfile.

## ADDED Requirements

### Requirement: Local Asset Directory Scaffolding

`harnaas init` SHALL create the project's `.harnaas` directory at the project root, and beneath it one
directory per asset type the selected harnesses can receive. The scaffolding SHALL be created only
after the manifest has been written, so a project never holds asset directories with no manifest
declaring what they are for.

#### Scenario: Scaffolding accompanies the manifest

- **WHEN** `harnaas init` completes successfully in a project with no `.harnaas` directory
- **THEN** `.harnaas` exists at the project root, holding one directory per asset type the selected
  harnesses can receive, and the manifest exists beside it

#### Scenario: Nothing is scaffolded when no manifest is written

- **WHEN** `harnaas init` ends without writing a manifest — the selection was cancelled, an existing
  manifest was refused, or no selection could be obtained
- **THEN** no `.harnaas` directory is created, and an existing one is unchanged

### Requirement: The Asset Types A Selection Earns

Every asset type SHALL be asked the same question: could an asset of this type, declaring nothing
beyond its path, reach at least one selected harness? A type whose answer is yes SHALL be scaffolded,
and a type whose answer is no SHALL NOT be. The question SHALL be answered by the same routing the
install flow uses to decide where an asset lands, so a directory is never offered for a pairing an
install would refuse, and never withheld from one an install would accept. Because a skill and an
instruction reach every recognized harness through locations no per-harness mapping computes, that
rule produces their directories for every selection harnaas can currently be given; it produces them
as a consequence rather than as an exception written into the derivation.

#### Scenario: A harness with a surface for every type earns every directory

- **WHEN** the selection names a harness whose mapping covers rules, commands and personas
- **THEN** the scaffolding holds a directory for each of skills, rules, instructions, commands and
  personas

#### Scenario: A type no selected harness can receive is not scaffolded

- **WHEN** the selection names only `devin-cli`, which has no command surface
- **THEN** no commands directory is scaffolded, while skills, rules, instructions and personas are

#### Scenario: The union of the selection decides

- **WHEN** the selection names one harness with a command surface and one without
- **THEN** a commands directory is scaffolded, because one selected harness can receive it

#### Scenario: A selected harness with no mapping earns only the shared types

- **WHEN** the selection names a harness harnaas recognizes but has no per-harness mapping for
- **THEN** that harness contributes the skills and instructions directories and no others, because a
  rule, a command and a persona reach a harness only through a mapping

#### Scenario: The derivation agrees with the install flow

- **WHEN** each asset type is asked whether it reaches each recognized harness
- **THEN** the answer matches whether the install flow would find a destination for an asset of that
  type, declaring nothing beyond its path, targeting that harness

### Requirement: Scaffolded Directory Names Are The Names Type Inference Reads

Each scaffolded directory SHALL be named with the exact path segment harnaas infers that asset type
from, so a file placed in one and declared in the manifest by its path is inferred as the type the
author intended without declaring the type explicitly.

#### Scenario: A file placed in a scaffolded directory infers its type

- **WHEN** an author places a file in a scaffolded asset-type directory and declares it in the
  manifest by its path under `.harnaas`
- **THEN** the declared asset's type is inferred as the type that directory is for, with no `type`
  field written

### Requirement: Each Scaffolded Directory Explains Itself

`harnaas init` SHALL write a `README.md` into each directory it creates, naming the asset type, what
belongs in the directory, and the manifest line that declares an asset from it. The file SHALL be the
author's from the moment it is written: no harnaas command rewrites, reformats, relocates or deletes
it, and no command depends on its content.

#### Scenario: A created directory carries an explanation

- **WHEN** `harnaas init` creates an asset-type directory
- **THEN** that directory holds a `README.md` naming the asset type, describing what belongs there,
  and showing the manifest entry that declares an asset from it

#### Scenario: The explanation survives a clone

- **WHEN** the scaffolding is committed and the repository is cloned elsewhere
- **THEN** every scaffolded directory is present in the clone, because each holds a tracked file

#### Scenario: No command depends on the explanation

- **WHEN** an author edits or deletes a scaffolded `README.md`
- **THEN** `harnaas install` and `harnaas lint` behave exactly as they would have otherwise, and
  neither reports a finding about it

### Requirement: Scaffolding Only Ever Adds

Scaffolding SHALL create what is missing and change nothing that exists. A directory already present
SHALL be left as it is, a `README.md` already present SHALL NOT be overwritten, and no file or
directory under `.harnaas` SHALL be modified, moved or removed. This SHALL hold on every run,
including a forced one, and including a run whose selection no longer covers a directory an earlier
run created.

#### Scenario: Existing content is untouched

- **WHEN** `harnaas init` scaffolds into a `.harnaas` directory that already holds asset files and
  explanations
- **THEN** every existing file is byte-for-byte unchanged, none is removed, and only the missing
  directories and their explanations are added

#### Scenario: A narrower selection removes nothing

- **WHEN** `harnaas init` is run again with a selection that no longer covers an asset type an earlier
  run scaffolded
- **THEN** the directory for that type and everything in it remain

#### Scenario: Re-running completes a partial scaffolding

- **WHEN** `harnaas init` is run in a project where some scaffolded directories exist and others do
  not
- **THEN** the missing ones are created, the present ones are untouched, and the outcome is the same
  as a first run into an empty project

### Requirement: Scaffolding Is Not Managed Content

Nothing `harnaas init` scaffolds SHALL be recorded in the lockfile, reported as an installed
destination, covered by a managed ignore-file entry, or overwritten by a later command. The `.harnaas`
directory is content harnaas reads and never a destination it writes to, so the scaffolding SHALL
carry no ownership claim of any kind.

#### Scenario: The lockfile records nothing about the scaffolding

- **WHEN** `harnaas install` runs in a project whose scaffolding is empty of assets
- **THEN** no scaffolded directory or explanation appears in the lockfile, and the install reports no
  destination under `.harnaas`

#### Scenario: The ignore file is not extended to cover the scaffolding

- **WHEN** `harnaas install` maintains the project's managed ignore-file entries
- **THEN** no entry covering `.harnaas` or anything beneath it is added, because the scaffolding is
  source content a team commits

#### Scenario: Lint reports nothing about an empty scaffolding

- **WHEN** `harnaas lint` runs in a project whose scaffolded directories hold no declared assets
- **THEN** it reports no finding about the scaffolding, and the directories' presence changes neither
  its findings nor its exit status

### Requirement: Scaffolding Is Reported

`harnaas init` SHALL report the local asset directories it created, and MUST NOT report as created a
directory that was already there. The report SHALL name the paths relative to the project root, in the
same order every run.

#### Scenario: Created directories are named

- **WHEN** `harnaas init` creates local asset directories
- **THEN** it prints each created directory's path relative to the project root, in a deterministic
  order

#### Scenario: Pre-existing directories are not claimed

- **WHEN** `harnaas init` scaffolds into a project where some of the directories already exist
- **THEN** only the directories it actually created are reported as created

### Requirement: A Failed Scaffolding Leaves A Usable Project

Where the manifest was written and the scaffolding then fails, `harnaas init` SHALL fail naming the
path it could not create and the reason, SHALL leave the written manifest in place, and SHALL name it
so the user knows the project is initialized. A subsequent run SHALL be able to complete the
scaffolding.

#### Scenario: A scaffolding failure keeps the manifest

- **WHEN** the manifest is written and creating a local asset directory then fails
- **THEN** the command exits non-zero naming the path and the reason, the manifest remains at the
  project root, and the message says the manifest was created

#### Scenario: A later run completes what failed

- **WHEN** the condition that caused the scaffolding to fail is resolved and `harnaas init` is run
  again with the force flag
- **THEN** the missing directories are created and the existing ones are untouched
