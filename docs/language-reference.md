# Forma Language Reference

Status: public reference entry for the `v0.1.0-alpha.1` release candidate.

Forma has two intentionally different specification levels:

1. The [alpha language profile](alpha-language-profile.md) is the release
   contract for what the current binary accepts and verifies.
2. The [v0 primitives and language specification](v0-primitives.md) is the
   broader design target. Its unimplemented rules are not alpha behavior.

Start with the alpha profile when writing an application today. Use the v0
document to understand intended language direction and normative concepts that
may graduate in later releases.

For a concise tutorial, use the [alpha language guide](language-guide.md).
The installed CLI emits the same guide plus complete examples with
`forma authoring-context`, so an authoring AI does not need a live website.

## Current reference map

| Topic | Authoritative document |
| --- | --- |
| Released syntax and known limits | [Alpha language profile](alpha-language-profile.md) |
| Core primitives, modifiers, EBNF, static semantics | [v0 specification](v0-primitives.md) |
| Identity syntax and semantics | [Identity semantic model](identity-semantic-model-proposal.md) and the alpha profile |
| Generation Request and Feedback | [Agent generation model](agent-generation.md) |
| Invariant and bounded expressions | [Expression proposal](expression-proposal.md) and the alpha profile |
| Action-owned mutation | [Changes proposal](changes-proposal.md) and the alpha profile |
| Relation values | [Relation-value proposal](relation-value-expression-proposal.md) |
| Exact numeric addition | [Numeric-addition proposal](numeric-addition-expression-proposal.md) |
| Named action predicate | [Action-precondition proposal](action-precondition-proposal.md) |

Proposal documents provide detailed rationale and test evidence. When a
proposal and the alpha profile differ in scope, the alpha profile wins for the
released binary.

## Documentation website

The future documentation website will publish this repository content with
navigation and version selection. It is not the normative source itself.
