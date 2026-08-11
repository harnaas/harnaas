## Context

harnaas starts from an empty repository, so every structural choice is still open. Rather than invent
one, this change imports the architecture of the entire.io CLI (`github.com/entireio/cli`), which
solves an adjacent problem — a Go CLI that installs versioned content into AI coding harnesses and
later verifies its integrity — and has already paid for its decisions in production. What follows
records those decisions, the reasoning behind them, and the places harnaas deliberately diverges.

Four forces shape the rest:

- **The manifest is the contract between all three commands.** `install` reads it to know what to
  fetch; `lint` reads it to know what should be present. Anything left ambiguous here is re-litigated
  twice, in changes that cannot change it without breaking the other.
- **The product is team standardization, not personal portability.** A repository declares its assets,
  everyone gets the same set, and CI enforces it. That is why the manifest is committed, hand-edited
  and reviewable, and why `lint` — not a wizard — is the enforcement surface.
- **`install` and `lint` are meant to run unattended in CI and to be driven by coding agents.**
  Non-interactive completeness and machine-readable output are structural requirements here, not
  polish added later.
- **Go's import-cycle rule is the real force acting on package layout.** In the source architecture it
  is named as the reason for nearly every extracted package, and it is the only extraction trigger
  this change adopts.

The load-bearing product decisions are recorded as ADRs under `docs/adr/`; the canonical vocabulary is
`CONTEXT.md` at the repository root. Where a decision below is settled in an ADR, it is cited rather
than re-argued.

## Goals / Non-Goals

**Goals:**

- Reproduce the source architecture's command layer, error and exit contract, signal handling and
  decoding conventions closely enough that its rules transfer without re-derivation.
- Fix the `harnaas.json` format now, in full, including how an asset's type and id are determined.
- Establish the layout, tooling and diagnostic conventions that the later changes extend rather than
  reshape.
- Make every workflow completable without a terminal.

**Non-Goals:**

- Fetching anything over the network, installing into a harness, or writing a lockfile — all deferred
  to `add-harnaas-install`.
- Integrity checking and update detection — deferred to `add-harnaas-lint`.
- Harness adapters. This change ships a roster of harness *identities*; the adapters that map an asset
  to a destination arrive with `install`.
- Telemetry, authentication, self-update and a plugin system. The source CLI has all four; none is
  needed for three commands, and each adds a dependency and a privacy surface for no present benefit.
- `add`, `remove`, `update` and `uninstall` commands. The manifest is edited by hand; convergence on
  it is `install`'s job.

## Decisions

### The architecture is imported wholesale rather than designed fresh

Go, cobra plus pflag for the command tree, the Charm v2 stack on the `charm.land` module domain for
interactive surfaces, testify with gotestsum for tests, and mise as the single toolchain and task
entry point with golangci-lint, goreleaser and a license gate behind it.

Layout follows the source repository's less common convention: the bulk of the code lives in a flat
`cmd/harnaas/cli` package with `<noun>_group.go` / `<noun>_<verb>.go` naming, `internal/` is reserved
for genuinely cross-binary code, and a subpackage is extracted only to break an import cycle. A
`domain` / `usecase` / `adapter` layering was rejected because the source architecture explicitly does
not have one, and importing half of an architecture buys the costs of both.

There is **no configuration library**. Configuration is `encoding/json` plus cobra flags plus
environment variables, with precedence documented at each read site. For a tool whose entire job is
reading two JSON files, viper or koanf would be a dependency that adds indirection and no capability.

### The root command carries no persistent flags

A flag that applies to some commands is registered locally on each command that honours it. The
reason is concrete: a persistent `--json` would be accepted and silently ignored by every
side-effecting verb, and cobra offers no way to hide a persistent flag from a subset of children. The
cost is a one-line registration per command; the benefit is that accepting a flag and honouring it
become the same thing.

The same reasoning explains why the project root travels in `context.Context` instead of arriving via
a global `-C` flag, and why reading the process working directory is banned outright by static
analysis rather than merely discouraged: a rule the compiler or the linter enforces survives review
turnover, and a convention does not.

### The entrypoint is the only component that prints an error or chooses an exit code

