# Forma

**en** / [ja](README.ja.md)

**A language for describing applications themselves at a high level.**

Forma replaces the natural-language prompt given to a coding agent with a
typed, checkable, and reviewable language.

> Humans decide application intent in Forma. The compiler resolves that intent,
> and an AI coding agent implements it in an ordinary software repository.

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

`list User` means “present a collection of users.” It does not prescribe an
HTML table, a React component, an endpoint, or a query builder. A coding agent
inspects the target repository and chooses an implementation that preserves the
resolved intent.

## The core execution model

AI generation is not an optional backend for Forma. It is the central
end-to-end execution model.

```text
Forma source
  → parse / check / resolve
  → Resolved Intent + Acceptance Facts
  → Generation Request
  → AI coding agent + target repository
  → ordinary application code
  → build / test
  → feedback to the agent
```

The deterministic Forma front end stops at meaning. It does not lower that
meaning into framework files. The coding agent receives a machine-readable
request and works in the actual repository, using its existing architecture,
libraries, conventions, and tests.

This boundary is the project’s defining distinction:

> A conventional DSL builds a code generator. Forma builds stronger input for
> the AI that writes the code.

See [Agent Generation Model](docs/agent-generation.md) for the responsibility
boundary and the planned request/feedback loop.

## What Forma describes

One compilation unit represents one application namespace. The current v0
design contains these application concepts.

```text
Application
├── Data
│   ├── Type
│   ├── Entity
│   ├── Field
│   └── Relation
├── Behavior
│   ├── State
│   └── Action
├── Presentation
│   ├── Page
│   ├── List
│   ├── Detail
│   └── Form
└── Authorization
    └── Role
```

A relation is an entity-typed field rather than a separate primitive. An action
currently represents an allowed entity state transition. The
[Forma v0 specification](docs/v0-primitives.md) defines the precise syntax and
semantics.

Additional concepts remain under design:

```text
Under design
├── Expression
│   ├── Derived value
│   ├── Invariant
│   └── Precondition
├── Changes
├── Occurrence
├── Effect
└── Identity
```

They are explored in the
[minimal expression proposal](docs/expression-proposal.md),
[order approval and inventory probe](docs/order-approval-proposal.md), and
[public membership proposal](docs/public-membership-proposal.md).

## Why Forma?

AI coding agents already accept high-level requests such as:

> Add a paginated user list with search by name and email.

Natural-language prompts are convenient but weak as durable application source.
Names are unresolved, types are implicit, omissions are hard to distinguish
from decisions, and later prompts can reinterpret earlier ones. Prose
specifications have the same synchronization problem: people must continuously
reread them and compare them with the implementation.

Forma turns that request into source that can be parsed, checked, diffed, and
reviewed:

```forma
page Users {
    list User {
        search name, email
        paginate 20
    }
}
```

The compiler catches misspelled fields, incompatible types, invalid state
transitions, unresolved actions, and inconsistent permissions before a coding
agent changes the repository. It then emits the application meaning with all
references and deterministic defaults resolved.

## Resolved Intent

The compiler’s main output is **Resolved Intent**, not a lowering-oriented
intermediate representation. It is a target-neutral, machine-readable account
of what the coding agent must implement.

It contains resolved entities, fields, constraints, states, actions,
permissions, pages, capabilities, navigation, and stable semantic identities.
It does not contain React components, HTTP verbs, SQL, directories, package
names, framework APIs, loading widgets, relation pickers, submission tokens, or
other implementation mechanisms.

Source Maps connect every resolved node back to Forma source so compiler and
repository failures can be explained in terms a human can review.

## Acceptance Facts

Forma also derives structured, target-neutral facts that should be true after
implementation. The following is their human-readable rendering:

```text
- User.activate succeeds only from Confirmed
- admin can view the Users page
- Users searches name and email
- Users has a logical page size of 20
- an invalid transition is rejected without changing state
```

