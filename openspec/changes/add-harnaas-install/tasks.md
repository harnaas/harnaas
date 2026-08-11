## 1. Source kind contract, registry and digests

- [x] 1.1 Define the resolved-source type: relative path to content for every file, plus provenance
      (kind, normalized source, requested ref, resolved commit, whether the ref is mutable).
- [x] 1.2 Define the source-kind contract every kind implements, and the registry keyed by kind name
      that dispatches to it so the install flow contains no kind-specific branching.
- [x] 1.3 Make an unregistered kind fail before any network request or filesystem write, naming the
      unsupported kind.
- [x] 1.4 Implement per-file content digests and the whole-source digest over sorted relative paths
      plus content digests, with file modes normalized out of the computation.
- [x] 1.5 Register `github` and `local` from `init()`, and assert in a test that v1 registers exactly
      those two.

## 2. Transport and archive safety

- [x] 2.1 Build the HTTPS client used by every fetch: reject a plaintext or loopback destination
      before the request is sent and on every redirect hop.
- [x] 2.2 Bound the redirect chain and fail reporting too many redirects rather than following
      indefinitely.
- [x] 2.3 Add a size-ceiling body reader that fails reporting the limit instead of truncating, and
      make sure no partial body ever reaches a caller.
- [x] 2.4 Implement the credential-redaction helper and route every error message, log record and
      persisted URL through it.
- [x] 2.5 Implement archive extraction into a root-anchored handle: select only the declared subtree,
      make extracted paths relative to it, and reject entries whose destination escapes the root
      including absolute paths and upward traversal.
- [x] 2.6 Reject symbolic-link, hard-link and device entries in the selected subtree rather than
      materializing them.
- [x] 2.7 Bound per-entry and total extracted size, failing without leaving partially extracted
      content behind.
- [x] 2.8 Report a path missing at the resolved commit as a failure naming the asset, the path and the
      commit.

## 3. GitHub source kind

- [x] 3.1 Implement ref resolution over the Git protocol against the remote, reusing the ambient Git
      credential helpers: tag and branch to commit, a full commit identifier used directly with no
      remote lookup, and an absent ref resolving the default branch.
- [x] 3.2 Report a branch or other moving ref as mutable in the resolution result, so the lockfile and
      lint can tell a pin from a moving target.
- [x] 3.3 Fail an unknown ref naming the asset, the repository and the missing ref.
- [x] 3.4 Implement archive retrieval for a resolved commit over HTTPS, memoized so a repository and
      commit is fetched at most once per run no matter how many assets reference it.
- [x] 3.5 Make every retrieval failure name the asset, the repository and the resolved commit, and
      make sure no failure path can produce an empty resolved source.
- [x] 3.6 Implement the token chain `HARNAAS_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`, then
      unauthenticated, resolved in one place.
- [x] 3.7 Make an authorization failure a distinct error naming the variable the token came from, or
      naming all three when none was set, with the token value never appearing in output or logs.
- [x] 3.8 Implement the content-addressed archive cache under the user cache directory with
      `HARNAAS_CACHE_DIR` overriding it, verifying an entry against its expected digest before reuse.
- [x] 3.9 Discard and re-fetch a corrupt or unreadable cache entry rather than failing the run, and
      add the caller-facing cache bypass.
- [x] 3.10 Implement offline resolution: satisfy every source from the cache, attempt no ref lookup,
      and fail naming every asset and ref that is not cached rather than stopping at the first.

## 4. Local source kind and source verification

- [x] 4.1 Implement local reads through a handle anchored at `.harnaas`, so a symbolic link created
      after validation cannot escape it, and make local sources resolve identically offline.
- [x] 4.2 Report a missing local source naming the asset and the expected path relative to the project
      root.
- [x] 4.3 Implement shape verification per asset type: `skill` must be a directory containing
      `SKILL.md`; `rule`, `instruction`, `command` and `persona` must each be a single regular file.
      Report the asset, its type, the expected shape and what was found.
