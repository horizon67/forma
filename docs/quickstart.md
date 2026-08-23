# Forma Alpha End-to-End Quickstart

Status: reviewed workflow for `v0.1.0-alpha.1`.

This path goes from a person's application request to Forma source, a
deterministic Generation Request, and ordinary application code written by
Codex. Forma stops before running generated code so a person can review it.

## 1. Install and identify Forma

After the alpha tag is published:

```sh
go install github.com/horizon67/forma/cmd/forma@v0.1.0-alpha.1
forma version
```

The expected output is:

```text
forma v0.1.0-alpha.1
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
  https://raw.githubusercontent.com/horizon67/forma/v0.1.0-alpha.1/docs/examples/alpha-quickstart.forma
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

Alpha.1 is intended for a fresh repository, or another repository the invoking
user owns and trusts. Commit or stash existing work before generation.

## 5. Authenticate Codex and generate

```sh
codex login
codex login status
forma generate --repository ./my-forma-app app.forma
```

Forma prints the exact implementation-prompt SHA-256, Codex's summary, and the
final Git status. It does not write the request into the target and does not run
the generated application on the host.

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
