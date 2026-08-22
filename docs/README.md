# Forma Documentation

Forma is preparing its first public pre-release, `v0.1.0-alpha.1`. The alpha
exists to exercise a deliberately bounded language and AI-generation workflow
in real repositories. It is not a compatibility promise for the complete v0
design.

## Start here

- [Install Forma](install.md)
- [Alpha language profile](alpha-language-profile.md)
- [Language reference](language-reference.md)
- [AI integration and credentials](ai-integration.md)
- [Current language direction](current-language-direction.md)

The end-user quickstart, language guide, CLI reference, security guide, and
release process are release blockers tracked in
the [roadmap](roadmap.md). They must be added before the alpha tag is created.

## Language design and reference

- [Alpha language profile](alpha-language-profile.md) — the public contract for
  syntax and semantics accepted by the alpha binary
- [Forma v0 primitives and language specification](v0-primitives.md) — the
  broader normative design target; not every part is implemented in alpha
- [Language design principles](language-design-principles.md) — the criteria
  used to add or reject language constructs
- [Agent generation model](agent-generation.md) — Generation Request,
  Acceptance Facts, feedback, and verification

## Experimental design records

The proposal and probe documents in this directory preserve research evidence
and rationale. They are not an alternative user-facing language specification.
An experimental construct is supported by the alpha only when the
[alpha profile](alpha-language-profile.md) says so.

## Project

- [Development roadmap](roadmap.md)
- [Current language direction](current-language-direction.md)
- [Reference runner proposal](reference-agent-runner-proposal.md)

## Document ownership

- `alpha-language-profile.md` defines what the released alpha accepts and what
  compatibility it promises.
- The future `language-guide.md` teaches implemented syntax without redefining
  it.
- `language-reference.md` is the public navigation layer for the alpha
  contract, design-target specification, and artifact schemas.
- `v0-primitives.md` defines the larger v0 design target. It must not be read as
  a claim that the alpha implements every v0 rule.
- `agent-generation.md` owns the machine-readable handoff and verification
  model.
- `roadmap.md` owns future work. A roadmap item is not current behavior.
- Proposal, probe, and evaluation documents retain evidence and decisions; they
  do not silently extend the released language.

The eventual documentation website must be generated from these versioned
repository documents. The website is a projection, not a second source of
truth.
