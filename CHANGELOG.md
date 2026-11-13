# Changelog

All notable changes to zap are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and zap uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.2.2]

### Fixed

- The release publishes the Homebrew cask (0.2.0 and 0.2.1 shipped binaries only).

## [0.2.1]

### Fixed

- The release now publishes the Homebrew cask, so `brew install thecloudranger/tap/zap`
  works. 0.2.0 shipped its binaries but not the cask.

## [0.2.0]

### Added

- `zap check`: one line on merged or empty worktrees and free disk space. Exits 3 when
  they use at least `--min-stale` (default 1 GB) or free space is under `--min-free`
  (default 10 GB), for cron, CI or a shell prompt. `--notify` adds a desktop notification.
- `zap schedule on|off|status`: run `zap check --notify` every day (`--every` to change)
  with a launchd agent on macOS or a systemd user timer on Linux; prints a cron line on
  Termux. It never deletes anything.
- `zap map`: what coding agents keep on the disk, to scale: worktrees per agent, Claude
  Code temp sessions in `/tmp/claude-<uid>`, and Claude Code and Codex transcripts. Temp
  sessions and transcripts are reported only, never deleted. `--tree` (experimental) draws
  nested blocks sized by disk use.
- `scan --all` and `clean` show the disk's free space and total.
- Install with Homebrew: `brew install thecloudranger/tap/zap`.

### Fixed

- A worktree created from someone else's unmerged commits
  (`git worktree add -b x origin/feature`) was reported `empty` and offered for removal.
  `empty` now also requires the tip to be in the target branch.
- After `zap undo`, a restored merged worktree was reported `empty` instead of `merged`.
- With a home folder reached through a symlink, paths were shown in full instead of as
  `~/...`.

## [0.1.0]

First release.

### Added

- `zap scan`: list a repository's worktrees, or every repository's with `--all`, with
  status (merged, empty, gone, missing, active), agent, size, dependencies, age and
  last activity. `--json` for scripts and agents.
- Merge detection for fast-forward, merge-commit, squash and rebase merges, deleted
  upstreams, and, with `--gh`, pull requests merged on GitHub at the branch tip.
- `zap explain PATH`: every check and safety gate behind a worktree's status.
- `zap init`, `zap clean`: remove merged, empty and missing worktrees of repositories
  that opted in with a `.zap` file, after a one-time disclaimer and a confirmation.
  `--dry-run`, `--yes`, `--all`, `--fetch`, `--unlock-stale`.
- Safety gates: main and protected branches, locks, uncommitted or hidden changes,
  ignored secrets (`keep_ignored`), `min_age`, processes working in the worktree, and
  the current directory. Everything is re-checked right before removal.
- Backups of every deleted branch under `refs/zap/backup/`, `zap history` and
  `zap undo`.
- `zap` with no command: an interactive picker.
- Opt-in `idle` rule, offered in the picker only.
- `zap roots` to choose where `--all` looks.
