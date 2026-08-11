## Context

`harnaas` starts from an empty repository, so every structural choice is still open. Rather than
invent one, this change imports the architecture of the entire.io CLI (`github.com/entireio/cli`),
which solves an adjacent problem — a Go CLI that installs versioned artifacts into AI coding
harnesses and later verifies their integrity — and has already paid for its decisions in production.
The design below records those decisions, the reasoning behind them as stated in that codebase, and
the three places `harnaas` deliberately diverges.

Constraints that shape the rest:

- The two follow-on changes (`add-harnaas-install`, `add-harnaas-lint`) build directly on what is
  fixed here; anything left ambiguous now gets re-litigated twice.
- Both `install` and `lint` are meant to run unattended in CI and to be driven by coding agents, so
  non-interactive completeness and machine-readable output are structural requirements, not polish.
- Go's import-cycle rule is the real force acting on package layout. In the source architecture it is
  named as the reason for almost every extracted package.

## Goals / Non-Goals

**Goals:**

- Reproduce the source architecture's command layer, error/exit contract, signal handling, and
  configuration-decoding conventions closely enough that its rules transfer without re-derivation.
- Fix the `harnaas.json` format now, since it is the contract between all three commands.
- Establish the layout and tooling that later changes extend rather than reshape.
- Make every workflow completable without a terminal.

**Non-Goals:**

- Fetching anything over the network, installing into a harness, or writing a lockfile — all deferred
  to `add-harnaas-install`.
- Integrity checking or update detection — deferred to `add-harnaas-lint`.
- Telemetry, authentication, self-update, and a plugin system. The source CLI has all four; none is
  needed for the three commands in scope, and each would add a dependency and a privacy surface for
  no present benefit.
- Supporting harnesses other than Claude Code. The adapter boundary is specified in the next change
  so that adding one later is additive.

## Decisions

### Language, stack and layout are taken wholesale from the source CLI

Go, `spf13/cobra` + `pflag` for the command tree, the Charm v2 stack on the `charm.land` module
domain (`huh/v2` for forms, `lipgloss/v2` for styling) for interactive surfaces, `stretchr/testify`
with `gotestsum` for tests, and `mise` as the single toolchain and task entry point with
`golangci-lint`, `goreleaser` and a license gate behind it.

Layout follows the source repo's less common convention: the bulk of the code lives in a flat
`cmd/harnaas/cli` package with `<noun>_group.go` / `<noun>_<verb>.go` file naming, and `internal/` is
reserved for code genuinely shared across binaries. Subpackages get extracted only when there is a
forcing reason, which in practice means breaking an import cycle. The alternative — a `domain` /
`usecase` / `adapter` layering — was rejected because the source architecture explicitly does not have
one, and importing half of an architecture yields the costs of both.

**No configuration library.** Config is `encoding/json` plus cobra flags plus environment variables,
with precedence documented at each read site. The source CLI has no viper or koanf and this keeps the
dependency surface small for a tool whose entire job is reading two JSON files.

### The root command carries no persistent flags

Flags that only apply to some commands are registered locally on each command that honours them. The
source CLI documents the reason: a persistent `--json` would be silently accepted and ignored by every
side-effecting verb, and cobra cannot hide a persistent flag from a subset of children. The cost is
repeating a one-line registration per command; the benefit is that accepting a flag is the same thing
as honouring it.

This is why the project root travels in `context.Context` rather than arriving via a global `-C`
flag: the source CLI resolves the repository root and threads it through the context, and forbids
reading the process working directory outright.

### Lint encodes the architecture rules, and the message names the replacement

`golangci-lint` runs with the standard set plus roughly fifty additional linters, and — copying the
source CLI's most transferable idea — `forbidigo` rules encode the project's "you must go through the
abstraction" rules as errors that name the function to use instead. The working-directory ban and the
ban on cobra's `Print*` helpers (which write to stderr, corrupting piped output) both start here.
`nolintlint` requires every suppression to name a specific linter and give a reason.

Import boundaries are enforced by a hand-written AST test against an allowlist rather than by
`depguard`. The source CLI's version fails with *"if this is intentional, add it to allowedPrefixes in
architecture_test.go"*, which makes widening the boundary a deliberate, reviewable edit. The concrete
boundary lands in `add-harnaas-install` alongside the adapter registry it protects.

### Errors are printed once, by the entrypoint

