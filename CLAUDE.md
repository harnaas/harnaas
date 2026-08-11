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

A Ctrl-C typed at a prompt is an interrupt that never became a signal: the form puts the terminal in
raw mode, which disables the line discipline's signal characters, so it arrives as a keystroke the
form consumes. That is why the prompt reports it as an interrupt rather than as an ordinary
cancellation, and why the entrypoint terminates on it as though the signal had been delivered. The
alternative is the exact outcome the re-raise exists to prevent, reached by a different route.
A cancelled prompt that *did* come from a cancelled context keeps that context's error as its cause
for the same reason: the entrypoint decides whether a run was signal-driven from the cause, and an
error that discards it turns a signalled shutdown into an ordinary exit.

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

### A source kind is registered per run, and dispatch happens before any work

`cmd/harnaas/cli/source` is the only place harnaas reaches the network or reads content out of a
repository, and every kind answers one question: given an interpreted asset and the `sources` entry
it references, produce the files and the provenance needed to record what landed. The install flow
asks a `Registry` for a resolver and hands it a request, so it contains no branch on which kind it
got, and an unsupported kind fails at the lookup — before the kind is entered — which is what makes
"no network request and no filesystem write" a property of the dispatch rather than a rule each kind
honours on its way in. The kind a request needs is answered once, on the request: the project-local
form references no entry and is local by definition of the grammar.

Registration takes a constructor rather than a value, because resolution is not stateless. An
archive is fetched at most once per repository and commit *per run*, and the memory of what has
already been fetched belongs to the run that did it — a kind shared across runs would either leak it
between them or need a global cache to hold it. A second registration of one name panics: it is
reachable only from harnaas's own wiring, and both silent outcomes end in a binary that resolves
through whichever kind was linked last.

A kind package registers itself from its own `init`, so whether harnaas can resolve a kind is whether
the binary links its package — and `cmd/harnaas/cli/sourcekinds.go` is the one file where that is
decided and reviewed. Self-registration's own failure mode is a kind that compiles, passes its own
tests and is missing from the registry because nothing imports it, so the set is asserted whole rather
than by membership: a kind that stopped registering is a manifest that suddenly fails to install, and
one that started registering is a source form harnaas accepts before anybody wrote down what it means.

### A resolved source cannot disagree with its own digest, and can never be empty

`NewResolved` is the only way to obtain a `Resolved`, so every one has files sorted by path, a digest
per file and a whole-source digest computed the same way — the same rule as `Interpret` being the
only route to an `Asset`. It refuses a source with no files at all, which is what stops a retrieval
that failed halfway from being handed on as a source that legitimately resolved to nothing and
converging to the deletion of everything the asset had installed.

Paths participate in the whole-source digest alongside content, so a renamed file changes it even
though no byte did, and each path is length-prefixed in the hashed serialization because an archive
entry may be named anything — without the frame, two different sets of files can serialize
identically. File modes are not hashed at all: an executable bit carries no meaning on a document,
and hashing one would make a Windows machine and a Linux machine permanently disagree about whether
upstream had moved. The requested ref is recorded beside the commit it resolved to and never
collapsed into it, because "the installed files still match the commit" and "the tag now points
somewhere else" are the two questions lint asks separately.

### There is one fetcher, and a URL is redacted by the type that prints it

`source.Fetcher` is the only way harnaas makes an HTTP request, and it holds every transport rule at
once: https only, never a destination on this machine, a bounded redirect chain, a total timeout and
a size ceiling. The rules live beside the source contract rather than inside the one kind that
fetches today, because a forge added later must not be able to make a request that skips them — and
the only structural way to guarantee that is for there to be no exported way to build a second
fetcher. The destination rule runs again on **every redirect hop**: Go's default policy follows ten
redirects across schemes and hosts and only the URL harnaas built was ever checked, so an https entry
point could otherwise deliver the archive over plaintext from anywhere. Loopback is refused because
content already on this machine is what a `local` source under `.harnaas` is for, so a hop aimed here
is either a misconfiguration or a redirect at a service that never expected an outside request. The
one exemption is unexported and exists so the suite can serve a body at all.

The body is read one byte past the ceiling and buffered whole. A bare `io.LimitReader` stops at the
limit and reports success, so an oversized archive would arrive truncated, extract, and have its
digest recorded as though it were the real content — the failure a content digest may never have.
Returning bytes rather than a stream is what makes "no partial body reaches a caller" structural.

