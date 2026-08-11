## 1. Finding model, ordering and reporting

- [x] 1.1 Define the finding type: optional asset identifier, optional file path, severity, problem
      statement, and remedy. Model the remedy as either a command alone, or a manifest edit carrying
      the file, the line, the exact current string, the exact replacement string and the follow-up
      command; a finding with no available remedy states that explicitly rather than omitting it.
- [x] 1.2 Define the report type: the finding set, per-host unchecked asset counts, the notes
      recording which checks were skipped and why, and the tallies of errors and warnings.
- [x] 1.3 Implement deterministic ordering by asset identifier then path then check kind, with
      project-level findings that carry no asset placed in a fixed position, so neither manifest nor
      lockfile ordering can affect the output.
- [x] 1.4 Implement the human renderer: group findings by asset, print severity, problem and remedy,
      print any before/after edit verbatim on its own lines, and end with the counts of errors and
      warnings plus every skipped-check note and the unchecked-asset count.
- [x] 1.5 Implement the JSON renderer: one document carrying every finding with its asset, severity,
      problem, remedy and path where applicable, plus the skip and unchecked summary; well-formed with
      an empty finding set; the only thing on standard output, with advisory text on standard error.
- [x] 1.6 Implement the severity-to-exit mapping: `0` with no error-severity finding, `2` with any,
      identical in every output mode.
- [x] 1.7 Implement strict mode promoting warnings for the exit computation only, leaving the printed
      severity of each finding unchanged.
- [x] 1.8 Route a lint failure — unreadable lockfile, unresolvable project root — to the runtime
      failure status, kept distinct from the findings status.

## 2. Manifest and lockfile consistency checks

- [x] 2.1 Implement loading the manifest and the lockfile for a read-only pass, reusing the foundation
      loader and the lenient lockfile decoder rather than reimplementing either.
- [x] 2.2 Implement the manifest check: a load or validation failure becomes exactly one finding, and
      every check depending on the manifest is suppressed and recorded as a skip note.
- [x] 2.3 Implement the collapsed "nothing installed yet" finding for an absent or empty lockfile
      alongside a manifest that declares assets — one finding naming `harnaas install`, never one per
      asset, and never a separate finding about the lockfile's absence.
- [x] 2.4 Implement the declared-but-not-installed check for an asset with no lockfile entry.
- [x] 2.5 Implement the stale-entry check for a lockfile entry whose asset the manifest no longer
      declares or no longer targets, with install named as the remedy since it converges the set.
- [x] 2.6 Implement the manifest-versus-lockfile disagreement check over recorded source, ref and
      type, naming both values, factored so frozen mode reuses it unchanged.

## 3. Installed content integrity checks

- [x] 3.1 Implement the installation walk: for each recorded installation, resolve the destination
      from the recorded scope-relative path plus scope name, and enumerate what is present.
- [x] 3.2 Implement per-file digest recomputation against the recorded digests, reporting each
      modified file individually with its own path and naming the forced install as the remedy.
- [x] 3.3 Implement missing-file detection for a recorded path that no longer exists.
- [x] 3.4 Implement absent-destination collapse: a destination that is gone entirely yields one
      finding for the installation rather than one per recorded file.
- [x] 3.5 Implement extraneous-file detection for a file present under a managed destination that the
      installation record does not list.
- [x] 3.6 Implement the unmanaged-conflict check for a declared asset whose destination exists on disk
      with no lockfile entry claiming it, with the finding stating that install will not overwrite it
      and that `--force` does not change that.

## 4. Managed block and bridge line checks

- [x] 4.1 Regenerate the expected `AGENTS.md` block content from the recorded instruction
      installations using install's block renderer, and compare it against the block on disk — no
      second renderer in the lint package.
- [x] 4.2 Implement marker validation for both managed blocks: absent, duplicated, or a start without
      an end is reported as malformed rather than interpreted.
- [x] 4.3 Implement the rule that content outside the markers is never inspected and never reported as
      drift.
- [x] 4.4 Implement the `.gitignore` block check: every installed path recorded in the lockfile must
      appear in the block, naming any path that is no longer ignored, and reporting entries the record
      does not account for.