The coding agent translates these facts into the target repository’s normal
unit, integration, request, or browser tests. Forma does not maintain a
framework adapter or prescribe HTTP status codes and DOM selectors.

Each fact has a stable ID. Repository-specific tests reference the IDs they
cover, and a successful generation run requires the requested and covered fact
ID sets to match and every covered fact to pass.

The expected meaning remains deterministic; only the repository-specific way of
implementing and testing it belongs to the agent.

## Forma and the target repository

The target repository is normal application source, not a disposable artifact.
The coding agent may add to an existing system, preserve hand-written code,
follow established architecture, and apply incremental changes. Humans may
continue to work in the repository.

Forma owns the application intent it expresses. The repository owns the
implementation. A semantic change made only in target code can drift from
Forma, so that change should be reflected in Forma source before the next agent
request. Forma does not attempt to make generated files byte-identical.

Concrete implementation decisions belong to the agent and repository:

- components and user-interface structure;
- routes, APIs, and transport;
- database schema, persistence, and migrations;
- framework and library usage;
- file layout and naming conventions;
- target-specific tests;
- incremental integration with existing code.

## Design principles

- Express application intent directly, not through framework vocabulary.
- Keep one canonical form for one application concept.
- Omit implementation mechanics, never semantically important facts.
- Resolve defaults and references deterministically and make them explainable.
- Make dependencies and change impact traceable through semantic identities.
- Let the coding agent choose implementation shape from repository context.
- Use build and test feedback to repair implementation, not to redefine intent.

The detailed review criteria are in
[Forma Language Design Principles](docs/language-design-principles.md).

## Goals

- Replace fragile coding prompts with typed, durable application intent.
- Make domain rules and user-visible capabilities explicit and checkable.
- Produce stable Resolved Intent and machine-readable Generation Requests.
- Let coding agents implement Forma in new or existing repositories.
- Feed build and test failures back into an iterative repair loop.
- Apply Forma changes incrementally without owning framework-specific generators.

## Non-goals

Forma is not intended to:

- generate each framework through a built-in deterministic lowerer;
- maintain target-profile capability matrices or framework adapter suites;
- standardize routes, SQL, components, directories, or test frameworks;
- require byte-identical application-code regeneration;
- use natural language as unchecked executable syntax;
- delegate parsing, name resolution, type checking, or Forma semantics to an LLM;
- replace every low-level or systems programming language.

## Status

Forma is in an early design phase. There is no compiler release yet. The
unreleased Go front end partially implements the design draft v0.4 grammar,
parser, name resolution, type checking, semantic validation, stable identities,
Resolved Intent, and Source Maps. It also contains an exploratory non-v0
self-only invariant slice.

The Go admin generator and target-neutral conformance adapter under
`experiments/` are now **frozen meaning-discovery prototypes**. They helped
identify information missing from the front end, but they are not the planned
Forma generation architecture. No second framework generator or shared runtime
adapter will be built as the next step.

- [Forma v0 specification](docs/v0-primitives.md)
- [Agent generation model](docs/agent-generation.md)
- [Development roadmap](docs/roadmap.md)
- [Language design principles](docs/language-design-principles.md)
- [Complete user-management example](examples/users.forma)
- [Order approval and inventory probe](examples/orders.forma)
- [Minimal expression proposal](docs/expression-proposal.md)
- [Frozen admin generation prototype](experiments/admin-e2e/README.md)
- [Frozen conformance prototype](experiments/conformance/README.md)

Run the checker from source with Go 1.24 or newer:

```bash
go run ./cmd/forma check examples/users.forma
go run ./cmd/forma check examples/orders.forma
go run ./cmd/forma resolve examples/users.forma
go test ./...
```

The files and directories passed to one `forma check` invocation form one
compilation unit. The two examples are independent applications and should be
checked separately.

`forma resolve` now emits canonical Resolved Intent JSON. The next tooling
milestones are the Generation Request and agent feedback loop. Framework-specific
`forma build --profile` is no longer on the core roadmap.
