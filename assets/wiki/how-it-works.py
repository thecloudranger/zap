#!/usr/bin/env python3
"""Writes how-it-works.svg: the animated overview on the wiki's Home page.

    uv run --with fonttools python3 assets/wiki/how-it-works.py [OUT.svg]

One self-contained SVG with CSS keyframes and no script: GitHub shows wiki and README
images with <img>, which plays CSS animation but never runs JavaScript or loads web
fonts. So every letter is drawn as an outline taken from Inter and JetBrains Mono (set
ZAP_FONTS to the folder holding Inter-Regular.otf, Inter-SemiBold.otf and
JetBrainsMono-Regular.ttf; default ~/Library/Fonts), and the text looks the same for
everyone.

Five stages loop every 20 s: agents work, you merge, zap scan, zap clean, zap undo. With
reduced motion the image holds the zap scan stage. The numbers are the demo fixture's
(assets/demo/fixture.sh), so they match the wiki's screenshots.
"""
import os
import sys

from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.ttLib import TTFont

T = 20  # loop length in seconds; stage k runs from 20k% to 20(k+1)%
W, H = 680, 487

C = dict(
    ground="#15151E", panel="#1A1A26", card="#1E1E2E", line="#34344A", ink="#F4F1E8",
    soft="#B9B6C8", faint="#77748A", accent="#FFD21F", red="#F38BA8", purple="#8957E5",
    green="#A6E3A1", blue="#89B4FA",
)

# ---- text as outlines ------------------------------------------------------------------
FONTS = os.path.expanduser(os.environ.get("ZAP_FONTS", "~/Library/Fonts"))
faces = {
    "sans": TTFont(os.path.join(FONTS, "Inter-Regular.otf")),
    "sans-bold": TTFont(os.path.join(FONTS, "Inter-SemiBold.otf")),
    "mono": TTFont(os.path.join(FONTS, "JetBrainsMono-Regular.ttf")),
}
glyph_defs, glyph_ids = [], {}


def glyph(face, name):
    key = (face, name)
    if key not in glyph_ids:
        gs = faces[face].getGlyphSet()
        pen = SVGPathPen(gs)
        gs[name].draw(pen)
        gid = f"g{len(glyph_ids)}"
        glyph_ids[key] = gid
        if pen.getCommands():
            glyph_defs.append(f'<path id="{gid}" d="{pen.getCommands()}"/>')
    return glyph_ids[key]


def _metrics(face):
    f = faces[face]
    return f.getBestCmap(), f["hmtx"], f["head"].unitsPerEm


def width(s, face="sans", size=13):
    cmap, hmtx, upm = _metrics(face)
    return sum(hmtx[cmap.get(ord(ch), cmap[ord("?")])][0] for ch in s) * size / upm


def text(x, y, s, face="sans", size=13, fill=None, anchor="start", cls=None):
    """Draws s with its baseline at y, as <use> references to outlined glyphs."""
    cmap, hmtx, upm = _metrics(face)
    w = width(s, face, size)
    x0 = x - (w / 2 if anchor == "middle" else w if anchor == "end" else 0)
    k = size / upm
    uses, adv = [], 0
    for ch in s:
        name = cmap.get(ord(ch), cmap[ord("?")])
        gid = glyph(face, name)
        if not ch.isspace():
            uses.append(f'<use href="#{gid}" x="{adv:.0f}"/>')
        adv += hmtx[name][0]
    attrs = f' class="{cls}"' if cls else ""
    # The outer group carries the animation; the inner one places and scales the glyphs.
    return (f'<g{attrs}><g fill="{fill or C["ink"]}" transform="translate({x0:.2f} {y:.2f}) scale({k:.5f} {-k:.5f})">'
            + "".join(uses) + "</g></g>")


# ---- animation helpers -----------------------------------------------------------------
css_rules, keyframes = [], []


