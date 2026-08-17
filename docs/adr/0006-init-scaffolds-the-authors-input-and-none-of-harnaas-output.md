# `harnaas init` scaffolds the author's input directory and none of harnaas's output

`harnaas init` originally wrote exactly one file, `harnaas.json`, and the rule was stated as that
count. What the count was protecting is a boundary: init must not create the destinations
`harnaas install` records ownership of, because anything init created there would be *unmanaged*, and
the next install would report a conflict against init's own output. `.harnaas` is on the other side of
that boundary — it is content the author writes and harnaas only ever reads — so init now creates it,
together with the asset-type directories the selected harnesses can actually receive, and still
creates no harness directory, no `AGENTS.md`, no `CLAUDE.md` and no ignore-file entry.

The boundary is ownership, not a file count. [0001](./0001-ownership-lives-in-the-lockfile.md) records
ownership in `harnaas.lock.json`, the lockfile records *destinations*, and `.harnaas` is never a
destination: the `local` source kind reads it, `lint` re-reads it to notice an edited local source, and
nothing writes to it. Nothing scaffolded there can collide with a managed set, because there is no
managed set it could be part of.

Stated as a count, the rule also cost something. The layout inside `.harnaas` is not cosmetic — the
first path segment is what harnaas infers an asset's type from — so an author hand-building it can
spell `rules` as `rule` and learn about it at the far end of an install, from a message about a type
that could not be inferred. init is the one moment harnaas knows which harnesses a project targets,
and the moment that layout can be produced correctly for nothing. Worse, the rule left init printing
guidance that attributed the work to `harnaas install`, which does not do it, and never did.

## Considered Options

**Keep the single-file rule and leave the layout to the author.** Rejected because it is the status quo
with the untrue sentence removed: the author still has to know five directory names harnaas will
silently infer types from, and still finds out about a wrong one later, from a diagnostic about
something else. Deleting the sentence would have made init honest and left the problem it was
covering for.

**Have `harnaas install` create the directories instead.** Rejected because install would be creating
its own inputs. It reads `.harnaas` to resolve a `local` source, and a missing path there is a
resolution failure naming the asset that declared it — which is the right diagnostic. An install that
created the directory first would turn "you declared content that is not there" into "harnaas made you
an empty directory and then failed", and it would do it on every run rather than once.

**Scaffold every asset type regardless of the selection.** Rejected because the directory set is the
one thing here that carries information. A `commands/` directory in a project targeting only a harness
that refuses commands ([0005](./0005-a-command-emulates-as-a-skill-only-where-it-can-be-silenced.md))
invites an author to write a command that cannot be delivered anywhere, and harnaas would have offered
them the place to put it. The set is derived from the same routing the install flow uses, so a
directory is never offered for a pairing an install would refuse.

**Create the directories empty.** Rejected because git does not track an empty directory: the
scaffolding would be invisible in `git status`, absent from the commit and gone on the next clone, so
it would exist only for the person who ran init — the one person who did not need it. Each created
directory therefore carries a `README.md` naming the type, what belongs there, and the manifest line
that declares it. A `.gitkeep` would survive the clone and answer none of that, at the moment the
question is being asked.

## Consequences

init writes more than one file, so "what did init do" is no longer answerable by naming a path. It
reports the directories it created, and never reports as created one that was already there.

The scaffolding is the author's from the moment it is written. Nothing under `.harnaas` is recorded in
the lockfile, covered by a managed ignore-file entry, or rewritten by a later command — including the
READMEs, which no harnaas command reads. `--force` replaces the manifest and grants nothing here: a
second run adds what is missing and changes nothing that exists, and a run whose selection is narrower
than an earlier one removes nothing, because those directories may hold content harnaas never wrote.

A project that installs only from GitHub gets four or five directories and their READMEs that it will
not use. They are deletable and are not recreated unless init runs again. A flag to suppress them is
deliberately not added yet: adding one later is additive, and removing one is not.

Nothing verifies the scaffolding afterwards. That is the cost of it not being managed, and it is the
same trade as the rest of `.harnaas`: `lint` reports on the assets a manifest declares, not on the
directories they might have been written into.
