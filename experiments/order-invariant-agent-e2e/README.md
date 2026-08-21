# Order Invariant Agent E2E

Status: machine verification complete — 278/278 Acceptance Facts passed; four compiler-owned Review Requirements await human review.

This experiment gives the full Generation Request derived from [`app.forma`](app.forma) to an AI coding agent and records the resulting ordinary Go application under [`target/`](target/). The target does not import Forma, embed a Forma runtime, or use a domain-specific generator. Framework choice, package layout, persistence boundary, HTTP routes, and repository-native tests belong to the generated application.

The experiment measures the first P3 Expression/Invariant slice, bounded Changes/atomic post-state, and the first one-hop relation value expression across one complete order and inventory application rather than isolated compiler fixtures.

## Measured boundary

```text
Forma source                     app.forma
Generation Request              278 Acceptance Facts
Review Requirements             4 (concurrency, atomic Changes, cross-entity write authorization, cross-entity value-read authorization)
Target application              standard-library Go HTTP application
Repository tests                52 distinct mapped tests
Implementation policies         none
```

The target includes Customers, Products, Orders, OrderLines, StockItems, StockReservations, ReservationPlans, order lifecycle transitions, role authorization, list queries, server-rendered surfaces, validation feedback, navigation, idempotent form dispatch, an authoritative stock invariant, and an action that changes two entities atomically. Every one of the 243 Facts whose subject is a `page/...` surface is mapped to at least one test that issues a request through the actual HTTP handlers. This includes all 104 access Facts, form mutation and replay, search/filter/sort, a reachable second page after the 20-record boundary, transition outcomes for every source state, confirmation accept/decline, and every Changes outcome. Pure domain/store tests remain supplementary checks and cannot be the only evidence for a page Fact.

`reserved <= onHand` is enforced inside the store mutex. The form handler cannot be the only enforcement path: repository tests call the store directly, and a second `ReserveStock` mutation boundary is exercised concurrently. See [`review-evidence.md`](review-evidence.md).

`StockReservation.commit` resolves the related StockItem target and the distinct ReservationPlan value source from the same locked pre-state, reads `plan.approvedReserved` rather than the decoy self field `requestedReserved`, validates the candidate StockItem, and commits both `StockReservation.status` and `StockItem.reserved` only after every check succeeds. Invariant rejection, missing target, missing value source, invalid source state, denied access, and declined confirmation leave the source and every resolved target unchanged.

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
verified 278 acceptance facts: all passed
  52 distinct tests, max 14 facts per test
human review required: 4 requirements are not machine-verified
```

`cmd/generate` only derives `generation-request.json` and the explicit `coverage.json`; it does not generate application code. `cmd/feedback` is an experiment-side measurement process. It validates every test reference, removes stale feedback before execution, runs the target's real `go test -count=1 -json ./...`, and publishes succeeded feedback only when every mapped test passes.

## Artifacts

| Artifact | SHA-256 | Purpose |
| --- | --- | --- |
| [`app.forma`](app.forma) | — | Forma source copied into the measured boundary |
| [`generation-request.json`](generation-request.json) | `141595b3fdd11e3f1bdb6dc36e132149813a53623d71accec9eec86b8bf75954` | canonical full request with 278 Facts and four Review Requirements |
| [`coverage.json`](coverage.json) | `55a6640025b0e740d7b1e528b59a34337db1701e8a4d9d48ab57ecf854ceb848` | exact Fact-to-repository-test mapping; unknown or omitted Facts are rejected |
| [`target/generation-feedback.json`](target/generation-feedback.json) | `f6350d135df38bbd4e12905f5acfe12efde1e587cdd87fcc67fdcc5dae48b02b` | result derived from the actual target test run |
| [`review-evidence.md`](review-evidence.md) | — | evidence for the four remaining human-only requirements |

## What this probe does not prove

- The four Review Requirements are still awaiting human review; a passing race detector and focused tests do not complete those architectural reviews.
- This is a bounded in-memory target, not evidence for durable or distributed transaction behavior, an existing production repository, or reproducibility by a second independent agent.
- The compiler does not yet emit a general application-level fixture-fidelity Review Requirement. This experiment closes its measured access boundary by mapping every access Fact to an actual HTTP request, but a target-neutral review subject and completion contract for arbitrary future surface Facts remain a language-design question.
- The target is not a reusable Go generator, framework profile, or Forma runtime.

The target is intentionally standalone. It is evidence that Forma determines application meaning while the AI produces a normal application, not evidence for a reusable Go generator or shared application runtime.
