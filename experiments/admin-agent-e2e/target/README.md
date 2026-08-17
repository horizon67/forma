# Generated target application

This standalone Go module is the controlled target for the admin agent-generation
experiment. Forma did not generate these files with a framework lowerer. A coding
agent first implemented them from a full Generation Request, then updated the
same repository from an incremental request while preserving the conventions in
[`ARCHITECTURE.md`](ARCHITECTURE.md).

Run its repository-native checks with:

```bash
go test ./...
go vet ./...
```

Run the application with:

```bash
go run ./cmd/server
```

The seed authentication boundary accepts an `X-Role: admin` request header or
a `role=admin` cookie. Authentication itself is outside the current Forma
example; only the `admin` authorization requirement comes from Forma.

The incremental update added an optional nickname to list, detail, edit, and
search behavior and changed the logical page size from 20 to 10. It retained the
existing package layout and tests. [`forma.implementation.yaml`](forma.implementation.yaml)
also fixes the minimum implementation policies used by this update.

[`generation-feedback.json`](generation-feedback.json) maps all 43 Acceptance
Facts to repository-relative test references and reports all three implementation
policies. The Forma repository validates it against the immutable incremental
request with:

```bash
go run ./cmd/forma verify \
  --repository experiments/admin-agent-e2e/target \
  --baseline internal/agentrequest/testdata/admin.request.json \
  internal/agentrequest/testdata/admin.incremental.request.json \
  experiments/admin-agent-e2e/target/generation-feedback.json
```

The historical full-run source, feedback, and Git identities remain under
[`../baseline`](../baseline) and [`../baseline.json`](../baseline.json). The
applied incremental request and target identities are fixed separately in
[`../incremental-baseline.json`](../incremental-baseline.json).
