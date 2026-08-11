## Purpose

Defines how a declared asset source becomes a verified set of files on disk: resolving a GitHub ref
to an immutable commit, retrieving and extracting the repository archive, reading local sources under
the project's `.harnaas` directory, and computing the content digests that install and lint both
depend on. It also fixes the transport and extraction safety rules for every network fetch the CLI
makes.

## ADDED Requirements

### Requirement: Source Kind Registry

Source kinds SHALL be resolved through a registry keyed by kind name, so a new kind can be added
without modifying the install flow. Every kind SHALL expose the same contract: given a normalized
source, produce a resolved set of files plus the provenance facts needed to record and later re-check
it. Version 1 registers the kinds `github` and `local`.

#### Scenario: Registered kind resolves through the common contract

- **WHEN** an asset declares a source of a registered kind
- **THEN** the install flow resolves it through the registry without kind-specific branching

#### Scenario: Unregistered kind fails before any side effect

- **WHEN** a source names a kind that is not registered
- **THEN** resolution fails naming the unsupported kind, and no network request or filesystem write
  occurs

### Requirement: GitHub Ref Resolution

A `github` source SHALL be resolved to an immutable commit identifier before any content is fetched.
A ref naming a tag or branch SHALL be resolved against the remote repository; a ref that is already a
full commit identifier SHALL be used directly; an absent ref SHALL resolve the repository's default
branch. The resolved commit SHALL be reported alongside the content so it can be recorded.

#### Scenario: Tag resolves to a commit

- **WHEN** a source declares the ref `v1.2.0`
- **THEN** resolution produces the commit that tag points at, and that commit is what content is
  fetched from

#### Scenario: Branch resolves to its current tip

- **WHEN** a source declares a branch ref
- **THEN** resolution produces the branch's current tip commit and reports the ref as mutable

#### Scenario: Commit identifier is used directly

- **WHEN** a source declares a full commit identifier as its ref
- **THEN** no remote ref lookup is required and that commit is used

#### Scenario: Unknown ref is reported clearly

- **WHEN** a source declares a ref that does not exist in the remote repository
- **THEN** resolution fails naming the asset, the repository, and the missing ref

### Requirement: Repository Archive Retrieval

Content for a `github` source SHALL be obtained by fetching the repository archive for the resolved
commit over HTTPS, rather than by requesting files individually, so one request serves an entire
repository regardless of how many assets reference it. The archive for a given repository and commit
SHALL be fetched at most once per run.

#### Scenario: Multiple assets from one repository share a fetch

- **WHEN** several assets reference different paths within the same repository at the same resolved
  commit
- **THEN** the archive is fetched once and every asset is extracted from it

#### Scenario: Fetch failure names the asset

- **WHEN** the archive cannot be retrieved
- **THEN** resolution fails identifying the asset, the repository and the resolved commit

### Requirement: Transport Security

Every network fetch SHALL use HTTPS and MUST reject a plaintext or loopback destination, including
one reached by redirect. Redirects SHALL be followed only up to a bounded limit. Response bodies
SHALL be read under a size ceiling and MUST fail rather than truncate when the ceiling is exceeded.
Credentials MUST NOT be written to disk or included in any error message.

#### Scenario: Plaintext destination is refused

- **WHEN** a fetch would be made over a non-HTTPS scheme
- **THEN** the request is refused before it is sent and resolution fails reporting the insecure
  destination

#### Scenario: Redirect to an insecure destination is refused

- **WHEN** an HTTPS request redirects to a plaintext or loopback destination
- **THEN** the redirect is not followed and resolution fails

#### Scenario: Redirect chain is bounded

- **WHEN** a request exceeds the redirect limit
- **THEN** the fetch fails reporting too many redirects rather than following indefinitely

#### Scenario: Oversized response fails rather than truncates

- **WHEN** a response body exceeds the configured size ceiling
- **THEN** the fetch fails reporting the limit, and no partial content is used

#### Scenario: Credentials are redacted from errors

- **WHEN** a fetch fails against a URL that carried credentials
- **THEN** the reported error contains the destination with its credentials removed

### Requirement: Archive Extraction Safety

Extraction SHALL take only the subtree named by the source's path and SHALL reject any archive entry
whose destination would fall outside the extraction root, including entries using absolute paths or
upward traversal. Symbolic-link, hard-link and device entries MUST be rejected rather than
materialized. Individual entry sizes and the total extracted size SHALL be bounded.

#### Scenario: Only the declared subtree is extracted

- **WHEN** a source names a path within a repository
- **THEN** only files under that path are extracted, and the extracted paths are relative to it

