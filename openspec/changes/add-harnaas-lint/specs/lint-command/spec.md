## Purpose

Defines `harnaas lint`, the read-only command that checks the manifest, the lockfile, the installed
files and the upstream sources against one another and reports every discrepancy with the exact edit
and command that resolve it. It covers the finding model, each local and upstream check, the severity
rule that the only passing state is pinned and current, the frozen and offline modes, output modes,
exit status, and the guarantee that lint never changes state.

## ADDED Requirements

### Requirement: Read-Only Guarantee

`harnaas lint` MUST NOT create, modify, move or delete any file in the project, in any harness
directory, or in the user's home, and MUST NOT rewrite the manifest or the lockfile. It SHALL report
what is wrong and how to fix it, leaving `harnaas install` as the single repair path. Writing to
caches under the user cache directory is permitted.

#### Scenario: No project file is touched

- **WHEN** lint runs against a project with several findings
- **THEN** every file in the project and in the harness directories is byte-for-byte unchanged
  afterwards

#### Scenario: Findings are reported, not repaired

- **WHEN** lint detects an installed file that no longer matches its recorded digest
- **THEN** it reports the finding and does not restore, delete or re-install the file

#### Scenario: Cache writes are allowed

- **WHEN** lint fetches upstream metadata during update detection
- **THEN** it may populate the content-addressed cache while still leaving the project untouched

### Requirement: Finding Model

Every discrepancy SHALL be reported as a finding carrying the asset it concerns where one applies, a
severity, a description of the problem, and a remedy naming the command or action that resolves it. A
file-level finding SHALL identify the specific path. A finding with no available remedy SHALL say so
rather than omitting the field.

#### Scenario: Finding names its remedy

- **WHEN** any finding is reported
- **THEN** it includes a remedy stating the command or action that would resolve it

#### Scenario: File-level finding names the file

- **WHEN** one file within an installed directory asset is modified
- **THEN** the finding identifies that file's path, not only the asset

#### Scenario: Project-level finding needs no asset

- **WHEN** the finding concerns the `.gitignore` managed block rather than one asset
- **THEN** it is reported without an asset identifier and still carries a severity and a remedy

### Requirement: Deterministic Finding Order

Lint SHALL emit findings in a stable order derived from the asset identifier and path, so that two
runs over identical state produce identical output. Reordering entries in the manifest or the lockfile
MUST NOT change the order in which findings appear.

#### Scenario: Repeat runs match

- **WHEN** lint runs twice against unchanged state
- **THEN** the two reports are identical, finding for finding

#### Scenario: Manifest order is irrelevant

- **WHEN** the assets in the manifest are reordered and lint runs again
- **THEN** the findings appear in the same order as before

### Requirement: Manifest And Lockfile Consistency Checks

Lint SHALL report a manifest that fails to load or validate, an asset declared in the manifest with no
corresponding lockfile entry, and a lockfile entry for an asset the manifest no longer declares or no
longer targets. A manifest that cannot be loaded SHALL be reported as a single finding and SHALL
suppress every check that depends on it rather than producing cascading noise.

#### Scenario: Invalid manifest is a single finding

- **WHEN** the manifest fails validation
- **THEN** lint reports that as one finding and does not additionally report every asset as missing

#### Scenario: Declared but never installed

- **WHEN** an asset appears in the manifest with no lockfile entry
- **THEN** lint reports it as not installed and names the install command as the remedy

#### Scenario: Lockfile entry for an undeclared asset

- **WHEN** the lockfile records an asset the manifest no longer declares
- **THEN** lint reports the stale entry and names the install command, which converges the set

### Requirement: Nothing Installed Collapses To One Finding

When no declared asset has been installed at all — because the lockfile is absent, or because it
records nothing — lint SHALL report exactly one finding stating that the project has not been
installed yet and naming `harnaas install`. It MUST NOT emit one finding per declared asset, and MUST
NOT treat the absent lockfile itself as a separate problem.

#### Scenario: Fresh checkout with no lockfile

- **WHEN** the manifest declares twelve assets and no lockfile exists
- **THEN** lint reports one finding saying nothing is installed yet, not twelve

#### Scenario: Empty lockfile

- **WHEN** a lockfile exists but records no installations while the manifest declares assets
- **THEN** lint reports the same single collapsed finding

### Requirement: Installed Content Integrity Checks

