## 1. Record the decision

- [x] 1.1 Write `docs/adr/0006-init-scaffolds-the-authors-input-and-none-of-harnaas-output.md` in the
      existing records' shape — a declarative title, one lead paragraph fusing the problem and the
      decision, then the options rejected and the costs accepted. The decision: `harnaas init` writes
      the manifest and the project's local asset directory, and still writes none of the destinations
      `harnaas install` records ownership of.
- [x] 1.2 State in it why the boundary is ownership rather than a file count: ADR 0001 records
      ownership in the lockfile, the lockfile records destinations, and `.harnaas` is an input harnaas
      reads and never a destination — so nothing scaffolded there can collide with a managed set.
- [x] 1.3 Record the rejected options: keeping the single-file rule and leaving the layout to the
      author, having `harnaas install` create the directories, and scaffolding every asset type
      regardless of the selection.
- [x] 1.4 Record the accepted costs: `init` now writes more than one file, a project installing only
      from GitHub gets directories it may not want, and the scaffolding is deliberately not tracked in
      the lockfile, so nothing verifies it later.

## 2. The multi-select prompt

- [x] 2.1 Add `MultiSelect` to `cmd/harnaas/cli/uiform`, built through the package's own `New` so it
      inherits the accessible wrapper and the theme, reads from the caller's reader and renders on the
      caller's writer.
- [x] 2.2 Give it the same cancellation contract as `Confirm`: a cancelled root context and a Ctrl-C
      both return `ErrCancelled`, with the context's cause preserved where there was one and
      `ErrInterrupted` as the cause where the terminal was in raw mode and never sent a signal.
- [x] 2.3 Enforce "at least one selection" — as a refusal the prompt returns, not as the form
      library's own validation. **Changed during implementation**: the library re-asks a failed
      validation from the same reader, and the accessible rendering builds a new scanner per question,
      so an input that has ended is asked and answers nothing forever. An empty selection therefore
      returns `ErrNothingSelected`, which behaves identically in both renderings and cannot spin.
- [x] 2.4 Feed the accessible form one line per read, for the same reason: it discards whatever its
      per-question scanner read ahead, so without this a multi-line answer loses everything after its
      first line and every later question is answered by end-of-input.
- [x] 2.5 Test it under `ACCESSIBLE=1`: a selection returns what was chosen, several choices return in
      the offered order rather than the ticked one, an empty submission and an exhausted input both
      return `ErrNothingSelected`, and a cancelled prompt returns `ErrCancelled` carrying its cause.

## 3. Which asset types a selection earns

- [x] 3.1 Add one exported lookup to `cmd/harnaas/cli/manifest` returning the directory segment an
      asset type is inferred from, derived from the existing directory-to-type table rather than
      written out a second time.
- [x] 3.2 Test that lookup against the inference table in both directions: every recognized type has a
      directory, every directory infers back to the type it was returned for, and the table has no
      entry the lookup cannot answer.
- [x] 3.3 Add `typeReachesHarness(target, assetType, registry)` beside `planTarget` in
      `cmd/harnaas/cli/targets.go`, answering whether an asset of that type declaring nothing beyond
      its path could reach that harness — expressed in the same helpers `planTarget` uses, not in a
      new table. An adapter is asked by being handed an asset, so the minimal probe lives here too:
      unexported, type and scope only, used by one function and never returned.
- [x] 3.4 Pin the two together: a test that walks every (asset type, roster harness) pair — plus a
      harness with no adapter, which is a supported state — and asserts `typeReachesHarness` agrees
      with `planTarget`'s supportedness for an asset of that type with no content.
- [x] 3.5 State in that test why the probe carries no content: a rule declaring path scoping can be
      unsupported where an unscoped one is supported, and a directory that does not exist yet has no
      content to ask about.
- [x] 3.6 Assert the answers the roster gives today, so a change to either side is visible: every type
      reaches `claude-code`; a command does not reach `devin-cli` while a skill, a rule, an
      instruction and a persona do.

## 4. Selection replaces detection

