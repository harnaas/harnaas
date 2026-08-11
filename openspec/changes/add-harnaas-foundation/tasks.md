## 1. Repository and toolchain scaffolding

- [x] 1.1 Initialize the Go module `github.com/harnaas/harnaas` and create `cmd/harnaas/`,
      `cmd/harnaas/cli/` and `internal/`.
- [x] 1.2 Add `mise.toml` pinning the Go and `golangci-lint` versions with `gotestsum` installed as a
      post-install step, keeping the Go version identical to the `go` directive in `go.mod`.
- [x] 1.3 Add `mise-tasks/` scripts for `build`, `fmt`, `lint` and `test`, plus a `check` task chaining
      fmt then lint then test; keep any inline `run` block in `mise.toml` under three lines.
- [x] 1.4 Add `.golangci.yaml` on the v2 schema with the standard set plus the extended linter list,
      `nolintlint` requiring both a specific linter and an explanation, and `run.build-tags` covering
      the `integration` and `e2e` tags.
- [x] 1.5 Add `forbidigo` rules banning the standard-library working-directory call and cobra's
      `Print*` helpers, each with a message naming the sanctioned replacement (the context-carried
      project root, and `cmd.OutOrStdout()`).
- [x] 1.6 Add `.goreleaser.yaml` building `cmd/harnaas` for darwin, linux and windows on amd64 and
      arm64 with CGO disabled, stamping version and commit into the `versioninfo` variables via
      ldflags.
- [x] 1.7 Add GitHub Actions workflows for test and lint with every action SHA-pinned, and a single
      aggregator job to serve as the required status check.
- [x] 1.8 Add the license allowlist and a `lint:licenses` task running `go-licenses` against it.
- [x] 1.9 Write `CLAUDE.md` recording the imported architecture rules and pointing at `CONTEXT.md` for
      vocabulary and `docs/adr/` for the load-bearing decisions; point `AGENTS.md` at it.

## 2. Process entrypoint, signals and exit codes

- [x] 2.1 Add `internal/procsignal` recording which signal initiated shutdown so the entrypoint can
      read it after in-flight work has unwound.
- [x] 2.2 Write `cmd/harnaas/main.go`: load version info, build the cancellable root context, and
      install the two-stage signal handler — first signal cancels the context and prints the
      force-quit notice, second signal terminates immediately.
- [x] 2.3 Implement signal-faithful termination: reset the handler and re-raise the original signal to
      the process, falling back to a `128`-plus-signal-number exit only where re-raising is
      unsupported, and distinguish a termination signal from an interrupt.
- [x] 2.4 Add the already-printed error type with `Unwrap`, plus the constructor commands use after
      printing a friendly explanation.
- [x] 2.5 Implement the entrypoint error switch: already-printed errors print nothing further; unknown
      command and unknown flag errors print the error with the root usage; positional-argument errors
      print the deepest matched subcommand's usage; every other error prints its message to stderr
      exactly once.
- [x] 2.6 Implement the exit-code mapping — `0` success, `1` runtime failure, `128`+signum for signals
      — and reserve `2` for a completed `lint` run reporting error findings, documenting beside the
      mapping that no other command may return it.
- [x] 2.7 Add tests for the error switch (each case, no double printing, cause still unwrappable) and
      for the exit-code mapping including signal-derived codes.

## 3. Root command and flag conventions

- [x] 3.1 Build the root command with cobra's error and usage printing silenced, no persistent flags,
      help groups declared in display order, and the version template.
- [x] 3.2 Attach subcommands by explicit constructor calls from the root builder; assert by
      construction that no command registers itself from a package `init`.
- [x] 3.3 Add the shared `--json` helper registering the flag locally on a command, plus its read
      helper that treats an undeclared flag as "not requested" rather than panicking.
- [x] 3.4 Wire command output through `cmd.OutOrStdout()` and `cmd.ErrOrStderr()`, with the `--json`
      document written to stdout alone and advisory, progress and warning text to stderr.
- [x] 3.5 Add tests: the root declares no persistent flags beyond cobra's built-ins; `--json` on a
      command that does not declare it fails as an unknown flag; a wrong positional-argument count
      prints the subcommand's usage, not the root's; an unrecognized subcommand exits non-zero.

## 4. Shared foundation packages

- [x] 4.1 Add `versioninfo` with build-time stamped variables and a loader that falls back to the
      embedded build information and then to a development placeholder, with an explicit stamp always
      winning.
- [x] 4.2 Add `paths` resolving the project root from the enclosing repository, with the context
      carrier and accessor that replace reading the process working directory, and a typed error for
      "no project root found" that commands can render.
- [x] 4.3 Add `logging` over `log/slog` writing structured records to a log file with component and
      command context helpers, an attribute-only API, and a documented rule that user content must
      never be logged.
