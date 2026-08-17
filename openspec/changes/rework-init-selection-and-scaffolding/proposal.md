## Why

The manifest's `harnesses` list is a guarantee — "we guarantee the declared assets work for these
harnesses" — and `harnaas init` is the one command that ever writes it. Today it fills that list by
guessing: it stats the roster's evidence paths, pre-fills whatever it found, and asks the user to
confirm a sentence rather than to make a choice. The guess is wrong in both directions and neither is
visible. A `.claude` directory exists in repositories that guarantee nothing and will never run
`harnaas install`; a project that has decided to adopt a harness has left no evidence of it yet, and
is offered the roster's default instead; and a shallow checkout, a fresh clone or a `.gitignore`d
configuration directory changes the answer for one project depending on when init ran. The user is
never shown the roster at all, so the harness they meant to name is one they have to already know the
id of and pass through a flag. Detection is answering the one question the author is best placed to
answer, and hiding the alternatives while it does.

The second problem is that init tells the user something untrue. Its closing guidance says
`harnaas install` creates `.harnaas/` — install never does, and nothing else does either: install
only *reads* local sources from there, and reports a missing one as a resolution failure. The author
of a project-local asset therefore has to create both the directory and the layout inside it by hand,
and that layout is not cosmetic — the first path segment is what harnaas infers an asset's type from,
so a directory spelled `rule` instead of `rules` produces a manifest that fails at the far end of an
install with a message about a type it could not infer. init is the one moment harnaas knows which
harnesses a project targets, and it is the moment that layout can be produced correctly for free.

## What Changes

- **Detection is removed from `harnaas init` entirely.** Nothing is stat-ed, nothing is pre-filled,
  and no harness is selected on the user's behalf. The roster's `ProjectEvidence` stays where it is —
  the adapters detect through it on install, and that use is unaffected.
- **A run that can prompt opens a selection listing every harness on the roster**, in roster order,
  with its display name and id, and the user chooses. The selection is a multi-select because the
  field is a list; an empty selection is refused at the prompt rather than written, because a manifest
  with an empty `harnesses` list declares assets and guarantees them for nothing.
- `--harness` is unchanged in spelling and meaning, and now has one meaning rather than two: it is the
  selection, not an override of a detected one. Passing it suppresses the prompt, as it does today.
- **BREAKING**: a run that cannot prompt and was given no `--harness` is refused, naming the flag and
  every recognized id, and writes nothing. Today the same run scaffolds a manifest from detection, or
  from the roster's default when detection found nothing — a guarantee nobody chose, written into the
  file a team reviews. Every workflow stays completable without a terminal, through the flag that
  already exists.
- **BREAKING**: `-y` / `--yes` is removed. It means "accept the pre-filled selection", and after this
  change there is no pre-filled selection to accept — a flag that is accepted and does nothing is the
  one thing the flag rules here forbid. `--harness <id>` replaces it in every documented recipe, and
  is what CI and coding agents should have been passing all along.
- **`harnaas init` creates `.harnaas/` and the asset-type directories the selection can actually
  receive.** `skills/` and `instructions/` always, because a skill and an instruction reach every
  harness through shared locations; `rules/`, `commands/` and `agents/` where at least one selected
  harness has a surface for that type. A `devin-cli`-only project therefore gets no `commands/`
  directory, because that pairing is refused (ADR 0005) and a directory harnaas would refuse to
  install from is a directory that should not exist.
- **Each created directory gets a `README.md`** naming the asset type, what belongs in the directory,
  and the manifest line that declares one. Git does not track an empty directory, so scaffolding
  without a file in it would be local-only — a layout that never reaches the teammate who clones the
  repository. The README is the author's file from the moment it is written: harnaas never rewrites,
  reformats or deletes it.
- **Scaffolding is additive and never destructive, `--force` included.** A directory that exists is
  left alone, a `README.md` that exists is never overwritten, and nothing under `.harnaas` is removed
  or modified. `--force` is about replacing the manifest and grants nothing here.
- **The "init writes exactly one file" rule is replaced by a narrower one that says what it was
  protecting.** init writes the manifest and the author's own local-asset scaffolding, and still
  creates or modifies no harness directory, no `AGENTS.md`, no `CLAUDE.md` and no ignore file. The
  original rule's own justification — anything init created would be unmanaged, and the next install
  would report a conflict against init's output — is a fact about *destinations*, which are recorded
  in the lockfile and written by install. `.harnaas` is an input harnaas reads and never writes to,
  so nothing scaffolded there can ever be an ownership conflict.
- Records one new architecture decision: init scaffolds the author's input directory and none of
  harnaas's output.

## Capabilities

### New Capabilities

- `local-asset-scaffolding`: the `.harnaas` layout `harnaas init` produces — which asset-type
  directories a set of selected harnesses earns, why a skill and an instruction are unconditional and
  the other three are not, the `README.md` written into each and who owns it afterwards, the rule that
  scaffolding only ever adds, what is reported, and the boundary that keeps this scaffolding out of
  the managed set recorded in the lockfile.

### Modified Capabilities

- `init-command`: harness detection is removed and replaced by an explicit selection over the whole
  roster; the no-terminal-no-flag run becomes a refusal instead of a scaffolded guess; the assume-yes
  flag is removed; and the single-file side-effect rule is narrowed to "the manifest and the local
  asset scaffolding, and nothing else".

`openspec/specs/` is empty — no change has been archived yet — so the text this delta modifies
currently lives in `add-harnaas-foundation/specs/init-command/spec.md` rather than in a main spec.
That has a consequence to plan for rather than discover: archiving copies a change's deltas into
`openspec/specs/` verbatim, so archiving `add-harnaas-foundation` *after* this change would write the
superseded detection requirements back over what this change replaces. Archive
`add-harnaas-foundation` first, or reconcile the two `init-command` deltas by hand at that point.

## Impact

- Depends on `add-harnaas-foundation` for the manifest format, the roster and the prompt rules, and on
  `add-harnaas-install` for the adapter contract that answers which types a harness has a surface for.
  Neither is restated here.
- Changes two files that hold init's behaviour today, `cmd/harnaas/cli/init.go` and
  `cmd/harnaas/cli/init_select.go`, and deletes the detection half of the second. Adds the local-asset
  scaffolding beside them.
- Adds one question to the adapter contract — which asset types this harness has a surface for — so
  that "which directories does this selection earn" is answered from the same table the install flow
  maps destinations with, rather than from a second list in the init command that would drift from it.
  Both shipped adapters answer it from the surfaces map they already hold.
- Adds one form helper to `cmd/harnaas/cli/uiform`, a multi-select, built through the same accessible
  wrapper and theme every prompt goes through. No new dependency: the form library already ships the
  field.
- Removes a flag from the command surface and changes what a flagless non-interactive run does. The
  declared command-surface test, the exit-code table in `e2e/` and every README recipe that runs
  `harnaas init --yes` change with it.
- Leaves `harnaas install`, `harnaas lint`, the lockfile schema, the manifest schema and the exit-code
  contract untouched. A project that already has a `harnaas.json` never runs this code again.
- Documentation: the README's `harnaas init` section — its flag table, its detection section, its
  worked examples and its "what it writes" paragraph — plus the repository's own conventions document,
  which states the single-file rule and the detection behaviour as architecture.
