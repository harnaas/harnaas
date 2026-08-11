## Purpose

Defines how `harnaas` determines that an installed asset has fallen behind its source — a mutable ref
whose commit has moved, a ref that has vanished, a pinned tag a newer release has superseded, or a
local source edited since it was installed — and which of those conditions each one produces. Every
finding here is an error, under the severity rule and remedy format the `lint-command` capability
defines. It also covers caching those results and degrading gracefully when the network or
credentials are unavailable, so an update check never turns a working command into a failing one.

## ADDED Requirements

### Requirement: Moved Mutable Ref Detection

For an asset installed from a ref that names a branch or the repository default branch, update
detection SHALL resolve that ref again and compare the result to the commit recorded in the lockfile
at install time. A differing commit SHALL be reported as an available update naming the recorded
commit, the current commit and the ref.

#### Scenario: Branch has advanced

- **WHEN** an asset was installed from a branch and that branch now points at a different commit
- **THEN** an error is reported naming the recorded commit, the current commit and the branch

#### Scenario: Branch is unchanged

- **WHEN** the branch still points at the commit recorded in the lockfile
- **THEN** no update finding is reported for that asset

### Requirement: Vanished Ref Detection

A ref recorded in the lockfile that no longer exists in the remote repository SHALL be reported as a
finding distinct from an available update, because no newer content is on offer and a reinstall would
fail outright. The finding SHALL name the missing ref and the source it belongs to.

#### Scenario: Recorded branch has been deleted

- **WHEN** the ref an asset was installed from no longer exists in the remote repository
- **THEN** a finding distinct from an available update is reported, naming the missing ref

#### Scenario: Vanished ref is not counted as an update

- **WHEN** a vanished ref is reported
- **THEN** the finding does not claim a newer commit or tag is available

### Requirement: Superseded Tag Detection

For an asset installed from a ref naming a version tag, update detection SHALL list the repository's
tags and report the highest tag that is newer than the installed one, using semantic-version ordering.
Tags that are not version tags SHALL be ignored. A pre-release tag MUST NOT be offered as an update to
an asset installed from a stable tag.

#### Scenario: Newer stable tag is offered

- **WHEN** an asset is installed from a tag and the repository has published a higher stable tag
- **THEN** an error is reported naming the installed tag and the highest newer stable tag

#### Scenario: Only the highest newer tag is reported

- **WHEN** several tags newer than the installed one exist
- **THEN** exactly one finding is reported, naming the highest

#### Scenario: Pre-release is not offered over a stable install

- **WHEN** the only newer tag is a pre-release and the asset was installed from a stable tag
- **THEN** no update finding is reported

#### Scenario: Non-version tags are ignored

- **WHEN** the repository carries tags that are not semantic versions
- **THEN** they are ignored for comparison rather than producing a spurious finding

### Requirement: Commit-Pinned Asset Is Never Checked

An asset whose source ref is an explicit commit identifier SHALL never be reported as outdated,
because the user pinned it deliberately. Update detection MUST NOT search for newer commits or tags on
its behalf and MUST NOT issue any network request for it.

#### Scenario: Commit-pinned asset produces no finding

- **WHEN** an asset's source ref is a full commit identifier
- **THEN** no update finding is produced for it and no remote lookup is performed on its behalf

### Requirement: Mutable Ref Is Not Reproducible

An asset whose source names a branch, or omits a ref, SHALL be reported as an error stating that the
asset is not reproducible and must be pinned. This finding SHALL be produced whether or not the ref has
moved, and MUST NOT be produced for an asset whose source names a version tag or a commit identifier.

#### Scenario: Branch-tracking asset is an error

- **WHEN** an asset's source names a branch or omits a ref
- **THEN** an error reports that the asset is not reproducible and directs the reader to pin the ref

#### Scenario: Unmoved branch is still an error

- **WHEN** a branch-tracking asset's branch still points at the recorded commit
- **THEN** the not-reproducible error is reported anyway

#### Scenario: Pinned asset is not flagged

- **WHEN** an asset's source names a version tag or a commit identifier
- **THEN** no not-reproducible finding is produced for it

### Requirement: Local Source Change Detection

For an asset installed from a local source, update detection SHALL re-read the source under
`.harnaas`, recompute its digest, and compare it to the digest recorded in the lockfile. A difference
SHALL be reported as an available update. This check requires no network and SHALL run when lint is
asked to skip the network.

#### Scenario: Edited local source is detected

- **WHEN** a file under `.harnaas` that backs an installed asset has been edited since install
- **THEN** an error is reported for that asset naming the install command as the remedy

#### Scenario: Local check runs offline

- **WHEN** lint runs with the network disabled
- **THEN** local source change detection still runs and its findings are reported

#### Scenario: Unchanged local source passes

- **WHEN** the local source's recomputed digest matches the recorded one
- **THEN** no finding is produced for that asset

### Requirement: Deleted Local Source Detection

A local source recorded in the lockfile whose path no longer exists under `.harnaas` SHALL be reported
as a finding distinct from an available update, since there is nothing to reinstall from. The finding
SHALL name the missing path.

#### Scenario: Missing local source is distinguished

- **WHEN** a local source recorded in the lockfile no longer exists on disk
- **THEN** a missing-source finding is reported rather than an available update, naming the path

### Requirement: Resolution Result Caching

Ref-resolution and tag-listing results SHALL be cached under the user cache directory with a bounded
freshness window, so repeated runs within that window do not repeat network requests. An entry past
its window SHALL be resolved again. Callers SHALL be able to force a refresh, and a corrupt entry
SHALL be discarded rather than failing the run.

#### Scenario: Repeat run within the window uses the cache

- **WHEN** lint runs twice in quick succession against the same sources
- **THEN** the second run reuses the cached resolution results and makes no network request

#### Scenario: Stale entry is refreshed

- **WHEN** a cached result is older than the freshness window
- **THEN** it is resolved again and the cache is updated

#### Scenario: Forced refresh bypasses the cache

- **WHEN** the caller requests a refresh
- **THEN** results are resolved from the remote regardless of cache freshness

#### Scenario: Corrupt cache entry does not fail the run

- **WHEN** a cached entry cannot be read or parsed
- **THEN** it is discarded, the result is resolved again, and the run continues

### Requirement: Graceful Network Degradation

A failure to reach a remote MUST NOT fail the lint run or mask local findings. Update detection SHALL
mark the affected assets unchecked, state why, and let every other check complete. Repeated failures
against one host SHALL be reported once for that host rather than once per asset, and an unchecked
asset MUST NOT be counted as an error.

#### Scenario: Unreachable host does not fail the run

- **WHEN** the remote host cannot be reached
- **THEN** local findings are still reported, the affected assets are marked unchecked, and the exit
  status is decided only by the checks that ran

#### Scenario: Authorization failure is reported distinctly

- **WHEN** update detection is denied access to a private repository
- **THEN** the affected assets are reported as unchecked because of an authorization failure, naming
  the environment variable used to supply a token

#### Scenario: Repeated host failures are summarized

- **WHEN** several assets reference the same unreachable host
- **THEN** the failure is reported once for that host rather than once per asset

#### Scenario: Unchecked assets are visible in the summary

- **WHEN** any asset could not be checked
- **THEN** the summary states how many assets went unchecked, so a clean result is not mistaken for a
  complete one
