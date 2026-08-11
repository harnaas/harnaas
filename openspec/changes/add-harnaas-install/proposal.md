## Why

A team's harness assets are copy-pasted files. Nobody knows which version is installed, whether
somebody edited it, or whether upstream has moved. `add-harnaas-foundation` gives a project a way to
declare what it wants, but `harnaas.json` is a file nothing consumes. This change makes the
declaration executable: it resolves every declared source to an immutable commit, renders the content
into the places each harness actually reads, and records exactly what landed and where it came from.
That record is what makes an install reproducible across a team, what stops `harnaas` from ever
touching a file it did not write, and what `harnaas lint` needs in order to detect drift or an
available update at all.

## What Changes

- New `harnaas install` command running four phases — resolve, plan, apply, record — with
  `--dry-run`, `--force`, `--offline`, a cache bypass and `--json`. No harness destination is touched
  before the plan is complete.
- A source-resolution layer behind a kind registry, shipping `github` (resolve a ref to an immutable
  commit over the Git protocol, fetch the repository archive for that commit over HTTPS, extract only
  the declared subtree) and `local` (read from `.harnaas/` through a handle anchored there).
- Transport and extraction safety as a hard boundary: HTTPS only with plaintext and loopback refused
  including after redirect, bounded redirect chains, size ceilings that fail rather than truncate,
  zip-slip and link-entry rejection, and credentials redacted from every error, log and written file.
- A content-addressed archive cache under the user cache directory (`HARNAAS_CACHE_DIR` overrides),
  verified before reuse and re-fetched rather than fatal when corrupt, plus an offline mode that
  resolves purely from it and names every asset it cannot satisfy.
- A GitHub token chain — `HARNAAS_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`, then unauthenticated —
  with an authorization failure that names the variable it read from.
- Shape verification before anything is planned: a `skill` must be a directory containing `SKILL.md`,
  every other type a single file, and a skill whose `SKILL.md` frontmatter `name` disagrees with its
  inferred id is refused rather than installed into silence.
- A harness-target layer that writes the **shared** targets first — `.agents/skills/<id>/SKILL.md`
  and the managed block in `AGENTS.md` — and falls back to a harness's own directory only where that
  harness does not read the shared one.
- A named-adapter contract and registry for the surfaces that have no shared equivalent (`rule`,
  `command`, `persona`), with `claude-code` as the only named adapter in v1, self-registration at
  init, deterministic listing, and an AST import-boundary test with an explicit allowlist.
- Support tiers on every adapter surface that gate writes: `live` writes silently, `removed` is
  refused with the replacement named, `gated` and `legacy` write with a note carried into the report.
- A managed, marker-delimited instruction block in `AGENTS.md` holding every `instruction` asset
  inlined verbatim and sorted by id, plus exactly one `@AGENTS.md` bridge line in `CLAUDE.md`. `rule`
  assets are standalone files and are never referenced from either.
- A rendering layer in which copying bytes verbatim is the default renderer rather than an invariant:
  v1 implements `identity` and `as-skill` (a `command` becomes a `SKILL.md` with model invocation
  disabled), declares to-TOML, to-YAML, to-TypeScript and shared-document renderers without
  implementing them, and reports selecting one as `unsupported` rather than falling back to identity.
- Emulation rules that never silently change semantics: an unscoped `rule` may be delivered through
  the instruction surface, a `rule` declaring `paths:` may not and is reported `unsupported`.
  Frontmatter is otherwise passed through untouched.
- Install-time limit checks measured on rendered output: 1 MB per `SKILL.md`, 4 MiB per memory file
  and an import depth of 5 fail the asset; roughly 40,000 characters of assembled always-on content
  warns and names the largest contributors.
- A new committed `harnaas.lock.json` recording per asset the normalized source, requested ref,
  resolved commit and source digest, and per installation the harness, scope, scope-relative
  destination, per-file digests and an installed digest — two independent digests, so "upstream
  moved" and "somebody edited this" are never confused.
