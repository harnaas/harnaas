## Why

`harnaas install` puts assets in place and records what it did, but from that moment the record and
reality can diverge silently: a developer edits an installed skill, a file gets deleted, a teammate
adds an asset without installing it, or the upstream repository publishes a new version nobody
notices. Those are exactly the failures the tool exists to prevent, and none of them surface on their
own. `harnaas lint` is the command that surfaces them — locally before they cause confusion, and in
CI as an enforceable gate.

## What Changes

- New `harnaas lint` command that checks the manifest, the lockfile and the installed files against
  each other and reports every discrepancy it finds.
- Checks covering: an invalid manifest, an asset declared but never installed, an installed file
  modified locally, a missing or extraneous file within an installed asset, a destination claimed by
  a hand-written file, and a lockfile entry for an asset no longer declared.
- Upstream update detection: a mutable ref whose commit has moved since install, and a pinned tag
  that a newer release has superseded.
- Findings carry a severity, and every one carries the concrete command or action that resolves it.
- `lint` is strictly read-only. It never installs, repairs, or rewrites anything; `harnaas install`
  remains the only command that changes state.
- Exit code `2` distinguishes "checks ran and found problems" from "the command failed", so CI can
  tell the two apart.
- `--offline` skips every network check, `--json` emits a machine-readable report, and `--strict`
  promotes warnings to errors.

## Capabilities

### New Capabilities

- `lint-command`: the integrity check itself — the finding model, the full set of local checks,
  severity and exit-code semantics, output modes, and the read-only guarantee.
- `update-detection`: determining whether an installed asset has been superseded upstream — moved
  mutable refs, newer released tags, result caching, and graceful degradation when the network or
  credentials are unavailable.

### Modified Capabilities

None. This change adds new capabilities and does not alter the requirements introduced by
`add-harnaas-foundation` or `add-harnaas-install`.

## Impact

- Depends on `add-harnaas-foundation` for the manifest format and the exit-code contract, and on
  `add-harnaas-install` for the lockfile contents, digest computation, ref resolution and transport
  rules. None of those are restated here.
- Adds no new runtime dependency; update detection reuses the ref resolution and transport already
  built for install, and tag comparison reuses the semantic-version library it already pulls in.
- Adds a short-lived cache of ref-resolution results under the user cache directory so repeated lint
  runs do not hit the network on every invocation.
- Makes `harnaas lint` usable as a required CI check, which is the first time the project's own
  pipeline can enforce that installed harness assets match what was declared.
- Completes the three-command surface; after this change `init`, `install` and `lint` cover the full
  intended workflow.
