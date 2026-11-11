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
zap scan --json          # JSON output for scripts and agents
zap explain <path>       # Why a worktree got its status: every check, pass/fail

zap init                 # Opt this repo in to cleaning (writes .zap)
zap clean --dry-run      # Show what would be removed, and what is kept and why
zap clean                # Remove them, after a confirmation
zap history              # What zap removed, with backup refs
zap version              # Show version
```

```text
~/dev/my-app  ·  4 linked worktrees  ·  target main

  WORKTREE                BRANCH         STATUS   AGE  ACTIVE  NOTES
  .claude/worktrees/auth  worktree-auth  merged   3d   2d      -
  .claude/worktrees/new   worktree-new   empty    1h   1h      -
  .worktrees/login        fix/login      active   5d   2h      dirty(1)
  .worktrees/old          old            missing  2w   -       -

1 merged · 1 empty · 1 missing · 1 active   (zap explain <path> for details)
```

| Status | Meaning |
|---|---|
| `merged` | Its commits are already in the target branch |
| `empty` | It never got a commit of its own |
| `missing` | Git still lists it, but its folder is gone |
| `active` | Anything else |

The target branch is `origin/HEAD`, then `main`, then `master`; override it with `--target`.

## Cleaning safely

`zap clean` only acts in repositories that contain a `.zap` file at their root. Everywhere
else it shows what it would do and changes nothing. Commit `.zap` so everyone on the repo
shares the same rules:

```toml
# .zap (every key is optional; these are the defaults)
# target = "main"                       # default: origin/HEAD, then main, then master
rules         = ["merged", "empty", "missing"]
min_age       = "1d"                    # skip anything active more recently (m, h, d, w)
delete_branch = true
protect       = ["main", "master"]      # branch globs that are never removed
```

A worktree is removed only if it passes every check: not the main worktree, not a protected
or target branch, not locked with `git worktree lock`, no uncommitted or untracked changes, no activity within `min_age`, and not the directory you run zap from.
zap re-checks each worktree right before removing it and skips anything that changed.

Before deleting a branch, zap saves its tip under `refs/zap/backup/`; `zap history` lists
them, and `git branch <name> <backup-ref>` brings a branch back. Files git ignores inside a
removed worktree (`node_modules`, build output, `.env`) are deleted for good.

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
