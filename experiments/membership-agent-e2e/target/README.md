# Generated target application

This standalone Go module is the controlled target for the membership
agent-generation experiment. It is a copy of the admin target produced by
[`../../admin-agent-e2e`](../../admin-agent-e2e/README.md), carried forward by a
coding agent that applied one more incremental Generation Request — the one that
adds Identity. Forma did not generate these files with a framework lowerer; the
conventions in [`ARCHITECTURE.md`](ARCHITECTURE.md) were preserved throughout.

Run its repository-native checks with:

```bash
go test ./...
```

```bash
go vet ./...
```

Run the application with:

```bash
go run ./cmd/server
```

The admin surface keeps its seed authorization boundary: an `X-Role: admin`
request header or a `role=admin` cookie. That boundary is unrelated to the
membership session — a signed-in member is not an admin.

The Identity update added signup with emailed verification, resend, sign-in,
sign-out, and a self-owned profile under `/members`. Credentials are stored as
PBKDF2-HMAC-SHA256 derivations and verification evidence only as a token digest,
so neither can be read back out of the store.
[`forma.implementation.yaml`](forma.implementation.yaml) fixes the minimum
implementation policies used by this update.

[`generation-feedback.json`](generation-feedback.json) maps all 81 Acceptance
Facts to repository-relative test references and reports all three implementation
policies. It is written by a generator that retracts the previous record before
it starts, runs this module's test suite, and only then publishes a new record
with a single rename. A failed run therefore leaves no feedback at all rather
than leaving the last passing one in place. The Forma repository validates it
against the request with:

```bash
go run ./cmd/forma verify --repository experiments/membership-agent-e2e/target --baseline internal/agentrequest/testdata/admin.incremental.request.json experiments/membership-agent-e2e/generation-request.json experiments/membership-agent-e2e/target/generation-feedback.json
```

The admin experiment's own artifacts — the historical full-run source, its
feedback, and the applied incremental request — remain under
[`../../admin-agent-e2e`](../../admin-agent-e2e/README.md) and are not modified
by this experiment.
