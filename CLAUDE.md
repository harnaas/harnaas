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

### Decoding stops at the first failure; interpretation reports every violation

Both live in `cmd/harnaas/cli/manifest`, because the extraction trigger is an import cycle and there
is none to break between them. What separates them is when they stop. A document that will not parse
has no second problem to find, so decoding returns one error. Every asset entry is independent, so
interpretation accumulates `Violation` values — each carrying its asset index and field as data, not
only inside its sentence — and the aggregate orders itself the same way on every run. A `Violation`
is deliberately not an `error`: a type satisfying `error` invites a caller to return the first one it
saw, which is the behaviour accumulating exists to prevent.

`Interpret` is the only way to obtain an `Asset`, and it returns nothing at all when it found a
violation. That is how "a document with any violation is never handed to a later phase" is enforced
rather than merely stated — no later phase has another route to the type it would install from. The
aggregate sorts by asset index then field, so document-level problems come first (`DocumentIndex` is
negative) and two runs over one file produce byte-identical output.

### A question whose answer would have to be invented is not asked

Where an entry's source string did not parse there is no path to infer a type or an id from, so
inference is skipped unless the entry declared the field itself. Everything independent of the path —
`targets`, and the fields the entry declared — is still checked, because the point of accumulating is
one run per file. What is not done is reporting a second problem the author never wrote and sending
them to look for it.

Uniqueness is the one question no single entry can answer about itself, so it is asked last, over the
entries that had nothing else wrong. It is per type rather than per manifest — a skill and a command
may both be `review`, because each type is its own namespace to the harness — and a collision is
attributed to the later entry naming the earlier one, so it is one violation rather than two.

### A source is parsed, never resolved, and a GitHub source is always pinned

`ParseSource` recognizes the kind and checks the shape; nothing fetches, stats or resolves, so a
manifest can be validated with no network and no filesystem. A `github` source with no `@ref` is
rejected rather than defaulted to a branch — the manifest exists to say which version of somebody
else's content this repository trusts, and a default would make two installs of one manifest produce
different files. A `local` source pins nothing and must name a directory under `.harnaas`.

Asset paths are checked textually, before anything is opened: a path that escapes `.harnaas` must
never be read, not even to discover whether it exists. Absolute paths in both spellings and any
backslash are refused on every platform, because a committed manifest that names two different files
depending on who ran `install` is worse than one that fails.

### Type and id are inferred separately

`skills/` → skill, `rules/` → rule, `instructions/` → instruction, `commands/` → command, `agents/`
→ persona, with the leaf as the id and any extension stripped. `InferType` and `InferID` are separate
functions because the object form suppresses inference one field at a time: an entry declaring `type`
for an unconventional layout still wants its id inferred, and must not be refused for a directory
name nobody is relying on.

### A default is inherited once, and a wrong scope is refused rather than degraded

An asset's targets are its own `targets` when it declares them and the manifest's `harnesses`
otherwise, and a name inherited from `harnesses` is checked once against the roster rather than once
per asset that inherited it: one misspelling in one list is one mistake, and attributing it to every
entry would bury the entries with problems of their own. An entry's own `targets` are checked
per position, because two bad names there are two independent edits.

`user` scope is accepted only where the roster records an unambiguous per-user location for every
target, and declaring it elsewhere is a violation — never a silent fall back to `project`, which
would install the asset somewhere the author did not ask for and would not notice. An `instruction`
is project scope only, definitionally: what distinguishes it from a rule is surviving a fresh clone
inside a committed file, and at user scope there is neither. Because every harness on today's roster
*has* a per-user location, the refusal is exercised through a seam taking the roster query as a
parameter; the rule has to hold before the first harness that lacks one is added, not after.

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

### `init` refuses before it asks, and a decline is not a cancellation

Everything that can refuse — an unrecognized `--harness` name, a manifest already at the root —
happens before the prompt and before anything is written. A question whose answer cannot change the
outcome spends the one moment the user is paying attention. `--harness` replaces detection entirely
rather than merging with it: merging would make the manifest depend on what happened to be in the
working tree when init ran, and would leave no way to scaffold a manifest that omits a harness the
project already contains. Detection itself only stats the roster's evidence paths, in the roster's
order, and takes the roster as a parameter so the "every detected harness, deterministically ordered"
rule is exercised before a second harness exists to exercise it.

A declined prompt exits `0`; a cancelled one exits non-zero. Both write nothing, and only the
cancelled run left the question unanswered — reporting a user's own "no" as a failure would make
`init` the one command whose success depends on agreeing with it.

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
non-test sources — the two rules that fail *quietly* (reading the working directory, printing
through a `Print*` helper) are checked over the whole module's syntax, because a plausible-looking
`//nolint` reason passes review more easily than it should.

### No test reads or writes real user state

`internal/testenv` gives a package its own home, cache and config directories, and a package whose
files ask the standard library where those are must install it — a rule an AST test over the module
enforces rather than a convention. The failure it prevents is silent: a test that appended to the
developer's real log, or read a harness configuration that exists only on that machine, is green
locally and green in CI for different reasons. The redirect is verified rather than assumed — it
sets the variables each platform derives its directories from and then asks the standard library
where they are, so a platform whose mapping is missing fails the suite instead of quietly using the
real one. The Go toolchain's own directories are pinned to where they resolve first, because they
are derived from the same home and a test shelling out to `go build` would otherwise re-download the
module graph into a directory the suite deletes.

### The command surface is declared, not derived

A test that asked the command tree what the command tree contains would agree with any tree, so the
full set of commands — and whether each one has a `--json` view — is written out in the test and
compared whole. Adding a command is therefore two lines, and the second one is where somebody
decides whether the new verb is readable by a CI job or a coding agent. Nothing declares `--json`
yet: a document restating the path `init` just printed would be a JSON view invented to have one.

### The process contract is tested as a process

`e2e/`, behind the `e2e` build tag and run by `mise run test:e2e`, builds the binary and runs it.
What it asserts is the part of the contract that only exists once there is a process: the status a
shell reads, and whether an interrupt *killed* harnaas or harnaas exited with a number that looks
like it. Neither is reachable from inside the test binary — the exit code is the entrypoint's own
`os.Exit`, and the re-raised signal would kill the test run rather than the subject — so the two
rules that a user notices most (a failure is `1` and never `2`, and Ctrl-C escapes a `while true`
loop) would otherwise have no test at all. The exit-code table lists every way this binary can
succeed and every way it can fail, and asserts "not `2`" separately from the status each case
expects, so the reservation survives somebody updating a case. `HARNAAS_E2E_BIN` names a binary to
run instead of building one, for a runner that already built the one it means to ship.

## Deliberately not here

Telemetry, authentication, self-update and a plugin system. The source CLI has all four; none pays
off across three commands, and each adds a dependency and a privacy surface.
