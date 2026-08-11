## Context

By this point `install` has produced two records of the same intent: `harnaas.json` says what the
project wants, `harnaas.lock.json` says what it got. The files on disk are a third view, and all three
drift apart in ordinary use. Nothing else in harnaas notices.

Two properties of the rest of the design decide almost everything about `lint`:

**The manifest is hand-edited only.** harnaas never writes `harnaas.json`. So for a whole class of
findings — an available update, a branch that should be pinned — there is no command that can fix
them. The best a tool can do is print the exact substitution and the command to run afterwards.

**Ownership lives in the lockfile, not in a marker inside installed files**
(`docs/adr/0001-ownership-lives-in-the-lockfile.md`). Installed bytes are the output of a renderer,
so they do not have to equal the source bytes; what makes a digest comparison meaningful is that
harnaas keeps two independent digests per installation — a *source* digest and an *installed* digest
— and the difference between those two comparisons is the difference between "upstream moved" and
"you edited this". That distinction is the spine of the entire finding taxonomy.

`lint` is also scoped by vocabulary. Per `CONTEXT.md`, lint examines the *installation* — never the
content of an asset. It does not judge whether a skill is well written.

The precedent is the source CLI's `plugin doctor`, which re-hashes each managed binary against the
digest recorded at install and reports that it "no longer matches the digest recorded at install; it
was modified or replaced outside entire", pairing every problem with a runnable fix.

## Goals / Non-Goals

**Goals:**

- Detect every way the installed state can diverge from what was declared and recorded.
- Make each finding actionable without further thought: a command, or a literal before/after edit.
- Be safe and useful as a required CI check, on a fresh checkout where nothing is installed.
- Never let a network condition change the verdict on a local problem.
- Keep the report's signal-to-noise high enough that people read it.

**Non-Goals:**

- Repairing anything. `harnaas install --force` is the single repair path.
- Judging asset content. Integrity and freshness, not prose quality or frontmatter semantics.
- Verifying provenance. As in `add-harnaas-install`, digests establish integrity over time, not
  authenticity of the upstream.
- Watching continuously. `lint` is a point-in-time check.

## Decisions

### Lint never repairs, and that is a boundary rather than a missing feature

A `--fix` flag is deliberately absent, for three separate reasons.

Every repair lint could perform is something `install` already does. Doing it in two places means two
implementations of overwrite protection, convergence and atomicity that have to agree forever — and
the day they disagree is the day a tool marketed as a safety check deletes someone's work.

A read-only command is safe to run anywhere — pre-commit hook, CI, a colleague's machine — without
anyone reasoning about what it might change. That property is binary: it disappears the moment one
flag can mutate state, because now every invocation has to be inspected for that flag.

And the findings most worth surfacing are exactly the ones where the right action is a judgement call.
A locally modified file may be someone's uncommitted improvement; an extraneous file may be
deliberate. Resolving those automatically destroys information; reporting them preserves it.

Cache writes are exempted explicitly rather than left ambiguous. A cache under the user cache
directory is not project state, and the alternative — refusing to cache — would make lint too slow to
run habitually, which is a worse outcome than a nuance in the read-only rule.

### An available update is an error, and so is tracking a mutable ref

Recorded in `docs/adr/0004-available-updates-are-lint-errors.md`. Nearly every comparable tool treats
an available update as advisory; harnaas does not, because a warning CI tolerates indefinitely does
not make a team's configuration uniform and current, which is the entire product thesis.

The second half is what makes the first half workable. If a branch-tracking asset were reported as
"outdated" whenever its branch moved, there would be no stable passing state at all — CI would be
permanently red through no fault of the project, and within a week someone would turn lint off.
Reporting it instead as *not reproducible — pin it* converts an infinite condition into an achievable
one. This is why the not-reproducible finding is emitted whether or not the ref has actually moved:
its subject is the manifest, not the remote.

The consequence is that severity is not a spectrum here. Every state that is not both pinned and
current is an error: available update, moved ref, vanished ref, changed local source, deleted local
source, mutable ref. Warning severity survives only for advisory findings that leave the installation
reproducible and current — which keeps `--strict` meaningful without it being the thing that carries
update enforcement.

