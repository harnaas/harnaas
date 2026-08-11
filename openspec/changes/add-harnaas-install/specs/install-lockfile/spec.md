## Purpose

Defines `harnaas.lock.json`, the machine-written record of what was actually installed and where it
came from. The manifest configures intent; the lockfile records facts. It is what makes an install
reproducible across a team, what establishes which files `harnaas` owns, and what `lint` reads to
detect drift and available updates.

## ADDED Requirements

### Requirement: Lockfile Location And Role

The lockfile SHALL be a single file named `harnaas.lock.json` at the project root, written by
`harnaas` and intended to be committed alongside the manifest. It SHALL record only observed facts
about completed installs and MUST NOT carry configuration; anything a user would want to change
belongs in the manifest.

#### Scenario: Lockfile is written at the project root

- **WHEN** an install completes successfully
- **THEN** `harnaas.lock.json` exists at the project root recording the installed assets

#### Scenario: Lockfile carries no configuration

- **WHEN** the lockfile is inspected
- **THEN** it contains only recorded install facts, and changing it does not alter what install would
  do beyond what those facts describe

### Requirement: Recorded Provenance

For each installed asset the lockfile SHALL record its identifier and type, the normalized source
including the ref that was requested, the immutable commit that ref resolved to for a remote source,
the whole-source digest, and the time the install completed.

#### Scenario: Remote asset records its resolved commit

- **WHEN** an asset installed from a GitHub source is recorded
- **THEN** its entry carries the requested ref and the commit that ref resolved to

#### Scenario: Local asset records its source path and digest

- **WHEN** an asset installed from a local source is recorded
- **THEN** its entry carries the source path relative to the project root and the whole-source digest,
  and no commit

#### Scenario: Requested ref is preserved alongside the commit

- **WHEN** a source declared a branch ref
- **THEN** both the branch name and the commit it resolved to are recorded, so a later run can tell
  whether the branch has moved

### Requirement: Recorded Installations

Each asset entry SHALL record one installation per target it was installed to, carrying the harness
name, the scope, the destination, and the digest of every installed file together with a digest
covering the installation as a whole. This per-file detail is what allows a later check to name which
file changed.

#### Scenario: Each target is recorded separately

- **WHEN** an asset is installed to two harnesses
- **THEN** its entry contains two installation records, one per harness

#### Scenario: Per-file digests are recorded for a directory asset

- **WHEN** a skill asset comprising several files is installed
- **THEN** the installation record contains a digest for each file, keyed by its path relative to the
  destination

#### Scenario: Installation digest covers the whole destination

- **WHEN** an installation is recorded
- **THEN** it carries a single digest covering all installed files, computed the same way as the
  whole-source digest

### Requirement: Machine-Portable Paths

Recorded destinations SHALL be stored relative to their scope root together with the scope name,
never as absolute paths. A user-scoped installation MUST therefore record the same value on every
machine, so a committed lockfile does not leak or depend on one developer's home directory.

#### Scenario: User-scoped path is machine-independent

- **WHEN** a user-scoped asset is installed on two different machines with different home directories
- **THEN** both lockfiles record the same scope and relative destination

#### Scenario: Absolute path is never recorded

- **WHEN** any installation is recorded
- **THEN** its destination is a relative path and the scope needed to resolve it

### Requirement: Credential Redaction

Any URL recorded in the lockfile SHALL have embedded credentials removed before it is written. A
token or password MUST never appear in the lockfile, which is a committed, world-readable file.

#### Scenario: Credentialed URL is redacted

- **WHEN** a source was resolved from a URL containing embedded credentials
- **THEN** the recorded URL has those credentials removed

### Requirement: Lenient Decoding And Versioning

Because the lockfile is machine-written and rewritten by every version of the CLI, decoding SHALL
ignore fields it does not recognize rather than failing, so a lockfile written by a newer `harnaas`
does not break an older one. The lockfile SHALL still carry a version field, and a version the CLI
cannot interpret at all SHALL be reported rather than silently misread.

#### Scenario: Unknown field is ignored

- **WHEN** the lockfile contains a field written by a newer version of the CLI
- **THEN** it is ignored and the rest of the lockfile is used normally

#### Scenario: Uninterpretable version is reported

- **WHEN** the lockfile declares a version the CLI cannot interpret
- **THEN** the CLI reports that the lockfile was written by a newer `harnaas` and exits non-zero
  rather than acting on a partial reading

#### Scenario: Corrupt lockfile does not destroy the installed set

- **WHEN** the lockfile is not well-formed JSON
- **THEN** the CLI reports the parse failure and exits non-zero without deleting or overwriting any
  installed file

### Requirement: Missing Lockfile Handling

An absent lockfile SHALL be treated as "nothing is managed yet", not as an error. In that state every
existing destination is unmanaged and therefore protected, so a first install into a project with
pre-existing harness files reports conflicts instead of overwriting them.

#### Scenario: First install with no lockfile

- **WHEN** install runs in a project that has no lockfile
- **THEN** it proceeds, treating every pre-existing destination as unmanaged

#### Scenario: Pre-existing files are not adopted

- **WHEN** a destination already exists and no lockfile has ever been written
- **THEN** that destination is reported as an unmanaged conflict rather than being silently claimed

### Requirement: Durable And Deterministic Writing

The lockfile SHALL be written atomically under the same exclusive lock that guards the install, so it
is never observed truncated and concurrent runs cannot interleave. Serialization SHALL be
deterministic — stable key ordering and stable entry ordering — so identical state always produces an
identical file and version control shows no spurious diff.

#### Scenario: Identical state yields an identical file

- **WHEN** the lockfile is written twice from identical install state
- **THEN** both files are byte-for-byte identical

#### Scenario: Interrupted write leaves a readable lockfile

- **WHEN** the process is interrupted while the lockfile is being written
- **THEN** the file on disk is either the previous complete lockfile or the new complete one, never a
  truncated document
