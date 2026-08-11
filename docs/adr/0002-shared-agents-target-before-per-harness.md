# Skills install to the shared `.agents/` directory before a harness's own

A survey of 23 AI coding harnesses found that 17 of them read `.agents/skills/<name>/SKILL.md`, and
21 read `AGENTS.md`. harnaas therefore writes skills to the shared location first and falls back to a
harness's own directory only where that harness does not read the shared one. The obvious design —
one directory tree per harness — is deliberately not what we do.

## Considered Options

Per-harness directories (`.claude/skills/`, `.cursor/skills/`, …) are more explicit: declaring a
harness would mean something concrete, and ownership would be unambiguous. They were rejected because
they produce N byte-identical copies of every skill, N lockfile installations, and N things to keep in
step — to buy explicitness the ecosystem has already stopped needing.

## Consequences

Two costs are accepted deliberately.

A shared directory is visible to **every** installed harness, not only the ones listed in
`harnaas.json`. The `harnesses` list therefore means "which harnesses we guarantee", not "which
harnesses will see this".

Ownership in the shared directory is a 17-way problem rather than a 2-way one, and other tools write
there too — `openspec` among them. The lockfile rule in
[0001](./0001-ownership-lives-in-the-lockfile.md) is what makes this safe: another tool's file at a
destination harnaas wants is simply an unmanaged conflict, reported and never overwritten.