- Ownership determined solely by the lockfile: a recorded destination is managed and may be updated
  or pruned; an unrecorded one is hand-written and is never overwritten or deleted, on any flag —
  `--force` applies only to drifted managed destinations.
- Convergence on the manifest: an asset no longer declared has its managed destinations removed, but
  only where the content still matches what was recorded; a drifted orphan is kept and reported.
  Emptying the `assets` array and running install is the documented full uninstall — there is no
  `uninstall` command.
- A managed block in the project's version-control ignore file listing exactly the installed paths,
  one entry per path, with coarse directory ignores explicitly rejected.
- Seven outcomes per asset and target — `created`, `updated`, `unchanged`, `emulated`,
  `conflict-unmanaged`, `conflict-drift`, `unsupported` — each blocked outcome carrying a runnable
  remedy, reported deterministically in both text and JSON.
- Atomicity, an exclusive lock around the lockfile read-modify-write, deterministic ordering by asset
  id, and partial-failure semantics that attempt every asset, report every failure and record only
  what landed.
- **BREAKING**: nothing. No prior released behaviour exists.

## Capabilities

### New Capabilities

- `source-resolution`: turning a declared source into a verified set of files on disk — the source
  kind registry, GitHub ref resolution and archive retrieval, local reads under `.harnaas`, transport
  and extraction safety, shape and skill-name verification, digests, caching, authentication and
  offline resolution.
- `harness-targets`: where an asset lands — shared-target precedence, the named adapter contract and
  registry, harness detection, scope resolution, support tiers, destination containment, the
  `claude-code` mapping, the managed instruction block in `AGENTS.md`, the `CLAUDE.md` bridge line,
  and the enforced adapter import boundary.
- `asset-rendering`: how the installed bytes are produced — rendering as the general case, the
  per-type render hook, the identity and as-skill renderers, declared-but-unimplemented renderers,
  rule-to-instruction emulation, frontmatter pass-through, the two independent digests, and the hard
  and soft harness limits.
- `install-command`: the resolve-plan-apply-record flow — the seven per-target outcomes, unmanaged
  and drift protection, convergence and full uninstall, the ignore-file managed block, offline
  installation, atomicity, concurrency safety, determinism, partial failure and machine-readable
  output.
- `install-lockfile`: `harnaas.lock.json` — its location and role, recorded provenance and
  installations, two digests per installation, harness attribution for shared destinations, portable
  paths, credential redaction, lenient decoding, missing-lockfile handling, and durable deterministic
  writing.

### Modified Capabilities

None. Every capability in this change is new: `openspec/specs/` is empty and nothing has been
implemented yet. This change does not alter any requirement introduced by `add-harnaas-foundation`.

## Impact

- Depends on `add-harnaas-foundation` for the manifest format (including type and id inference from
  the asset path), project-root resolution carried in `context.Context`, the error and exit-code
  contract, output-stream discipline and the atomic-write helper. None of those are restated here.
- Adds runtime dependencies: a Git implementation for `ls-remote`-style ref resolution over the Git
  protocol, a file-locking library for the install lock, and a YAML frontmatter parser. Archive
  handling, HTTP, hashing and atomic rename come from the standard library.
- Writes into the project working tree for the first time: `.agents/skills/`, `.claude/`, `AGENTS.md`,
  `CLAUDE.md` and the version-control ignore file, plus the harness's per-user home for user-scoped
  assets. `harnaas.json` is still never written.
- Adds a content-addressed cache under the user cache directory, and a new environment surface:
  `HARNAAS_CACHE_DIR`, `HARNAAS_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`.
- Introduces the first network access in the CLI. The transport rules, cache and token chain set here
  are what `add-harnaas-lint`'s update detection reuses rather than reimplements.
- Produces the lockfile `add-harnaas-lint` reads. The two changes must agree on its contents, which
  is why the format is specified here, with the producer, rather than there.
- Adds a hand-written AST test over adapter package imports to CI, which must be widened explicitly
  whenever an adapter legitimately needs a new dependency.
- Tests require a local HTTP test server, crafted malicious archives, and temporary projects; no
  network access is required to run the suite.