- [x] 4.1 Delete detection from `cmd/harnaas/cli/init_select.go`: the evidence stat loop, the
      per-harness check, the origin enumeration, the selection struct's origin field and the function
      that explains which origin was used.
- [x] 4.2 Keep flag parsing as it is — a repeated `--harness`, validated against the roster, first
      unrecognized name refused with the roster's own diagnostic, repeats collapsed — and keep the
      display-name renderer the reports use.
- [x] 4.3 Add the selection itself: every roster entry offered in the roster's order, each showing its
      display name and the id the manifest will hold, nothing pre-selected, the result ordered by the
      roster rather than by the order the user ticked boxes.
- [x] 4.4 Add the refusal for a run that can neither prompt nor read a flag: a typed error naming
      `--harness` and every recognized id, phrased as `{problem, fix}` like every other diagnostic.
- [x] 4.5 Confirm `harness.ProjectEvidence` and `harness.Default` are untouched — the adapters still
      detect through the first, and the second is the roster's statement about itself rather than
      something `init` now consults.

## 5. The local asset scaffolding

- [x] 5.1 Add the scaffolding to the flat `cli` package: given the selection, the set of asset types
      at least one selected harness can receive, walked in the manifest's own type order.
- [x] 5.2 Create `.harnaas` and one directory per earned type through a handle anchored at the project
      root, so a `.harnaas` that is a symbolic link out of the project is refused by the kernel rather
      than written through.
- [x] 5.3 Write a `README.md` into each directory it created, naming the asset type, saying what
      belongs there, and showing the manifest entry that declares an asset from it — using the
      directory name the inference lookup returned, so the example and the directory cannot disagree.
- [x] 5.4 Create each README with create-if-absent semantics rather than the staged-and-renamed atomic
      write, and say why in the code: the atomic helper replaces whatever is there, and never touching
      an author's file outranks the atomicity of a file nothing parses.
- [x] 5.5 Return which directories were created and which were already there, so the report can name
      the first set without claiming the second.
- [x] 5.6 Make the failure diagnostic name the path and the reason, and say the manifest was created,
      so the reader knows the project is initialized and a re-run finishes the job.
- [x] 5.7 Log the outcome through the logging package with counts and paths only — no file content, no
      README text.

## 6. The command flow

- [x] 6.1 Remove `-y` / `--yes` from `harnaas init`, leaving no alias and no hidden acceptance: a
      script passing it gets cobra's unknown-flag error.
- [x] 6.2 Order the run so everything that can refuse still happens before the prompt: validate the
      flag's names, refuse an existing manifest without `--force`, refuse a run that can obtain no
      selection, then prompt.
- [x] 6.3 Write the manifest first and scaffold second, so a project never holds asset directories
      with no manifest declaring what they are for.
- [x] 6.4 Scope `--force` to the manifest in code as well as in prose: the scaffolding takes no flag,
      creates only what is missing, and removes and rewrites nothing.
- [x] 6.5 Rework the report on stdout: the created manifest, then the created directories, then what
      to do next.
- [x] 6.6 Correct the closing guidance so it attributes to `harnaas install` only what install does —
      installing declared assets, writing into harness directories, maintaining the ignore-file
      entries — and no longer says install creates `.harnaas`.
- [x] 6.7 Rewrite the command's long help: no detection, the selection and the flag as the two ways to
      choose, and the scaffolding as something init now creates.

## 7. Tests for the command

- [x] 7.1 Migrate every existing in-process init test to pass `--harness`, since the prompt decision
      answers "no" under `go test` and a flagless run is now a refusal.
- [x] 7.2 Test the refusal directly: no flag and no prompt writes nothing, exits non-zero, and names
      both `--harness` and every recognized id.
- [x] 7.3 Test the selection by driving it with the test-TTY variable and accessible mode: a chosen
      set reaches the manifest in roster order, an empty submission cannot complete, and a cancelled
      prompt writes neither the manifest nor any directory.
- [x] 7.4 Test the scaffolded set per selection: a harness with a surface for every type earns five
      directories, a `devin-cli`-only selection earns four with no commands directory, and a selection
      naming both earns the union.
