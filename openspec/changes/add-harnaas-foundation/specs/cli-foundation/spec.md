## Purpose

Defines the process-level contract every `harnaas` command inherits: how the command tree is built,
how flags are scoped, how errors reach the user, what the exit codes mean, how interrupts terminate
the process, and how user output is kept apart from diagnostic logging. It exists so command
behaviour stays uniform and so people, scripts and CI can drive the CLI without a terminal.

## ADDED Requirements

### Requirement: Command Tree Construction

The CLI SHALL expose a single root command named `harnaas` that suppresses cobra's own error and
usage printing, so the process entrypoint is the only component that renders errors. Subcommands
SHALL be constructed by dedicated constructor functions and attached to the root explicitly; command
registration MUST NOT occur as a package initialization side effect.

#### Scenario: Root command suppresses cobra error output

- **WHEN** a subcommand returns an error
- **THEN** cobra prints neither the error nor the usage block, and the entrypoint renders the error
  exactly once

#### Scenario: Unknown subcommand shows usage

- **WHEN** the user invokes `harnaas` with an unrecognized subcommand or flag
- **THEN** the CLI prints the error together with the root command's usage, and exits non-zero

#### Scenario: Wrong argument count shows the failing subcommand's usage

- **WHEN** a subcommand is invoked with the wrong number of positional arguments
- **THEN** the usage block shown is that of the deepest matched subcommand, not the root command

### Requirement: Flag Scoping Policy

The root command MUST NOT declare any persistent flags. A flag that applies to only some commands
SHALL be registered locally on each command that honours it, so a command can never accept a flag it
silently ignores. Machine-readable output SHALL be requested through a local `--json` flag declared
by each command that supports it, never through a shared global flag.

#### Scenario: No global flags exist on the root

- **WHEN** the user runs `harnaas --help`
- **THEN** the help output lists no persistent flags beyond cobra's built-in `--help` and `--version`

#### Scenario: Unsupported --json is rejected

- **WHEN** the user passes `--json` to a command that does not declare it
- **THEN** the CLI reports an unknown flag error and exits non-zero rather than ignoring the flag

#### Scenario: A locally declared flag is honoured

- **WHEN** the user passes `--json` to a command that declares it
- **THEN** that command emits its machine-readable form, and the flag remains absent from every
  command that does not declare it

### Requirement: Error Rendering Contract

A command that has already printed a friendly, user-facing explanation of a failure SHALL return an
error marked as already-printed; every other failure SHALL be returned unwrapped for the entrypoint
to print to stderr. The entrypoint MUST NOT print an already-printed error a second time, MUST be the
only component that maps an error to an exit code, and an already-printed error MUST remain
unwrappable so callers can still inspect its cause.

#### Scenario: Friendly message is not duplicated

- **WHEN** a command prints a formatted explanation and returns an already-printed error
- **THEN** the entrypoint prints nothing further and the process still exits non-zero

#### Scenario: Plain error is printed once by the entrypoint

- **WHEN** a command returns an ordinary error without printing anything
- **THEN** the entrypoint writes that error's message to stderr exactly once

### Requirement: Exit Code Contract

The CLI SHALL exit `0` on success and `1` on any runtime failure. Exit code `2` SHALL be reserved for
lint findings specifically: `lint` completing its checks and reporting at least one error-severity
finding. No other command SHALL exit `2`, and a lint run that fails to complete SHALL exit `1` like
any other runtime failure. Termination by signal SHALL exit `128` plus the signal number. Any further
exit code MUST NOT be introduced without a documented meaning.

#### Scenario: Successful run exits zero

- **WHEN** a command completes without error
- **THEN** the process exits with status `0`

#### Scenario: Runtime failure exits one

- **WHEN** a command fails because of invalid configuration, an I/O error, or a network error
- **THEN** the process exits with status `1`

#### Scenario: Lint findings exit two

- **WHEN** `lint` runs its checks to completion and reports one or more error-severity findings
- **THEN** the process exits with status `2`, distinct from both success and runtime failure

#### Scenario: A lint run that cannot complete exits one

- **WHEN** `lint` itself fails partway through rather than reporting findings
- **THEN** the process exits with status `1`, so a broken run is never mistaken for findings

#### Scenario: Non-lint commands never exit two

- **WHEN** any command other than `lint` finishes, whether successfully or not
- **THEN** its exit status is `0`, `1`, or a signal status, and never `2`

### Requirement: Interrupt And Termination Handling

The first interrupt or termination signal SHALL cancel the root context so in-flight work unwinds,
and SHALL print a notice telling the user that signalling again forces an exit. A second signal SHALL
force termination. On exit the CLI SHALL re-raise the original signal to itself rather than calling a
plain process exit, falling back to a `128`-plus-signal-number exit only where re-raising is
unsupported.

#### Scenario: First interrupt cancels work

- **WHEN** the user sends an interrupt during a long-running command
- **THEN** the root context is cancelled, in-flight work unwinds, and a notice explains that
  signalling again forces a quit

#### Scenario: Second interrupt forces exit

