## Context

This change turns the declaration fixed in `add-harnaas-foundation` into filesystem effects. Four
things make it harder than "download and copy".

- **The destinations are directories the user already owns.** `.agents/skills/` holds both assets a
  team declared and a skill somebody wrote yesterday, and nothing on disk distinguishes them. Getting
  ownership wrong means either clobbering work or never being able to update anything.
- **The same content has to be re-identified later.** `lint` must answer "did somebody edit this?"
  and "has upstream moved?" independently, and both are only answerable from what install recorded at
  the time it ran.
- **Harnesses fail silently.** A skill whose frontmatter `name` disagrees with its directory is
  dropped without a message on at least three harnesses; a memory file over 4 MiB is skipped; a write
  to a surface a harness removed is a no-op that looks exactly like success. A tool that installs into
  those conditions reports a lie.
- **This is the first network access, the first write outside the project's own files and the first
  archive extraction in the CLI**, so the security rules set here are the ones every later feature
  inherits.

A survey of 23 harnesses reshaped the change: 17 read `.agents/skills/<id>/SKILL.md` and 21 read
`AGENTS.md`, so the obvious architecture — one adapter per supported harness — is not what this
builds. Most harnesses are served with no adapter at all, and an adapter is needed only for the
surfaces that have no shared equivalent. See
[0002-shared-agents-target-before-per-harness.md](../../../docs/adr/0002-shared-agents-target-before-per-harness.md).

The entire.io CLI's managed-plugin subsystem solves the analogous problem for binaries and supplies
most of the mechanics — tag resolution over Git, download over HTTPS, archive entry safety, atomic
staging. The decisions below are where harness assets differ from executables.

## Goals / Non-Goals

**Goals**

- Never destroy work the user did not delegate to `harnaas`.
- Never produce a silent no-op. Every condition a harness would ignore without saying so is turned
  into a visible refusal or a named outcome.
- Make an install reproducible: the same manifest and lockfile on another machine produce the same
  files.
- Record enough provenance that `lint` needs no bookkeeping of its own.
- Serve the many harnesses that read shared locations without writing an adapter for each, while
  keeping harness-specific knowledge behind one boundary so a second adapter is purely additive.

**Non-Goals**

- Signature or attestation verification. Digests here establish *integrity over time*, not
  *authenticity of origin*; a trust model needs a key-distribution story that does not exist yet.
- Transitive dependencies between assets. Every asset resolves independently.
- Interactive conflict resolution or merging. Conflicts are reported with a remedy; the user decides.
- Forges other than GitHub, and adapters other than `claude-code`. The two registries exist so that
  adding either does not reshape the flow.
- Implementing the format-converting renderers. They are named by the contract in v1 and deliberately
  left unbuilt.
- An `uninstall` command. Emptying the manifest's `assets` array and installing does it, and keeps
  convergence as the single code path that removes anything.

## Decisions

### Shared targets first; a named adapter is the exception

Recorded in
[0002-shared-agents-target-before-per-harness.md](../../../docs/adr/0002-shared-agents-target-before-per-harness.md).
Its consequence for this change is structural: support is two-tiered. `skill` and `instruction` reach
a harness through shared locations with no per-harness code, while `rule`, `command` and `persona` —
which have no shared equivalent anywhere — reach it only through a named adapter. v1 ships exactly one
named adapter and still serves most of the ecosystem, and "harness with no adapter" is a supported
state rather than a gap.

This is also why the `harnesses` list in the manifest means "the harnesses we guarantee", not "the
harnesses that will see this". A shared directory is visible to every harness installed on the
machine. Nothing in the design can change that, so the lockfile records which harness an adapter
resolved a destination *for*, as attribution rather than exclusive ownership.

### Ownership lives in the lockfile

Recorded in
[0001-ownership-lives-in-the-lockfile.md](../../../docs/adr/0001-ownership-lives-in-the-lockfile.md).
Two consequences are specified rather than left implicit, because both look like bugs if you meet
them without the rule: deleting the lockfile does not orphan installed files, it makes them
unmanaged, which is protective; and a project with pre-existing harness files gets conflict reports on
its first install rather than a silent takeover.

The shared-target decision sharpens this. Ownership in `.agents/` is a seventeen-way problem, and
other tools write there too. The lockfile rule is what makes that safe without negotiation: another
tool's file at a destination `harnaas` wants is simply an unmanaged conflict — reported, never
overwritten.

### Force applies to drift, never to unmanaged files

`--force` overwrites a managed destination whose content no longer matches what was recorded. It
deliberately does not override unmanaged-path protection. The two situations look similar and are
not. A drifted managed file is something `harnaas` put there and the user edited, so restoring it is
recoverable and comprehensible. An unmanaged file was never `harnaas`'s, and no flag on an install
command should be able to destroy it. A user who wants `harnaas` to own that path deletes it
themselves, which is an explicit act with an obvious meaning.

### Rendering is the general case; verbatim copying is only the default

