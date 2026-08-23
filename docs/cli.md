# Forma CLI Reference

Status: command contract for the `v0.1.0-alpha.1` release candidate.

Forma has one installed executable, `forma`. It compiles one explicitly
selected source set as one application. Separate examples are separate
applications and must not be passed to one command merely because they share a
directory.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | The requested command completed successfully. |
| `1` | Valid command input reached compilation, verification, or Codex execution, but the requested result failed. |
| `2` | Usage, source selection, installation, authentication, or repository preflight was invalid. |

Unknown commands and options fail with exit `2`. Unsupported Forma syntax
reaches the compiler and fails with exit `1` and source-mapped diagnostics.
Artifact schema mismatches and unsuccessful Generation Feedback fail with exit
`1`; missing artifact files are command-input errors and fail with exit `2`.

## Process and mutation summary

| Command | Output | Starts a child process | Network capable | Mutates an application repository |
| --- | --- | --- | --- | --- |
| `version` / `--version` | binary version text | no | no | no |
| `authoring-context` | bundled Markdown | no | no | no |
| `check` | validation summary or diagnostics | no | no | no |
| `resolve` | Resolved Intent JSON | no | no | no |
| `project` | deterministic read-only projection | no | no | no |
| `request` | Generation Request JSON | no | no | no |
| `verify` | verification and human-review summary | no | no | no |
| `generate` | Codex summary, prompt digest, Git status, review instructions | trusted Git and Codex executables | yes, through Codex | yes, through Codex |

`generate` is the only compiler command that asks an AI to edit code. Forma
does not run the generated application's build, tests, server, or feedback
adapter on the host.

## Commands

### `forma version`

```sh
forma version
forma --version
```

Prints `forma <version>`. A tagged install reports its exact tag. An untagged
source or pseudo-version build reports `devel`, a short revision when
available, and `dirty` when applicable.

### `forma authoring-context`

```sh
forma authoring-context > forma-authoring-context.md
```

Emits a self-contained, version-matched language guide and complete examples
for an AI that writes `.forma` source. It does not contact a documentation site
or an AI provider.

### `forma check`

```sh
forma check <file.forma | directory>...
```

Parses, resolves, type-checks, and semantically validates one application.
Directories are walked for `.forma` files. Declaration order, argument order,
and source path do not create implicit namespaces or entry pages.

### `forma resolve`

```sh
forma resolve <file.forma | directory>... > resolved-intent.json
```

Emits canonical Resolved Intent JSON followed by one newline.

### `forma project`

```sh
forma project navigation <source...>
forma project outcomes <source...>
forma project states <source...>
forma project flow <source...>
```

Emits deterministic, read-only human views. A projection never becomes a
second application source of truth.

### `forma request`

```sh
forma request [--manifest <policy.yaml>] <source...>
forma request --previous <request.json> [--manifest <policy.yaml>] <source...>
```

Emits a canonical full or bounded incremental Generation Request. The command
does not invoke Codex. `--previous` identifies an immutable baseline request;
unsupported removals and unchanged inputs fail rather than being guessed.

### `forma generate`

```sh
forma generate --repository <directory> \
  [--manifest <policy.yaml>] [--allow-dirty] <source...>
```

Builds a full canonical request in memory and passes the exact implementation
prompt to an authenticated Codex CLI. The target must be in a Git worktree with
an existing commit. Forma locks the worktree and rejects Git-visible dirty or
hidden-index state unless the documented explicit exception applies.

The command prints the SHA-256 of the complete prompt, including the canonical
request, but does not write that prompt or request into the target. After Codex
returns it prints Git status and stops. Review the diff and confirm that
boundary assertions actually ran before executing repository commands.

`--allow-dirty` bypasses only the ordinary dirty-worktree rejection. It does
not bypass worktree locking, unsafe ownership, or hidden-index checks, and the
final status can no longer be attributed solely to Codex.

### `forma verify`

```sh
forma verify [--repository <directory>] [--baseline <request.json>] \
  <request.json> <feedback.json>
```

Validates request lineage, exact fact coverage, referenced repository evidence,
Implementation Policy coverage, and current/historical schema compatibility.
It never executes the referenced tests. Human Review Requirements remain human
work and are displayed rather than converted into machine success.

## Artifact versions

The current and accepted historical artifact versions are listed in the
[alpha language profile](alpha-language-profile.md). Unknown fields and
unsupported versions fail closed. The broader artifact semantics are in the
[agent generation model](agent-generation.md).
