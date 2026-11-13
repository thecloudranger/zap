# Security

zap deletes files, so the bug that matters most is zap removing something it should have kept:
a worktree with uncommitted work, an unmerged branch, or anything in a repository without a
`.zap` file. Treat that as a security problem too.

## Report a problem

Use [Report a vulnerability](https://github.com/thecloudranger/zap/security/advisories/new)
to tell me privately. Please include your zap version (`zap --version`), your OS, the steps that
lead to it, and the output of `zap explain --json <path>` for the affected worktree if you
still have it. Please don't open a public issue for these.

I'll reply within a few days, and I'll credit you in the release notes for the fix unless you
ask me not to.

## Supported versions

Fixes go into the latest release. Update with `brew upgrade zap` or download the newest
release.
