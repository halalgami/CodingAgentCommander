#!/usr/bin/env python3
"""Raid art pack (dashboard phase 6a).

  prep.py concepts   # concept raid set -> frontend/src/assets/raid/*.webp, downscaled
  prep.py build      # scripts/raidart/cut/*.png -> frontend/src/assets/raid/*.webp (trimmed)
  prep.py sheet      # scripts/raidart/contact-sheet.png of every raid image

  prep.py looksjobs  # scripts/raidart/looks.json: 6b looks, legends and gear (phase 6b)

Figures fit 420 px tall, arenas 960 px wide. The whole pack must stay within
4 MB (phase 6b); concepts step down the webp quality until the concept set fits
1.2 MB, and build steps down its own quality until the pack fits.
"""
import glob, json, os, sys
from PIL import Image

SRC = "docs/superpowers/concepts/2026-10-09-dashboard/assets/raid"
CUT, OUT = "scripts/raidart/cut", "frontend/src/assets/raid"
FIG_H, ARENA_W, SMALL_H = 420, 960, 300


def fit(im, w, h):
    im = im.copy()
    im.thumbnail((w, h), Image.LANCZOS)
    return im


def concepts():
    os.makedirs(OUT, exist_ok=True)
    files = sorted(glob.glob(f"{SRC}/*.webp"))
    for q in (80, 72, 64, 56):
        total = 0
        for src in files:
            name = os.path.basename(src)
            im = Image.open(src)
            if name.startswith("bg-"):
                im = fit(im.convert("RGB"), ARENA_W, ARENA_W)
            else:
                im = fit(im.convert("RGBA"), 10000, FIG_H)
            im.save(f"{OUT}/{name}", "WEBP", quality=q, method=6)
            total += os.path.getsize(f"{OUT}/{name}")
        print(f"quality {q}: {len(files)} files, {total // 1024} KB")
        if total <= 1200 * 1024:
            return
    sys.exit("concept set does not fit 1.2 MB even at quality 56")


def build():
    srcs = sorted(glob.glob(f"{CUT}/*.png"))
    for q in (72, 64, 56):
        for src in srcs:
            im = Image.open(src).convert("RGBA")
            bbox = im.getchannel("A").getbbox()
            if not bbox:
                sys.exit(f"{src}: empty after cutout")
            name = os.path.basename(src)[:-4]
            small = name.startswith(("ally-", "slime-", "inn", "chest", "gear-"))
            im = fit(im.crop(bbox), 10000, SMALL_H if small else FIG_H)
            im.save(f"{OUT}/{name}.webp", "WEBP", quality=q, method=6)
        total = sum(os.path.getsize(f) for f in glob.glob(f"{OUT}/*"))
        print(f"quality {q}: {len(srcs)} built, pack total {total // 1024} KB")
        if total <= 4 * 1024 * 1024:
            return
    sys.exit("raid pack is over 4 MB even at quality 56")


IDS = ["opus", "sonnet", "haiku", "glm", "kimi", "qwen"]
POSES = ["attack", "await", "hurt", "stance", "victory"]
# (hair, eyes, trim, accessory) per look; the outfit's main signature colour stays.
LOOKS = {
    "opus": [("deep auburn", "emerald green", "silver", "a crescent-moon hair ornament"),
             ("platinum blonde in a long braid", "violet", "rose gold", "a feathered circlet")],
    "sonnet": [("platinum blonde", "sapphire blue", "gold filigree", "a small red plume on a hair clip"),
               ("jet black", "amber", "silver", "a jewelled ear cuff")],
    "haiku": [("silver-white", "golden", "black lacquer", "a fox-mask hair clip"),
              ("dark crimson", "jade", "bronze", "a long headband with trailing tails")],
    "glm": [("platinum blonde", "teal", "brass", "a small feather on a hair clip"),
            ("black with violet streaks", "gold", "silver", "tinted round glasses pushed up on her head")],
    "qwen": [("silver-white", "crimson", "gold", "a kanzashi flower hairpin"),
             ("chestnut brown", "violet", "red cord", "small bells on a hair ribbon")],
    "kimi": [("pink", "cyan", "orange hazard-stripe", "a headset with a small antenna"),
             ("dark navy", "amber", "chrome", "a star decal on her goggles")],
}
KEEP = ("same character, same adult woman, same face shape, same pose, same weapon, same outfit design with the same "
        "neckline and full coverage, same huge exaggerated anime-style bust and voluptuous hourglass build as the "
        "reference, fully clothed, tasteful fantasy game art; same painterly anime style, full body, plain light grey "
        "background, no text")
