# Alpha quickstart fresh-repository dogfood — 2026-08-23

Status: first end-to-end evidence for the alpha thin runner. This proves that
the current path can create and run an ordinary application. It is not release
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
denied at `page/Welcome`. The same dogfood must be rerun to confirm the
generated page boundary before tagging.

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

The page-only access Fact gap is closed in the compiler; repeat this run before
tagging to confirm Codex implements and tests the new Fact at the page boundary.
Keep cascade behavior and generated-server hardening visible in the alpha
limitations/review guide rather than expanding the language spec without more
application evidence. Treat live progress as an alpha UX improvement after
correctness and release gates.
