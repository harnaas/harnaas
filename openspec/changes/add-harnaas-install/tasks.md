## 1. Harness adapter layer

- [ ] 1.1 Define the harness adapter contract: registry name, display name, presence detection, scope
      root resolution, and destination mapping for an asset type.
- [ ] 1.2 Implement the registry with self-registration at package initialization, lookup by name,
      deterministic listing, and a loud failure on duplicate registration.
- [ ] 1.3 Implement scope resolution, with the per-user harness home derived in exactly one place and
      a clear failure when it cannot be determined.
- [ ] 1.4 Implement the `claude-code` adapter: presence detection and destination mapping for
      `skill`, `command`, `subagent` and `rule`.
- [ ] 1.5 Implement the managed rule reference block in the scope's memory file: deterministic
      ordering, byte-for-byte preservation of everything outside the markers, creation when absent,
      and complete removal when no rules remain.
- [ ] 1.6 Implement the unsupported-combination outcome so an unmapped asset type on a harness is
      skipped and reported rather than failing the run.
- [ ] 1.7 Implement destination containment through a handle anchored at the scope root, rejecting
      any destination that would resolve outside it.
- [ ] 1.8 Add the import-boundary test parsing adapter package imports against an allowlist, with a
      failure message directing the author to widen it deliberately, and assert every adapter package
      self-registers.

## 2. Source resolution

- [ ] 2.1 Define the source-kind contract returning resolved files plus provenance, and the registry
      that dispatches on kind.
- [ ] 2.2 Implement GitHub ref resolution over the Git protocol: tag and branch lookup, direct use of
      a full commit identifier, default-branch fallback, and marking mutable refs as such.
- [ ] 2.3 Implement repository archive retrieval over HTTPS keyed by repository and resolved commit,
      fetching at most once per repository and commit per run.
- [ ] 2.4 Implement the transport rules: HTTPS-only with plaintext and loopback rejected including
      after redirect, a bounded redirect chain, a response size ceiling that fails rather than
      truncates, and credential redaction in errors.
- [ ] 2.5 Implement archive extraction: select only the declared subtree, reject entries escaping the
      extraction root, reject link and device entries, and bound per-entry and total size.
- [ ] 2.6 Implement local source reading through a handle anchored at `.harnaas`, refusing symbolic
      links that escape it and reporting a missing path relative to the project root.
- [ ] 2.7 Implement source shape verification per asset type, including the skill definition file
      requirement.
- [ ] 2.8 Implement digest computation: per-file digests plus a whole-source digest over sorted
      relative paths and content, with normalized file modes.
- [ ] 2.9 Implement the content-addressed archive cache under the user cache directory, verifying an
      entry before reuse, discarding and refetching a corrupt entry, and honouring a bypass flag.
- [ ] 2.10 Implement token-based authentication from the environment, a distinct actionable error for
      access denied, and a network failure that never yields empty content.

## 3. Lockfile

- [ ] 3.1 Define the lockfile document types: version, per-asset provenance, and per-target
      installation records with per-file and whole-installation digests.
- [ ] 3.2 Implement lenient decoding that ignores unknown fields, with an explicit failure for an
      uninterpretable version and for malformed JSON that leaves installed files untouched.
- [ ] 3.3 Implement scope-relative destination recording so no absolute path is ever written.
- [ ] 3.4 Implement credential redaction for any recorded URL.
- [ ] 3.5 Implement deterministic serialization with stable key and entry ordering, so identical
      state produces a byte-identical file.
- [ ] 3.6 Implement atomic lockfile writing under the install's exclusive lock.
- [ ] 3.7 Implement the missing-lockfile case as "nothing is managed", so pre-existing destinations
      are treated as unmanaged rather than adopted.

## 4. Install command

- [ ] 4.1 Add `install.go` with the command constructor and its dry-run, force, no-cache and JSON
      flags.
- [ ] 4.2 Implement the resolve phase over all declared assets, collecting failures without aborting.
- [ ] 4.3 Implement the plan phase computing one outcome per asset and target: created, updated,
      unchanged, unmanaged conflict, locally modified, or unsupported.
- [ ] 4.4 Implement unmanaged-path protection, including the rule that force does not override it.
- [ ] 4.5 Implement local-modification detection against recorded digests and the force path that
      restores source content.
- [ ] 4.6 Implement convergence: remove managed destinations for undeclared assets and dropped
      targets, keeping and reporting any that were modified locally.
- [ ] 4.7 Implement atomic application with staging outside the destination, whole-directory
      replacement, and staging cleanup on both success and failure.
- [ ] 4.8 Implement the exclusive lock around the read-modify-write of the lockfile with a bounded
      wait and a clear contention message.
- [ ] 4.9 Implement deterministic ordering by asset identifier for both processing and reporting.
- [ ] 4.10 Implement the record phase writing only what landed on disk, and the non-zero exit when any
      asset failed.
- [ ] 4.11 Implement the human report and the JSON report, keeping advisory text off standard output
      when JSON is requested.
- [ ] 4.12 Implement dry-run so it computes the same outcomes and writes nothing.

## 5. Tests

- [ ] 5.1 Add adapter tests for destination mapping across asset types and scopes, unsupported
      combinations, and containment including a symlinked path component.
- [ ] 5.2 Add rule-block tests: creation, deterministic regeneration, preservation of surrounding
      content, and complete removal when the last rule is dropped.
- [ ] 5.3 Add resolution tests against a local HTTP test server covering redirect limits, plaintext
      and loopback rejection, size ceilings, and credential redaction.
- [ ] 5.4 Add extraction tests with crafted archives containing traversal entries, link entries and
      oversized entries.
- [ ] 5.5 Add digest tests proving path sensitivity, order independence and cross-platform stability.
- [ ] 5.6 Add cache tests covering reuse, corrupt-entry refetch and bypass.
- [ ] 5.7 Add install tests over temporary projects for each outcome state, including that an
      unmanaged file survives a forced run.
- [ ] 5.8 Add convergence tests covering removal of an undeclared asset, a dropped target, and the
      retention of a modified orphan.
- [ ] 5.9 Add idempotence tests asserting a second run reports all-unchanged and leaves a
      byte-identical lockfile.
- [ ] 5.10 Add a determinism test asserting that reordering the manifest changes neither the report
      nor the lockfile.
- [ ] 5.11 Add a concurrency test asserting two simultaneous runs serialize and produce a consistent
      lockfile.
- [ ] 5.12 Add partial-failure tests asserting unrelated assets still install, every failure is
      reported, and the lockfile records only what landed.

## 6. Verification

- [ ] 6.1 Run `mise run fmt`, then `mise run lint`, then `mise run test`, re-running lint after any
      formatting change.
- [ ] 6.2 Install a real skill from a public GitHub repository into a scratch project and confirm the
      destination, the lockfile entry and the resolved commit.
- [ ] 6.3 Edit an installed file and confirm the next run reports it as locally modified and that the
      force flag restores it.
- [ ] 6.4 Create a hand-written file at a declared destination and confirm it survives both a normal
      and a forced run.
- [ ] 6.5 Confirm a dry run against a dirty working tree writes nothing and predicts the outcomes of
      the subsequent real run.
