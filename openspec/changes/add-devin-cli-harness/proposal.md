## Why

harnaas recognizes exactly one harness. The roster holds one entry, the adapter registry holds one
adapter, and the README says "version 1 ships exactly one" in five places — while every mechanism
built to make a second one cheap has never carried any weight: the shared skills directory, the
shared memory file, the roster-and-adapter split, and the fallback table for a harness that does not
read the shared location, which is deliberately empty. A tool whose purpose is telling a team what is
in effect across their harnesses has to work for more than one of them, and the design cannot be
trusted to be additive until something has been added. Devin CLI is that second harness, and it is
the right one to add first because it lands almost entirely on the shared paths and diverges in
exactly one place that matters: it has no commands surface at all, and its skill format has no
setting harnaas can write to stop it starting a skill unprompted. That second fact is not a Devin
quirk to be worked around. It is the first evidence that "deliver a command as a skill" was specified
as though every harness could be silenced, and it is cheaper to correct that assumption now, with one
harness meeting it, than after several do.

## What Changes

- `devin-cli` becomes a recognized harness id, accepted wherever a harness is named — the manifest's
  `harnesses` list, an asset's `targets`, and `harnaas init --harness` — and listed in the diagnostic
  that names the ids harnaas recognizes.
- Detection treats the presence of a Devin CLI configuration directory at the project root as the
  whole of the evidence, so `harnaas init` pre-fills a manifest for a project already using it, and
  a project using both harnesses lists them in a deterministic order.
- A named adapter maps the two types Devin CLI has its own surface for: a `rule` and a `persona`,
  each beneath the harness's own directory with the asset id as the file stem and the content
  reproduced byte for byte.
- **A `skill` and an `instruction` reach Devin CLI with no new code at all.** Devin CLI reads the
  shared skills directory and the shared memory file directly, so a skill is written once and not
  copied, an instruction reaches it through the managed block that already exists, and the harness
  needs no import line of its own. The fallback table for harnesses that do not read the shared
  skills directory stays empty, which is the finding rather than an omission.
- **A `command` targeting Devin CLI is reported unsupported, and the reason is stated accurately.**
  Devin CLI has no commands directory; its slash commands are its skills. The existing emulation
  would deliver the command as a skill and write a setting the harness does not read, producing a
  command the harness is still free to start on its own initiative — the exact outcome the command
  type exists to prevent, arrived at through a file that reports success.
- **Emulating a command through a skill surface gains a stated precondition.** harnaas delivers a
  command that way only where it can also write, in that harness's own vocabulary, the setting that
  stops the harness starting it unprompted. Where it cannot, the pairing is reported unsupported
  naming that reason, never as an emulation, and the asset's other targets still install.
- The refusal a harness with no skill surface receives is corrected: it currently claims the harness
  has no skill surface, which for Devin CLI is false, and a diagnostic that misstates the problem
  sends its reader to check something that is not wrong.
- **Devin CLI's per-user layout is answered differently at each layer, and both answers are right.**
  The roster records that it has a per-user location, because a skill installed at `user` scope
  reaches it through the shared per-user skills directory; the adapter offers no per-user root,
  because Devin CLI keeps rules and everything else under two different directories and spells the
  second one differently per platform. A user-scoped `skill` therefore installs, and a user-scoped
  `rule` or `persona` is reported unsupported naming the pairing rather than installed at a guessed
  path.
- **BREAKING**: nothing. Every existing manifest, lockfile and installed destination is unaffected;
  a project that never names `devin-cli` sees no change in behaviour or output.

## Capabilities

### New Capabilities

- `devin-cli-harness`: Devin CLI as a harness harnaas recognizes — the roster identity and display
  name, project detection and its deliberate exclusions, the rule and persona destinations beneath
  the harness's own directory, the absent command surface, the shared routes a skill and an
  instruction already take to reach it, the split per-user layout and how each layer answers it, the
  adapter's own shape and its no-I/O guarantee, and what lint reports about a Devin CLI destination.
- `command-emulation`: when a `command` may reach a harness through that harness's skill surface —
  the precondition that autonomous invocation can be disabled in the harness's own vocabulary, what
  is reported when it cannot, why that outcome is never "emulated", and the requirement that the
  diagnostic never misstates which surface is missing.

### Modified Capabilities

None. `openspec/specs/` is empty — no capability has been implemented yet, so everything this change
touches is new and there is no main spec a delta could modify. Three statements proposed by
`add-harnaas-install` are superseded or narrowed by this change and should be read together with it
until that change is archived: its `harness-targets` requirement that "Version 1 SHALL ship exactly
one named adapter, `claude-code`" and the accompanying scenario that "`claude-code` is the only named
adapter present", both of which are scoped to version 1; and its `asset-rendering` requirement that
"Where a target harness has no command surface, a `command` asset SHALL be rendered as a `SKILL.md`
with model invocation disabled", which assumes disabling is always possible. The `command-emulation`
capability narrows that last one rather than contradicting it — it states the precondition the
requirement already relies on without saying so.

Those three statements are deliberately left where they are, as the record of what `add-harnaas-install`
proposed, and this change edits none of them. The consequence has to be planned for rather than
discovered: archiving `add-harnaas-install` copies its deltas into `openspec/specs/` verbatim, so
archiving it *after* this change would write the superseded text into the main spec as though it were
current. Either archive it before this change is archived, or reconcile the three statements by hand
at that point.

## Impact

- Depends on `add-harnaas-foundation` for the roster, the manifest format and scope validation, on
  `add-harnaas-install` for the adapter contract, the renderer set and the shared targets, and on
  `add-harnaas-lint` for the checks that read a destination. None of those is restated here.
- Adds one roster entry and one adapter package. The adapter is a pure mapping, registered from its
  own package initialization and linked from the one file where linking adapters is decided, so the
  change is additive in the way the registry was built for.
- Changes one existing decision outside the new package: the precondition and diagnostic for
  delivering a command through a skill surface. That path is currently unreachable — no shipped
  adapter offers a skill surface — so nothing installed today changes, and this change is what makes
  it reachable for the first time.
- Adds no runtime dependency, no command, no flag, no environment variable and no cache surface. The
  command tree, the exit-code contract and the lockfile schema are all unchanged; a harness id is
  already a free-form value in a lockfile installation record.
- Records one new architecture decision, that a command is delivered through a skill surface only
  where autonomous invocation can be disabled in that harness's own vocabulary.
- Updates the documentation that currently states harnaas supports exactly one harness: the README's
  harness list, its detection-evidence table, its destination tables and its "where assets land"
  section, the repository's own conventions document, and the adapter package's documentation.
- Widens what the test suite pins: the registered-adapter set is asserted whole, so it changes by
  design rather than by accident, and the roster's ordering rule is exercised by a second entry for
  the first time.
- Makes ADR 0002's shared-target bet checkable. Until now every asset installed for the only
  recognized harness; from this change a skill written once is genuinely read by two harnesses, and
  a destination is genuinely claimed by more than one.
