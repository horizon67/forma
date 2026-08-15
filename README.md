# Forma

**English** | [日本語](README.ja.md)

**A higher-level programming language for building applications.**

Forma is an experimental programming language for describing software in terms
of application concepts—entities, states, relationships, actions, pages, lists,
and forms—rather than framework-specific mechanics.

> Forma source expresses what an application is. The compiler determines its
> meaning, and a target generator turns that meaning into an implementation.

```forma
type Email = String matches /.+@.+/

entity User {
    name  String required
    email Email required unique

    state status Pending | Confirmed | Active | Suspended initial Pending
}

action User.activate: Confirmed -> Active

page Users {
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

`list User` means “present a collection of users.” It does not mean an HTML
table, a React component, or a database query. A target profile chooses an
appropriate implementation while preserving the declared application semantics.

## Why Forma?

Application source code contains two different kinds of information:

1. decisions specific to the application;
2. implementation mechanics required by frameworks and runtimes.

Forma aims to maximize the density of the first and minimize the amount of the
second.

In the example above, the developer decided which entity to present, which
fields are visible and searchable, how results can be filtered, and the logical
page size. Request encoding, data fetching, loading and failure states, query
construction, widgets, cache updates, and routing are implementation mechanics.

A conventional implementation may require coordinating all of these concerns
across a frontend, backend, database, API contract, and tests. Forma treats them
as consequences of one application-level declaration.

## Why Forma now?

AI-assisted development has changed the relationship between people and source
code. Developers write fewer implementation lines by hand, and reading every
line of a large generated codebase is no longer realistic. Yet applications are
still split across frontend, backend, database, schema, and test languages, while
their specification is scattered among design documents, implementation code,
API specs, issues, and prompts.

Spec-driven approaches such as Kiro's `design.md` are useful attempts to reduce
that fragmentation. A prose specification, however, must still be reread,
interpreted, checked against the implementation, and kept synchronized after
every change. When machines perform most implementation work, asking humans to
operate both long specifications and generated code is tedious and prone to
drift.

Forma's hypothesis is that the missing layer is not a more detailed document,
but a higher-level language: readable as a blueprint and parseable, checkable,
and executable as a program. Humans maintain concise, semantically dense Forma
source. The compiler fixes its meaning, target generators—including AI—produce
implementations, and conformance checks the result.

```text
Humans read and review       Forma source
Machines generate and own   target code
Compiler + conformance      keep their meanings aligned
```

Forma is neither a store for natural-language prompts nor a one-shot scaffolder
driven by a design document. It aims to consolidate the blueprint,
implementation intent, and checkable specification that are currently scattered
across a project into one executable application language.

## Philosophy

### Code should look like the model

Application code should resemble the concepts developers actually reason about.
A user has states. An order has a lifecycle. A page contains a searchable list.
An action changes the system. These concepts should be directly expressible.

### Raise the level of abstraction

Existing languages abstract away machine instructions, memory addresses, and
CPU registers. Forma attempts to move one level higher by making larger
application semantics part of the language.

### Complexity belongs below the abstraction

Forma does not assume that software is simple. It moves repeated implementation
complexity behind language-level concepts so compilers and runtimes can handle
it consistently.

### Prefer semantics over syntax sugar

Forma is not intended to be shorter syntax for Ruby, Go, or TypeScript.
Constructs such as `entity`, `state`, `action`, `list`, and `page` carry semantic
meaning understood by the compiler.

### One source, multiple targets

Forma source is not coupled to a particular framework. The same application
model should be lowerable through different target profiles while preserving
the same observable behavior.

### Forma is the source humans read

Generated React, Ruby, Go, or other target code is not source that humans keep
editing. Forma source must therefore be the application's only source of truth:
diffable, reviewable, searchable, and durable in version control. Diagrams are
derived views generated from Forma, not a second UML input that can drift.

A requirement that Forma cannot express must not be patched into generated
code. A profile that cannot implement declared intent produces a compile error;
requirements absent from the language are explicitly outside v0's scope. Any
future escape hatch must remain visible and versioned as a profile or
source-level extension.

### Readable accurately and over time

Forma source prioritizes accurate understanding of application meaning over
brevity or prose-like syntax. Without knowing the target framework, a reader
should be able to explain what exists, who can see and change it, which
constraints and state transitions apply, and what a change will affect.

Forma omits implementation mechanics, not semantically important facts. The
same concept uses the same form; implicit defaults and reference resolution
follow closed, deterministic rules that the compiler can expand and explain.
See [Forma Language Design Principles](docs/language-design-principles.md) for
the detailed review criteria.

### Fix meaning, not implementation shape

Syntax, name resolution, types, permissions, transitions, navigation, and
conformance expectations are deterministic. Component boundaries, file layout,
and framework APIs need not be byte-identical. An AI target generator may vary
the implementation only while preserving resolved Semantic IR and passing the
same conformance contract.

## One declaration, many consequences

Consider a state transition:

```forma
action User.activate: Confirmed -> Active
```

This is not shorthand for one `if` statement. It declares the application rule:

> A user can become active only from the confirmed state.

From that rule, a target may produce an authoritative backend guard, frontend
action availability, an API contract, state refresh behavior, tests for valid
and invalid transitions, and documentation. One Forma declaration can affect
many generated artifacts because it represents meaning, not a code template.

## Architecture

Forma is designed around a target-neutral semantic intermediate representation
and a conformance contract derived from it, not direct source-to-source
rewriting.

```text
Natural language (optional)
          │
          ▼ optional AI
     Forma source
          │
          ▼ deterministic front end