This is the decision that most changed shape during design, and it sits in apparent tension with ADR
0001, which rejected in-file markers partly because injecting one would mean the installed bytes never
equal the source bytes.

The resolution is that byte-equality with upstream was never the invariant worth defending. The
question that matters is *"does this destination still hold what `harnaas` wrote?"*, and that is
answered by comparing the destination against a recorded **installed digest**. A separate **source
digest** answers *"has upstream moved?"*. Once those two comparisons are independent, a transformation
between source and destination stops being a threat to drift detection, and copying verbatim becomes
merely the renderer that happens to apply almost everywhere.

Markers remain wrong for the reasons the ADR gives that this does not touch: a comment is not valid
syntax in every format, ownership expressed in file content evaporates when someone edits the file,
and a marker cannot express "this directory of twelve files is mine". Ownership stays in the lockfile;
the bytes are free.

The payoff is that `as-skill` — delivering a `command` to a harness with no command surface, as a
`SKILL.md` with model invocation disabled — is expressible at all, and that adding a format converter
later needs no redesign. The render hook is therefore part of the adapter contract in v1 even though
only two renderers exist, because widening an interface after adapters exist is the expensive move.

### Refuse rather than produce a silent no-op

One principle explains five requirements that otherwise look unrelated and unusually strict:

- a `removed` support tier is refused rather than written, because the harness ignores that path;
- a skill whose frontmatter `name` disagrees with its inferred id fails resolution, because several
  harnesses drop it without a message;
- the 1 MB `SKILL.md`, 4 MiB memory file and depth-5 import limits fail the asset, because a file past
  them is skipped by the harness;
- a declared-but-unimplemented renderer reports `unsupported` and never falls back to identity,
  because identity would write a file the harness cannot read;
- a `rule` declaring `paths:` is `unsupported` on a harness with no rules directory rather than
  emulated as an instruction, because emulation would silently widen it from scoped to always-on.

In every case the alternative is a green install and a harness that quietly ignores the file — the
single worst outcome for a tool whose whole purpose is telling a team what is actually in effect. The
40,000-character assembled-content threshold is deliberately *not* in this list: it is a degradation
point, not something the harness enforces, so it warns and never blocks.

The name-mismatch check is read-only on purpose. Rewriting the frontmatter to match would fix the
symptom, but `harnaas` does not rewrite frontmatter (it would change bytes the author controls, and
`paths:` is the case where doing so would change meaning), so the check reports and stops.

### Instruction content goes in an `AGENTS.md` block; rules are separate files

Recorded in
[0003-instruction-content-in-an-agents-md-block.md](../../../docs/adr/0003-instruction-content-in-an-agents-md-block.md).
Two details of this change follow from it.

Provenance in the block is carried in HTML comments. Claude Code strips HTML comments before the model
sees them, so naming each asset and its source costs zero context — which is what makes per-asset
attribution affordable in a file that is always in the prompt.

`rule` and `instruction` differ on exactly one axis: whether the content survives a fresh clone with
nobody having run install. That axis is why instructions are inlined into a committed file while rules
are standalone files listed in the ignore block, and why a rule is never referenced from the block or
the bridge line. A referenced rule would be an instruction with extra steps.

### Installed files are ignored by version control, path by path

Installed content is fully reproducible from the manifest plus the lockfile, so committing it
duplicates state and turns every upstream bump into a large, meaningless diff. The block in the ignore
file lists individual installed paths rather than the directories containing them, which is more
verbose and more code, and is required by the shared-target decision: installed and hand-written
skills live side by side under the same parent, so `.agents/skills/` as a single entry would untrack
somebody's hand-written skill. Precision here is what keeps ADR 0002 from having a nasty second-order
cost.

### Fetch archives over HTTPS, resolve refs over the Git protocol

Three ways to get files out of GitHub were available.

Per-file requests through the REST contents API are the obvious approach and the worst: a skill is a
directory, so each install needs a listing plus one request per file against a rate limit that is
punishing unauthenticated — a CI job installing a handful of skills would exhaust it.

Cloning handles private repositories and other forges naturally, but pulls whole history for a few
kilobytes of markdown and makes the cache a set of repositories to maintain.

Fetching the repository archive for a resolved commit is one request per repository and commit, has no
per-file budget, is trivially content-addressable by commit, and reuses archive handling the source
CLI already has. Refs are resolved separately over the Git protocol because that path has no API rate
limit and goes through the user's existing credential helpers, so a private repository works without
`harnaas` inventing an auth story. Resolve names with Git, move bytes over HTTPS.

Resolution is split from retrieval for a second reason: ADR
[0004-available-updates-are-lint-errors.md](../../../docs/adr/0004-available-updates-are-lint-errors.md)
makes an available update a hard failure, which is only coherent if the lockfile retains both what was
*asked for* and what it *resolved to*. Recording the requested ref alongside the resolved commit is a
requirement of this change that exists entirely to serve that one.

### Containment is enforced by the kernel, not by string comparison

