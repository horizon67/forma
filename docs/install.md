# Install Forma

Status: alpha preparation. No GitHub Release has been published yet.

## Requirements

- macOS or Linux
- Go 1.24 or later when building from source
- Git for the reference generation workflow
- the exact release-qualified Codex CLI version for the planned
  `forma generate` reference runner; the current design probe qualifies
  `codex-cli 0.144.4`, and the release notes must name the final pinned version

Windows and prebuilt release binaries have not yet been qualified.
`forma generate` rejects a different Codex CLI version at preflight because
the pinned sandbox, project-trust, and hook behavior is version-dependent. An
alpha user may continue explicitly with `--allow-unqualified-codex`; the result
is visibly labeled unqualified and cannot be used as release-qualification
evidence. Missing Codex and versions that cannot satisfy the pinned command
surface remain setup errors.

## Build the current development version

From a clone of this repository:

```sh
go build -o ./bin/forma ./cmd/forma
./bin/forma version
./bin/forma check examples/users.forma
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

Installation and compiler-only commands create no Forma user state. The
planned `forma generate` command does: alpha retains run evidence indefinitely
under `$HOME/Library/Application Support/forma/runs` on macOS or
`${XDG_STATE_HOME:-$HOME/.local/state}/forma/runs` on Linux. Review or copy any
evidence you need, then explicitly remove that Forma state directory if you
want a complete uninstall. Runs made with `--artifacts DIR` remain in that
caller-selected directory instead; `forma generate` prints the exact location
for every result. Generated application files are part of the target repository
and are never removed by uninstalling Forma.

## AI setup

`forma check`, `resolve`, `project`, `request`, and `verify` do not need an API
key. Creating application code requires the reference AI runner and separate
Codex authentication. Saved-login use requires `codex login`; fresh API-key
automation does not and may leave `CODEX_HOME` unset because Forma supplies a
private temporary home for that run. Continue with
[AI integration and credentials](ai-integration.md).

## Release blockers

Before this document can be treated as a released installation contract, the
project must verify clean installation on macOS and Linux, publish checksummed
binaries, and test tag/binary/document version consistency. These checks are
tracked in the [alpha roadmap](roadmap.md#fastest-alpha-cut--v010-alpha1current-priority).
