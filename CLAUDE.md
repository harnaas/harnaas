# harnaas

A Go CLI that manages a project's AI-harness assets as a declared, versioned dependency:
`harnaas.json` declares them, `harnaas install` places them, `harnaas lint` verifies them.

Two documents this file deliberately does not duplicate:

- **[CONTEXT.md](CONTEXT.md)** — the canonical vocabulary. *Harness*, *asset*, *skill*, *rule*,
  *instruction*, *command*, *persona*, *manifest*, *lockfile*, *managed*, *drift*, *finding*. Use
  these words and avoid the listed alternatives, in code, in output and in commit messages. Note in
  particular that the *manifest* is `harnaas.json` and the *lockfile* is `harnaas.lock.json` — the
  entire.io CLI this architecture is imported from uses "manifest" for the lockfile's role. Do not
  "correct" harnaas to match it.
- **[docs/adr/](docs/adr/)** — the load-bearing product decisions. Cite an ADR rather than
  re-arguing it; if you need to contradict one, write a new ADR.

## Architecture

The architecture is imported wholesale from the entire.io CLI (`github.com/entireio/cli`) rather
than designed fresh. The rules below are the imported ones plus harnaas's deliberate divergences.

### Layout

- `cmd/harnaas/` — the process entrypoint, and nothing else.
- `cmd/harnaas/cli/` — the bulk of the code, in one flat package. Command files are named
  `<noun>_group.go` for a group root and `<noun>_<verb>.go` for a leaf command.
- `internal/` — reserved for genuinely cross-binary code.
- A subpackage is extracted **only** to break a Go import cycle. That is the whole extraction
  trigger. There is no `domain` / `usecase` / `adapter` layering, and adding one is not an
  improvement — importing half an architecture buys the costs of both.

### Stack

cobra + pflag for the command tree, the Charm v2 stack on the `charm.land` module domain for
interactive surfaces, testify with gotestsum for tests. **No configuration library**: configuration
is `encoding/json` plus cobra flags plus environment variables, with precedence documented at each
read site.

mise is the single toolchain and task entry point (`mise run check` = fmt, then lint, then test).
Keep the Go version in `mise.toml` and the `go` directive in `go.mod` identical.

### The root command carries no persistent flags

A flag that applies to some commands is registered locally on each command that honours it —
`--json` included. A persistent `--json` would be accepted and silently ignored by every
side-effecting verb, and cobra cannot hide a persistent flag from a subset of children. Accepting a
flag and honouring it must be the same act.

### The entrypoint is the only component that prints an error or picks an exit code

Cobra's error and usage printing is silenced globally. The entrypoint switches on the returned
error and decides what to print and how to exit. A command that has already printed a friendly
explanation returns an error marked as already-printed — still unwrappable, so callers can inspect
the cause. Everything else is returned raw. Never print an error and also return it unmarked.

Exit codes: `0` success, `1` runtime failure, `2` **reserved** for a `lint` run that completed and
found error-severity findings, `128`+signum for signals. No command other than `lint` may exit `2`.

### Signals are re-raised, not swallowed

The first interrupt cancels the root context and prints a force-quit notice; a second terminates
immediately. On termination the process re-raises the original signal to itself rather than calling
a plain exit, falling back to `128`+signum only where re-raising is unsupported. A shell aborts a
`while true; do …; done` loop only when the child is *killed by* the signal — a plain exit with
status 130 is an ordinary exit, and the user's Ctrl-C never escapes the loop.

### The project root travels in `context.Context`

Resolved once from the enclosing repository and carried in the request context. Reading the process
working directory is banned by `forbidigo`, whose message names the replacement. There is no global
`-C` flag.

### Decoding strictness follows who writes the file

Committed, human-edited files decode **strictly** — an unknown field is nearly always a typo.
Machine-rewritten files decode **leniently** — a newer binary introduces fields an older binary
would otherwise reject, bricking the file with no fix available to the user who hits it. That test,
not the filename, is why `harnaas.json` is strict and `harnaas.lock.json` is lenient.

The manifest's `version` is read on its own, leniently, *before* the strict pass. A manifest written
by a newer harnaas carries fields this binary does not know, and strictness would report the first of
them — an arbitrary one — as a misspelling, sending the author hunting for a typo in a correct file.
Read first, the same manifest produces the message that helps: upgrade. Reading it with
`json.Unmarshal` rather than a decoder also settles trailing data, which a streaming decoder would
drop in silence.

