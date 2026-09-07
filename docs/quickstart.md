# Forma Alpha End-to-End Quickstart

Status: reviewed workflow for `v0.1.0-alpha.2`.

This path goes from a person's application request to Forma source, a
deterministic Generation Request, and ordinary application code written by
Codex. Forma stops before running generated code so a person can review it.

## 1. Install and identify Forma

After the alpha tag is published:

```sh
go install github.com/horizon67/forma/cmd/forma@v0.1.0-alpha.2
forma version
```

The expected output is:

```text
forma v0.1.0-alpha.2
```

See [installation](install.md) for prebuilt binaries, PATH, source builds, and
uninstall instructions.

## 2. Give the installed language to an authoring AI

```sh
forma authoring-context > /tmp/forma-authoring-context.md
```

Give that file and the person's application request to the AI that writes
`app.forma`. The context already tells the AI to stop rather than invent syntax
when Forma is unavailable or rejects the result. Do not paste an unrelated or
copied language guide of unknown version.

To exercise the implementation path before authoring a new application,
download the source pinned to the same tag:

```sh
curl -fsSLo app.forma \
  https://raw.githubusercontent.com/horizon67/forma/v0.1.0-alpha.2/docs/examples/alpha-quickstart.forma
```

From a tagged repository checkout, the equivalent command is
`cp docs/examples/alpha-quickstart.forma app.forma`. The example is embedded in
`authoring-context` for the AI to read, but installing one executable does not
silently copy source files into the current directory.

## 3. Compile and review application meaning

```sh
forma check app.forma
forma project flow app.forma > /tmp/forma-flow.md
forma request app.forma > /tmp/forma-request.json
```

`check` proves that the installed front-end accepts the source. `project flow`
is a human review view. `request` is the canonical coding-agent handoff. None of
these commands contacts an AI provider or changes a target repository.

## 4. Prepare a fresh target

```sh
mkdir my-forma-app
cd my-forma-app
git init
git -c user.name=Forma -c user.email=forma@example.invalid \
  commit --allow-empty -m "Initial target"
cd ..
```

Alpha.2 is intended for a fresh repository, or another repository the invoking
user owns and trusts. Commit or stash existing work before generation.

## 5. Authenticate Codex and generate

Alpha.2 displays progress automatically on stderr. Optional
`--progress=json` reserves stderr for progress JSONL, while the final result
stays on stdout. `--verbose` adds safe activity categories, and
`--heartbeat-interval 10s` changes the default 30-second heartbeat. These options
do not change what is generated. See [generation progress](generation-progress.md).

```sh
codex login
codex login status
forma generate --repository ./my-forma-app app.forma
```

Forma prints the exact implementation-prompt SHA-256, Codex's summary, and the
final Git status. It saves the canonical request in local Git metadata, not in
application files, and does not run the generated application on the host.

## 6. Review before executing

```sh
git -C my-forma-app status --short
git -C my-forma-app diff --stat
git -C my-forma-app diff
```

Inspect authentication and authorization boundaries, persistence, deletion
behavior, dependencies, migrations, and every displayed Human Review
Requirement. Read the generated tests rather than trusting a green summary:
confirm that public-boundary assertions ran and were not skipped because the
Codex sandbox lacked sockets or another runtime capability.

Only after review, run the build and test commands belonging to the generated
repository. Their names depend on the framework Codex selected; Forma does not
invent a universal command.

## 7. Incremental updates and no-op

This section requires alpha.2 or newer; the older `v0.1.0-alpha.1` executable
does not support automatic history or `generate --previous`. Alpha.2 can read
the `v0alpha4` Request saved by the original alpha.

When step 5 used alpha.2, Forma already saved its exact agent
input in local Git metadata. After reviewing, testing, and committing the target
changes, use the same command again, whether or not `app.forma` changed:

```sh
forma generate --repository ./my-forma-app app.forma
```

To change implementation technology, pass the updated Manifest as well:

```sh
forma generate --repository ./my-forma-app \
  --manifest forma.implementation.yaml app.forma
```

The Manifest must exist and use the [Implementation Policy schema](implementation-policy-manifest-proposal.md).
Omitting it inherits the baseline's policy. A policy-only change starts a
bounded update; if hand-written changes already satisfy it, Codex can return
without editing any files. Commit or stash target changes before generation,
as for a full run.

Identical application meaning and policy produce `no application or policy
changes`, exit `0`, and no Codex invocation. This works without Codex installed
or logged in, but the target must still pass Git preflight. No-op does not
verify the existing code or repair a failed implementation.

Forma saves a candidate just before editing starts and adopts it as the next
comparison baseline only after the run and history save complete. This does
not mark tests or human review as passed. A failed/interrupted attempt blocks
automatic generation until deliberate recovery; rerunning unchanged source
does not quietly repair or approve its output.

If step 5 used the older alpha without history, keep its original Request from
step 3. After reviewing the implementation and confirming it matches that
Request, explicitly adopt it once, before changing your specification:

```sh
forma generate --repository ./my-forma-app \
  --previous /tmp/forma-request.json app.forma
```

With unchanged meaning this imports the supplied baseline without calling
Codex. If meaning already changed, it performs the corresponding incremental
update and saves its actual input on completion. Subsequent normal commands
need no `--previous`. Do not manufacture a baseline from today's specification
and assume an existing repository already implements it.

Use a directory source selector when an application will span multiple source
files: that selection stays stable when files are added within the directory.
Changing the selector itself requires explicit re-binding. See
[generation history and recovery](generation-history.md) for branch changes,
clones, failed runs, history backups, and manual imports.

## What each layer guarantees

| Layer | Guarantee |
| --- | --- |
| Forma source and compiler | Parsed, resolved, type-checked application intent within the alpha profile |
| Generation Request | Canonical intent, Acceptance Facts, Source Map, and explicit human-review boundary |
| Codex output | A proposed repository implementation, not trusted merely because Codex completed |
| Repository tests | Only the behavior their executed assertions actually observe |
| Human review | Product choices and implementation properties that Forma cannot machine-verify |

The recorded [fresh-repository dogfood](evaluations/alpha-dogfood-2026-08-23.md)
uses this workflow and preserves both its successful observations and the
generated-code defects found during review.
