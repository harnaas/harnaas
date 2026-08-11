## Why

`harnaas install` records what it placed, but from that moment the record and reality diverge
silently: someone edits an installed skill mid-debug, a file gets deleted, a teammate adds an asset to
the manifest and never installs it, or an upstream repository publishes a tag nobody notices. harnaas
exists so that a team's harness configuration is uniform, reproducible and current — and none of those
failures announce themselves. `harnaas lint` is what turns that intent into something enforceable: a
read-only check that is safe to run anywhere, names the exact fix for everything it finds, and exits
non-zero so CI can require it.

## What Changes

- New `harnaas lint` command comparing three views of the same reality — the manifest, the lockfile
  and the files on disk — and reporting every discrepancy as a finding with a severity and a remedy.
- Consistency checks: a manifest that fails to load or validate, an asset declared with no lockfile
  entry, and a lockfile entry for an asset the manifest no longer declares or targets.
- Integrity checks against the recorded digests: a modified file (reported per file, with its path), a
  recorded file that is gone, a file under a managed destination the record does not account for, and
  a destination claimed by a hand-written file that install will never overwrite.
- Managed-block checks: drift in the block harnaas owns in `AGENTS.md` and in `.gitignore`, missing,
  duplicated or malformed markers, and the `@AGENTS.md` bridge line in `CLAUDE.md` that Claude Code
  needs in order to read instruction content at all.
- Update detection: a mutable ref whose commit has moved, a ref that has vanished upstream, a stable
  tag that supersedes the installed one (highest only, non-version tags ignored, never a pre-release
  over a stable install), a changed local source under `.harnaas/`, and a deleted one.
- **Severity rule: the only passing state is pinned and current.** An available update is an error, and
  so is tracking a mutable ref — reported as "not reproducible, pin it" rather than as an update, so
  the passing state stays reachable. Warnings are reserved for advisory findings.
- Remedies print the exact edit: the manifest line, the current source string, the replacement string,
  and the command to run afterwards. Lint never offers to apply it — the manifest is hand-edited only.
- Noise control: an invalid manifest is one finding that suppresses its dependents, "nothing installed
  at all" collapses to one finding rather than one per asset, and an absent destination collapses to
  one finding rather than one per recorded file. An absent lockfile is not itself a problem.
- Modes: `--frozen` verifies the lockfile still satisfies the manifest with no files read and no
  network — the fast PR gate on a fresh checkout; `--offline` runs every local check including local
  source detection and states what it skipped; `--strict` promotes warnings for exit purposes; `--json`
  emits one document on stdout with advisory text on stderr.
- Exit status: `0` clean, `2` findings, `1` lint itself failed — so a red pipeline is unambiguous.
- Strictly read-only: no project, harness or home file is ever created, modified or deleted. Cache
  writes under the user cache directory are the only writes, and update-detection results are cached
  there with a bounded freshness window so lint is cheap enough to run habitually.
- Graceful degradation: an unreachable host marks its assets unchecked, is reported once per host,
  never fails the run, and is counted in the summary so a clean result is not mistaken for a complete
  one.

## Capabilities

### New Capabilities

- `lint-command`: the check itself — the finding model, deterministic ordering, every local and
  managed-block check, the severity rule, remedies that print the exact edit, frozen and offline modes,
  output modes, exit status, and the read-only guarantee.
- `update-detection`: deciding whether an installed asset has fallen behind its source — moved mutable
  refs, vanished refs, superseded tags, commit pins that are never checked, local source change and
  deletion, resolution caching, and graceful degradation when the network or credentials are
  unavailable.

### Modified Capabilities

None. `openspec/specs/` is empty — no capability has been implemented yet, so everything this change
touches is new. It does not alter requirements proposed by `add-harnaas-foundation` or
`add-harnaas-install`, which are themselves still unimplemented.

## Impact

- Depends on `add-harnaas-foundation` for the manifest format, project-root resolution and the
  exit-code contract, and on `add-harnaas-install` for the lockfile contents, digest computation, ref
  resolution and transport rules. None of those are restated here.
- Constrains install's package layout: lint must compare against exactly what install would write, so
  the digest computation, the `AGENTS.md` block renderer and the `.gitignore` block renderer have to
  live in shared packages rather than being private to the install command. Two implementations that
  drift apart would make lint report phantom drift.
- Adds no new runtime dependency. Update detection reuses the Git ref resolution, the HTTPS transport
  and the semantic-version library install already pulls in.
- New code: an `internal/lint` package holding the finding model, the check set and the two report
  renderers; a command file registering `--frozen`, `--offline`, `--strict`, `--refresh` and `--json`
  locally, since the root command carries no persistent flags.
- Adds a resolution-result cache (ref resolution and tag listings) under the user cache directory,
  alongside the content-addressed archive cache install already maintains, honouring the same
  `HARNAAS_CACHE_DIR` override.
- Writes nothing into the project working tree, any harness directory, or the user's home.
- Makes the harness configuration enforceable in CI for the first time: `harnaas lint --frozen` as the
  per-PR gate, and a scheduled full `harnaas lint` for currency, so an upstream tag reddens a schedule
  rather than an unrelated pull request.
- Completes the three-command surface — after this change `init`, `install` and `lint` cover the whole
  intended workflow, and no further command is planned.
