#!/usr/bin/env bash
# Records the demo tapes: the README GIF and the website clips.
#
#   assets/demo/record.sh              # every tape
#   assets/demo/record.sh map check    # only these tapes
#
# Not with `vhs <tape>` directly. This script does three things first, all of which
# a tape cannot do for itself (vhs applies a tape's Env from the first frame):
#
#   Builds zap from this checkout into /tmp/zap-demo/bin, so a recording always shows
#   the code it ships with, not whichever zap happens to be on PATH.
#
#   Builds a throwaway HOME in /tmp/zap-demo/home with made-up repositories
#   (fixture.sh). The tapes point HOME, git's config and ZAP_AGENT_TMP at it.
#
#   Runs every command the tapes run, in plain text against that HOME, and refuses to
#   record if the output names the recorder: their user name, their real home folder,
#   or any path outside the demo. A tape must never film the machine it was made on.
#
# Recording is slow and CPU heavy (a headless browser plus ffmpeg), so vhs runs at the
# lowest priority. Without gifsicle a GIF stays as vhs wrote it.
set -euo pipefail

here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
root=/tmp/zap-demo
H=$root/home

for tool in vhs go git; do
  command -v "$tool" >/dev/null 2>&1 || { echo "record.sh: $tool is not on PATH" >&2; exit 1; }
done

mkdir -p "$root/bin"
(cd "$repo" && CGO_ENABLED=0 go build -trimpath -o "$root/bin/zap" ./cmd/zap)
bash "$here/fixture.sh" "$H" >/dev/null

# The leak check: everything the tapes show, as text.
# The same PATH the tapes use (settings.tape): zap needs lsof from /usr/sbin.
z() { env -i PATH="$root/bin:/usr/bin:/bin:/usr/sbin:/sbin" HOME="$H" GIT_CONFIG_GLOBAL="$H/.gitconfig" \
  ZAP_AGENT_TMP="$H/tmp" TERM=dumb "$root/bin/zap" "$@"; }
out=$root/leakcheck.txt
(
  cd "$H/dev/my-app"
  z scan --all; z explain .claude/worktrees/auth; z clean --all --dry-run
  z check || true; z map; z history; z roots
) > "$out" 2>&1
me=$(id -un)
for needle in "$me" "$HOME" "/Users/" "/home/$me"; do
  if grep -F -q -- "$needle" "$out"; then
    echo "record.sh: the demo output contains \"$needle\"; not recording. See $out" >&2
    exit 1
  fi
done
if grep -E -q '(^|[^~])/(private/)?(tmp|var)/' "$out"; then
  echo "record.sh: the demo output shows a raw temp path; not recording. See $out" >&2
  exit 1
fi

cd "$here"
tapes=("$@")
[ ${#tapes[@]} -eq 0 ] && tapes=(zap scan picker map check undo)
for t in "${tapes[@]}"; do
  bash "$here/fixture.sh" "$H" >/dev/null    # every take starts from the same disk
  echo "recording $t.tape"
  nice -n 19 vhs "$t.tape"
done

if command -v gifsicle >/dev/null 2>&1; then
  for g in *.gif; do nice -n 19 gifsicle -O3 --lossy=30 --batch "$g"; done
fi
echo "done: $(ls "$here"/*.gif "$here"/*.mp4 2>/dev/null | wc -l | tr -d ' ') files in $here"
