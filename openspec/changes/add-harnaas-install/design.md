## Context

This change turns the declaration fixed in `add-harnaas-foundation` into filesystem effects. Three
things make it harder than "download and copy":

- The destinations are directories users already own and edit by hand. `.claude/skills/` holds both
  assets a team declared and skills a developer wrote yesterday, and nothing on disk distinguishes
  them. Getting ownership wrong means either clobbering someone's work or refusing to ever update.
- The same content must be re-identified later. `lint` has to answer "did this change locally?" and
  "has upstream moved?", and both questions are only answerable if install records enough at the time
  it runs.
- Everything here is the first network access, the first write outside the project's own files, and
  the first archive extraction in the CLI, so the security rules set here are the ones every later
  feature inherits.

The entire.io CLI's managed-plugin subsystem (`plugin_fetch.go`, `plugin_manifest.go`,
`plugin_install_remote.go`) solves the analogous problem for binaries and supplies most of the
answers; the divergences below are where harness assets differ from executables.

## Goals / Non-Goals

**Goals:**

- Never destroy work the user did not delegate to `harnaas`.
- Make an install reproducible: the same manifest and lockfile on another machine produce the same
  files.
- Record enough provenance that `lint` needs no additional bookkeeping.
- Keep the harness-specific knowledge behind one adapter boundary so a second harness is additive.

**Non-Goals:**

- Signature or attestation verification of upstream sources. Digests here establish *integrity over
  time*, not *authenticity of origin*; a trust model would need a key distribution story that does not
  exist yet.
- Transitive dependencies between assets. Every asset is resolved independently.
- Interactive conflict resolution or merging. Conflicts are reported with a remedy; the user decides.
- Forges other than GitHub. The source-kind registry exists so adding one does not reshape the flow.

## Decisions

### Fetch repository archives, resolve refs over the Git protocol

Three ways to get files out of GitHub were available.

Per-file requests through the REST contents API are the obvious approach and the worst one: a skill
is a directory, so every install needs a listing plus one request per file, all against a rate limit
that is punishing when unauthenticated — and a CI job installing a handful of skills would exhaust it.

Cloning with a Git implementation handles private repositories and non-GitHub forges naturally, but
pulls whole history for a few kilobytes of markdown and makes the cache a repository to maintain.

Fetching the repository archive for a resolved commit is one HTTPS request per repository and commit,
carries no per-file API budget, is trivially content-addressable by commit, and reuses the tar
handling and entry-safety checks the source CLI already has. That is the choice.

Refs are resolved separately, over the Git protocol against the remote, because that path has no API
rate limit and goes through the user's existing Git credential helpers — so a private repository works
without `harnaas` inventing its own auth story. The split is deliberate: resolve names with Git, move
bytes over HTTPS, exactly as the source CLI resolves plugin tags over Git and downloads assets over
HTTPS.

### The lockfile is the ownership record — not a marker in the file

The source CLI marks the files it scaffolds with an embedded comment, so re-running setup updates its
own files and never clobbers a hand-written file at the same path. That works because it *generates*
the content and is free to add a line to it.

`harnaas` copies content verbatim, and that difference is decisive. Injecting a marker would mean the
installed bytes never equal the upstream bytes, so the digest that answers "has this been modified
locally?" could no longer be compared against the source — the very check `lint` exists to perform
would be defeated by the mechanism meant to enable it. It would also silently corrupt formats where an
arbitrary comment is not valid.

So ownership lives entirely in the lockfile: a destination it records is managed, a destination it
does not record is the user's. Two consequences follow and are specified rather than left implicit.
Deleting the lockfile does not orphan files — it makes them unmanaged, which is protective. And a
project with pre-existing harness files gets conflict reports on first install rather than a silent
takeover, which is the correct default when `harnaas` cannot know who wrote them.

The one place a marker *is* right is the rules block in `CLAUDE.md`, because `harnaas` authors that
block itself. Claude Code reads memory files, not a rules directory, so a rule asset only takes effect
once something references it. That block is generated content, it is delimited, everything outside it
is preserved, and it is removed when the last rule goes away. That this differs from how another
harness would deliver a rule — Cursor has a rules directory and needs no block at all — is precisely
why the adapter boundary exists.

### Force applies to drift, never to unmanaged files

