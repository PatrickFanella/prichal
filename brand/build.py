#!/usr/bin/env python3
"""Build the Prichal brand pack: marks, lockups, social and video layouts.

Output follows the Subcult starter-template layout (logos/, social/, video/,
tokens, source register, asset manifest) so the pack can move into the shared
branding library unchanged. Run from anywhere:

    python3 brand/build.py           # SVG only
    python3 brand/build.py --render  # also PNG (needs rsvg-convert)
    python3 brand/build.py --render --outline  # also outlined SVG (needs inkscape)
"""
from pathlib import Path
import argparse, hashlib, html, json, os, shutil, subprocess, tempfile
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parent
BRAND = json.loads((ROOT / "brand.json").read_text())
C = BRAND["colors"]
F = BRAND["fonts"]
GENERATED = ["logos", "social", "video", "tokens.json", "tokens.css", "asset-manifest.json"]


def lum(hex_color):
    v = [int(hex_color[i:i + 2], 16) / 255 for i in (1, 3, 5)]
    v = [x / 12.92 if x <= .04045 else ((x + .055) / 1.055) ** 2.4 for x in v]
    return .2126 * v[0] + .7152 * v[1] + .0722 * v[2]


def contrast(a, b):
    lo, hi = sorted([lum(a), lum(b)])
    return (hi + .05) / (lo + .05)


# Text pairings used for ordinary-size text. Each must reach 4.5:1.
TEXT_PAIRS = {
    "text on background": (C["text"], C["background"]),
    "accent on background": (C["accent"], C["background"]),
    "secondary on background": (C["secondary"], C["background"]),
    "background on text": (C["background"], C["text"]),
    "dark_accent on text": (C["dark_accent"], C["text"]),
}
for label, (fg, bg) in TEXT_PAIRS.items():
    if contrast(fg, bg) < 4.5:
        raise SystemExit(f"{label} contrast {contrast(fg, bg):.2f} is below 4.5:1")


# --- Mark -------------------------------------------------------------------
# A docking node: a sphere with its equator drawn in perspective and short
# collars at the four in-plane ports. The red flange is the occupied port.
# There is deliberately no centre dot or inner ring; with the cross of ports
# those read as a gun sight.

def mark(x, y, size, ink, accent, field):
    s = size / 512

    def port(angle, color):
        return (f'<g transform="rotate({angle} 256 256)">'
                f'<rect x="218" y="62" width="76" height="70" fill="{ink}"/>'
                f'<rect x="196" y="36" width="120" height="34" fill="{color}"/></g>')

    ports = port(0, ink) + port(180, ink) + port(270, ink) + port(90, accent)
    body = (f'<circle cx="256" cy="256" r="134" fill="{field}" stroke="{ink}" stroke-width="26"/>'
            f'<path d="M 122 256 A 134 44 0 0 0 390 256" fill="none" stroke="{ink}" stroke-width="14"/>'
            f'<path d="M 122 256 A 134 44 0 0 1 390 256" fill="none" stroke="{ink}" stroke-width="6" '
            f'stroke-dasharray="14 12"/>')
    return f'<g transform="translate({x} {y}) scale({s:.5f})">{ports}{body}</g>'


def text(x, y, s, size, font, color, weight=400, spacing=0, anchor="start"):
    return (f'<text x="{x}" y="{y}" fill="{color}" font-family="{html.escape(font, quote=True)}" '
            f'font-size="{size}" font-weight="{weight}" letter-spacing="{spacing}" '
            f'text-anchor="{anchor}">{html.escape(s)}</text>')


def rect(x, y, w, h, color):
    return f'<rect x="{x}" y="{y}" width="{w}" height="{h}" fill="{color}"/>'


def wordmark(x, y, size, color):
    return text(x, y, BRAND["name"].upper(), size, F["display"], color, 700, size * .06)


def ticks(x, y, length, color, step=24, vertical=True):
    """Engineering-drawing scale: short ticks with a longer one every fourth."""
    out = []
    for i, p in enumerate(range(0, int(length) + 1, step)):
        t = 18 if i % 4 == 0 else 9
        out.append(rect(x, y + p, t, 2, color) if vertical else rect(x + p, y, 2, t, color))
    return "".join(out)


def label(x, y, s, size=18, color=None, anchor="start"):
    return text(x, y, s, size, F["label"], color or C["secondary"], 400, size * .12, anchor)


# --- Layouts ----------------------------------------------------------------

def scheme(dark):
    if dark:
        return dict(bg=C["text"], ink=C["background"], accent=C["dark_accent"], muted="#aab0ba", rule="#3a4252")
    return dict(bg=C["background"], ink=C["text"], accent=C["accent"], muted=C["secondary"], rule=C["rule"])


def frame(w, h, k, mx):
    """Paper field, left tick scale and the port-label strip shared by layouts."""
    return (rect(0, 0, w, h, k["bg"])
            + ticks(mx * .4, mx, h - 2 * mx, k["rule"])
            + rect(mx, h - mx, w - 2 * mx, 2, k["rule"]))


