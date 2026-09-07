# Forma Alpha Security and Trust Boundary

Status: security contract for the `v0.1.0-alpha.2` thin runner.

[Generation progress](generation-progress.md) accepts only
fixed provider-independent activity categories and diagnostics, never raw AI
payloads or stderr. Numeric CLI exit status and fixed failure observations remain
available in human diagnostics, without exposing raw provider errors. The final
AI summary remains explicitly unverified free text
and is not covered by that non-disclosure boundary. It is obtained from a bounded
private file outside the target, without granting the agent another sandbox
writable directory. Cancellation uses process-group teardown, bounded Git
cleanup, and pending history; it is not rollback. Progress sink failure does not
prevent execution cleanup or cause an automatic retry.

## Trust model

Compiler-only commands operate on user-selected Forma source and artifacts and
do not invoke an AI, repository command, or network service. `forma generate`
is different: it invokes the installed Git executable for repository preflight
and the installed Codex CLI to edit one target worktree.

Alpha.2 is for a fresh repository or another repository the invoking user owns
and trusts. It is not containment for hostile repositories, plugins, build
scripts, dependencies, or generated code.

## Credential boundary

Forma does not accept an API key option. It asks `codex login status` to confirm
an existing Codex login and forwards a small named runtime/network environment
allowlist. It does not put `OPENAI_API_KEY`, `CODEX_ACCESS_TOKEN`, application
secrets, credential values, or evidence values into Forma source, a Generation
Request, the implementation prompt, or generated files.

Codex authentication and provider network traffic remain Codex behavior. See
[AI integration](ai-integration.md) for supported login flows, account costs,
and the complete environment boundary.

## Repository preflight

Before Codex starts, Forma:

- canonicalizes the target and containing Git worktree;
- takes a non-blocking exclusive lock on the worktree directory;
- requires an existing commit;
- rejects ordinary Git-visible changes unless `--allow-dirty` is explicit;
- rejects hidden `assume-unchanged` / `skip-worktree` index state;
- fails with a dedicated diagnostic for unsafe Git ownership; and
- keeps the worktree lock through final status capture.

The dirty check protects a person's visible uncommitted work. It is not a
tamper-proof measurement of repository contents against a previous agent run.

Forma also retains the lock through local generation history
selection and durable result storage. History lives in the per-worktree Git
metadata directory; it is comparison state, not signed evidence or proof of
repository correctness. No-op does not contact Codex or run tests. Existing
targets without history and interrupted/invalid history stop for explicit
adoption or recovery. See [generation history](generation-history.md) for
identity, retention, and failure handling.

## Generated code and tests

Forma passes a canonical request to Codex in a `workspace-write` sandbox and
does not add another writable directory. Codex can inspect and run commands in
that sandbox while implementing. After Codex exits, Forma does not run target
code, tests, servers, migration tools, package installers, or an agent-authored
feedback adapter with host authority.

A person reviews the diff first. Generated tests can be incomplete or can skip
assertions when the sandbox lacks a capability such as socket binding. Run
reviewed repository commands explicitly in the intended environment and verify
that authoritative/public boundaries were actually reached.

## Semantic limits with security impact

- A valid request does not prove secure framework configuration, secret
  storage, cryptography, dependency selection, or deployment settings.
- Human Review Requirements are displayed and never silently marked complete.
- The alpha language does not define cascade, detach, or restrict behavior for
  deletion through required relations. Treat generated deletion behavior as a
  product decision requiring review.
- `--allow-dirty` weakens attribution of the final diff and does not relax
  locking, ownership, or hidden-index checks.
- The prompt SHA-256 identifies the exact combined agent input but does not make
  the generated output trustworthy.

Hostile-repository execution, an external tamper-resistant evidence store,
automatic feedback execution, and automatic repair remain post-alpha work in
the [hardened runner research](reference-agent-runner-proposal.md).