- [x] 4.4 Add `palette` as a zero-dependency package of base16 slot constants with semantic aliases and
      deliberately no body-text colour.
- [x] 4.5 Add `uiform` wrapping the form library behind an accessible-mode wrapper and a shared theme,
      and `interactive` resolving whether prompting is possible from the attached streams and the
      environment.
- [x] 4.6 Add `jsonutil` with indented marshalling that does not escape HTML and emits a trailing
      newline, plus an atomic write that stages in the destination directory, syncs, renames, and
      removes the staging file on both success and failure.
- [x] 4.7 Add tests: stamp beats build info; project root resolves from a nested directory and fails
      outside a repository; an injected failure mid-write leaves the previous file intact with no
      staging file; accessible mode is honoured; interactive detection returns false for piped streams.

## 5. Harness roster

- [ ] 5.1 Define the harness descriptor: canonical id, display name, whether the harness has an
      unambiguous per-user location, and the observable project evidence indicating it is in use.
- [ ] 5.2 Register `claude-code` as the only recognized harness in this change and name it the default;
      keep the roster data-only, with no destination mapping and no write behaviour.
- [ ] 5.3 Add lookup helpers: recognized ids in deterministic order, an unknown-id error that lists the
      recognized ones, and the per-user-location query that manifest scope validation calls.
- [ ] 5.4 Add tests: the unknown-id error names the input and lists recognized ids; ordering is stable
      across runs; the default id is itself recognized.

## 6. Manifest document, decoding and discovery

- [ ] 6.1 Define the document types: integer `version`, `harnesses` string array, `sources` map of key
      to source string, and `assets` array whose entries are either a string or an object.
- [ ] 6.2 Implement asset-entry decoding accepting both the string and object forms and rejecting any
      other JSON type, naming the entry's index.
- [ ] 6.3 Implement strict decoding across the document and the asset object — unknown fields rejected
      naming the field, malformed JSON rejected naming the parse location, wrong-shaped fields rejected
      naming the field — returning no partial document on failure.
- [ ] 6.4 Implement version handling: `1` decodes per this specification, a greater version is refused
      with a message saying the manifest was written by a newer harnaas and to upgrade, and a missing
      or non-integer version is an error.
- [ ] 6.5 Implement discovery of `harnaas.json` at the project root resolved from the context-carried
      root, with a missing-manifest error naming `harnaas init`.
- [ ] 6.6 Implement the subdirectory rule: a `harnaas.json` found below the project root is an error
      naming that file and stating that only the root manifest is read.
- [ ] 6.7 Keep the loader read-only — open for reading only, no write path outside `init` — so no
      command can rewrite, reformat or normalize the manifest.
- [ ] 6.8 Add tests: unknown field; malformed JSON; both asset entry forms; `assets` not an array;
      newer-version upgrade message; missing version; discovery from a nested directory; subdirectory
      manifest error; missing manifest naming `harnaas init`.

## 7. Manifest interpretation and validation

- [ ] 7.1 Parse a `sources` value into kind, repository and ref (as in `github:acme/assets@v1.2.0`),
      rejecting a kind outside the registered set naming the kind and the source key; resolve nothing.
- [ ] 7.2 Implement the asset string grammar — `<sourceKey>:<path>` and a project-local path beginning
      `.harnaas/` — rejecting any other string naming the entry and both accepted forms, and rejecting
      an undeclared source key naming the key and pointing at the `sources` block.
- [ ] 7.3 Implement local containment: reject an absolute path, and reject a path escaping `.harnaas`
      through parent-directory segments, at load time and before any content is read.
- [ ] 7.4 Implement type and id inference from the path — `skills/` → skill, `rules/` → rule,
      `instructions/` → instruction, `commands/` → command, `agents/` → persona, leaf as the id with
      any extension stripped — rejecting an unrecognized containing directory and directing the author
      to the object form.
- [ ] 7.5 Implement the object override form carrying the source string plus any of `type`, `id`,
      `targets` and `scope`, with a declared value suppressing inference for that field and unknown
      fields rejected naming the field and the asset.
- [ ] 7.6 Implement target defaulting: object `targets` when present, otherwise the manifest's
      `harnesses`; reject an empty effective target list, and reject a target the roster does not
      recognize, listing the recognized ones.
- [ ] 7.7 Implement scope defaulting: `project` by default; accept `user` only where the roster reports
      an unambiguous per-user location, failing by name for any other target rather than falling back;
      reject `user` on an `instruction` asset.
- [ ] 7.8 Implement semantic validation that accumulates every violation into one aggregate error
      rather than stopping at the first, covering duplicate ids within a type (naming both entries) and
      ids that are not a single safe path segment, and ensure a document with any violation is never
      handed to a later phase.