Cobra's error and usage printing is silenced globally; the entrypoint switches on the returned error
and decides what to print and how to exit. A command that has already printed a friendly explanation
returns an error marked as already-printed — still unwrappable, so callers can inspect its cause — and
everything else is returned raw. This keeps the exit policy in one readable place instead of spread
across every command, and it makes "printed exactly once" a property of the design rather than of
each author's discipline.

### Exit code 2 is reserved now, though nothing in this change can emit it

The source CLI has no exit-code taxonomy: `1` for every failure and `128`+signum for signals. harnaas
keeps that and adds exactly one code, `2`, for a diagnostic command that ran correctly and found
problems. Without it a CI job cannot distinguish "lint crashed" from "lint worked and your harness has
drifted" — which is the single most important distinction lint exists to make, and the reason
[0004-available-updates-are-lint-errors.md](../../../docs/adr/0004-available-updates-are-lint-errors.md)
can make a non-zero exit meaningful at all.

It is specified here, in the foundation, rather than in `add-harnaas-lint` for two reasons: the
contract belongs to the process, not to one command; and reserving it up front lets this change also
state the negative rule — that no other command may ever exit `2` — which is not enforceable if the
code is introduced later by the command that uses it.

### Signals are re-raised, not swallowed

The first interrupt cancels the root context and prints a notice; a second forces exit. On termination
the process re-raises the original signal to itself instead of calling a plain exit, falling back to
`128`+signum only where re-raising is unsupported. The source CLI explains why this matters: a shell
aborts a `while true; do …; done` loop only when the child is *killed by* the signal. A plain exit with
status 130 is an ordinary exit, so the loop keeps respawning and the user's Ctrl-C never escapes.

### Decoding strictness follows who writes the file, not what the file is called

This is the subtlest imported rule, and it decides the manifest's behaviour. Committed, human-edited
files decode **strictly** — an unknown field is nearly always a typo, and surfacing it immediately is
worth the cost. Machine-rewritten files decode **leniently**, because a newer binary introduces fields
an older binary would then reject, bricking the file for everyone still on the older version, with no
fix available to the user who hits it.

Applying that test rather than a naming convention is what puts harnaas's two files on opposite sides:
`harnaas.json` is hand-edited, so it is strict; `harnaas.lock.json` is machine-written, so it is
lenient. That split is only coherent because ownership is recorded in the lockfile rather than in the
manifest or in markers inside installed files — see
[0001-ownership-lives-in-the-lockfile.md](../../../docs/adr/0001-ownership-lives-in-the-lockfile.md).

### harnaas never writes the manifest, and every remedy is an edit

Apart from `init` creating the file once, no command writes, reformats or normalizes `harnaas.json`.
This is why there is no `add`, `remove` or `update` command: they would exist only to edit a file a
person can edit.

The reason is that the manifest is the artifact a team reviews. A tool that rewrites it makes its diffs
untrustworthy — a reviewer can no longer tell an intentional pin bump from a normalization pass — and
reformatting churn would land in every pull request that ran a command. Phrasing every remedy as an
exact edit rather than a fix command also keeps the human in the loop on the one decision that
actually matters: which version of somebody else's content this repository is about to trust.

### An asset's type and id are inferred from its path, with an object form as the escape hatch

`acme:skills/review` says everything: the containing directory names the type, the leaf names the id.
Writing `{"source": "acme:skills/review", "type": "skill", "id": "review"}` would restate in twelve
tokens what the path already said, and would introduce a second source of truth that can disagree with
the first.

The convention is not invented here — `skills/`, `commands/` and `agents/` are already how asset
repositories are laid out, because that is how the harnesses themselves read them. Inference makes the
common case one string per asset, which keeps the manifest short enough to review as a list. A path
whose containing directory names no type is an error that points at the object form rather than a
guess, so the failure mode is a message rather than an asset silently installed as the wrong type.

The object form then carries the genuinely per-asset decisions that no path can express — a narrowed
`targets` list and a non-default `scope` — so there is exactly one place to look when an asset behaves
differently from its neighbours.

### Sources are declared once under a key, and assets reference the key

A repository and ref appear exactly once, in `sources`. The alternative — a full `github:acme/assets@v1.2.0`
prefix on every asset string — makes a version bump an N-line edit where N is the number of assets from
that repository, and makes it possible for two assets to disagree about which ref they came from while
both look correct in review.