Every URL harnaas prints, logs or writes goes through `source.RedactURL`, which drops userinfo **and
the query string**: an archive download redirects to a signed URL whose `token=` grants the bearer
the access the request had, so a redaction that removed only userinfo would have looked right and
leaked the credential harnaas actually meets. Each transport error holds the URL raw and redacts
where its message is built, so redaction is a property of the type rather than a step every caller
constructing one has to remember — and a cause is unwrapped out of its `url.Error` before it is
quoted, because that wrapper restates the unredacted URL.

### An archive is extracted into memory, and every name in it is untrusted

Extraction takes the subtree the manifest named and produces exactly the path-to-content mapping
`NewResolved` consumes. Nothing is written, which is what makes "no partially extracted content is
left behind" structural rather than a cleanup step some failure path could skip: a failed extraction
returns no map at all and there is nowhere else the bytes could have gone. It is also why containment
here is textual where `.harnaas` reads and destination writes go through a kernel-anchored handle —
the time-of-check-to-time-of-use race those handles exist for needs a filesystem to race with, and an
archive already in hand has none. What is checked is the archive's own names, which are somebody
else's untrusted input in the same class as an asset id.

The wrapper directory a forge puts around a repository archive is taken from the archive rather than
reconstructed from the repository and commit, because its spelling is the forge's to choose and a
harnaas that predicted it would be one forge release away from selecting nothing and reporting every
asset's path as missing. An archive with a second top-level directory is refused instead, since
"strip the first component" would otherwise discard content in silence. A subtree naming a single
file — a rule and a command are one file, not a directory — resolves under its own leaf name, because
the path relative to a file is nothing and `NewResolved` refuses a file with no path.

Containment is checked for **every** entry and kind only for the selected ones. An entry that climbs
above the root cannot be classified as inside or outside the subtree in the first place, while a
symbolic link elsewhere in the repository is its owner's arrangement of their own files and refusing
an asset over one would make an unrelated tree harnaas's business. Three ceilings apply, and the
third is not redundant: per file and per asset bound what is installed, and the decompressed stream
is bounded separately because an archive whose selected subtree is one small file is still read to
its end, so neither of the other two ever sees the bytes a compression bomb produces. The stream
reader fails past its ceiling rather than reporting the end, for the reason the body reader does —
stopping would hand the tar reader an archive that appears to end there.

### Names are resolved with Git, bytes are moved over HTTPS

A `github` ref becomes a commit through `git ls-remote`, not through the forge's API. That path goes
through whatever credential helper the user already configured, so a private repository resolves
without harnaas inventing an authentication story, and it has no API rate limit for a CI job to
exhaust. Content then arrives as one archive per repository and commit rather than one request per
file. Resolution stays separate from retrieval for a second reason: the lockfile records what was
*asked for* beside what it *resolved to*, because "the files still match the commit" and "the tag now
points somewhere else" are the two questions `lint` asks separately.

A full commit identifier is answered without asking the remote at all — the reply cannot differ from
what the manifest wrote, and a repository that is unreachable today should not fail a resolution that
needs nothing from it. Only a *full* identifier counts: an abbreviation names whichever object it is
unique against today, so one that grew a second match upstream would silently become a different
install. A bare name means the tag before the branch, which is Git's own precedence, and an annotated
tag resolves to the commit it peels to rather than to the tag object, which is not what an archive is
taken from.

Content then arrives as one archive per repository *and commit* — keyed by the commit rather than by
the ref, because two assets pinned to a tag and to the commit it names are one retrieval. That memo is
the reason a kind is constructed per run: several assets legitimately name different subtrees of one
repository, so retrieval is asked for per asset and performed once. A retrieval that *failed* is
remembered too, since the next asset would meet the same host and the same answer — and the caller
attributes the remembered failure to its own asset, so the second asset's diagnostic still names the
second asset. The archive URL is the API's tarball endpoint rather than the content host directly,
because that is the documented route taking an authorization header, and it is why the fetcher follows
a bounded redirect chain at all: the endpoint answers with a signed URL whose query string carries the
grant `RedactURL` exists to drop.

A diagnostic wrapping a finished diagnostic quotes its *problem* and not its fix, through
`source.Problem`. A retrieval failure names the asset, the repository and the commit — the third being
a fact the manifest does not contain — and offers one remedy, because a message carrying two leaves
the reader deciding which of them is theirs.