- [x] 4.5 Implement the bridge-line check: exactly one `@AGENTS.md` line in `CLAUDE.md` when
      instruction assets are recorded, with a missing file, a missing line and a duplicated line each
      a finding naming the install command and stating that the instruction content is not being read.
- [x] 4.6 Implement the suppression rule: no bridge-line or `CLAUDE.md` finding at all when no
      instruction asset is declared or installed.

## 5. Update detection

- [x] 5.1 Implement ref classification for each asset — explicit commit, version tag, or branch and
      absent ref — since the classification decides every downstream check and whether any request is
      made at all.
- [x] 5.2 Implement the not-reproducible error for a branch or an omitted ref, emitted whether or not
      the ref has moved, and never emitted for a version tag or a commit identifier.
- [x] 5.3 Implement moved-mutable-ref detection: re-resolve the ref and compare against the recorded
      commit, naming the recorded commit, the current commit and the ref.
- [x] 5.4 Implement vanished-ref detection as a finding distinct from an available update, which must
      not claim any newer commit or tag is on offer.
- [x] 5.5 Implement superseded-tag detection: list the repository's tags, order by semantic version,
      report only the highest newer stable tag, ignore non-version tags, and never offer a pre-release
      over a stable installation.
- [x] 5.6 Implement the commit-pin rule: no update finding and no remote request on that asset's
      behalf.
- [x] 5.7 Implement local source change detection by re-reading the source under `.harnaas`,
      recomputing the source digest with install's digest code, and comparing it against the recorded
      value.
- [x] 5.8 Implement deleted-local-source detection as a distinct finding naming the missing path.
- [x] 5.9 Assign error severity to every update-detection finding and assert in code that no flag path
      can downgrade one.

## 6. Remedy edits

- [x] 6.1 Implement locating the manifest line that declares a given asset's source and capturing the
      exact current string, so a remedy can name the file and the line.
- [x] 6.2 Implement rendering the replacement source string for a superseded tag, and for pinning a
      branch to a tag or to the commit the branch currently resolves to.
- [x] 6.3 Implement verbatim printing of both strings so applying the edit is a literal substitution,
      and confirm the follow-up command is always included.
- [x] 6.4 Implement the no-edit path: drift, missing file and changed local source print the command
      alone with no before/after block.

## 7. Resolution caching and network degradation

- [x] 7.1 Implement the resolution cache under the user cache directory, storing ref-resolution and
      tag-listing results with their retrieval time and honouring the cache-directory override.
- [x] 7.2 Implement the bounded freshness window, reusing an entry inside it and resolving again past
      it.
- [x] 7.3 Implement forced refresh bypassing the cache regardless of freshness.
- [x] 7.4 Implement discard-on-corruption: an unreadable or unparseable entry is dropped, the result
      is resolved again, and the run continues.
- [x] 7.5 Implement host-level failure tracking: the first failure marks the host, remaining assets on
      that host are marked unchecked without further attempts, and one finding is reported per host.
- [x] 7.6 Implement the distinct authorization-failure path, naming the token environment variable in
      the finding.
- [x] 7.7 Implement the accounting rule that unchecked assets never count as errors and always appear
      in the summary count.

## 8. Command wiring and modes

- [x] 8.1 Add the lint command constructor with `--frozen`, `--offline`, `--strict`, `--refresh` and
      `--json` registered locally, since the root command carries no persistent flags.
- [x] 8.2 Wire the single pass: load, consistency checks, integrity checks, managed-block checks,
      update detection, render, exit — reusing the resolution and transport built for install.
- [x] 8.3 Implement offline mode: skip every network check, still run local source change and deletion
      detection, and add the skip note that appears even in a clean report.
- [x] 8.4 Implement frozen mode: manifest and lockfile only, reading no destination and making no
      request, reporting an unsatisfied declaration, an undeclared entry, and a source, ref or type
      disagreement.
- [x] 8.5 Define and document the precedence between `--frozen` and `--offline` in the help text, so
      combining them is unambiguous rather than accidental.
- [x] 8.6 Enforce the read-only guarantee structurally: route every check through a read-only
      filesystem handle so no code path in the lint flow can write outside the user cache directory.

