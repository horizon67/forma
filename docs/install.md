# Install Forma

Status: installation contract for the `v0.1.0-alpha.1` release candidate.

## Requirements

- macOS or Linux
- Go 1.24 or later when building from source
- Git for the reference generation workflow
- Codex CLI for the `forma generate` reference runner

Windows has not been qualified. The release workflow qualifies source install
and the release binary on macOS and Linux. The release notes name the Codex CLI
version used for alpha dogfood.
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

## Install the tagged alpha with Go

Use the explicit tag after it appears on the
[GitHub Releases page](https://github.com/horizon67/forma/releases):

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

## Install a prebuilt binary

Download `SHA256SUMS` and the archive matching the machine from the same GitHub
Release. The published archive names are:

```text
forma_0.1.0-alpha.1_darwin_amd64.tar.gz
forma_0.1.0-alpha.1_darwin_arm64.tar.gz
forma_0.1.0-alpha.1_linux_amd64.tar.gz
forma_0.1.0-alpha.1_linux_arm64.tar.gz
```

Set the downloaded archive name and verify that one entry before extracting:

```sh
ARCHIVE=forma_0.1.0-alpha.1_darwin_arm64.tar.gz

# Linux
grep "  $ARCHIVE$" SHA256SUMS | sha256sum -c -

# macOS
grep "  $ARCHIVE$" SHA256SUMS | shasum -a 256 -c -
```

Extract the selected archive and place its single `forma` executable in a
directory on `PATH`, for example `$HOME/.local/bin`. Then run:

```sh
forma version
forma authoring-context > /tmp/forma-authoring-context.md
```

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

Development builds additionally retain local generation comparison history in
the per-worktree Git directory under `forma/`. It is not removed by uninstalling
the binary, committed, or transferred by clone. See
[generation history](generation-history.md) for exact paths, retention, and
safe adoption/recovery. Automatic no-op does not require Codex authentication.

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

## Release qualification

`scripts/release-check.sh` is the local release gate. CI runs it on clean macOS
and Linux workers. It uses a fresh Go module/build cache to install Forma, runs
the public compiler workflow twice to prove deterministic output, exercises the
membership and order repository E2E targets, and builds all four release
archives with checksums.

On a tag, the release workflow additionally runs
`GOPROXY=direct go install github.com/horizon67/forma/cmd/forma@<tag>` on both
operating systems, checks the installed binary version, reverifies downloaded
release-asset checksums, and publishes the immutable tag as a GitHub
pre-release. This is deliberately an unauthenticated end-user installation
probe and therefore requires the repository to remain public; the workflow
stops with a dedicated diagnostic if it is private. See the
[alpha roadmap](roadmap.md#fastest-alpha-cut--v010-alpha1current-priority).
