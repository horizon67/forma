# Forma CLI Reference

Status: command contract for the `v0.1.0-alpha.1` release candidate.

Development extension: automatic generation history, explicit `generate --previous`,
and Generation Request `v0alpha5` are available in the current source build, not the published
`v0.1.0-alpha.1` binary. Existing `v0alpha4` requests remain valid inputs.

Development builds also include [provider-independent generation progress](generation-progress.md)
and a Codex adapter. Progress is automatic; `--progress=text|json`, `--verbose`,
and `--heartbeat-interval <1s..5m>` customize observation only. JSON mode reserves
stderr for progress JSONL, including errors; human result output stays on stdout.
Cancellation and timeout exit `1` and are distinguished in the terminal result.

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
| `generate` (full/update) | Codex summary, prompt digest, Git status, history, review instructions | trusted Git and Codex executables | yes, through Codex | application edits through Codex; local Git metadata through Forma |
| `generate` (automatic no-op) | no-change message, baseline digest, Git preflight result | trusted Git only | no | no |
| `generate --previous` (no-op/import) | no-change message, explicit baseline adoption | trusted Git only | no | local Git metadata only; application files unchanged |

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
Policy-only updates are supported, including changes to policy mode, value,
instruction, or conventions. An omitted `--manifest` inherits the baseline's
Manifest. Unlike `generate`, `request` still fails on unchanged inputs with
exit `1` and no JSON output; a no-op is not a Generation Request wire kind.

### `forma generate`

```sh
forma generate --repository <directory> \
  [--previous <request.json>] [--manifest <policy.yaml>] \
  [--allow-dirty] [--progress=text|json] [--verbose] \
  [--heartbeat-interval <duration>] <source...>
```

Without `--previous`, selects the last completed comparison baseline from local
per-worktree history for this target, branch, and source selection. A genuinely
fresh target without history uses full generation; otherwise canonical Resolved
Intent, Acceptance Facts, Review Requirements, and Implementation Policy select
incremental update or no-op. An omitted `--manifest` inherits the selected
baseline's Manifest; an explicit one replaces it for comparison.
Policy additions and changes are supported.
Policy removal/ID replacement and unsupported semantic removals fail closed.

An existing target without history, invalid history, an unfinished generation,
or incompatible checkout history stops with exit `1`, not full generation or
normal no-op. `--previous` explicitly selects a validated baseline in preference
to automatic history and can adopt/re-bind an existing application or recover
an unfinished attempt. Corrupt history is never silently discarded, even with
`--previous`. See [history identity, state transitions, and recovery](generation-history.md).
The failure output identifies the history file and current key; branch/selector
mismatches also list existing keys. Invalid catalogs must be restored or backed
up and moved aside with their pending marker **before** explicit re-import;
`--previous` alone does not repair them.

Policy `instruction` and advisory `conventions` changes select an update even
though Forma does not interpret their prose. YAML comments, formatting, and
canonicalized entry order do not. Source Map paths/coordinates, prompt text,
and Request history are excluded from the change decision; the baseline
identity still hashes the complete canonical baseline Request.

Advisory convention additions and removals are explicit `conventionChanges`
entries (`kind: added | removed`, `value`), sorted by value; a text edit is a
removal plus an addition. Removing advice does not require the opposite behavior
or authorize code deletion or unrelated refactoring. Convention-only changes
select an incremental update, which may legitimately finish with zero diff.

If there are no changes, prints `no application or policy changes` and exits
`0` after Git preflight. It never looks up Codex, checks authentication, or
starts an agent. Automatic no-op does not write history or modify application
files. An explicit `--previous` no-op imports that supplied Request into local
history, with no agent run recorded, and explicitly warns that the caller is
asserting the target already implements that Request. This is adoption, not
initial generation: it does not generate missing code. No-op means no application
update was applied, not that repository behavior or tests passed.

If there are changes, sends an incremental request to Codex with the complete
current constraints and explicit change sets. Instructions limit edits to
the requested delta and necessary related code, dependencies, build/CI,
tests, and documentation. Unchanged Facts remain required for regression
verification. An already-compliant implementation may finish with zero diff.
These edit-scope instructions require human review of the actual diff; Forma
does not mechanically prove that every implementation change was related.

The target must be in a Git worktree with
an existing commit. Forma locks the worktree and rejects Git-visible dirty or
hidden-index state unless the documented explicit exception applies. This
preflight also applies to no-op: invalid targets or a dirty worktree without
`--allow-dirty` fail with exit `2`, even if the request has no changes.

The command prints the SHA-256 of the complete prompt. It saves the exact
canonical Request in the worktree's Git metadata, not in application files.
After Codex returns it prints Git status and stops. Review the diff and confirm that
boundary assertions actually ran before executing repository commands.
Codex completion (exit `0`) is not verification success. Unrun/skipped checks
must be reported as unverified, and Human Review Requirements remain open.

The canonical baseline digest proves Request lineage, not repository correctness.
The comparison baseline advances only after successful execution, complete Git
evidence, unchanged checkout identity, and durable history completion. It is not
a verified baseline: history retains `unverified` tests and `pending` human review.
Agent-reported checks and separate `forma verify` runs do not automatically
upgrade those fields. Repair and audit remain separate future workflows;
`generate` never switches to them automatically.

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
