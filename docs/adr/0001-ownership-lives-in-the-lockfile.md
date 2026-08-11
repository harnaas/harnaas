# Ownership lives in the lockfile, not in a marker inside the file

harnaas needs to know which files under `.agents/` and `.claude/` it may update or delete and which
are the user's. The entire.io CLI, whose architecture harnaas otherwise copies, solves this by
embedding an `ENTIRE-MANAGED …` marker in every file it writes. harnaas records ownership in
`harnaas.lock.json` instead: a destination recorded there is managed, and a destination absent from it
is never overwritten or deleted, on any flag.

## Considered Options

The marker approach was rejected because entire.io *generates* the content it scaffolds and is
therefore free to add a line to it, whereas harnaas *copies* content verbatim from upstream. Injecting
a marker would mean the installed bytes never equal the source bytes — which would defeat the digest
comparison that `harnaas lint` uses to distinguish "you edited this" from "upstream changed this".
The mechanism meant to enable the check would have destroyed it. A marker would also be invalid
syntax in formats that carry no comment form.

## Consequences

Deleting the lockfile does not orphan installed files; it makes them unmanaged, which is protective
rather than destructive. A project with pre-existing harness files gets conflict reports on its first
install rather than a silent takeover. Both follow from the rule and are intended.

The exception is content harnaas authors itself rather than copies — the block in `AGENTS.md` and the
block in `.gitignore` — where a marker is correct precisely because there are no upstream bytes to
stay equal to. See [0003](./0003-instruction-content-in-an-agents-md-block.md).
