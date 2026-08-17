## Context

See proposal.md — Why. What the design has to work within:

- `harnaas init` today resolves a selection in `cmd/harnaas/cli/init_select.go` from three origins —
  flag, detection, roster default — explains which origin it used on stderr, and asks the user to
  confirm the resulting sentence through `uiform.Confirm`. `interactive.CanPrompt` decides whether
  asking is allowed at all, and is biased towards "no": it answers no under `go test`, under a coding
  agent, under CI, and whenever either stream is not a terminal.
- The manifest's `harnesses` list is a guarantee about what the declared assets are supported on, not
  an observation about the working tree. An unrecognized id is a validation error for that reason.
- `.harnaas` is an input. `manifest.LocalRoot` names it, the `local` source kind reads it through a
  handle anchored there, and `lint` re-reads it to detect an edited local source. Nothing writes to
  it: install's `MkdirAll` calls are all destination parents, and the roster, the adapters and the
  lockfile never mention it. init's own closing guidance says `harnaas install` creates it, which is
  not true of the shipped install.
- Which types reach which harness is already answered in one place: `planTarget` in
  `cmd/harnaas/cli/targets.go`. It is where the two-tier design lives — a skill goes to the shared
  skills directory, an instruction into the memory file's managed block, and a rule, a command and a
  persona through a named adapter — and where the refusals live too: no adapter, a removed surface,
  a command onto a harness that cannot be silenced (ADR 0005), a scoped rule that would widen if
  emulated as an instruction.
- The five asset types and the directory segments they are inferred from are one table,
  `assetTypeDirectories` in `cmd/harnaas/cli/manifest`: `skills`, `rules`, `instructions`, `commands`,
  `agents`. It is unexported and maps directory to type — the direction inference reads. The order the
  types are listed in is `manifest.AssetTypes()`.

## Goals / Non-Goals

**Goals:**

- The `harnesses` list is only ever what a person chose, in every environment harnaas runs in.
- The whole roster is visible at the moment the choice is made, so choosing does not require already
  knowing an id.
- A project that runs `init` ends up with the local asset layout its selection can actually use, and
  that layout reaches a teammate who clones the repository.
- The set of scaffolded directories cannot disagree with what an install would accept, structurally
  rather than by two lists being kept in step.
- Non-interactive completeness is preserved: every path is reachable with flags alone.

**Non-Goals:**

- Changing what an asset is, where one lands, or anything about `install` and `lint`. The scaffolding
  is empty directories and explanations; nothing in it is installed, recorded or verified.
- A search-as-you-type selection. The roster holds two entries; a list that has to be searched is a
  problem to solve when there is one.
- A `--json` view of `init`. Nothing declares `--json` yet, and a document restating the paths init
  just printed would be a JSON view invented to have one.
- Keeping `harnaas init --yes` working. It is removed rather than redefined; see the decision below.
- Populating the scaffolded directories with example assets. An example asset is content harnaas
  would then be the author of, and the manifest would have to declare it or not — both worse than an
  empty directory that explains itself.

## Decisions

### The roster is listed and nothing is pre-selected

The selection presents every harness on the roster, in the roster's order, each as its display name
with the id the manifest will hold beside it. Nothing is pre-selected.

*Alternative — pre-select what detection found.* This is what the OpenSpec CLI's own `init` does with
the tools it detects, and it is a good fit there: a tool directory is evidence that the user works
with that tool, and the choice being made is which local files to generate. harnaas's field is a
guarantee about which harnesses a team supports the declared assets on, and a pre-ticked box is the
same guess as today's pre-filled sentence in a form that is *harder* to notice: a confirmed sentence
at least says out loud what it detected. Detection would also have to keep answering after the change
that removes it, which means keeping the stat calls, the origins and the sentences that explain them,
for a hint.

*Alternative — keep the old confirm prompt and add a picker behind a flag.* Two ways to answer one
question, one of which is the guess.

Detection is not being deleted from harnaas. `harness.ProjectEvidence` stays where it is and the
adapters keep detecting through it when an install reports what a project uses. What is deleted is
init's own copy of the stat loop, and with it `harnessOrigin`, `explainSelection` and the roster
default's use in `init` — `harness.Default` itself stays, since it is the roster's statement about
itself rather than init's.