`--force` overwrites a managed destination whose content no longer matches what was recorded. It
deliberately does **not** override the unmanaged-path protection. The two situations look similar and
are not: a drifted managed file is something `harnaas` put there and the user edited, so restoring it
is a recoverable, understandable action. An unmanaged file was never `harnaas`'s to begin with, and no
flag on an install command should be able to destroy it. A user who wants `harnaas` to own that path
can delete it themselves, which is an explicit act.

### Install converges on the manifest, but only over content it still recognizes

Removing an asset from the manifest removes its installed files, so the manifest stays the single
description of what a project has. Pruning is restricted to destinations recorded in the lockfile
whose content still matches the recorded digest: if the user edited an orphaned file, deleting it
would destroy work, so it is kept and reported instead. This keeps convergence from ever being
destructive in the one case where it would matter.

### Directory destinations are replaced as a unit

A skill is a directory, and updating it file-by-file leaves windows where the harness could read a
half-old, half-new skill. Content is staged outside the destination and moved into place, so a
destination is only ever the old tree or the new tree. This also gives interruption safety for free.

### Containment is enforced by the kernel, not by string comparison

Both `.harnaas` reads and harness writes go through a handle anchored at the relevant root. Validating
a path and then opening it is a time-of-check-to-time-of-use race — a component can become a symbolic
link in between — and path-prefix comparison is notoriously easy to get wrong across platforms. An
anchored handle makes escape impossible at the point of use rather than merely improbable at the point
of validation. This is what allows the asset id, the source path, and the archive entry names to be
treated as a single class of untrusted input.

### Digests cover paths as well as content

The whole-source digest is computed over the sorted set of relative paths and their content digests,
so a renamed file changes the digest even when its bytes do not. Sorting makes the value independent
of archive order and filesystem iteration order; recording per-file digests alongside it is what lets
`lint` say *which* file changed rather than only that something did. File modes are normalized rather
than hashed, since harness assets are documents and an executable bit on one carries no meaning worth
preserving.

### The lockfile is committed

An uncommitted lockfile would make ownership a per-machine fact, so a teammate's first install would
report every already-installed file as an unmanaged conflict. Committing it also makes `lint` a
meaningful CI gate: the pipeline can assert that what is installed matches what was declared and
recorded. That in turn forces the portability requirement — destinations are stored relative to their
scope root, so a user-scoped entry does not embed one developer's home directory in a shared file.

### Adapters are decoupled by a test, not by convention

The adapter package may depend on the contract and on utility packages, and not on the install flow,
the lockfile, or the command layer. An automated check parses imports and fails against an allowlist,
so widening the boundary is an explicit, reviewable edit rather than something that happens by
accident during a refactor. The same check confirms every adapter registers itself, which is the one
failure mode self-registration is prone to: an adapter that compiles, passes its own tests, and is
absent from the registry because nothing imports it.

## Risks / Trade-offs

- **Archive fetching downloads a whole repository for a few files.** Acceptable for asset repositories,
  which are documentation-sized; caching by commit means the cost is paid once. If a very large
  repository becomes a real use case, a per-file path can be added behind the source-kind registry
  without changing the install flow.
- **Ownership is lost if the lockfile is deleted.** Chosen over embedded markers with eyes open. The
  failure is protective — files become unmanaged, not orphaned — and committing the lockfile makes the
  situation rare.
- **Pruning deletes files.** The narrowest safe form was chosen: only lockfile-recorded destinations
  whose content still matches. A user who edited an orphan keeps their edit.
- **Rewriting `CLAUDE.md` touches a file users care deeply about.** Mitigated by writing only inside
  the markers, preserving everything else byte-for-byte, and specifying the empty case so the block
  disappears entirely rather than lingering.
- **No authenticity verification.** A compromised upstream repository serves compromised content, and
  `harnaas` will faithfully record its digest. The lockfile still bounds the blast radius: the change
  is visible as a moved commit and a changed digest, which is what `lint` reports.
- **Two files must stay in agreement.** Manifest and lockfile are separate documents describing
  overlapping things. Specifying the lockfile here rather than in the lint change keeps the producer
  and the format together, and skew between them is itself a finding `lint` reports.

## Open Questions

- Whether a future `harnaas adopt` should let a user hand an existing unmanaged file to `harnaas` by
  recording it in the lockfile. It would remove the main friction of the unmanaged-conflict default,
  but it is additive and not needed to ship.
- Whether user-scoped installs should be recorded in the committed lockfile at all, given the
  destination exists on only one machine. Recorded for now, because `lint` cannot check what it cannot
  see; revisit if teams find the shared record noisy.