def anim(name, frames, ease="ease-in-out", base=None):
    """frames: [(percent, {prop: value})]. base: the still style (the zap scan stage),
    used with reduced motion. Returns the class name."""
    if base:
        css_rules.append(f".{name}{{{';'.join(f'{k}:{v}' for k, v in base.items())}}}")
    body = " ".join(f"{p}%{{{';'.join(f'{k}:{v}' for k, v in props.items())}}}" for p, props in frames)
    keyframes.append(f"@keyframes {name}{{{body}}}")
    css_rules.append(f".{name}{{animation:{name} {T}s {ease} infinite both}}")
    return name


def window(name, on, off, fade=1.2):
    """Visible from on% to off%, fading in and out."""
    return anim(name, [(0, {"opacity": 0}), (max(on - fade, 0), {"opacity": 0}), (on, {"opacity": 1}),
                       (off, {"opacity": 1}), (min(off + fade, 100), {"opacity": 0}), (100, {"opacity": 0})],
                base={"opacity": 1 if on <= 50 <= off else 0})


def travel(name, points, start):
    """A token that moves through points [(x, y, seconds-as-percent after start)], then fades."""
    def at(x, y, o):
        return {"transform": f"translate({x}px,{y}px)", "opacity": o}
    x0, y0, _ = points[0]
    fr = [(0, at(x0, y0, 0)), (round(max(start - .01, 0), 2), at(x0, y0, 0))]
    fr += [(round(start + dt, 2), at(x, y, 1)) for x, y, dt in points]
    xl, yl, dl = points[-1]
    fr += [(round(start + dl + .8, 2), at(xl, yl, 0)), (100, at(xl, yl, 0))]
    return anim(name, fr, ease="linear", base={"opacity": 0})


# ---- the scene -------------------------------------------------------------------------
out = []
add = out.append

cards = [  # agent, folder, size, status, colour
    ("Claude Code", "auth", "520 MB", "merged", C["green"]),
    ("Codex", "a1f3", "269 MB", "empty", C["blue"]),
    ("Cursor", "k2p9", "185 MB", "merged", C["green"]),
    ("You", "search", "218 MB", "active", C["faint"]),
]
CW, GAP, X0, CY, CH = 138, 14, 43, 132, 96
SPAN = 4 * CW + 3 * GAP
cx = [X0 + i * (CW + GAP) + CW / 2 for i in range(4)]
zapped = (0, 1)
strike = {0: 65, 1: 69}
PX, PY, PW, PH = 160, 22, 360, 74      # the repository panel
HUB = (340, PY + PH)                    # where the wires leave it
WY = 114                                # the wires' horizontal run


def wire_d(i):
    return f"M{HUB[0]} {HUB[1]}V{WY}H{cx[i]}V{CY}"


def wire_points(i):
    run = abs(cx[i] - HUB[0]) / 120   # longer runs take longer
    return [(HUB[0], HUB[1], 0), (HUB[0], WY, .4), (cx[i], WY, .4 + run), (cx[i], CY, .8 + run)]


add(f'<rect x="0.5" y="0.5" width="{W-1}" height="{H-1}" rx="15.5" fill="{C["ground"]}" stroke="{C["line"]}"/>')

# The repository: its folder, target, and main branch as a line of commits.
add(f'<rect x="{PX+.5}" y="{PY+.5}" width="{PW-1}" height="{PH-1}" rx="10" fill="{C["panel"]}" stroke="{C["line"]}"/>')
add(f'<path d="M{PX} {PY+30}H{PX+PW}" stroke="{C["line"]}"/>')
add(text(PX + 16, PY + 20, "Repository", "sans-bold", 11.5, C["faint"]))
add(text(PX + PW - 16, PY + 20, "target main", "mono", 11.5, C["faint"], "end"))
add(text(PX + 16, PY + 57, "~/dev/my-app", "mono", 13, C["ink"]))
add(text(PX + 134, PY + 57, "main", "mono", 12, C["faint"]))
LX0, LX1, LY = PX + 180, PX + PW - 20, PY + 52
add(f'<path d="M{LX0} {LY}H{LX1}" stroke="{C["line"]}" stroke-width="2"/>')
for x in (LX0 + 8, LX0 + 34, LX0 + 60):
    add(f'<circle cx="{x}" cy="{LY}" r="4" fill="{C["card"]}" stroke="{C["soft"]}" stroke-width="1.5"/>')
