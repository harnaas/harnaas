## Purpose

Defines how `harnaas` determines that an installed asset has fallen behind its source: a mutable ref
whose commit has moved, a pinned tag a newer release has superseded, or a local source edited since
it was installed. It also covers caching those results and degrading gracefully when the network or
credentials are unavailable, so an update check never turns a working command into a failing one.

## ADDED Requirements

### Requirement: Moved Mutable Ref Detection

For an asset installed from a ref that names a branch or the repository default branch, update
detection SHALL resolve that ref again and compare the result to the commit recorded at install time.
A differing commit SHALL be reported as an available update naming both commits.

#### Scenario: Branch has advanced

- **WHEN** an asset was installed from a branch and that branch now points at a different commit
- **THEN** an update is reported naming the recorded commit, the current commit and the branch

#### Scenario: Branch is unchanged

- **WHEN** the branch still points at the recorded commit
- **THEN** no update is reported for that asset

#### Scenario: Branch has disappeared

- **WHEN** the ref recorded at install no longer exists in the remote repository
- **THEN** that is reported as a finding distinct from an available update, because reinstalling
  would fail

### Requirement: Superseded Tag Detection

For an asset installed from a ref naming a version tag, update detection SHALL look for a newer tag in
the same repository and report the highest one that is newer than the installed tag. Comparison SHALL
follow semantic-version ordering. Pre-release tags MUST NOT be offered as an update to an asset
installed from a stable tag.

#### Scenario: Newer stable tag is offered

- **WHEN** an asset is installed from a tag and the repository has published a higher stable tag
- **THEN** an update is reported naming the installed tag and the highest newer stable tag

#### Scenario: Only the highest newer tag is reported

- **WHEN** several tags newer than the installed one exist
- **THEN** exactly one update finding is reported, naming the highest

#### Scenario: Pre-release is not offered over a stable install

- **WHEN** the only newer tag is a pre-release and the asset was installed from a stable tag
- **THEN** no update is reported

#### Scenario: Non-version tags are ignored

- **WHEN** the repository carries tags that are not semantic versions
- **THEN** they are ignored for comparison rather than producing a spurious update

### Requirement: Pinned Commit Is Never Reported As Outdated

An asset installed from a ref that is an explicit commit identifier SHALL never be reported as
outdated, because the user pinned it deliberately. Detection MUST NOT search for newer commits or tags
in that case.

#### Scenario: Commit-pinned asset produces no update finding

- **WHEN** an asset's source ref is a full commit identifier
- **THEN** no update finding is produced for it and no remote lookup is performed on its behalf

### Requirement: Mutable Ref Advisory

An asset whose source tracks a mutable ref SHALL be reported as a warning noting that its installs are
not reproducible, independent of whether that ref has moved. The finding SHALL suggest pinning to a
tag or commit.

#### Scenario: Branch-tracking asset is flagged

- **WHEN** an asset's source names a branch or omits a ref
- **THEN** a warning reports that the asset tracks a mutable ref and suggests pinning it

#### Scenario: Pinned asset is not flagged

- **WHEN** an asset's source names a version tag or a commit identifier
- **THEN** no mutable-ref warning is produced for it

### Requirement: Local Source Change Detection

For an asset installed from a local source, update detection SHALL re-read the source under
`.harnaas`, recompute its digest, and compare it to the digest recorded at install. A difference SHALL
be reported as an available update. This check requires no network and SHALL run in offline mode.

#### Scenario: Edited local source is detected

- **WHEN** a file under `.harnaas` that backs an installed asset has been edited since install
- **THEN** an update is reported for that asset naming the install command as the remedy

#### Scenario: Local check runs offline

- **WHEN** lint runs in offline mode
- **THEN** local source change detection still runs and its findings are reported

#### Scenario: Deleted local source is distinguished

- **WHEN** a local source recorded in the lockfile no longer exists
- **THEN** that is reported as a missing source rather than as an available update

### Requirement: Resolution Result Caching

Ref-resolution and tag-listing results SHALL be cached under the user cache directory with a bounded
freshness window, so repeated runs within that window do not repeat network requests. A cache entry
past its window SHALL be refreshed. Callers SHALL be able to force a refresh, and a corrupt cache
entry SHALL be discarded rather than causing a failure.

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
report the affected asset as unchecked, state why, and allow every other check to complete. Repeated
failures against the same host SHALL be reported once rather than per asset.

#### Scenario: Unreachable host does not fail the run

- **WHEN** the remote host cannot be reached
- **THEN** local findings are still reported, the affected assets are marked unchecked, and the exit
  status is decided only by the checks that ran

#### Scenario: Authentication failure is reported distinctly

- **WHEN** update detection is denied access to a private repository
- **THEN** the affected assets are reported as unchecked because of an authorization failure, naming
  the environment variable used to supply a token

#### Scenario: Repeated host failures are summarized

- **WHEN** several assets reference the same unreachable host
- **THEN** the failure is reported once for that host rather than once per asset

#### Scenario: Unchecked assets are visible in the summary

- **WHEN** any asset could not be checked
- **THEN** the report's summary states how many assets went unchecked so a clean result is not
  mistaken for a complete one
