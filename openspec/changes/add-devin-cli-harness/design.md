## Context

See `proposal.md` — Why. What matters for the approach is the state of the code this change arrives
into. harnaas already has every seam a second harness needs and has never used one: the roster is a
data-only list keyed by id, the named adapters register themselves and are linked from one reviewed
file, the two shared destinations are computed without consulting an adapter at all, and the table
naming harnesses that do not read the shared skills directory exists and is empty. So the shape of
this change is settled before it starts — one roster entry, one adapter package, one blank import —
and the only real work is deciding what Devin CLI's surfaces are and what to do about the one asset
type it has no surface for.

The surfaces were taken from Cognition's published documentation for Devin for Terminal rather than
from observation, and the parts that decide this design are: it reads the shared skills directory
under both the project root and the user's home and says so explicitly; it reads the shared memory
file natively; it keeps rules and personas under its own directory; it has no commands directory,
because a slash command there *is* a skill; and a skill's invocation modes default to both "the user
typed it" and "the model decided", with no setting named in the vocabulary harnaas's existing
command emulation writes.

## Goals / Non-Goals

**Goals:**

- Recognize `devin-cli` everywhere a harness is named, with detection good enough for `init` to
  scaffold a manifest for a project already using it.
- Place a `rule` and a `persona` natively beneath the harness's own directory.
- Leave the shared routes untouched: a skill and an instruction must reach Devin CLI without a line
  of harness-specific code, which is the claim ADR 0002 and ADR 0003 make and which this change is
  the first opportunity to test.
- State the precondition that command emulation has always depended on, and make the refusal that
  follows from it name the surface that is actually missing.
- Keep the change additive: nothing installed for `claude-code` today may move, and no output for a
  project that never names `devin-cli` may differ.

**Non-Goals:**

- Cloud Devin. Playbooks, Knowledge and Secrets are account-level features the CLI explicitly does
  not read, and harnaas manages files in a repository and a home directory.
- The legacy directories Devin CLI also reads for compatibility with other tools. harnaas writes to
  the surface a harness documents as its own; writing to another harness's directory because a third
  one happens to read it is how one asset acquires several homes nobody can reason about.
- Translating frontmatter between harnesses. A rule authored for one harness's vocabulary installs
  verbatim, and making harnaas rewrite it would require a writer for a format whose quoting decides
  what the document means.
- A renderer that speaks Devin CLI's own skill vocabulary. That is what would make command emulation
  possible here, and it is deliberately deferred — see Open Questions.
- Extending the adapter contract so a harness can name more than one per-user root. Devin CLI would
  need it; one harness needing it is not yet evidence the contract is wrong.

## Decisions

### Devin CLI is a second harness, not a second architecture

Every mechanism this change touches was built to take another entry. The roster is a sorted literal
of plain data; the adapter registry keys on the harness id and panics only on a duplicate; the
install flow obtains a destination without branching on which harness it is addressing. The whole
implementation is therefore a roster entry, a package that answers four questions, and one blank
import — and the one thing that is *not* additive is called out separately below rather than smuggled
in beside them.

The alternative worth naming is generalizing while adding: introducing a per-harness table for the
memory-file bridge, or a scope-root abstraction, because Devin CLI shows both would eventually be
useful. Both are rejected on the same ground the extraction rule rests on — the second instance is
what tells you the shape of the abstraction, and this change has one instance of each.

### The id names the terminal tool rather than the product

`devin-cli`, not `devin`. Cognition ships an agent under the Devin name that harnaas does not manage
and cannot manage: its Playbooks, Knowledge and Secrets are not files in anybody's repository, and
the CLI's own documentation says it does not read them. A manifest's `harnesses` list states a
guarantee, so the id has to name the thing the guarantee is about. `devin` would read as the broader
claim, and the narrower spelling costs a manifest author four characters.

It also sorts after `claude-code`, so the roster's ordering rule takes its first real entry without
the existing one moving.

