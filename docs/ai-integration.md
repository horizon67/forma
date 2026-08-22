# AI Integration and Credentials

Status: contract for the alpha reference runner; `forma generate` is not yet
implemented.

Forma compilation and AI execution are separate boundaries:

```text
Forma source
  -> deterministic compiler and Generation Request
  -> Codex reference runner
  -> repository-native application code and tests
  -> Generation Feedback
  -> deterministic forma verify
```

The compiler decides language semantics. Codex decides repository-specific
implementation details. The runner must not ask Codex to reinterpret invalid
Forma source or weaken the immutable Generation Request.

## Alpha reference agent

The first alpha supports one official generation path: Codex CLI in
non-interactive mode. The planned command is:

```sh
forma generate \
  --repository ./target \
  --feedback-command forma-feedback \
  app.forma
```

Here `forma-feedback` is a pre-existing executable on the preflight `PATH`.
This minimal form avoids granting Codex permission to create or modify the
adapter; it does not make the repository code that adapter runs safe.

The runner will invoke a fresh, bounded `codex exec --ephemeral` process with
`--ignore-user-config`, `--ignore-rules`, a forced-untrusted
Forma-resolved Git-root candidate, a `workspace-write` sandbox, and the target
repository as its explicit working root. Project-scoped Codex configuration is
not supported in alpha:
any `.codex` entry on the target-to-Git-root search path fails preflight before
Codex starts, except the exact existing user `CODEX_HOME` selected for saved
login or explicitly by the caller when it is outside the target. A target-local
entry and a symlink merely pointing to that state root remain rejected. API-key
mode without an explicit home has no such project-path exception. The
path-keyed untrusted override is additional defense and is recorded as the root
Forma sent, not as proof that Codex adopted the same root.
Forma sets `features.hooks=false` and does not bypass hook trust.
Administrator-enforced managed hooks remain part of the host trust boundary.
It supplies no additional writable directory and explicitly sets
`sandbox_workspace_write.network_access=false`, so commands the agent runs in
the repository cannot make outbound network connections. Codex's own provider
connection is separate. Compiler-only commands do not start Codex, access the
network, edit the target repository, or create runner state.

Forma permits only one `generate` command at a time for a canonical containing
Git worktree. It acquires a non-blocking advisory lock on the worktree directory
before checking dirtiness or reading target content and holds it through final
evidence publication. A concurrent run through the same path, a symlink alias,
or a nested target fails preflight with exit `2`; `--allow-dirty` does not
bypass the lock. This serializes Forma runs, not unrelated host tools that edit
the same repository.

