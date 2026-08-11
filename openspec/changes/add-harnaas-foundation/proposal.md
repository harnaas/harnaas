## Why

Teams accumulate AI-harness assets — skills, rules, instructions, commands, personas — as untracked
files copy-pasted into `.claude/` by whoever set the machine up. Nobody can say which version is
installed, whether somebody edited it locally, or whether upstream has moved on. harnaas makes those
assets a declared, versioned, verifiable dependency of the repository, so every developer and CI get
the same set. This change lays the foundation the other two commands stand on: the binary and its
process contract, the `harnaas.json` declaration every later phase reads, and the `init` command that
creates it.

## What Changes

- New Go CLI `harnaas` (module `github.com/harnaas/harnaas`, binary `cmd/harnaas`), with its
  architecture, layout, stack and tooling imported from the entire.io CLI.
- A process entrypoint that is the only component rendering errors and mapping them to exit codes —
  `0` success, `1` runtime failure, `2` reserved for lint findings, `128`+signum for signals — plus
  two-stage interrupt handling that re-raises the original signal instead of calling a plain exit.
- A cobra root command with **no persistent flags**, cobra's own error and usage printing silenced,
  subcommands attached by explicit constructor calls rather than by package-init side effect, and
  `--json` registered locally by each command that honours it.
- Project-root resolution performed once and carried in `context.Context`, with static analysis
  failing any direct read of the process working directory and naming the replacement.
- Output and logging separation: user-facing text on the command's own output stream, `--json`
  documents alone on stdout with advisory text on stderr, and structured diagnostics to a log file
  that must never contain file contents, prompts or credentials.
- Non-interactive completeness as a process-level rule: every workflow driveable by flags, prompts
  rendered in an accessible mode on request, and colour drawn only from the terminal's base palette
  with body text left unstyled.
- The `harnaas.json` format: a committed, hand-edited, strictly decoded document at the repository
  root only, carrying `version`, a default `harnesses` list, a `sources` map of key → repository at a
  ref, and an `assets` array.
- Asset entries as strings — `<sourceKey>:<path>` for a declared source, or a `.harnaas/…` path for
  local content — whose **type and id are inferred from the path**: the containing directory names
  the type (`skills/`, `rules/`, `instructions/`, `commands/`, `agents/`), the leaf names the id.
- An object form for assets that overrides `type`, `id`, `targets` and `scope` where a source does not
  follow that directory convention or an asset needs to differ from the manifest's defaults.
- Manifest validation ahead of every later phase: declared-source resolution, local containment under
  `.harnaas`, target defaulting from `harnesses`, scope defaulting to `project` with `user` accepted
  only where a harness has an unambiguous per-user location and never on an `instruction`, unique
  single-segment ids, and every violation reported together rather than one per run.
- A data-only harness roster — recognized ids, the default id, per-user-location availability, and the
  observable evidence that a harness is present — which manifest validation and `init` detection both
  read. The adapters that give those ids installation behaviour arrive with `add-harnaas-install`.
- A new `harnaas init` that detects present harnesses, confirms the selection through an accessible
  prompt or accepts it from flags, and writes **exactly one file** — `harnaas.json`, atomically —
  refusing to replace an existing manifest without `--force`.
- Explicitly not in this change: no network access, no lockfile, no installing, and no writing to any
  harness directory, `AGENTS.md`, `CLAUDE.md` or `.gitignore`. Those belong to `install` and `lint`.

## Capabilities

### New Capabilities

- `cli-foundation`: the process-level contract every command inherits — command tree construction,
  flag scoping, error rendering, the exit-code taxonomy, interrupt handling, version reporting,
  project-root resolution, output-stream discipline, logging privacy, non-interactive operation and
  terminal presentation.
- `harnaas-manifest`: the `harnaas.json` declaration — root-only location, read-only handling, strict
  decoding, document shape, the `sources` block, the asset string grammar, path-inferred type and id,
  the object override form, target and scope defaulting, semantic validation and version handling.
- `init-command`: `harnaas init` — manifest scaffolding, harness detection and fallback, single-file
  side effects, guidance-only setup advice, overwrite protection, the interactive and non-interactive
  selection paths, and atomic write durability.

### Modified Capabilities

None. `openspec/specs/` is empty and nothing is implemented yet; this change introduces the project,
so all three capabilities above are new and none is modified.

## Impact

- Creates the repository from empty: `go.mod`, `cmd/harnaas/main.go`, `cmd/harnaas/cli/`, `internal/`,
  `mise.toml`, `mise-tasks/`, `.golangci.yaml`, `.goreleaser.yaml`, `.github/workflows/`, and the
  `CLAUDE.md` / `AGENTS.md` pair recording the imported architecture rules.
- Adds runtime dependencies: `spf13/cobra` and `spf13/pflag` for the command tree, and the
  `charm.land` v2 stack (`huh/v2`, `lipgloss/v2`) for `init`'s prompt. No configuration library —
  `encoding/json`, cobra flags and environment variables only.
- Adds development dependencies and gates: `stretchr/testify`, `gotestsum`, `golangci-lint` v2,
  `mise` as the single toolchain and task entry point, `goreleaser` for stamped release builds, and
  `go-licenses` as an allowlist gate.
- Touches only files it creates plus `harnaas.json` at the project root. No user harness state, no
  network calls, no cache directory, no telemetry, no authentication.
- Reserves exit code `2` although no command in this change can emit it, so `add-harnaas-lint` does
  not have to extend the contract later.
- Fixes conventions the other two changes build on and must not restate: the flat command package, the
  silenced-cobra plus entrypoint-renders-errors split, atomic staged writes, the strict-versus-lenient
  decoding rule keyed on who writes a file, non-interactive completeness, and the `{problem, fix}`
  shape of every user-facing diagnostic.
- Ships no installable behaviour: `harnaas init` produces a manifest that nothing yet consumes.
  `add-harnaas-install` makes the declaration executable; `add-harnaas-lint` makes it enforceable.
