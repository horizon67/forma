# Target architecture

This directory is the AI-produced application measured by the order invariant E2E experiment. It is a separate Go module and has no dependency on Forma.

- `internal/domain` owns entities, order lifecycle semantics, roles, capabilities, and the pure stock predicate.
- `internal/store` owns the authoritative in-memory persistence boundary, uniqueness, idempotent form dispatch, query behavior, and atomic invariant enforcement.
- `internal/web` owns HTTP authorization, server-rendered surfaces, form parsing, observable feedback, and redirects.
- `cmd/server` composes and runs the ordinary application.

The in-memory repository is an implementation choice for the experiment, not a Forma runtime. Its mutex is the transaction boundary: relation target/value resolution, pre-state value read, stock post-state calculation, invariant validation, and commit happen while the same lock is held.