Lexer / Parser / AST / Checker
          │
          ▼
Semantic IR + Conformance Contract
          │
          ▼ target profile + generator (AI allowed)
Generated Application
          │
          ▼ build + deterministic conformance
   Accepted Artifact
```

The compiler resolves types, relationships, state transitions, permissions,
actions, and navigation before a target profile selects concrete components,
transport, persistence, and runtime behavior.

The deterministic boundary runs from Forma source through the Semantic IR and
conformance contract:

```text
Forma source + front-end version -> Semantic IR + Conformance Contract
```

A target generator may use AI, and generated code may differ between runs. That
code is a disposable artifact rather than human-edited source. Its correctness
is determined by a successful build and a deterministic conformance contract,
not by byte-for-byte identity.

## Forma and AI

AI-assisted development showed that developers often describe application
changes at a much higher level than existing programming languages allow:

> Add a paginated user list with search by name and email.

Today, a coding agent may expand that instruction into many pieces of
framework-specific code. Forma explores whether the same high-level instruction
can become durable, reviewable, and mechanically checkable source code:

```forma
page Users {
    list User {
        search name, email
        paginate 20
    }
}
```

AI may translate natural language into Forma, help author target profiles, or
generate target artifacts from Semantic IR. Forma's syntax, semantics,
validation, Semantic IR, and conformance contract remain deterministic and do
not depend on model judgment.

An AI target generator receives resolved Semantic IR, a versioned target
profile, and an output contract—not unchecked Forma source. Generated output is
accepted only after it builds and passes conformance; failures are returned as
diagnostics for another generation attempt.

## Goals

- Express application decisions with high semantic density.
- Make domain rules and user-visible capabilities explicit and checkable.
- Generate consistent behavior across frontend, backend, persistence, and tests.
- Keep Forma source independent of target frameworks.
- Make semantics and acceptance decisions reproducible without an LLM.
- Treat target code as a generated artifact that never needs manual editing.

## Non-goals

Forma is not intended to:

- replace every low-level or systems programming language;
- expose every capability of every target framework;
- use natural language as executable syntax;
- delegate parsing, semantic resolution, or the conformance oracle to an LLM;
- manually edit generated target code and create a second source of truth;
- become a collection of framework-specific shortcuts.

## Status

Forma is in an early design phase. There is no compiler release yet. The
unreleased Go front end partially implements the design draft v0.4 surface
syntax, together with the lexer, parser, syntax AST, name resolution, type
checking, semantic validation, diagnostics, and `forma/v0.3` core Semantic IR.

This does not mean the entire normative draft is implemented. The conformance
contract, IR source map, target-profile capability check, and artifact
generation and verification protocols remain future work. The normative
document is design draft v0.4; the reference implementation covers only part
of it.

- [Forma v0 specification](docs/v0-primitives.md)
- [Development roadmap](docs/roadmap.md)
- [Complete user-management example](examples/users.forma)
- [Architecture Manifest proposal (exploratory)](docs/architecture-manifest.md)
- [Public membership and identity proposal (exploratory)](docs/public-membership-proposal.md)

Run the checker from source with Go 1.24 or newer:

```bash
go run ./cmd/forma check examples/users.forma
go test ./...
```

`forma build`, `forma conformance`, and `forma run` remain future work.