def port_strip(x, y, k, size=16):
    return label(x, y, "PORT 01 · 02 · 03 · 04 · 05 · 06", size, k["muted"])


def hero(w, h, dark=False, eyebrow=None, headline=None, sub=None, mark_size=None, footer=True):
    k = scheme(dark)
    mx = round(min(w, h) * .09)
    m = mark_size or round(min(w * .3, h * .62))
    s = frame(w, h, k, mx)
    s += mark(mx, (h - m) / 2, m, k["ink"], k["accent"], k["bg"])
    tx = mx + m + mx * .7
    room = w - tx - mx
    head = headline or BRAND["name"].upper()
    size = min(h * .26, room / (len(head) * .6))
    ty = h / 2 + size * .30
    s += label(tx, ty - size * 1.02, eyebrow or f'{BRAND["cyrillic"]} · ACCOUNT HOSTING', max(14, h * .022, size * .13), k["muted"])
    s += text(tx, ty, head, size, F["display"], k["ink"], 700, size * .06)
    s += rect(tx, ty + size * .2, min(room, size * 1.6), max(4, size * .045), k["accent"])
    s += text(tx, ty + size * .2 + max(30, h * .07, size * .34), sub or BRAND["tagline"], max(18, h * .04, size * .22), F["body"], k["ink"])
    if footer:
        s += port_strip(mx, h - mx * .45, k, max(13, h * .028))
        s += label(w - mx, h - mx * .45, BRAND["website"], max(13, h * .028), k["muted"], "end")
    return s


def stacked(w, h, dark=False, kicker="", body_lines=(), note=None):
    k = scheme(dark)
    mx = round(w * .09)
    m = round(w * .26)
    top = mx + (170 if h > 1500 else 0)
    s = frame(w, h, k, mx)
    s += mark(mx, top, m, k["ink"], k["accent"], k["bg"])
    s += wordmark(mx + m + 36, top + m * .64, m * .42, k["ink"])
    y = top + m + mx
    s += rect(mx, y, w - 2 * mx, 4, k["accent"])
    s += label(mx, y + 64, kicker, 22, k["muted"])
    size = 64 if w < 1200 else 72
    for i, line in enumerate(body_lines):
        s += text(mx, y + 150 + i * size * 1.18, line, size, F["display"], k["ink"], 700, 1)
    if note:
        s += text(mx, y + 170 + len(body_lines) * size * 1.18, note, 30, F["body"], k["muted"])
    s += port_strip(mx, h - mx * .45, k, 18)
    s += label(w - mx, h - mx * .45, BRAND["website"], 18, k["muted"], "end")
    return s


def lockup(w, h, dark=False, mono=False):
    k = scheme(dark)
    accent = k["ink"] if mono else k["accent"]
    m = h * .74
    pad = (h - m) / 2
    return (mark(pad, pad, m, k["ink"], accent, k["bg"])
            + wordmark(pad + m + h * .16, h * .66, h * .5, k["ink"])
            + label(pad + m + h * .17, h * .84, "ACCOUNT HOSTING", h * .065, k["muted"]))


def standalone_mark(w, h, dark=False, mono=False, field=False):
    k = scheme(dark)
    accent = k["ink"] if mono else k["accent"]
    bg = rect(0, 0, w, h, k["bg"]) if field else ""
    inset = w * .1 if field else 0
    return bg + mark(inset, inset, w - 2 * inset, k["ink"], accent, k["bg"])


def screenshot_frame(w, h):
    k = scheme(False)
    mx = 96
    s = frame(w, h, k, mx) + mark(mx, mx - 10, 72, k["ink"], k["accent"], k["bg"])
    s += wordmark(mx + 96, mx + 46, 44, k["ink"])
    s += label(w - mx, mx + 40, "VERIFIED CAPTURE · DATE · SOURCE", 16, k["muted"], "end")
    s += (f'<rect x="{mx}" y="{mx + 100}" width="{w - 2 * mx}" height="{h - 2 * mx - 160}" fill="#fffdf8" '
          f'stroke="{k["ink"]}" stroke-width="2"/>')
    s += text(w / 2, h / 2 + 10, "Place a verified screenshot here.", 30, F["body"], k["muted"], anchor="middle")
    return s


def lower_third(w, h):
    k = scheme(True)
    return (rect(96, 820, 1180, 175, k["bg"]) + rect(96, 820, 10, 175, k["accent"])
            + mark(130, 845, 125, k["ink"], k["accent"], k["bg"])
            + text(290, 895, "TITLE / SPEAKER", 52, F["display"], k["ink"], 700, 2)
            + text(290, 955, "A short line of context.", 30, F["body"], k["ink"]))