- [ ] 7.9 Make aggregate error output deterministic — ordered by asset index then field — so repeated
      runs produce identical messages.
- [ ] 7.10 Add table-driven parallel tests over every grammar, inference and validation rule, including
      a fixture manifest matching the documented example that loads and validates clean, and a manifest
      with several independent violations that reports all of them.

## 8. Init command

- [ ] 8.1 Add the `init` command constructor with its `--force`, assume-yes and repeatable harness
      flags, registered locally on the command.
- [ ] 8.2 Implement harness detection from the roster's observable evidence, in deterministic order,
      creating nothing while detecting.
- [ ] 8.3 Implement the fallback when nothing is detected: use the default harness and state which one
      was chosen and that nothing was detected, never writing an empty `harnesses` list.
- [ ] 8.4 Make the harness flag override detection entirely, and fail on an unsupported harness name —
      naming it and the supported ones — before anything is written.
- [ ] 8.5 Implement the accessible interactive confirmation of the selection, take the non-interactive
      path automatically with no terminal attached or when assume-yes is passed, and write nothing and
      exit non-zero when the prompt is cancelled.
- [ ] 8.6 Implement the scaffold: version `1`, the selected `harnesses`, an empty `sources` object and
      an empty `assets` array, formatted for hand editing.
- [ ] 8.7 Write the manifest atomically through `jsonutil`, replacing an existing manifest in full when
      forced, leaving the previous file intact and no staging file behind on failure.
- [ ] 8.8 Implement overwrite protection: refuse an existing manifest, name the force flag in the
      refusal, leave the file untouched, and exit non-zero.
- [ ] 8.9 Print the created manifest path and the next command on success, with any remaining setup —
      ignoring installed paths, creating `.harnaas/`, populating the manifest — printed as guidance
      naming `harnaas install`, never performed and never behind a flag.
- [ ] 8.10 Add tests over temporary project directories: the scaffold decodes and validates under the
      strict loader unmodified; refusal and forced replacement; single and multiple detected harnesses;
      the fallback message; the harness flag overriding detection; an unsupported harness name writing
      nothing; a piped run completing without blocking; a cancelled prompt writing nothing.
- [ ] 8.11 Add the single-file side-effect test: after `init`, a pre-existing `.gitignore`, `AGENTS.md`
      and `CLAUDE.md` are byte-for-byte unchanged, none of them is created when absent, no harness
      directory and no `.harnaas/` exists, and `harnaas.json` is the only file that appeared.

## 9. Cross-cutting guardrail tests

- [ ] 9.1 Add a wiring test asserting the root declares no persistent flags, every registered command
      is reachable, and each command's `--json` availability matches what it declares.
- [ ] 9.2 Add a test proving the `--json` path writes a single parseable document to stdout while
      advisory text goes to stderr, driven through the shared helper.
- [ ] 9.3 Add an AST test over non-test sources asserting no call to the standard-library
      working-directory function and no use of cobra's `Print*` helpers, so the rule survives a lint
      suppression.
- [ ] 9.4 Add a test asserting logging writes only to the log file — nothing on stdout or stderr — and
      that reading a file with a sentinel string produces log records containing the path but not the
      sentinel.
- [ ] 9.5 Add a test harness that redirects per-user directories (home, cache, config) under
      `t.TempDir()` so no test can read or write real user state.
- [ ] 9.6 Add an end-to-end tagged test running the built binary: success exits `0`, a runtime failure
      exits `1`, no command in this change exits `2`, and an interrupt terminates the process by signal
      rather than by ordinary exit.

## 10. Verification

- [ ] 10.1 Run `mise run fmt`, then `mise run lint`, then `mise run test`, re-running lint after any
      formatting change, and confirm all three are clean.
- [ ] 10.2 Build the binary and confirm `harnaas --help` lists no persistent flags beyond cobra's
      built-ins, and that `--version` reports the stamped version on a goreleaser snapshot build and
      the build-information version on a plain `go build`.
- [ ] 10.3 From a nested subdirectory of a scratch project, run `harnaas init` and confirm the manifest
      lands at the repository root, that the printed path is the one created, and that no other file or
      directory appeared.
- [ ] 10.4 Confirm a piped, terminal-less `harnaas init` completes without blocking, and that
      cancelling the interactive run writes nothing and exits non-zero.
- [ ] 10.5 On a POSIX shell, interrupt a running command inside a `while true` loop and confirm the
      loop stops, demonstrating the process was killed by the signal rather than exiting normally.
- [ ] 10.6 Hand-edit a manifest to introduce several violations at once and confirm the loader reports
      all of them, deterministically, with each message naming the entry and the edit that fixes it.
- [ ] 10.7 Run `openspec validate add-harnaas-foundation --strict` and confirm the change's artifacts
      and all three delta specs pass.
