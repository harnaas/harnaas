# An available update is a lint error, not a warning

`harnaas lint` exits non-zero when any asset is behind a newer tag, and also when any asset tracks a
mutable ref such as a branch. The only state that passes is one where every source is pinned to a tag
or commit and is current. Nearly every comparable tool treats an available update as advisory; this
one does not.

## Considered Options

Treating updates as warnings was the initial design, and it was rejected because harnaas exists to
make a team's harness configuration uniform and current — a warning that CI tolerates indefinitely
does not achieve that.

Branch-tracking assets are errors too, for a specific reason: a branch moves whenever upstream
commits, so if a moved branch were reported as "outdated" there would be no stable passing state at
all and CI would be permanently red. Reporting it instead as "not pinned, not reproducible" gives the
user an achievable fix — pin it — and makes the forcing function coherent rather than infinite.

## Consequences

`harnaas lint` is not deterministic across time: a project can pass today and fail tomorrow because
somebody upstream published a tag, with no local change. This is inherent to enforcing currency and is
mitigated by `--offline`, which performs every local check and skips the network entirely, for callers
who need a time-invariant result.

Exit code `2` is reserved for findings so that a red pipeline distinguishes "lint found problems" from
"lint itself failed" — the one place harnaas extends the exit-code contract it inherited from
entire.io.
