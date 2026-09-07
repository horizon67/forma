# Local generation history

Status: CLI contract introduced in `v0.1.0-alpha.2`; not part of `v0.1.0-alpha.1`.
Generation Requests remain `v0alpha5`, with published `v0alpha4` baselines readable.
History has its own schema, `forma/generation-history/v0alpha1`.

Progress and cancellation are described in
[generation progress](generation-progress.md). Preparation/authentication is an
adapter responsibility; history selection, candidate persistence, completion,
and lock release remain Forma responsibilities. No history schema or comparison
identity changes are introduced by progress or adapter selection.

## Normal operation

Repeat the same command; there is no full/update/no-op mode flag:

```sh
forma generate --repository ./target --manifest policy.yaml ./app.forma
```

The compiler still uses the explicitly supplied source selection and Manifest.
No watcher, source discovery change, or Manifest auto-discovery is introduced.
An omitted Manifest inherits the chosen baseline's policy. Comment/formatting
changes and canonical ordering do not select an update. The comparison reuses
the same semantic/policy planner as `request --previous`.

| Available evidence | Action |
| --- | --- |
| No history, target contains only inputs/scaffolding | Full generation |
| Completed comparison baseline, semantic/policy delta | Incremental update |
| Completed comparison baseline, no delta | No-op, no Codex lookup or authentication |
| Existing implementation without history | Stop; deliberate adoption required |
| Failed/interrupted attempt or incomplete history save | Stop; deliberate recovery required |
| Corrupt/mismatched history or incompatible checkout | Stop; never assume full/no-op |

Normal Git preflight is unchanged, including the existing commit requirement
and dirty-worktree guard. Review and commit your generated files before the
next normal run. `--allow-dirty` only opts into retaining existing uncommitted
work; it does not override history failure, corruption, or the worktree lock.

The fresh-target check walks all files, including Git-ignored files. It allows
only the selected source files, the explicit Manifest, root `.git` metadata,
and these regular scaffolding files:

- root `README.md`, `.gitignore`, `.gitattributes`, `.editorconfig`, `LICENSE`,
  `LICENSE.md`, `LICENSE.txt`, `CODEOWNERS`, and `Makefile`;
- `.github/CODEOWNERS` and `.yml`/`.yaml` files directly in `.github/workflows`.

This is a narrow scaffolding allowance, not a general exemption for `.github`,
scripts, nested build files, or all clean/tracked files. Other files and symlinks
stop automatic initial generation. For a new application, select an empty
target directory in a Git worktree with a commit instead of manufacturing a
`--previous` Request from today's specification. `--previous` is only for a
baseline you have confirmed matches an existing implementation; an unchanged
Request imports a no-op assertion and **does not generate missing code**.
Having no history is not evidence that arbitrary existing application code is
safe to regenerate. If history for the same target exists under another branch
or source selection, explicit re-binding is required even for an empty target.

## Storage and application identity

History is stored at `<per-worktree-git-dir>/forma/generation-history.json`.
Forma asks trusted Git for `rev-parse --absolute-git-dir`; it does not assume
that `.git` is a directory or use ambient `GIT_DIR`. Linked worktrees therefore
get different metadata locations. No application file, ignore rule, or tracked
file is added by history management. Installation and compiler-only commands
still create no history.

After locked Git preflight, the CLI prints both the history path and current
application key, including when history loading or selection fails. If a
different branch/source selection already has records for this target, it also
prints those existing keys, branches, and selectors for inspection; it does
not silently select one of them. Each key hashes:

- canonical absolute worktree and target paths;
- current branch reference, or detached HEAD identity;
- sorted, deduplicated, canonical absolute **source selectors**.

A directory selector stays stable as `.forma` files are added beneath it.
Switching between a file selector and a directory selector is a new identity,
even if they currently discover the same files. Manifest paths are not part of
the key: the Manifest is policy for the same application, not a second app.
Overlapping application selections are not inferred to be equivalent.

Separate targets/source selections have separate records. Branch switches do
not borrow another branch's baseline. For the same key, the saved execution
HEAD must remain an ancestor of the current HEAD; resets or unrelated history
stop automatic selection. Ordinary descendant commits do not select an update
by themselves. Hand edits, discarded changes, and test failures are not proof
of semantic change and do not trigger repair.

History is local and is not transferred by clone, push, or fetch. A clone with
existing application code requires adoption. Moving a repository/source tree
or copying metadata to another worktree produces missing/mismatched identity,
not an automatic regeneration. A new branch can be explicitly adopted after
review. Detached checkouts are scoped to their exact HEAD.

## Execution completion is not verification

1. Acquire the containing worktree lock, then select/validate history and plan.
2. For a real update, authenticate Codex. Setup/authentication failure does not
   create an editing attempt and can be retried normally.
