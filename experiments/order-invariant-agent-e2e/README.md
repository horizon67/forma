# Order Invariant Agent E2E

Status: machine verification complete — 275/275 Acceptance Facts passed; three compiler-owned Review Requirements await human review.

This experiment gives the full Generation Request derived from [`app.forma`](app.forma) to an AI coding agent and records the resulting ordinary Go application under [`target/`](target/). The target does not import Forma, embed a Forma runtime, or use a domain-specific generator. Framework choice, package layout, persistence boundary, HTTP routes, and repository-native tests belong to the generated application.

The experiment measures the first P3 Expression/Invariant slice and the bounded Changes/atomic-post-state slice across one complete order and inventory application rather than isolated compiler fixtures.

## Measured boundary

```text
Forma source                     app.forma
Generation Request              275 Acceptance Facts
Review Requirements             3 (concurrency, atomic Changes, cross-entity authorization)
Target application              standard-library Go HTTP application
Repository tests                52 distinct mapped tests
Implementation policies         none
```

The target includes Customers, Products, Orders, OrderLines, StockItems, StockReservations, order lifecycle transitions, role authorization, list queries, server-rendered surfaces, validation feedback, navigation, idempotent form dispatch, an authoritative stock invariant, and an action that changes two entities atomically. Every one of the 242 Facts whose subject is a `page/...` surface is mapped to at least one test that issues a request through the actual HTTP handlers. This includes all 104 access Facts, form mutation and replay, search/filter/sort, a reachable second page after the 20-record boundary, transition outcomes for every source state, confirmation accept/decline, and every Changes outcome. Pure domain/store tests remain supplementary checks and cannot be the only evidence for a page Fact.

`reserved <= onHand` is enforced inside the store mutex. The form handler cannot be the only enforcement path: repository tests call the store directly, and a second `ReserveStock` mutation boundary is exercised concurrently. See [`review-evidence.md`](review-evidence.md).

`StockReservation.commit` evaluates `reservedAfter` and resolves the related StockItem from the same locked pre-state, validates the candidate StockItem, and commits both `StockReservation.status` and `StockItem.reserved` only after every check succeeds. Invariant rejection, missing target, invalid source state, denied access, and declined confirmation leave both records unchanged.

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
verified 275 acceptance facts: all passed
  52 distinct tests, max 14 facts per test
human review required: 3 requirements are not machine-verified
```

`cmd/generate` only derives `generation-request.json` and the explicit `coverage.json`; it does not generate application code. `cmd/feedback` is an experiment-side measurement process. It validates every test reference, removes stale feedback before execution, runs the target's real `go test -count=1 -json ./...`, and publishes succeeded feedback only when every mapped test passes.

## Artifacts

| Artifact | SHA-256 | Purpose |
| --- | --- | --- |
| [`app.forma`](app.forma) | — | Forma source copied into the measured boundary |
| [`generation-request.json`](generation-request.json) | `ebba73e4026b3e58e9dfa7367693fa9f0a882b1448af1a58e7cc93795cbcb3e5` | canonical full request with 275 Facts and three Review Requirements |
| [`coverage.json`](coverage.json) | `84c947b8f71f40f3d9a7746abcdf1c3e0f5c36fad9bd8b71779d0b4ba4494ceb` | exact Fact-to-repository-test mapping; unknown or omitted Facts are rejected |
| [`target/generation-feedback.json`](target/generation-feedback.json) | `704e859e88c5cb2cfe18d98332d59a7380f1579a370758b10606a2740c0921e2` | result derived from the actual target test run |
| [`review-evidence.md`](review-evidence.md) | — | evidence for the three remaining human-only requirements |

## What this probe does not prove

- The three Review Requirements are still awaiting human review; a passing race detector and focused tests do not complete those architectural reviews.
- This is a bounded in-memory target, not evidence for durable or distributed transaction behavior, an existing production repository, or reproducibility by a second independent agent.
- The compiler does not yet emit a general application-level fixture-fidelity Review Requirement. This experiment closes its measured access boundary by mapping every access Fact to an actual HTTP request, but a target-neutral review subject and completion contract for arbitrary future surface Facts remain a language-design question.
- The target is not a reusable Go generator, framework profile, or Forma runtime.

The target is intentionally standalone. It is evidence that Forma determines application meaning while the AI produces a normal application, not evidence for a reusable Go generator or shared application runtime.
