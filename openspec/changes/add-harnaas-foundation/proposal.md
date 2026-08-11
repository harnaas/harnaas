## Why

Teams accumulate AI-harness assets — skills, rules, slash commands, subagents — as untracked files
copy-pasted into `.claude/` by whoever set the machine up. Nobody knows which version is installed,
whether it was edited locally, or whether upstream has moved on. `harnaas` makes those assets a
declared, versioned, verifiable dependency of the project. This change lays the foundation every
later command stands on: the binary, its architecture, the `harnaas.json` declaration format, and
the `init` command that creates it.

## What Changes

- New Go CLI `harnaas` (module `github.com/harnaas/harnaas`, binary `cmd/harnaas`), with its
  architecture, layout, stack and tooling imported from the entire.io CLI.
- A process entrypoint that owns error rendering and exit codes, plus two-stage signal handling that
  re-raises the terminating signal rather than calling `os.Exit`.
- A cobra root command with **no persistent flags**, silenced cobra error/usage printing, and help
  groups declared at registration time.
- Project-root resolution that travels in `context.Context` rather than reading process cwd.
- The `harnaas.json` declaration format: version, target harnesses, and a list of assets, each with a
  type, a source (remote GitHub or local `.harnaas/` path), targets and an install scope. Strict
  decoding, plus normalization of the string source shorthand into the canonical object form.
- A new `harnaas init` command that detects installed harnesses, writes `harnaas.json`, and refuses
  to overwrite an existing file without `--force`.
- No network access, no installing, and no writing into any harness directory — `install` and `lint`
  arrive in the two follow-on changes.

## Capabilities

### New Capabilities

- `cli-foundation`: the binary's process lifecycle — command tree construction, flag conventions,
  error and exit-code contract, signal handling, version stamping, project-root resolution, logging
  and user-facing output separation, and the non-interactive-fallback guarantee.
- `harnaas-manifest`: the `harnaas.json` declaration format — schema, discovery, decoding strictness,
  source shorthand normalization, and validation rules.
- `init-command`: `harnaas init` — harness detection, scaffold content, overwrite protection, and its
  interactive and non-interactive paths.

### Modified Capabilities

None. This change introduces the project; there are no existing specs.

## Impact

- Creates the repository from empty: `go.mod`, `cmd/harnaas/`, `internal/`, `mise.toml`,
  `mise-tasks/`, `.golangci.yaml`, `.goreleaser.yaml`, `.github/workflows/`, `CLAUDE.md`.
- Adds runtime dependencies: `spf13/cobra`, `spf13/pflag`, and the `charm.land` v2 stack
  (`huh/v2`, `lipgloss/v2`) for the `init` form. No config library — `encoding/json` and cobra flags
  only.
- Adds development dependencies: `stretchr/testify`, `gotestsum`, `golangci-lint`, `mise`,
  `goreleaser`, `go-licenses`.
- Establishes conventions that changes `add-harnaas-install` and `add-harnaas-lint` build on and must
  not restate: the flat command package, the registry-plus-AST-boundary-test pattern, atomic writes,
  and the `{problem, fix}` shape of user-facing diagnostics.
- No user-visible install behavior ships here; `harnaas init` produces a file that nothing yet
  consumes.
