# Human Review Evidence

Review Requirements:

```text
review/entity/StockItem/invariant/stockAvailable/concurrent-invariant-enforcement
review/action/StockReservation/commit/atomic-changes-enforcement
review/action/StockReservation/commit/cross-entity-write-authorization
```

Status: awaiting human review. This document records evidence; it does not convert the requirement into machine-verified Fact coverage.

## Measured authorization enforcement paths

Authorization is intentionally enforced at the closest authoritative boundary rather than through one experiment-wide wrapper:

| Boundary | Covered operations | HTTP access tests |
| --- | --- | --- |
| `web.authorize` | list/detail/form reads, create/edit submits, and Reservations list | top-level tests issue requests with every principal named by their Facts |
| `store.TransitionOrder` | submit, approve, reject, ship | action-specific HTTP tests reach the store guard through list and detail routes |
| `store.DeleteOrder` | delete | 1 HTTP action test reaches the store guard through list and detail routes |
| `store.CommitStockReservation` | confirmed cross-entity commit | the reservation HTTP test reaches the action-owned staff-only guard; staff succeeds while admin is denied |

The pure `domain.Allowed` role-matrix tests remain useful repository tests but receive no Fact coverage credit. The generator regression test requires every `page/...` Fact—not only access Facts—to reference at least one `internal/web` test.

## Authoritative mutation boundary inventory

`StockItem` storage is private to `internal/store.Store`. The target contains three write paths:

| Boundary | Fields changed | Enforcement |
| --- | --- | --- |
| `PutStockItem` | initial `product`, `location`, `onHand`, `reserved` | holds `Store.mu`, calls `domain.ValidateStock` before insertion |
| `UpdateStockItem` | form-editable `product`, `location`, `onHand`, `reserved` | holds `Store.mu` across read, post-state construction, `ValidateStock`, and commit |
| `ReserveStock` | increments `reserved` | holds `Store.mu` across read, increment, `ValidateStock`, and commit |
| `CommitStockReservation` | sets `StockReservation.status` and related `StockItem.reserved` | holds `Store.mu` across source-state check, target resolution, pre-state value read, candidate validation, and both commits |

No other package can write the private `stockItems` map. The HTTP handler calls `UpdateStockItem`; it does not own or duplicate the invariant.

## Atomic Changes evidence

`internal/store/store_test.go#TestStockReservationCommitIsAtomicAcrossReservationAndStock` observes accepted, invariant-rejected, target-unavailable, and source-state-rejected outcomes. Every rejected case rereads both entities and asserts that neither changed. `TestConcurrentStockReservationCommitsCannotPartiallyViolateInvariant` starts valid and invalid commits together; the valid pair commits, the invalid reservation remains Pending, and stock remains within the invariant.

`internal/web/server_test.go#TestReservationCommitSurfaceObservesEveryAtomicOutcome` repeats those outcome checks through the shipped HTTP surface. `TestReservationCommitConfirmationAndCrossEntityAuthorization` additionally proves that declining confirmation dispatches zero repository calls while acceptance dispatches exactly once.

The implementation uses one mutex rather than a transaction framework because this target is an in-memory application. This is evidence for the bounded target, not a claim about database or distributed transaction behavior.

## Cross-entity authorization evidence

The source page and `StockReservation.commit` are available to `staff`; the existing StockItem edit surface remains `admin`-only. `store.CommitStockReservation` checks the action-owned `ReservationCommit` capability and does not inherit roles from StockItemEdit. `internal/store/store_test.go#TestStockReservationCommitAuthorizationOwnsCrossEntityWritePath` and the HTTP confirmation/access test prove the intentional asymmetry: staff can commit the reservation, while an admin without the staff role is denied and neither entity changes.

## Evidence that enforcement is not UI-only

- `internal/store/store_test.go#TestStockValidationAndInvariantRejectAtomically` calls the store directly and proves invalid post-states leave every stored field and version unchanged.
- `internal/store/store_test.go#TestStockInvariantAcceptsValidAndRejectsInvalidPostStates` covers both predicate results at the repository boundary.
- `internal/web/server_test.go#TestStockItemEditSurfaceValidationInvariantAndNavigation` proves the form reaches the same store boundary and returns `invalid` while preserving input.

## Concurrent operation evidence

`internal/store/store_test.go#TestConcurrentReservationsPreserveStockInvariant` starts two goroutines together against `onHand=10, reserved=0`. Each tries to reserve 6. Exactly one commit succeeds, the other receives `ErrInvariant`, and the final stored post-state is `reserved=6 <= onHand=10`.

The test would expose a stale-read implementation in which both operations validate `0+6 <= 10` before committing a combined reservation of 12. In the target implementation, calculation and commit occur under the same mutex.

## Human review questions

- Are all current mutation boundaries listed above?
- Is invariant evaluation inside the authoritative lock/commit boundary on each path?
- Can any caller mutate a stored `StockItem` or the backing map without those methods?
- Does the concurrent test exercise a genuine conflicting post-state rather than two non-conflicting absolute writes?
- Is enforcement independent of HTTP/UI validation?
- Do source-page access, action access, and destination behavior remain composed without inheriting the StockItemEdit role?
- Are target identity resolution and all pre-state reads inside the same boundary as validation and both commits?
- Can cancellation, process failure, or an error after the first write leave only one entity changed?
