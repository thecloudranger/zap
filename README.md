<div align="center">
  <h1>⚡ zap</h1>
  <p><b>Find stale git worktrees and remove them safely.</b></p>
  <a href="https://github.com/thecloudranger/zap/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/thecloudranger/zap/ci.yml?branch=main&style=flat-square&label=ci" alt="CI"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/thecloudranger/zap?style=flat-square" alt="Go"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" alt="License"></a>
</div>

> zap is in early development. It only deletes in repositories that opted in with a `.zap` file.

## Quick Start

zap is a single Go binary for macOS and Linux.

```bash
go install github.com/thecloudranger/zap/cmd/zap@latest
```

**Run**

```bash
zap scan                 # List worktrees of the repo you're in
zap scan ~/dev/my-app    # …or of any repo by path
zap scan --all           # …or of every repo under your scan roots
zap scan --json          # JSON output for scripts and agents
zap explain <path>       # Why a worktree got its status: every check, pass/fail

zap init                 # Opt this repo in to cleaning (writes .zap)
zap clean --dry-run      # Show what would be removed, and what is kept and why
zap clean                # Remove them, after a confirmation
zap clean --all          # …in every opted-in repo at once
zap history              # What zap removed, with backup refs
zap undo                 # Bring back what the last clean removed
zap roots                # Show or change where --all looks (zap roots add|remove|reset)
zap version              # Show version
```

```text
$ zap scan --all
~/dev/api  ·  1 linked worktree  ·  target main

  WORKTREE                     BRANCH   AGENT  STATUS  AGE  ACTIVE  NOTES
  ~/.codex/worktrees/a1f3/api  a85e7da  codex  empty   6h   6h      detached

1 empty

~/dev/my-app  ·  2 linked worktrees  ·  target main

  WORKTREE                BRANCH         AGENT   STATUS  AGE  ACTIVE  NOTES
  .claude/worktrees/auth  worktree-auth  claude  merged  3d   2d      -
  .worktrees/login        fix/login      -       active  5d   2h      dirty(1)

1 merged · 1 active

Scanned 2 repositories in 300ms; 2 with linked worktrees. Roots: zap roots
```

| Status | Meaning |
|---|---|
| `merged` | Its work is already in the target branch: fast-forward, merge commit, squash or rebase merge |
| `empty` | It never got a commit of its own |
| `gone` | Its remote branch was deleted (often after a pull request is merged) |
| `missing` | Git still lists it, but its folder is gone |
| `active` | Anything else |

The target branch is `origin/HEAD`, then `main`, then `master`; override it with `--target`.
AGENT is guessed from the folder a worktree lives in (for example `.claude/worktrees` or
`~/.codex/worktrees`). `zap scan` outside a repository, or with `--all`, searches `~/dev`, `~/code`,
`~/src`, `~/projects` and known agent worktree folders; `zap roots` changes that list.
Add `--fetch` to `scan`, `explain` or `clean` to fetch remotes first, so pull requests merged on
GitHub are seen.

## Cleaning safely

`zap clean` only acts in repositories that contain a `.zap` file at their root. Everywhere
else it shows what it would do and changes nothing. Commit `.zap` so everyone on the repo
shares the same rules:

```toml
# .zap (every key is optional; these are the defaults)
# target = "main"                       # default: origin/HEAD, then main, then master
rules         = ["merged", "empty", "missing"]   # "gone" can be added
min_age       = "1d"                    # skip anything active more recently (m, h, d, w)
delete_branch = true
protect       = ["main", "master"]      # branch globs that are never removed
```

A worktree is removed only if it passes every check: not the main worktree, not a protected
or target branch, not locked with `git worktree lock`, no uncommitted or untracked changes, no activity within `min_age`, and not the directory you run zap from.
zap re-checks each worktree right before removing it and skips anything that changed.

Branches are deleted only when their work is provably in the target (or they have none);
a `gone` or `missing` worktree with unmerged commits loses its folder or registration but keeps
its branch. Before deleting a branch, zap saves its tip under `refs/zap/backup/`.

`zap undo` brings back the last run: it recreates the branches from those backups and re-adds
the worktrees at their old paths (`zap history` lists runs; `zap undo <RUN>` picks one). Files
git ignores inside a removed worktree (`node_modules`, build output, `.env`) are deleted for
good and cannot be restored.

The first time you clean, zap asks you to read a short disclaimer and type `zap`. In scripts,
`zap clean --yes` skips the confirmation (the disclaimer must have been accepted once), and
`--json` gives machine-readable output.

## Development

```bash
make build      # ./zap
make test       # unit + end-to-end tests
make check      # everything CI runs
```

## License

[MIT](LICENSE)