3. Immediately before Codex exec, save a durable pending marker and candidate:
   the exact canonical Request bytes/digest, chosen baseline, prior valid
   baseline, execution HEAD, and start time. A save failure prevents dispatch.
4. On successful execution, require complete final Git evidence and unchanged
   branch/HEAD/Git-directory identity. Save the completed result and comparison
   baseline atomically, then clear the pending marker.
5. On failure/interruption, retain the previous valid comparison baseline and
   leave the attempt pending. Unknown/truncated final evidence is not success.

The lock spans selection, authentication, candidate save, agent execution, and
completion save. Other Forma runs in the same worktree fail with the existing
lock error. One unfinished transaction conservatively blocks that worktree
until its original application/branch is recovered.

Catalog writes use a private temporary file, file sync, atomic rename, and
directory sync. A separate `generation-pending` marker remains on any uncertain
save, including errors after rename. It prevents an apparently completed
catalog from being used for automatic no-op. `lastAttempt.priorBaseline` retains
the preceding valid baseline for inspection in that case. A stale pending
marker after a crash causes a recovery stop, not a guessed completion.

`verification: unverified` and `humanReview: pending` are separate from execution
status and never become passed merely because Codex exits zero or reports a
green summary. The comparison baseline is **not** a verified implementation.
Separate `forma verify` commands and external build/test results remain separate
evidence; this slice does not import them into history or automatically approve
human review. No-op runs no checks, and does not repair existing defects.

Automatic no-op makes no history writes. An explicit no-op import is different:
it stores a user's baseline assertion, marked `origin: explicit`, without
inventing an agent run. Completed runs use `origin: completed`. Only the latest
attempt and the baselines needed for that attempt are retained per application;
this is comparison state, not a complete audit archive.
An automatically selected baseline retains its original origin and HEAD;
selection is not a new explicit assertion. Supplying `--previous` deliberately
creates an `explicit` assertion bound to the current checkout HEAD.

## Explicit adoption and recovery

`--previous <request.json>` takes precedence over automatic selection. The
Request still passes schema/canonical validation and the incremental planner.
It does not prove that the target implements that Request; the caller must
review that correspondence. An unchanged explicit Request imports a baseline
without running Codex. A changed one drives an incremental update; only a
successful completed update advances the comparison baseline.

After a failed/interrupted attempt, first review the partial repository edits.
Recover with a known, reviewed baseline using the same target/source selectors
and branch. If the supplied baseline and current meaning match, this is an
explicit assertion that updating is unnecessary, **not** an automatic retry.
If they differ, the new bounded update is allowed. Import clears the unresolved
marker but retains the last failed/interrupted attempt as such. Failure of a
new update never overwrites the preceding valid baseline with its candidate.

An existing application key selects a record in `records`. The printed **current**
key has no baseline on a new branch/source selection; inspect the separately
printed existing keys and confirm which, if any, matches the current checkout.
Each `baseline` contains a `request` string with the exact canonical JSON bytes.
You may export that string to a separate new file for review and explicit
adoption. For example, with `jq`, substitute the printed path and a reviewed
existing key (not an absent current key):

```sh
jq -e -j --arg key '<existing-key>' \
  '.records[$key].baseline.request // error("no baseline for this key")' \
  '<printed-history-path>' > reviewed-request.json
```

`-j` preserves the Request string without adding a newline. Use a new output
path, check the command succeeds, and review the exported Request against the
repository before passing it to `--previous`. Do not copy a candidate or modify
its embedded digest to make validation pass. A failed first attempt may have
no baseline at all; it must not be adopted just because its candidate is present.
When an old application has no saved Request, reconstruct one only from specifications you
have confirmed match that implementation, then deliberately adopt it.

Malformed/unsupported history is rejected even with `--previous`; it is never
silently overwritten. Back up the specific printed history file and its sibling
`generation-pending` marker, restore known-good metadata, or move those two
files aside after review and explicitly import a known baseline. Moving the
catalog aside removes automatic selection for **all applications in that
worktree**, so preserve the backup and re-adopt other applications deliberately.
Never delete the containing `.git` directory as a history recovery step.
After moving a repository, `--previous` alone cannot bypass the catalog's old
worktree identity. Follow the same backup/move-aside procedure; do not edit only
the catalog's `worktree` field, because record identities also bind the original
target and source-selector paths and their keys.

History files use mode `0600`, and the history directory is created with `0700`.
Symlink/non-regular history files are rejected; changes to the loaded catalog
or pending marker during execution are not silently overwritten. This is
local crash/corruption protection, not an authenticity guarantee against a
hostile same-user process or target-controlled Git metadata. Preserve the thin
runner's trusted, user-owned repository boundary. Uninstalling the executable
does not delete history or application files; retain or remove these specific
local metadata files deliberately after review.