merge_x = {0: LX0 + 90, 2: LX0 + 120}
merge_at = {0: 27, 2: 33}  # when each merged worktree's commit lands on main
for i in (0, 2):
    t, x = merge_at[i], merge_x[i]
    dot = anim(f"commit{i}", [(0, {"opacity": 0, "transform": "scale(.2)"}), (t - .2, {"opacity": 0, "transform": "scale(.2)"}),
                              (t + .6, {"opacity": 1, "transform": "scale(1.4)"}), (t + 1.6, {"opacity": 1, "transform": "scale(1)"}),
                              (97, {"opacity": 1, "transform": "scale(1)"}), (100, {"opacity": 0, "transform": "scale(1)"})],
               base={"opacity": 1})
    add(f'<circle class="{dot}" cx="{x}" cy="{LY}" r="4" fill="{C["accent"]}" style="transform-box:fill-box;transform-origin:center"/>')
    add(text(x, LY + 16, cards[i][1], "mono", 9, C["faint"], "middle", cls=window(f"commitlabel{i}", t + 1, 97)))

# Worktree cards, their wires, and the yellow route zap takes to each.
for i, (agent, folder, size, status, col) in enumerate(cards):
    t0 = 2 + 3 * i
    x = X0 + i * (CW + GAP)
    if i in zapped:
        z = strike[i]
        show = [(0, 0), (t0, 0), (t0 + 2, 1), (z, 1), (z + 2.5, 0), (84, 0), (87, 1), (97, 1), (100, 0)]
    else:
        show = [(0, 0), (t0, 0), (t0 + 2, 1), (97, 1), (100, 0)]
    wire = anim(f"wire{i}", [(p, {"opacity": o}) for p, o in show], base={"opacity": 1})
    add(f'<path class="{wire}" d="{wire_d(i)}" fill="none" stroke="{C["line"]}" stroke-width="1.5" stroke-dasharray="3 5"/>')
    # The route lights up while zap works on this worktree: during the scan, and before a strike.
    lit = [(41 + 2 * i, 44 + 2 * i)] + ([(strike[i] - 2.5, strike[i] + .5)] if i in zapped else [])
    fr = [(0, {"opacity": 0})]
    for a, b in lit:
        fr += [(a - .6, {"opacity": 0}), (a, {"opacity": 1}), (b, {"opacity": 1}), (b + .8, {"opacity": 0})]
    fr.append((100, {"opacity": 0}))
    route = anim(f"route{i}", fr, base={"opacity": 0})
    add(f'<path class="{route}" d="{wire_d(i)}" fill="none" stroke="{C["accent"]}" stroke-width="1.5" stroke-linejoin="round"/>')

    if i in zapped:
        ghost = window(f"ghost{i}", strike[i] + 1, 85, fade=1.5)
        add(f'<rect class="{ghost}" x="{x+.5}" y="{CY+.5}" width="{CW-1}" height="{CH-1}" rx="10" fill="none" stroke="{C["faint"]}" stroke-dasharray="4 5"/>')
    cardcls = anim(f"card{i}", [(p, {"opacity": o, "transform": f"translateY({0 if o else 8}px)"}) for p, o in show],
                   base={"opacity": 1})
    add(f'<g class="{cardcls}">')
    add(f'<rect x="{x+.5}" y="{CY+.5}" width="{CW-1}" height="{CH-1}" rx="10" fill="{C["card"]}" stroke="{C["line"]}"/>')
    # The border takes the status colour once zap has scanned the worktree.
    edge = window(f"edge{i}", 44 + 2 * i, 97, fade=1.5)
    add(f'<rect class="{edge}" x="{x+.5}" y="{CY+.5}" width="{CW-1}" height="{CH-1}" rx="10" fill="none" stroke="{C["red"] if i == 2 else col}" stroke-opacity=".6"/>')
    add(text(x + 14, CY + 22, agent, "sans", 12, C["soft"]))
    add(text(x + 14, CY + 44, folder, "mono", 14, C["ink"]))
    if i in zapped:
        add(text(x + 14, CY + 63, size, "mono", 11.5, C["soft"], cls=window(f"size{i}", 0, 84, fade=1.5)))
        add(text(x + 14, CY + 63, "no node_modules", "mono", 11.5, C["faint"], cls=window(f"nodeps{i}", 87, 97, fade=1.5)))
    else:
        add(text(x + 14, CY + 63, size, "mono", 11.5, C["soft"]))
    tw = width(status, "mono", 9.5) + 10
    parts = [f'<rect x="{x+14}" y="{CY+71}" width="{tw:.1f}" height="16" rx="8" fill="none" stroke="{col}"/>',
             text(x + 14 + tw / 2, CY + 82.5, status, "mono", 9.5, col, "middle")]
    assert (17 + tw + width("keeps .env", "mono", 9.5) + 10 if i == 2 else 14 + tw) <= CW - 8, "tags overflow the card"
    if i == 2:
        kw = width("keeps .env", "mono", 9.5) + 10
        parts += [f'<rect x="{x+17+tw:.1f}" y="{CY+71}" width="{kw:.1f}" height="16" rx="8" fill="none" stroke="{C["red"]}"/>',
                  text(x + 17 + tw + kw / 2, CY + 82.5, "keeps .env", "mono", 9.5, C["red"], "middle")]
    if i == 3:
        parts.append(text(x + 19 + tw, CY + 82.5, "dirty", "mono", 9.5, C["faint"]))
    add(f'<g class="{window(f"tag{i}", 44 + 2 * i, 97, fade=1.5)}">' + "".join(parts) + "</g>")
    add("</g>")

