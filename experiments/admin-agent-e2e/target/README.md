# Generated target application

This standalone Go module is the first controlled target for the admin agent
generation experiment. Forma did not generate these files with a framework
lowerer. A coding agent implemented them using the Generation Request while
preserving the repository conventions in [`ARCHITECTURE.md`](ARCHITECTURE.md).

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

[`generation-feedback.json`](generation-feedback.json) maps all 43 Acceptance
Facts to repository-relative test references. The Forma repository validates
it against the immutable golden request with `forma verify`.