### The configuration directory is the whole of the evidence, because the memory file belongs to everybody

Detection reads observable state a harness left behind, and the only such state unique to Devin CLI
is its own configuration directory. The shared memory file is read by 21 of the 23 harnesses ADR 0003
surveyed; treating it as evidence would report Devin CLI as detected in every project that has ever
written one, and `init` would scaffold a guarantee nobody asked for.

The check is deliberately not a resolution: a configuration directory that is a symbolic link into a
tree that is not checked out is still evidence the harness was used here, and following it would
answer "no" for a project that plainly says yes. That is the rule the existing harness already
applies, and detection reads the roster's declared evidence rather than keeping a second list, so
`init` scaffolding a manifest and an install reporting a harness cannot disagree about one project.

### Devin CLI reads the shared skills directory, so the fallback table stays empty

Devin CLI documents the shared skills directory as a supported location at both project and user
scope, in as many words. ADR 0002 chose to write there first precisely so that most harnesses need no
per-harness skill destination, and this is the first harness added since to test the claim. The table
of harnesses that do not read it therefore stays empty — which is a finding rather than an omission,
because the table exists so that the day a harness *does* need an entry, that is the single edit.

The consequence ADR 0002 already accepted becomes real here for the first time: one skill file is now
genuinely claimed by two harnesses, so removing one harness from an asset's targets must not remove a
file the other still reads. The lockfile records the claim per harness, which is what makes that
answerable rather than guessed.

### The instruction block reaches Devin CLI with no bridge and no change

Devin CLI reads the project's memory file natively, so the managed block described by ADR 0003
reaches it with nothing added. The bridge line that exists for the harness which does not read that
file stays exactly as it is — it is keyed on a filename rather than on a harness id, it is written
whenever any instruction is installed, and generalizing it into a per-harness table now would be
building an abstraction with one member for a harness that does not need it.

### A persona installs as a flat file rather than as a directory

Devin CLI reads both `agents/<id>.md` and `agents/<id>/AGENT.md`. Where a harness offers two
spellings harnaas picks the one a reader can act on: the install report names a path, and a path to a
file is something to open, while a path to a directory is something to go looking inside. The flat
form also matches the shape of every other single-file asset harnaas installs, so a rule, a command
and a persona are all "one file whose stem is the asset id" rather than three different answers.

### Devin CLI has no command surface, and emulating one would re-create what the command type exists to prevent

This is the only decision in the change that is not a table edit. A command is invoked deliberately
and a skill is started by the harness when a description matches, so the existing emulation delivers
a command as a skill *and* writes the setting that stops the harness starting it — the disabling is
the whole of the renderer rather than a detail of it. Devin CLI's skill format has no such setting in
the vocabulary that renderer writes, and both of its invocation modes are on by default, so the
emulation would produce a file that reports success and leaves the harness free to run somebody's
deploy command because the conversation mentioned deploying.

So the precondition becomes explicit: a command reaches a harness through its skill surface only
where harnaas can also silence it there. Devin CLI cannot be silenced today, so a command targeting
it is reported unsupported. Recorded in `docs/adr/0005-a-command-is-emulated-only-where-it-can-be-silenced.md`.

Two alternatives were considered. **Emulate anyway and note the caveat in the report** — rejected
because the note is read once and the file is read forever, and a tool whose purpose is telling a
team what is in effect must not install something whose behaviour contradicts what it says.
**Write the harness's own suppression key from this change** — rejected as scope: it needs a second
renderer, a way for a surface to declare which one it wants, and a decision about what an emulated
command in the *shared* skills directory means when several harnesses read it and only one of them
understands the key. That is a change of its own, and the precondition is what makes deferring it
safe.

Correcting the existing refusal is part of the same decision. It currently tells the reader the
harness has no skill surface, which for this harness is false; a diagnostic that misstates the
problem sends its reader to check something that is not wrong, which is worse than one that says
less. The refusal path is unreachable today — no shipped adapter offers a skill surface — so nothing
installed changes, and this change is what makes it reachable for the first time.

