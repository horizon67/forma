# Alpha quickstart fresh-repository dogfood — 2026-08-23

Status: first end-to-end evidence for the alpha thin runner, followed by two
controlled reruns of the page-access finding. This proves that the current path
can create and run an ordinary application and records where the Generation
Request and its implementation instructions each matter. It is not release
qualification and it does not prove that every Acceptance Fact was implemented.

## Question

Can a source-built Forma binary compile the bundled quickstart into a canonical
Generation Request, hand that request to Codex, and leave a reviewable, runnable
application in a fresh Git repository without Forma executing the generated
program on the host?

## Inputs

- Forma repository HEAD before the uncommitted alpha work:
  `43ed48b3ef21584d2d222104aaa77a880631c327`
- Forma binary: `forma devel 43ed48b dirty`
- source: `docs/examples/alpha-quickstart.forma`
- source SHA-256:
  `f8ae085e9227637973ec36ae7ed63a82bda4d41d65a339256a2b3ea149d02799`
- Generation Request schema: `forma/generation-request/v0alpha4`
- Generation Request: 217 Acceptance Facts, 0 human Review Requirements
- Generation Request SHA-256:
  `37d81e19e14c7f185bdeba48508b5c339e3ac7b9cd4a47129f7c8831ce622c3e`
- Codex CLI: `codex-cli 0.144.4`, saved ChatGPT login
- target: a new empty Git repository with one initial commit
  (`a5497a7b3477f3c3eebae9ea81627ee4c071a1de`)

The run used the public command:

```sh
forma generate --repository "$fresh_repository" \
  docs/examples/alpha-quickstart.forma
```

## Observed result

`forma generate` exited 0 after approximately 2.5 minutes. It left these seven
untracked paths and stopped for human review:

```text
app.js
domain.js
index.html
package.json
server.js
styles.css
test/
```

The generated result was a dependency-free Node.js single-page project/task
application. The independent human-side checks, run only after reviewing the
files, passed:

```text
npm test             6 passed, 0 failed
node --check app.js  passed
node --check server.js
                     passed
GET /                200 text/html
GET /app.js           200 text/javascript
```

Browser inspection confirmed the welcome page, projects and tasks navigation,
role selection, and a project search that reduced the two seeded rows to the
matching `ATLAS-01` row. Forma itself did not start the server or run these
commands; the dogfood operator did so after the generated diff was available.

## Findings

### Fixed after the run: a page-only surface had no access Fact

The quickstart declares:

```forma
page Welcome {
    allow manager, member
    continue Projects
}
```

The Resolved Intent retains `allows: [manager, member]`, but the 217 canonical
Facts contain only the `continue` navigation Fact for `page/Welcome`. There is
no page access Fact because the current builder derives page access through an
entity view, and this page has only a surface transition.

The generated application consequently checks access after its special
`welcome` branch. Browser inspection reproduced the result: after selecting
`Signed out`, `#/projects` was denied but direct navigation to `#/welcome`
rendered the page. The generated domain access unit test passed because it did
not observe this page boundary.

This was a compiler Fact-completeness gap, not merely a styling or target test
issue. The compiler now emits canonical allowed/denied Facts owned by a
role-restricted page when no view or Identity interaction already owns the
surface. Its validator rejects a missing or altered Fact. The same source now
produces 220 Facts, including `manager` / `member` allowed and `anonymous`
denied at `page/Welcome`. The reruns below distinguish the compiler fix from
the implementation-agent translation rule.

## Page-access reruns

Both reruns used Forma commit `ac4e516`, the same quickstart source SHA-256
shown above, Generation Request schema `forma/generation-request/v0alpha4`,
220 Acceptance Facts, and 0 human Review Requirements. R2 used
`forma devel ac4e516` and target initial commit
`46396b76b77cd506f3d07a0d7368aad4e23f0128`; R3 used
`forma devel ac4e516 dirty` for the prompt change and target initial commit
`60a2e7a9edba4de3fd9c5e738b1a2efd278f38b8`. The canonical request SHA-256
was identical in both runs:

```text
ab69fd3d8428007f9de4efa75506954d178abc724d08925cd0e7fe81b6cb28f9
```

The three new Facts require `manager` and `member` to be allowed and an
anonymous principal to be denied at `page/Welcome`, with authoritative
enforcement.