- [x] 4.4 Add a frontmatter reader that parses without re-serializing, so pass-through is structural
      rather than a rule someone has to remember.
- [x] 4.5 Implement skill name-match verification against the inferred id, failing with both names in
      the message, and failing separately when frontmatter is absent or unparseable. The check must
      read only — no bytes are rewritten.

## 5. Harness target contract, registry and scope

- [x] 5.1 Define the adapter contract: registry identity, presence detection, a root per supported
      scope, a destination for an asset type and scope, a support tier per surface, and the per-type
      render hook.
- [x] 5.2 Make "no surface for this type" a value the adapter returns rather than an invented path.
- [x] 5.3 Implement the registry with `init()` self-registration, lookup by identity, and a listing
      whose order is deterministic and independent of registration order.
- [x] 5.4 Fail loudly at startup on duplicate registration, and make an unknown harness error name it
      and list the registered ones.
- [x] 5.5 Implement scope resolution: `project` by default, `user` only where the adapter declares an
      unambiguous per-user root.
- [x] 5.6 Make `user` scope on an adapter that does not offer it a validation error naming the asset
      and the harness, refused before any write, with no silent fallback to `project`.
- [x] 5.7 Confirm the adapter layer relies on manifest validation to reject an `instruction` asset at
      `user` scope, and does not re-implement that check.
- [ ] 5.8 Implement destination containment through a handle anchored at the scope root, rejecting an
      escaping destination and surviving a path component swapped between validation and use.
- [ ] 5.9 Implement the support-tier gate: `live` writes with no note, `removed` is refused naming the
      replacement surface, `gated` and `legacy` write with notes carried into the install report.
- [ ] 5.10 Implement the `unsupported` outcome for an asset type with no destination on a target, so
      the asset's other targets still install and the run continues.
- [x] 5.11 Add the AST import-boundary test: parse each adapter package's imports against an explicit
      allowlist, fail naming the offending package and import, and assert in the same test that every
      adapter package registers itself.

## 6. Shared targets and the claude-code adapter

- [ ] 6.1 Implement the shared skill target `.agents/skills/<id>/SKILL.md`, written once regardless of
      how many targeted harnesses read it, with no duplicate per-harness copy.
- [ ] 6.2 Route `instruction` assets to the shared memory-file target rather than to any per-harness
      surface.
- [ ] 6.3 Make `skill` and `instruction` install for a harness that has no named adapter registered.
- [ ] 6.4 Encode, in exactly one table, which harnesses are known not to read the shared skills
      directory, and add the per-harness fallback copy only for those.
- [x] 6.5 Implement the `claude-code` adapter: presence detection from observable evidence with no
      side effects, project and user scope roots, and support tiers per surface.
- [x] 6.6 Map `rule` to `rules/<id>.md`, `command` to `commands/<id>.md` and `persona` to
      `agents/<id>.md` beneath the resolved scope root, using the asset id as the file stem.
- [x] 6.7 Assert that rule files are standalone: nothing references them from `CLAUDE.md`, `AGENTS.md`
      or any managed block.

## 7. Managed blocks

- [x] 7.1 Implement a reusable marker-delimited block editor: locate, replace, insert and remove a
      block, preserving everything outside the markers byte-for-byte and creating the file when it is
      absent.
- [x] 7.2 Implement the `AGENTS.md` instruction block: every `instruction` asset inlined verbatim,
      sorted by asset id, each preceded by an HTML comment naming the asset and its source.
- [x] 7.3 Remove the instruction block and both markers when the last instruction asset goes, leaving
      the rest of `AGENTS.md` intact.
- [x] 7.4 Implement the `CLAUDE.md` bridge line: ensure exactly one `@AGENTS.md` line appended at the
      end, create the file containing only that line when absent, and never duplicate it on re-run.
- [x] 7.5 Remove the bridge line when no instruction assets remain, deleting `CLAUDE.md` only when the
      file then contains nothing else.
