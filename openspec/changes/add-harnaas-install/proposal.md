## Why

After `add-harnaas-foundation` a project can declare which harness assets it wants, but nothing puts
them anywhere — `harnaas.json` is a file nothing consumes. This change makes the declaration
executable: it resolves each declared source, places the resulting files into the right harness
location, and records exactly what landed and where it came from. That recorded provenance is also
the prerequisite for `lint`, which cannot detect drift or updates without knowing what was installed
and from which commit.

## What Changes

- New `harnaas install` command that resolves every declared asset, plans the resulting filesystem
  changes, applies them, and records the outcome.
- A source-resolution layer with a pluggable notion of source kind, implementing `github` (resolve a
  ref to a commit, fetch the repository archive over HTTPS, extract only the declared subtree) and
  `local` (read from the project's `.harnaas` directory).
- A harness-target layer: an adapter interface and registry that maps an asset type and scope to a
  concrete destination, with the `claude-code` adapter as the only implementation, plus an enforced
  import boundary keeping adapters decoupled from install internals.
- A new committed `harnaas.lock.json` recording, per asset, the normalized source, the resolved
  commit, and the digest of every installed file.
- Ownership is determined by the lockfile: a path recorded there is managed and may be updated; a
  path not recorded there is hand-written and is never overwritten.
- A managed, marker-delimited block in `CLAUDE.md` that references installed `rule` assets, since
  Claude Code has no rules directory of its own.
- `--dry-run` prints the plan without touching the filesystem; `--force` overwrites locally modified
  managed files.
- **BREAKING** for nothing — no prior released behavior exists.

## Capabilities

### New Capabilities

- `source-resolution`: turning a declared source into a verified, on-disk set of files — ref
  resolution, archive fetch, caching, and the transport and extraction safety rules.
- `harness-targets`: the harness adapter contract and registry, destination mapping per asset type
  and scope, harness detection, and the enforced import boundary around adapters.
- `install-command`: the resolve, plan, apply and record flow — per-target outcome states, conflict
  and drift handling, atomicity, concurrency safety, determinism, and partial-failure semantics.
- `install-lockfile`: the `harnaas.lock.json` provenance record — its contents, path portability
  across machines, decoding leniency, and durability.

### Modified Capabilities

None. This change adds new capabilities and does not alter the requirements introduced by
`add-harnaas-foundation`.

## Impact

- Depends on `add-harnaas-foundation` for the manifest format, project-root resolution, error and
  exit-code contract, and atomic-write helper. Those requirements are not restated here.
- Adds dependencies: a Git implementation for ref resolution, a file-locking library for lockfile
  serialization, and a semantic-version library for tag comparison. Archive handling and hashing use
  the standard library.
- Writes into the project working tree (`.claude/` and `CLAUDE.md` at project scope) and into the
  user's harness home for assets scoped to the user. Adds a content-addressed cache under the user
  cache directory.
- Introduces the first network access in the CLI, and with it the transport security rules that
  `lint`'s update check will reuse.
- Produces the lockfile that `add-harnaas-lint` reads; the two changes must agree on its contents,
  which is why it is specified here rather than there.