### R2: the Fact alone was insufficient for this agent run

The first rerun used those Facts with the original general implementation
prompt. Codex produced nine passing repository tests, but the relevant test
called a pure `canAccess` role helper rather than the public page boundary. The
client also converted missing role state into `manager`:

```text
storedRole:null -> selectedRole manager, denied false, welcome true
storedRole:manager -> denied false, welcome true
storedRole:member -> denied false, welcome true
storedRole:anonymous -> denied true, welcome false
```

This was measured independently against the generated client. Direct
`#/welcome` navigation with no stored role still rendered Welcome. The request
was complete, but the implementation agent had treated authoritative access as
a helper-level role decision and had invented an allowed default identity.

### R3: general translation rules reached the public boundary

The implementation prompt was then clarified without changing the Forma
source, request, or any Welcome-specific rule. It now says that an access Fact
whose expected enforcement is authoritative must be implemented and tested at
the public boundary that presents or invokes its subject, and that missing
identity/session/role state must not be converted into an allowed default role.

Codex generated an HTTP boundary that requires both identity and an allowed
role before every API operation, plus a client that shows login before routing
to Welcome. Host-side validation, run after reviewing the generated files,
observed:

```text
npm test                                      11 passed, 0 failed
GET /api/projects (no identity)               403
GET /api/projects (manager role, no identity) 403
GET /api/projects (member identity)           200
GET /api/projects (manager identity)          200
```

Browser inspection confirmed that direct anonymous `#/welcome` navigation
showed login, while authenticated member and manager sessions both reached
Welcome. The generated login button initially omitted `type="submit"`, which
caused its own client binding code to fail. Its original `public/app.js`
SHA-256 was
`ee91a808800eb5b6812bdf0db6e53e740ea35eaa835102e03d5c479f248b8e90`.
After a reviewed one-line target repair, both authenticated browser paths
worked. This was a generated-code defect, not a Forma semantic change.

The generated server tests also skipped their HTTP assertions when sandbox
socket binding returned `EPERM`. Running them on the host proved that sockets
were actually opened and the 11 tests passed; the skip remains evidence that
generated tests themselves require review. The runner now prints the SHA-256
of the exact implementation prompt on every Codex execution and explicitly
asks the reviewer to confirm that boundary assertions ran rather than relying
on a green test summary.

### Language/review gap: relation deletion policy is unspecified

The generated `deleteProject` operation also deletes every related Task. Forma
declares `Task.project Project required`, but the alpha contract does not say
whether deleting the Project must be rejected, cascade, detach, or be delegated
to a target-specific policy. Codex therefore invented destructive cascade
behavior even though the runner prompt tells it not to invent absent product
requirements.

The alpha must not silently present that choice as Forma-guaranteed semantics.
The cheapest pre-release response is to call it out in known limitations and
human review guidance. A later language decision may add an explicit deletion
policy or a Review Requirement; it is not encoded as a growing list of
case-specific runner prompt rules.

### Generated-code review findings

- The generated tests prove the role function but not the page/handler access
  boundary described above. The runner instruction was tightened after this
  run to require surface Facts to be tested at their named boundary rather
  than only through a lower-layer helper.
- The small development server joins the request path to the process working
  directory without an explicit containment check. It is suitable only as a
  reviewed local sample, not a production server.
- Codex progress was captured until completion, so the terminal showed no live
  progress for most of the 2.5-minute run. This is a usability gap, not a
  semantic blocker.

## Decision

The thin-runner architecture is sufficient for the alpha goal: a user can go
from Forma source through a canonical request to a normal, runnable application
and then review and test it. The run also demonstrates why the alpha must retain
human review and why dogfood precedes further language expansion.

The page-only access Fact gap is closed in the compiler. The reruns show that
the Fact was necessary but, for this model run, was not sufficient without a
general translation rule for authoritative access and anonymous principals.
No additional Fact family or Welcome-specific prompt rule is needed: the same
220-Fact request reached the HTTP and browser boundaries after that general
clarification.

The one-line client repair and vacuous sandbox skip also show why alpha stops
for human review instead of claiming one-shot generated-code correctness. Keep
cascade behavior and generated-server hardening visible in the alpha
limitations/review guide rather than expanding the language spec without more
application evidence. The page-access blocker is now closed; clean-environment
qualification and release gates remain before tagging.
