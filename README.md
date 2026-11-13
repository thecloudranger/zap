<div align="center">
  <img src="assets/logo/zap.svg" width="88" alt="zap logo: a git branch whose fork is a lightning bolt">
  <h1>zap</h1>
  <p><b>Clean up after your coding agents.</b></p>
  <p>zap finds the git worktrees that Claude Code, Codex, Cursor and you leave behind, removes the merged ones safely, with undo, and shows what else agents keep on your disk.</p>
  <a href="https://github.com/thecloudranger/zap/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/thecloudranger/zap/ci.yml?branch=main&style=flat-square&label=ci" alt="CI"></a>
  <a href="https://github.com/thecloudranger/zap/releases/latest"><img src="https://img.shields.io/github/v/release/thecloudranger/zap?style=flat-square" alt="Release"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/thecloudranger/zap?style=flat-square" alt="Go"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square" alt="License"></a>
</div>

<p align="center">
  <img src="assets/demo/zap.gif" alt="zap on a demo machine: scan --all lists five worktrees, map draws what agents keep on disk, check reports free space, explain shows why a squash-merged worktree counts as merged, the picker zaps three worktrees after showing their safety checks, and undo brings them back" width="900">
</p>

> [!IMPORTANT]
> **zap deletes only when you say so.** Worktrees are removed only by `zap clean` or the picker,
> after you confirm, in repositories you opted in with a `.zap` file, and only when every safety
> check passes. Branches are backed up first and `zap undo` brings them back. `scan`, `explain`,
> `map`, `check` and `schedule` never delete anything, and zap never deletes agent temp sessions
> or transcripts.

## Install

zap is a single static binary for macOS and Linux (amd64 and arm64).

```bash
brew install thecloudranger/tap/zap
```

