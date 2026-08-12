## Purpose

Defines when a `command` may reach a harness through that harness's skill surface. A command is
invoked deliberately by a user and a skill is started by the harness when a description matches, so
delivering one as the other is sound only where harnaas can also say, in that harness's own
frontmatter vocabulary, that it must not be started unprompted. Covers that precondition, what is
reported when it does not hold, and the rule that a refusal names the surface that is actually
missing.

## ADDED Requirements

### Requirement: Emulating A Command As A Skill Requires Suppressible Autonomous Invocation

harnaas SHALL deliver a `command` through a harness's skill surface only where it can also write, in
that harness's own frontmatter vocabulary, the setting that stops the harness from starting the skill
on its own initiative. Where a harness has a skill surface but no such setting, the pairing MUST be
reported unsupported and nothing SHALL be written, because a delivered command the harness may start
unprompted is the outcome the `command` type exists to prevent, reached through a file that reports
success. Where the setting exists, the command SHALL be delivered with it applied, and the
installation SHALL be reported as emulated rather than as a plain install.

#### Scenario: A harness whose skill frontmatter has no suppression setting

- **WHEN** a `command` asset targets a harness with a skill surface harnaas cannot tell to stop
  invoking a skill on its own initiative
- **THEN** the pairing is reported unsupported, nothing is written, and the outcome is never recorded
  as emulated

#### Scenario: A harness with no skill surface at all

- **WHEN** a `command` asset targets a harness that has neither a command surface nor a skill surface
- **THEN** the pairing is reported unsupported and nothing is written

#### Scenario: A harness that can be silenced takes the command as a skill

- **WHEN** a `command` asset targets a harness with no command surface whose skill frontmatter can
  disable autonomous invocation
- **THEN** it is installed as a skill with that setting applied and the outcome is reported as
  emulated

### Requirement: An Unsupported Command Names Why It Cannot Be Emulated

The diagnostic for a `command` that cannot reach a harness SHALL state the problem and the exact edit
that resolves it, and SHALL name the surface that is actually missing. It MUST NOT report that a
harness has no skill surface where the harness has one, because a diagnostic that misstates the
problem sends its reader to check something that is not wrong. Every other target of the same asset
SHALL still install, and the run MUST NOT fail because one pairing was refused.

#### Scenario: The reason names the real obstacle

- **WHEN** a `command` targets a harness whose skill surface exists but cannot be silenced
- **THEN** the reported reason says the harness has no command surface and that delivering it as a
  skill would leave the harness free to start it unprompted

#### Scenario: The other targets still install

- **WHEN** a `command` asset targets one harness that refuses it and another that installs it
  natively
- **THEN** the native installation completes, the refusal is reported beside it, and the run succeeds
