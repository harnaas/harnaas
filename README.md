# harnaas

A Go CLI that manages a project's AI-harness assets as a declared, versioned dependency.

`harnaas.json` declares them, `harnaas install` places them, `harnaas lint` verifies them.

---

## Table of contents

- [Why](#why)
- [Status](#status)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Concepts](#concepts)
- [Command reference](#command-reference)
  - [`harnaas`](#harnaas-1)
  - [`harnaas init`](#harnaas-init)
  - [`harnaas install`](#harnaas-install)
  - [`harnaas lint`](#harnaas-lint)
  - [`harnaas completion`](#harnaas-completion)
  - [`harnaas help`](#harnaas-help)
- [The manifest: `harnaas.json`](#the-manifest-harnaasjson)
- [The lockfile: `harnaas.lock.json`](#the-lockfile-harnaaslockjson)
- [Where assets land](#where-assets-land)
- [Exit codes](#exit-codes)
- [Environment variables](#environment-variables)
- [Running without a terminal](#running-without-a-terminal)
- [Signals](#signals)
- [Files harnaas writes outside your project](#files-harnaas-writes-outside-your-project)
- [Development](#development)
- [Further reading](#further-reading)

---

## Why

Teams accumulate AI-harness configuration — skills, rules, personas, always-on instructions — as
files that are copied between repositories by hand and then drift. Nobody can say which version of a
shared skill a given repository is on, or whether somebody edited it locally.

harnaas treats that configuration the way npm and Cargo treat code dependencies:

- **You declare** what you want in a committed, hand-edited manifest (`harnaas.json`), pinned to a
  tag or a commit.
- **harnaas places** it where each harness expects to find it, and records exactly what landed in a
  committed lockfile (`harnaas.lock.json`).
- **harnaas checks** that what is installed still matches what was declared, that nobody edited it
  outside the tool, and that nothing has moved on upstream.

Two rules shape everything else:

1. **harnaas never writes your manifest.** Apart from `harnaas init` creating it once, no command
   writes, reformats or normalizes `harnaas.json`. There is deliberately no `add`, `remove` or
   `update` command. The manifest is what your team reviews; a tool that rewrites it makes its diffs
   untrustworthy. Every remedy harnaas prints is phrased as the exact edit for a person to make.
2. **harnaas never touches a file it did not create.** Ownership lives in the lockfile. A destination
   recorded there is *managed*; a destination that exists but is not recorded is *unmanaged* and is
   never overwritten or deleted — on any flag, `--force` included.

## Status

harnaas was built in three tracked changes, all complete:

| Capability | Change | Status |
| --- | --- | --- |
| CLI foundation, manifest format, `harnaas init` | `add-harnaas-foundation` | **Complete** (74 / 74) |
| Source resolution, harness adapters, `harnaas install` | `add-harnaas-install` | **Complete** (122 / 122) |
| `harnaas lint`, update detection | `add-harnaas-lint` | **Complete** (74 / 74) |

All three commands work end to end. Everything documented below is implemented and tested; where a
capability is deliberately absent — a renderer the contract names but nobody has written, a harness
with no adapter — it is described as what it is rather than as a gap.

Two things v1 does not do, both by choice rather than omission: no forge other than GitHub, and no
named adapter other than `claude-code`. Both registries exist so that adding either is additive
rather than a reshaping, and most harnesses need no adapter at all — see
[Where assets land](#where-assets-land).

## Installation

### macOS and Linux, with Homebrew

```sh
brew tap harnaas/tap
brew install --cask harnaas
```

A cask rather than a formula, and it installs on Linux too — the generated cask carries `on_linux`
blocks pointing at the Linux archives. See the
[tap](https://github.com/harnaas/homebrew-tap) for why.

### Windows, with Scoop

```powershell
scoop bucket add harnaas https://github.com/harnaas/scoop-bucket
scoop install harnaas
```

### With Go

```sh
go install github.com/harnaas/harnaas/cmd/harnaas@latest
```

This puts `harnaas` in `$(go env GOPATH)/bin`. Make sure that directory is on your `PATH`.

Requires Go 1.26.5 or newer — keep it aligned with the `go` directive in [`go.mod`](go.mod).

> **Until the first release above `v0.8.42`, this route does not work.** This repository was seeded
> from the entire.io CLI carrying that project's tags, and they were pushed before the history was
> rewritten. The tags are gone from the remote but the Go module proxy is immutable, so `@latest`
> still resolves to `v0.8.42` — a version whose `go.mod` reads `module github.com/entireio/cli` and
> which contains no `cmd/harnaas`. Only a tag above it takes the module path back. Homebrew and
> Scoop are unaffected: they resolve release assets, not the module proxy.

### From source

The repository uses [mise](https://mise.jdx.dev/) as its single toolchain and task runner, which
pins the Go, golangci-lint, shellcheck and goreleaser versions for you:

```sh
git clone https://github.com/harnaas/harnaas.git
cd harnaas
mise install        # installs the pinned toolchain
mise run build      # produces ./harnaas (./harnaas.exe on Windows)
```

Or without mise:

```sh
go build -o harnaas ./cmd/harnaas
```

### Release binaries

Releases are built by [goreleaser](.goreleaser.yaml) for darwin, linux and windows on both amd64 and
arm64. Only a release build carries a stamped version; every other build self-reports what it can
recover from Go's embedded build information, so `harnaas --version` is always truthful about which
binary you are running.

### Verifying the install

```console
$ harnaas --version
harnaas 0.0.0-20260811105907-14edd937dd6a+dirty (14edd937dd6ac4ace33b6fa39b3f0a0f8ff88d34)
```

A release build reports its tag instead, e.g. `harnaas 1.2.0 (a1b2c3d)`.

## Quick start

```sh
cd your-project          # must be inside a git repository
harnaas init             # creates harnaas.json at the repository root
```

```console
$ harnaas init
Detected Claude Code in this project.
Created /home/you/your-project/harnaas.json

Next: declare the assets you want in harnaas.json, then run `harnaas install`.
`harnaas install` creates .harnaas/, writes into the harness directories and
maintains the ignore-file entries for what it installed. init wrote none of them.
```

The scaffolded manifest:

```json
{
  "version": 1,
  "harnesses": [
    "claude-code"
  ],
  "sources": {},
  "assets": []
}
```

`sources` and `assets` are written empty rather than omitted, so you can see both fields and the
shape their content goes in. Fill them in by hand — see
[The manifest](#the-manifest-harnaasjson) — then commit the file.

## Concepts

harnaas uses a deliberately narrow vocabulary. [CONTEXT.md](CONTEXT.md) is the canonical glossary;
this is the short version.

### The things being managed

| Term | Meaning |
| --- | --- |
| **Harness** | An AI coding tool that reads configuration from files in a project or a user home — Claude Code, Codex, Cursor, Gemini CLI. |
| **Asset** | A single unit of harness configuration harnaas installs: one skill, rule, instruction, command or persona. |

### The five asset types

| Type | Directory convention | What it is |
| --- | --- | --- |
| `skill` | `skills/` | Loaded by the harness on its own initiative when its description matches the task. A directory containing `SKILL.md`. |
| `rule` | `rules/` | Always-on guidance installed as its own file, which the harness discovers automatically. Untracked by version control. |
| `instruction` | `instructions/` | Always-on guidance concatenated into a managed block in the project's committed `AGENTS.md`. Survives a fresh clone with nobody having run install. **Project scope only.** |
| `command` | `commands/` | Invoked deliberately by the user typing a token for it. |
| `persona` | `agents/` | A delegated worker with its own model and tool budget, which the harness dispatches work to. |

`agents/` naming a persona is the one place harnaas reads a word it does not write — the directory
name belongs to the ecosystem, and renaming it would mean no existing asset repository could be
referenced without writing an object per asset.

### The two files

| File | Written by | Decoded |
| --- | --- | --- |
| `harnaas.json` — the **manifest** | You (and `harnaas init`, once) | **Strictly.** An unknown field is nearly always a typo, and surfacing it immediately is worth refusing to run. |
| `harnaas.lock.json` — the **lockfile** | harnaas | **Leniently.** A newer binary introduces fields an older one would otherwise reject, bricking the file for everyone still on the older version. |

Note that harnaas follows npm and Cargo here. The entire.io CLI whose architecture harnaas borrows
uses "manifest" for the lockfile's role; harnaas deliberately does not.

### Installing

| Term | Meaning |
| --- | --- |
| **Source** | Where an asset's content comes from: a path in a GitHub repository at some ref, or a path under the project's `.harnaas` directory. |
| **Scope** | Which root an asset installs beneath — `project` for the repository, `user` for the harness's per-user home. |
| **Managed** | A destination recorded in the lockfile. harnaas may update or remove it, because it put it there. |
| **Unmanaged** | A destination on disk that the lockfile does not record. harnaas never overwrites or deletes one, on any flag. |
| **Drift** | A managed destination whose content no longer matches what was installed — somebody edited it outside harnaas. |
| **Managed block** | A marker-delimited region harnaas owns inside a file your team also writes. Everything outside the markers is preserved byte for byte. |
| **Convergence** | Install bringing the installed set into agreement with the manifest, *including removing assets that are no longer declared*. |

---

# Command reference

## `harnaas`

The root command. Prints help and exits `0`.

```console
$ harnaas
harnaas manages a project's AI-harness assets as a declared, versioned
dependency: harnaas.json declares them, harnaas install places them, and
harnaas lint verifies them.

Usage:
  harnaas [flags]
  harnaas [command]

Setup Commands:
  init        Create harnaas.json for this project

Additional Commands:
  help        Help about any command

Flags:
  -h, --help      help for harnaas
  -v, --version   version for harnaas
```

| Flag | Effect |
| --- | --- |
| `-h`, `--help` | Print help for the root command or any subcommand. |
| `-v`, `--version` | Print `harnaas <version> (<commit>)` and exit `0`. |

**The root carries no persistent flags.** This is deliberate and load-bearing: a flag that applies to
some commands is registered locally on each command that honours it — `--json` included. A persistent
`--json` would be accepted and silently ignored by every side-effecting verb, and cobra cannot hide a
persistent flag from a subset of its children. Accepting a flag and honouring it must be the same
act.

No command declares `--json` yet. It is registered per command as each machine-readable view is
built, so passing it to a command that has none is a usage error rather than a silent no-op:

```console
$ harnaas init --json
Error: invalid usage: unknown flag: --json
```

**Where output goes.** User-facing results go to stdout; advisory, progress and warning text goes to
stderr. When a command eventually supports `--json`, the JSON document is the only thing on stdout.

**Diagnostics never reach your terminal.** They go to a log file — see
[Files harnaas writes outside your project](#files-harnaas-writes-outside-your-project).

---

## `harnaas init`

Create `harnaas.json` at the project root, declaring which harnesses this project targets.

```
harnaas init [flags]
```

### What it writes

**One file: `harnaas.json`, at the repository root. Nothing else.**

The harness directories, the `.harnaas` directory and any ignore-file entries belong to
`harnaas install`, which records what it created. Anything `init` created would be *unmanaged*, and
the next install would report a conflict against `init`'s own output. There is deliberately no flag
that makes `init` do any of it.

### Flags

| Flag | Default | Effect |
| --- | --- | --- |
| `--force` | off | Replace an existing `harnaas.json`. Without it, an existing manifest is refused. |
| `-y`, `--yes` | off | Accept the harness selection without prompting, so a user on a terminal can take the non-interactive path deliberately. |
| `--harness <id>` | *(detection)* | Target this harness. **Repeat the flag for each one.** Replaces detection entirely. |
| `-h`, `--help` | | Print help. |

`--harness` is a repeated flag rather than a comma-separated list, because a harness id is one token:
splitting on commas would turn `--harness "a, b"` into a name with a leading space and a diagnostic
about whitespace nobody typed deliberately.

Recognized harness ids today: **`claude-code`**.

### Harness detection

When you pass no `--harness`, init detects which harnesses the project already uses by checking for
observable evidence in the repository:

| Harness | Evidence (any one is enough) |
| --- | --- |
| `claude-code` | `.claude` or `CLAUDE.md` at the project root |

Detection only stats those paths — it reads nothing and creates nothing. That matters more than it
looks, because init runs before the project is under harnaas management at all: a detection pass that
created a directory to look inside would be the one file init is forbidden to write.

The check is an `lstat` rather than a `stat`, because the question is whether the harness left
something behind, not whether it resolves. A symlink named `.claude` pointing at a directory that is
not checked out is still evidence.

If nothing is detected, init falls back to `claude-code` and says so. A manifest with an empty
`harnesses` list would declare assets and guarantee them for nothing.

**`--harness` replaces detection entirely rather than merging with it.** Merging would make the
manifest depend on what happened to be in the working tree the moment init ran, and would leave no
way to scaffold a manifest that omits a harness the project already contains.

### Where the selection came from

init tells you on stderr, before asking you to confirm — because detection can be wrong in both
directions:

| Origin | Message |
| --- | --- |
| From `--harness` | *(nothing — you typed it a moment ago; repeating it back is noise)* |
| Detected | `Detected Claude Code in this project.` |
| Default | `No supported harness detected in this project; targeting Claude Code.` |

### The confirmation prompt

On an interactive terminal, init asks before writing:

```
Create harnaas.json targeting Claude Code? [Y/n]
```

**Not asking is the normal case, not a degraded one.** A run with no terminal, a run inside a coding
agent, a run in CI, and a run with `--yes` all proceed from the detected or flag-supplied values. See
[Running without a terminal](#running-without-a-terminal).

**Declining and cancelling are different acts:**

| You do | Exit | Result |
| --- | --- | --- |
| Answer `n` | `0` | Nothing written. `Nothing was created. Re-run with --harness to name the harnesses yourself.` |
| Press Ctrl-C | non-zero | Nothing written. The question was never answered. |

Reporting a user's own "no" as a failure would make init the one command whose success depends on
agreeing with it. Cancelling is not declining — a command that folds them together does the declined
thing to a user who asked for nothing at all.

### Order of operations

Everything that can refuse happens **before the prompt and before anything is written**. A question
whose answer cannot change the outcome spends the one moment the user is paying attention.

1. Resolve the project root.
2. Resolve the harness selection (`--harness` names are validated here).
3. Refuse an existing manifest, unless `--force`.
4. Explain where the selection came from.
5. Prompt, if prompting is possible and was not waived.
6. Write the manifest — staged, synced, and renamed into place, so a forced run either replaces the
   old manifest completely or leaves it intact.
7. Report what was created and what to do next.

### Examples

**Scaffold from detection:**

```console
$ harnaas init
Detected Claude Code in this project.
Created /home/you/your-project/harnaas.json

Next: declare the assets you want in harnaas.json, then run `harnaas install`.
`harnaas install` creates .harnaas/, writes into the harness directories and
maintains the ignore-file entries for what it installed. init wrote none of them.
```

**Non-interactive (CI, scripts, coding agents):**

```sh
harnaas init --yes
```

**Name the harnesses yourself:**

```sh
harnaas init --harness claude-code --yes
```

**An existing manifest is refused:**

```console
$ harnaas init
a manifest already exists at /home/you/your-project/harnaas.json

Edit it directly, or re-run with --force to replace it with a fresh one.
$ echo $?
1
```

**An unrecognized harness is refused before anything is written:**

```console
$ harnaas init --harness cursor
unknown harness "cursor"

Use a harness harnaas recognizes: claude-code.
$ echo $?
1
```

**Run from anywhere in the repository — the manifest always lands at the root:**

```console
$ cd src/api && harnaas init --yes
Created /home/you/your-project/harnaas.json
```

Every path harnaas touches resolves against the project root, never against your working directory.
The root is the nearest enclosing directory containing `.git`, which is the same answer git itself
gives, so a command run inside a submodule acts on the submodule.

**Outside a repository:**

```console
$ harnaas init
no project root found: /tmp/scratch is not inside a repository

Run harnaas from inside your project's repository, or create one there with `git init`.
$ echo $?
1
```

### Exit codes

| Code | Condition |
| --- | --- |
| `0` | Manifest created, **or** the prompt was declined. |
| `1` | Any refusal or runtime failure: existing manifest, unrecognized harness, no repository, cancelled prompt, I/O error. |

---

## `harnaas install`

Makes the filesystem match the manifest.

```console
$ harnaas install
created            house-style (claude-code) -> .claude/rules/house-style.md
created            review (claude-code) -> .agents/skills/review
created            tone (claude-code) -> AGENTS.md

3 created
```

A second run with nothing changed:

```console
$ harnaas install
unchanged          house-style (claude-code) -> .claude/rules/house-style.md
unchanged          review (claude-code) -> .agents/skills/review
unchanged          tone (claude-code) -> AGENTS.md

3 unchanged

Everything already matches harnaas.json.
```

The lockfile after that second run is **byte-identical** to the first. A recorded install time moves
only when the installation changed, not when install last ran — otherwise "nothing changed" could
never be shown by the committed file itself.

### Flags

| Flag | Effect |
| --- | --- |
| `--dry-run` | Compute and print the full plan, then exit without writing to any destination, the lockfile, the memory file or the ignore file. |
| `--force` | Restore **drifted managed** destinations, and only those. It does not, and will never, override unmanaged protection. |
| `--offline` | Resolve every source from the local cache and make no network request. A missing cache entry fails, naming what is missing, rather than falling back to the network. |
| `--no-cache` | Ignore the archive cache for this run. |
| `--json` | Emit the whole report as a single JSON document, the only thing on stdout. |

### The phases

Resolve every declared source → compute a plan → apply it → record the result. **No filesystem
change to any harness destination happens before the plan is complete**, so a failure during
resolution leaves your harness untouched.

### The seven outcomes

Every asset-and-target combination produces exactly one outcome, and every one is reported —
including the boring ones:

| Outcome | Meaning |
| --- | --- |
| `created` | Written for the first time. |
| `updated` | Content replaced with the newly resolved source. |
| `unchanged` | Already installed and already matching. Reported rather than omitted. |
| `emulated` | Installed through *another type's* surface because this harness has no surface of its own. Never reported as `created` or `updated`, and the report names the surface used and the behaviour that differs. |
| `conflict-unmanaged` | The destination exists and no lockfile entry claims it. Left byte-for-byte alone. |
| `conflict-drift` | A managed destination was edited since install. Left alone; the report names the flag that would overwrite it. |
| `unsupported` | This harness has no surface for this type, and emulating it would change its semantics. Nothing is written. |

Any outcome that blocked or altered an install carries a runnable remedy.

### Protection rules

- **Unmanaged destinations are never overwritten, replaced or deleted — on any flag.** `--force`
  applies only to drifted destinations harnaas itself installed. With no lockfile present, existing
  destinations are all unmanaged, which is protective rather than an error.
- **Drifted destinations are preserved by default** and reported, so "you edited this" is never
  silently undone.

### Convergence and uninstalling

Installing brings the managed set into agreement with the manifest, **removing managed destinations
whose asset is no longer declared** or no longer targets that harness. Removal applies only to
destinations recorded in the lockfile whose content still matches — a drifted orphan is left in place
and reported.

**To uninstall completely: empty the manifest's `assets` array and run install.** That removes every
managed destination and both managed blocks, and leaves the lockfile recording no installations.
There is no separate uninstall command.

### Guarantees

- **Idempotent.** A second run with nothing changed makes no filesystem change, reports every target
  `unchanged`, and exits `0`.
- **Deterministic.** Assets are processed in a stable order derived from their identifiers, not from
  manifest order. Reordering your manifest changes neither the output nor the lockfile, and repeated
  runs produce byte-identical lockfiles.
- **Atomic.** Each destination is staged outside its final location and moved into place; a directory
  is replaced as a unit, and staging artefacts are cleaned up on success and failure alike.
- **Partial failure is contained.** One failing asset does not abort the others. Every failure is
  reported, the command exits non-zero, and the lockfile records only what actually landed.
- **Concurrency-safe.** An exclusive lock covers the read-modify-write of the lockfile, and lock
  contention is reported rather than waited on indefinitely.

### The ignore-file block

Install maintains a marker-delimited managed block in your version-control ignore file listing
exactly the paths it installed — **one entry per installed path, never a directory-wide ignore**, so
a hand-written skill sitting beside an installed one stays tracked. Content outside the markers is
preserved byte for byte.

---

## `harnaas lint`

The read-only check that what is installed still agrees with the manifest and the lockfile, and that
nothing has moved on upstream. Lint examines the *installation* — never the content of an asset.

**Lint never repairs anything.** It reports what is wrong and how to fix it, leaving `harnaas
install` as the single repair path. It creates, modifies, moves and deletes nothing in your project,
your harness directories or your home. (Writing to the cache under your user cache directory is
permitted.)

```console
$ harnaas lint
Everything installed agrees with the manifest and the lockfile.
$ echo $?
0
```

```console
$ harnaas lint                          # after somebody edited an installed file
review
  error: SKILL.md was modified outside harnaas
    Run `harnaas install --force` to restore it, or keep the edit and stop declaring the asset.

1 error, 0 warnings
$ echo $?
2
```

Findings name the smallest thing that is wrong. An edited instruction names *the instruction*, not
the `AGENTS.md` that holds twelve of them; a changed file inside a skill names *the file*, not the
directory. That is what the per-file digests in the lockfile are for.

### Flags

| Flag | Effect |
| --- | --- |
| `--frozen` | Verify the lockfile still satisfies the manifest using those two files alone — no installed files read, no network request. The CI gate for "did somebody edit the manifest without reinstalling?" |
| `--offline` | Run every local check, including local-source change detection, and skip every network check. The report states that update detection was skipped. |
| `--strict` | Promote warnings to errors for the purpose of the exit status. |
| `--refresh` | Bypass the cached ref-resolution and tag-listing results. |
| `--json` | Emit the findings as a single JSON document, the only thing on stdout. |

### Findings

Every discrepancy is reported as a **finding** carrying the asset it concerns, a severity, the
problem, and a remedy naming the command or edit that resolves it. Findings are emitted in a stable
order derived from asset id and path, so two runs over identical state produce identical output.

What lint checks:

- **Manifest and lockfile consistency** — a manifest that will not load or validate (reported as a
  *single* finding that suppresses every dependent check, rather than cascading noise), an asset
  declared but never installed, a lockfile entry for an asset the manifest no longer declares.
- **Nothing installed at all** collapses to exactly one finding naming `harnaas install` — not one
  per declared asset.
- **Installed content integrity** — every installed file's digest re-computed and compared with the
  digest recorded at install time. Modified files, missing files, and extraneous files under a
  managed destination are each reported, naming the specific path. An absent destination collapses to
  one finding.
- **Unmanaged conflicts** — a declared asset whose destination exists but is claimed by no lockfile
  entry, because install will refuse to overwrite it.
- **Managed block drift** in `AGENTS.md` and in the ignore file — missing, duplicated or malformed
  markers, and content that no longer matches what the recorded installations imply. Content
  *outside* the markers is ignored entirely and never reported as drift.
- **The bridge line** — that `CLAUDE.md` contains exactly one `@AGENTS.md` line when instruction
  assets are installed, since without it Claude Code never reads the managed block.
- **Updates** — see below.

### Update detection

| Condition | Reported as |
| --- | --- |
| A branch has advanced past the recorded commit | Available update, naming the recorded commit, the current commit and the branch |
| The recorded ref no longer exists upstream | A distinct finding — no newer content is on offer and a reinstall would fail outright |
| A newer stable tag exists | Available update, naming only the *highest* newer stable tag |
| A local source under `.harnaas` was edited since install | Available update *(runs offline)* |
| A local source recorded in the lockfile no longer exists | A distinct missing-source finding |
| The source ref is a full commit identifier | **Never checked.** You pinned it deliberately; no lookup is performed on its behalf. |

Pre-release tags are never offered as an update to an asset installed from a stable tag, and tags
that are not semantic versions are ignored rather than producing spurious findings.

**A failure to reach a remote never fails the run or masks local findings.** Affected assets are
marked *unchecked*, the reason is stated, and repeated failures against one host are summarized once
for that host rather than once per asset. The summary states how many assets went unchecked, so a
clean result is never mistaken for a complete one.

### The severity rule: pinned and current

**An available update is an error, not a warning.** So is tracking a mutable ref such as a branch.
The only state that passes is one where every source is pinned to a tag or commit *and* is current.
No flag downgrades an available update.

This is unusual, and it is deliberate — see
[ADR 0004](docs/adr/0004-available-updates-are-lint-errors.md). A warning that CI tolerates
indefinitely does not make a team's configuration uniform and current. Branch-tracking assets are
reported as *"not pinned, not reproducible"* rather than as *"outdated"*, precisely so there is a
stable passing state: a branch moves whenever upstream commits, so reporting a moved branch as
outdated would leave CI permanently red with no achievable fix.

Warning severity is reserved for advisory findings that leave the installation reproducible and
current.

### Remedies show the exact edit

A finding that needs a manifest change prints the manifest, the line declaring the source, the exact
current string and the exact replacement string — verbatim, so applying the edit is a literal
substitution — followed by the command to run afterwards. A finding that needs no manifest edit
prints the command alone.

Because the manifest is hand-edited only, **lint never offers to apply the edit itself.**

### Exit codes

| Code | Condition |
| --- | --- |
| `0` | No error-severity finding. Warnings alone still exit `0` (unless `--strict`). |
| `2` | Lint ran its checks to completion and found at least one error-severity finding. |
| `1` | Lint itself failed to run — an unreadable lockfile, for instance. |

**Exit `2` is reserved.** It exists so a CI job can tell "lint crashed" from "lint worked and your
harness has drifted" — the distinction lint exists to make. No command other than `lint` may return
it, and a lint run that fails partway exits `1` like any other runtime failure. The exit status is
identical in every output mode.

---

## `harnaas completion`

Generate a shell completion script. The command is functional but hidden from help.

```sh
harnaas completion bash
harnaas completion zsh
harnaas completion fish
harnaas completion powershell
```

Each script is written to stdout; redirect or source it the way your shell expects.

## `harnaas help`

```sh
harnaas help          # same as harnaas --help
harnaas help init     # same as harnaas init --help
```

---

# The manifest: `harnaas.json`

The committed, hand-edited declaration of which assets your project wants and where they come from.
It lives at the repository root and nowhere else — there is no `--manifest` flag, because a manifest
somewhere else would mean two developers in the same repository could install different asset sets
and both be right.

A `harnaas.json` found *below* the root is an error naming that file, not a second declaration to
merge. The search skips dot-directories and dependency trees (`node_modules`, `vendor`): a manifest
inside a vendored library is that library's, and its author is not the person harnaas would be
talking to.

## A complete example

```json
{
  "version": 1,
  "harnesses": ["claude-code"],
  "sources": {
    "acme": "github:acme/ai-assets@v1.4.0",
    "house": "local:.harnaas/house"
  },
  "assets": [
    "acme:skills/code-review",
    "acme:instructions/tone.md",
    "house:rules/house-style.md",
    ".harnaas/rules/no-todo-comments.md",
    {
      "source": "acme:content/reviewer.md",
      "type": "persona",
      "id": "reviewer",
      "targets": ["claude-code"],
      "scope": "project"
    },
    {
      "source": "acme:commands/ship",
      "scope": "user"
    }
  ]
}
```

## Top-level fields

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `version` | number | yes | Manifest schema version. Currently `1`. |
| `harnesses` | string[] | no | The harnesses assets target by default. |
| `sources` | object | no | Short key → source string. |
| `assets` | array | no | The declared assets, as strings or objects. |

### `version`

The version is read **on its own, leniently, before the strict pass**. A manifest written by a newer
harnaas carries fields this binary does not know, and strictness would report the first of them — an
arbitrary one — as a misspelling, sending you hunting for a typo in a correct file. Read first, the
same manifest produces the message that actually helps: upgrade harnaas.

### `harnesses`

Names the harnesses your assets target by default. An asset that declares its own `targets` overrides
this list for itself.

The list states a **guarantee**, not exclusivity: it means "we guarantee the declared assets work for
these". Because skills and instructions land in shared locations that several harnesses read, a
harness absent from the list will still see assets installed for one that is present. That is why an
unrecognized id is an error — it is a guarantee harnaas cannot make — and why an asset being visible
to a harness nobody listed is not a bug.

A misspelling here is reported **once**, against the list, not once per asset that inherited it.

Recognized ids: `claude-code`.

### `sources`

Maps a short key to a source string, so a repository and a ref appear exactly once each.

```json
"sources": {
  "acme": "github:acme/ai-assets@v1.4.0",
  "house": "local:.harnaas/house"
}
```

**Source grammar:**

| Kind | Form | Rules |
| --- | --- | --- |
| `github` | `github:<owner>/<repo>@<ref>` | The `@<ref>` is **mandatory**. |
| `local` | `local:.harnaas/<dir>` | Must name a directory under `.harnaas`. Pins nothing, so no `@ref` is allowed. |

**A GitHub source with no ref is rejected rather than defaulted to a branch.** The manifest exists to
say which version of somebody else's content this repository trusts, and a default would make two
installs of one manifest produce different files.

**Keys** may contain letters, digits, `-`, `_` and `.` — anything else could not be referenced from
an asset entry, because the asset grammar splits on the first colon and a key containing a slash
would be indistinguishable from a path.

Nothing here is resolved at parse time: `ParseSource` recognizes the kind and checks the shape.
Nothing fetches, stats or resolves, so a manifest can be validated with no network and no filesystem
— which is what lets `lint` report a bad source offline.

### `assets`

Each entry is either a **source string** or an **object**.

#### The string form

```json
"assets": [
  "acme:skills/code-review",
  ".harnaas/rules/house-style.md"
]
```

Two accepted forms, distinguished by prefix:

| Form | Example | Meaning |
| --- | --- | --- |
| Keyed | `acme:skills/code-review` | `<sourceKey>:<path>` — the path within the source `acme` declares. |
| Project-local | `.harnaas/rules/house-style.md` | A path under the project's own `.harnaas` directory. |

Your own content can be referenced either way: through a `local:` source key when several assets
share a subdirectory (`house` above resolves `house:rules/house-style.md` to
`.harnaas/house/rules/house-style.md`), or by writing the `.harnaas/…` path directly for a one-off.

One directory rather than any path you like is what makes a local asset auditable: everything harnaas
may read out of the repository is in one place, and a path that leaves it is a mistake harnaas can
name without knowing anything about your layout.

**Refused outright:**

| Input | Why |
| --- | --- |
| `/etc/skills/x` or `C:/skills/x` | Absolute paths. harnaas does not install from anywhere on the machine. Both spellings are refused on every platform. |
| `acme:skills\review` | Backslashes. A backslash is an ordinary filename character on Linux and a separator on Windows, so a manifest containing one would name two different files depending on who ran install. |
| `acme:../elsewhere` | Leaves the root of the source. |
| `.harnaas/../secrets` | Leaves `.harnaas`. Checked **textually, before anything is opened** — a path that escapes must never be read, not even to discover whether it exists. |
| `""` | No source. |

#### The object form

```json
{
  "source": "acme:content/reviewer.md",
  "type": "persona",
  "id": "reviewer",
  "targets": ["claude-code"],
  "scope": "project"
}
```

| Field | Required | Default |
| --- | --- | --- |
| `source` | yes | — |
| `type` | no | Inferred from the containing directory |
| `id` | no | Inferred from the path's leaf, extension stripped |
| `targets` | no | The manifest's `harnesses` |
| `scope` | no | `project` |

Use the object form when the source's layout does not follow the directory convention, when two
sources share a leaf name, or when one asset needs different targets or scope from the rest.

## Type and id inference

`type` and `id` are inferred **separately**, because the object form suppresses inference one field
at a time. An entry declaring `type` for an unconventional layout still wants its id inferred, and
must not be refused for a directory name nobody is relying on.

**Type** comes from the containing directory:

| Directory | Type |
| --- | --- |
| `skills/` | `skill` |
| `rules/` | `rule` |
| `instructions/` | `instruction` |
| `commands/` | `command` |
| `agents/` | `persona` |

**Id** is the path's leaf with any extension stripped, so `tone.md` and `tone` name the same asset.

A declared `type` **suppresses inference entirely rather than being checked against it.** The object
form exists precisely for a source whose layout does not follow the convention; comparing the two
would refuse the case it was added for.

## `targets`

An asset's targets are its own `targets` where it declares them, and the manifest's `harnesses`
otherwise.

- A **nil** (absent) `targets` inherits. If `harnesses` is also empty, that is a violation — the
  asset has nowhere to install.
- An **empty** `targets` (`[]`) is a violation of its own: the author declared an empty list, so the
  asset could never be installed anywhere. The two are told apart because the edit that fixes them
  differs.
- Each position in an entry's own `targets` is checked separately, because two bad names there are
  two independent edits.

## `scope`

| Scope | Where it installs |
| --- | --- |
| `project` *(default)* | Beneath the project root of each target harness. |
| `user` | In the target harness's per-user location. |

Scope is declared **per asset rather than chosen per run**, because it is a property of the content —
a team's house-style rule belongs to the repository, a personal shortcut belongs to the person. A
flag would make one manifest install to two different places depending on who ran the command.

`user` is accepted only where every target harness has an unambiguous per-user location. Declaring it
elsewhere is a violation — **never a silent fall back to `project`**, which would install the asset
somewhere the author did not ask for and would not notice.

**An `instruction` is `project` scope only, definitionally.** What distinguishes an instruction from
a rule is surviving a fresh clone inside a committed file; at user scope there is neither a clone nor
a committed file, so the type would mean nothing. This is not a limitation of the installer.

## What harnaas checks about the content it resolved

Once a source has been resolved — from GitHub or from `.harnaas`, checked identically either way —
harnaas verifies the shape of what came back before anything is installed:

| Type | Required shape |
| --- | --- |
| `skill` | A **directory** containing `SKILL.md`. |
| everything else | Exactly **one regular file**. |

**A skill's frontmatter `name` must match its id.** A harness that reads a skill's frontmatter uses
that name to decide the skill is there, so a skill installed as `review` whose frontmatter says
`code-review` installs cleanly, reports success and is never invoked — the one outcome a tool whose
purpose is telling a team what is in effect must not produce. harnaas refuses it rather than
correcting it: frontmatter is decoded *out of* the file and there is no encoder at all, so no later
phase can rewrite your frontmatter by accident. That matters most for a `rule`, where a YAML writer's
choices about quoting and folding a `paths:` list can change what the rule applies to.

Absent, unparseable and present-without-a-name are one diagnostic with three reasons rather than
three errors, because your next action is the same in all three: open the file and look at the top of
it.

## How validation reports problems

**Decoding stops at the first failure; interpretation reports every violation.** A document that will
not parse has no second problem to find, so decoding returns one error and no document at all — half
a manifest is not a smaller manifest, it is a different one. Every asset entry is independent, so
interpretation accumulates every violation into one aggregate, ordered identically on every run
(document-level problems first, then by asset index and field).

Every diagnostic is shaped `{problem, fix}`: what is wrong, and the exact edit that resolves it.

**A question whose answer would have to be invented is not asked.** Where an entry's source string
did not parse, there is no path to infer a type or an id from, so inference is skipped unless the
entry declared the field itself. Everything independent of the path is still checked, because the
point of accumulating is one run per file. What harnaas does not do is report a second problem you
never wrote and send you looking for it.

**Uniqueness is asked last**, over the entries that had nothing else wrong. It is **per type** rather
than per manifest — a skill and a command may both be `review`, because each type is its own
namespace to the harness — and a collision is attributed to the later entry naming the earlier one,
so it is one violation rather than two.

---

# The lockfile: `harnaas.lock.json`

The machine-written record of what was actually installed and from where. **Commit it alongside the
manifest.** The manifest configures intent; the lockfile records facts, and carries no configuration
at all.

**The lockfile is what establishes ownership.** A destination recorded there is managed; a
destination absent from it is never overwritten or deleted, on any flag. Deleting the lockfile does
not orphan installed files — it makes them *unmanaged*, which is protective. See
[ADR 0001](docs/adr/0001-ownership-lives-in-the-lockfile.md) for why ownership is not a marker
embedded in each installed file: harnaas *copies* content verbatim, and injecting a marker would mean
the installed bytes never equal the source bytes, defeating the very digest comparison that
distinguishes "you edited this" from "upstream changed this".

For each asset it records the id and type, the normalized source, **the ref that was requested and
the commit that ref resolved to** — kept separately, because "the installed files still match the
commit" and "the tag now points somewhere else" are two questions lint asks separately — the source
digest, and when the install completed.

Each asset carries one installation record per target, with the harness, the scope, the destination,
a digest per installed file, and a digest over the installation as a whole. Per-file digests are what
let a later check name *which* file changed rather than only that something did.

**Two independent digests per installation.** The *source* digest covers what the content was
produced from; the *installed* digest covers the bytes that landed. A changed source digest means new
content is available upstream; a changed installed digest means the destination was edited. Because
an installation is rendered rather than necessarily copied, the two differing is not on its own
drift.

---

# Where assets land

## Shared targets first

A survey of 23 AI coding harnesses found that 17 read `.agents/skills/<name>/SKILL.md` and 21 read
`AGENTS.md`. harnaas therefore writes to the shared location first and falls back to a harness's own
directory only where that harness does not read the shared one — see
[ADR 0002](docs/adr/0002-shared-agents-target-before-per-harness.md).

| Type | Shared destination |
| --- | --- |
| `skill` | `.agents/skills/<id>/SKILL.md` |
| `instruction` | A managed block in the project's `AGENTS.md` |

One write serves many harnesses; a harness that reads the shared target never receives a duplicate
copy. Shared targets need no named adapter, so `skill` and `instruction` assets install even for a
harness harnaas has no adapter for.

## Per-harness targets

`rule`, `command` and `persona` have no shared equivalent anywhere, so they install only through a
named adapter. **Version 1 ships exactly one: `claude-code`.**

| Type | Destination (relative to the scope root) |
| --- | --- |
| `rule` | `.claude/rules/<id>.md` |
| `command` | `.claude/commands/<id>.md` |
| `persona` | `.claude/agents/<id>.md` |

The scope root is the project root at `project` scope and your home directory at `user` scope; the
relative path is the same for both. The asset id is always the stem, which is what makes a
destination predictable from the manifest alone.

`skill` and `instruction` are deliberately **absent** from that table rather than also mapped into
`.claude`. An adapter answering for them would be a second place harnaas decides where a skill lands,
and the two answers would be one release from disagreeing.

Targeting a harness that has no named adapter with a `rule`, `command` or `persona` is reported
`unsupported` rather than resolved to a guessed path — once written, a guessed destination is
indistinguishable from a real one.

A rule installs as a standalone file the harness discovers on its own. Nothing references it from
`CLAUDE.md`, `AGENTS.md` or a managed block.

## The instruction block

Instruction content goes into a marker-delimited block in `AGENTS.md`
([ADR 0003](docs/adr/0003-instruction-content-in-an-agents-md-block.md)):

```markdown
<!-- harnaas:begin instructions -->
...instruction content, ordered by asset id...
<!-- harnaas:end instructions -->
```

The markers are HTML comments because the memory file is Markdown, where they render as nothing. They
are matched as **whole lines**, so a marker quoted inside a fenced code block or indented into
somebody's list is not mistaken for the real one. Everything outside them is preserved byte for byte,
and harnaas refuses to act on a file whose markers do not pair up rather than guessing where its
region was meant to be.

Claude Code does not read `AGENTS.md`, so a single `@AGENTS.md` bridge line is added to `CLAUDE.md`.
`lint` checks that exactly one exists whenever instruction assets are installed.

---

# Exit codes

| Code | Meaning |
| --- | --- |
| `0` | The command completed and found nothing to report. |
| `1` | Any runtime failure: bad configuration, I/O, network, a refusal. |
| `2` | **Reserved.** `lint` ran its checks to completion and reported at least one error-severity finding. |
| `128`+*n* | Terminated by signal *n* — `130` for SIGINT, `143` for SIGTERM. See [Signals](#signals). |

Exit `2` is reserved for that single meaning so a CI job can tell "lint crashed" from "lint worked
and your harness has drifted". No command other than `lint` may return it, and a lint run that fails
partway exits `1` like any other runtime failure. Nothing in the binary emits `2` today, and the
end-to-end suite asserts "not `2`" separately from the status each case expects, so the reservation
survives somebody updating a case.

**The entrypoint is the only component that prints an error or picks an exit code.** Cobra's error
and usage printing is silenced globally; commands return errors, and what those errors look like on
your terminal is decided in one place.

---

# Environment variables

| Variable | Read by | Effect |
| --- | --- | --- |
| `HARNAAS_LOG_FILE` | Logging | Override the log file path outright. |
| `HARNAAS_LOG_LEVEL` | Logging | `debug`, `info`, `warn` or `error`. Unset or unrecognized means `info`. |
| `HARNAAS_CACHE_DIR` | Archive and resolution caches | Override harnaas's cache root. Names the root outright, so it *replaces* the default rather than nesting under it, and it moves both caches together. |
| `HARNAAS_GITHUB_TOKEN` | GitHub sources | Token for archive downloads. **First** in the chain. |
| `GH_TOKEN` | GitHub sources | Second in the chain. Honoured because `gh` already sets it. |
| `GITHUB_TOKEN` | GitHub sources | Third in the chain. Honoured because an Actions job is handed one. |
| `NO_COLOR` | Styling | Set to anything non-empty and harnaas emits no escape sequences at all ([no-color.org](https://no-color.org)). |
| `CI` | Prompting | Any value but `false` disables prompting. |
| `HARNAAS_TEST_TTY` | Prompting | Force the prompting decision. `1` forces on; any other non-empty value forces off. For tests. |
| `HARNAAS_E2E_BIN` | Test suite | Run the end-to-end suite against this binary instead of building one. |

## About the GitHub token

The chain is `HARNAAS_GITHUB_TOKEN` → `GH_TOKEN` → `GITHUB_TOKEN`, read once per run, proceeding
unauthenticated when none is set. The two ambient names are honoured because harnaas is rarely the
only tool on the machine that needs a token, so a CI job that already authenticates needs no
harnaas-specific configuration. The harnaas-specific name comes first so a project that needs
something the ambient token cannot read has somewhere to say so.

**A variable set to nothing counts as unset**, because a job that exports a token conditionally
leaves an empty value behind and an empty bearer header fails a request that would have succeeded
without one.

**A token is named by where it came from, never by what it is.** The value travels in a type whose
every rendering prints the *origin variable*, so a diagnostic can be actionable without quoting a
secret. The token is dropped on any redirect that leaves the host **and port** harnaas asked, and
every URL harnaas prints, logs or writes has its userinfo *and query string* stripped — an archive
download redirects to a signed URL whose `token=` grants the bearer the access the request had.

Note that ref resolution does **not** use the token: a `github` ref becomes a commit through
`git ls-remote`, which goes through whatever credential helper you already configured. A private
repository resolves without harnaas inventing an authentication story, and there is no API rate limit
for a CI job to exhaust.

---

# Running without a terminal

**Every workflow is completable without a terminal, and no information is reachable only through a
prompt, picker or full-screen interface.** The primary consumers are CI jobs and coding agents,
neither of which has a terminal.

Whether harnaas may prompt is answered from the command's own streams plus the environment — never by
probing a controlling terminal. The gates, first match wins:

1. `HARNAAS_TEST_TTY`, when set, is the whole answer.
2. Running under `go test` → no.
3. A coding-agent sentinel in the environment (`CLAUDECODE`, `CURSOR_AGENT`, `GEMINI_CLI`,
   `COPILOT_CLI`, `PI_CODING_AGENT`, …) → no. An agent runs harnaas as a subprocess and often hands
   it the agent's own terminal, which no human is watching.
4. `CI` set to anything but `false` → no. Self-hosted runners do attach terminals, so the stream test
   alone would let a prompt through and hang the job.
5. Otherwise: **both** streams must be terminals.

The decision is biased towards "no", because a flag-driven path always exists while a prompt shown to
something that cannot answer does not degrade — it hangs. Note that `harnaas init > out.txt` has a
terminal attached to the process and still must not prompt, which is why the test asks about the
command's own streams.

## In CI

On every pull request, check that the lockfile still satisfies the manifest:

```yaml
- run: harnaas lint --frozen        # exits 2 if it does not
```

`--frozen` reads no installed file and makes no request, so it works in a fresh checkout where
nothing has been installed — which is the state a CI job is in. It answers the question a reviewer
needs: *did somebody edit the manifest and not reinstall?*

Check **currency** on a schedule instead, not per pull request:

```yaml
- run: harnaas lint --refresh       # exits 2 if any asset is behind upstream
```

The split matters. An available update is an *error*, not a warning
([ADR 0004](docs/adr/0004-available-updates-are-lint-errors.md)), so running that check on every
pull request would fail a change that touched nothing about the assets because somebody published a
tag upstream that morning. That is how a forcing function becomes an obstacle people learn to
bypass. On a schedule the same rule is a standing job somebody picks up.

harnaas does this to itself — see [`ci.yml`](.github/workflows/ci.yml) and
[`currency.yml`](.github/workflows/currency.yml).

To install in CI without reaching the network, commit the lockfile and use the cache:

```yaml
- run: harnaas install --offline
```

---

# Signals

The first interrupt cancels the root context and prints a force-quit notice; a second terminates
immediately.

```
Interrupting… press Ctrl-C again to force quit.
```

On termination harnaas **re-raises the original signal to itself** rather than calling a plain exit,
falling back to `128`+signum only where re-raising is unsupported (Windows has no signal-to-self).

This matters more than it sounds: a shell aborts a `while true; do harnaas …; done` loop only when
the child is *killed by* the signal. A plain exit with status 130 is an ordinary exit, so the loop
keeps respawning harnaas and your Ctrl-C never escapes it.

A Ctrl-C typed **at a prompt** is an interrupt that never became a signal — the form puts the
terminal in raw mode, which disables the line discipline's signal characters, so it arrives as a
keystroke the form consumes. harnaas treats it as an interrupt anyway and terminates as though the
signal had been delivered, because the alternative is the exact outcome the re-raise exists to
prevent, reached by a different route.

---

# Files harnaas writes outside your project

**No command leaves a log behind in your working tree.**

| What | Where | Override |
| --- | --- | --- |
| Log file | `<user cache dir>/harnaas/logs/harnaas.log` | `HARNAAS_LOG_FILE` |
| Archive cache | `<user cache dir>/harnaas/archives/` | `HARNAAS_CACHE_DIR` |
| Resolution cache | `<user cache dir>/harnaas/resolutions/` | `HARNAAS_CACHE_DIR` |

On Windows the user cache directory is `%LOCALAPPDATA%`, so the log is at
`%LOCALAPPDATA%\harnaas\logs\harnaas.log`. On Linux it is `~/.cache`; on macOS, `~/Library/Caches`.

Logs are newline-delimited JSON:

```json
{"time":"2026-08-11T08:09:08.26Z","level":"INFO","msg":"harnaas started","version":"1.2.0"}
{"time":"2026-08-11T08:09:08.28Z","level":"ERROR","msg":"harnaas failed","exit_code":1,"error_type":"*pflag.NotExistError"}
```

Diagnostics go to that file and **never to a terminal — and never to a stream as a fallback either**.
Where the file cannot be opened, records are discarded, because a fallback that turns a disk problem
into a corrupted `--json` document is worse than no logging.

**What may be logged:** identifiers, paths, durations, counts and outcomes. **What may not:** file
contents, prompt or memory text, captured output and credentials. The files harnaas handles are a
team's instructions and rules — exactly the content nobody expects to find copied into a log they did
not know existed. Note above that even the failure is recorded by error *type*, never by message,
because errors from later phases quote the manifest that produced them.

## The archive cache

An archive is fetched at most once per run by the resolver's own memo, and at most once per *machine*
by the cache. It is content-addressed in the literal sense: an archive is filed under its own digest,
and a pointer records which digest belongs to a given `(kind, repository, commit)`. Verifying an entry
is therefore checking the bytes against the name they are filed under, which cannot drift out of step
the way a digest recorded beside them could.

**Nothing in the cache can fail a run.** A miss, an unreadable entry, a pointer to nothing and content
that no longer hashes to its own name are one answer — fetch it — and a damaged entry is removed on
the way out. A cache write that fails is a log record, never an error: the cache exists to make a run
cheaper, and one that can make a run fail has cost more than it saves.

The credential is deliberately **not** part of the cache key: the archive of a commit is the same
bytes whoever fetched it, so a token is an access decision and not a content one. The consequence is
that an entry is readable by whoever can read the directory holding it, which is why the default
location is your own cache directory with owner-only permissions.

## The resolution cache

`lint` records what a ref resolution and a tag listing answered, so a second run inside a bounded
freshness window makes no request. `--refresh` reaches past it.

Nothing in it can fail a run. A miss, an unreadable entry, one that will not parse and one past its
window are a single answer — ask again — and a damaged entry is removed on the way out so the next
run does not pay to rediscover it. A write that fails is a log record, and a machine with no cache
directory gets a slower `lint` rather than a failed one.

Only answers are recorded, never failures. Caching a failure would make an outage outlive itself,
which is the opposite of what the per-host summary does: that collapses one outage *within* a run.

To clear either cache, delete its directory. Neither holds anything harnaas cannot fetch again.

---

# Development

## Tasks

mise is the single toolchain and task entry point.

| Command | What it does |
| --- | --- |
| `mise run check` | fmt, then lint, then test. **The one to run before committing.** |
| `mise run fmt` | `gofmt -s -w .` |
| `mise run lint` | golangci-lint, gofmt check, mise config, `go mod tidy` check, shellcheck, `goreleaser check` |
| `mise run test` | The unit suite through gotestsum |
| `mise run test:e2e` | The end-to-end suite against a freshly built binary |
| `mise run build` | Build `./harnaas` |
| `mise run lint:licenses` | Check dependency licenses against [`.allowed-licenses`](.allowed-licenses) |
| `mise run lint:goreleaser` | Validate [`.goreleaser.yaml`](.goreleaser.yaml), deprecated properties included |
| `mise run release` | Build and publish the release for the current tag; the release workflow runs this |

`check` is deliberately sequential rather than a `depends` list: linting a tree that `fmt` has not
rewritten yet reports formatting noise, and a failed lint should stop before the slower test run.

## Layout

```
cmd/harnaas/          the process entrypoint, and nothing else
cmd/harnaas/cli/      the bulk of the code, in one flat package
  adapter/            the harness adapter contract, registry, and one adapter per package
  harness/            the harness roster — data only
  manifest/           decoding and interpreting harnaas.json
  source/             the only place harnaas reaches the network or reads repository content
  …
internal/             genuinely cross-binary code only
e2e/                  the process contract, behind the `e2e` build tag
docs/adr/             the load-bearing product decisions
openspec/changes/     the tracked changes and their specs
```

**A subpackage is extracted only to break a Go import cycle.** That is the whole extraction trigger.
There is no `domain` / `usecase` / `adapter` layering, and adding one is not an improvement.

Command files are named `<noun>_group.go` for a group root and `<noun>_<verb>.go` for a leaf command.

## Stack

cobra + pflag for the command tree, the Charm v2 stack on the `charm.land` module domain for
interactive surfaces, testify with gotestsum for tests.

**No configuration library.** Configuration is `encoding/json` plus cobra flags plus environment
variables, with precedence documented at each read site.

Keep the Go version in [`mise.toml`](mise.toml) and the `go` directive in [`go.mod`](go.mod)
identical.

## Static analysis encodes the rules

golangci-lint runs the standard set plus the extended list in [`.golangci.yaml`](.golangci.yaml).
`forbidigo` turns "go through the abstraction" into a build failure whose message names the
replacement — reading the process working directory is banned in favour of `paths.ProjectRoot(ctx)`,
and cobra's `Print*` helpers are banned because they write to stderr. `nolintlint` requires every
suppression to name a specific linter and give a reason.

Where a rule must survive a suppression, it is **also** asserted by an AST test over non-test
sources. The two rules that fail *quietly* — reading the working directory, and printing through a
`Print*` helper — are checked over the whole module's syntax, because a plausible-looking `//nolint`
reason passes review more easily than it should.

## Tests

- **No test reads or writes real user state.** `internal/testenv` gives a package its own home, cache
  and config directories, and a package whose files ask the standard library where those are must
  install it — a rule an AST test over the module enforces rather than a convention. The failure it
  prevents is silent: a test that appended to your real log, or read a harness configuration that
  exists only on your machine, is green locally and green in CI for different reasons.
- **The command surface is declared, not derived.** A test that asked the command tree what it
  contains would agree with any tree, so the full set of commands — and whether each has a `--json`
  view — is written out in the test and compared whole. Adding a command is two lines, and the second
  is where somebody decides whether the new verb is readable by a CI job.
- **The process contract is tested as a process.** `e2e/`, behind the `e2e` build tag, builds the
  binary and runs it. It asserts the part of the contract that only exists once there is a process:
  the status a shell reads, and whether an interrupt *killed* harnaas or harnaas exited with a number
  that merely looks like it. Neither is reachable from inside the test binary.

The signal test skips on Windows, where there is no signal-to-self for the entrypoint to re-raise
with; the unit suite covers the `128`+signum fallback there.

## Line endings

The repository is LF everywhere, pinned by [`.gitattributes`](.gitattributes). Without it, a Windows
clone under `core.autocrlf=true` gets CRLF and the project's own checks fail for reasons unrelated to
the code: shellcheck rejects every `mise-tasks` script for a literal carriage return (SC1017), and
`mise run lint:gomod` reports `go.mod` as modified when `go mod tidy` changed nothing.

If you cloned before that file existed, renormalize your working tree once:

```sh
git add --renormalize .
```

---

# Further reading

| Document | What it is |
| --- | --- |
| [CONTEXT.md](CONTEXT.md) | The canonical vocabulary. Use these words and avoid the listed alternatives — in code, in output and in commit messages. |
| [CLAUDE.md](CLAUDE.md) | The architecture in full, and the reason behind each rule. |
| [docs/adr/](docs/adr/) | The load-bearing product decisions. Cite an ADR rather than re-arguing it. |
| [openspec/changes/](openspec/changes/) | The tracked changes, their specs and their task lists. |

## Deliberately not here

Telemetry, authentication, self-update and a plugin system. The source CLI this architecture is
imported from has all four; none pays off across three commands, and each adds a dependency and a
privacy surface.
