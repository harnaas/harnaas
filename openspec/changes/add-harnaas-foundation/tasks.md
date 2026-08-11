## 1. Repository and toolchain scaffolding

- [ ] 1.1 Initialize the Go module `github.com/harnaas/harnaas` and create the `cmd/harnaas/`,
      `cmd/harnaas/cli/`, and `internal/` directories.
- [ ] 1.2 Add `mise.toml` pinning the Go and `golangci-lint` versions, with `gotestsum` installed as
      a post-install step; keep the Go version identical to the `go` directive in `go.mod`.
- [ ] 1.3 Add `mise-tasks/` scripts for `build`, `fmt`, `lint`, `test`, and a `check` task that
      chains fmt, lint and test; keep inline `run` blocks in `mise.toml` under three lines.
- [ ] 1.4 Add `.golangci.yaml` on the v2 schema with the standard set plus the extended linter list,
      `nolintlint` requiring an explanation and a specific linter, and `run.build-tags` covering the
      `integration` and `e2e` tags.
- [ ] 1.5 Add the `forbidigo` rules banning the standard-library working-directory call and cobra's
      `Print*` helpers, each with a message naming the sanctioned replacement.
- [ ] 1.6 Add `.goreleaser.yaml` building `cmd/harnaas` for darwin, linux and windows on amd64 and
      arm64 with CGO disabled, stamping version and commit via ldflags.
- [ ] 1.7 Add GitHub Actions workflows for test and lint with every action SHA-pinned, and a single
      aggregator job as the required status check.
- [ ] 1.8 Add the license allowlist and the `lint:licenses` task.
- [ ] 1.9 Write `CLAUDE.md` recording the imported architecture rules, and point `AGENTS.md` at it.

## 2. Process entrypoint and command tree

- [ ] 2.1 Add `internal/procsignal` recording which signal initiated shutdown.
- [ ] 2.2 Write `cmd/harnaas/main.go`: load version info, build the cancellable root context, install
      the two-stage signal handler that prints the force-quit notice and force-exits on a second
      signal.
- [ ] 2.3 Implement signal-faithful termination that re-raises the original signal and falls back to
      a `128`-plus-signal-number exit where re-raising is unsupported.
- [ ] 2.4 Add the `SilentError` type with `Unwrap`, and the entrypoint error switch covering
      already-printed errors, unknown command or flag, positional-argument errors, and the default
      case.
- [ ] 2.5 Implement the exit-code contract: `0` success, `1` runtime failure, `2` reserved for
      findings, `128`+signal for signals.
- [ ] 2.6 Build the root command with cobra error and usage printing silenced, no persistent flags,
      help groups declared in display order, and the version template.
- [ ] 2.7 Add the shared `addJSONFlag` helper registering `--json` locally, plus its read helper that
      treats a missing flag as not requested.

## 3. Foundation packages

- [ ] 3.1 Add `versioninfo` with build-time stamped variables and a loader that falls back to the
      embedded build information, letting an explicit stamp win.
- [ ] 3.2 Add `paths` resolving the project root from the enclosing repository, plus the context
      carrier and accessor used in place of reading the process working directory.
- [ ] 3.3 Add `logging` over `log/slog` writing structured records to a log file, with context
      helpers for component and command, and a documented rule that user content must not be logged.
- [ ] 3.4 Add `palette` as a zero-dependency package of base16 slot constants with semantic aliases
      and no body-text colour.
- [ ] 3.5 Add `uiform` wrapping the form library with the accessible-mode wrapper and a shared theme,
      and `interactive` resolving whether prompting is possible.
- [ ] 3.6 Add `jsonutil` with atomic file writing and indented marshalling that emits a trailing
      newline and does not escape HTML.

## 4. Manifest format

- [ ] 4.1 Define the `harnaas.json` document types: version, harness list, and asset entries with id,
      type, source, targets and scope.
- [ ] 4.2 Implement strict decoding that rejects unknown fields and reports the offending field.
- [ ] 4.3 Implement version handling: accept version `1`, refuse a newer version with upgrade
      guidance, reject a missing version.
- [ ] 4.4 Implement source normalization from the `github:` and `local:` string shorthands into the
      canonical object form, including the default-branch fallback when no ref is given.
- [ ] 4.5 Implement local-source containment so a path must be relative and must resolve inside
      `.harnaas`, rejecting absolute paths and upward traversal.
- [ ] 4.6 Implement target and scope defaulting, rejecting an asset whose effective target list is
      empty.
- [ ] 4.7 Implement semantic validation collecting every violation: duplicate ids, ids that are not a
      safe single path segment, unknown asset types, and unknown harness names.
- [ ] 4.8 Implement manifest discovery from the context-carried project root, with a missing-manifest
      error that names `harnaas init`.

## 5. Init command

- [ ] 5.1 Add `init.go` with the command constructor, its force, assume-yes and harness flags.
- [ ] 5.2 Implement harness detection based on observable project evidence, falling back to the
      default harness and explaining the fallback.
- [ ] 5.3 Implement the scaffold writer producing a human-formatted manifest that the loader accepts
      unmodified, written atomically.
- [ ] 5.4 Implement overwrite protection that names the force flag in its refusal, and full
      replacement when forced.
- [ ] 5.5 Implement the accessible interactive selection, the flag-driven equivalent, automatic
      selection of the non-interactive path when no terminal is attached, and writing nothing when
      the prompt is cancelled.
- [ ] 5.6 Print the created path and the next command on success.

## 6. Tests

- [ ] 6.1 Add unit tests for manifest decoding, shorthand normalization, defaulting, containment and
      validation, table-driven and parallel.
- [ ] 6.2 Add tests asserting strict decoding rejects an unknown field and that a newer version
      produces the upgrade message.
- [ ] 6.3 Add `init` tests over temporary project directories covering scaffold validity, overwrite
      refusal and forced replacement, detection and fallback, and the non-interactive path.
- [ ] 6.4 Add a test asserting `init` leaves an existing ignore file byte-for-byte unchanged and
      creates no directories.
- [ ] 6.5 Add a wiring test asserting the root command declares no persistent flags and that every
      registered command is reachable.
- [ ] 6.6 Add tests for the entrypoint error switch and the exit-code mapping, including the
      signal-derived codes.
- [ ] 6.7 Add a test isolating per-user directories under test so no test can touch real user state.

## 7. Verification

- [ ] 7.1 Run `mise run fmt`, then `mise run lint`, then `mise run test`, re-running lint after any
      formatting change.
- [ ] 7.2 Build the binary and confirm `harnaas --help`, `harnaas --version` and `harnaas init`
      behave as specified from a subdirectory of a test project.
- [ ] 7.3 Confirm a piped, terminal-less `harnaas init` completes without blocking.
