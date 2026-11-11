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
zap version              # Show version
```

```text
~/dev/my-app  ·  3 linked worktrees

  WORKTREE                         BRANCH                  HEAD     STATE
  ~/dev/codex-7f3a                 (detached)              1ad210a  -
  .claude/worktrees/auth-refactor  worktree-auth-refactor  1ad210a  locked
  .worktrees/login-redirect        fix/login-redirect      1ad210a  -
```

## Development

```bash
make build      # ./zap
make test       # unit + end-to-end tests
make check      # everything CI runs
```

## License

[MIT](LICENSE)