For each recorded installation lint SHALL re-compute the digest of every installed file and compare it
with the digest recorded at install time. It SHALL report a file whose content differs, a recorded
file that is missing, and a file present under a managed destination that the record does not account
for. An installation whose destination is absent entirely SHALL collapse to one finding rather than
one per recorded file.

#### Scenario: Modified file is detected

- **WHEN** an installed file's content differs from its recorded digest
- **THEN** lint reports that the file was modified outside harnaas and names the forced install as the
  way to restore it

#### Scenario: Missing file is detected

- **WHEN** a file recorded in an installation no longer exists
- **THEN** lint reports the missing path and names the install command as the remedy

#### Scenario: Absent destination collapses

- **WHEN** an installation's destination directory no longer exists at all
- **THEN** lint reports one finding for the installation rather than one for each recorded file

#### Scenario: Extraneous file is detected

- **WHEN** a file exists inside a managed destination that the installation record does not list
- **THEN** lint reports the extraneous path so an accidental addition can be told from an intentional
  one

### Requirement: Unmanaged Conflict Check

Lint SHALL report a declared asset whose destination exists on disk but is recorded by no lockfile
entry, because install will refuse to overwrite it. The finding SHALL name the destination and state
that harnaas never claims a path it did not create, on any flag.

#### Scenario: Hand-written file blocks an install

- **WHEN** an asset's destination exists and no lockfile entry claims it
- **THEN** lint reports the unmanaged conflict, names the path, and explains that install will not
  overwrite it, `--force` included

### Requirement: Managed Block Drift Checks

Lint SHALL verify the managed block harnaas owns in `AGENTS.md` and the managed block it owns in
`.gitignore`. It SHALL report a block whose markers are missing, duplicated or malformed, and a block
whose content no longer matches what the recorded installations imply. Content outside the markers
SHALL be ignored entirely, never reported as drift.

#### Scenario: Instruction block edited by hand

- **WHEN** the managed block in `AGENTS.md` no longer matches the recorded instruction assets
- **THEN** lint reports managed-block drift naming `AGENTS.md` and the install command as the remedy

#### Scenario: Ignore block no longer lists an installed path

- **WHEN** an installed path recorded in the lockfile is absent from the managed block in `.gitignore`
- **THEN** lint reports the drift and names the path that is no longer ignored

#### Scenario: Missing or broken markers

- **WHEN** a managed block's start marker is present and its end marker is not
- **THEN** lint reports the block as malformed rather than attempting to interpret its content

#### Scenario: Surrounding content is not drift

- **WHEN** the team edits `AGENTS.md` outside the markers
- **THEN** lint reports no finding for those edits

### Requirement: Bridge Line Check

When instruction assets are recorded as installed, lint SHALL verify that `CLAUDE.md` contains exactly
one `@AGENTS.md` bridge line. A missing bridge line, a missing `CLAUDE.md`, or a duplicated bridge line
SHALL each be reported as a finding naming the install command, because without it the harness never
reads the managed block.

#### Scenario: Bridge line missing

- **WHEN** instruction assets are installed and `CLAUDE.md` contains no `@AGENTS.md` line
- **THEN** lint reports that the bridge line is missing and that the instruction content is not being
  read

#### Scenario: Bridge line duplicated

- **WHEN** `CLAUDE.md` contains more than one `@AGENTS.md` line
- **THEN** lint reports the duplication and names the install command as the remedy

#### Scenario: No instruction assets means no check

- **WHEN** no instruction asset is declared or installed
- **THEN** lint reports nothing about `CLAUDE.md` or the bridge line

### Requirement: Severity Rule Of Pinned And Current

Lint SHALL treat as an error every state that is not both pinned and current: an available update, a
moved or vanished ref, a changed or deleted local source, and an asset tracking a mutable ref such as a
branch, which SHALL be reported as not reproducible with the instruction to pin it. Warning severity
SHALL be reserved for advisory findings that leave the installation reproducible and current. An
available update MUST NOT be emitted at a lower severity, and no flag SHALL downgrade one.

#### Scenario: Available update is an error

- **WHEN** the only finding is that a newer stable tag exists upstream
- **THEN** that finding carries error severity

#### Scenario: Tracking a branch is an error

- **WHEN** an asset's source names a branch rather than a tag or commit
- **THEN** lint reports an error saying the installation is not reproducible and to pin the ref

#### Scenario: Pinned and current passes

- **WHEN** every asset is pinned to an immutable ref, is current with its source, and matches disk
- **THEN** lint reports no errors