- **WHEN** the user sends a second interrupt while shutdown is still in progress
- **THEN** the process terminates immediately without waiting for the in-flight work

#### Scenario: Enclosing shell loop is broken by interrupt

- **WHEN** the CLI is interrupted while running inside a shell loop
- **THEN** the process reports termination by that signal, so the enclosing loop stops rather than
  respawning the CLI

#### Scenario: Termination signal is distinguishable from interrupt

- **WHEN** the process is terminated by a termination signal rather than an interrupt
- **THEN** the exit status reflects that signal, not the interrupt signal

### Requirement: Version Reporting

The CLI SHALL report a version and commit stamped into the binary at release-build time. Where no
stamp is present, it SHALL fall back to the module version and revision recorded in the embedded
build information, and SHALL report a development placeholder only when neither source is available.
An explicit build-time stamp MUST take precedence over the embedded build information.

#### Scenario: Released binary reports its stamped version

- **WHEN** the user runs `harnaas --version` on a released build
- **THEN** the stamped version and commit are printed

#### Scenario: Installed-from-source binary reports the module version

- **WHEN** the binary was built without version stamping but carries module build information
- **THEN** the reported version comes from that build information rather than the development
  placeholder

### Requirement: Project Root Resolution

Commands SHALL resolve the project root once from the enclosing repository and carry it in the
request `context.Context`, rather than reading the process working directory at each use site. Code
MUST NOT call the standard library's working-directory function for path resolution, and the
repository's static-analysis configuration SHALL fail the build when it does, naming the replacement.

#### Scenario: Command works from a subdirectory

- **WHEN** the user runs a command from a nested subdirectory of the project
- **THEN** the project root resolves to the repository root and every relative path resolves against
  it, producing the same result as running from the root

#### Scenario: Lint rejects direct working-directory reads

- **WHEN** code calls the standard library's working-directory function for path resolution
- **THEN** static analysis fails with a message naming the context-carried project root to use
  instead

#### Scenario: No project root available

- **WHEN** a command that requires a project is run outside any repository and no root can be
  determined
- **THEN** the CLI reports that no project root was found and exits non-zero

### Requirement: Output Stream Discipline

User-facing output SHALL be written to the command's own output stream and never through cobra's
print helpers, which route to stderr. When a command runs with `--json`, the JSON document SHALL be
the only thing written to standard output; advisory, progress and warning text SHALL go to standard
error, so a machine consumer can parse standard output whole.

#### Scenario: Machine-readable output is not polluted

- **WHEN** a command runs with `--json` and the CLI also has advisory information to convey
- **THEN** the JSON document is the only thing written to stdout and the advisory text goes to stderr

#### Scenario: Human output goes to stdout

- **WHEN** a command reports its result in human-readable form
- **THEN** the text is written to the command's configured output stream, so tests and pipes capture
  it without stderr redirection

### Requirement: Diagnostic Logging Privacy

Diagnostic logging SHALL be structured and written to a log file rather than the terminal, so it
never interleaves with user output. Log records MUST NOT contain user content — file contents, memory
or prompt text, captured command output, or credentials. Identifiers, paths, durations, counts and
status values are permitted.

#### Scenario: Logs exclude user content

- **WHEN** a command reads a file and logs its progress
- **THEN** the log records the identifier, the path and the outcome, and never the file's contents

#### Scenario: Logging does not disturb the terminal

- **WHEN** a command emits log records while producing user output
- **THEN** the records go to the log file only, and neither stdout nor stderr shows them

### Requirement: Non-Interactive Operation

Every workflow SHALL be completable from a non-interactive terminal. Information the user needs MUST
NOT be reachable only through a prompt, picker, wizard or full-screen interface. Where a command
prompts interactively, it SHALL provide an equivalent flag-driven path, and SHALL detect a
non-interactive environment and take that path automatically rather than blocking on input.

#### Scenario: Piped invocation does not block

- **WHEN** a command that normally prompts is run with its output piped and no terminal attached
- **THEN** it completes using flag-supplied or default values without waiting for input

#### Scenario: Every prompt has a flag equivalent

- **WHEN** a command asks the user to choose or confirm something interactively
- **THEN** the same choice is expressible with a command-line flag on that command

#### Scenario: Nothing is exclusive to a full-screen interface

- **WHEN** a workflow is driven entirely by flags
- **THEN** every result and message the interactive path would have shown is also emitted on the
  non-interactive path

### Requirement: Terminal Presentation

Interactive prompts SHALL render in an accessible, screen-reader-friendly mode when the environment
requests it. Styled output SHALL draw colors only from the terminal's own base palette so it respects
the user's theme, and body text MUST be left unstyled rather than pinned to a specific foreground
color, so it stays legible on both light and dark backgrounds.

#### Scenario: Accessible mode is honoured

- **WHEN** the accessibility environment variable is set and a command shows a prompt
- **THEN** the prompt renders in its accessible, non-graphical form

#### Scenario: Body text stays readable on a light terminal

- **WHEN** output is rendered in a terminal with a light background
- **THEN** body text uses the terminal's default foreground and remains visible