An unknown ref is a property of the output and never of the exit status: `ls-remote` exits
successfully having printed nothing, so a run that checked the status would report every unknown ref
as a resolution to nothing. What comes back is checked for being an object identifier before it is
used, because a remote's answer is untrusted text on its way into a URL — and every pattern harnaas
passes is built with a `refs/` prefix, so a ref out of a manifest can never arrive as something git
reads as an option. `GIT_TERMINAL_PROMPT=0` and a detached stdin close both routes to a credential
prompt, which under a CI job or a coding agent is not a question anybody answers but the run hanging.
Git missing from the machine is its own diagnostic, because it is the one ref failure no edit to
`harnaas.json` would fix.

### A token is named by where it came from, never by what it is

The HTTP half of a `github` source authenticates with a token read once per run from
`HARNAAS_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`, in that order, and proceeds unauthenticated when
none is set. The two ambient names are honoured because harnaas is rarely the only tool on the machine
that needs a token — `gh` sets one and an Actions job is handed one — so a CI job that already
authenticates needs no harnaas-specific configuration, and the harnaas-specific name comes first so a
project that needs something the ambient token cannot read has somewhere to say so. A variable set to
nothing counts as unset, because a job that exports a token conditionally leaves an empty value
behind and an empty bearer header fails a request that would have succeeded without one.

The token travels as a `source.Credential`, which carries the value *and the name of where it came
from*, and whose every rendering — `String`, `GoString`, and so every `%v`, `%s` and `%#v` — prints
the origin. That is what lets a diagnostic be actionable without quoting a secret: the type has
something safe to name, so no message has to reach for the value. `AuthorizationError` has no field a
token could be put in for the same reason. Naming the *variable* is also what makes the message
correct for the reader — a run that had a token is told which one to check, and a run that had none is
told every variable it could have supplied one through.

A credential is a parameter of `Fetch` rather than a second method, so an unauthenticated request is
the zero value rather than a call that forgot to say. It is dropped on any redirect that leaves the
host **and port** harnaas asked: Go's own policy only drops it across unrelated domains, and the hop
that matters here is an archive endpoint answering with a signed URL on a content host that carries
its own grant — so there is never a hop that both leaves the service and needs the token.

`401`, `403` and `404` from the archive endpoint are all access decisions. The third is not the
obvious reading: GitHub answers `404` for a repository the caller may not see, so a stranger cannot
learn a private repository exists by asking — and by the time an archive is fetched, ref resolution
has already proved over Git, with the *user's Git credentials*, that the repository exists and holds
this commit. A `404` here is the forge declining to admit to harnaas's HTTP identity what it just
admitted to its Git one, and the fix is a token rather than a line in `harnaas.json`.

### A cache entry that cannot prove what it is gets discarded, not returned

An archive is fetched at most once per run by the kind's own memo and at most once per *machine* by
`source.ArchiveCache`, which lives beside the source contract for the reason the fetcher does: a forge
added later must not be able to keep its own store under its own rules. Entries sit under the user's
cache directory with `HARNAAS_CACHE_DIR` overriding it — the override names harnaas's cache root
outright, so it replaces the default rather than nesting under it.

The store is content-addressed in the literal sense: an archive is filed under its own digest, and a
pointer named by the digest of `(kind, repository, commit)` records which digest that is. Verifying an
entry is therefore checking the bytes against the name they are filed under, which cannot drift out of
step the way a digest recorded beside them could. Each field of the key is length-prefixed before
hashing for the reason a resolved source's paths are — unframed, `acme/assets` at commit `a` and
`acme/asset` at commit `sa` are one entry, and one repository would be served the other's content. A
pointer's contents are checked to be a hexadecimal sum *before* they are joined into a path, because a
file on disk is untrusted input in the same class as an archive entry name: without that check a
pointer reading `../../elsewhere` is read and then deleted by the discard path.

Nothing here can fail a run. A miss, an unreadable entry, a pointer to nothing and content that no
longer hashes to its own name are one answer — fetch it — and a damaged entry is removed on the way
out so the next run does not pay to rediscover it. A cache write that fails is a log record, never an
error: the cache exists to make a run cheaper, and one that can make a run fail has cost more than it
saves. Only a fetch that *succeeded* is filed, which is exactly the opposite of the in-run memo's rule
that a failure is remembered too — one outage should not be multiplied across the assets that met it,
and it should not survive the run either.

The credential is deliberately not part of the key: the archive of a commit is the same bytes whoever
fetched it, so a token is an access decision and not a content one. The consequence is that an entry
is readable by whoever can read the directory holding it, which is why the default location is the
user's own cache directory with owner-only permissions — the person who fetched it had access at the
time, and nobody else on the machine gains any.

The bypass is the absence of a cache rather than a flag on one: `source.RunOptions` carries the cache
the run may use, a nil one stores and reuses nothing, and the options are handed to every kind at
`Registry.NewResolver` so a run cannot end up with one kind reading the cache and another not.

