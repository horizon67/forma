# Forma

**en** / [ja](README.ja.md)

**A language for specifying what to build precisely and leaving how to build it to AI.**

Forma is an early experimental programming language for expressing the
application specification given to coding agents as typed, checkable, and
reviewable source rather than leaving it only in natural language.

The project is testing this hypothesis through both language design and
end-to-end experiments:

> If humans decide application meaning in Forma and a compiler removes its
> ambiguity, AI can safely implement and update that meaning in an ordinary
> software repository.

Forma is not yet a production-ready compiler or a complete ecosystem.

## Why Forma is needed

AI coding agents can already turn requests like this into application code:

> Add a user list with search by name and email and a page size of 20.

Natural-language prompts, however, are weak as durable application source.
Field names and action references cannot be resolved mechanically. Type and
state-transition contradictions cannot be checked before implementation. It is
hard to tell an intentional omission from a missing requirement, and another
agent or a later session may interpret the same prose as different behavior.

### Why now?

AI-assisted development is reducing the amount of framework-specific code that
humans write line by line. At the same time, continually inspecting every
generated frontend, backend, database, and test change is not practical. This
creates a need for a layer between concise, meaning-dense source maintained by
humans and ordinary repository code maintained with AI.

### Relationship to Spec-Driven Development

Spec-Driven Development (SDD) is a useful way to state requirements before
implementation and reduce misunderstanding between humans and coding agents
through specifications, plans, and tasks. Forma does not reject SDD.

When authoritative application behavior remains primarily natural-language
Markdown, however, several problems remain:

- field-name typos and broken action references cannot be rejected reliably before implementation;
- type, state-transition, permission, and constraint consistency remains interpretive;
- specifications, code, and tests must be reread and synchronized after changes;
- it is difficult to verify mechanically which requirements are covered by tests.

If SDD solves the problem of starting implementation without a specification,
Forma makes the core of that specification parseable, type-checkable,
reference-resolved, and coverage-checkable. Markdown remains useful for context,
design rationale, and non-functional requirements. Forma fixes application
structure and behavior.

## Forma's answer

The earlier request can be written in Forma as follows:

```forma
type Email = String matches /.+@.+/

entity User {
    name  String required
    email Email required unique

    state status Pending | Confirmed | Active | Suspended initial Pending
}

action User.activate: Confirmed -> Active

page Users {
    allow admin

    list User {
        columns name, email, status
        search name, email
        filter status
        sort name asc
        paginate 20
        actions activate
    }
}
```

Before a coding agent changes the repository, the compiler can catch misspelled
fields, incompatible types, invalid state transitions, unresolved actions, and
inconsistent permissions. References and defaults are resolved deterministically,
giving humans and AI the same application intent.

At the same time, `list User` means only “present a collection of users.” It
does not prescribe an HTML table, React component, endpoint, or query builder.
The coding agent reads the target repository and chooses an implementation that
fits its architecture.

## How an application is built

AI generation is not an optional backend for Forma. It is the central execution
model.

```text
Forma source
  → Go front end: format / parse / resolve / type check / semantic check
  → Resolved Intent + Acceptance Facts
  → Generation Request
  → AI coding agent + target repository
  → ordinary application code
  → build / test
  → feedback to the agent
```

The Forma front end stops after deciding application meaning. It does not lower
that meaning directly into framework-specific files. The coding agent receives
a machine-readable request and implements it using the architecture, libraries,
conventions, and tests in the actual repository.

> A conventional DSL builds a code generator. Forma builds stronger input for
> the AI that writes the code.

See [Agent Generation Model](docs/agent-generation.md) for the detailed
responsibility boundary and request/feedback loop.

### What to build and what to build it with

Generation separates three inputs explicitly:

| Input | Decision it owns |
| --- | --- |
| `*.forma` | **What to build:** entities, states, actions, pages, and permissions |
| Implementation Policy Manifest | **What to build it with:** frameworks, libraries, and prohibitions |
| Target repository | **What exists now:** code, dependencies, architecture, and build/test commands |

For example, Forma may require a user list searchable by name, the manifest may
specify Ransack as the search library, and the repository may be a Rails
application. The coding agent combines all three into a repository-native
implementation. Forma core does not interpret the technology name `ransack`.

The smallest experimental `v0alpha1` Implementation Policy Manifest is
implemented, but its schema is not yet final. See its
[design](docs/implementation-policy-manifest-proposal.md) for details.

## What Forma describes

One compilation unit represents one application namespace. The current v0
design contains these concepts:

| Area | Concepts |
| --- | --- |
| Data | Type, Entity, Field, Relation |
| Behavior | State, Action |
| Presentation | Page, List, Detail, Form |
| Authorization | Role |

A relation is an entity-typed field rather than a separate primitive. An action
currently represents an allowed entity state transition. The
[Forma v0 specification](docs/v0-primitives.md) defines the precise syntax and
semantics.

The following concepts are being designed through concrete examples:

| Concept | What it should express |
| --- | --- |
| Expression | field references, comparisons, arithmetic, and conditions |
| Derived Value | values calculated from other values |
| Invariant / Precondition | always-valid constraints and conditions before an action |
| Changes | post-state caused by an action |
| Occurrence / Effect | facts that occurred and external effects such as email or notifications |
| Identity | relationships between a principal and domain data |

They are explored in the
[minimal expression proposal](docs/expression-proposal.md),
[order approval and inventory probe](docs/order-approval-proposal.md), and
[public membership proposal](docs/public-membership-proposal.md). The concrete staged flow for P1 is fixed in the
[email-verified membership probe](docs/email-verified-membership-probe.md).

## What the compiler gives to AI

Forma source is not merely converted into a longer prompt. The front end
resolves its meaning and produces machine-readable output.

### Resolved Intent

Resolved Intent is the application meaning a coding agent must implement. It
contains resolved entities, fields, constraints, states, actions, permissions,
pages, capabilities, navigation, and stable semantic identities.

It does not contain React components, HTTP verbs, SQL, directories, package
names, framework APIs, loading widgets, relation pickers, submission tokens, or
other implementation mechanisms. Source Maps connect every node back to Forma
source so compiler and repository failures can be explained at a location a
human can review.

### Acceptance Facts

Acceptance Facts are target-neutral statements that must hold after
implementation:

```text
- User.activate succeeds only from Confirmed
- admin can view the Users page
- Users searches name and email
- Users has a logical page size of 20
- an invalid transition is rejected without changing state
```

The coding agent translates each fact into the target repository's normal unit,
integration, request, or browser tests. Forma does not standardize HTTP status
codes or DOM selectors. Instead, each fact has a stable ID. After generation,
Forma verifies that the requested and covered fact ID sets match and that every
fact passes.

In short, **Forma decides what must be guaranteed**; the agent decides **how to
implement and observe it in that repository**.

## Relationship to the target repository

The target repository is normal application source, not a disposable artifact.
The coding agent may add to an existing system, preserve hand-written code,
follow established architecture, and apply incremental changes. Humans may
continue to work in the repository.

Forma owns the application intent it expresses. The repository owns the
concrete implementation:

- components and user-interface structure;
- routes, APIs, and transport;
- database schema, persistence, and migrations;
- framework and library usage;
- file layout and naming conventions;
- target-specific tests;
- integration with existing code.

A semantic change made only in target code drifts from Forma source and should
be reflected back into Forma before the next agent request. Forma does not
require byte-identical application-code regeneration.

## Current status

Forma is in an early design phase and has no compiler release yet. The current
Go front end partially implements the design draft v0.4 grammar, parser, name
resolution, type checking, semantic validation, stable identities, Resolved
Intent, and Source Maps. It also implements a minimal admin-flow Acceptance
Facts and Generation Request slice and exploratory non-v0 self-only Invariant,
bounded action-owned Changes, and one-hop relation-value expression slices.

The first Invariant vertical slice emits two entity-level Acceptance Facts for
satisfied and violated post-state predicates, plus an authoritative rejection
Fact for each form submit that edits a referenced field. It carries the resolved
Expression tree and atomic outcome into a Generation Request and exposes
concurrent enforcement as a separate human Review Requirement. The
repository-specific coding-agent run now passes all 278 Acceptance Facts in a
standalone Go application across 52 mapped tests. The same target implements
`StockReservation.commit`: its implicit transition and related StockItem write
commit atomically from one pre-state snapshot, with the stored value read from
a distinct required ReservationPlan relation. Four concurrency, atomicity,
cross-entity write authorization, and cross-entity value-read authorization
Review Requirements await human review.

In the first controlled agent run, a Generation Request produced from Forma was
given to an AI coding agent, which implemented an admin interface in a
standalone Go repository. All 43 derived Acceptance Facts were verified. This
experiment did not have Forma generate a Go admin application; Forma decided
the meaning, and AI implemented an ordinary Go application.

The following incremental run updated the same repository without rebuilding
it, adding `User.nickname` and changing the page size from 20 to 10. All 43
Facts and the existing 12 tests remained covered, and the required, preferred
deviation, and forbidden paths of the Implementation Policy Manifest were
verified.

The old Go admin generator and target-neutral conformance adapter under
`experiments/` are **frozen meaning-discovery prototypes**, not the planned
architecture. A second framework generator or shared runtime adapter is not the
next step.

### Run from source

Go 1.24 or newer can run the checker and current generation workflow:

```bash
go run ./cmd/forma check examples/users.forma
go run ./cmd/forma check examples/orders.forma
go run ./cmd/forma check examples/public-membership.forma
go run ./cmd/forma check examples/email-verified-membership.forma
go run ./cmd/forma resolve examples/users.forma
go run ./cmd/forma project navigation examples/email-verified-membership.forma
go run ./cmd/forma project outcomes experiments/membership-agent-e2e/app.forma
go run ./cmd/forma project states experiments/membership-agent-e2e/app.forma
go run ./cmd/forma project flow examples/email-verified-membership.forma
go run ./cmd/forma request experiments/admin-agent-e2e/app.forma
go run ./cmd/forma request --previous internal/agentrequest/testdata/admin.request.json --manifest experiments/admin-agent-e2e/target/forma.implementation.yaml experiments/admin-agent-e2e/app.forma
go run ./cmd/forma verify --repository experiments/admin-agent-e2e/target --baseline internal/agentrequest/testdata/admin.request.json internal/agentrequest/testdata/admin.incremental.request.json experiments/admin-agent-e2e/target/generation-feedback.json
go test ./...
```