- [x] 7.6 Implement the version-control ignore block listing exactly the installed paths, one entry
      per path, regenerated on every install and pruned as convergence removes destinations.
- [x] 7.7 Reject coarse directory ignores in that block, so a hand-written asset beside an installed
      one stays tracked.

## 8. Asset rendering

- [x] 8.1 Define the renderer contract as a deterministic function of source content and target, and
      name every renderer including the ones v1 does not implement.
- [x] 8.2 Wire renderer selection through the adapter's per-type hook, so the install flow never
      branches on a harness name, and default to identity where no hook is declared.
- [x] 8.3 Implement the identity renderer: bytes reproduced exactly, including frontmatter, line
      endings and trailing whitespace, with no normalization or re-encoding, file by file across a
      skill directory.
- [x] 8.4 Implement the `as-skill` renderer: a `command` becomes a `SKILL.md` with model invocation
      disabled, keeping the command's id and body and changing no other frontmatter key.
- [x] 8.5 Prefer a native command surface where the harness has one, so `as-skill` applies only when
      there is no alternative.
- [x] 8.6 Make selecting a declared-but-unimplemented renderer report `unsupported` naming the
      renderer, write nothing, and never fall back to identity.
- [x] 8.7 Implement rule-to-instruction emulation for a `rule` with no path scoping, and report a
      `rule` declaring `paths:` as `unsupported` naming the path scoping as the reason.
- [x] 8.8 Report every emulated installation with the `emulated` outcome plus a statement of how it
      differs from native support, in both the text and JSON reports, and never as `created`,
      `updated` or `unchanged`.
- [x] 8.9 Record emulation in the lockfile so a later run and lint can tell an emulated installation
      from a native one.
- [x] 8.10 Enforce the hard limits against rendered output, not source: 1 MB per `SKILL.md`, 4 MiB per
      memory file checked during planning, and an import depth of 5. Name the limit, the measured
      value and the asset, and write nothing for it.
- [x] 8.11 Warn at roughly 40,000 characters of assembled always-on content per scope, naming the
      total and the largest contributors, without failing the run or changing any byte installed.

## 9. Lockfile

- [x] 9.1 Define the lockfile document types: version, per-asset provenance, and per-target
      installation records.
- [x] 9.2 Record per asset the id, type, normalized source, requested ref, resolved commit for a
      remote source, source digest and install time — keeping the requested ref after resolution.
- [x] 9.3 Record per installation the harness, scope, destination, a digest per installed file keyed
      by its path relative to the destination, and an installed digest over the installation as a
      whole.
- [x] 9.4 Keep the source digest and the installed digest independent, so a rendered installation's
      two differing digests are never read as drift.
- [x] 9.5 Store destinations relative to the scope root together with the scope name, so no absolute
      path is ever written and a user-scoped entry is identical on every machine.
- [x] 9.6 Treat the harness field as attribution rather than exclusive ownership: the same destination
      may appear under several harnesses, and it is removed only when no recorded harness claims it.
- [x] 9.7 Redact credentials from any URL before it is written.
- [x] 9.8 Implement lenient decoding that ignores unknown fields, an explicit report for an
      uninterpretable version, and a parse failure that exits non-zero without deleting or overwriting
      any installed file.
- [x] 9.9 Treat an absent lockfile as "nothing is managed" rather than an error, so every pre-existing
      destination is unmanaged and protected.
- [x] 9.10 Implement deterministic serialization with stable key and entry ordering, so identical
      state produces a byte-identical file.
- [x] 9.11 Write the lockfile atomically under the install's exclusive lock.

## 10. Install command

- [x] 10.1 Add `install.go` with the command constructor and its `--dry-run`, `--force`, `--offline`,
      cache-bypass and `--json` flags, registered locally rather than as persistent flags.
- [x] 10.2 Implement the resolve phase over every declared asset, collecting failures instead of
      aborting, with no harness destination touched while it runs.
