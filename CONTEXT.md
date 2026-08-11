# harnaas

A CLI that manages a project's AI-harness configuration as a declared, versioned dependency. A team
states which assets it wants in a manifest; `harnaas install` fetches them and places them where each
harness expects; `harnaas lint` verifies that what is installed still matches what was declared.

## Language

### The things being managed

**Harness**:
An AI coding tool that reads its configuration from files in a project or a user home — Claude Code,
Codex, Cursor, Gemini CLI.
_Avoid_: agent, editor, client, host, IDE

**Asset**:
A single unit of harness configuration that harnaas installs — one skill, rule, instruction, command
or persona.
_Avoid_: artifact, resource, plugin, extension, capability

### Asset types

**Skill**:
An asset the harness loads on its own initiative when its description matches the task at hand.
_Avoid_: capability

**Rule**:
Always-on guidance installed as its own file, which the harness discovers and loads automatically.
Untracked by version control.

**Instruction**:
Always-on guidance that harnaas concatenates into a managed block in the project's committed memory
file. Distinct from a rule in that it survives a fresh clone without anyone having run install, and
cannot be scoped to particular paths. Project scope only — at user scope there is no clone, so the
distinction from a rule disappears.

**Command**:
An asset the user invokes deliberately by typing a token for it.

**Persona**:
A delegated worker with its own model and tool budget, which the harness dispatches work to.
_Avoid_: agent, subagent

### The two files

**Manifest**:
`harnaas.json` — the committed, hand-edited declaration of which assets a project wants and where
they come from.
_Avoid_: config, declaration, settings

**Lockfile**:
`harnaas.lock.json` — the machine-written record of what was actually installed and from which
commit. Committed alongside the manifest.
_Avoid_: manifest — note that the entire.io codebase harnaas borrows its architecture from uses
"manifest" for precisely this file and "metadata" for the declaration. harnaas follows npm and Cargo
instead. Do not "correct" this.

### Installing

**Source**:
Where an asset's content comes from — a path within a GitHub repository at some ref, or a path under
the project's `.harnaas` directory.

**Scope**:
Which root an asset installs beneath — `project` for the repository's own harness directory, `user`
for the harness's per-user home.

**Managed**:
Describes a destination recorded in the lockfile. harnaas may update or remove a managed destination,
because it put it there.

**Unmanaged**:
Describes a destination that exists on disk but is not recorded in the lockfile. harnaas never
overwrites or deletes one, on any flag.

**Drift**:
The condition of a managed destination whose content no longer matches what was installed — somebody
edited it outside harnaas.

**Managed block**:
A marker-delimited region harnaas owns inside a file the team also writes. Everything outside it is
preserved untouched. Used where harnaas must contribute to a file it cannot own outright — the
project's memory file and its version-control ignore file.

**Shared target**:
A destination more than one harness reads, so a single write serves several of them. Contrast with a
per-harness target, which only its own harness reads.

**Emulated**:
Describes an asset installed through a different type's surface because its own is absent on that
harness — a command delivered as a skill that the model will not invoke on its own. Reported as its
own outcome, never as plain success.

**Support tier**:
How current a harness surface is, which decides whether harnaas will write to it: live surfaces are
written, removed ones are refused, and gated or deprecated ones are written with a note.

**Convergence**:
Install bringing the installed set into agreement with the manifest, including removing assets that
are no longer declared.

### Checking

**Lint**:
The read-only check that the installed state still agrees with the manifest and the lockfile, and
that nothing has moved on upstream. Lint examines the *installation* — never the content of an asset.
_Avoid_: doctor, check, verify

**Finding**:
One problem lint reports, carrying a severity and the command that resolves it.
_Avoid_: issue, error, violation, diagnostic