Keying also gives the later changes a natural unit: `install` fetches one archive per repository and
commit regardless of how many assets it covers, and `lint` checks for a newer tag once per source
rather than once per asset. Both fall out of the manifest shape instead of needing a deduplication
pass.

### The `harnesses` list states a guarantee, not exclusivity

Because skills and instructions land in shared locations that many harnesses read — see
[0002-shared-agents-target-before-per-harness.md](../../../docs/adr/0002-shared-agents-target-before-per-harness.md)
— a harness that is *not* in the list will still see assets installed for one that is. The list
therefore means "the harnesses we guarantee this works for", not "the harnesses that can see this".

This matters to the manifest's validation rules, which is why it is settled here: an unrecognized
harness name is an error because it is a guarantee harnaas cannot make, but an asset installed for
`claude-code` being visible to Cursor is not a bug to be prevented. It also sets what `init`'s
detection is for — pre-filling a list of guarantees, not enumerating what is installed on the machine.

### Scope defaults to project, and `user` is validated rather than degraded

`user` scope is accepted only where a target harness has an unambiguous per-user location, and
declaring it elsewhere is a validation error. Silently falling back to project scope would install the
asset somewhere the author did not ask for and would not notice, and a wrong install location is
exactly the failure harnaas exists to eliminate.

An `instruction` is project scope only, and that is a definitional consequence rather than a
limitation: an instruction differs from a rule by surviving a fresh clone inside a committed file —
see
[0003-instruction-content-in-an-agents-md-block.md](../../../docs/adr/0003-instruction-content-in-an-agents-md-block.md).
At user scope there is no clone and no committed file, so the distinction from a rule disappears and
the type would mean nothing.

### One manifest, at the repository root

A `harnaas.json` in a subdirectory is an error naming the root manifest, not a second declaration to
be merged. Merging would make the effective asset set depend on which directory a command ran from,
which contradicts the whole point: the repository declares one set of assets and everyone gets it.
Erroring instead of ignoring is deliberate too — a file named `harnaas.json` that harnaas silently
skips is worse than one it rejects.

### A data-only harness roster ships before the adapters that give it meaning

Manifest validation must reject an unknown harness name and must know whether a harness has an
unambiguous per-user location, and `init` must detect which harnesses a project already uses. All
three needs land in this change, while the adapters that map an asset type to a destination land in
the next one.

The seam is drawn so the two cannot drift into conflict: the roster is **data only** — id, display
name, whether a per-user root exists, and the observable evidence that the harness is present — with
no destination mapping and no write behaviour. `add-harnaas-install` attaches adapters to those ids
and asserts in its registry test that every adapter's id exists in the roster. The alternative,
deferring the roster into `install`, would have forced this change to accept any string as a harness
name and then reject it two phases later, at a point where the error can no longer name the manifest
line that caused it.

### `init` writes exactly one file, and its advice is only advice

`init` writes `harnaas.json` and nothing else: no harness directory, no `.harnaas/`, no `AGENTS.md`,
no `CLAUDE.md`, no ignore-file entry. Remaining setup is printed as guidance naming the command that
performs it, and there is deliberately no flag that makes `init` perform it.

The reason is ownership. A destination is managed only when the lockfile records it
([0001](../../../docs/adr/0001-ownership-lives-in-the-lockfile.md)), and `init` writes no lockfile —
so anything else it created would be *unmanaged*, and the very next `install` would report a conflict
against `init`'s own output. Entries in the ignore file have the same problem in reverse: `install`
maintains a managed block listing exactly the paths it installed, and an entry written before anything
is installed is a claim about files that do not exist. Keeping `init` to one file is what makes both
of those rules hold without special cases.

### Interactive is an affordance, never the only path

Every workflow is completable from a non-interactive terminal, and no information is reachable only
through a prompt, picker or full-screen interface. For `init` the form is a convenience over a
flag-driven path that is always available and is selected automatically when no terminal is attached.
This is not accessibility theatre: the primary consumers of these commands are CI jobs and coding
agents, neither of which has a terminal, and a workflow that only a human can complete is a workflow
CI cannot enforce.

Prompts render through an accessible-mode wrapper rather than the form library directly, and colour
comes from the terminal's own base palette with body text left unstyled, so output stays legible on a
light background and inverts with the user's theme.

