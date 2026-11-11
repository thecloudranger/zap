---
name: zap
description: Find stale git worktrees (merged, empty, gone, missing) and remove them safely with the zap CLI. Use when the user asks to clean up, list or free disk space from git worktrees, including ones left behind by coding agents in .claude/worktrees, ~/.codex/worktrees and similar folders.
---

# zap: stale git worktrees

`zap` finds git worktrees whose work already landed and removes them, with backups and undo.
Read before you act, and never delete without the user's explicit go-ahead.

## Look (always safe)

```bash
zap scan --json                 # worktrees of the current repository
zap scan --all --json           # every repository under the user's scan roots
zap explain --json <path>       # every check and safety gate behind one worktree's status
zap clean --dry-run --json      # what clean would remove, and what it keeps and why
zap clean --all --dry-run --json
```

Add `--fetch` to see pull requests merged on the remote; add `--gh` to ask GitHub too.

Every `--json` command prints one envelope on stdout:
`{schemaVersion, command, status: "ok"|"error", data, warnings, error: {code, message}}`.

Statuses: `merged` (work is in the target branch), `empty` (no commits of its own), `gone`
(remote branch deleted), `missing` (folder gone), `idle` (unmerged and untouched), `active`.

## Remove (only after the user says so)

1. Show the user the dry run: which worktrees, their size, and anything kept with its reason.
2. Only when they agree: `zap clean --yes --json` (add `--all` if that is what they approved).
3. Report `data.results`: each `outcome` is `zapped`, `skipped` or `failed`.

Rules:

- zap only removes in repositories with a `.zap` file. Do not run `zap init` or edit `.zap`
  unless the user asks: that is the user's decision to opt a repository in.
- Exit code 4 with error code `refused` and a message about the disclaimer means the user has
  not accepted zap's one-time disclaimer. Do not work around it; ask the user to run
  `zap clean` once in their own terminal.
- Never pass `--unlock-stale` unless the user asks: it releases locks other tools set.
- Never delete worktrees with `rm -rf` or `git worktree remove --force` instead of zap.
- Idle worktrees are never removed by `zap clean`; only the user can pick them in `zap`.

## Undo

```bash
zap history --json    # runs, with the backup ref of every deleted branch
zap undo --json       # bring back the latest run: branches and worktrees
zap undo <RUN> --json
```

Files git ignores inside a removed worktree (node_modules, build output, caches) are not
restored.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | ok |
| 1 | error (see `error.code`) |
| 2 | bad usage |
| 4 | refused: confirmation or disclaimer missing; nothing was changed |
| 130 | interrupted |
