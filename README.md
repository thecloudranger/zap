<div align="center">
  <h1>⚡ zap</h1>
  <p><b>Find stale git worktrees and remove them safely.</b></p>
  <a href="https://github.com/thecloudranger/zap/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/thecloudranger/zap/ci.yml?branch=main&style=flat-square&label=ci" alt="CI"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/thecloudranger/zap?style=flat-square" alt="Go"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" alt="License"></a>
</div>

> zap is in early development. It only deletes in repositories that opted in with a `.zap` file.

## Quick Start

zap is a single static binary for macOS and Linux (amd64 and arm64).

**Download** the archive for your system from the
[latest release](https://github.com/thecloudranger/zap/releases/latest), check it against
`checksums.txt`, and put `zap` on your `PATH`:

```bash
tar xzf zap_*_darwin_arm64.tar.gz && mv zap ~/.local/bin/
```

The binary is not notarized: if macOS refuses to open a copy downloaded with a browser, run
`xattr -d com.apple.quarantine ~/.local/bin/zap`.

**Or build it** with Go 1.27+:

```bash
go install github.com/thecloudranger/zap/cmd/zap@latest
```

**Run**

```bash
zap                      # Interactive picker: choose, review, zap
zap scan                 # List worktrees of the repo you're in
zap scan ~/dev/my-app    # …or of any repo by path
zap scan --all           # …or of every repo under your scan roots
zap scan --json          # JSON output for scripts and agents
zap scan --no-size       # Skip measuring disk use
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

  WORKTREE                     BRANCH   AGENT  STATUS  SIZE    DEPS    AGE  ACTIVE  NOTES
  ~/.codex/worktrees/a1f3/api  a85e7da  codex  empty   412 MB  398 MB  6h   6h      detached

1 empty  ·  412 MB, 412 MB merged or empty

~/dev/my-app  ·  2 linked worktrees  ·  target main

  WORKTREE                BRANCH         AGENT   STATUS  SIZE    DEPS    AGE  ACTIVE  NOTES
  .claude/worktrees/auth  worktree-auth  claude  merged  1.1 GB  1.0 GB  3d   2d      -
  .worktrees/login        fix/login      -       active  640 MB  590 MB  5d   2h      dirty(1)

1 merged · 1 active  ·  1.7 GB, 1.1 GB merged or empty

Scanned 2 repositories in 1.2s; 2 with linked worktrees. Roots: zap roots
Linked worktrees use about 2.2 GB; 1.5 GB of that is in merged or empty ones.
```

| Status | Meaning |
|---|---|
| `merged` | Its work is already in the target branch: fast-forward, merge commit, squash or rebase merge |
| `empty` | It never got a commit of its own |
| `gone` | Its remote branch was deleted (often after a pull request is merged) |
| `idle` | Unmerged and untouched for `idle_after` (only with `"idle"` in `.zap` rules) |
| `missing` | Git still lists it, but its folder is gone |
| `active` | Anything else |

The target branch is `origin/HEAD`, then `main`, then `master`; override it with `--target`.
AGENT is guessed from the folder a worktree lives in (for example `.claude/worktrees` or
`~/.codex/worktrees`). `zap scan` outside a repository, or with `--all`, searches `~/dev`, `~/code`,
`~/src`, `~/projects` and known agent worktree folders; `zap roots` changes that list.
Add `--fetch` to `scan`, `explain` or `clean` to fetch remotes first, so pull requests merged on
GitHub are seen. Add `--gh` to also ask GitHub (through the `gh` CLI) for merged pull requests; one
only counts if its head commit is exactly the branch tip.

SIZE is what removing a worktree would free, measured like `du`; DEPS is the part of it in
dependency and build folders such as `node_modules`, `.venv` and `target`. Hard-linked files
(a pnpm store, for example) are not counted, because removing one link frees nothing;
`zap explain` reports them as shared.

## Cleaning safely

`zap clean` only acts in repositories that contain a `.zap` file at their root. Everywhere
else it shows what it would do and changes nothing. Commit `.zap` so everyone on the repo
shares the same rules:

```toml
# .zap (every key is optional; these are the defaults)
# target = "main"                       # default: origin/HEAD, then main, then master
rules         = ["merged", "empty", "missing"]   # "gone" and "idle" can be added
min_age       = "1d"                    # skip anything active more recently (m, h, d, w)
idle_after    = "30d"                   # when an unmerged worktree counts as idle
delete_branch = true
protect       = ["main", "master"]      # branch globs that are never removed
keep_ignored  = [".env", ".env.*", ".envrc", "*.pem", "*.key", "*.p12", "*.pfx", "*.tfstate"]
```

A worktree is removed only if it passes every check:

- not the main worktree, and not on a protected or target branch
- not locked with `git worktree lock`
- no uncommitted or untracked changes, and no files whose changes git status is told to hide
  (`assume-unchanged`, or `skip-worktree` outside a sparse checkout)
- no ignored files matching `keep_ignored`, such as `.env` or keys, which are not in git
- no activity within `min_age`
- no other process (a shell, editor or agent) working inside it, and not the directory you
  run zap from

If a check cannot run, the worktree is kept. zap re-checks each worktree right before removing
it and skips anything that changed.

A lock whose reason names a process that is no longer running is reported as stale;
`zap clean --unlock-stale` releases it. Idle worktrees are never removed by `zap clean`: the
picker offers them unselected, and removing one keeps its branch.

Branches are deleted only when their work is provably in the target (or they have none);
a `gone` or `missing` worktree with unmerged commits loses its folder or registration but keeps
its branch. Before deleting a branch, zap saves its tip under `refs/zap/backup/`.

`zap undo` brings back the last run: it recreates the branches from those backups and re-adds
the worktrees at their old paths (`zap history` lists runs; `zap undo <RUN>` picks one). Other
files git ignores inside a removed worktree (`node_modules`, build output, caches) are deleted
for good and cannot be restored.

Running `zap` with no command opens a picker: worktrees zap would remove start selected, kept
ones show why, `e` shows every safety check for a worktree, `i` opts a repository in, and
`enter` reviews the selection before anything is removed.

The first time you clean, zap asks you to read a short disclaimer and type `zap`. In scripts,
`zap clean --yes` skips the confirmation (the disclaimer must have been accepted once), and
`--json` gives machine-readable output.

## With a coding agent

[`skills/zap/SKILL.md`](skills/zap/SKILL.md) teaches an agent to use zap safely: look with
`--json` and `--dry-run`, and remove only after you agree. For Claude Code:

```bash
mkdir -p ~/.claude/skills/zap
curl -fsSL https://raw.githubusercontent.com/thecloudranger/zap/main/skills/zap/SKILL.md \
  -o ~/.claude/skills/zap/SKILL.md
```

## Development

```bash
make build      # ./zap
make test       # unit + end-to-end tests
make check      # everything CI runs
```

## License

[MIT](LICENSE)
