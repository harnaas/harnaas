## Purpose

Defines how a declared asset source becomes a verified set of files on disk: resolving a GitHub ref
to an immutable commit, retrieving and extracting the repository archive, reading local sources under
the project's `.harnaas` directory, verifying that what arrived has the shape its asset type
requires, and computing the content digests that install and lint both depend on. It also fixes the
transport security, caching, authentication and offline rules for every fetch the CLI makes.

## ADDED Requirements

### Requirement: Source Kind Registry

Source kinds SHALL be resolved through a registry keyed by kind name, so a new kind can be added
without modifying the install flow. Every kind SHALL expose the same contract: given a normalized
source, produce a resolved set of files plus the provenance facts needed to record and later re-check
it. Version 1 SHALL register the kinds `github` and `local`.

#### Scenario: Registered kind resolves through the common contract

- **WHEN** an asset declares a source of a registered kind
- **THEN** the install flow resolves it through the registry without kind-specific branching

#### Scenario: Unregistered kind fails before any side effect

- **WHEN** a source names a kind that is not registered
- **THEN** resolution fails naming the unsupported kind, and no network request or filesystem write
  occurs

### Requirement: GitHub Ref Resolution

A `github` source SHALL be resolved to an immutable commit identifier over the Git protocol, reusing
the ambient Git credential helpers, before any content is fetched. A tag or branch ref SHALL be
resolved against the remote repository; a ref that is already a full commit identifier SHALL be used
directly; an absent ref SHALL resolve the repository's default branch. The resolved commit SHALL be
reported alongside the content so it can be recorded.

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
commit over HTTPS, rather than by requesting files individually, and SHALL be fetched at most once
per repository and commit per run. A retrieval failure SHALL name the asset, the repository and the
resolved commit, and MUST NOT be treated as the source resolving to empty content.

#### Scenario: Multiple assets from one repository share a fetch

- **WHEN** several assets reference different paths within the same repository at the same resolved
  commit
- **THEN** the archive is fetched once and every asset is extracted from it

#### Scenario: Fetch failure names the asset

- **WHEN** the archive cannot be retrieved
- **THEN** resolution fails identifying the asset, the repository and the resolved commit

#### Scenario: Network failure never yields empty content

- **WHEN** the remote host is unreachable
- **THEN** resolution fails naming the asset and the host, and no asset is treated as resolving to
  empty content

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

#### Scenario: Extraction size ceiling is enforced

- **WHEN** the selected subtree exceeds the total extraction size ceiling
- **THEN** extraction fails reporting the limit and no partially extracted content is used

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
planned. A `skill` source MUST resolve to a directory containing `SKILL.md`. A `rule`, `instruction`,
`command` or `persona` source MUST resolve to a single regular file. A mismatch SHALL be reported
naming the asset, its type, the expected shape and what was found instead.

#### Scenario: Skill source must be a directory

- **WHEN** an asset of type `skill` resolves to a single file
- **THEN** resolution fails reporting that a skill source must be a directory

#### Scenario: Skill without SKILL.md is rejected

- **WHEN** an asset of type `skill` resolves to a directory containing no `SKILL.md`
- **THEN** resolution fails naming the asset and the missing `SKILL.md`

#### Scenario: Single-file type resolving to a directory is rejected

- **WHEN** an asset of type `persona` resolves to a directory
- **THEN** resolution fails reporting that this asset type requires a single file

#### Scenario: Every non-skill type is held to the single-file shape

- **WHEN** an asset of type `rule`, `instruction`, `command` or `persona` resolves to anything other
  than one regular file
- **THEN** resolution fails naming the asset and its type

### Requirement: Skill Name Match Verification

A `skill` source SHALL be rejected when the `name` field of its `SKILL.md` frontmatter does not match
the id inferred for the asset, because harnesses that read that field drop a mismatched skill
silently. This check SHALL be read-only: content is still copied byte-for-byte and frontmatter is
never rewritten. A `SKILL.md` whose frontmatter is absent or unparseable SHALL fail naming the asset
and the file.

#### Scenario: Mismatched name is a hard failure

- **WHEN** a skill inferred as id `review` has a `SKILL.md` whose frontmatter `name` is `code-review`
- **THEN** resolution fails reporting both the inferred id and the frontmatter name, and the asset is
  not installed

#### Scenario: Matching name resolves unchanged

- **WHEN** a skill's frontmatter `name` equals its inferred id
- **THEN** resolution succeeds and the file's bytes are unmodified

#### Scenario: Unparseable frontmatter is reported

- **WHEN** a skill's `SKILL.md` has no frontmatter or frontmatter that cannot be parsed
- **THEN** resolution fails naming the asset and the file