### A run that can neither prompt nor read a flag is refused

`--harness` still wins outright and suppresses the prompt. Where it is absent and `CanPrompt` says no,
init fails, naming the flag and every recognized id, and writes nothing.

This is the same shape the OpenSpec CLI takes when it has no signal at all: it refuses and prints the
valid tool ids. The alternative — falling back to `harness.Default` — writes the exact thing this
change exists to stop writing, and writes it in the environment where nobody will read the sentence
saying so. Keeping detection for the no-terminal case only is worse than either: one project would
scaffold two different manifests depending on whether a terminal happened to be attached.

The refusal is not a loss of non-interactive completeness. The flag existed before this change and is
the documented CI path; what changes is that CI must now say which harnesses it means.

### `-y` / `--yes` is removed rather than redefined

The flag means "accept the pre-filled selection without prompting". After this change there is no
pre-filled selection, so the flag has nothing to accept. Redefining it as "do not prompt" would make
it a flag whose only effect is to turn a working interactive run into the refusal above. Accepting it
and ignoring it is what the root command's flag rule exists to forbid — accepting a flag and
honouring it must be the same act.

Removing it is breaking, and deliberately loud: a script that passes `--yes` fails with cobra's
unknown-flag error rather than quietly doing something else. `--harness <id>` is the replacement in
every recipe, and it is what those scripts should have been passing to begin with, since `--yes` was
only ever "agree with whatever was detected".

### The scaffolded set is derived from the install flow's own routing

The question "does this selection earn a `commands/` directory" is exactly the question
"would an install find a destination for a command targeting one of these harnesses", and the second
one already has an implementation. A new `typeReachesHarness(target, assetType, registry)` sits beside
`planTarget` in `targets.go`, expressed in the same helpers it uses — `readsSharedSkills`, the
registry lookup, the adapter's surface and its tier, `suppressesSkillAutoInvocation` — and init unions
its answers over the selection, walking the five types in the manifest's own order.

*Alternative — add `Surfaces() []manifest.AssetType` to the adapter contract and union that.* Rejected
because the adapter is not the whole answer. A command reaches a harness with no command surface
through the skill-emulation path, where ADR 0005's precondition decides it; a skill reaches most
harnesses without consulting an adapter at all. An adapter-only union gets `devin-cli` right today by
coincidence — no command surface *and* no way to silence it — and would get the next harness wrong in
whichever direction the two facts stop agreeing.

*Alternative — probe `planTarget` directly with a synthetic `manifest.Asset`.* This is the most
faithful answer and is how the agreement is *tested*, but not how the init command asks: `Interpret`
is the only route to an `Asset` precisely so that no later phase can invent one, and a hand-built
Asset in the init command would be a second route sitting next to the install flow.

An adapter is asked where an asset lands by being handed the asset, so `typeReachesHarness` cannot
avoid constructing one — the alternative is narrowing the adapter contract, which `adapter.Adapter`
argues against for its own reasons. The probe therefore lives in `targets.go`, beside the routing that
already speaks this language: unexported, carrying only a type and a scope, used by one function, and
never returned. What the rule protects is unaffected — nothing is installed from it, and no later
phase ever sees it.

*Alternative — a table in the init command listing which types each harness takes.* A second place
harnaas answers a question it already answers, which is the drift the roster/adapter split exists to
prevent.

The two are pinned together by a test that walks every (type, harness) pair on the roster and asserts
`typeReachesHarness` agrees with `planTarget`'s `supported()` for an asset of that type with no
content. The probe Asset is built in the test, where inventing one is legitimate. The one deliberate
difference the test encodes: a rule that declares path scoping can be unsupported where an unscoped
one is supported, and the scaffolding question is asked about a directory that has no content yet, so
it is asked with none.

### The directory name comes from the inference table, read backwards

A scaffolded directory is only useful if a file dropped into it and declared by its path infers the
type the author expected, so the name has to be the exact segment inference reads. That table exists,
maps directory to type, and is unexported. The scaffolding needs the other direction, so `manifest`
gains one exported lookup — type to directory — derived from the same map rather than written out a
second time. Deriving it is what keeps "the directory harnaas scaffolds" and "the directory harnaas
infers from" the same string by construction; a second literal would be correct on the day it was
written and is exactly the kind of pair nobody re-checks.