- [x] 10.3 Implement the plan phase producing exactly one of the seven outcomes per asset and target:
      `created`, `updated`, `unchanged`, `emulated`, `conflict-unmanaged`, `conflict-drift`,
      `unsupported`.
- [x] 10.4 Attach a runnable remedy — the command or manifest edit that resolves it — to every outcome
      that blocked or altered an install.
- [x] 10.5 Implement unmanaged-path protection: a destination not recorded in the lockfile is never
      overwritten, replaced or deleted, and `--force` does not change that.
- [x] 10.6 Implement drift detection against the recorded installed digest, and the `--force` path
      that restores resolved source content to drifted managed destinations only.
- [x] 10.7 Implement convergence: remove managed destinations for assets no longer declared or no
      longer targeting a harness, only where content still matches, keeping and reporting a drifted
      orphan.
- [x] 10.8 Make an emptied `assets` array remove every managed destination and both managed blocks and
      leave the lockfile recording no installations, and document it as the full uninstall.
- [x] 10.9 Implement atomic application: stage outside the destination and land by rename, replace a
      directory destination as a unit, and clean up staging on both success and failure.
- [x] 10.10 Implement the exclusive lock covering the lockfile read-modify-write, with a bounded wait
      and a clear contention message instead of blocking indefinitely.
- [x] 10.11 Process and report assets in a stable order derived from asset id, independent of manifest
      order and filesystem iteration order.
- [x] 10.12 Implement the record phase writing only what actually landed, and a non-zero exit when any
      asset failed while the rest still installed.
- [x] 10.13 Implement the human-readable report including tier and threshold notes, and the single
      JSON document covering every asset, target, outcome, destination and remedy, with advisory text
      on standard error so standard output stays valid JSON.
- [x] 10.14 Implement `--dry-run` so it computes the same outcomes, still reports resolution failures
      and exits non-zero for them, and writes to no destination, lockfile or managed block.
- [x] 10.15 Wire `--offline` through to offline resolution and make a missing cache entry name the
      source and ref rather than falling back to the network.

## 11. Tests

- [ ] 11.1 Registry tests: deterministic listing, duplicate registration failing loudly, unknown
      harness listing the registered ones, and exactly two source kinds in v1.
- [ ] 11.2 Adapter tests: destination mapping across every asset type and scope, missing surfaces
      reported rather than invented, user scope refused where unoffered, and containment holding when
      a path component is swapped for a link.
- [ ] 11.3 Support-tier tests: a `removed` surface refused naming its replacement, `gated` and
      `legacy` written with their notes present in the report, `live` written silently.
- [ ] 11.4 Transport tests against a local HTTP test server: plaintext and loopback rejection
      including after redirect, redirect limit, size ceiling failing rather than truncating, and
      credential redaction in the error.
- [ ] 11.5 Extraction tests with crafted archives: traversal entries, absolute-path entries, symbolic
      and hard links, device entries, oversized entries and an oversized total.
- [ ] 11.6 Resolution tests: tag, branch and commit refs, default-branch fallback, unknown ref
      message, one archive fetch shared by several assets, and a network failure that never yields
      empty content.
- [ ] 11.7 Auth tests: precedence across the three variables, unauthenticated success, an
      authorization failure naming the right variable, and no token value in any message or file.
- [ ] 11.8 Cache tests: reuse with no network request, `HARNAAS_CACHE_DIR` honoured, corrupt entry
      discarded and re-fetched, and bypass.
- [ ] 11.9 Offline tests: a fully cached run with no network, an uncached source naming every missing
      asset rather than the first, no ref lookup attempted, and local sources still resolving.
- [ ] 11.10 Verification tests: each shape mismatch per type, a skill missing `SKILL.md`, a name
      mismatch failing with both names, and absent or unparseable frontmatter.
- [ ] 11.11 Digest tests: identical content to identical digest across platforms, mode differences
      ignored, content change and rename both changing the digest.