These project-trust, hook, and sandbox-network settings are defined in the
[Codex configuration reference](https://developers.openai.com/codex/config-reference).
The supported Codex CLI version is pinned by the Forma release and must pass a
disposable-repository integration probe that edits a file and runs a harmless
command under this exact non-interactive posture. A different version fails
preflight by default. For alpha dogfood, `--allow-unqualified-codex` permits an
explicitly accepted mismatch, labels the terminal and evidence as
`codex-version: unqualified`, and does not qualify release evidence. It cannot
bypass any other preflight or verification gate.

Codex documents `codex exec` as its script and CI interface and recommends
explicit sandbox permissions:
[Codex non-interactive mode](https://developers.openai.com/codex/non-interactive-mode).

## Authentication

### Local use: saved Codex login

The preferred local path is to install Codex CLI and authenticate it before
running Forma. `codex exec` reuses saved CLI authentication. Forma does not read
or copy Codex's authentication files. It resolves `CODEX_HOME` from the
environment or the documented `~/.codex` default, passes the canonical path to
Codex, and records the path and `saved-login` mode without recording auth-file
names or contents. The selected directory must already exist; when the default
is missing, run `codex login` before `forma generate`. This behavior follows
the official
[Codex environment-variable contract](https://developers.openai.com/codex/config-file/environment-variables).
If a non-empty `CODEX_API_KEY` is also present, API-key authentication takes
precedence over this saved login. Forma prints the selected authentication mode
and state-root origin before Codex starts and again in the final result.

### API key automation

Codex automation accepts `CODEX_API_KEY`. Scope it to the runner invocation and
use it only in an isolated or explicitly host-authorized environment:

```sh
CODEX_API_KEY="..." \
  forma generate \
  --repository ./target \
  --feedback-command forma-feedback \
  app.forma
```

A fresh automation environment does not run `codex login` and does not need a
pre-existing `$HOME/.codex`. If `CODEX_HOME` is unset, Forma creates an
unpredictable private mode-`0700` home under the preflighted trusted run root,
passes it explicitly to Codex, and removes it after the Codex process group is
quiescent. Forma copies neither saved authentication nor the API key into that
home. Codex owns the state-root contents, however, and may write
credential-derived state there, so Forma treats the whole directory as
credential-bearing and never intentionally retains it. After an interrupted
run, the next startup removes a marked same-owner ephemeral home immediately
when its non-blocking run lock proves that no concurrent run owns it; unlike an
adapter-copy directory, no age delay applies. If automation sets `CODEX_HOME`
explicitly, Codex's public contract still requires that directory to exist
before the run, and Forma never treats that caller-owned directory as
disposable. After `SIGKILL`, power loss, or host failure, the private directory
can remain until the next Forma startup, so the trusted run root must be treated
as sensitive storage even though the normal path removes it promptly.

The reference runner will not accept `--api-key`; Forma itself does not write a
credential to disk, copy one into a Generation Request, or print one. This is
not a claim that Codex writes no credential-derived state to `CODEX_HOME`, which
is why Forma isolates and promptly removes the API-key-mode ephemeral home.
Repository feedback commands must run with `CODEX_API_KEY` and
`OPENAI_API_KEY` removed from their child environment.

The Codex parent process receives a small, versioned environment allowlist:
resolved `CODEX_HOME`, `HOME`, exact `PATH`, basic locale/shell/temp values,
optional `CODEX_API_KEY`, and standard proxy/CA variables needed for the
provider connection. Evidence records the non-secret context and authentication
mode, state-root origin, and ephemeral cleanup result, not key, proxy, or state
contents. In addition to Codex's default secret-name filtering, the pinned
runner argv explicitly excludes `CODEX_API_KEY`, `OPENAI_API_KEY`, and
`CODEX_HOME` from repository shell commands; proxy and credential-agent
variables are explicitly excluded as well.

Official Codex guidance warns against job-level credentials when
repository-controlled code runs in the same job. GitHub Actions should use the
Codex GitHub Action or an equivalent credential proxy instead of broadly
exporting a key. See the authentication section of
[Codex non-interactive mode](https://developers.openai.com/codex/non-interactive-mode#authenticate-in-automation).

`OPENAI_API_KEY` is the standard credential for applications that call the
OpenAI API directly. The alpha Forma runner does not implement a raw Responses
API agent loop, so it uses Codex's `CODEX_API_KEY` contract instead. See the
[OpenAI API quickstart](https://developers.openai.com/api/docs/quickstart).

## Secret boundary

Secrets must not appear in:

- `.forma` source;
- Generation Request or Generation Feedback JSON;
- implementation policy files;
- prompts or agent final messages;
- repository files, tests, snapshots, or recorded logs;
- command-line arguments or shell history.

The runner must report only whether authentication is available, never a key
value or credential-derived identifier.

## Execution boundary

Codex edits the target under its `workspace-write` sandbox. The Forma parent
process is outside that sandbox. A feedback executable invoked by the parent
runs with the user's host privileges, as do repository programs or tests that
the adapter starts, even after AI credential variables are removed. Credential
stripping does not make agent-written code trusted. As an alpha prerequisite,
run local generation only in a disposable or isolated environment where host
execution of generated repository code is explicitly acceptable. Otherwise do
not use the local runner, regardless of adapter origin.

By default, `--feedback-command` must resolve to a pre-existing executable
outside the target worktree. A target-local adapter requires
`--allow-target-feedback-command`. If Codex creates or changes that adapter,
execution additionally requires `--accept-agent-feedback-command`; otherwise
Forma prints the adapter path and diff, does not execute it, and exits `3`.
An accepted agent-authored adapter is labeled in run evidence and separately
from machine verification because its fixture fidelity still requires human
review.

### Trusted run root

Run evidence, the temporary immutable adapter copy, and any credential-bearing
API-key-mode ephemeral Codex home share one trusted root outside the target,
every path writable by the Codex sandbox, and every Git worktree. Forma records
the sandbox-writable path set used for this decision and fails preflight with
exit `2` if the installed runtime cannot determine it. It does not assume that
the target cwd is the complete `workspace-write` boundary.

Without `--artifacts`, macOS uses
`$HOME/Library/Application Support/forma` and Linux uses
`${XDG_STATE_HOME:-$HOME/.local/state}/forma`, subject to the same boundary and
exec-capability checks. `--artifacts DIR` selects a different external root; it
cannot opt back into a target-local, agent-writable, or Git-controlled path.
Persistent evidence is written to `runs/<run-id>` below this root, so a later
generation run cannot modify an earlier run's evidence through the repository
sandbox. This is not a tamper-proof audit store against the invoking user,
another process with the same UID, root, storage rollback, or host compromise.

Alpha retains these run directories indefinitely and does not prune them
automatically. Every run prints its exact evidence directory. Evidence may
contain repository paths, Git status, Codex prose and diagnostics, and measured
application behavior; review and explicitly remove runs that are no longer
needed. This retention contract applies to `runs/<run-id>`, not ephemeral Codex
homes or adapter copies. A caller-supplied `--artifacts DIR` is retained under
that caller-owned location by the same policy. Installing Forma and using
compiler-only commands do not create either default directory.

For fresh-target generation where Codex must create the adapter, the broader
permission form is explicit:

```sh
forma generate \
  --repository ./target \
  --feedback-command ./scripts/forma-feedback \
  --allow-target-feedback-command \
  --accept-agent-feedback-command \
  app.forma
```

The flags record adapter provenance and consent; they do not make host
execution safer. Before measurement, Forma terminates the Codex process group
and executes a parent-owned copy of the inspected adapter bytes from outside
the target, with the target as working directory. This closes adapter
replacement through the mutable source path but does not make generated
application or test code trusted.

### Feedback adapter contract

Forma runs the inspected bytes from a randomized, parent-created private
directory outside the target. The directory and exclusively created executable
use mode `0700`. Before Codex starts, Forma validates the selected filesystem
with a separate short-lived execution probe; it creates the eventual randomized
copy directory only after the Codex process group is quiescent. If no such
location is available, preflight exits `2`; Forma does not fall back to the
mutable adapter source. An external `--artifacts` directory can provide a
trusted exec-capable root when the platform user-state location is unsuitable.

The adapter process receives:

- the canonical target repository as its working directory;
- the actual immutable-copy path as `argv[0]`; and
- each `--feedback-arg` unchanged as `argv[1]` onward.

This contract applies equally to pre-reviewed external adapters and
target-local adapters. An adapter must resolve repository files from its
working directory. It must not use `$0`, `${BASH_SOURCE[0]}`, `dirname` of the
executable, or another executable-relative path to find repository resources.
A shell adapter can start with:

```sh
repository_root=$(pwd -P)
```

and invoke repository tools relative to `repository_root`. A common pattern
such as `cd "$(dirname "$0")/.."` is incompatible because `$0` deliberately
names the immutable copy outside the repository.

## Mutation and verification boundary

`forma generate` is the only alpha command authorized to start an AI process
and mutate the target repository. It must:

1. compile the source and build the immutable request before starting Codex;
2. refuse a non-Git target or an unexpectedly dirty tree by default;
3. pass the request without persisting credentials in the repository;
4. enforce the external, target-local, and agent-authored feedback-command
   permissions before executing anything after the agent exits;
5. remove AI credential variables from the feedback command environment;
6. decide success with `forma verify`, not with the agent's prose claim;
7. record adapter origin separately from verification success; and
8. leave changes reviewable and never commit or push them automatically.

The exact command and failure contract are specified in the
[reference runner proposal](reference-agent-runner-proposal.md).

## User-owned costs and external state

The user owns provider billing, rate limits, model availability, network
access, and the effects of repository commands run by the coding agent. The
runner must make timeouts and non-zero exits visible and must not retry without
a documented bound.
