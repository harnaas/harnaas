# Instruction content goes in a marked block inside AGENTS.md

`instruction` assets are always-on guidance that must survive a fresh clone without anyone running
install, which means they have to live in a committed file the project already reads. harnaas writes
them into a marker-delimited block in `AGENTS.md` — read natively by 21 of 23 surveyed harnesses —
and adds a single `@AGENTS.md` line to `CLAUDE.md` to bridge Claude Code, which does not read
`AGENTS.md` at all.

## Considered Options

Two alternatives were examined and rejected.

**A file harnaas owns outright, pulled in by an import line.** Much the nicer model: ownership is
total, drift is a whole-file comparison, and no marker parsing is needed. Rejected because import
syntax exists in only about 6 of 23 harnesses — Codex, Cursor, Pi, Zed, Continue, Augment, Trae, Warp
and Factory have none — so every other harness would need a marker block regardless, and we would
build and maintain both mechanisms to use the nicer one a quarter of the time.

**Not writing into user-owned files at all**, which is `openspec init`'s central design rule. It
ships a legacy-cleanup module whose only job is un-writing marker blocks its own older versions added
to `CLAUDE.md` and `AGENTS.md`. That rule works for openspec because its single artifact can live in a
tool-owned subtree; harnaas's `instruction` type is *defined* as content in a file the user owns, so
there is no subtree to retreat into. We are knowingly re-adopting what openspec removed, for a
requirement openspec does not have.

## Consequences

Drift is region-level rather than file-level, and merge conflicts will land inside the block. Both are
accepted. Everything outside the markers is preserved byte-for-byte, an edit inside the block is
reported and never silently overwritten, and the block is removed entirely when the last instruction
asset goes away.