### An offline run refuses a name, and refuses it without asking

Offline is the second field on `source.RunOptions`, settled at the same seam the cache is, because a
run is offline for every kind or for none. What it forbids is a request of *any* kind, which is two
different requests: the archive, and the lookup that turns a ref into a commit. The archive half is
the cache running out — a miss becomes a refusal rather than a fetch, remembered by the in-run memo
like every other retrieval failure so a second asset of one repository is not made to wait for the
same nothing, and still named after the asset that read it. The ref half is not the cache's business
at all: `v1.2.0` and `main` are names in somebody else's repository, and what they point at today is
a fact this machine can only be told. So a name fails offline, and the branch sits above the lookup
rather than inside it — a lookup that decided to refuse would still be a lookup, and no lookup is the
property that was asked for. A full commit identifier resolves offline exactly as it does online,
which is also how the lockfile's recorded commit will resolve once `install` feeds one in: it is a
commit identifier, so it needs no new offline path.

Serving whatever archive this machine happens to hold for a repository is the one answer offline
mode exists to refuse — it installs a commit the manifest never asked for and reports it as the one
it did. Skipping the asset and reporting it unchanged are the same failure by a quieter route. An
offline miss is therefore its own diagnostic rather than a retrieval failure: nothing was asked, so
there is no host, status or cause to quote, and the remedy is one run with the network rather than a
line in `harnaas.json` to change.

### A local source is read through a handle, and asks nobody anything

The `local` kind is the one that makes no request: there is no ref to resolve and no archive to
fetch, so an offline run and a networked one do identical work and the run's cache has nothing to
offer a read costing one syscall. It therefore takes `RunOptions` and ignores them, rather than
branching on them — the options describe requests, and this kind makes none.

What it has that no other kind does is a live filesystem. The manifest grammar already refused every
path that leaves `.harnaas` *textually*, but a directory validated in one moment can hold a symbolic
link to somewhere else in the next, so every read goes through a handle anchored at `.harnaas` and
containment becomes the kernel's answer at the moment of the read rather than harnaas's answer at the
moment of the check. That is the mirror image of archive extraction, which checks containment
textually because an archive already in hand has no filesystem to race with.

Three refusals are told apart because they need three different edits: the content is not there, the
path led out of `.harnaas`, or the machine would not let harnaas read it. The middle one is
recognized by the standard library's own wording, because `os` keeps that sentinel unexported — the
fallback is what makes the match safe, since a rewording reports a containment violation as a read
that failed, which is less specific and never wrong, and never lets the read through. Every
diagnostic names the path relative to the *project root*, in the manifest's own spelling, because
that is the file the reader has open.

The asset's own path is the one place a link is followed, where the anchor bounds where it can lead.
Inside the tree a link is refused like any other entry that is not a regular file, which is the rule
archive extraction already applies and additionally makes the walk unable to loop.

### What resolved is verified once, above the kinds, and never rewritten

A skill fetched from a forge and a skill read out of `.harnaas` are wrong in exactly the same ways, so
`source.Verify` sits above the kinds rather than inside each of them: every kind resolves, and one
function checks what resolved. A `skill` must be a directory carrying a `SKILL.md`; every other type
must be one regular file. Whether a source *was* a file is derived rather than carried — both kinds
resolve a single file under its own leaf name, so one file whose path is the leaf of the declared
subtree is a file — which keeps the shape a property of what every kind already agrees on instead of a
flag each kind sets and one of them eventually sets wrongly. Its one blind spot is stated in the code:
a directory holding exactly one file named after itself reads as a file, and is refused either way.

The name check is the reason the whole phase exists. A harness that reads a skill's frontmatter `name`
uses it to decide the skill is there, so a skill installed as `review` whose frontmatter says
`code-review` installs cleanly, reports success and is never invoked — the one outcome a tool whose
purpose is telling a team what is in effect must not produce. harnaas refuses it rather than
correcting it, and the refusal is structural: `source.Frontmatter` splits a block textually, decodes
values *out* of it into the caller's own type, and offers no encoder at all. There is nothing to
re-serialize with, so no later phase can rewrite an author's frontmatter by accident — which matters
most for a `rule`, where a YAML writer's choices about quoting and folding a `paths:` list can change
what the rule applies to.

Absent, unparseable and present-without-a-name are one diagnostic with three reasons rather than three
types, because the reader's next action is the same in all three: open the file and look at the top of
it. A block that is never closed is "no frontmatter" for the same reason.

### An adapter answers where, and the absence of an answer is one of the answers

