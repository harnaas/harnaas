# A command is delivered as a skill only where the harness can be told not to start it

A command is invoked deliberately by a user and a skill is started by the harness when a description
matches, so harnaas delivers a command through a harness's skill surface *and* writes the frontmatter
setting that stops the harness starting it — the disabling is the whole of that renderer rather than a
detail of it. Devin CLI is the first harness on the roster whose skill format has no such setting in
the vocabulary harnaas writes, and whose invocation modes are both on by default. harnaas therefore
refuses the pairing: a command reaches a harness through its skill surface only where harnaas can also
silence it there, and where it cannot, the pairing is reported unsupported and nothing is written.

## Considered Options

**Emulate anyway and state the caveat in the install report.** Rejected because the note is read once
and the file is read forever. The installed skill would be indistinguishable from one the team meant
to be self-starting, and the first anybody learns of it is the harness running a deploy command
because the conversation mentioned deploying — which is the failure the `command` type exists to name
and the emulation exists to prevent. A tool whose purpose is telling a team what is in effect must not
install something whose behaviour contradicts what it reported.

**Write each harness's own suppression key from a renderer per harness.** The right answer eventually,
and rejected here as scope rather than as direction. It needs a second renderer, a way for a surface
to declare which one it wants, and an answer to what an emulated command means in the *shared* skills
directory, which several harnesses read and only one of which would understand the key. Stating the
precondition first is what makes deferring that safe: adding such a renderer turns an unsupported
pairing into an emulated one and invalidates nothing decided here.

**Refuse by default and record which harnesses can be silenced.** Rejected because it inverts a table
harnaas already keeps the other way round. The harnesses that do not read the shared skills directory
are recorded as the exceptions they are, and recording suppression as an allowlist would make every
harness added later fail a pairing until somebody remembered to enable it — a silence that reads as a
missing surface rather than as a missing entry.

## Consequences

The default is that a harness honours the setting unless it is recorded as not honouring it, which is
the same shape as the shared-skills table beside it and carries the same cost: a harness added without
that check gets a command emulated with a key it may ignore. The cost is accepted because the entry is
one line in one file, and because the alternative fails the harnesses that would have worked.

A team declaring commands and adding a harness that cannot be silenced gets one unsupported pairing
per command, reported with its reason while every other target of the same asset installs. No manifest
edit fixes it, which is deliberate — the remedy is a change to harnaas rather than a change the reader
is told to make to a file that is already correct.

The refusal for a harness with no skill surface at all is kept separate, because the two say different
things: one harness has nowhere to put a command, and the other has somewhere that would be wrong to
use. A diagnostic that reported the second as the first would send its reader to check a surface that
is not missing.
