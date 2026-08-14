# Forma

**English** | [日本語](README.ja.md)

**A higher-level programming language for building applications.**

Forma is an experimental programming language for describing software in terms
of application concepts—entities, states, relationships, actions, pages, lists,
and forms—rather than framework-specific mechanics.

> Forma source expresses what an application is. Compilers decide how it is
> implemented.

```forma
type Email = String matches /.+@.+/

entity User {
    name  String required
    email Email required unique

    state status Pending | Confirmed | Active | Suspended
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

Forma is designed around a target-neutral semantic intermediate representation,
not direct source-to-source rewriting.

```text
Natural language (optional)
          │
          ▼
     Forma source
          │
          ▼
   Parser / Typed AST
          │
          ▼
Semantic Model / Application IR
      ├─ Web target ──── React
      ├─ Server target ─ Ruby / Go
      └─ Native target ─ Binary
```

The compiler resolves types, relationships, state transitions, permissions,
actions, and navigation before a target profile selects concrete components,
transport, persistence, and runtime behavior.

Compilation is intended to be deterministic:

```text
Forma source + target profile + compiler version -> generated application
```

AI is not part of this normative compilation path.

## Forma and AI

AI-assisted development showed that developers often describe application
changes at a much higher level than existing programming languages allow:

> Add a paginated user list with search by name and email.

Today, a coding agent may expand that instruction into many pieces of
framework-specific code. Forma explores whether the same high-level instruction
can become durable, reviewable, deterministic source code:

```forma
page Users {
    list User {
        search name, email
        paginate 20
    }
}
```

AI may help translate natural language into Forma. Forma itself is intended to
have deterministic syntax, semantics, validation, and compilation.

## Goals

- Express application decisions with high semantic density.
- Make domain rules and user-visible capabilities explicit and checkable.
- Generate consistent behavior across frontend, backend, persistence, and tests.
- Keep Forma source independent of target frameworks.
- Make generated behavior reproducible without an LLM.

## Non-goals

Forma is not intended to:

- replace every low-level or systems programming language;
- expose every capability of every target framework;
- use natural language as executable syntax;
- rely on an LLM for deterministic compilation;
- become a collection of framework-specific shortcuts.

## Status

Forma is in an early design phase. There is no compiler release yet.

- [Forma v0 specification](docs/v0-primitives.md)
- [Complete user-management example](examples/users.forma)

The planned initial compiler interface is:

```text
forma check
forma build
forma run
```