`cmd/harnaas/cli/adapter` is the contract and registry for the harnesses that need per-harness code,
and each adapter is a package beneath it. Most harnesses need none: a skill and an instruction reach
them through shared locations, and only a `rule`, a `command` and a `persona` — which have no shared
equivalent anywhere — go through an adapter. "A harness with no adapter" is therefore a supported
state, which is why a lookup that finds none is a typed diagnostic the caller attributes to the asset
that needed one, never a refusal the registry decides on its own. It earns its package the way
`source` does: the install flow in the flat `cli` package imports the adapters, so a contract living
beside the flow would be a cycle one file later — and having it separate is what makes the import
boundary checkable at all.

An adapter answers questions and performs no I/O, the same rule the harness roster holds to. It is
handed an `fs.FS` to detect through, so "detection creates nothing" is a property of the signature
rather than a rule each adapter honours; a harness that is not installed is an absence and never an
error, because there is nothing for a caller to do about it. Its roots are returned *relative* to the
scope's own root — the project root, or the user's home — because an adapter reading the environment
for a home directory would be a second place harnaas decides where a user's files live. A destination
is relative to the resolved scope root beneath that, so one mapping serves both scopes and the path
the lockfile records means the same thing on another machine.

Where a harness has no surface for a type, the adapter says so as a value rather than mapping it to
an invented path: once written, a guessed destination is indistinguishable from a real one, so the
caller reports the pairing unsupported and installs the asset's other targets. The terms travel with
the path in one `Destination` — support tier, replacement, note and renderer — because a caller that
could obtain a path separately could write it without ever asking whether the harness still reads
there. The renderer is a *name* rather than a function, which is both what keeps the rendering layer
outside the import boundary and what lets a surface declare a renderer nobody has written yet: that
pairing is reported unsupported, where falling back to copying would write a file the harness cannot
read.

Which scope an asset has is settled before an adapter is asked anything, so `adapter.ResolveRoot`
asks the one question only an adapter can answer: does this harness have an unambiguous directory for
that scope. The roster's `HasPerUserLocation` is deliberately not taken as the same answer — it is a
fact the manifest layer can check without linking a single adapter, while the adapter is the thing
that knows its own directories — and where the two disagree the adapter is right. A scope it does not
offer is refused rather than resolved to the project root, because an asset installed at a scope its
author did not ask for lands in a file they have no reason to open. What this layer must *not* do is
restate the rules the manifest already applied: an `instruction` at `user` scope is refused once,
upstream, for a reason about what an instruction is rather than about any harness's directories, and
a second refusal here would be a second place to keep it correct.

Registration mirrors the source registry, with one deliberate difference: an adapter is registered as
a value rather than as a constructor, because a source kind remembers which archives this run fetched
and an adapter is a pure mapping with nothing to remember. Handing every run its own copy would
suggest it had state. A second adapter for one harness panics, for the reason a second source kind of
one name does.

### The one adapter maps three types, and says nothing about the other two

`cmd/harnaas/cli/adapter/claudecode` is v1's only named adapter: `.claude` at both scopes, a rule at
`rules/<id>.md`, a command at `commands/<id>.md` and a persona at `agents/<id>.md`, each with the
asset id as the stem. A `skill` and an `instruction` are deliberately *absent* from that table rather
than mapped into `.claude` as well — they reach every harness through shared locations, and an
adapter answering for them would be a second place harnaas decides where a skill lands. The scope
does not enter the mapping at all: it selects the root the path is joined beneath, which is
`Root`'s answer asked once by `ResolveRoot`, so one relative path serves both scopes.

Detection reads the *roster's* evidence rather than a second list, because `init` detecting a harness
to scaffold a manifest and an adapter detecting one to report an install must not disagree about one
project — and it uses `fs.Lstat` for the reason `init` uses `Lstat`: the question is whether the
harness left something behind, not whether it resolves. A rule installs as a standalone file the
harness discovers on its own, and nothing references it from `CLAUDE.md`, `AGENTS.md` or a managed
block; the adapter can add no such reference because a destination is the whole of its answer.

Adapter packages self-register from their own `init` and `cmd/harnaas/cli/adapters.go` is the one
file where linking them is decided, exactly as `sourcekinds.go` is for source kinds. The import
boundary is a test rather than a linter rule, because it is a property of this directory rather than
of the module: an adapter may reach the contract, the roster and the manifest vocabulary, and
nothing else — not the install flow, which imports *it*. Go permits that import right up until the
cycle closes, so the test is what makes the failure arrive while it is still one import.

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
