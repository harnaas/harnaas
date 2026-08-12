## 1. Record the decisions

- [x] 1.1 Write the architecture decision that a command is delivered through a harness's skill
      surface only where autonomous invocation can be disabled in that harness's own vocabulary,
      matching the existing records' shape — a declarative title, one lead paragraph fusing the
      problem and the decision, then the options rejected and the costs accepted. State that the
      alternative is a file that reports success while the harness remains free to start it.
- [x] 1.2 Note in that record what makes the refusal safe to defer: the precondition is exactly what a
      renderer speaking a harness's own skill vocabulary would later satisfy, so adding one turns an
      unsupported pairing into an emulated one and invalidates nothing written here.

## 2. Devin CLI on the harness roster

- [x] 2.1 Add the roster entry: the id `devin-cli`, the display name `Devin CLI`, the record that this
      harness has a per-user location, and the harness's own configuration directory at the project
      root as the sole evidence a project already uses it. Keep the roster ordered by id.
- [x] 2.2 Confirm the shared memory file is deliberately absent from that evidence, and say why in the
      entry's vicinity: most recognized harnesses read it, so its presence proves nothing about this
      one and would report the harness as detected in almost every project.
- [x] 2.3 Confirm the harness `init` falls back to when it detects none is unchanged, so a project
      showing evidence of nothing still scaffolds the manifest it scaffolded before.
- [x] 2.4 Confirm the roster package still holds only data — no behaviour, no package initialization,
      and no reach into the filesystem, the network or the environment.

## 3. The Devin CLI adapter

- [x] 3.1 Add the adapter package, registering itself under `devin-cli` from its own package
      initialization, and link it from the one file where linking adapters is decided and reviewed.
- [x] 3.2 Map the two surfaces this harness has: a rule beneath the harness directory's rules
      directory and a persona beneath its agents directory, each taking the asset id as the file stem
      and the ordinary markdown extension, each a live surface reproducing the source bytes exactly.
- [x] 3.3 Choose the flat persona spelling rather than the directory form the harness also reads, so
      the path the report names is a file a reader can open, and record the reason beside the mapping.
- [x] 3.4 Leave a skill and an instruction out of the surface table, and say why: both reach this
      harness through shared locations, and an adapter answering for them would be a second place
      harnaas decides where they land.
- [x] 3.5 Answer the harness's own directory for project scope and offer no root for any other scope,
      recording that this harness keeps its per-user rules and its per-user everything-else under two
      different directories and spells the second differently per platform, so there is no single root
      a destination could be counted from.
- [x] 3.6 Detect the harness by reading the roster's declared evidence rather than a second list kept
      beside it, without following a symbolic link, so that scaffolding a manifest and reporting an
      install can never disagree about one project.
- [x] 3.7 Confirm the new package stays inside the adapter import boundary — the contract, the roster
      and the manifest vocabulary, and nothing else — without widening the allowlist.
- [x] 3.8 Confirm this harness is absent from the table of harnesses known not to read the shared
      skills directory, and record that the absence is the finding rather than an omission.

## 4. The command-emulation precondition

- [x] 4.1 Replace the condition under which a command is delivered through a harness's skill surface:
      ask whether the harness can be told not to start the skill on its own initiative, rather than
      whether its adapter declares a skill surface. Note that the previous condition was unsatisfiable
      by any shipped adapter, so nothing installed today changes.
- [x] 4.2 Report a command that cannot be delivered as unsupported, naming the harness, saying that it
      has no command surface, and saying that delivering it as a skill would leave the harness free to
      start it unprompted. Never claim a harness has no skill surface where it has one.
- [x] 4.3 Keep the refusal per pairing: the asset's other targets still install, the run still
      succeeds, and the outcome is never recorded as emulated.
- [x] 4.4 Confirm the harness that has a command surface is unaffected — it installs natively, the
      emulation path is never entered for it, and its reported outcome is a plain install.
- [x] 4.5 While in that code, remove or justify the emulation flag carried on a planned target that
      nothing reads: the reported outcome is taken from what the renderer produced, and a second
      source for one fact is how the two answers start to disagree.
- [x] 4.6 Stop the report naming a destination for a pairing that wrote nothing. An asset with no
      destination of its own is an instruction, which lands in the memory file and is usefully named
      by it; an unsupported pairing has no destination because nothing was written, and borrowing
      the instruction's answer prints an arrow at a file harnaas did not touch. Found while
      verifying a refused command by hand.

## 5. Documentation