Cobra's error and usage printing is silenced globally; the entrypoint switches on the returned error
and decides what to print and how to exit. Commands that have already printed a friendly explanation
return an error marked as already-printed so it is not duplicated; everything else is returned raw.
This keeps the exit-code policy in one readable place instead of spread across every command.

### Exit code 2 means "findings", and it is the one deviation from the source contract

The source CLI has no exit-code taxonomy: `1` for every failure, `128+signum` for signals. `harnaas`
keeps that and adds exactly one code — `2` for a diagnostic command that ran correctly and found
problems. Without it, a CI job cannot distinguish "`lint` crashed" from "`lint` worked and your
harness has drifted", which is the single most important distinction `lint` exists to make. This is
the conventional linter contract, and it is the only place the imported model is extended.

### Signals are re-raised, not swallowed

The first interrupt cancels the root context and prints a notice; a second forces exit. On
termination the process re-raises the original signal to itself instead of calling exit, falling back
to `128+signum` only where re-raising is unsupported. The source CLI explains why this matters: a
shell only aborts a `while true; do …; done` loop when the child is *killed by* the signal — a plain
exit with status 130 is an ordinary exit, so the loop keeps respawning and the user's Ctrl-C never
escapes.

### Decoding strictness follows who writes the file, not who authored the content

This is the subtlest imported rule and it decides the manifest's behaviour. The source CLI decodes its
committed, team-edited settings **strictly**, rejecting unknown fields because they are usually typos
worth surfacing immediately. It decodes machine-rewritten files **leniently**, because a newer binary
will introduce fields an older binary would then reject, bricking that file for everyone still on the
older version — a break the user cannot fix.

Applying that test: `harnaas.json` is committed and hand-edited, so it decodes strictly. The lockfile
introduced in the next change is machine-rewritten, so it decodes leniently. Deriving the rule from
*who writes the file* rather than from "manifests are lenient" is what makes these two land on
opposite sides.

### Interactive is an affordance, never the only path

Every workflow must be completable from a non-interactive terminal, and information the user needs
must never be reachable only through a prompt or picker. The source CLI states this as a review rule
and mandates that reviewers flag any change where a non-interactive caller can only see a menu. For
`init` it means the interactive form is a convenience over a flag-driven path that is always
available and is selected automatically when no terminal is attached. Prompts render through an
accessible-mode wrapper rather than the form library directly, and colours come from a base16-only
palette with body text left unstyled so it inverts with the terminal theme.

### Logging is structured, file-bound, and free of user content

Diagnostic logging goes to a log file through `log/slog`, never to the terminal, and user-facing
output goes to the command's own output stream. The privacy rule is inherited verbatim as a hard
rule: logs must not contain user data — identifiers, paths, durations, counts and outcomes are
permitted; file contents and prompts are not. For `harnaas` this matters because the files it handles
are exactly the ones a user would not expect to be copied into a log.

## Risks / Trade-offs

- **Importing an architecture from a much larger CLI risks over-building.** `harnaas` has three
  commands; the source has dozens. Mitigated by importing the conventions and explicitly declining
  the subsystems (telemetry, auth, plugins, self-update) that only pay off at that scale.
- **A flat command package grows badly if the CLI does.** Accepted deliberately: the source CLI runs
  this convention at more than a hundred files and the extraction trigger — an import cycle — is
  objective rather than a matter of taste.
- **Strict manifest decoding will reject a manifest written by a newer `harnaas`.** That is the
  intended trade for catching typos, and it is bounded by the version field: the failure is an
  explicit "upgrade `harnaas`" message rather than a confusing unknown-field error.
- **No filesystem abstraction means tests touch a real filesystem.** Accepted, following the source
  CLI, which uses temporary directories rather than an abstraction layer. Traversal safety comes from
  kernel-level directory confinement instead, which is stronger than an interface seam would be.
- **Adding exit code 2 diverges from the imported contract.** Contained by specifying it once in
  `cli-foundation` and by reserving it for exactly one meaning, so it cannot proliferate.

## Open Questions

- Whether `rule` assets should eventually be expressible as inline content in `harnaas.json` rather
  than always pointing at a file. Deferred: the file-pointer form is sufficient for v1 and inline
  content would complicate the digest model that `lint` depends on.
- Whether `harnaas` should ship a JSON Schema for `harnaas.json` for editor completion. Deferred; the
  source CLI generates no schemas and validation lives in code today.
