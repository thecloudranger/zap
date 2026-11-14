#!/usr/bin/env python3
"""Shows each wiki screenshot at half its pixel width, so 2x recordings stay sharp.

    python3 assets/wiki/set-widths.py ../zap.wiki

Rewrites ![alt](images/x.png) and earlier <img src="images/x.png" ...> tags in every page
to <img src="images/x.png" width="HALF" alt="alt">. Needs ImageMagick's identify.
"""
import html, pathlib, re, subprocess, sys

wiki = pathlib.Path(sys.argv[1])
def half(png):
    w = subprocess.run(["magick", "identify", "-format", "%w", str(wiki / png)], capture_output=True, text=True, check=True).stdout
    return int(w) // 2

md = re.compile(r'!\[([^\]]*)\]\((images/[^)]+\.png)\)')
tag = re.compile(r'<img src="(images/[^"]+\.png)" width="\d+" alt="([^"]*)">')
for page in wiki.glob("*.md"):
    s = page.read_text()
    s2 = md.sub(lambda m: f'<img src="{m[2]}" width="{half(m[2])}" alt="{html.escape(m[1], quote=True)}">', s)
    s2 = tag.sub(lambda m: f'<img src="{m[1]}" width="{half(m[1])}" alt="{m[2]}">', s2)
    if s2 != s:
        page.write_text(s2)
        print("updated", page.name)
