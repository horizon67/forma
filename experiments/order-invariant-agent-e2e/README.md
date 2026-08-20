# Order Invariant Agent E2E

Status: machine verification complete — 172/172 Acceptance Facts passed; one compiler-owned concurrency Review Requirement awaits human review.

This experiment gives the full Generation Request derived from [`app.forma`](app.forma) to an AI coding agent and records the resulting ordinary Go application under [`target/`](target/). The target does not import Forma, embed a Forma runtime, or use a domain-specific generator. Framework choice, package layout, persistence boundary, HTTP routes, and repository-native tests belong to the generated application.

The experiment measures the first P3 Expression/Invariant slice across the complete order and inventory example rather than an isolated compiler fixture.

## Measured boundary

```text
Forma source                     app.forma
Generation Request              172 Acceptance Facts
Review Requirements             1 concurrent-invariant-enforcement
Target application              standard-library Go HTTP application
Repository tests                39 distinct mapped tests
Implementation policies         none
```

The target includes Customers, Products, Orders, OrderLines, StockItems, order lifecycle transitions, role authorization, list queries, server-rendered surfaces, validation feedback, navigation, idempotent form dispatch, and an authoritative stock invariant. Every one of the 165 Facts whose subject is a `page/...` surface is mapped to at least one test that issues a request through the actual HTTP handlers. This includes all 98 access Facts, form mutation and replay, search/filter/sort, and a reachable second page after the 20-record boundary. Pure domain/store tests remain supplementary checks and cannot be the only evidence for a page Fact.

`reserved <= onHand` is enforced inside the store mutex. The form handler cannot be the only enforcement path: repository tests call the store directly, and a second `ReserveStock` mutation boundary is exercised concurrently. See [`review-evidence.md`](review-evidence.md).

## Reproduce

From the repository root:

```bash
go run ./experiments/order-invariant-agent-e2e/cmd/generate
go test ./experiments/order-invariant-agent-e2e/cmd/...

cd experiments/order-invariant-agent-e2e/target
go test ./...
go vet ./...
cd ../../../

go run ./experiments/order-invariant-agent-e2e/cmd/feedback
go run ./cmd/forma verify \
  --repository experiments/order-invariant-agent-e2e/target \
  experiments/order-invariant-agent-e2e/generation-request.json \
  experiments/order-invariant-agent-e2e/target/generation-feedback.json
```

Expected verification output:

```text
verified 172 acceptance facts: all passed
  39 distinct tests, max 14 facts per test
human review required: 1 requirements are not machine-verified
```

`cmd/generate` only derives `generation-request.json` and the explicit `coverage.json`; it does not generate application code. `cmd/feedback` is an experiment-side measurement process. It validates every test reference, removes stale feedback before execution, runs the target's real `go test -count=1 -json ./...`, and publishes succeeded feedback only when every mapped test passes.

## Artifacts

| Artifact | SHA-256 | Purpose |
| --- | --- | --- |
| [`app.forma`](app.forma) | — | Forma source copied into the measured boundary |
| [`generation-request.json`](generation-request.json) | `72bc8a5f9d690414c2280836be6fa4ca790d2227e72b3f7d272f0267a2d285b3` | canonical full request with 172 Facts and one Review Requirement |
| [`coverage.json`](coverage.json) | `a310d523f9f8497a776b914800cf20c9b96bddcd735956aa668b455cad07d392` | exact Fact-to-repository-test mapping; unknown or omitted Facts are rejected |
| [`target/generation-feedback.json`](target/generation-feedback.json) | `3abbc4e39659454b36198e7bb32ca7ac922d96a2d59624a6cee9c7b073885c2f` | result derived from the actual target test run |
| [`review-evidence.md`](review-evidence.md) | — | evidence for the remaining human-only concurrency requirement |

## What this probe does not prove

- The concurrency Review Requirement is still awaiting human review; a passing race detector and the conflicting-reservation test do not complete that architectural review.
- This is a bounded in-memory target, not evidence for durable or distributed transaction behavior, an existing production repository, or reproducibility by a second independent agent.
- The compiler does not yet emit a general application-level fixture-fidelity Review Requirement. This experiment closes its measured access boundary by mapping every access Fact to an actual HTTP request, but a target-neutral review subject and completion contract for arbitrary future surface Facts remain a language-design question.
- The target is not a reusable Go generator, framework profile, or Forma runtime.

The target is intentionally standalone. It is evidence that Forma determines application meaning while the AI produces a normal application, not evidence for a reusable Go generator or shared application runtime.
