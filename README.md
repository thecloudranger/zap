<div align="center">
  <h1>⚡ zap</h1>
  <p><b>Find the git worktrees on your disk and see which ones are stale.</b></p>
  <a href="https://github.com/thecloudranger/zap/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/thecloudranger/zap/ci.yml?branch=main&style=flat-square&label=ci" alt="CI"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/thecloudranger/zap?style=flat-square" alt="Go"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" alt="License"></a>
</div>

> zap is in early development. It only reads; it never modifies or deletes anything.

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

## Development

```bash
make build      # ./zap
make test       # unit + end-to-end tests
make check      # everything CI runs
```

## License

[MIT](LICENSE)