Both `.harnaas` reads and destination writes go through a handle anchored at the relevant root.
Validating a path and then opening it is a time-of-check-to-time-of-use race — a component can become
a symbolic link in between — and prefix comparison is notoriously easy to get wrong across platforms.
An anchored handle makes escape impossible at the point of use rather than merely improbable at the
point of validation, which lets the asset id, the source path and every archive entry name be treated
as one class of untrusted input.

### Digests cover paths as well as content

The whole-source digest is computed over the sorted set of relative paths and their content digests,
so renaming a file changes the digest even when no bytes change. Sorting makes the value independent
of archive order and filesystem iteration order. Per-file digests recorded alongside it are what let
`lint` say *which* file changed rather than only that something did. File modes are normalized rather
than hashed, because harness assets are documents and an executable bit carries no meaning worth
preserving — and preserving it would make the digest differ across platforms that report modes
differently.

### Convergence prunes only what it still recognizes

Removing an asset from the manifest removes its installed files, so the manifest stays the single
description of what a project has, and emptying `assets` is a complete uninstall. Pruning is limited
to destinations recorded in the lockfile whose content still matches: if somebody edited an orphan,
deleting it would destroy work, so it is kept and reported. A shared destination another recorded
harness still claims is likewise kept — attribution is per harness, the file is one file.

### The lockfile is committed and decoded leniently

An uncommitted lockfile would make ownership a per-machine fact, so a teammate's first install would
report every already-installed file as an unmanaged conflict. Committing it also makes `lint` a real
CI gate. That forces two further requirements: destinations are stored relative to their scope root
with the scope named, so a user-scoped entry does not embed one developer's home directory in a shared
file; and decoding ignores unknown fields, because a committed machine-written file is rewritten by
every version of the CLI on the team and a newer binary must not brick an older one. Strictness runs
the other way for the manifest, which is hand-edited, where an unknown field is a typo worth
reporting.

### Adapters are decoupled by a test, not by convention

An adapter package may import the contract and general utilities, and not the install flow, the
lockfile or the command layer. A hand-written AST check parses imports against an allowlist, so
widening the boundary is an explicit reviewable edit rather than something that happens during a
refactor. The same check asserts every adapter package self-registers, which is the failure mode
self-registration is prone to: an adapter that compiles, passes its own tests, and is missing from the
registry because nothing imports it.

## Risks / Trade-offs

- **A shared directory is written for harnesses nobody declared.** Accepted in ADR 0002 and mitigated
  here by making the lockfile's harness field attribution rather than ownership, so a destination
  survives one harness dropping out while another still claims it.
- **Archive fetching downloads a whole repository for a few files.** Fine for asset repositories,
  which are documentation-sized, and caching by commit means the cost is paid once. A per-file path
  can be added behind the source-kind registry later without touching the install flow.
- **Ownership is lost if the lockfile is deleted.** Chosen over embedded markers with eyes open. The
  failure is protective — files become unmanaged, not orphaned — and committing the lockfile makes it
  rare.
- **Pruning deletes files.** The narrowest safe form was chosen: only lockfile-recorded destinations
  whose content still matches. A user who edited an orphan keeps their edit.
- **Rewriting `AGENTS.md`, `CLAUDE.md` and the ignore file touches files users care about.** Mitigated
  by writing only inside markers, preserving everything else byte-for-byte, specifying the empty case
  so blocks disappear rather than linger, and deleting `CLAUDE.md` only when it holds nothing but the
  bridge line. Merge conflicts will land inside the block; that is accepted.
- **Refusing rather than warning will annoy people.** A skill whose frontmatter `name` is wrong fails
  the install even though the file is perfectly readable, and the remedy is upstream. This is the
  deliberate price of never reporting an install that a harness ignores.
- **The rendering layer is more machinery than v1 needs.** Two renderers and one adapter do not
  require a hook on the contract. It is there because the interface is cheap now and expensive once
  adapters exist, and because the two-digest model that makes it safe is needed anyway.
- **No authenticity verification.** A compromised upstream serves compromised content and `harnaas`
  records its digest faithfully. The lockfile bounds the blast radius: the change is visible as a moved
  commit and a changed digest, which is what `lint` reports.
- **Manifest and lockfile describe overlapping things and can skew.** Specifying the lockfile here,
  with its producer, keeps format and writer together, and skew between the two is itself a finding
  `lint` reports.

## Open Questions

- Whether a future `harnaas adopt` should let a user hand an existing unmanaged file to `harnaas` by
  recording it in the lockfile. It would remove the main friction of the unmanaged-conflict default,
  but it is additive and not needed to ship.
- Whether user-scoped installations belong in the committed lockfile at all, given the destination
  exists on one machine. Recorded for now, because `lint` cannot check what it cannot see; revisit if
  teams find the shared record noisy.
- Which harnesses are known *not* to read the shared skills directory, and therefore need a
  per-harness copy. v1 ships one named adapter, so the fallback table is nearly empty and the survey
  data behind it will age; it needs a maintenance story before the second adapter lands.
