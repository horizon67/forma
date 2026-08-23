# Install Forma

Status: alpha preparation. No GitHub Release has been published yet.

## Requirements

- macOS or Linux
- Go 1.24 or later when building from source
- Git for the reference generation workflow
- Codex CLI for the `forma generate` reference runner

Windows and prebuilt release binaries have not yet been qualified.
The release notes will name the Codex CLI version used for alpha dogfood.
Alpha.1 is a thin runner for repositories the invoking user owns and trusts;
it does not claim hostile-repository containment from a version matrix.

## Build the current development version

From a clone of this repository:

```sh
go build -o ./bin/forma ./cmd/forma
./bin/forma version
./bin/forma authoring-context > /tmp/forma-authoring-context.md
./bin/forma check examples/users.forma
codex login status
```

A source build without release metadata reports `forma devel`, optionally
followed by a short revision and `dirty` marker, for example
`forma devel 7c507ab dirty`. It never presents a Go pseudo-version as a tagged
Forma release.

## Install the tagged alpha

The following command becomes supported only after `v0.1.0-alpha.1` is
published:

```sh
go install github.com/horizon67/forma/cmd/forma@v0.1.0-alpha.1
forma version
```

The installed binary must report the same tag. The release will also provide
versioned macOS and Linux binaries with SHA-256 checksums. Do not substitute an
untagged `@latest` build when reproducing an alpha Generation Request.
An untagged module-proxy build reports `forma devel <revision>` using the
revision embedded in its Go pseudo-version, so two development binaries remain
distinguishable even when VCS build settings are absent.

## PATH

`go install` normally writes the binary to `GOBIN`, or to `GOPATH/bin` when
`GOBIN` is unset. Add that directory to `PATH` if `forma` cannot be found.

## Upgrade and uninstall

Install a new explicit tag with the same `go install` command. Removing the
single `forma` binary from the installation directory uninstalls the
executable. Forma does not install a runtime or modify application repositories
during installation.

Installation and compiler-only commands create no Forma user state. The thin
alpha `forma generate` edits only the target repository through Codex and does
not install a Forma runtime or persistent external evidence store. Generated
application files remain part of the target repository and are never removed
by uninstalling Forma.

## AI setup

`forma authoring-context`, `check`, `resolve`, `project`, `request`, and
`verify` do not need an API key. Creating application code requires the
reference AI runner and separate Codex authentication. Use `codex login` for
ChatGPT authentication. For API-key authentication, follow the official CLI
flow before running Forma:

```sh
printenv OPENAI_API_KEY | codex login --with-api-key
codex login status
```

Forma reuses the resulting Codex login and does not accept an API key as a
Forma argument or write it into the application repository. Continue with
[AI integration and credentials](ai-integration.md).

## Release blockers

Before this document can be treated as a released installation contract, the
project must verify clean installation on macOS and Linux, publish checksummed
binaries, and test tag/binary/document version consistency. These checks are
tracked in the [alpha roadmap](roadmap.md#fastest-alpha-cut--v010-alpha1current-priority).
