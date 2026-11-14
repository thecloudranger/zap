#!/usr/bin/env bash
# Records the wiki screenshots and crops the empty terminal below each one.
#
#   assets/demo/wiki.sh                 # writes assets/demo/wiki/images/*.png
#   assets/demo/wiki.sh ../zap.wiki/images
#
# Recording goes through record.sh, so its leak check runs first. Needs ImageMagick.
set -euo pipefail
here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
out=${1:-$here/wiki/images}
command -v magick >/dev/null 2>&1 || { echo "wiki.sh: magick (ImageMagick) is not on PATH" >&2; exit 1; }
mkdir -p "$here/wiki/shots" "$out"
"$here/record.sh" wiki wiki-picker
for f in "$here"/wiki/shots/*.png; do
  bg=$(magick "$f" -format '%[pixel:p{3,3}]' info:)   # the terminal's own background
  magick "$f" -trim -bordercolor "$bg" -border 56 "$out/$(basename "$f")"
done
rm -f "$here"/wiki/*.gif
echo "wiki.sh: $(ls "$out"/*.png | wc -l | tr -d ' ') screenshots in $out"