- [ ] 11.12 Rendering tests: identity preserving line endings and trailing whitespace, `as-skill`
      changing only the invocation key, unknown frontmatter keys surviving in order, `paths:` never
      rewritten, and an unimplemented renderer reporting `unsupported` with nothing written.
- [ ] 11.13 Emulation tests: an unscoped rule emulated and reported as such, a path-scoped rule
      reported `unsupported`, a native rules directory winning over emulation, and an emulated
      installation still reported as emulated on re-run.
- [ ] 11.14 Limit tests: an oversized `SKILL.md` failing, an oversized assembled memory file failing
      during planning with the existing file untouched, excessive import depth failing with the chain
      named, limits measured on rendered output, and the soft threshold warning without affecting the
      exit code.
- [ ] 11.15 Managed block tests for `AGENTS.md`, `CLAUDE.md` and the ignore file: creation when
      absent, deterministic regeneration, surrounding content preserved byte-for-byte, block removal
      with the last instruction, bridge line not duplicated, `CLAUDE.md` deleted only when empty, and
      per-path ignore entries with no directory ignore.
- [ ] 11.16 Install outcome tests over temporary projects covering all seven outcomes, including an
      unmanaged file surviving a forced run and a drifted destination restored by force.
- [ ] 11.17 Convergence tests: an undeclared asset removed, a dropped target removed, a modified
      orphan kept and reported, a shared destination surviving one harness dropping out, and an
      emptied `assets` array removing everything except what is protected.
- [ ] 11.18 Idempotence and determinism tests: a second run reporting all `unchanged` with no
      rewrite, a byte-identical lockfile, and reordering the manifest changing neither report nor
      lockfile.
- [ ] 11.19 Atomicity tests: an interrupted destination write leaving old or new content but never a
      mixture, a directory update never left half-applied, and no staging artefact surviving a
      failure.
- [ ] 11.20 Concurrency tests: two simultaneous runs serializing into a consistent lockfile, and lock
      contention reported within the bounded wait rather than hanging.
- [ ] 11.21 Partial-failure tests: unrelated assets still installing, every failure reported rather
      than the first, a non-zero exit, and the lockfile recording only what landed.
- [ ] 11.22 Lockfile tests: unknown fields ignored, an uninterpretable version reported, malformed
      JSON destroying nothing, absolute paths never written, credentials redacted, and an absent
      lockfile treated as nothing managed.
- [ ] 11.23 JSON output tests: a single well-formed document covering every outcome, and advisory text
      going to standard error without corrupting it.

## 12. Verification

- [ ] 12.1 Run `mise run fmt`, then `mise run lint`, then `mise run test`, re-running lint after any
      formatting change.
- [ ] 12.2 Install a real skill from a public GitHub repository into a scratch project; confirm the
      shared destination, the lockfile entry, the resolved commit and the ignore-block entry.
- [ ] 12.3 Declare an `instruction` asset and confirm the `AGENTS.md` block content, its HTML comment
      provenance, and a single `@AGENTS.md` line in `CLAUDE.md`; then remove it and confirm both
      disappear.
- [ ] 12.4 Edit an installed file and confirm the next run reports `conflict-drift` with a remedy, and
      that `--force` restores it.
- [ ] 12.5 Hand-write a file at a declared destination and confirm it survives both a normal and a
      forced run, reported as `conflict-unmanaged` each time.
- [ ] 12.6 Confirm a dry run against a dirty working tree writes nothing and predicts the outcomes of
      the subsequent real run exactly.
- [ ] 12.7 Empty the `assets` array, run install, and confirm complete removal of managed
      destinations, both managed blocks and every lockfile installation.
- [ ] 12.8 Run install with the network disabled, once fully cached and once with a missing entry, and
      confirm the success and the named failure.
- [ ] 12.9 Confirm no token value appears in the log file, the lockfile or any error, and that the log
      contains no user content.
