# AI Integration and Credentials

Status: alpha.1 thin-runner contract. `forma authoring-context` and the first
full-request-only `forma generate` path are implemented. The first
fresh-repository dogfood created and ran an application. Its page-access
finding is fixed in the compiler and the repeated generation now enforces it at
the HTTP and browser boundaries. The clean-environment gate is implemented and
locally qualified; GitHub's macOS/Linux tag workflow remains before release.

Development builds extend this historical alpha.1 contract with automatic
full/incremental/no-op history and an AI-neutral execution/progress boundary.
`internal/agentrunner` owns execution evidence; `internal/agentbackend/codex` is
the first adapter, selected only in CLI assembly. Provider selection and other
real AI integrations remain future work, not assumptions in Forma semantics.
See [generation history](generation-history.md) and
[generation progress and adapters](generation-progress.md) for the current
source-build contract. The progress flags and backend identity never enter a
Generation Request or its semantic/baseline comparison.

Forma uses AI at two separate boundaries:

```text
person's application request
  -> authoring AI + forma authoring-context
  -> checked .forma source
  -> deterministic Generation Request
  -> Codex + target repository
  -> ordinary application code
  -> human diff review and explicit repository commands
```

The compiler decides language semantics. The authoring AI translates a
person's request into checked Forma source. Codex decides repository-specific
implementation details from the resulting Generation Request.

## Give an AI the installed Forma language

The Forma binary embeds the public alpha guide and two complete examples. An
application can obtain the exact context shipped with its installed binary:

```sh
forma authoring-context > /tmp/forma-authoring-context.md
```

Pass that output together with the person's application request to the AI that
writes `app.forma`. The output identifies its context schema, Forma binary
version, and language profile. It requires no API key, network access, or
target repository.

Do not replace this with a copied guide of unknown version or ask the model to
invent unsupported syntax. Write the result to a `.forma` file and validate it:

```sh
forma check app.forma
forma project flow app.forma
```

`docs/language-guide.md` is the public source embedded by the command;
`docs/alpha-language-profile.md` remains the normative accepted-language
boundary.

## Generate application code

The first alpha supports one implementation agent: Codex CLI in
non-interactive mode:

```sh
forma generate --repository ./target app.forma
```

Forma compiles a full canonical Generation Request and passes it through stdin
to a 30-minute-bounded `codex exec --ephemeral` process with the target as its
working repository and a `workspace-write` sandbox. The request is not written
into the target. The alpha invocation also ignores user config and user/project
execution-policy rules; it does not add another writable directory. It does
not implement an incremental generation command, provider abstraction, or raw
Responses API loop. OpenAI documents
[`codex exec` as the non-interactive interface](https://developers.openai.com/codex/non-interactive-mode).

The target must be inside a Git worktree with an existing commit. Generation
locks that worktree for the run and rejects Git-visible uncommitted changes by
default. Commit or stash the Forma source and other work first. For an
intentional dirty-tree experiment, `--allow-dirty` is explicit and the output
warns that the final status cannot be attributed solely to Codex.

The command stops after Codex returns. It prints Codex's final summary, the
SHA-256 of the exact implementation prompt (instructions plus canonical
request), the target's current porcelain status, and the next review steps.
The digest identifies the exact combined agent input in a saved run log; when
paired with the canonical request digest it distinguishes a template change
without writing the prompt into the target. Forma does not
automatically execute application code, tests, or an agent-authored feedback
adapter on the host. The user reviews the Git diff and explicitly runs the
repository's normal commands. A green summary is not sufficient by itself: the
reviewer confirms that boundary tests really ran and did not skip their
assertions because Codex's sandbox lacked sockets or another runtime
capability. Codex may use commands inside its own workspace sandbox while
implementing the request.

The implementation instructions preserve an important semantic distinction in
the request: an access Fact whose expected enforcement is `authoritative` must
be implemented and tested at the public boundary that presents or invokes its
subject. A UI visibility check or direct pure role-helper test is not enough.
An anonymous principal has no authenticated identity and no roles; missing
identity, session, or role state must not be converted into an allowed default
role. These are general request-translation rules, not application-specific
Welcome-page behavior.

Alpha.1 is for a fresh repository, or another repository the invoking user
owns and trusts. Running untrusted repository code automatically, publishing a
tamper-resistant external evidence store, and automatic repair are post-alpha
work preserved in the hardened-runner research document.

## Authentication

Creating the authoring context and compiling Forma do not need an API key.
Only the Codex generation step needs Codex authentication.

For local use, install Codex CLI, run its login flow, and confirm it before
generation:

```sh
codex login
codex login status
```

`forma generate` runs `codex login status` and then reuses that saved Codex
authentication. Forma does not read the saved credential contents.

For API-key authentication, the official Codex flow reads
`OPENAI_API_KEY` through stdin and stores the resulting login in the selected
Codex credential store:

```sh
printenv OPENAI_API_KEY | codex login --with-api-key
codex login status
forma generate --repository ./target app.forma
```

The thin runner does not forward `OPENAI_API_KEY`, `CODEX_ACCESS_TOKEN`, or
unrelated application secrets to its Codex child. It forwards only a named
runtime/network allowlist needed to reuse the already selected login. Forma
does not copy a key into the Generation Request, prompt text, logs, or generated
files. See the official
[Codex authentication documentation](https://developers.openai.com/codex/auth)
for supported login methods.

## Cost, network, and review responsibility

`forma authoring-context`, `check`, `resolve`, `project`, `request`, and
`verify` are deterministic local commands and do not contact an AI provider.
`forma generate` is network-capable because Codex connects to its provider.
Provider access, account limits, model availability, token usage, and charges
belong to the selected Codex authentication and account.

Generated code is not trusted merely because it was produced from a valid
Generation Request. Forma can determine and preserve application intent; a
person still reviews the generated diff, security-sensitive implementation
choices, migrations, dependencies, and any human Review Requirements before
running or shipping the application.

The first recorded run and its two page-access reruns are in the
[alpha quickstart dogfood](evaluations/alpha-dogfood-2026-08-23.md). It is
evidence that the handoff can produce a runnable application. It also records
why the Acceptance Fact and its general public-boundary translation rule were
both needed, plus the generated-code defects that kept human review necessary.

## Post-alpha automation

The earlier fully automated runner design included host execution of feedback
adapters, isolated persistent evidence, credential-bearing temporary state,
automatic cleanup, and hostile-repository containment. Those controls address
a different trust model and are intentionally not required before people have
used the thin alpha workflow. The design remains available in
[`reference-agent-runner-proposal.md`](reference-agent-runner-proposal.md).
