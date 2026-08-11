## Context

By this point `install` has produced two artefacts that describe the same reality from different
angles: `harnaas.json` says what the project wants, `harnaas.lock.json` says what it got. The files on
disk are a third view, and all three drift apart in ordinary use — someone edits an installed skill
during a debugging session, someone adds an asset and forgets to install, an upstream repository tags
a new release.

`lint` is the command that compares all three. Its design is dominated by two questions: what it is
allowed to do about what it finds, and how it behaves when the network is not available. Both answers
are shaped by the fact that its most valuable place to run is CI, where nothing can be interactive,
nothing should be repaired implicitly, and a flaky network must not turn a code review into a red
build.

The precedent is the source CLI's `plugin doctor`, which re-hashes each managed binary against the
digest recorded at install and reports *"no longer matches the digest recorded at install; it was
modified or replaced outside entire"*, pairing every problem with a runnable fix.

## Goals / Non-Goals

**Goals:**

- Detect every way the installed state can diverge from what was declared and recorded.
- Make each finding actionable: a user should never have to work out what to do next.
- Be safe and useful as a required CI check.
- Never let a network condition change the verdict on a local problem.

**Non-Goals:**

- Repairing anything. Deliberate — see below.
- Judging asset content. `lint` checks integrity and freshness, not whether a skill is well written.
  The name means "check the installation", not "check the prose".
- Verifying authenticity of upstream sources. As in `add-harnaas-install`, digests establish integrity
  over time, not provenance.
- Watching for changes continuously. `lint` is a point-in-time check.

## Decisions

### Lint never repairs

A `--fix` flag was the obvious feature and is deliberately absent. Three reasons decide it.

Every repair `lint` could perform is something `install` already does, and doing it in two places
means two implementations of overwrite protection, convergence and atomicity that must agree forever.
Every finding therefore names an `install` invocation instead, which is the same shape the source
CLI's doctor uses: the fix is a command the user can read, understand and run.

A read-only command is safe to run anywhere — in a pre-commit hook, in CI, on a colleague's machine —
without anyone having to reason about what it might change. That property disappears the moment one
flag can mutate state, because now every invocation has to be checked for that flag.

And the findings most worth surfacing are precisely the ones where the right action is a judgement
call. A locally modified file might be someone's uncommitted improvement; an extraneous file might be
deliberate. Automatically resolving those destroys information. Reporting them preserves it.

### Exit code 2 means findings, and only findings

CI needs to distinguish three outcomes, not two: everything is fine, the check ran and found
problems, and the check itself broke. Collapsing the last two into a single non-zero status makes a
red build ambiguous exactly when someone is trying to debug it — an unreadable lockfile and a drifted
skill would look identical.

So `0` is clean, `2` is findings, and `1` remains a genuine runtime failure. This is the only place
`harnaas` extends the exit-code contract it inherited, and reserving `2` for one meaning is what keeps
that extension from spreading.

### Severity splits "reality disagrees with the record" from "worth knowing"

Errors are conditions where the installed state does not match what was declared or recorded: drift,
missing files, unmanaged conflicts, skew between manifest and lockfile. These are objectively wrong
and someone must act.

Warnings are conditions that are true, useful, and not necessarily wrong: an upstream update exists,
a source tracks a mutable ref. A project can legitimately sit on an older version or deliberately
track a branch, and a tool that fails the build for that would be turned off within a week.

`--strict` exists for teams who disagree with that line and want updates to gate their pipeline. Making
it opt-in rather than the default means the useful behaviour is available without the annoying
behaviour being imposed.

### Network failure never decides the verdict

Update detection is the only part of `lint` that needs a network, and it is the least important part.
If GitHub is unreachable, the local integrity checks are still completely valid and still worth
running. So a fetch failure marks the affected assets as unchecked, reports the cause once per host
rather than once per asset, and leaves the exit status to be decided by the checks that actually ran.

The corollary is specified explicitly: a clean report must say what it did not check. A green result
that quietly skipped half its work is worse than a red one, because it is trusted.

### Caching keeps lint cheap enough to run often

A command intended for pre-commit hooks and repeated local runs cannot make a round of network calls
every time. Ref-resolution and tag-listing results are cached with a bounded freshness window, with a
forced-refresh escape hatch. A corrupt cache entry is discarded rather than propagated as a failure —
a cache is an optimization, and an optimization that can fail the command is a liability.

### Local sources are checked for updates too

An asset backed by a file under `.harnaas` can fall behind exactly like a remote one: the file is
edited, and the installed copy no longer matches. Treating that as an update rather than as drift is
the right framing — the *source* moved ahead, and the remedy is to install, not to restore. It also
means update detection has a meaningful offline component, so `--offline` is not simply "turn off half
the command".

### One check suppresses the others rather than cascading

An unloadable manifest would otherwise make every declared asset look uninstalled and every lockfile
entry look orphaned, burying the one finding that matters under dozens that are artefacts of it. So a
manifest failure is reported as a single finding and the checks that depend on it are skipped. The
same reasoning collapses a wholly missing destination into one finding instead of one per recorded
file.

### Findings are ordered deterministically

The report is ordered by asset identifier and path so two runs against the same state produce
identical output. This is what makes the JSON report diffable and makes a CI log comparable between
runs — the same reason install's execution order is fixed.

## Risks / Trade-offs

- **No `--fix` means an extra step for the common case.** Accepted. The remedy is always a printed
  command, and `install` is idempotent, so the extra step is cheap and explicit.
- **Extraneous-file detection may be noisy.** A harness or editor could write incidental files inside a
  managed directory. Reported as a finding rather than ignored, because silently tolerating unknown
  files inside a managed destination would undermine the integrity claim; if real-world noise appears,
  the narrow fix is an ignore rule, not weakening the check.
- **Exit code 2 is a contract callers must learn.** Bounded by specifying it once and reserving it for
  a single meaning.
- **Update detection makes `lint` non-deterministic across time.** The same project can be clean today
  and warn tomorrow because upstream tagged a release. Contained by keeping updates at warning severity
  and by `--offline` for callers who need a fully deterministic check.
- **Cached results can be stale within the freshness window.** Intentional. The alternative is a
  network round trip on every invocation, which would make `lint` too slow to run habitually — and the
  forced-refresh path exists for when currency matters.

## Open Questions

- Whether extraneous-file detection should support an ignore list once real usage shows what
  incidental files appear inside managed destinations. Deferred until there is evidence rather than
  speculation.
- Whether `lint` should eventually verify that a `skill` asset's definition file parses as the harness
  expects. That is content validation rather than installation integrity, and it would need a
  per-harness schema; the adapter boundary is where it would go if it is ever wanted.
