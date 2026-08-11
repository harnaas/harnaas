## Purpose

Defines `harnaas install`, the command that makes the filesystem match the manifest. It covers the
phased flow from resolution to recording, the outcome reported for each asset and target, how
hand-written and drifted files are protected, how the installed set converges on the manifest, how the
version-control ignore file is maintained, and the atomicity, ordering and failure semantics that make
the command safe to re-run and safe to run in CI.

## ADDED Requirements

### Requirement: Phased Install Flow

`harnaas install` SHALL proceed in distinct phases: resolve every declared source, compute a plan of
the changes each asset and target implies, apply that plan, and record the result. No filesystem
change to any harness destination may occur before the plan is complete, so a failure during
resolution leaves the harness untouched.

#### Scenario: Resolution failure leaves the harness untouched

- **WHEN** one declared source fails to resolve and resolution is still in progress
- **THEN** no harness destination has been modified and the command reports the resolution failure

#### Scenario: Plan precedes any write

- **WHEN** the command runs successfully
- **THEN** every change applied corresponds to an entry computed during the planning phase

### Requirement: Per-Target Outcome Reporting

Each asset and target combination SHALL produce exactly one outcome drawn from a fixed set of seven:
`created`, `updated`, `unchanged`, `emulated`, `conflict-unmanaged`, `conflict-drift`, and
`unsupported`. The command SHALL report every outcome, and any outcome that blocked or altered an
install SHALL carry a runnable remedy naming the command or manifest edit that resolves it.

#### Scenario: Every target reports one of the seven outcomes

- **WHEN** the command completes
- **THEN** each asset and target combination appears exactly once in the report carrying exactly one
  of the seven defined outcomes

#### Scenario: Blocked outcome carries a runnable remedy

- **WHEN** a target reports `conflict-unmanaged`, `conflict-drift` or `unsupported`
- **THEN** the report states which condition applied and gives the command or edit that would resolve
  it

#### Scenario: Unchanged targets are still reported

- **WHEN** an asset is already installed and matches its source
- **THEN** it is reported as `unchanged` rather than omitted

### Requirement: Emulated Installation Outcome

An asset installed through another asset type's surface because the harness has no surface of its own
SHALL be reported as `emulated` and never as `created` or `updated`, and the report SHALL name the
surface used and the behaviour that differs. An asset that cannot be delivered through another surface
without changing its semantics SHALL be reported `unsupported` instead of being emulated silently.

#### Scenario: Command delivered through a skill surface

- **WHEN** a `command` asset targets a harness with no command surface and is installed as a skill
- **THEN** the outcome is `emulated` and the report states that the harness will not invoke it on its
  own initiative

#### Scenario: Semantics-changing emulation is refused

- **WHEN** an asset could only be emulated by dropping or broadening its declared scoping
- **THEN** the outcome is `unsupported` rather than `emulated`, and nothing is written for that target

### Requirement: Plan Preview Without Side Effects

The command SHALL support a dry-run mode that computes and prints the full plan and then exits without
writing to any harness destination, the lockfile, the memory file, or the version-control ignore file.
Dry-run output SHALL be the same set of outcomes a real run would report.

#### Scenario: Dry run writes nothing

- **WHEN** the command runs in dry-run mode
- **THEN** no harness destination, lockfile, memory file or ignore file is modified

#### Scenario: Dry run predicts the real outcomes

- **WHEN** a dry run reports a set of outcomes and is immediately followed by a real run with no
  intervening change
- **THEN** the real run produces the same outcomes

#### Scenario: Dry run still reports resolution failures

- **WHEN** a source cannot be resolved during a dry run
- **THEN** the failure is reported and the command exits non-zero

### Requirement: Unmanaged Path Protection

A destination that exists but is not recorded in the lockfile SHALL be treated as hand-written and
MUST NOT be overwritten, replaced, or deleted on any flag. Such a destination SHALL be reported
`conflict-unmanaged`, identifying the path and the asset that wanted it.

#### Scenario: Hand-written file is preserved

- **WHEN** an asset's destination already exists and no lockfile entry claims it
- **THEN** the file is left byte-for-byte unchanged and the target reports `conflict-unmanaged`

#### Scenario: Force does not override unmanaged protection