Or download the archive for your system from the
[latest release](https://github.com/thecloudranger/zap/releases/latest), check it against
`checksums.txt`, and put `zap` on your `PATH`. The binary is not notarized: if macOS refuses to
open a copy downloaded with a browser, run `xattr -d com.apple.quarantine ~/.local/bin/zap`.
With Go 1.27+: `go install github.com/thecloudranger/zap/cmd/zap@latest`.

## Use

```bash
zap                      # Interactive picker: choose, review, zap
zap scan                 # List worktrees of the repo you're in
zap scan --all           # …or of every repo under your scan roots
zap explain <path>       # Why a worktree got its status: every check, pass/fail
zap map                  # What coding agents keep on this disk
zap check                # One line on merged or empty worktrees and free disk space

zap init                 # Opt this repo in to cleaning (writes .zap)
zap clean --dry-run      # Show what would be removed, and what is kept and why
zap clean                # Remove them, after a confirmation (--all: every opted-in repo)
zap history              # What zap removed, with backup refs
zap undo                 # Bring back what the last clean removed
zap schedule on          # Run `zap check --notify` every day; it never deletes
zap roots                # Show or change where --all looks (zap roots add|remove|reset)
```

Add `--json` to `scan`, `explain`, `clean`, `check` or `map` for scripts and agents, and
`--no-size` to `scan` to skip measuring disk use.

<details>
<summary>Example: <code>zap scan --all</code></summary>

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
The disk has 14 GB free of 494 GB.
```

</details>

## How zap decides

| Status | Meaning |
|---|---|
| `merged` | Its work is already in the target branch: fast-forward, merge commit, squash or rebase merge |
| `empty` | It never got a commit of its own |
| `gone` | Its remote branch was deleted (often after a pull request is merged) |
| `idle` | Unmerged and untouched for `idle_after` (only with `"idle"` in `.zap` rules) |
| `missing` | Git still lists it, but its folder is gone |
| `active` | Anything else |

The target branch is `origin/HEAD`, then `main`, then `master`; override it with `--target`.
AGENT is guessed from the folder a worktree lives in (`.claude/worktrees`, `~/.codex/worktrees`,
`~/.cursor/worktrees`, `conductor/workspaces`, `.t3/worktrees`). `zap scan --all` searches `~/dev`,
`~/code`, `~/src`, `~/projects`, and the agent folders that sit outside repositories
(`~/.codex/worktrees`, `~/.cursor/worktrees`, `~/conductor/workspaces`, `~/.t3/worktrees`).
Add `--fetch` to see pull requests merged
on the remote, and `--gh` to also ask GitHub through the `gh` CLI; a pull request only counts if
its head commit is exactly the branch tip.

## Removing

`zap clean` only acts in repositories that contain a `.zap` file at their root. Everywhere else
it shows what it would do and changes nothing. Commit `.zap` so everyone on the repo shares the
same rules:

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
- not locked with `git worktree lock` (a lock naming a process that is no longer running is
  reported as stale; `--unlock-stale` releases it)
- no uncommitted or untracked changes, and no files whose changes git status is told to hide
  (`assume-unchanged`, or `skip-worktree` outside a sparse checkout)
- no ignored files matching `keep_ignored`, such as `.env` or keys, which are not in git
- no activity within `min_age`
- no other process (a shell, editor or agent) working inside it, and not the directory you
  run zap from

If a check cannot run, the worktree is kept, and zap re-checks each one right before removing it.
Branches are deleted only when their work is provably in the target (or they have none); a
`gone` or `missing` worktree with unmerged commits keeps its branch. Idle worktrees are never
removed by `zap clean`: the picker offers them unselected, and removing one keeps its branch.

> [!NOTE]
> Before deleting a branch, zap saves its tip under `refs/zap/backup/`. `zap undo` recreates the
> branches and re-adds the worktrees at their old paths (`zap history` lists runs, `zap undo <RUN>`
> picks one). Files git ignores inside a removed worktree (`node_modules`, build output, caches)
> are deleted for good.

The first time you clean, zap shows a short disclaimer and asks you to type `zap`. In scripts,
`zap clean --yes` skips the confirmation once the disclaimer has been accepted.

### The picker

`zap` with no command opens a picker. Worktrees zap would remove start selected; kept ones show
why.

| Key | Does |
|---|---|
| `↑` `↓` | Move |
| `space` | Select or unselect a worktree |
| `a` | Select or unselect a whole repository; `[` `]` jump between repositories |
| `e` | Show every safety check for a worktree |
| `i` | Opt the repository in (writes `.zap`) |
| `enter` | Review the selection; then `y` zaps, `n` goes back |
| `q` | Quit without changing anything |

## Before the disk fills

A cleanup script that fails quietly is how a 2 TB disk ends up full. `zap check` scans every
repository under your scan roots and prints one line; it exits `3` when merged or empty worktrees
use at least `--min-stale` (default 1 GB) or free space drops under `--min-free` (default 10 GB),
so it fits cron, CI or a shell prompt. `zap schedule on` runs `zap check --notify` every day
(`--every 6h` for more often) with a desktop notification: a launchd agent on macOS, a systemd
user timer on Linux, a cron line to add yourself on Termux. `zap schedule off` removes it.

```text
$ zap check
4 merged or empty worktrees use 1.4 GB. The disk has 14 GB free of 494 GB.
Agent temp files and transcripts use 23 MB more; see zap map.
Run zap to review them; its safety checks decide what goes.
```

Agents keep more than worktrees. `zap map` draws all of it to scale: worktrees per agent, Claude
Code's temp sessions in `/tmp/claude-<uid>`, and the transcripts Claude Code and Codex need to
resume a session.

<details>
<summary>Example: <code>zap map</code></summary>

```text
$ zap map
Coding agents keep 1.6 GB on this disk. It has 14 GB free of 494 GB.

  ▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒█████████▓░
  ▒ merged or empty worktrees 1.4 GB   █ worktrees in use 218 MB   ▓ temp sessions 19 MB   ░ transcripts 3.8 MB

  Worktrees                    1.6 GB  ▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒████  1.4 GB merged or empty
    Claude Code                940 MB  ▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒               2 worktrees, 940 MB merged or empty
    Codex                      268 MB  ▒▒▒▒▒                           1 worktree, 268 MB merged or empty
    other                      218 MB  ████                            1 worktree
    Cursor                     185 MB  ▒▒▒                             1 worktree, 185 MB merged or empty

  Claude Code temp sessions     19 MB  ▓                               6 sessions in /tmp/claude-501
    ~/dev/my-app                11 MB  ▓                               3 sessions
    ~/dev/api                  8.2 MB  ▓                               3 sessions

  Claude Code transcripts      3.0 MB  ░                               1 session, needed to resume them
    ~/dev/my-app               3.0 MB  ░                               1 session

  Codex transcripts            803 KB  ░                               1 session, needed to resume them
```

</details>

## What it measures

- **SIZE** is what removing a worktree would free, measured like `du`: allocated blocks, not
  file lengths. **DEPS** is the part of it in dependency and build folders such as
  `node_modules`, `.venv` and `target`.
- **Hard-linked files** (a pnpm store, for example) are not counted, because removing one link
  frees nothing; `zap explain` reports them as shared.
- **Free space** is what `df` reports for the disk holding your home folder.
- **"Merged or empty"** is what zap's checks say about a worktree's work, not a promise that
  `zap clean` will remove it: a safety check (an ignored `.env`, a running editor) can still
  keep it.

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
assets/demo/record.sh   # re-record the demo GIF against a made-up HOME (needs vhs)
```

## License

[MIT](LICENSE)
