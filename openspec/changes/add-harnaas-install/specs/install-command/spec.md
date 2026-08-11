## Purpose

Defines `harnaas install`, the command that makes the filesystem match the manifest. It covers the
phased flow from resolution to recording, the outcome reported for each asset and target, how
hand-written and locally modified files are protected, how the installed set converges on the
manifest, and the atomicity, ordering and failure semantics that make the command safe to re-run and
safe to run in CI.

## ADDED Requirements

### Requirement: Phased Install Flow

`harnaas install` SHALL proceed in distinct phases: resolve every declared source, compute a plan of
the changes each asset and target implies, apply that plan, and record the result. No filesystem
change to any harness directory may occur before the plan is complete, so a failure during resolution
leaves the harness untouched.

#### Scenario: Resolution failure leaves the harness untouched

- **WHEN** one declared source fails to resolve and resolution is still in progress
- **THEN** no harness directory has been modified and the command reports the resolution failure

#### Scenario: Plan precedes any write

- **WHEN** the command runs successfully
- **THEN** every change applied corresponds to an entry computed during the planning phase

### Requirement: Per-Target Outcome Reporting

Each asset and target combination SHALL produce exactly one outcome from a fixed set: created,
updated, unchanged, skipped because the destination is unmanaged, skipped because the managed
destination was modified locally, or unsupported by that harness. The command SHALL report every
outcome, and an outcome that prevented an install SHALL be accompanied by the action that resolves
it.

#### Scenario: Every target reports an outcome

- **WHEN** the command completes
- **THEN** each asset and target combination appears exactly once in the report with one of the
  defined outcomes

#### Scenario: Blocked outcome carries a remedy

- **WHEN** an asset is skipped because its destination is unmanaged or locally modified
- **THEN** the report states which condition applied and names the flag or action that would proceed

#### Scenario: Unchanged targets are still reported

- **WHEN** an asset is already installed and matches its source
- **THEN** it is reported as unchanged rather than omitted

### Requirement: Plan Preview Without Side Effects

The command SHALL support a dry-run mode that computes and prints the full plan and then exits
without writing to any harness directory, the lockfile, or the memory file. Dry-run output SHALL be
the same set of outcomes a real run would report.

#### Scenario: Dry run writes nothing

- **WHEN** the command runs in dry-run mode
- **THEN** no harness directory, lockfile or memory file is modified

#### Scenario: Dry run predicts the real outcomes

- **WHEN** a dry run reports a set of outcomes and is immediately followed by a real run with no
  intervening change
- **THEN** the real run produces the same outcomes

#### Scenario: Dry run still reports resolution failures

- **WHEN** a source cannot be resolved during a dry run
- **THEN** the failure is reported and the command exits non-zero

### Requirement: Unmanaged Path Protection

A destination that exists but is not recorded in the lockfile SHALL be treated as hand-written and
MUST NOT be overwritten, replaced, or deleted. Such a destination SHALL be reported as an unmanaged
conflict identifying the path and the asset that wanted it.

#### Scenario: Hand-written file is preserved

- **WHEN** an asset's destination already exists and no lockfile entry claims it
- **THEN** the file is left byte-for-byte unchanged and the asset is reported as an unmanaged conflict

#### Scenario: Force does not override unmanaged protection

- **WHEN** the user re-runs with the force flag against an unmanaged destination
- **THEN** the destination is still not overwritten, because force applies only to files `harnaas`
  itself installed

### Requirement: Local Modification Protection

A destination recorded in the lockfile whose current content no longer matches the recorded digest
SHALL be treated as locally modified and MUST NOT be overwritten by default. The user SHALL be able to
overwrite such a destination with an explicit force flag, which restores it to the resolved source
content.

#### Scenario: Modified managed file is preserved by default

- **WHEN** a managed destination has been edited since it was installed
- **THEN** it is left unchanged and reported as locally modified, naming the flag that would overwrite
  it

#### Scenario: Force restores the source content

- **WHEN** the user runs with the force flag and a managed destination was locally modified
- **THEN** the destination is replaced with the resolved source content and reported as updated

### Requirement: Convergence On The Manifest

Installing SHALL bring the managed set into agreement with the manifest, removing managed
destinations whose asset is no longer declared or no longer targets that harness. Removal SHALL apply
only to destinations recorded in the lockfile whose content still matches the recorded digest; a
locally modified one SHALL be left in place and reported.

#### Scenario: Undeclared asset is removed

- **WHEN** an asset previously installed is removed from the manifest and install runs
- **THEN** its managed destination is deleted and the lockfile no longer records it

#### Scenario: Dropped target is removed

- **WHEN** an asset stops targeting one harness but still targets another
- **THEN** only the dropped harness's destination is removed

#### Scenario: Modified orphan is kept and reported

- **WHEN** a destination whose asset is no longer declared has been modified locally
- **THEN** it is left in place and reported, rather than being deleted

### Requirement: Atomic Application

Each destination SHALL be written by staging the new content outside its final location and then
moving it into place, so a destination is never observed in a partially written state. A directory
destination SHALL be replaced as a unit rather than file-by-file, and staging artifacts SHALL be
cleaned up on both success and failure.

#### Scenario: Interrupted write leaves a consistent destination

- **WHEN** the process is interrupted while a destination is being written
- **THEN** the destination is either its previous content or the complete new content, never a
  partial mixture

#### Scenario: Directory replacement is not partial

- **WHEN** a skill directory is updated and the update fails partway
- **THEN** the destination directory is not left containing a mix of old and new files

#### Scenario: Staging artifacts do not survive a failure

- **WHEN** an install fails after staging content
- **THEN** no staging directory or temporary file remains in the harness tree

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
no filesystem change, report every target as unchanged, and exit zero.

#### Scenario: Second run is a no-op

- **WHEN** install runs immediately after a successful install with nothing changed
- **THEN** every target reports unchanged, no file modification times in the harness change, and the
  command exits zero

### Requirement: Machine-Readable Output

The command SHALL support emitting its report as a JSON document containing every asset, target,
outcome, destination and, where applicable, the remedy. When that mode is active the JSON document
SHALL be the only thing written to standard output.

#### Scenario: JSON report is machine-parseable

- **WHEN** install runs with JSON output requested
- **THEN** standard output contains a single well-formed JSON document describing every outcome

#### Scenario: Advisory text does not corrupt JSON output

- **WHEN** install has advisory information to convey while JSON output is requested
- **THEN** that text is written to standard error and standard output remains valid JSON