### Requirement: Remedies Show The Exact Edit

A finding that requires a manifest change SHALL print the exact before and after text of the edit —
the manifest, the line declaring the source, the exact current string and the exact replacement
string — followed by the command to run afterwards, and both strings SHALL be printed verbatim so
applying the edit is a literal substitution. A finding that needs no manifest edit SHALL print the
command alone with no before/after block. Because the manifest is hand-edited only, lint MUST NOT
offer to apply the edit itself.

#### Scenario: Update remedy shows the diff

- **WHEN** lint reports that `v1.4.0` supersedes the installed `v1.2.0`
- **THEN** the remedy names the manifest and the line declaring that source, shows the current source
  line, the replacement line, and `harnaas install`

#### Scenario: Pinning remedy shows the resolved ref

- **WHEN** lint reports an asset tracking a branch
- **THEN** the remedy shows the replacement source line pinned to the commit the branch currently
  resolves to

#### Scenario: Edit is copyable as written

- **WHEN** any finding prints a before/after edit
- **THEN** the two strings are printed verbatim, so applying the edit is a literal substitution

#### Scenario: Finding needing no edit names only the command

- **WHEN** the finding is a changed local source, drift, or a missing file, none of which needs a
  manifest edit
- **THEN** no before/after edit is printed and the install command is given as the whole remedy

### Requirement: Frozen Mode

Lint SHALL support a frozen mode that verifies the lockfile still satisfies the manifest using the
manifest and the lockfile alone — no installed files are read and no network request is made. It SHALL
report a declared asset with no lockfile entry, a lockfile entry that is undeclared, and a lockfile
entry whose recorded source, ref or type disagrees with the manifest.

#### Scenario: Fresh checkout passes

- **WHEN** frozen mode runs on a clean clone where nothing has been installed and the lockfile agrees
  with the manifest
- **THEN** lint reports no findings and exits `0`

#### Scenario: Manifest changed without reinstalling

- **WHEN** an asset was added to the manifest and the lockfile was not regenerated
- **THEN** frozen mode reports the unsatisfied declaration and exits with the findings status

#### Scenario: Ref changed without reinstalling

- **WHEN** the manifest requests a different ref than the lockfile records for an asset
- **THEN** frozen mode reports the disagreement naming both refs

#### Scenario: No files and no network

- **WHEN** frozen mode runs
- **THEN** no installed destination is read, no source is fetched, and no missing-file or drift finding
  is produced

### Requirement: Offline Operation

Lint SHALL support an offline mode that performs every local check, including local-source change
detection, and skips every check requiring network access. Whenever network checks are skipped, by
request or by unavailability, the report SHALL state that they were skipped, so a clean result is never
mistaken for a complete one.

#### Scenario: Offline mode performs local checks only

- **WHEN** lint runs in offline mode
- **THEN** integrity, consistency, managed-block and local-source checks all run, no network request is
  made, and the report notes that update detection was skipped

#### Scenario: Skipped checks are visible in a clean report

- **WHEN** lint runs offline and finds no local problems
- **THEN** the report still states that update detection did not run

### Requirement: Exit Status

Lint SHALL exit `0` when no error-severity finding is present, `2` when at least one is, and `1` when
lint itself fails to run — for example an unreadable lockfile. Strict mode SHALL promote warnings to
errors for the purpose of the exit status. The exit status SHALL be identical in every output mode.

#### Scenario: Clean project exits zero

- **WHEN** lint finds no discrepancies
- **THEN** it reports that everything is consistent and exits `0`

#### Scenario: Warnings alone still exit zero

- **WHEN** lint reports only warnings
- **THEN** it exits `0` and the warnings are still printed

#### Scenario: Errors exit two

- **WHEN** lint reports at least one error-severity finding, such as an available update
- **THEN** it exits `2`

#### Scenario: Strict mode promotes warnings

- **WHEN** lint runs in strict mode and reports only warnings
- **THEN** it exits `2`

#### Scenario: Lint failure exits one

- **WHEN** lint cannot complete because the lockfile is unreadable
- **THEN** it exits `1`, distinct from the status used to signal findings

### Requirement: Output Modes

Lint SHALL print a human-readable report by default, grouped by asset, showing each finding's severity,
problem and remedy, and ending with a summary count of errors and warnings. It SHALL also support
emitting the same findings as a single JSON document, in which case that document is the only thing
written to standard output and advisory text goes to standard error.

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