- **WHEN** the user re-runs with the force flag against an unmanaged destination
- **THEN** the destination is still not overwritten and the outcome is still `conflict-unmanaged`,
  because force applies only to drifted destinations `harnaas` itself installed

#### Scenario: Absent lockfile makes everything unmanaged

- **WHEN** install runs with no lockfile present and destinations already exist on disk
- **THEN** those destinations are protected as unmanaged rather than being treated as an error

### Requirement: Drifted Destination Protection

A destination recorded in the lockfile whose current content no longer matches the recorded digest
SHALL be treated as drifted and MUST NOT be overwritten by default; it SHALL be reported
`conflict-drift`. An explicit force flag SHALL overwrite drifted managed destinations, and only those,
restoring them to the resolved source content.

#### Scenario: Drifted destination is preserved by default

- **WHEN** a managed destination has been edited since it was installed
- **THEN** it is left unchanged, reported `conflict-drift`, and the report names the flag that would
  overwrite it

#### Scenario: Force restores the source content

- **WHEN** the user runs with the force flag and a managed destination has drifted
- **THEN** the destination is replaced with the resolved source content and reported as `updated`

### Requirement: Convergence On The Manifest

Installing SHALL bring the managed set into agreement with the manifest, removing managed destinations
whose asset is no longer declared or no longer targets that harness. Removal SHALL apply only to
destinations recorded in the lockfile whose content still matches the recorded digest; a drifted one
SHALL be left in place and reported.

#### Scenario: Undeclared asset is removed

- **WHEN** an asset previously installed is removed from the manifest and install runs
- **THEN** its managed destination is deleted and the lockfile no longer records it

#### Scenario: Dropped target is removed

- **WHEN** an asset stops targeting one harness but still targets another
- **THEN** only the dropped harness's destination is removed

#### Scenario: Drifted orphan is kept and reported

- **WHEN** a destination whose asset is no longer declared has drifted
- **THEN** it is left in place and reported, rather than being deleted

### Requirement: Full Uninstall By Empty Manifest

Emptying the manifest's `assets` array and running install SHALL remove every managed destination, the
managed block in the memory file, and the managed block in the version-control ignore file, and SHALL
leave the lockfile recording no installations. This SHALL be the documented way to uninstall
completely; `harnaas` SHALL provide no separate uninstall command.

#### Scenario: Emptying the assets array removes everything

- **WHEN** the manifest's `assets` array is emptied and install runs
- **THEN** every managed destination is removed, both managed blocks are removed entirely, and the
  lockfile records no installations

#### Scenario: Full uninstall still protects what harnaas did not install

- **WHEN** the assets array is emptied while an unmanaged or drifted destination exists
- **THEN** that destination is left in place and reported rather than deleted

### Requirement: Version Control Ignore Managed Block

Install SHALL maintain a marker-delimited managed block in the project's version-control ignore file
listing exactly the paths it installed. The block SHALL be regenerated on every install and pruned as
convergence removes destinations. Content outside the markers SHALL be preserved byte-for-byte.

#### Scenario: Installed paths are listed individually

- **WHEN** install writes destinations for the declared assets
- **THEN** the managed block lists exactly those paths, one entry per installed path

#### Scenario: Removed destination is pruned from the block

- **WHEN** an asset is removed from the manifest and convergence deletes its destination
- **THEN** the corresponding entry is removed from the managed block

#### Scenario: Surrounding ignore rules are preserved

- **WHEN** the ignore file contains hand-written rules before and after the markers
- **THEN** those rules are unchanged after install

### Requirement: Precise Ignore Entries Only

The managed block MUST NOT contain a coarse directory ignore that would cover paths `harnaas` did not
install. Each entry SHALL name an installed path precisely, so that a hand-written asset sitting in
the same directory as an installed one remains tracked by version control.

#### Scenario: Hand-written asset beside an installed one stays tracked

- **WHEN** a hand-written skill directory sits alongside an installed skill directory under the same
  parent
- **THEN** only the installed path is ignored and the hand-written one remains tracked

#### Scenario: No directory-wide ignore is emitted

- **WHEN** several assets install into the same parent directory
- **THEN** the block lists each installed path rather than collapsing them into a directory ignore

### Requirement: Offline Installation