### The roster says Devin CLI has a per-user location and the adapter says it has no per-user root, and both are right

Devin CLI keeps per-user rules under one home directory and per-user everything-else under another,
and spells the second differently per operating system. There is no single root a destination could
be counted from, so the adapter offers none, and a user-scoped rule or persona is refused rather than
installed somewhere its author would have no reason to look.

The roster answers a different question and answers it `true`. Its flag decides whether an asset may
declare `user` scope for this target at all, and a user-scoped *skill* reaches Devin CLI perfectly
well through the shared per-user skills directory, which the harness documents. Recording `false`
would refuse something that demonstrably works, in order to be tidy about something that does not.
The contract already anticipates the two layers disagreeing — the roster is a fact the manifest layer
can check without linking a single adapter, the adapter is the thing that knows its own directories,
and where they disagree the adapter is right — so this is the designed behaviour rather than a hole
in it.

The cost is stated plainly: a user-scoped rule for Devin CLI is refused at install time rather than
during manifest validation, which is later and names the pairing rather than the line. That is the
trade the two-layer design makes, and it is the correct side of it here because the alternative
refuses working installs.

### Frontmatter is never translated, so a rule authored for another harness stays the author's problem

A rule installs byte for byte. Devin CLI's rule frontmatter uses its own keys, and a rule authored
for a different harness will land intact and be read as an unscoped rule rather than as the scoped
one it was written to be. harnaas does not fix that, for the reason it has no frontmatter writer at
all: re-encoding a document whose quoting decides what it applies to is how a tool silently changes
what a team declared. The manifest names a source, and choosing a source whose content suits the
targets is the author's decision to make.

## Risks / Trade-offs

- **Devin CLI's surfaces are documented rather than observed.** A published path can move, and
  harnaas would keep writing to it, producing files nothing reads. → The support tier travels with
  every destination for exactly this reason: the day a surface becomes legacy or removed, the edit is
  one field in one table and the install report changes with it. A removed surface is refused rather
  than written, because writing there is a no-op that looks like success.
- **The user-scope split moves a refusal from validation to install.** A user-scoped rule targeting
  Devin CLI passes manifest validation and is refused later. → The later diagnostic names the asset,
  the harness and the scope, and the alternative — recording no per-user location at all — would
  refuse user-scoped skills that work.
- **The shared skills directory becomes a genuinely multi-harness ownership problem.** Until now
  every claim came from one harness. → ADR 0001 already settles it: ownership lives in the lockfile,
  a destination is removed only when no recorded harness still claims it, and a file harnaas did not
  install is an unmanaged conflict reported and never overwritten.
- **An asset installed for one harness is visible to the other anyway.** Devin CLI reads the other
  harness's persona directory and memory file directly. → ADR 0002 accepted this: the `harnesses`
  list means "which harnesses we guarantee", never "which harnesses will see this". It is why an
  unrecognized id is an error and why visibility to an unlisted harness is not.
- **`command` support for Devin CLI is a real gap, not a technicality.** A team that declares
  commands and adds this harness gets an unsupported pairing per command. → It is reported per
  pairing with its reason, every other target still installs, and the remedy is a change harnaas can
  make later without any manifest edit.

## Open Questions

- Whether a later change adds a renderer speaking Devin CLI's own skill vocabulary, so a command can
  be emulated after all. Deferring is safe because the precondition specified here is what such a
  renderer would satisfy — adding one changes an unsupported pairing into an emulated one and touches
  no requirement written now.
- Whether Devin CLI's split per-user layout ever justifies letting an adapter name more than one
  per-user root, or a root anchored somewhere other than the home directory. One harness is not
  enough to know the shape, and today's answer refuses rather than guesses, so a later contract
  change would only turn refusals into installs.
- Whether the memory-file bridge should become per-harness data rather than a filename constant. It
  needs no change here, and the second harness that needs a bridge is what would tell us what the
  table should hold.