- [x] 7.5 Test that scaffolding only adds: existing directories and existing READMEs are byte-for-byte
      unchanged after a forced re-run, a narrower selection removes nothing, and a partial scaffolding
      is completed rather than recreated.
- [x] 7.6 Test the boundary that has not moved: no harness directory, no `AGENTS.md`, no `CLAUDE.md`
      and no ignore file is created or modified, and none of the scaffolding appears in a lockfile
      after a subsequent install.
- [x] 7.7 Test that a file placed in a scaffolded directory and declared by its path under `.harnaas`
      interprets as the type that directory is for, with no `type` field written.
- [x] 7.8 Update any test that declares the command surface, so the removed flag is removed there too
      rather than silently dropping out.

## 8. The process contract

- [x] 8.1 Update `e2e/exit_codes_test.go`: replace every `init --yes` with an explicit `--harness`,
      keeping each case's expected status and its separate assertion that the status is never `2`.
- [x] 8.2 Add an e2e case for the flagless non-interactive run: exit `1`, and stderr naming
      `--harness`.
- [x] 8.3 Add an e2e case asserting `init --yes` now fails as an unknown flag, so the removal is
      visible in the same table that records every other way this binary can fail.
- [x] 8.4 Update `e2e/signal_test.go`: it waits for the confirm prompt's `[Y/n]` as evidence the
      process is blocked on an answer, and must now wait for the accessible multi-select's own prompt
      text. Keep the two-interrupt assertion and the reason it is a process-level test.

## 9. Documentation

- [x] 9.1 Rewrite the README's `harnaas init` section: what it writes, the flag table without the
      removed flag, the selection in place of the detection section, and the refusal a flagless
      non-interactive run now receives.
- [x] 9.2 Update every worked example in that section — the scaffold-from-detection transcript, the
      non-interactive recipe and the "name the harnesses yourself" recipe — and add one showing the
      scaffolded directories in the output.
- [x] 9.3 Update the README's `.harnaas` layout section to say `harnaas init` creates the directories
      and which ones a selection earns, and to keep saying that harnaas only ever reads what is in
      them.
- [x] 9.4 Update `CLAUDE.md`: the paragraph stating that init refuses before it asks keeps its rule
      and loses detection; the single-file claim is replaced by the boundary the new ADR records; and
      the sentence saying `harnaas install` creates `.harnaas` is corrected wherever it appears.
- [x] 9.5 Note the removed flag and the changed no-terminal behaviour where the project records
      breaking changes for a release, phrased as the edit that fixes each one. The project keeps no
      changelog file — release notes are generated from commit messages — so the user-facing record
      is a short "If you used an earlier version" table in the README's `harnaas init` section, and
      the release half is the commit message's own `BREAKING CHANGE:` trailer.

## 10. Verification

- [x] 10.1 Run `mise run check` — format, lint, then the unit suite — and `mise run test:e2e`, and fix
      what they report rather than suppressing it. `mise` is not on this machine, so the same three
      steps were run directly: `gofmt -s -w .`, `golangci-lint run ./...` at the version `mise.toml`
      pins (2.11.3 — the newer 2.12.2 on this machine reports 122 pre-existing `goconst` findings
      across untouched files, so it is not the version to judge this change by), `go test ./...`, and
      `go test -tags e2e -count=1 ./e2e/...`. All clean.
- [x] 10.2 Re-read the two spec deltas against the implementation and confirm every scenario has a
      test that would fail if the behaviour regressed, including the three that are properties of an
      absence: nothing scaffolded when no manifest is written, nothing added to the ignore file, and
      no lint finding about an empty scaffolding. Four gaps found and closed: a selection whose
      harness has no mapping, the explanations being nobody's dependency, lint's silence about the
      scaffolding, and a scaffolding failure keeping and naming the manifest.
- [x] 10.3 Verify on Linux what Windows cannot run: the signal e2e test is skipped there, and it now
      waits on the accessible selection's own prompt. A cross-compiled binary run under WSL confirms
      the sentinel is printed and the process then blocks on an answer, which is what that test
      depends on — plus the flagged run's four directories for `devin-cli` and the flagless refusal.