# You merge: a commit travels up from each merged worktree to main.
for i in (0, 2):
    fwd = wire_points(i)
    total = fwd[-1][2]
    pts = [(x, y, round(total - dt, 2)) for x, y, dt in reversed(fwd)]
    pts.append((merge_x[i], LY, total + 1.0))
    add(f'<circle class="{travel(f"merge{i}", pts, merge_at[i] - total - 1.0)}" r="4.5" fill="{C["accent"]}"/>')

# zap scan: a token runs down each wire in turn.
for i in range(4):
    add(f'<circle class="{travel(f"scan{i}", wire_points(i), 41 + 2 * i)}" r="4.5" fill="{C["accent"]}"/>')

# zap clean: a bolt on each merged or empty worktree; its branch drops into the backup refs.
for i in zapped:
    z = strike[i]
    bolt = anim(f"bolt{i}", [(0, {"opacity": 0}), (z - .5, {"opacity": 0}), (z, {"opacity": 1}), (z + 1.5, {"opacity": 1}),
                             (z + 2.5, {"opacity": 0}), (100, {"opacity": 0})], ease="linear", base={"opacity": 0})
    bx, by = cx[i] + 34, CY - 50
    add(f'<path class="{bolt}" d="M{bx+6} {by}L{bx-12} {by+32}H{bx}L{bx-8} {by+58}L{bx+14} {by+22}H{bx+2}L{bx+10} {by}Z" fill="{C["accent"]}"/>')