### Exit code 2 means findings, and only findings

CI needs three outcomes, not two: clean, the check found problems, the check itself broke. Collapsing
the last two makes a red build ambiguous exactly when someone is debugging it — an unreadable lockfile
and a drifted skill would look identical. So `0` is clean, `2` is findings, `1` is a genuine runtime
failure. This is the one place harnaas extends the exit-code contract inherited from entire.io, and
reserving `2` for a single meaning is what stops that extension spreading.

### Frozen mode exists because a full lint is the wrong PR gate

On a fresh checkout nothing is installed, so a full lint reports "nothing installed yet" and tells the
reviewer nothing. And because updates are errors, a full lint can go red on a pull request that
changed nothing, purely because somebody upstream published a tag — the time-variance ADR 0004 accepts.

`--frozen` answers the only question a pull request can answer from the manifest and the lockfile
alone: does the lockfile still satisfy the manifest? Declared-but-unrecorded, recorded-but-undeclared,
and a recorded source, ref or type that disagrees with the manifest. No file is read, no request is
made. It is the same role `npm ci` and `--frozen-lockfile` play elsewhere, and it makes the intended CI
shape two jobs: `--frozen` on every PR, a full lint on a schedule.

### Offline mode is a complete local check, not half a command

`--offline` runs everything that does not need a network — including local source change detection,
which is a genuine freshness check that happens to require no remote. That is what keeps offline from
being "lint with a feature removed".

Its counterpart is mandatory: a report that skipped checks must say so, even when it is clean. A green
result that quietly skipped half its work is more dangerous than a red one, precisely because it is
trusted. The same rule covers the involuntary case — an unreachable host marks its assets unchecked,
is reported once per host rather than once per asset, never fails the run, and is counted in the
summary. The exit status is decided only by the checks that actually ran.

### Remedies print the exact edit, because there is nothing better available

Since harnaas never writes the manifest, a finding that needs a manifest change cannot end in an
offer to apply it. Prose — "consider upgrading to a newer version" — puts the work of finding the line
and composing the replacement back on the reader. So the finding names the file and the line, prints
the current source string and the replacement string verbatim, and ends with `harnaas install`.
Applying it is a literal substitution. Findings that need no manifest edit (drift, missing file,
changed local source) print the command alone, with no before/after block to skim past.

### Local sources are updates, not drift

A file under `.harnaas/` that has been edited since install is not corruption of the installed copy —
the *source* moved ahead, and the remedy is to install, not to restore. Framing it as an update rather
than as drift makes the remedy correct and keeps the finding distinct from a hand-edited destination.
A local source that no longer exists is a third thing again: nothing to reinstall from, so it is
reported separately rather than as an update to something that is gone.

### Collapse rather than cascade

An unloadable manifest would otherwise make every declared asset look uninstalled and every lockfile
entry look orphaned — dozens of findings that are artefacts of one. So a manifest failure is a single
finding that suppresses its dependents. The same reasoning collapses "nothing installed at all" into
one finding instead of one per declared asset, and a wholly absent destination into one finding
instead of one per recorded file.

An absent lockfile is deliberately *not* itself a finding. Per the lockfile design, no lockfile means
nothing is managed, which is the protective state, not a broken one. Reporting it as a problem would
punish the correct fresh-clone case.

### Managed-block drift is region-level, and the bridge line is a real check

Instruction content lives in a marker-delimited block inside `AGENTS.md`
(`docs/adr/0003-instruction-content-in-an-agents-md-block.md`), which is content harnaas authors
rather than copies — the deliberate exception to ADR 0001's no-markers rule. Two consequences follow
for lint. Drift is compared over the region, and everything outside the markers is ignored entirely,
never reported; the team owns that file. And malformed markers — a start without an end, or a
duplicate pair — are reported as malformed rather than interpreted, because guessing where a block
ends is how a tool eventually eats someone's prose.