LEGEND = ("her outfit upgraded to a legendary version of the same design: richer embroidery, gold filigree, glowing "
          "runes in her signature colour and a flowing cape; no added skin, nothing removed")
PROPS = ", painterly anime fantasy game art style, single object centered on a plain light grey background, no people, no characters, no text, no logos"
GEAR = {
    "gear-familiar-opus": "a tiny amber fire-spirit owl familiar floating, glowing softly",
    "gear-familiar-sonnet": "a tiny coral phoenix chick familiar floating, glowing softly",
    "gear-familiar-haiku": "a tiny green spirit wisp shaped like a fox kit, floating, glowing softly",
    "gear-familiar-glm": "a tiny violet mechanical drone familiar with a single lens, floating",
    "gear-familiar-qwen": "a tiny magenta glowing butterfly spirit familiar, floating",
    "gear-familiar-kimi": "a tiny teal round robot orb familiar with antenna, floating",
    "gear-banner": "a small heraldic war banner on a pole with a gold star emblem on deep blue cloth",
    "gear-weapon": "an ornate glowing fantasy sword floating point-down, vertical",
    "gear-halo": "a glowing golden halo ring of light, seen slightly from the front",
    "gear-wings": "a pair of large luminous feathered spirit wings spread wide, symmetrical, no body",
}


def looksjobs():
    jobs = []
    for id in IDS:
        for p in POSES:
            base = f"scripts/raidart/raw/{id}-{p}.png"
            for n, (hair, eyes, trim, acc) in enumerate(LOOKS[id], 1):
                jobs.append({"kind": "edit", "base": base, "out": f"scripts/raidart/raw/{id}-v{n}-{p}.png",
                             "rmbg_to": f"{CUT}/{id}-v{n}-{p}.png", "style": "",
                             "prompt": f"{KEEP}; recolour only: hair {hair}, eyes {eyes}, outfit trim {trim}, and add {acc}"})
            jobs.append({"kind": "edit", "base": base, "out": f"scripts/raidart/raw/{id}-legend-{p}.png",
                         "rmbg_to": f"{CUT}/{id}-legend-{p}.png", "style": "", "prompt": f"{KEEP}; {LEGEND}"})
    for name, prompt in GEAR.items():
        jobs.append({"kind": "t2i", "out": f"scripts/raidart/raw/{name}.png", "rmbg_to": f"{CUT}/{name}.png",
                     "style": PROPS, "prompt": prompt})
    json.dump(jobs, open("scripts/raidart/looks.json", "w"), indent=1)
    print(f"{len(jobs)} jobs -> scripts/raidart/looks.json")


def sheet():
    files = sorted(glob.glob(f"{OUT}/*.webp"))
    cell, cols = 200, 8
    rows = (len(files) + cols - 1) // cols
    out = Image.new("RGBA", (cols * cell, rows * cell), (40, 42, 50, 255))
    for i, f in enumerate(files):
        im = fit(Image.open(f).convert("RGBA"), cell - 20, cell - 20)
        out.alpha_composite(im, ((i % cols) * cell + (cell - im.width) // 2, (i // cols) * cell + (cell - im.height) // 2))
    out.save("scripts/raidart/contact-sheet.png")
    print(f"sheet: scripts/raidart/contact-sheet.png ({len(files)} images)")


if __name__ == "__main__":
    {"concepts": concepts, "build": build, "sheet": sheet, "looksjobs": looksjobs}[sys.argv[1]]()
