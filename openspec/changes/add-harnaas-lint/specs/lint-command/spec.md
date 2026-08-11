## Purpose

Defines `harnaas lint`, the read-only command that checks the manifest, the lockfile and the files on
disk against one another and reports every discrepancy with the action that resolves it. It covers
the finding model, each local check, severity and exit-code semantics, output modes, and the
guarantee that lint never changes state.

## ADDED Requirements

### Requirement: Read-Only Guarantee

`harnaas lint` MUST NOT create, modify, move or delete any file in the project or in any harness
directory, and MUST NOT rewrite the manifest or the lockfile. It SHALL report what is wrong and how
to fix it, leaving `harnaas install` as the only command that changes state. Writing to caches under
the user cache directory is permitted.

#### Scenario: No project file is touched

- **WHEN** lint runs against a project with several findings
- **THEN** every file in the project and in the harness directories is byte-for-byte unchanged
  afterwards

#### Scenario: Findings are reported, not repaired

- **WHEN** lint detects an installed file that no longer matches its recorded digest
- **THEN** it reports the finding and does not restore, delete or re-install the file

### Requirement: Finding Model

Every discrepancy SHALL be reported as a finding carrying the asset it concerns, a severity, a
description of the problem, and a remedy naming the command or action that resolves it. Findings for
a file-level problem SHALL identify the specific path. A finding with no available remedy SHALL say so
rather than omitting the field.

#### Scenario: Finding names its remedy

- **WHEN** any finding is reported
- **THEN** it includes a remedy stating the command or action that would resolve it

#### Scenario: File-level finding names the file

- **WHEN** one file within an installed directory asset is modified
- **THEN** the finding identifies that file's path, not only the asset

#### Scenario: Findings are ordered deterministically

- **WHEN** lint reports several findings
- **THEN** they appear in a stable order derived from the asset identifier and path, identical across
  runs

### Requirement: Manifest And Lockfile Consistency Checks

Lint SHALL report a manifest that fails to load or validate, an asset declared in the manifest with no
corresponding lockfile entry, and a lockfile entry for an asset no longer declared or no longer
targeting that harness. A manifest that cannot be loaded SHALL be reported as a single finding and
SHALL suppress the checks that depend on it rather than producing cascading noise.

#### Scenario: Invalid manifest is a single finding

- **WHEN** the manifest fails validation
- **THEN** lint reports that as one finding and does not additionally report every asset as missing

#### Scenario: Declared but never installed

- **WHEN** an asset appears in the manifest with no lockfile entry
- **THEN** lint reports it as not installed and names the install command as the remedy

#### Scenario: Lockfile entry for an undeclared asset

- **WHEN** the lockfile records an asset the manifest no longer declares
- **THEN** lint reports the stale entry and names the install command, which converges the set

#### Scenario: Missing lockfile with declared assets

- **WHEN** the manifest declares assets and no lockfile exists
- **THEN** lint reports that nothing has been installed yet rather than treating the absent lockfile
  as an error

### Requirement: Installed Content Integrity Checks

For each recorded installation lint SHALL re-compute the digest of every installed file and compare it
to the digest recorded at install time. It SHALL report a file whose content differs, a recorded file
that is missing, an installation whose destination is absent entirely, and a file present under a
managed destination that the record does not account for.

#### Scenario: Modified file is detected

- **WHEN** an installed file's content differs from its recorded digest
- **THEN** lint reports that the file was modified outside `harnaas` and names the forced install as
  the way to restore it

#### Scenario: Missing file is detected

- **WHEN** a file recorded in an installation no longer exists
- **THEN** lint reports the missing path and names the install command as the remedy

#### Scenario: Absent destination is detected

- **WHEN** an installation's destination directory or file no longer exists at all
- **THEN** lint reports the installation as missing rather than reporting each recorded file
  separately

#### Scenario: Extraneous file is detected

- **WHEN** a file exists inside a managed destination that the installation record does not list
- **THEN** lint reports the extraneous path so the user can tell an accidental addition from an
  intentional one

### Requirement: Unmanaged Conflict Check

Lint SHALL report a declared asset whose destination exists on disk but is not recorded in the
lockfile, because install will refuse to overwrite it. The finding SHALL name the destination and
explain that install will not claim a path it did not create.

#### Scenario: Hand-written file blocks an install

- **WHEN** an asset's destination exists and no lockfile entry claims it
- **THEN** lint reports the unmanaged conflict, names the path, and explains that install will not
  overwrite it

### Requirement: Severity And Exit Status

Each finding SHALL carry a severity of error or warning. A condition where the installed state does
not match what was recorded or declared SHALL be an error; a condition that is merely worth attention,
such as an available upstream update or a source tracking a mutable ref, SHALL be a warning. Lint
SHALL exit `0` when no error is present, and `2` when at least one is. A strict mode SHALL promote
warnings to errors for exit purposes.

#### Scenario: Clean project exits zero

- **WHEN** lint finds no discrepancies
- **THEN** it reports that everything is consistent and exits `0`

#### Scenario: Warnings alone still exit zero

- **WHEN** lint reports only warnings
- **THEN** it exits `0` and the warnings are still printed

#### Scenario: Errors exit two

- **WHEN** lint reports at least one error-severity finding
- **THEN** it exits `2`, distinct from the exit status used for a command failure

#### Scenario: Strict mode promotes warnings

- **WHEN** lint runs in strict mode and reports only warnings
- **THEN** it exits `2`

#### Scenario: Command failure is distinguishable from findings

- **WHEN** lint itself fails, for example because the lockfile is unreadable
- **THEN** it exits with the runtime-failure status rather than the findings status

### Requirement: Output Modes

Lint SHALL print a human-readable report by default, grouped by asset, showing each finding's
severity, problem and remedy, and ending with a summary count. It SHALL also support emitting the
same findings as a single JSON document, in which case that document is the only thing written to
standard output.

#### Scenario: Human report summarizes

- **WHEN** lint completes with findings
- **THEN** the report groups findings by asset and ends with counts of errors and warnings

#### Scenario: JSON report contains the full finding set

- **WHEN** lint runs with JSON output requested
- **THEN** standard output is a single well-formed JSON document containing every finding with its
  asset, severity, problem, remedy and path where applicable

#### Scenario: Clean run in JSON mode is still valid JSON

- **WHEN** lint finds nothing and JSON output is requested
- **THEN** standard output is a valid JSON document with an empty finding set

### Requirement: Offline Operation

Lint SHALL support an offline mode that performs every local check and skips every check requiring
network access. When network checks are skipped, whether by request or by unavailability, the report
SHALL state that they were skipped so a clean result is never mistaken for a fully checked one.

#### Scenario: Offline mode performs local checks only

- **WHEN** lint runs in offline mode
- **THEN** all integrity and consistency checks run, no network request is made, and the report notes
  that update checks were skipped

#### Scenario: Skipped checks are visible in a clean report

- **WHEN** lint runs offline and finds no local problems
- **THEN** the report still states that update detection did not run
