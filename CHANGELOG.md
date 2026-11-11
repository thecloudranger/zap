# Changelog

All notable changes to zap are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and zap uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

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
