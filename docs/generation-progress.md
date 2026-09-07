# Generation progress and agent adapters

Status: CLI contract introduced in `v0.1.0-alpha.2`; not shipped in `v0.1.0-alpha.1`.

## Normal use

No extra option is needed. Repeat the same generation command and Forma chooses
full generation, incremental update, or no-op from its local comparison history:

```sh
forma generate --repository ./target --manifest policy.yaml ./app.forma
```

Progress goes to stderr as newline-delimited text. Both terminals and redirected
output use the same format; no spinner, carriage return, or screen redraw is used.
The final human-readable output stays on stdout: repository, implementation input
(prompt) SHA-256, AI summary, Git status, Human Review Requirements, and the warning
that completion is not verification. AI summaries are explicitly **unverified free
text and may contain sensitive information**. The progress allowlist does not
sanitize or certify their contents; terminal controls, Unicode format controls
(including bidi overrides), and implicit Unicode line separators are removed.
Readable text and explicit newlines/tabs remain; this is not secret redaction.

Options affect observation only, never language semantics or baseline identity:

| Option | Contract |
| --- | --- |
| `--progress=text` | Default newline text progress |
| `--progress=json` | stderr exclusively contains progress JSONL |
| `--verbose` | Include safe activity categories and phase durations |
| `--heartbeat-interval <duration>` | Default `30s`; accepted range `1s` through `5m` |

Both `--progress=json` and `--progress json` forms are accepted, as are both
forms of the heartbeat option. Invalid or repeated options fail with exit `2`.
When JSON mode is recognized, even usage and compiler errors keep stderr JSONL;
the detailed human diagnostics (including compiler source excerpts) go to stdout.
Compiler-only commands keep their existing output and error behavior.
In text mode human diagnostics normally go to stderr after progress drains. If
that sink is stalled or broken, diagnostics fall back to stdout with an explicit
notice, rather than being discarded or written concurrently to the stalled sink.

## Observations, not verification

Phases distinguish `compile`, `repository_validation`, `history_selection`,
`agent_preparation`, `agent_starting`, `agent_running`, `cleanup`, `final_status`,
and `history_save`. A no-op does not prepare, authenticate, look up, or run an
agent. `agent_running` requires the adapter's actual-start notification; merely
calling the adapter does not demonstrate that execution began. A backend with no
notifications is usable, but its phase stays `agent_starting` until cleanup.
Text mode renders human labels such as "compiling Forma source", "validating
repository", "agent running", and "capturing final repository status"; the JSON
phase enums above do not change. Before an editing attempt, stdout includes a
Ctrl+C hint and warns that cancellation is not rollback.

Elapsed durations use Go's monotonic clock. Remaining time comes from the actual
execution context deadline. The existing 30-minute deadline still begins after
compilation and explicit input planning, before repository preflight; it is not
reset when authentication or execution starts. Final Git identity inspection and
history persistence each get an independent five-second cleanup budget after
cancellation. History checks its budget before starting a durable write;
filesystem syscalls are not themselves cancellable. Once the write and fsyncs
succeed, the store advances its in-memory state and finishes marker cleanup even
if that budget expires during the write. A real write/fsync/marker failure still
requires recovery; late context expiry alone does not invent an ambiguous save.

A heartbeat says Forma is still waiting, **not** that the AI made progress. The
last-activity age is present only after a recognized agent event. Heartbeats and
phase changes do not reset it. Without verbose, activity still updates this age
but is not printed individually. No percent-complete estimate is invented.

JSON uses `forma/generation-progress/v0alpha1` and only these event types:
`phase`, `heartbeat`, `activity`, `diagnostic`, `result`. Fields are:

- `schema`, `type`, `elapsedMs`;
- optional `remainingMs`, `activityAgeMs`;
- fixed `phase`, optional `durationMs` for a finished phase in verbose mode;
- fixed `activity` category or `code`;
- terminal `status`: `completed`, `no_op`, `failed`, `cancelled`, `timed_out`.

Activity categories are `turn`, `message`, `reasoning`, `command`, `file_change`,
`tool`, `search`, `plan`. These are names only, never their contents. Diagnostic
codes are `invalid_agent_event`, `oversized_agent_event`, `agent_reported_error`,
`generation_failed`, `compilation_failed`, `invalid_options`, and
`review_partial_changes`. There is no arbitrary AI message/payload field.
`generation_failed` means a recorded editing attempt failed, including failure
to finalize its evidence/history; usage, compilation, and preflight failures do
not emit it. Cancellation/timeout use their distinct terminal result and recovery
notice. These diagnostics do not imply application verification was performed.

