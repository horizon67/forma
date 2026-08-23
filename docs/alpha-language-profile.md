# Forma `v0.1.0-alpha.1` Language Profile

Status: release-candidate profile. This document defines the language and
artifact subset intended for the first public alpha. The tag has not yet been
published.

## Contract

The alpha freezes the executable semantics currently proven through the named
Action Precondition slice. It is deliberately smaller than the broader
[Forma v0 design](v0-primitives.md). A construct described in a proposal or in
the v0 design is not part of this release unless it is listed here and accepted
by the released reference front-end.

The alpha may make breaking language, artifact, and CLI changes in later
pre-releases. A binary must reject unsupported syntax and incompatible artifact
versions rather than silently reinterpret them.

## Public commands

The compiler profile includes:

- `forma version`
- `forma authoring-context`
- `forma check`
- `forma resolve`
- `forma project navigation|outcomes|states|flow`
- `forma request`
- `forma generate`
- `forma verify`

`forma authoring-context` emits the public guide and complete examples embedded
in the installed binary. `forma generate` is the one mutating, network-capable
command. It stops after Codex edits the target so a person can review the diff;
it does not automatically execute agent-authored code or a feedback adapter on
the host. All other commands remain deterministic and do not invoke an LLM.

## Artifact baseline

| Artifact | Alpha version |
| --- | --- |
| Resolved Intent | `forma/resolved-intent/v0.12` |
| Source Map | `forma/source-map/v0.6` |
| Acceptance Facts | `forma/acceptance-facts/v0alpha11` |
| Navigation Projection | `forma/navigation-projection/v0alpha2` |
| Outcome Projection | `forma/outcome-projection/v0alpha6` |
| Domain State Projection | `forma/domain-state-projection/v0alpha1` |
| Flow Projection | `forma/flow-projection/v0alpha3` |
| Review Requirements | `forma/review-requirements/v0alpha6` |
| Implementation Policy Manifest | `forma/implementation-policy/v0alpha1` |
| Generation Request | `forma/generation-request/v0alpha4` |
| Generation Feedback | `forma/generation-feedback/v0alpha2` |

Schema version strings are semantic boundaries, not decorative metadata.
Unknown or unsupported versions must fail validation.

### Historical input compatibility

The alpha verifier also accepts these pinned historical forms. Their scope is
narrow: they are read for verification and incremental lineage, never emitted
as the current format and never interpreted using current compiler semantics.

| Input position | Accepted version | Scope |
| --- | --- | --- |
| Generation Request (historical full) | `forma/generation-request/v0alpha1` | `verify` request and historical baseline |
| Generation Request (historical incremental) | `forma/generation-request/v0alpha2` | `verify` request and historical baseline |
| Generation Feedback (legacy pair) | `forma/generation-feedback/v0alpha1` | only with a `v0alpha1` request |
| Resolved Intent (historical request) | `forma/resolved-intent/v0.4` | nested in the two historical request schemas |
| Acceptance Facts (historical request) | `forma/acceptance-facts/v0alpha1` | nested in the two historical request schemas |
| Source Map (historical request) | `forma/source-map/v0.2` | nested in the two historical request schemas |

## Supported core declarations

One explicit source set passed to the CLI is one compilation unit and one
application namespace. Source order must not change name resolution or the
canonical artifact result.

The alpha accepts these core declarations:

- `entry Page`
- named scalar and union `type` declarations;
- `entity`, scalar fields, optional or required to-one relations, and to-many
  relation fields;
- one optional `state` declaration per entity with an explicit initial value;
- standard `create`, `view`, `edit`, and `delete` actions;
- named domain state-transition `action` declarations;
- parameterless pages and pages with one entity parameter;
- `list`, `detail`, and create/edit `form` views;
- page-local `continue Destination` transitions;
- `role` and `allow` access declarations.

The six built-in scalar types are `String`, `Int`, `Decimal`, `Bool`, `Date`,
and `DateTime`. Named scalar constraints include `matches`, `min`, and `max` in
the combinations accepted by the checker. Alpha does not promise transitive
constraint composition for named types whose immediate base is another named
type.

Field modifiers include `required`, `unique`, `readonly`, `default`, and
`label`. View modifiers include explicit columns/fields, search, filter, stable
sort, bounded pagination, contextual actions, submit, and explicit navigation
destinations where required.

## Supported navigation and access semantics

- A compilation unit has at most one parameterless default `entry` page.
- A `continue` is an operation-free transition owned by its source page and
  targets a fixed parameterless page.
- Page actions resolve to one destination. Ambiguous standard create/view/edit
  destinations require an explicit `goto`.
- Page access, action access, and destination access are composed at the
  invoking surface.
- A role-restricted page with no entity view or Identity interaction owns its
  allowed and denied access Facts directly at the page boundary.