### The manifest is read from the project root, and only from there

A `harnaas.json` below the root is an error naming that file, not a second declaration to merge:
merging would make the asset set depend on which directory a command ran from, and silently skipping
it is worse still, because its author believes it declares something. The search for one skips
dot-directories and dependency trees (`node_modules`, `vendor`) — a manifest inside a vendored
library is that library's, and its author is not the person harnaas would be talking to. The search
runs before the missing-manifest check, so a project whose only manifest sits in a subdirectory is
never told to run `harnaas init`.

### The harness roster is data only

`cmd/harnaas/cli/harness` holds an id, a display name, whether the harness has an unambiguous
per-user location, and the project-root-relative evidence that a project already uses it. It maps
nothing to a destination, stats nothing and writes nothing — `init` does the stat calls, and the
adapters that turn an asset into a file attach to these ids in a later change. Keeping the roster
behaviourless is what stops it and the adapters from drifting into two disagreeing answers, so a
test asserts the package imports no filesystem, network or environment package.

An id absent from the roster is a validation error rather than a pass-through, because the
`harnesses` list states a guarantee. An unrecognized id is one harnaas cannot make; an asset
installed for `claude-code` also being visible to another harness is not a bug.

### harnaas never writes the manifest, and every remedy is an edit

Apart from `init` creating it once, no command writes, reformats or normalizes `harnaas.json`. This
is why there is no `add`, `remove` or `update` command. The manifest is what a team reviews; a tool
that rewrites it makes its diffs untrustworthy. Phrase every remedy as the exact edit that fixes
it, not as a fix command.

### Output streams and logging

User-facing text goes to `cmd.OutOrStdout()`; advisory, progress and warning text to
`cmd.ErrOrStderr()`. Under `--json` the document is the only thing on stdout. Cobra's `Print*`
helpers write to stderr and are banned by `forbidigo`.

Diagnostics go to a log file through `log/slog`, never to the terminal — and never to a stream as a
fallback either: where the file cannot be opened, records are discarded, because a fallback that
turns a disk problem into a corrupted `--json` document is worse than no logging. The file lives
under the user's cache directory (`HARNAAS_LOG_FILE` overrides it), not under the project, so no
command leaves a log behind in a team's working tree. **Identifiers, paths,
durations, counts and outcomes may be logged. File contents, prompt or memory text, captured output
and credentials may not.** The files harnaas handles are a team's instructions and rules — exactly
the content nobody expects to find copied into a log they did not know existed.

### Non-interactive completeness

Every workflow is completable without a terminal, and no information is reachable only through a
prompt, picker or full-screen interface. The primary consumers are CI jobs and coding agents,
neither of which has a terminal. Prompts render through an accessible-mode wrapper, and colour
comes from the terminal's own base palette with body text left unstyled.

Whether a prompt may be shown is answered from the command's own streams plus the environment, never
by probing a controlling terminal: a coding agent hands its subprocess a real terminal nobody is
watching, and `harnaas init > out.txt` has one attached and still must not prompt. The decision is
biased towards "no", because a flag-driven path always exists while a prompt shown to something that
cannot answer does not degrade — it hangs.

A cancelled prompt is not a "no". Declining and walking away are different acts, and a command that
folds them together does the declined thing to a user who asked for nothing at all.

### Diagnostics have a shape

Every user-facing diagnostic is `{problem, fix}`: what is wrong, and the exact edit or command that
resolves it. Validation accumulates every violation into one aggregate error, ordered
deterministically, rather than stopping at the first.

### Writes are atomic

Files are staged in the destination directory, synced, renamed into place, and the staging file is
removed on both success and failure. A failed write leaves the previous file intact.

### Static analysis encodes the rules

golangci-lint runs the standard set plus the extended list in `.golangci.yaml`. `forbidigo` turns
"go through the abstraction" into a build failure whose message names the replacement. `nolintlint`
requires every suppression to name a specific linter and give a reason, so widening a rule is
visible in review. Where a rule must survive a suppression, it is also asserted by an AST test over
non-test sources.

## Deliberately not here

Telemetry, authentication, self-update and a plugin system. The source CLI has all four; none pays
off across three commands, and each adds a dependency and a privacy surface.