## 9. Tests

- [x] 9.1 Add the read-only test: hash the whole project tree and the harness directories before and
      after a run over a project with many findings, and assert byte-for-byte equality.
- [x] 9.2 Add table-driven local-check tests over temporary projects covering drift, missing file,
      absent destination, extraneous file, unmanaged conflict, declared-but-not-installed and stale
      entry.
- [x] 9.3 Add collapse tests: an invalid manifest yields exactly one finding with no cascade; twelve
      declared assets with no lockfile yield exactly one; an empty lockfile yields the same one.
- [x] 9.4 Add managed-block tests: a hand-edited `AGENTS.md` block, edits outside the markers
      producing no finding, a start marker without an end, a `.gitignore` block missing an installed
      path, and a block regenerated from the lockfile matching install's output exactly.
- [x] 9.5 Add bridge-line tests: missing line, duplicated line, missing `CLAUDE.md`, and no finding at
      all when no instruction asset is installed.
- [ ] 9.6 Add update-detection tests against a local remote covering a moved branch, a vanished ref,
      one newer stable tag, several newer tags where only the highest is reported, a pre-release-only
      case, non-version tags, and a commit pin asserted to issue zero requests.
- [x] 9.7 Add not-reproducible tests over a branch that moved, a branch that did not, a version tag
      and a commit identifier.
- [x] 9.8 Add local source tests covering an edited source, a deleted source and an unchanged source,
      each also exercised under offline mode.
- [x] 9.9 Add cache tests: reuse inside the window, refresh past it, forced refresh, and a corrupt
      entry that is discarded without failing the run.
- [x] 9.10 Add degradation tests: an unreachable host reported once for eight assets, a second
      reachable host still checked, an authorization failure naming the environment variable, the
      unchecked count in the summary, and an exit status decided only by the checks that ran.
- [x] 9.11 Add exit-status tests: clean run, warnings only, any error present, strict mode with only
      warnings, an unreadable lockfile, and identical status under `--json`.
- [x] 9.12 Add remedy tests: a superseded tag printing the current and replacement strings verbatim
      plus the install command, a branch pin printing the resolved commit, and a changed local source
      printing the command alone.
- [ ] 9.13 Add frozen-mode tests using instrumented filesystem and transport doubles: a fresh checkout
      passing with zero file reads and zero requests, an asset added to the manifest reported as
      unsatisfied, and a changed ref reported naming both refs.
- [x] 9.14 Add determinism tests: two runs over unchanged state producing identical reports, a
      reordered manifest and lockfile producing identical output, and a valid JSON document with an
      empty finding set.

## 10. Verification

- [ ] 10.1 Run `mise run fmt`, then `mise run lint`, then `mise run test`, re-running lint after any
      formatting change.
- [ ] 10.2 In a scratch project, install assets, then edit one installed file, delete another, add a
      stray file under a managed destination, hand-edit the `AGENTS.md` block and remove the bridge
      line; confirm each is reported once with the correct path, severity and remedy.
- [ ] 10.3 Apply each printed remedy literally — including substituting a printed manifest edit
      character for character — and confirm the following lint run is clean.
- [ ] 10.4 Confirm the exit statuses on that project: `2` with any error, `0` when every asset is
      pinned and current, `2` in strict mode with only warnings, and `1` for an unreadable lockfile.
- [ ] 10.5 Confirm with the network unavailable that local findings are intact, the report states that
      update detection was skipped, and the summary counts the unchecked assets.
- [ ] 10.6 Confirm `--frozen` exits `0` on a fresh clone where nothing has been installed, and `2`
      once an asset is added to the manifest without regenerating the lockfile.
- [ ] 10.7 Confirm that a project pinned to a tag reports an error as soon as a newer stable tag is
      published upstream, and that a branch-tracking asset reports the not-reproducible error even
      while its branch is unmoved.
- [ ] 10.8 Add `harnaas lint --frozen` as a required CI gate on every pull request, and a scheduled
      full `harnaas lint` run so an upstream release reddens the schedule rather than an unrelated
      pull request.