SY = CY + CH + 16
SHELF_NOTE = "Backup: each branch's last commit, for zap undo"
add(f'<g class="{window("shelf", 63, 97, fade=1.5)}"><rect x="{X0+.5}" y="{SY+.5}" width="{SPAN-1}" height="44" rx="8" fill="none" stroke="{C["line"]}" stroke-dasharray="4 5"/>'
    + text(X0 + SPAN - 14, SY + 19, SHELF_NOTE, "sans", 12, C["soft"], "end")
    + text(X0 + SPAN - 14, SY + 35, "kept in git under refs/zap/backup/", "mono", 10, C["faint"], "end") + "</g>")
for i, label in zip(zapped, ("worktree-auth", "a1f3")):
    z = strike[i]
    w = width(label, "mono", 11) + 18
    q = lambda y, o=1: {"transform": f"translateY({y}px)", "opacity": o}
    chip = anim(f"chip{i}", [(0, q(-46, 0)), (z + .5, q(-46, 0)), (z + 1, q(-46)), (z + 3.5, q(0)), (84, q(0)),
                             (86.5, q(-46, 0)), (100, q(-46, 0))], base={"opacity": 0})
    add(f'<g class="{chip}"><rect x="{cx[i]-w/2:.1f}" y="{SY+13}" width="{w:.1f}" height="18" rx="9" fill="{C["card"]}" stroke="{C["accent"]}"/>'
        + text(cx[i], SY + 26, label, "mono", 11, C["ink"], "middle") + "</g>")

assert X0 + SPAN - 14 - width(SHELF_NOTE, "sans", 12) > cx[1] + 40, "shelf note runs into the chips"

# Disk use of the four worktrees, against 1.3 GB.
DY = SY + 69
add(text(X0, DY, "Disk used by agent worktrees", "sans", 12, C["soft"]))
for name, val, on, off in (("v1", "1.2 GB", 13, 66), ("v1b", "671 MB", 68, 70), ("v2", "403 MB", 72, 84), ("v3", "about 405 MB", 88, 97)):
    add(text(X0 + SPAN, DY, val, "mono", 12, C["ink"], "end", cls=window(name, on, off, fade=1.5)))
add(f'<rect x="{X0}" y="{DY+8}" width="{SPAN}" height="8" rx="4" fill="{C["card"]}"/>')
fill = anim("disk", [(0, {"transform": "scaleX(0)"}), (2, {"transform": "scaleX(0)"}), (5, {"transform": "scaleX(.40)"}),
                     (8, {"transform": "scaleX(.61)"}), (11, {"transform": "scaleX(.75)"}), (14, {"transform": "scaleX(.92)"}),
                     (65, {"transform": "scaleX(.92)"}), (67.5, {"transform": "scaleX(.52)"}), (69, {"transform": "scaleX(.52)"}),
                     (71.5, {"transform": "scaleX(.31)"}), (85, {"transform": "scaleX(.31)"}), (88, {"transform": "scaleX(.312)"}),
                     (97, {"transform": "scaleX(.312)"}), (100, {"transform": "scaleX(0)"})],
            base={"transform": "scaleX(.92)"})
add(f'<rect class="{fill}" x="{X0}" y="{DY+8}" width="{SPAN}" height="8" rx="4" fill="{C["purple"]}" style="transform-box:fill-box;transform-origin:left center"/>')