The map is small enough that the reverse lookup can be built at package initialization or walked per
call; either is fine, and neither is a decision worth spending a reader's attention on. What is not
fine is a copy.

### Each created directory carries a `README.md`, and it is the author's from that moment

Git does not track an empty directory. Scaffolding that put nothing in one would be invisible in
`git status`, absent from the commit, and gone on the next clone — so the layout would exist only for
the person who ran `init`, which is the one person who did not need it.

*Alternative — `.gitkeep`.* Survives the clone and teaches nothing. The moment the directory is opened
is the moment the question "what goes in here, and how do I declare it" is being asked, and a
zero-byte file is a refusal to answer it.

*Alternative — a single `.harnaas/README.md` describing all five.* Fewer files, but four of the five
directories are then empty again and drop out of the clone.

The README names the asset type, says what belongs in the directory, and shows the manifest line that
declares an asset from it. No harnaas command reads it, and none rewrites it: it is written once, by
`init`, into a directory that did not exist a moment earlier.

Creation uses `O_CREATE|O_EXCL` rather than the staged-and-renamed atomic write. The atomic helper
cannot express "only if absent" — a rename replaces whatever is there — and for this file, never
touching an author's version outranks the atomicity of a file nothing parses. A partially written
README on a crashed run is a cosmetic loss; a rewritten one is harnaas editing content it does not
own.

### Scaffolding follows the manifest, and only ever adds

Order within a run: validate the flag's names → refuse an existing manifest → refuse a run that
cannot obtain a selection → prompt → write the manifest → scaffold. Everything that can refuse still
happens before the prompt, which is the rule init already holds to. The manifest is written first
because it is the deliverable: asset directories with no manifest declaring what they are for are
scaffolding for nothing, while a manifest with no directories is a complete, valid initialization.

A scaffolding failure after the manifest was written is therefore an error that names the path and the
reason *and* says the manifest exists — the project is initialized, and a re-run completes the rest.
That re-run needs `--force` today, because the manifest now exists; that is a wrinkle the migration
notes and not a reason to invent a second flag.

Scaffolding never removes or rewrites anything, on any flag. `--force` authorizes replacing the
manifest and nothing else, because `.harnaas` holds content the author wrote and harnaas only ever
reads. This is ADR 0001's rule seen from the other side: ownership is recorded in the lockfile, the
lockfile records destinations, and `.harnaas` is never a destination — so nothing scaffolded there can
be managed, and nothing managed can be scaffolded over.

A re-run with a narrower selection leaves directories an earlier selection earned. They may hold the
author's assets, and an empty one costs nothing; removing either would be harnaas deleting from a
directory it does not own.

### Writes go through a handle anchored at the project root

The paths are constants, so containment is not in doubt textually — but `.harnaas` is a live directory
that may be a symbolic link to somewhere else on the machine, and `os.MkdirAll` would follow it. The
scaffolding opens a root at the project root and creates through it, so a `.harnaas` that leads out of
the project is refused by the kernel rather than written through. It is the same rule the `local`
source kind already applies when reading the same directory.

### The multi-select goes through `uiform`

A new `uiform.MultiSelect` is built the way `Confirm` is: through `New`, so it inherits the accessible
wrapper and the base16 theme, renders on stderr, and maps a cancelled prompt and a Ctrl-C to
`ErrCancelled` and `ErrInterrupted` with the context's cause preserved.

The "at least one" rule was planned as the form library's own validation, so the user would fix it in
place. Implementation changed that, for a reason worth recording. The library's accessible rendering
builds a fresh buffered scanner for every question and discards whatever it read ahead, and it answers
a failed validation by asking again from the same reader. Two consequences: a multi-line answer loses
everything after its first line, and a submission that fails validation against an input that has
ended is re-asked and answers nothing, forever — a prompt that cannot be answered spinning rather than
failing, which is the exact failure the `interactive` package exists to prevent. So `uiform` feeds the
accessible form one line per read, and an empty selection is a refusal `MultiSelect` returns
(`ErrNothingSelected`) rather than a validation it re-asks. The refusal behaves identically in both
renderings, and the caller — which is the thing that knows a `harnesses` list must name something —
turns it into the diagnostic naming `--harness`.