The `@AGENTS.md` bridge line in `CLAUDE.md` gets its own check because its failure mode is invisible:
Claude Code does not read `AGENTS.md`, so a missing bridge line means installed instruction content is
silently not being read by the harness it was installed for. Everything looks correct on disk. That is
precisely the class of failure lint exists to surface, so it is checked whenever instruction assets
are recorded — and not checked at all when none are, to avoid a finding about a file the project has
no reason to have.

Lint must regenerate the expected block content using install's renderer rather than its own. Two
renderers that diverge would produce phantom drift on a correct project, which is the worst possible
failure for a tool whose credibility rests on its findings being real.

### Unmanaged conflicts and extraneous files matter more because the target is shared

Skills install to the shared `.agents/` tree before any per-harness directory
(`docs/adr/0002-shared-agents-target-before-per-harness.md`), a directory other tools also write into
— `openspec` among them. Ownership there is an N-way problem, and the lockfile rule is what makes it
safe. Lint is the reporting half of that rule: a destination that exists but is claimed by no lockfile
entry is an unmanaged conflict, reported with the explicit statement that install will not overwrite
it and that `--force` does not change that. A file inside a managed destination that the record does
not list is reported rather than ignored, because silently tolerating unknown files inside a managed
destination would hollow out the integrity claim.

### Only the highest newer stable tag, never a pre-release

Semantic-version ordering is the only total order available over tags, so it is what "newer" means;
tags that are not versions are ignored rather than guessed at. Reporting only the highest newer tag
keeps it to one finding per asset — a list of six intermediate versions is noise, since the remedy is
the same edit either way. A pre-release is never offered to an installation pinned at a stable tag,
because for a team standardizing its harness configuration that is a reduction in stability presented
as an improvement.

### Deterministic ordering

Findings are ordered by asset identifier and path, independent of manifest and lockfile ordering, so
two runs over the same state produce identical output. That is what makes the JSON report diffable and
a CI log comparable between runs — the same reasoning that fixes install's execution order.

## Risks / Trade-offs

- **Lint is not deterministic across time.** A project passes today and fails tomorrow because someone
  upstream published a tag, with no local change. Inherent to enforcing currency, accepted in ADR 0004,
  and contained by `--frozen` as the per-PR gate and `--offline` for a time-invariant local result.
- **Everything-is-an-error risks alarm fatigue.** Mitigated by the passing state being genuinely
  reachable — pinned and current — and by the one condition that would otherwise be permanently red
  (branch tracking) being reported as a pin instruction instead of an update. If teams still turn it
  off, the evidence will show up as `--strict` being irrelevant and update findings being suppressed
  wholesale, not as a quiet erosion.
- **No `--fix` means an extra step for the common case.** Accepted. The remedy is always printed,
  `install` is idempotent, and the alternative is duplicating install's write path.
- **Lint depends on install's internals staying shared.** The digest computation and both block
  renderers must be common code. If they fork, lint reports drift that does not exist. Mitigated by
  keeping them in shared packages and exercising them from both sides in the same golden tests.
- **Extraneous-file detection may be noisy in the shared tree**, where other tools legitimately write.
  Reported anyway, because the alternative weakens the integrity claim; the narrow fix if real noise
  appears is an ignore rule, not a weaker check.
- **Frozen mode can pass while the disk is wrong.** By construction — it reads no files. Bounded by
  being an explicitly named mode whose report states what it did not check, and by the full lint
  existing for the rest.
- **Exit code `2` is a contract callers must learn.** Bounded by specifying it once and reserving it
  for exactly one meaning.
- **Cached resolutions can be stale within the freshness window.** Intentional. A network round trip
  per invocation would make lint too slow to run habitually, and forced refresh exists for when
  currency matters more than speed.

## Open Questions

- Whether extraneous-file detection needs an ignore list, and whether it should be per-project or
  built in. Deferred until real usage shows which incidental files actually appear inside managed
  destinations — speculating now would bake in the wrong list.
- Whether the resolution cache's freshness window should be user-configurable or a fixed constant. A
  fixed value is assumed; a flag is easy to add later and impossible to remove.
- Whether a future non-`claude-code` adapter needs its own bridge-line equivalent, which would make
  the bridge check adapter-owned rather than a fixed check. Not a v1 concern with one named adapter,
  but the check should not be written in a way that makes moving it painful.
