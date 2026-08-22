# Codex Reference Agent Runner Proposal

Status: alpha implementation contract — pre-implementation design frozen after
the 2026-08-22 alpha release-preparation review. Reopen it only for a
correctness or security blocker discovered during implementation.

## Decision

The first public alpha must let a user write Forma and obtain application code,
not merely stop at a Generation Request. It will therefore include one
official reference path:

```text
forma generate -> codex exec -> repository feedback command -> forma verify
```

This is a bounded integration with Codex CLI, not a provider abstraction and
not a raw OpenAI Responses API agent loop. The compiler and interchange
formats remain independent of Codex.

## Why Codex CLI first

A direct model response is not enough to build an application. The agent needs
repository inspection, file editing, commands, tests, sandboxing, and a bounded
non-interactive lifecycle. Codex CLI already provides that execution layer.
Official Codex documentation defines `codex exec` for scripts and CI, supports
stdin context, an ephemeral session, and explicit `workspace-write` sandboxing:
[Codex non-interactive mode](https://developers.openai.com/codex/non-interactive-mode).

Implementing the same tool loop against the Responses API would add a second
agent runtime before Forma has completed its first external dogfood. Multiple
providers and a raw API runner remain post-alpha work.

## Command surface

The first slice adds:

```text
forma generate \
  --repository DIR \
  --feedback-command EXECUTABLE \
  [--feedback-arg ARG]... \
  [--manifest POLICY.yaml] \
  [--allow-target-feedback-command] \
  [--accept-agent-feedback-command] \
  [--allow-dirty] \
  [--allow-unqualified-codex] \
  [--artifacts DIR] \
  <file.forma | source-directory>...
```

Decisions:

- `generate` is a command of the distributed `forma` binary so the first-use
  path is discoverable. Internally it is a separate orchestration component and
  may not change compiler semantics.
- Codex is fixed in alpha. There is no `--provider`, implicit provider search,
  model fallback, or provider-specific data in Generation Request.
- The runner resolves the `codex` executable before mutation and records its
  version in run evidence. Alpha accepts the release-qualified Codex CLI
  version by default. A missing executable is always a usage/setup error; a
  different parseable version requires `--allow-unqualified-codex`.
- `--allow-unqualified-codex` relaxes only the version equality check. It does
  not relax capability, sandbox, project-configuration, run-root, adapter, or
  verification gates. The terminal and run evidence distinguish an
  `unqualified` runner from a release-qualified one, and release qualification
  refuses this flag.
- Project-scoped Codex configuration is outside the alpha contract. There is no
  acknowledgement flag that permits a target `.codex` layer.
- The repository-specific feedback executable and every argument are separate
  argv elements. The runner never invokes a shell or parses a command string.
- The feedback executable must resolve outside the target worktree by default.
  A target-local executable requires `--allow-target-feedback-command`.
  Executing one created or modified by the agent additionally requires
  `--accept-agent-feedback-command`.
- `--artifacts DIR` selects the trusted external run root for persistent
  evidence, the temporary adapter copy, and any API-key-mode ephemeral Codex
  home. When omitted, the runner uses the platform user-state location defined
  below; it never defaults inside the target or beside a possibly nested target
  directory.
- One Codex process is started. There is no automatic outer retry in the first
  slice; the user reviews a failed run before starting another.
- The command never commits, pushes, deletes unrelated files, or rewrites Forma
  source.

## Preflight

<!-- Editing convention: in a numbered contract list, only the penultimate
item ends with "; and"; earlier items end with semicolons. -->

Before starting Codex, the runner must:

1. resolve and canonicalize the source and target paths;
2. reject a target that is not a Git worktree;
3. acquire the canonical containing worktree's exclusive generation lock;
4. reject a dirty target unless `--allow-dirty` is explicit;
5. reject a source path that resolves through a symlink outside the allowed
   source root;
6. compile the source and stop on any diagnostic;
7. build and canonically validate the Generation Request;
8. validate the implementation policy when supplied;
9. determine the authentication mode and Codex state-root input, then reject
   project-scoped Codex configuration on the target's project-search path;
10. resolve `codex` to an absolute executable and require the release-qualified
   version unless the explicit unqualified-version flag was supplied;
11. snapshot the exact Forma source and policy bytes;
12. resolve, classify, and snapshot the feedback executable candidate;
13. resolve and record the complete conservative set of paths writable by the
    exact agent sandbox configuration;
14. select the trusted external run root and verify it with the short-lived
    private execution probe described below;
15. materialize the selected `CODEX_HOME`, including a private ephemeral home
    for API-key authentication when the caller did not set one; and
16. construct the immutable prompt completely before mutation.

An external feedback executable must exist at preflight. A missing executable
is allowed only when its canonical candidate path is inside the target and
`--allow-target-feedback-command` was supplied; it is then recorded as a
possible `agent-created` adapter. A target-local path without that flag fails
preflight. Supplying `--accept-agent-feedback-command` without the target-local
flag is invalid usage.

Compiler failure, unsupported alpha syntax, missing policy input, or schema
version mismatch must not start the agent.

Immediately after resolving the canonical containing Git worktree, Forma opens
that worktree directory without following a symlink and acquires a non-blocking
exclusive advisory lock on its file descriptor. The descriptor is
close-on-exec, is held through agent execution, feedback, verification, and
artifact publication, and is released only when the command returns. Because
the lock is on the canonical containing worktree rather than the spelling of
`--repository`, symlink aliases and nested target paths serialize whenever the
configured sandbox could write the same worktree. `--allow-dirty` does not
bypass this guard. Contention or a filesystem that cannot establish the lock is
preflight exit `2` before dirty-tree or target-content inspection and before
Codex startup; `SIGKILL` and process exit release the advisory lock in the OS.
The lock serializes Forma generation runs only and does not claim to stop an
unrelated host process from editing the repository.

Forma first determines the authentication mode. A non-empty `CODEX_API_KEY`
selects `api-key-environment`; otherwise the mode is `saved-login`. An explicit
`CODEX_HOME` is made absolute and canonicalized and must already resolve to an
existing directory in either mode, matching Codex's public environment-variable
contract. In saved-login mode, an absent `CODEX_HOME` selects the documented
`$HOME/.codex` default, which must also already exist; otherwise preflight exits
`2` and tells the caller to run `codex login` first.

API-key mode does not require a prior login or a pre-existing default
`$HOME/.codex`. When `CODEX_HOME` is absent in that mode, Forma waits until the
trusted run root has passed its invariants, then exclusively creates an
unpredictable mode-`0700` directory there with prefix
`.forma-codex-home-v1-`. It passes that existing canonical directory to Codex
as the explicit `CODEX_HOME` and removes it after the Codex process group is
quiescent on every handled result. Forma copies no saved authentication or API
key into that directory, but Codex owns the state-root contents and may write
credential-derived state there. The runner therefore treats the entire
ephemeral home as credential-bearing for its lifetime: mode `0700`, never
included in evidence beyond its canonical path, origin, and cleanup result, and
never retained intentionally. Forma records the selected path and one of
`environment`, `default`, or `ephemeral-api-key`, but never records, reads, or
copies its contents.

Forma then resolves the canonical target and containing Git worktree and
examines the target directory and each ancestor through that worktree root with
`lstat`. A `.codex` entry is normally preflight exit `2`; the runner does not
follow it or start Codex. The sole exception is the exact logical existing user
state entry selected from an explicit `CODEX_HOME`, or from the saved-login
default, when that entry lies outside the canonical target directory. This
permits a user state directory such as `$HOME/.codex` when an outer dotfiles
worktree is rooted at `$HOME`. API-key mode without an explicit `CODEX_HOME`
has no exception on this chain because its selected home will be created later
under the trusted run root. The exception never permits a target-local
`.codex`, nor does another target entry become exempt merely because its
symlink resolves to the same state directory. Every other `.codex` file,
directory, or symlink on the chain remains rejected.

This is the primary control that keeps project-scoped Codex configuration out
of alpha runs. It does not depend on Forma and Codex choosing the same textual
project root, and alpha provides no project-configuration override. Supporting
reviewed project configuration is deferred until Codex exposes an
effective-configuration handshake that the runner can verify or Forma defines
a separate isolation contract.

The trusted run root is one canonical directory shared by persistent run
evidence, temporary adapter copies, and API-key-mode ephemeral Codex homes.
`--artifacts DIR` selects it explicitly.
Without that option, macOS uses
`$HOME/Library/Application Support/forma` and Linux uses
`${XDG_STATE_HOME:-$HOME/.local/state}/forma`. The chosen root must, in this
order:

1. be outside the target worktree;
2. be disjoint from every path writable by the exact agent sandbox; and
3. not be inside any Git worktree.

The writable-path set includes at least the target's containing Git worktree
and every additional or temporary writable root exposed by the Codex
configuration used for the run. The runner records the canonical set in run
evidence. If the installed Codex/runtime contract cannot make that set
determinable, preflight exits `2` instead of inferring it from the target cwd.
This is a Forma containment requirement beyond the public `workspace-write`
permission class.

The root is a real non-symlink directory owned by the invoking user and is not
writable by group or other users. Before Codex starts, the parent creates an
unpredictable mode-`0700` probe directory there, executes a parent-owned probe,
and removes it. In API-key mode without an explicit `CODEX_HOME`, the parent
then creates the private `.forma-codex-home-v1-` directory described above.
After the Codex process group is quiescent, the parent creates a new
unpredictable mode-`0700` directory with prefix `.forma-exec-v1-` in the same
root for the inspected adapter bytes. Thus the eventual adapter-copy pathname
does not exist during the agent stage.

Failure to create, restrict, or execute from the probe directory is preflight
exit `2`; failure of the platform default tells the caller to choose an
external, exec-capable `--artifacts` directory satisfying the same three
invariants. The runner never falls back to executing the inspected source path
or to a shared predictable temporary filename.

The private directories block access by other unprivileged OS users, but are
not a boundary against another process running as the same user or against
root. The adapter directory closes the target-controlled path race; the
ephemeral home limits where Codex may leave credential-derived state. Neither
is a general host sandbox. The runner removes each ephemeral home and
adapter-copy directory on every handled exit path and never reuses a directory
left by an interrupted process. The two prefixes have different startup cleanup
rules:

- `.forma-exec-v1-` may be removed only when it is a real directory owned by
  the invoking user, contains Forma's exclusively created ownership marker, and
  is older than the maximum run duration plus a grace period;
- `.forma-codex-home-v1-` may contain credential-derived state, so it has no age
  delay. Before Codex starts, the parent writes the ownership marker and holds
  an exclusive advisory run lock for the directory's lifetime. A later startup
  first validates the trusted root, then removes a same-owner, marked real
  directory immediately only after acquiring that lock non-blockingly; lock
  contention means a concurrent run may own it and the directory is left
  untouched. Failure to establish and hold the lock is preflight exit `2` and
  never starts Codex.

Neither rule follows a top-level symlink, removes an unknown entry, or removes a
directory that may belong to a concurrent run. Cleanup of an ephemeral home
uses `lstat` throughout: it may unlink a nested symlink entry but never traverses
that entry outside the marked directory.

The initial Git HEAD and porcelain status are recorded in run evidence. A dirty
run is evidence-labeled and must not claim which changes came from Codex.
This clean-tree gate protects Git-visible uncommitted user work; it is not a
tamper-detection boundary against target-controlled `.git` metadata. Forma
disables ambient system/global Git configuration, overrides repository-local
`core.excludesFile` and `core.fsmonitor` for evidence commands, and rejects
`assume-unchanged` or `skip-worktree` index entries, but ignored-file policy and
other repository metadata remain repository inputs. Retry integrity and
cross-run provenance must therefore rely on trusted external snapshots and
evidence rather than interpreting a clean porcelain status as proof that an
earlier agent could not hide state.
After Codex exits, every source and request-construction input is compared with
the preflight snapshot before measurement. Any change fails the run; an agent
may implement the request but may not rewrite the intent or policy that defined
it.

## Codex invocation

The reference invocation is equivalent to:

```sh
codex exec \
  --ephemeral \
  --ignore-user-config \
  --ignore-rules \
  --sandbox workspace-write \
  --config 'sandbox_workspace_write.network_access=false' \
  --config 'features.hooks=false' \
  --config 'shell_environment_policy.ignore_default_excludes=false' \
  --config 'shell_environment_policy.exclude=["CODEX_API_KEY","OPENAI_API_KEY","CODEX_HOME","HTTP_PROXY","HTTPS_PROXY","ALL_PROXY","NO_PROXY","http_proxy","https_proxy","all_proxy","no_proxy","SSH_AUTH_SOCK","SSH_AGENT_PID","GIT_ASKPASS","SSH_ASKPASS"]' \
  --config 'projects."<forma-resolved-git-root>".trust_level="untrusted"' \
  --cd <target> \
  "<Forma implementation instruction>"
```

These are separately constructed argv elements, not a shell command. The
runner records the absolute Codex executable and this exact argv array in run
evidence. The prompt argument is recorded because it is part of argv, and is
also covered by the prompt-template digest; it may not contain credentials.

The parent constructs the Codex process environment from an allowlist rather
than forwarding the caller environment wholesale:

- canonical `CODEX_HOME`, `HOME`, the exact preflight `PATH`, basic user,
  locale, shell, terminal, and temporary-directory variables;
- `CODEX_API_KEY` only when it is present for API-key authentication;
- standard HTTP(S)/all-proxy and no-proxy variables, plus standard CA-certificate
  path variables needed by the provider connection; and
- no `OPENAI_API_KEY`, provider/base-URL override, cloud credential, SSH agent,
  Git credential, or unrelated application-secret variable.

The runner records the environment-policy version, exact inherited variable
names, canonical `CODEX_HOME`, its `environment`, `default`, or
`ephemeral-api-key` origin, the exact `PATH`, and `saved-login` or
`api-key-environment` authentication mode. For an ephemeral home it also
records creation and cleanup results, never directory contents. It records only
presence for proxy variables and never records proxy URLs, credential values,
credential digests, auth-file names, or auth-file contents. An invalid explicit
`CODEX_HOME`, a missing saved-login default, failure to create or clean up the
private API-key home, or malformed allowlisted environment input fails without
printing secret values. Forma does not open the saved-auth state merely to
prove that login will succeed.

After the selected state root exists and every preflight item has passed, but
immediately before Codex starts, terminal output prints
`codex-auth: <mode> (CODEX_HOME: <origin>)`. The final result repeats that line
alongside version qualification and any feedback-adapter provenance. Thus a
caller can see that an exported `CODEX_API_KEY` selected
`api-key-environment` instead of saved login without opening run evidence, and
the pre-agent line never describes an ephemeral home that has not been
materialized. No credential value or credential-derived identifier is printed.

Codex itself receives the selected authentication input, but repository shell
commands must not. In addition to Codex's automatic KEY/SECRET/TOKEN
exclusions, the pinned argv explicitly removes `CODEX_API_KEY`,
`OPENAI_API_KEY`, `CODEX_HOME`, proxy variables, and credential-agent variables
from the shell-tool environment. This is defense in depth alongside the
instruction not to inspect or copy authentication state; it does not turn local
execution into a secret-isolation boundary.

The runner supplies no `--add-dir`, `--profile`, `--search`, output-file flag,
or dangerous sandbox, approval, or hook-trust bypass. It asks Codex to treat
Forma's canonical containing Git root as `untrusted` as defense in depth, but
does not claim the path-keyed override was adopted: Codex silently ignores a
key for a different project path. The primary control is the preflight
`.codex` rejection above. `--ignore-rules` independently excludes user and
project exec-policy `.rules` files. `--ignore-user-config` keeps a local
profile, MCP servers, and writable-root additions from changing the run, while
`features.hooks=false` disables ordinary lifecycle hooks; authentication
remains a separate input. The runner never passes
`--dangerously-bypass-hook-trust`. Administrator-enforced managed policy and
hooks are a host trust input and must not be represented as repository policy.

Sandboxed repository commands have outbound network disabled explicitly with
`sandbox_workspace_write.network_access=false`. This does not disable the
Codex client's own authenticated provider connection. Alpha has no flag that
allows the target to opt sandboxed commands back into network access. The
runner treats the containing Git worktree and every runtime temporary write
root as agent-writable during preflight. If the installed Codex version does
not support this exact configuration or Forma cannot establish its effective
writable-path set, preflight exits `2`.

The project-trust, hook, and sandbox-network keys follow the official
[Codex configuration reference](https://developers.openai.com/codex/config-reference).
That reference establishes the keys and says trusted projects may load
project-scoped configuration; it is not evidence that a path-keyed override was
applied to a particular run.

The release-qualified integration probe runs this exact posture in a disposable
Git repository and requires Codex to edit one file and execute one harmless
repository command. On 2026-08-22, `codex-cli 0.144.4` with the forced-untrusted
override created `hello.txt`, ran `ls -la`, and exited `0` under
`approval: never` and `workspace-write`. This establishes that untrusted
project posture does not remove non-interactive edit or command capability for
the currently qualified version. Updating the supported Codex version requires
rerunning this probe; another version fails preflight rather than inheriting the
result by assumption unless the caller explicitly chooses the unqualified
version path. That path still launches the exact pinned argv, but a newer CLI
may interpret or silently ignore configuration differently; those runtime
semantics are user-accepted rather than release-qualified.

The process working directory is the target repository. The canonical
Generation Request is supplied on stdin as additional context; it is not put
in an environment variable or an agent-editable repository file. The
instruction states that:

- the request is immutable and authoritative;
- Forma parsing and semantics must not be reinterpreted;
- implementation belongs only in the target repository;
- repository-native application code and tests must be created or updated;
- a target-local feedback command, when explicitly allowed, must be implemented
  or updated and kept truthful; an external feedback command must not be
  modified;
- tests, coverage mappings, requests, and verification rules must not be
  weakened to obtain green status;
- no commit or push is authorized; and
- the runner, not the agent's final message, decides success.

The prompt template is versioned as runner behavior and its SHA-256 is recorded
with the run. Changing the prompt does not change the language artifact schema,
but does require runner regression tests and release notes.

The runner does not select JSON event output or ask Codex to write an output
file. It captures Codex progress from stderr and the final message from stdout,
each with a fixed maximum size, as diagnostic evidence. Either stream exceeding
its bound terminates the agent stage. The final message is never parsed as
Generation Feedback and cannot make the run pass.

The runner starts Codex as the leader of a dedicated process group. It does not
enter the feedback stage until Codex has exited and that process group has been
terminated and reaped. This lifecycle rule is Forma runner containment beyond
the sandbox behavior documented by Codex; it must not be inferred from
`workspace-write` alone.

## Authentication

`codex exec` reuses saved CLI authentication by default. For automation, Codex
also supports a `CODEX_API_KEY` scoped to one invocation. The runner follows
these rules:

- no API-key command-line flag;
- no prompt, request, feedback, run artifact, or log may contain credential
  values;
- no reading or copying of Codex's saved authentication file;
- an explicit `CODEX_HOME` must already exist, while API-key mode with no
  explicit home uses a fresh private home under the trusted run root and does
  not require `codex login` or `$HOME/.codex`;
- Forma never writes a credential to the private home, but treats the directory
  as credential-bearing because Codex may persist authentication-related state
  under its own state root;
- absence or rejection of credentials is reported without printing values;
- `CODEX_API_KEY` and `OPENAI_API_KEY` are removed from every post-agent child
  process, especially the repository feedback command; and
- API-key mode is documented only for isolated or explicitly host-authorized
  environments because repository-controlled code executes during an agent
  run.

Official Codex documentation defines `CODEX_HOME` as the root for configuration,
authentication, logs, sessions, and other CLI state, with `~/.codex` as its
default, requires an explicitly set home to exist, and defines
`CODEX_API_KEY` for non-interactive processes:
[Codex environment variables](https://developers.openai.com/codex/config-file/environment-variables).
Run evidence identifies the resolved root, its origin, its cleanup result when
ephemeral, and the authentication mode, never the credential or
authentication-file contents.

Official Codex guidance warns against broad job-level credentials when
repository-controlled code runs in the same environment and recommends the
Codex GitHub Action for GitHub Actions. The alpha local runner does not claim to
replace that credential proxy.

## Feedback command

Forma cannot infer how an arbitrary repository builds, tests, or maps tests to
Acceptance Facts. The caller therefore supplies an executable and argv. For an
explicitly allowed target-local executable, the instruction tells Codex to
implement or update that repository-native command.

After Codex exits zero and its process group is quiescent, the trusted parent
process:

1. resolves the feedback executable to the preflight external executable or to
   the canonical target-local path explicitly authorized by the caller;
2. opens it once, validates and records the digest of those exact bytes, and
   writes those bytes with exclusive creation and mode `0700` to a randomized
   path in a private directory on the preflighted root outside the target;
3. constructs a child environment from an allowlist or a credential-stripped
   copy, never the Codex credential-bearing environment;
4. runs the immutable copy directly without a shell, with the target repository
   as its working directory, the actual immutable-copy path as `argv[0]`, and
   each caller-supplied `--feedback-arg` preserved as `argv[1]` onward;
5. captures stdout with a fixed maximum size as Generation Feedback JSON;
6. keeps stderr as bounded diagnostic output;
7. validates the feedback schema and fact/policy coverage; and
8. invokes the same verifier used by `forma verify` against the immutable
   in-memory request.

The parent itself runs outside the Codex `workspace-write` sandbox. Therefore a
target-local feedback executable runs with the invoking user's host privileges,
not with Codex sandbox permissions. Credential stripping limits secret
exposure; it does not make repository code trusted.

The runner records the preflight and final executable path and digest and
classifies the adapter as `external-preexisting`,
`target-preexisting-unchanged`, `agent-created`, or `agent-modified`. A
target-local command is rejected unless `--allow-target-feedback-command` was
supplied. When it is opened for copying, an external executable must still
resolve to the same path and digest captured at preflight. A target-local
executable must be a regular non-symlink file whose canonical parent remains
inside the target; symlink escapes are rejected. The copied bytes, rather than
the subsequently mutable source path, are executed, closing the digest-to-exec
race through that source path. Feedback adapters must resolve repository
resources from the supplied target working directory, not relative to the
copied executable's location.
In particular, a script that changes directory using `$0`, `BASH_SOURCE`, or
another executable-relative lookup is outside the alpha adapter contract. The
copy path in `argv[0]` is intentional: reporting the original path would make
the invoked process claim that mutable source bytes were executed.

If the agent created or changed the adapter, the parent prints its path and a
bounded unified diff, including the content of a new file, and exits `3` before
execution unless `--accept-agent-feedback-command` was also supplied. There is
no implicit interactive confirmation and the acceptance flag is valid only
together with the target-command flag.

An explicitly accepted agent-authored adapter may support a fresh-repository
first generation. Its tests and mappings remain application implementation
subject to human review; canonical request validation prevents omission or
invention of Fact IDs, but cannot prove fixture fidelity by itself. Both the
result display and run artifact must show `feedback-adapter: agent-authored`
separately from the `forma verify` result. For later incremental repair,
protecting an already-reviewed feedback command requires the existing retry
baseline mechanism or a later generalized integrity slice.

The runner does not treat `go test`, `npm test`, or any framework command as a
universal default. The quickstart repository provides one concrete feedback
adapter as an example, not a core protocol rule.

## Artifacts

The parent writes artifacts after the agent stage and, when authorized, the
feedback command have finished, from parent-owned in-memory bytes:

- canonical Generation Request;
- measured Generation Feedback;
- bounded Codex final message and diagnostics;
- initial/final Git state;
- Forma version, Codex version, the absolute Codex executable, and the exact
  Codex argv as a JSON string array rather than a reconstructed shell string;
- Codex version qualification (`qualified` or `unqualified`), the
  release-qualified and installed versions, and whether the explicit override
  was used;
- the Codex environment-policy version, inherited variable names, exact
  `PATH`, resolved `CODEX_HOME`, its origin and ephemeral cleanup result,
  proxy-variable presence, and authentication mode, with credential and proxy
  values excluded;
- the Forma-resolved Git-root candidate and exact trust-level override sent to
  Codex, without claiming Codex adopted that path as its project root;
- the disabled sandbox-command network posture and complete sandbox-writable
  path set;
- prompt-template digest;
- exact feedback argv with credential values excluded;
- feedback-adapter classification plus preflight, final-source, and executed-copy
  digests;
- final verifier result, or the safety gate that prevented measurement.

Persistent output is `<trusted-run-root>/runs/<run-id>`. The run directory is
created exclusively with mode `0700`; `run-id` is globally unique and the
record includes the canonical target path. It is never placed under the target,
even when `.forma-build/` is ignored by Git. Consequently a later Codex run in
the same target cannot rewrite an earlier run's adapter provenance, digests, or
verifier result through its repository sandbox. `--artifacts DIR` changes the
trusted run root, not these invariants.

This protects evidence from current and later agent sandboxes. It is not a
tamper-proof audit store against the invoking user, another process with the
same UID, root, storage rollback, or host compromise.

Alpha keeps persistent run directories indefinitely and performs no automatic
pruning. Every handled result prints the exact run directory. The records can
contain repository paths, Git status, Codex prose, diagnostics, and measured
application evidence, so users must treat the root as potentially sensitive,
copy any evidence they need to retain, and explicitly remove unwanted run
directories. A later cleanup command or bounded-retention option may replace
this policy only as an explicit CLI contract change. Installing Forma and
running compiler-only commands do not create the trusted run root; only
`forma generate` does. A caller-selected `--artifacts DIR` remains
caller-owned and is reported in the same way.

Generation Request and Feedback retain their existing canonical schemas. Run
metadata is an operational record and must not be inserted into either schema
for this slice.

## Result and exit codes

The command returns:

- `0` only when compilation, Codex, the feedback command, and `forma verify`
  all succeed;
- `1` for compiler diagnostics, agent failure, timeout, feedback failure,
  invalid feedback, verification rejection, or a reported blocker;
- `2` for invalid CLI usage or a preflight prerequisite failure, before Codex
  starts;
- `3` when Codex completed but the run stopped before host measurement because
  an agent-created or agent-modified adapter requires review.

Missing permission for a known target-local candidate is preflight exit `2`.
An installed Codex version that differs from the release-qualified version is
also exit `2` unless `--allow-unqualified-codex` is explicit. With that flag,
the run may still return `0` when every agent, feedback, and verification gate
passes, but terminal output and evidence remain visibly labeled
`codex-version: unqualified`; it is not release-qualification evidence.
Missing acceptance for an adapter that Codex actually created or changed is
post-agent exit `3`. No feedback process is started in either case, but exit
`3` guarantees that the target contains a reviewable agent-stage diff and the
run artifact records the `feedback-adapter-review` stop stage.

Human Review Requirements remain visible even after machine verification. Exit
zero means the machine-verifiable contract passed; it does not claim that a
human review was completed.

On failure, repository changes remain reviewable. The runner does not reset or
delete them. It prints the artifact location and the failing stage without
including secrets.

## Bounded execution

The first slice starts one fresh Codex process group with a documented finite
timeout. The timeout has a conservative default and a fixed upper bound. On
normal exit, timeout, cancellation, or parent error, the runner terminates and
reaps the entire group before proceeding or returning. Failure to quiesce the
group is exit `1` and suppresses the feedback command. A deliberately detached
process can escape a POSIX process group, so the immutable executable copy is
still required. No automatic retry, repair decision, or intent-gap publication
is included until the simple generation path has external evidence.

## Security and trust boundaries

```text
deterministic / no network
  source -> check -> request

explicitly mutating / networked
  generate parent -> codex exec in target sandbox

credential-stripped host measurement
  explicitly authorized feedback executable -> feedback bytes -> canonical verify

trusted external run root / outside every agent-writable path and Git worktree
  persistent evidence + credential-bearing ephemeral Codex home + immutable adapter copy
```

The parent owns ordering, immutable request bytes, credential stripping,
bounded capture, and final verification. Codex owns repository edits. The
repository owns build/test implementation. Humans own unresolved Review
Requirements and review of the generated diff.

Authorization of a feedback executable is provenance and consent evidence, not
a claim that host execution is safe. Every adapter class may compile or run
agent-generated repository code with the invoking user's privileges. Local
generation therefore requires an environment where that host execution is
acceptable; otherwise the whole runner must be isolated.

`check`, `resolve`, `project`, `request`, and `verify` must remain usable in an
environment with no Codex installation, no API key, and no network.

## First implementation slice

1. Extract reusable request construction and verification from CLI formatting
   without changing artifact bytes.
2. Add injectable command-runner tests for preflight, version qualification,
   Codex argv/cwd/stdin/environment, and credential-stripped feedback
   execution.
3. Implement the lifetime per-worktree advisory lock, Git clean-tree, and
   trusted-run-root guards, including platform defaults, sandbox-writable-path
   disjointness, and Git-worktree rejection.
4. Implement the saved-login and API-key state-root branches, including a
   credential-bearing private `CODEX_HOME`, a lifetime run lock, and immediate
   lock-gated stale-home cleanup.
5. Implement one full-request generation; add historical baseline support in
   a later incremental-generation slice.
6. Add bounded output, dedicated process-group teardown, and timeout handling.
7. Preflight the external copy root with a short-lived private probe, create a
   fresh private directory only after the agent process group is quiescent,
   execute only an exclusively created parent-owned immutable adapter copy,
   and test its `argv[0]`, permissions, cleanup, and resistance to source
   replacement.
8. Add target-local and agent-authored feedback opt-in gates, adapter-origin
   evidence, and a visually distinct unreviewed-adapter result.
9. Add CLI help, install, AI-auth, security, and quickstart documentation.
10. Run fresh-target dogfood with saved Codex login, then with scoped API-key
   auth in an isolated disposable environment; release evidence uses the
   qualified version without the override, while one separate dogfood test
   fixes the unqualified labeling path.

## Required negative tests

- invalid Forma never starts Codex;
- unsupported artifact version never starts Codex;
- missing Codex and invalid repository fail before mutation;
- a missing, unparseable, or mismatched Codex CLI version without an override
  fails before mutation;
- a different parseable Codex CLI version proceeds only with
  `--allow-unqualified-codex`, stays visibly `unqualified` in output and
  evidence even after verification success, and cannot satisfy the release
  gate;
- dirty tree fails unless explicitly allowed;
- a second `forma generate` for the same canonical containing worktree fails
  preflight before dirty-tree or target-content inspection and before Codex
  startup, including through a symlink alias, nested target path, or
  `--allow-dirty`;
- distinct containing worktrees do not contend, and the lock is released on
  every handled return as well as by the OS after abrupt process termination;
- an unsupported worktree advisory lock fails closed, and the close-on-exec
  descriptor is not inherited by Codex or a detached child;
- the exact existing user `CODEX_HOME` entry outside the target does not fail
  the `.codex` chain scan, while a target-local entry, a different entry, and a
  symlink merely targeting `CODEX_HOME` fail with exit `2` before Codex starts;
- an explicitly configured `CODEX_HOME` path that does not exist fails in both
  authentication modes, and a missing default fails saved-login preflight
  without failing API-key mode;
- API-key mode without an explicit `CODEX_HOME` neither requires nor creates
  `$HOME/.codex`; it creates an unpredictable mode-`0700` home under the trusted
  run root, passes that canonical path to Codex, copies no saved auth, treats
  any Codex-written contents as credential-bearing, and removes the directory
  on every handled result;
- a run root inside the target, overlapping any agent-writable path, or inside
  any Git worktree fails preflight with exit `2` and never starts Codex;
- an adapter-copy root that cannot host a mode-`0700` probe directory or cannot
  execute the parent-owned probe fails preflight with exit `2` and never starts
  Codex;
- changing Forma source or the implementation policy after preflight fails
  before feedback is accepted;
- Codex receives canonical request bytes on stdin and the target as cwd;
- Codex receives the exact pinned argv: `--ignore-user-config`,
  `--ignore-rules`, an explicit `workspace-write` sandbox, disabled sandbox
  network and ordinary hooks, and a forced-untrusted Forma-resolved Git root;
  it receives no
  additional writable directory, profile, search, output-file, or dangerous
  bypass flag;
- target `.rules` and project hooks cannot alter the effective runner
  configuration, while project `.codex` is rejected before Codex starts;
- run evidence records the same absolute executable and argv array that were
  launched, together with the Codex version, effective sandbox posture, and
  the trust path Forma sent without relabeling it as Codex-confirmed;
- the Codex process receives only the versioned environment allowlist; evidence
  records its non-secret context and auth mode while no credential, proxy URL,
  or auth-file content reaches artifacts;
- when a saved-login home is available and `CODEX_API_KEY` is also present,
  `api-key-environment` takes precedence and both the pre-agent terminal output
  and final result report that mode and its state-root origin;
- the pinned shell-environment exclusion array explicitly names
  `CODEX_API_KEY` and `OPENAI_API_KEY`, and repository shell commands do not
  receive `CODEX_HOME`, API keys, proxy variables, credential-agent variables,
  or other secret-named environment;
- the release integration probe proves the supported Codex version can edit and
  execute a harmless repository command with the pinned untrusted posture;
- the runner never adds a credential to argv, prompt, artifact, stdout, or
  stderr;
- feedback command receives neither `CODEX_API_KEY` nor `OPENAI_API_KEY`;
- an external pre-existing feedback command runs without target-command flags;
- a target-local command never runs without `--allow-target-feedback-command`;
- a target-local symlink escape and an externally changed executable never run;
- an agent-created or agent-modified command never runs without both feedback
  permission flags, exits `3`, and leaves its diff reviewable;
- a child left in the Codex process group is terminated before feedback, and a
  group that cannot be quiesced suppresses feedback;
- replacing the adapter source after its bytes are copied cannot change the
  executable bytes used for measurement;
- a pre-existing copy pathname or symlink is never followed or overwritten,
  the copy is created exclusively with mode `0700`, and no failure falls back
  to executing the target or external source path;
- feedback sees the immutable copy path as `argv[0]`, receives caller feedback
  arguments unchanged from `argv[1]`, runs with the target as cwd, and the
  private copy directory is removed on every handled result;
- stale-copy cleanup removes only an expired, same-owner, marked
  `.forma-exec-v1-` directory and leaves symlinks, unknown entries, and a
  concurrent run untouched;
- stale-home cleanup applies the owner, marker, symlink, and non-blocking
  run-lock checks to `.forma-codex-home-v1-` without an age delay and never
  treats an arbitrary `CODEX_HOME` as disposable; a top-level symlink and a
  lock held by a concurrent run are left untouched, while a nested symlink is
  unlinked without traversing its target;
- an interrupted ephemeral home containing a decoy credential-state file is
  removed at the next startup regardless of age when its lock is free, is not
  copied into evidence, and remains untouched while a concurrent owner holds
  the lock;
- an accepted agent-authored adapter is labeled separately from verification
  in terminal output and run evidence;
- a later run cannot change or delete an earlier run's external evidence through
  the agent sandbox;
- a later run does not automatically prune an earlier persistent run directory;
- a Codex zero exit plus invalid/missing feedback does not pass;
- a fabricated, incomplete, or non-canonical Fact set does not pass;
- feedback command non-zero, oversized output, timeout, and verifier rejection
  all return non-zero;
- agent prose saying success cannot affect the result;
- failure preserves the target diff and does not commit or push;
- compiler artifacts remain byte-identical with and without runner support.

## Not in this slice

- provider selection or fallback;
- direct Responses API orchestration;
- model selection guarantees;
- project-scoped `.codex` configuration, including an acknowledgement flag;
- remote/cloud repository execution;
- automatic commits, branches, pull requests, or deployment;
- automatic unbounded retry;
- incremental `forma generate`; incremental `request` and `verify` remain
  supported;
- a universal framework-independent feedback generator;
- elevating human Review Requirements into fabricated machine Facts; and
- changing any language or interchange schema version.

## Review decisions

- `CODEX_API_KEY` is supported for an isolated or explicitly host-authorized
  local invocation after the feedback-execution boundary above is enforced.
  Saved login and API-key auth do not change the host-code execution risk.
- Fresh-repository generation may use an agent-created feedback adapter only
  with explicit execution permission, origin evidence, and human-review
  labeling. A pre-reviewed template is not mandatory.
- `alpha.1` implements full-request generation only. Incremental `generate` is
  deferred while incremental `request` and `verify` remain supported.
- Alpha rejects project `.codex` configuration before Codex starts. It also
  sends a path-keyed untrusted override as defense in depth, without claiming
  Codex confirmed that root, ignores user configuration and exec-policy rules,
  disables ordinary hooks without bypassing hook trust, and disables outbound
  network for sandboxed repository commands. Run evidence records the actual
  argv and version qualification. A different parseable Codex version requires
  `--allow-unqualified-codex`, remains visibly unqualified, and cannot satisfy
  the release gate.
- Alpha resolves and records `CODEX_HOME` and authentication mode, passes a
  versioned allowlist to Codex, and strips state, credential, proxy, and
  credential-agent variables from repository shell commands. The exact user
  `CODEX_HOME` entry outside the target is not mistaken for project config.
  Saved-login mode requires an existing user home. API-key mode with no
  explicit home instead uses and cleans a private home under the trusted run
  root, so fresh automation neither runs `codex login` nor creates
  `$HOME/.codex`. That private home is treated as credential-bearing because
  Codex owns its contents; interrupted-run cleanup uses a per-run lock and no
  age delay. The selected authentication mode and state-root origin appear in
  terminal output as well as evidence.
- Alpha persistent run evidence has explicit indefinite/manual retention. Forma
  does not silently delete audit evidence; users remove it after review, and
  uninstall documentation names the default and custom locations.
