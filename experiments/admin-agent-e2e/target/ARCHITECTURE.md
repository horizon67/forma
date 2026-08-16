# Target repository conventions

This directory is an ordinary, standalone Go application used as the first
agent-generation target. Its repository-owned choices are:

- `net/http` with server-rendered HTML;
- `internal/domain`, `internal/store`, and `internal/web` package boundaries;
- a store interface with an in-memory implementation for this experiment;
- principal and role information supplied by the existing `internal/auth`
  boundary;
- standard `go test ./...` and `go vet ./...` verification commands;
- no third-party dependencies.

These choices are not part of Forma semantics. The coding agent must preserve
them while implementing the Generation Request. In a production repository,
the authentication boundary and store implementation would be replaced by the
repository's real infrastructure.
