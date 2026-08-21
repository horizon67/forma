# Order and inventory target

Run the application:

```bash
go run ./cmd/server
```

Requests use `X-Roles: admin`, `X-Roles: staff`, or `X-Roles: admin,staff` as the experiment's authentication fixture. The application exposes `/orders`, `/stock-items`, and `/reservations` and uses only the Go standard library. `StockReservation.commit` is the bounded cross-entity Changes implementation: it resolves the related StockItem and ReservationPlan, reads `plan.approvedReserved`, and commits the reservation transition and related stock update in one store lock.

Run repository-native verification:

```bash
go test ./...
go vet ./...
```

Forma is not a dependency of this module. The parent experiment owns the immutable Generation Request, Fact-to-test mapping, measurement command, and human review evidence.