The files and directories passed to one `forma check` invocation form one
compilation unit. Each example is an independent application and should be
checked separately.

`forma resolve` emits canonical Resolved Intent JSON. `forma project navigation`
emits a deterministic, read-only text view of the navigation already present in
Resolved Intent; it does not become a second source of truth or infer an
undeclared default entry. The membership example declares `entry SignUp` and
page-local `continue` transitions through `OnboardingGuide`; older sources without
`entry` remain explicitly `unspecified`. `forma project outcomes` expands
observable Acceptance Facts into case-oriented review text. It separates explicitly absent, zero,
excluded, or unchanged results as `must not` guarantees without inventing the
inverse of an unstated fact. `forma project states` shows entity state machines,
creation initializers, transition invocations, confirmation/role requirements,
and Identity state eligibility. Session lifecycle is not misrepresented as a
domain transition. `forma project flow` composes those three projections into
a deterministic Markdown/Mermaid review view. Exact semantic IDs link outcome
groups and state elements to navigation edges; unmatched detail remains in an
explicit index. The diagram is not editable application meaning and does not
infer layout. `forma request` emits a Generation Request, and
`forma verify` validates agent feedback against the immutable request. The
email-verified signup/signin Identity probe is complete
through Stage D: current artifacts use Resolved Intent v0.11, Source Map v0.6, 85 Acceptance Facts,
and three Review Requirements are verified in the existing admin target. The
first bounded P2 automated-repair probe is also complete. Separate fresh agent
processes repaired a controlled test failure and build failure in one attempt
each and returned both runs to 85/85. A third process left the repository
unchanged when a protected test exceeded the immutable request; trusted
remeasurement published the Forma intent gap as a structured `test/blocked`
human handoff. The bounded navigation-language probe chose and implemented
page-local ownership: one top-level `entry`, and operation-free `continue Page`
members on their source pages. P3 is now in progress: self-only Invariant, the
first bounded Changes/atomic-post-state slice, and one required to-one relation
value and one exact binary numeric addition on a Changes RHS reach the Parser, Resolved Intent, Acceptance Facts,
Generation Request, and a 278/278 repository E2E in an ordinary Go application.
The addition slice carries the full expression tree and runtime-bound operands into Facts, rejects unsupported type bounds and named-type chains, and detects repository integer overflow without partial commit. Five human Review Requirements
remain pending; Occurrence and Effect follow only after the full Order approval boundary is
fixed. Projection readability
evaluation runs independently.

## Design documents

- [Forma v0 specification](docs/v0-primitives.md)
- [Agent generation model](docs/agent-generation.md)
- [Implementation Policy Manifest proposal](docs/implementation-policy-manifest-proposal.md)
- [Development roadmap](docs/roadmap.md)
- [Identity variant probe](docs/identity-variant-probe.md)
- [Language design principles](docs/language-design-principles.md)
- [Current language direction](docs/current-language-direction.md)
- [Changes and atomic post-state proposal](docs/changes-proposal.md)
- [Relation value expression in Changes proposal](docs/relation-value-expression-proposal.md)
- [Numeric addition in Changes proposal](docs/numeric-addition-expression-proposal.md)
- [Membership flow notation probe](docs/membership-flow-notation-probe.md)
- [Membership flow human evaluation](docs/evaluations/membership-flow/README.md)
- [Complete user-management example](examples/users.forma)
- [Order approval and inventory probe](examples/orders.forma)
- [Email-verified membership probe](docs/email-verified-membership-probe.md)
- [Identity semantic model proposal](docs/identity-semantic-model-proposal.md)
- [Identity surface syntax proposal](docs/identity-surface-syntax-proposal.md)
- [Email-verified membership example](examples/email-verified-membership.forma)
- [Public membership v0 subset](examples/public-membership.forma)
- [Minimal expression proposal](docs/expression-proposal.md)
- [Active admin agent-generation experiment](experiments/admin-agent-e2e/README.md)
- [Frozen admin generation prototype](experiments/admin-e2e/README.md)
- [Frozen conformance prototype](experiments/conformance/README.md)

## Design boundaries

Forma aims to express application intent directly, resolve defaults and
references deterministically, and make change impact traceable through semantic
identities. Build and test feedback repairs implementation; it does not redefine
intent.

Forma is not intended to:

- generate each framework through a built-in deterministic lowerer;
- maintain target-profile capability matrices or framework adapter suites;
- standardize routes, SQL, components, directories, or test frameworks;
- require byte-identical application-code regeneration;
- use unchecked natural language as executable syntax;
- delegate parsing, name resolution, type checking, or Forma semantics to an LLM;
- replace every low-level or systems programming language.

The detailed review criteria are in
[Forma Language Design Principles](docs/language-design-principles.md).