### Logging is structured, file-bound and free of user content

Diagnostics go to a log file through `log/slog`, never to the terminal, so they can never interleave
with user output or corrupt a `--json` document. The privacy rule is inherited verbatim and treated as
hard: identifiers, paths, durations, counts and outcomes may be logged; file contents, memory or prompt
text, captured output and credentials may not. For harnaas this is sharper than usual, because the
files it handles — a team's instructions and rules — are exactly the ones nobody expects to find copied
into a log they did not know existed.

### Static analysis encodes the architecture rules, and the message names the replacement

golangci-lint runs the standard set plus roughly fifty additional linters, and — copying the source
CLI's most transferable idea — `forbidigo` rules turn "you must go through the abstraction" into build
failures whose messages name the function to use instead. The working-directory ban and the ban on
cobra's `Print*` helpers (which write to stderr and would corrupt piped output) both start here.
`nolintlint` requires every suppression to name a specific linter and give a reason, so widening a rule
is visible in review. The import-boundary AST test that protects the adapter registry follows the same
pattern and lands with `add-harnaas-install`, alongside the registry it protects.

## Risks / Trade-offs

- **Importing an architecture from a much larger CLI risks over-building.** harnaas has three commands;
  the source has dozens. Mitigated by importing the conventions and explicitly declining the subsystems
  — telemetry, auth, plugins, self-update — that only pay off at that scale.
- **A flat command package grows badly if the CLI does.** Accepted deliberately: the source CLI runs
  this convention past a hundred files, and the extraction trigger (an import cycle) is objective
  rather than a matter of taste.
- **Path-inferred type and id fails for a source that does not follow the convention.** The object form
  is the escape hatch, but a source laid out differently needs an object per asset, and the manifest
  loses the brevity that motivated inference. Accepted because the convention is the ecosystem's, and
  because the failure is a message naming the fix rather than a wrong install.
- **Strict decoding rejects a manifest written by a newer harnaas.** That is the intended trade for
  catching typos, and it is bounded by the version field: the failure is an explicit "upgrade harnaas"
  message rather than a confusing unknown-field error.
- **The root-only rule is unfriendly to monorepos**, where per-package harness assets are a plausible
  want. Accepted for v1: one declaration per repository is what makes the team-standardization story
  true, and relaxing it later is additive while tightening it later would not be.
- **The harness roster duplicates knowledge the adapters will own.** Mitigated by keeping it data-only
  and by the registry test in `add-harnaas-install` that fails if an adapter's id is missing from it,
  so the duplication cannot silently diverge.
- **`init`'s detection can be wrong in both directions** — a committed `.claude/` in a repository
  nobody uses Claude Code on, or a harness configured only in the user's home. Accepted because
  detection only pre-fills a selection the user confirms or overrides with a flag, and because the
  manifest states a guarantee rather than an observation.
- **Reserving exit code `2` diverges from the imported contract.** Contained by specifying it once in
  `cli-foundation`, giving it exactly one meaning, and stating the negative rule that no other command
  may use it.
- **There is no filesystem abstraction, so tests touch a real filesystem.** Accepted, following the
  source CLI, which uses temporary directories instead. Traversal safety comes from kernel-level
  directory confinement rather than from an interface seam, which is the stronger guarantee of the two.

## Open Questions

- **Does a `local` source key add anything over the `.harnaas/…` string form?** Both are accepted, and
  for the common case the path form is plainly better. The keyed form is kept because it lets a future
  source kind slot into one grammar rather than growing a second one; if it earns nothing by the time
  `install` ships, it should be removed while there is still no released behaviour to keep.
- **Should `harnaas.json` support a per-package manifest in a monorepo?** Deferred with the root-only
  rule above. Any answer must say what happens when two packages declare the same asset id.
- **Should a `rule` be expressible as inline content in the manifest** rather than always pointing at a
  file? Deferred: the file-pointer form is sufficient for v1, and inline content would complicate the
  digest model `lint` depends on.
- **Should harnaas publish a JSON Schema for `harnaas.json`** so editors can complete and validate it?
  Attractive given strict decoding, but validation lives in code today and a schema would be a second
  source of truth to keep in step. Deferred until the format has stopped moving.
