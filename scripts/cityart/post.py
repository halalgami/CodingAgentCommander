#!/usr/bin/env python3
"""Cutouts → shipped sprites: trim, fit within 512 px, transparent webp, checks,
and a contact sheet for the owner.

  post.py build   # scripts/cityart/cut/*.png → frontend/src/assets/city/*.webp
  post.py sheet   # scripts/cityart/contact-sheet.png of every city sprite
"""
import glob, os, sys
from PIL import Image

CUT, OUT = "scripts/cityart/cut", "frontend/src/assets/city"


def build():
    os.makedirs(OUT, exist_ok=True)
    for src in sorted(glob.glob(f"{CUT}/*.png")):
        im = Image.open(src).convert("RGBA")
        bbox = im.getchannel("A").getbbox()
        if not bbox:
            sys.exit(f"{src}: empty after cutout")
        im = im.crop(bbox)
        im.thumbnail((512, 512))
        corners = [im.getpixel(p)[3] for p in [(0, 0), (im.width - 1, 0), (0, im.height - 1), (im.width - 1, im.height - 1)]]
        if max(corners) > 16:
            print(f"WARN {src}: corners not transparent {corners}")
        dst = f"{OUT}/{os.path.basename(src)[:-4]}.webp"
        im.save(dst, "WEBP", quality=82, method=6)
        kb = os.path.getsize(dst) // 1024
        print(f"ok {dst} {im.width}x{im.height} {kb} KB")
        if kb > 120:
            print(f"WARN {dst}: {kb} KB")


def sheet():
    files = sorted(glob.glob(f"{OUT}/*.webp"))
    cell = 200
    cols = 8
    rows = (len(files) + cols - 1) // cols
    out = Image.new("RGBA", (cols * cell, rows * cell), (230, 232, 225, 255))
    for i, f in enumerate(files):
        im = Image.open(f).convert("RGBA")
        im.thumbnail((cell - 20, cell - 20))
        out.alpha_composite(im, ((i % cols) * cell + (cell - im.width) // 2, (i // cols) * cell + (cell - im.height) // 2))
    out.save("scripts/cityart/contact-sheet.png")
    print(f"sheet: scripts/cityart/contact-sheet.png ({len(files)} sprites)")


if __name__ == "__main__":
    {"build": build, "sheet": sheet}[sys.argv[1]]()