### One new ADR

`docs/adr/0006-init-scaffolds-the-authors-input-and-none-of-harnaas-output.md` records the boundary
that replaces the single-file rule: init writes the manifest and the author's input directory, never a
destination harnaas would later claim ownership of. The rule it replaces is stated in `CLAUDE.md` and
in the README as architecture, so it is superseded in writing rather than edited away.

## Risks / Trade-offs

- **A `harnaas init` in CI, or one passing `--yes`, starts failing.** → The failure is loud and names
  the fix: the unknown-flag error for `--yes`, and a refusal naming `--harness` and every recognized
  id for the flagless run. Both are one edit, and both are BREAKING in the proposal, the ADR and the
  release notes.
- **The in-process test suite prompts nowhere.** `CanPrompt` answers no under `go test`, so every
  existing init test that relied on detection now hits the refusal. → That is a migration, not a
  hazard: those tests pass `--harness` and become tests of what they were always asserting. The tests
  that exercise the prompt keep driving it through `HARNAAS_TEST_TTY=1` with `ACCESSIBLE=1`, as the
  signal e2e test already does — and that test's `[Y/n]` sentinel becomes the accessible
  multi-select's own prompt text.
- **A project that only ever installs from GitHub gets four or five README files it did not ask
  for.** → They are additive, deletable, and never recreated unless `init` runs again. A flag to
  suppress them is deferred rather than rejected; see Open Questions.
- **`typeReachesHarness` drifts from `planTarget`.** → The pairing test walks every (type, harness)
  pair on the roster, so a divergence is a failing test at the moment it is introduced rather than a
  directory that quietly stops matching what installs.
- **A scaffolded directory suggests harnaas will install what is put in it.** It will not: an asset is
  installed because the manifest declares it, and a file dropped into `.harnaas/rules/` with no
  manifest entry is inert. → The README in each directory shows the manifest line, which is the whole
  of the answer, at the moment the question is asked.
- **The refusal for a flagless non-interactive run is a worse first experience for someone piping
  `harnaas init` somewhere.** → Accepted deliberately: the alternative is a guarantee nobody chose,
  written into the file a team reviews, discovered later by whoever wonders why an asset installed for
  a harness they do not use.

## Migration Plan

1. Land the ADR first: it is what the code, the README and `CLAUDE.md` all cite afterwards.
2. `uiform.MultiSelect`, with its own tests, before the command uses it.
3. `typeReachesHarness` and the pairing test in `targets.go`, before init depends on it.
4. Rework `init_select.go` — delete detection, the origins and the explanation; keep flag parsing and
   its de-duplication — then `init.go`: the new refusal, the selection call, the scaffolding step and
   the corrected closing guidance.
5. Migrate the in-process init tests to `--harness`, and the prompt-driving ones to the multi-select.
6. `e2e/exit_codes_test.go`: replace `init --yes` with `init --harness claude-code`, add a case for
   the flagless non-interactive refusal, and keep the "not 2" assertion on every case.
   `e2e/signal_test.go`: change the sentinel it waits for.
7. Documentation last, in one pass: the README's `harnaas init` section (what it writes, the flag
   table, the detection section, every worked example), the `.harnaas` layout section, and the
   `CLAUDE.md` paragraphs stating the single-file rule and detection.

There is no rollback concern in the data: no file format changes, and a manifest written by either
version is read identically by both. Rolling back the binary restores the old behaviour with no
migration, and the scaffolded directories stay where they are, inert.

Archive order matters and is stated in the proposal: `add-harnaas-foundation` must be archived before
this change, or its `init-command` delta will overwrite this one's requirements at archive time.

## Open Questions

- Should a later change add a flag that suppresses the scaffolding for projects that install only from
  GitHub? Deferred deliberately: the answer wants evidence that the directories are unwanted, and
  adding the flag later is additive, while removing one is not.
- Should the selection gain a "select every harness" affordance, or search, when the roster is long
  enough to need one? Two entries is not that roster, and the answer will be obvious when it is.
- Should `harnaas init` grow a `--json` view now that it reports several created paths rather than
  one? It is the first command with a result worth structuring, but nothing consumes one yet.