# The stepper, the caption band and the legend.
tabs = ["Agents work", "You merge", "zap scan", "zap clean", "zap undo"]
caps = [
    "Each coding agent works in its own worktree, with its own dependencies.",
    "You merge their work into main. The worktrees stay on your disk.",
    "zap scan checks each worktree: merged or empty, and anything it has to keep.",
    "zap clean saves each branch's last commit, then deletes the worktree folders and frees the space.",
    "zap undo brings them back. You reinstall node_modules and other ignored files.",
]
TY, TH = DY + 34, 44
TW = W / 5
add(f'<path d="M0 {TY}H{W}M0 {TY+TH}H{W}" stroke="{C["line"]}"/>')
for k, (tab, cap) in enumerate(zip(tabs, caps)):
    a, b = 20 * k, 20 * (k + 1)
    if k:
        add(f'<path d="M{k*TW:.1f} {TY}V{TY+TH}" stroke="{C["line"]}"/>')
    face = "mono" if tab.startswith("zap") else "sans-bold"
    lit = anim(f"tab{k}", [(0, {"opacity": .45}), (max(a - .01, 0), {"opacity": .45}), (a, {"opacity": 1}),
                           (b - .01, {"opacity": 1}), (b, {"opacity": .45}), (100, {"opacity": .45})], ease="step-end",
               base={"opacity": 1 if k == 2 else .45})
    add(text(k * TW + TW / 2, TY + 27, tab, face, 13.5, C["ink"], "middle", cls=lit))
    tw = width(tab, face, 13.5) + 24
    bar = anim(f"prog{k}", [(0, {"transform": "scaleX(0)"}), (a, {"transform": "scaleX(0)"}), (b - .3, {"transform": "scaleX(1)"}),
                            (b, {"transform": "scaleX(0)"}), (100, {"transform": "scaleX(0)"})], ease="linear",
               base={"transform": f"scaleX({1 if k == 2 else 0})"})
    add(f'<rect class="{bar}" x="{k*TW+TW/2-tw/2:.1f}" y="{TY+TH-2}" width="{tw:.1f}" height="2" fill="{C["accent"]}" style="transform-box:fill-box;transform-origin:left center"/>')
    add(text(24, TY + TH + 29, cap, "sans", 13.5, C["soft"], cls=window(f"cap{k}", a + .8, b - .8, fade=.8)))

LG = TY + TH + 48
add(f'<path d="M0 {LG}H{W}" stroke="{C["line"]}"/>')
lx, ly = 24, LG + 27
add(f'<path d="M{lx} {ly-4}H{lx+16}" stroke="{C["accent"]}" stroke-width="1.5"/>')
add(text(lx + 22, ly, "zap's route", "sans", 12, C["soft"]))
lx += 22 + width("zap's route", "sans", 12) + 20
for label, col in (("merged", C["green"]), ("empty", C["blue"]), ("active", C["faint"]), ("keeps .env", C["red"])):
    w = width(label, "mono", 10) + 14
    add(f'<rect x="{lx:.1f}" y="{ly-12}" width="{w:.1f}" height="16" rx="8" fill="none" stroke="{col}"/>')
    add(text(lx + w / 2, ly - .5, label, "mono", 10, col, "middle"))
    lx += w + 10
lx += 10
w = width("branch", "mono", 10) + 16
add(f'<rect x="{lx:.1f}" y="{ly-12}" width="{w:.1f}" height="16" rx="8" fill="{C["card"]}" stroke="{C["accent"]}"/>')
add(text(lx + w / 2, ly - .5, "branch", "mono", 10, C["ink"], "middle"))
add(text(lx + w + 8, ly, "backup", "sans", 12, C["soft"]))
assert LG + 44 <= H, (LG, H)

label = ("How zap works, in five steps. Coding agents each work in their own git worktree. "
         "You merge their work into main, and the worktrees stay on disk. zap scan marks two as merged, one as empty "
         "and one as active, and keeps a merged one because it holds an ignored .env file. zap clean backs up "
         "each branch and removes the merged and empty worktrees, freeing disk space. zap undo brings them back.")
static = "@media (prefers-reduced-motion: reduce){*{animation:none!important}}"
svg = (f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}" viewBox="0 0 {W} {H}" role="img" aria-label="{label}">'
       f'<title>{label}</title><defs>{"".join(glyph_defs)}</defs>'
       f'<style>{" ".join(css_rules)} {" ".join(keyframes)} {static}</style>'
       + "".join(out) + "</svg>\n")
path = sys.argv[1] if len(sys.argv) > 1 else "how-it-works.svg"
open(path, "w").write(svg)
print(f"wrote {path} ({len(svg)//1024} KB, {len(glyph_ids)} glyphs)")