LAYOUTS = {
    "logos/mark": (512, 512, lambda w, h: standalone_mark(w, h)),
    "logos/mark-dark": (512, 512, lambda w, h: standalone_mark(w, h, dark=True, field=True)),
    "logos/mark-mono": (512, 512, lambda w, h: standalone_mark(w, h, mono=True)),
    "logos/mark-mono-dark": (512, 512, lambda w, h: standalone_mark(w, h, dark=True, mono=True, field=True)),
    "logos/lockup": (1200, 300, lambda w, h: lockup(w, h)),
    "logos/lockup-dark": (1200, 300, lambda w, h: rect(0, 0, w, h, C["text"]) + lockup(w, h, dark=True)),
    "logos/lockup-mono": (1200, 300, lambda w, h: lockup(w, h, mono=True)),
    "social/avatar": (512, 512, lambda w, h: standalone_mark(w, h, field=True)),
    "social/banner": (1500, 500, lambda w, h: hero(w, h)),
    "social/banner-tall": (1500, 600, lambda w, h: hero(w, h)),
    "social/landscape-post": (1600, 900, lambda w, h: hero(w, h, headline="DOCK YOUR ACCOUNT.", sub="We handle the rest.")),
    "social/square-post": (1080, 1080, lambda w, h: stacked(w, h, kicker="ACCOUNT HOSTING", body_lines=("DOCK YOUR", "ACCOUNT.", "WE HANDLE", "THE REST."), note="Placeholder copy. Replace before publishing.")),
    "social/story": (1080, 1920, lambda w, h: stacked(w, h, kicker="FOR CREATORS", body_lines=("YOUR NAME.", "YOUR POSTS.", "YOUR SERVER."), note="Placeholder copy. Replace before publishing.")),
    "social/quote": (1080, 1080, lambda w, h: stacked(w, h, dark=True, kicker="QUOTE", body_lines=("EXACT QUOTE", "GOES HERE."), note="Attribution / source")),
    "social/release": (1080, 1080, lambda w, h: stacked(w, h, kicker="RELEASE · VERSION", body_lines=("VERIFIED", "CHANGE."), note="Useful consequence for owners.")),
    "social/screenshot-frame": (1600, 1000, screenshot_frame),
    "video/title-card": (1920, 1080, lambda w, h: hero(w, h, dark=True)),
    "video/thumbnail": (1920, 1080, lambda w, h: hero(w, h, headline="EPISODE TITLE", sub="Placeholder. One concrete promise.")),
    "video/end-card": (1920, 1080, lambda w, h: hero(w, h, dark=True, sub=BRAND["call_to_action"])),
    "video/lower-third": (1920, 1080, lower_third),
}


def svg_doc(name, w, h, body):
    title = html.escape(f'{BRAND["name"]} {name}')
    return (f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}">'
            f'<title>{title}</title>{body}</svg>\n')


def main():
    p = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    p.add_argument("--render", action="store_true", help="render PNGs with rsvg-convert and the bundled fonts")
    p.add_argument("--outline", action="store_true", help="write *-outlined.svg logos with inkscape")
    a = p.parse_args()

    for g in GENERATED:
        target = ROOT / g
        if target.is_dir():
            shutil.rmtree(target)
        elif target.exists():
            target.unlink()

    with tempfile.TemporaryDirectory() as tmp:
        fc = Path(tmp) / "fonts.conf"
        fc.write_text(f'<?xml version="1.0"?><fontconfig><dir>{ROOT / "fonts"}</dir>'
                      f'<cachedir>{Path(tmp) / "cache"}</cachedir></fontconfig>')
        env = dict(os.environ, FONTCONFIG_FILE=str(fc))
        for name, (w, h, build) in LAYOUTS.items():
            path = ROOT / f"{name}.svg"
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(svg_doc(name, w, h, build(w, h)))
            ET.parse(path)
            if a.outline and name.startswith("logos/"):
                subprocess.run(["inkscape", str(path), "--export-text-to-path", "--export-plain-svg",
                                "--export-filename", str(path.with_name(path.stem + "-outlined.svg"))],
                               check=True, env=env, capture_output=True)
            if a.render:
                subprocess.run(["rsvg-convert", "-w", str(w), "-o", str(path.with_suffix(".png")), str(path)],
                               check=True, env=env)

    (ROOT / "tokens.json").write_text(json.dumps({
        "colors": C,
        "fonts": F,
        "spacing": [8, 16, 24, 32, 48, 64, 96],
        "contrast": {k: round(contrast(*v), 2) for k, v in TEXT_PAIRS.items()},
    }, indent=2, ensure_ascii=False) + "\n")
    (ROOT / "tokens.css").write_text(":root {\n" + "".join(f"  --prichal-{k.replace('_', '-')}: {v};\n" for k, v in C.items())
                                     + "".join(f"  --prichal-font-{k}: \"{v}\";\n" for k, v in F.items()) + "}\n")

    files = []
    for q in sorted(ROOT.rglob("*")):
        if q.is_file() and q.name != "asset-manifest.json" and "__pycache__" not in q.parts:
            files.append({"path": str(q.relative_to(ROOT)), "sha256": hashlib.sha256(q.read_bytes()).hexdigest(), "bytes": q.stat().st_size})
    (ROOT / "asset-manifest.json").write_text(json.dumps({"brand": BRAND["name"], "files": files}, indent=2) + "\n")
    print(f"{len(LAYOUTS)} layouts written to {ROOT}")


if __name__ == "__main__":
    main()
