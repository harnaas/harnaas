## 1. Finding model and reporting

- [ ] 1.1 Define the finding type carrying asset, optional path, severity, problem and remedy.
- [ ] 1.2 Implement deterministic ordering of findings by asset identifier and path.
- [ ] 1.3 Implement the human report: grouped by asset, showing severity, problem and remedy, ending
      with counts of errors, warnings and unchecked assets.
- [ ] 1.4 Implement the JSON report as a single document containing every finding, valid and
      well-formed even when the finding set is empty.
- [ ] 1.5 Implement the severity-to-exit-status mapping: `0` when no error, `2` when any error, and
      the runtime-failure status for a lint failure such as an unreadable lockfile.
- [ ] 1.6 Implement strict mode promoting warnings to errors for exit purposes only.

## 2. Local checks

- [ ] 2.1 Implement the manifest check, reporting a load or validation failure as a single finding and
      suppressing the checks that depend on it.
- [ ] 2.2 Implement the declared-but-not-installed check against the lockfile.
- [ ] 2.3 Implement the stale-lockfile-entry check for assets no longer declared or no longer
      targeting a harness.
- [ ] 2.4 Implement the missing-lockfile case as an informational "nothing installed yet" rather than
      an error.
- [ ] 2.5 Implement content integrity: recompute every installed file's digest and compare against the
      recorded value, reporting modified files individually.
- [ ] 2.6 Implement missing-file and absent-destination detection, collapsing a wholly missing
      destination into one finding.
- [ ] 2.7 Implement extraneous-file detection for files present under a managed destination that the
      installation record does not list.
- [ ] 2.8 Implement the unmanaged-conflict check for a declared asset whose destination exists but is
      unrecorded.

## 3. Update detection

- [ ] 3.1 Implement moved-mutable-ref detection comparing a re-resolved branch or default-branch ref
      against the recorded commit.
- [ ] 3.2 Implement the vanished-ref case as a finding distinct from an available update.
- [ ] 3.3 Implement superseded-tag detection: list tags, compare by semantic version, report only the
      highest newer one, ignore non-version tags, and never offer a pre-release over a stable install.
- [ ] 3.4 Implement the rule that a commit-pinned asset is never reported as outdated and triggers no
      remote lookup.
- [ ] 3.5 Implement the mutable-ref advisory warning suggesting a pin.
- [ ] 3.6 Implement local source change detection by re-reading `.harnaas` and comparing digests, and
      distinguish a deleted local source from a change.
- [ ] 3.7 Implement the resolution cache with a bounded freshness window, forced refresh, and
      discard-on-corruption.
- [ ] 3.8 Implement graceful degradation: mark affected assets unchecked, report a host failure once
      rather than per asset, distinguish an authorization failure, and keep the exit status decided by
      the checks that ran.

## 4. Command wiring

- [ ] 4.1 Add `lint.go` with the command constructor and its offline, strict, refresh and JSON flags.
- [ ] 4.2 Wire the local checks and update detection into a single pass, reusing the resolution and
      transport built for install.
- [ ] 4.3 Enforce the read-only guarantee: no code path in the lint flow writes outside the user cache
      directory.
- [ ] 4.4 Implement offline mode skipping every network check while still running local source change
      detection, and note the skip in the report.

## 5. Tests

- [ ] 5.1 Add a test asserting that a lint run over a project with many findings leaves every project
      and harness file byte-for-byte unchanged.
- [ ] 5.2 Add table-driven tests for each local check over temporary projects: drift, missing file,
      absent destination, extraneous file, unmanaged conflict, not installed, stale entry.
- [ ] 5.3 Add a test asserting an invalid manifest yields exactly one finding with no cascade.
- [ ] 5.4 Add exit-status tests: clean, warnings only, errors present, strict mode, and a lint failure
      distinguished from findings.
- [ ] 5.5 Add update-detection tests against a local remote covering a moved branch, a vanished ref, a
      newer stable tag, several newer tags, a pre-release, non-version tags, and a commit pin.
- [ ] 5.6 Add local source change tests covering an edited source, a deleted source, and running under
      offline mode.
- [ ] 5.7 Add cache tests for reuse within the window, refresh past it, forced refresh, and
      discard-on-corruption.
- [ ] 5.8 Add degradation tests asserting an unreachable host does not fail the run, is reported once
      per host, and that unchecked counts appear in the summary.
- [ ] 5.9 Add report tests asserting deterministic ordering and that the JSON document is valid with an
      empty finding set.

## 6. Verification

- [ ] 6.1 Run `mise run fmt`, then `mise run lint`, then `mise run test`, re-running lint after any
      formatting change.
- [ ] 6.2 In a scratch project, install assets, edit one installed file, delete another, and confirm
      lint reports both with the correct paths, severities and remedies.
- [ ] 6.3 Confirm the reported remedy command actually resolves the finding when run.
- [ ] 6.4 Confirm exit status is `2` with errors present, `0` with only warnings, and `0` in strict
      mode only when there are no findings at all.
- [ ] 6.5 Confirm lint completes with local findings intact while the network is unavailable, and that
      the summary reports the unchecked assets.
- [ ] 6.6 Add `harnaas lint` to the project's own CI workflow as a gate.