```json
{"schema":"forma/generation-progress/v0alpha1","type":"heartbeat","elapsedMs":60000,"remainingMs":1740000,"activityAgeMs":15000,"phase":"agent_running"}
```

## Ownership and dependency direction

The CLI wires the current Codex adapter, but planning/history and
`internal/agentrunner` do not import it. The common runner accepts `Backend`, which
prepares a `Prepared` execution with an immutable `InputSHA256`, `Run`, and `Close`.
The interface requires no PID, command-line arguments, stdout, or OS process.
An API-backed implementation can implement exactly the same boundary. Tests use
an in-memory fake backend that imports no Codex package and executes no AI CLI.

Preparation authenticates and constructs immutable input without editing the
target. Only after successful preparation and input-digest validation does Forma
save its candidate/pending marker. It then executes the prepared input, closes
private resources, checks final Git evidence, and durably updates history while
holding the same worktree lock. Completion decisions belong to Forma, not the AI.
The terminal progress result is finalized after history handling and lock release.

The first real adapter is `internal/agentbackend/codex`: executable discovery,
saved authentication, environment allowlist, prompt, CLI arguments, and event
interpretation live there. Generic process/pipes/process-group handling stays in
`internal/agentrunner`. Other live providers, provider-selection settings, and
new credential/contract handling are deferred. Changing an adapter or display
option alone must not change a Request or cause regeneration.

## Codex stream and output safety

The adapter uses `codex exec --json` and `--output-last-message`, following the
[official non-interactive output contract](https://learn.chatgpt.com/docs/non-interactive-mode#make-output-machine-readable).
The final message is read only from the separate file; `agent_message` stream
contents are never treated as the final summary. A private directory outside the
target holds the summary, with mode `0700` and an initially `0600` file. Reading
rejects symlinks/non-regular files and more than 1 MiB; the directory is removed
after process completion. Missing/unsafe final output or failed cleanup prevents
normal completion.

Stdout is parsed incrementally, bounded to 256 KiB per event, with no total
stream-size ceiling. Unknown events are ignored. Invalid or oversized events
are discarded with a fixed, deduplicated diagnostic; draining continues through
the next newline and subsequent events. This observation degradation is not, by
itself, evidence that implementation failed. A reported failed turn is a failure.
Raw stderr is drained without being retained or displayed. Commands, tool output,
file contents, reasoning text, credentials, and unknown fields never enter the
progress protocol, including verbose mode and authentication errors.
Human failure diagnostics preserve numeric CLI exit status and fixed observations
such as a reported failed turn, incomplete process cleanup, or an observation
failure. They never echo raw runner errors or infer model, rate-limit, or sandbox
causes from private output. In JSON mode these human details go to stdout.

The display has a nonblocking queue of at most 64 events. Repeated activity,
heartbeat, and diagnostic observations coalesce; lifecycle events take priority.
The terminal event has a separate reserved slot. A stalled writer receives only
a bounded final flush opportunity (100 ms); output may be incomplete when the
destination is broken or stalled. It cannot block pipe draining, process cleanup,
or lock release, and display failure alone neither reruns the AI nor changes a
successfully completed editing attempt into a failed one.

## Interruption and recovery

On macOS/Linux, Ctrl+C (SIGINT) and SIGTERM cancel the shared context. Codex and
its process group are terminated using the existing generic group teardown.
Final Git checks use bounded cleanup contexts independent of the cancelled run.
Before candidate save, cancellation creates no editing attempt. After candidate
save, cancellation, timeout, or execution failure retains pending history and
the previous comparison baseline. Cancellation is **not rollback**: review the
partial diff before any explicit recovery described in
[generation history](generation-history.md).
After durable completion or a completed no-op, a late signal does not retroactively
change the terminal result to `cancelled` or change exit `0` to `1`.

Exit codes stay `0`/`1`/`2`; interruption and timeout both exit `1`, distinguished
by the progress result. History schema is unchanged: their durable attempt status
remains `failed` with pending recovery, not a new per-cause persistence schema.
Neither completion nor no-op marks tests or Human Review Requirements passed.

The trust boundary is unchanged: this is for a user-owned, trusted worktree, not
containment against a hostile same-user process. Forced termination (SIGKILL),
power loss, and processes deliberately escaping the managed group are not clean
cancellation; pending history is the conservative recovery guard.