- [x] 5.1 Update the README's statement of which harness ids harnaas recognizes, and its detection
      evidence table, to carry both harnesses.
- [x] 5.2 Update the README's "where assets land" section: it currently states that version 1 ships
      exactly one named adapter above a destination table with one harness in it. Give the per-harness
      destinations per harness, and say which types each harness has no surface for.
- [x] 5.3 Update the README's account of command emulation to state the precondition and what is
      reported when it does not hold.
- [x] 5.4 Update the repository's own conventions document where it describes the roster and the one
      named adapter, including the sentence recording that every harness on today's roster has a
      per-user location — which this change is the first to complicate.
- [x] 5.5 Update the adapter package's own documentation, which also states that version 1 ships
      exactly one adapter, and name the second one there.
- [x] 5.6 State in the manifest reference which harness ids are recognized, so an author reading only
      that section learns of the second harness.

## 6. Tests

- [x] 6.1 Assert the roster stays ordered by id with a second entry present, and that every entry
      still declares a display name and at least one piece of project evidence.
- [x] 6.2 Assert the new id is recognized, that its display name is not accepted where an id belongs,
      and that an unknown-harness diagnostic lists both recognized ids.
- [x] 6.3 Assert detection: the harness's configuration directory is evidence, the shared memory file
      alone is not, a symbolic link is not followed, an absent harness is an absence rather than a
      failure, and detection creates nothing.
- [x] 6.4 Assert that a project showing evidence of both harnesses scaffolds both, in roster order,
      and that a project showing evidence of neither still scaffolds the fallback.
- [x] 6.5 Assert the destination mapping per type, that both a skill and an instruction have no
      surface on this adapter, and that one relative destination serves whichever scope resolves.
- [x] 6.6 Assert that this adapter offers a root for project scope and none for any other, and that a
      user-scoped rule or persona targeting it is refused naming the asset, the harness and the scope,
      with nothing written.
- [x] 6.7 Assert that a user-scoped skill targeting this harness installs to the shared per-user
      skills directory, so the roster's answer and the adapter's answer are exercised as the two
      different questions they are.
- [x] 6.8 Extend the assertion that pins the set of registered adapters, which is compared whole so
      that both losing one and gaining one fail rather than pass quietly.
- [x] 6.9 Assert one shared skill write for an asset targeting both harnesses, with no copy beneath
      either harness's own directory, and assert the shared file survives one harness being removed
      from the asset's targets while the other still claims it.
- [x] 6.10 Assert an instruction targeting only the new harness reaches the memory file's managed
      block, that nothing is written beneath the harness's directory for it, and that the existing
      bridge line behaves exactly as it did before.
- [x] 6.11 Assert a command targeting only the new harness is reported unsupported with its reason and
      writes nothing, and that a command targeting both installs natively for one while being refused
      for the other in the same run.
- [x] 6.12 Assert a rule and a persona install beneath the harness's directory byte for byte,
      including frontmatter keys the harness does not read, with no key added, removed or rewritten.
- [x] 6.13 Assert lint over a project using the new harness reports drift, an absent destination and an
      unmanaged conflict exactly as it does for the existing one, and that a project never naming the
      new harness produces the findings it produced before.
- [x] 6.14 Assert the new adapter package satisfies the import boundary and the self-registration
      check, which discover it from the directory rather than from a list.

## 7. Verification

- [x] 7.1 Run `mise run fmt`, then `mise run lint`, then `mise run test`, re-running lint after any
      formatting change.
- [x] 7.2 In a scratch project holding only the new harness's configuration directory, confirm `init`
      reports the harness as detected and scaffolds a manifest targeting it.
- [x] 7.3 In that project, install one asset of each type targeting the new harness and confirm by
      hand where each landed: the rule and the persona beneath the harness's directory, the skill in
      the shared skills directory once, the instruction in the memory file's managed block, and the
      command reported unsupported with nothing written.
- [x] 7.4 Confirm the lockfile records the harness on each installation, that the shared skill carries
      both harnesses when both are targeted, and that removing one target leaves the shared file in
      place.
- [x] 7.5 Confirm a user-scoped skill installs beneath the home directory while a user-scoped rule for
      the same harness is refused, and that the refusal names the asset, the harness and the scope.
- [x] 7.6 Run `harnaas lint` over that project and confirm it passes, then edit one installed file and
      confirm the drift finding names it and the command that repairs it.
- [x] 7.7 Run `openspec validate add-devin-cli-harness --strict` and confirm the change is valid.