The command SHALL accept an offline flag that resolves every declared source entirely from the local
cache and performs no network access. When a source or resolved commit required by the manifest is not
present in the cache, the command SHALL fail naming exactly what is missing rather than falling back
to the network.

#### Scenario: Fully cached run succeeds without network access

- **WHEN** install runs with the offline flag and every declared source is cached
- **THEN** the install completes with the same outcomes as an online run and makes no network request

#### Scenario: Missing cache entry is named

- **WHEN** install runs with the offline flag and a declared source is not in the cache
- **THEN** the command reports which source and ref are missing and exits non-zero

### Requirement: Atomic Application

Each destination SHALL be written by staging the new content outside its final location and then
moving it into place, so a destination is never observed in a partially written state. A directory
destination SHALL be replaced as a unit rather than file-by-file, and staging artefacts SHALL be
cleaned up on both success and failure.

#### Scenario: Interrupted write leaves a consistent destination

- **WHEN** the process is interrupted while a destination is being written
- **THEN** the destination is either its previous content or the complete new content, never a partial
  mixture

#### Scenario: Directory replacement is not partial

- **WHEN** a skill directory is updated and the update fails partway
- **THEN** the destination directory is not left containing a mix of old and new files

#### Scenario: Staging artefacts do not survive a failure

- **WHEN** an install fails after staging content
- **THEN** no staging directory or temporary file remains beneath any destination root

### Requirement: Concurrency Safety

Concurrent `harnaas install` runs against the same project MUST NOT interleave their writes to the
lockfile. The command SHALL take an exclusive lock covering the read-modify-write of the lockfile, and
SHALL report a clear message rather than blocking indefinitely when the lock cannot be acquired.

#### Scenario: Concurrent runs serialize

- **WHEN** two installs run against the same project at the same time
- **THEN** their lockfile updates are serialized and the resulting lockfile is internally consistent

#### Scenario: Lock contention is reported

- **WHEN** the lock cannot be acquired within a bounded wait
- **THEN** the command reports that another run holds the lock and exits non-zero rather than waiting
  indefinitely

### Requirement: Deterministic Execution Order

Assets SHALL be processed in a stable order derived from their identifiers rather than from manifest
order or filesystem iteration order, so repeated runs produce identical output and identical lockfile
contents. Reported outcomes SHALL follow that same order.

#### Scenario: Reordering the manifest does not change output

- **WHEN** the assets in the manifest are reordered without other changes and install runs
- **THEN** the reported outcomes and the resulting lockfile are unchanged

#### Scenario: Repeated runs produce identical lockfiles

- **WHEN** install runs twice with no intervening change
- **THEN** the lockfile is byte-for-byte identical after each run

### Requirement: Partial Failure Semantics

A failure affecting one asset MUST NOT abort the processing of unrelated assets. The command SHALL
attempt every asset, report every failure, exit non-zero if any failed, and record in the lockfile
only what actually landed on disk.

#### Scenario: One failing asset does not block the others

- **WHEN** one asset fails to resolve and the rest resolve successfully
- **THEN** the successful assets are installed, the failure is reported, and the command exits
  non-zero

#### Scenario: Lockfile reflects only what landed

- **WHEN** some assets fail and others install
- **THEN** the lockfile records the installed assets and does not record the failed ones

#### Scenario: All failures are reported together

- **WHEN** several assets fail for different reasons
- **THEN** every failure appears in the report rather than only the first

### Requirement: Idempotent Re-Run

Running install again with no change to the manifest, the sources, or the installed files SHALL make
no filesystem change, report every target as `unchanged`, and exit zero.

#### Scenario: Second run is a no-op

- **WHEN** install runs immediately after a successful install with nothing changed
- **THEN** every target reports `unchanged`, no destination or managed block is rewritten, and the
  command exits zero

### Requirement: Machine-Readable Output

The command SHALL support emitting its report as a single JSON document containing every asset,
target, outcome, destination and, where applicable, the remedy. When that mode is active the JSON
document SHALL be the only thing written to standard output.

#### Scenario: JSON report is machine-parseable

- **WHEN** install runs with JSON output requested
- **THEN** standard output contains a single well-formed JSON document describing every outcome

#### Scenario: Advisory text does not corrupt JSON output

- **WHEN** install has advisory information to convey while JSON output is requested
- **THEN** that text is written to standard error and standard output remains valid JSON