#### Scenario: Traversing entry is rejected

- **WHEN** an archive contains an entry whose path escapes the extraction root
- **THEN** extraction fails reporting the offending entry and nothing is written outside the root

#### Scenario: Link entry is rejected

- **WHEN** an archive contains a symbolic-link or hard-link entry within the selected subtree
- **THEN** extraction fails reporting that links are not supported in harness assets

#### Scenario: Missing path in repository is reported

- **WHEN** the source's path does not exist at the resolved commit
- **THEN** resolution fails naming the asset, the path and the commit

### Requirement: Local Source Reading

A `local` source SHALL be read from the project's `.harnaas` directory through a confined handle
anchored at that directory, so no read can escape it even via a symbolic link created after
validation. A source that does not exist SHALL fail naming the expected path relative to the project
root.

#### Scenario: Local file is read

- **WHEN** an asset declares a local source that exists under `.harnaas`
- **THEN** its content is read and made available for installation

#### Scenario: Symlink escaping .harnaas is refused

- **WHEN** a path under `.harnaas` is a symbolic link pointing outside that directory
- **THEN** the read is refused and resolution fails reporting the containment violation

#### Scenario: Missing local source is reported

- **WHEN** a local source names a path that does not exist
- **THEN** resolution fails naming the asset and the expected path relative to the project root

### Requirement: Source Shape Verification

A resolved source SHALL be verified against the shape its asset type requires before installation is
planned. A `skill` source MUST resolve to a directory and MUST contain the skill definition file the
harness expects; the other asset types MUST resolve to a single regular file. A mismatch SHALL be
reported naming the asset, the expected shape and what was found.

#### Scenario: Skill source must be a directory

- **WHEN** an asset of type `skill` resolves to a single file
- **THEN** resolution fails reporting that a skill source must be a directory

#### Scenario: Skill without its definition file is rejected

- **WHEN** an asset of type `skill` resolves to a directory that does not contain the expected skill
  definition file
- **THEN** resolution fails naming the asset and the missing file

#### Scenario: Single-file type must not be a directory

- **WHEN** an asset of type `command` resolves to a directory
- **THEN** resolution fails reporting that this asset type requires a single file

### Requirement: Content Digest Computation

Resolution SHALL produce a digest for every resolved file and a single digest covering the resolved
source as a whole. The whole-source digest SHALL be computed deterministically from the sorted set of
relative paths and their content digests, so the same content always yields the same value regardless
of filesystem ordering, fetch order, or platform.

#### Scenario: Identical content yields an identical digest

- **WHEN** the same source content is resolved twice on different platforms
- **THEN** both resolutions produce the same whole-source digest

#### Scenario: Changed content changes the digest

- **WHEN** any resolved file's content differs
- **THEN** the whole-source digest differs

#### Scenario: Renaming a file changes the digest

- **WHEN** a file within the resolved source is renamed but its content is unchanged
- **THEN** the whole-source digest differs, because paths participate in the computation

### Requirement: Resolution Caching

Fetched archives SHALL be cached by content under the user cache directory and reused when the same
repository and commit are requested again. A cache entry MUST be verified against its expected digest
before reuse, and a corrupt or unreadable entry SHALL be discarded and re-fetched rather than
failing. Callers SHALL be able to bypass the cache.

#### Scenario: Second run reuses the cache

- **WHEN** the same repository and commit are resolved again on a later run
- **THEN** the cached archive is reused and no network request is made

#### Scenario: Corrupt cache entry is refetched

- **WHEN** a cached archive no longer matches its expected digest
- **THEN** the entry is discarded, the archive is fetched again, and resolution succeeds

#### Scenario: Cache can be bypassed

- **WHEN** the caller requests that the cache not be used
- **THEN** content is fetched fresh and the run does not read the cache

### Requirement: Authenticated And Offline Behaviour

Resolution SHALL support reading a token from the environment for private repositories, and SHALL
surface an authentication failure as a distinct, actionable error rather than as a generic fetch
failure. When the network is unreachable, resolution SHALL fail naming the asset and the unreachable
host rather than silently falling back to stale or empty content.

#### Scenario: Private repository resolves with a token

- **WHEN** a token is present in the environment and a source references a private repository the
  token can read
- **THEN** resolution succeeds

#### Scenario: Authentication failure is actionable

- **WHEN** a source references a repository the caller is not authorized to read
- **THEN** resolution fails reporting that access was denied and naming the environment variable used
  to supply a token

#### Scenario: Network failure never yields empty content

- **WHEN** the remote host is unreachable
- **THEN** resolution fails naming the asset and the host, and no asset is treated as resolving to
  empty content