- `delete` always requires confirmation. A domain action marked `confirm`
  requires confirmation before dispatch.
- The alpha does not define cascade, detach, or restrict behavior when a
  deleted record is referenced by another record. A generated implementation
  must not present a destructive cascade as Forma-guaranteed behavior; this
  repository-specific choice remains human review.
- Source-state rejection, successful transition state, navigation, and
  observable feedback are represented in Acceptance Facts where supported.

URLs, route names, UI layout, framework choice, persistence layout, and test
framework are repository-specific implementation choices, not Forma syntax.

## Supported Identity facet

The alpha includes the implemented email-verified membership slice:

- one Identity bound to an entity;
- identifier canonicalization with the implemented named operations;
- local password proof policy;
- registration, verification/resend, signin, and signout interactions;
- owner and authenticated page requirements;
- fixed success/continue destinations and explicit feedback vocabulary.

This is not a general authentication-provider or credential DSL. The complete
accepted shape is demonstrated by
[`experiments/membership-agent-e2e/app.forma`](../experiments/membership-agent-e2e/app.forma).

## Experimental domain-behavior slice included in alpha

The following constructs are supported for evaluation but remain explicitly
experimental.

### Invariant

An entity may declare a named self-only predicate comparing its own required
fields with `<=`:

```forma
entity StockItem {
    onHand   Quantity required
    reserved Quantity required

    invariant stockAvailable: reserved <= onHand
}
```

The compiler emits satisfied/violated facts, authoritative mutation rejection
facts for affected form submissions, and a concurrency Review Requirement.
Relation traversal in an Invariant is not accepted.

### Changes

An explicit domain action may contain one `changes` assignment. Its target is
`self` or one required to-one relation. All values are evaluated from one
consistent pre-state and the state transition plus change commit atomically.

The right-hand value may be:

- one compatible scalar field from `self`;
- one scalar field reached through one required to-one relation; or
- one exact binary numeric addition using the supported operands.

Numeric addition is limited to the closed, direct-built-in named-type cases
accepted by the checker. Representation failure and unavailable relation values
must not partially commit.

### Action Precondition

An explicit domain action may contain one named `precondition` using the first
implemented `<=` predicate slice. It may read supported self and one-hop
required-relation values and one exact numeric addition. Evaluation uses the
same consistent pre-state as Changes.

Precondition false, relation/value unavailable, source-state mismatch,
confirmation decline, post-state Invariant violation, and success remain
distinct outcomes. The compiler emits facts for supported machine-observable
outcomes and Review Requirements for concurrency and enforcement boundaries.

The complete executable example is
[`experiments/order-invariant-agent-e2e/app.forma`](../experiments/order-invariant-agent-e2e/app.forma).

## Generation and verification boundary

`forma request` deterministically emits resolved intent, source locations,
Acceptance Facts, human Review Requirements, implementation policy, requested
change metadata, and verification policy. A coding agent implements that
request in an ordinary repository.

`forma verify` validates canonical request contents, required fact coverage,
feedback shape, applicable implementation policies, and review evidence. It
does not make an agent's prose claim authoritative and does not prove a human
Review Requirement automatically.

## Explicitly unsupported in this alpha

- multiple Changes assignments in one action;
- runtime alias handling between multiple mutation targets;
- collection binding, fan-out mutation, or collection expressions;
- creating records from domain actions;
- general Derived Value, Occurrence, or Effect constructs;
- arbitrary arithmetic, boolean operators, nesting, optional traversal, or
  multi-hop relation paths;
- general action parameters or statement blocks;
- inherited named-type constraint composition as a release guarantee;
- rename/delete/data-migration semantics for incremental intent changes;
- `forma fmt`, `forma explain`, editor integration, and LSP;
- framework generators, target profiles, and multiple AI providers;
- stable schema migration and long-term compatibility guarantees.

An unsupported construct must produce a diagnostic. It must not be partially
accepted with missing facts or delegated to the coding agent as prose.

## Evidence baseline

The release candidate is grounded in two repository E2E targets:

- membership: 85/85 Acceptance Facts, 3 implementation policies, 3 Review
  Requirements;
- order/inventory: 280/280 Acceptance Facts, 52 mapped tests, 6 Review
  Requirements.

These targets are regression evidence, not application templates and not proof
that every framework can implement the profile without a documented blocker.

## Relationship to other documents

- This profile owns what the alpha binary claims to support.
- [The v0 primitives specification](v0-primitives.md) owns the larger language
  design target.
- [Agent generation](agent-generation.md) owns artifact and verification
  semantics.
- [The roadmap](roadmap.md) owns future work and release blockers.
- Individual proposal documents own rationale and mutation evidence, not
  released syntax.