### Requirement: Content Digest Computation

Resolution SHALL produce a digest for every resolved file and a single digest covering the resolved
source as a whole. The whole-source digest SHALL be computed deterministically from the sorted set of
relative paths and their content digests with file modes normalized, so the same content always
yields the same value regardless of filesystem ordering, fetch order, or platform.

#### Scenario: Identical content yields an identical digest

- **WHEN** the same source content is resolved twice on different platforms
- **THEN** both resolutions produce the same whole-source digest

#### Scenario: Differing file modes do not change the digest

- **WHEN** two resolutions of the same content differ only in file permission bits
- **THEN** both produce the same whole-source digest

#### Scenario: Changed content changes the digest

- **WHEN** any resolved file's content differs
- **THEN** the whole-source digest differs

#### Scenario: Renaming a file changes the digest

- **WHEN** a file within the resolved source is renamed but its content is unchanged
- **THEN** the whole-source digest differs, because paths participate in the computation

### Requirement: Resolution Caching

Fetched archives SHALL be cached by content under the user cache directory, with `HARNAAS_CACHE_DIR`
overriding that location. A cache entry MUST be verified against its expected digest before reuse,
and a corrupt or unreadable entry SHALL be discarded and re-fetched rather than failing the run.
Callers SHALL be able to bypass the cache.

#### Scenario: Second run reuses the cache

- **WHEN** the same repository and commit are resolved again on a later run
- **THEN** the cached archive is reused and no network request is made

#### Scenario: Cache location override is honoured

- **WHEN** `HARNAAS_CACHE_DIR` names a directory
- **THEN** entries are written and read under that directory and the default user cache directory is
  not used

#### Scenario: Corrupt cache entry is refetched

- **WHEN** a cached archive no longer matches its expected digest
- **THEN** the entry is discarded, the archive is fetched again, and resolution succeeds

#### Scenario: Cache can be bypassed

- **WHEN** the caller requests that the cache not be used
- **THEN** content is fetched fresh and the run does not read the cache

### Requirement: Authentication Token Chain

Resolution SHALL take a GitHub token from the first of `HARNAAS_GITHUB_TOKEN`, `GH_TOKEN`,
`GITHUB_TOKEN` that is set, in that order, and SHALL proceed unauthenticated when none is set. An
authorization failure SHALL be a distinct, actionable error naming the variable the token came from,
or naming the chain when no token was found. A token value MUST NOT appear in output, logs, or
anything persisted.

#### Scenario: First set variable wins

- **WHEN** both `HARNAAS_GITHUB_TOKEN` and `GITHUB_TOKEN` are set
- **THEN** the value of `HARNAAS_GITHUB_TOKEN` is used

#### Scenario: No token proceeds unauthenticated

- **WHEN** none of the three variables is set and the repository is public
- **THEN** resolution succeeds without authentication

#### Scenario: Authorization failure names the variable

- **WHEN** a source references a repository the supplied token cannot read
- **THEN** resolution fails reporting that access was denied and naming the environment variable the
  token was read from

#### Scenario: Missing token is named as the chain

- **WHEN** access is denied and no token variable was set
- **THEN** the error names `HARNAAS_GITHUB_TOKEN`, `GH_TOKEN` and `GITHUB_TOKEN` as the ways to
  supply one

#### Scenario: Token value never appears in output

- **WHEN** any resolution failure is reported
- **THEN** no token value appears in the message, the log, or any file written

### Requirement: Offline Resolution

When resolution is requested in offline mode it SHALL satisfy every source from the cache and from
local sources and MUST NOT make any network request, including ref resolution. A source whose content
is not already cached SHALL fail naming that asset and the ref or commit that is missing, rather than
being skipped, treated as unchanged, or resolved to stale content from a different commit.

#### Scenario: Fully cached run succeeds without network

- **WHEN** offline resolution runs and every declared source is present in the cache
- **THEN** every asset resolves and no network request is made

#### Scenario: Uncached source is named

- **WHEN** offline resolution encounters a source whose commit is not in the cache
- **THEN** resolution fails naming that asset and the missing commit, and the report lists every such
  asset rather than stopping at the first

#### Scenario: Unresolvable mutable ref fails offline

- **WHEN** offline resolution encounters a source declaring a branch or tag with no cached resolution
- **THEN** resolution fails naming the asset and the ref, and no remote ref lookup is attempted

#### Scenario: Local sources still resolve offline

- **WHEN** offline resolution encounters a `local` source under `.harnaas`
- **THEN** it resolves from disk exactly as it would with the network available
