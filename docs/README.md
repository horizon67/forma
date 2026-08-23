# Forma Documentation

Forma is preparing its first public pre-release, `v0.1.0-alpha.1`. The alpha
exists to exercise a deliberately bounded language and AI-generation workflow
in real repositories. It is not a compatibility promise for the complete v0
design.

## Start here

- [Install Forma](install.md)
- [End-to-end alpha quickstart](quickstart.md)
- [CLI reference](cli.md)
- [Write Forma for the alpha](language-guide.md)
- [Alpha language profile](alpha-language-profile.md)
- [Language reference](language-reference.md)
- [AI integration and credentials](ai-integration.md)
- [Security and trust boundary](security.md)
- [Current language direction](current-language-direction.md)

The release-candidate process and its remaining external qualification are
tracked in the [roadmap](roadmap.md).

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
- [First alpha quickstart dogfood](evaluations/alpha-dogfood-2026-08-23.md)
- [Post-alpha hardened runner research](reference-agent-runner-proposal.md)
- [Alpha.1 release notes](releases/v0.1.0-alpha.1.md)

## Document ownership

- `alpha-language-profile.md` defines what the released alpha accepts and what
  compatibility it promises.
- `language-guide.md` teaches implemented syntax without redefining it and is
  embedded by `forma authoring-context` together with complete examples.
- `language-reference.md` is the public navigation layer for the alpha
  contract, design-target specification, and artifact schemas.
- `v0-primitives.md` defines the larger v0 design target. It must not be read as
  a claim that the alpha implements every v0 rule.
- `agent-generation.md` owns the machine-readable handoff and verification
  model.
- `roadmap.md` owns future work. A roadmap item is not current behavior.
- Proposal, probe, and evaluation documents retain evidence and decisions; they
  do not silently extend the released language.

Release contracts and end-user guidance are maintained in English. Internal
design decisions, proposals, probes, and evaluation records may be written in
Japanese; [`README.ja.md`](../README.ja.md) is the maintained Japanese counterpart to the root
README. Document language does not change the ownership rules above.

The eventual documentation website must be generated from these versioned
repository documents. The website is a projection, not a second source of
truth.
