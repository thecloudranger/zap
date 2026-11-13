#!/usr/bin/env bash
# Builds the throwaway HOME the demo tapes record against. Every repository, branch and
# project name here is made up, so a recording never shows the recorder's own machine.
#
#   assets/demo/fixture.sh /tmp/zap-demo/home          # the README story
#   assets/demo/fixture.sh /tmp/zap-demo/home --deep   # plus long, deep and dotted paths
#
# The story: in ~/dev/my-app, Claude Code's fix-login worktree was merged by fast-forward
# and its auth worktree by squash, a Codex worktree never got a commit, and your own search
# worktree is in progress with an uncommitted file. In ~/dev/api, a Cursor worktree is
# merged but still holds a .env, so zap keeps it. Both repositories are opted in with .zap.
set -euo pipefail
H=${1:?usage: fixture.sh <demo-home> [--deep]}
DEEP=${2:-}
case "$H" in /tmp/*|/private/tmp/*|/var/folders/*) ;; *) echo "fixture.sh: refusing to build outside a temp folder: $H" >&2; exit 2 ;; esac
rm -rf "$H" && mkdir -p "$H"
export HOME=$H GIT_CONFIG_GLOBAL=$H/.gitconfig GIT_CONFIG_NOSYSTEM=1
git config --global user.name dev
git config --global user.email dev@example.com
git config --global init.defaultBranch main

# ago 3d -> a timestamp three days back, with BSD (macOS) or GNU date.
ago() {
  local n=${1%?} u=${1: -1} bsd unit
  case $u in d) bsd=d unit=days ;; h) bsd=H unit=hours ;; *) bsd=M unit=minutes ;; esac
  if date -v-1d >/dev/null 2>&1; then
    date -v-"$n$bsd" "+%Y-%m-%dT%H:%M:%S"
  else
    date -d "-$n $unit" "+%Y-%m-%dT%H:%M:%S"
  fi
}
touchago() { local t; t=$(ago "$1"); t=${t//[-:]/}; t=${t/T/}; touch -t "${t:0:12}.${t:12:2}" "${@:2}"; }
at() { export GIT_AUTHOR_DATE=$(ago "$1") GIT_COMMITTER_DATE=$(ago "$1"); }
# blob <file> <bytes>: filler without the random generator, so building is cheap.
blob() { mkdir -p "$(dirname "$1")"; head -c "$2" /dev/zero | LC_ALL=C tr '\0' 'x' > "$1"; }
deps() { blob "$1/node_modules/.pnpm/chunk.bin" $(($2 * 1000000)); }
age_wt() {  # age_wt <wt-path> <age>: creation, files and admin dir look old
  touchago "$2" "$(git -C "$1" rev-parse --git-dir)/commondir"
  find "$1" -path "$1/node_modules" -prune -o -type f -print0 | while IFS= read -r -d '' f; do touchago "$2" "$f"; done
}
slug() { echo "$1" | tr '/.' '--'; }   # how Claude Code names a project folder

# ~/dev/my-app
R=$H/dev/my-app
mkdir -p "$R" && cd "$R" && git init -q
printf 'node_modules/\n.env\n' > .gitignore
mkdir -p src && echo 'export const login = () => {}' > src/login.ts && echo '# my-app' > README.md
at 20d; git add -A && git commit -qm "init"
echo 'export const auth = () => {}' > src/auth.ts; at 12d; git add -A && git commit -qm "auth scaffold"

git worktree add -q -b worktree-fix-login .claude/worktrees/fix-login
echo 'export const login = (u: string) => u.trim()' > .claude/worktrees/fix-login/src/login.ts
at 3d; git -C .claude/worktrees/fix-login commit -qam "fix login trimming"
deps .claude/worktrees/fix-login 384
git merge -q --ff-only worktree-fix-login
age_wt .claude/worktrees/fix-login 3d

git worktree add -q -b worktree-auth .claude/worktrees/auth
echo 'export const auth = (t: string) => t.length > 0' > .claude/worktrees/auth/src/auth.ts
at 9d; git -C .claude/worktrees/auth commit -qam "auth check"
echo '// tokens' >> .claude/worktrees/auth/src/auth.ts; git -C .claude/worktrees/auth commit -qam "auth tokens"
deps .claude/worktrees/auth 512
git merge -q --squash worktree-auth && at 8d && git commit -qm "auth (#12)"
age_wt .claude/worktrees/auth 8d

git worktree add -q -b search .worktrees/search
echo 'export const search = () => {}' > .worktrees/search/src/search.ts
at 1h; git -C .worktrees/search add -A && git -C .worktrees/search commit -qm "search wip"
echo 'export const rank = () => {}' > .worktrees/search/src/rank.ts
deps .worktrees/search 208

mkdir -p "$H/.codex/worktrees/a1f3"
git worktree add -q --detach "$H/.codex/worktrees/a1f3/my-app" main
deps "$H/.codex/worktrees/a1f3/my-app" 256
age_wt "$H/.codex/worktrees/a1f3/my-app" 2d
printf '# zap rules: defaults\n' > .zap

# ~/dev/api: a merged Cursor worktree that still holds an .env, which zap keeps
A=$H/dev/api
mkdir -p "$A" && cd "$A" && git init -q
printf 'node_modules/\n.env\n' > .gitignore && echo 'package main' > main.go
at 15d; git add -A && git commit -qm "init"
mkdir -p "$H/.cursor/worktrees/api"
git worktree add -q -b cursor-rate-limit "$H/.cursor/worktrees/api/k2p9"
echo '// rate limit' >> "$H/.cursor/worktrees/api/k2p9/main.go"
at 4d; git -C "$H/.cursor/worktrees/api/k2p9" commit -qam "rate limit"
git merge -q --ff-only cursor-rate-limit
echo 'STRIPE_KEY=sk_test_demo' > "$H/.cursor/worktrees/api/k2p9/.env"
deps "$H/.cursor/worktrees/api/k2p9" 176
age_wt "$H/.cursor/worktrees/api/k2p9" 4d
printf '# zap rules: defaults\n' > .zap

# Agent temp sessions (read through ZAP_AGENT_TMP) and transcripts.
T=$H/tmp/claude-$(id -u)
for spec in "$H/dev/my-app:5:60" "$H/dev/api:3:40" "$H/dev/web:2:15"; do
  IFS=: read -r path n mb <<< "$spec"
  mkdir -p "$path"
  for s in $(seq 1 "$n"); do
    blob "$T/$(slug "$path")/s$s/tasks/out.bin" $((mb * 1000000 / n))
    blob "$H/.claude/projects/$(slug "$path")/s$s.jsonl" $((mb * 400000 / n))
  done
done
blob "$H/.codex/sessions/2026/11/rollout-1.jsonl" 90000000

if [ "$DEEP" = --deep ]; then
  newrepo() { mkdir -p "$1" && cd "$1" && git init -q && echo "# $(basename "$1")" > README.md && git add -A && git commit -qm init && printf '# zap rules: defaults\n' > .zap; }
  merged() { git -C "$1" worktree add -q -b "$3" "$2"; echo "$3" > "$2/CHANGE-$3.md"; git -C "$2" add -A; git -C "$2" commit -qm "$3"; git -C "$1" merge -q --ff-only "$3"; deps "$2" "$4"; }
  active() { git -C "$1" worktree add -q -b "$3" "$2"; echo "$3" > "$2/WIP-$3.md"; git -C "$2" add -A; git -C "$2" commit -qm wip; deps "$2" "$4"; }
  P=$H/dev/clients/northwind-traders/platform/services/payments-reconciliation-service
  newrepo "$P"
  merged "$P" "$P/.claude/worktrees/fix-ledger-rounding-for-multi-currency-settlements" worktree-fix-ledger-rounding 96
  merged "$P" "$P/.claude/worktrees/add-idempotency-keys-to-webhook-retries" worktree-idempotency-keys 64
  active "$P" "$P/.claude/worktrees/migrate-reconciliation-jobs-to-temporal-workflows" worktree-temporal-migration 80
  D=$H/dev/my.app.v2
  newrepo "$D"
  merged "$D" "$D/.claude/worktrees/upgrade-to-react-20" worktree-react-20 40
  for name in billing-ui docs-site infra-terraform mobile-app data-pipeline design-tokens auth-service \
    search-indexer cli-tools status-page email-templates feature-flags analytics-dashboard; do
    path=$H/dev/clients/northwind-traders/$name
    mkdir -p "$path"
    blob "$T/$(slug "$path")/s1/out.bin" $(( (${#name} % 9 + 1) * 1000000 ))
    blob "$H/.claude/projects/$(slug "$path")/s1.jsonl" $(( (${#name} % 5 + 1) * 400000 ))
  done
fi

# The one-time disclaimer is already accepted, so clean only asks for its confirmation.
mkdir -p "$H/.config/zap"
printf '[disclaimer]\nversion = 1\naccepted_at = %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$H/.config/zap/config.toml"
echo "demo home ready: $H"
