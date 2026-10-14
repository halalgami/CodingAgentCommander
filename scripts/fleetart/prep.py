#!/usr/bin/env python3
"""Fleet art pack (dashboard phase 7a).

  prep.py jobs    # scripts/fleetart/jobs.json: fal jobs for damaged ships, neutral craft, backgrounds, cutouts
  prep.py jobs7c  # scripts/fleetart/jobs-7c.json: Sector Map station cutouts + galaxy backdrop (phase 7c)
  prep.py stage   # copy chosen samples that need no fal call into scripts/fleetart/cut/
  prep.py build   # cut/*.png -> frontend/src/assets/fleet/*.webp, quality stepped down until <= 3 MB
  prep.py sheet   # scripts/fleetart/contact-sheet.jpg
"""
import glob, json, os, shutil, sys
from PIL import Image

RAW, CUT, OUT = "scripts/fleetart/raw", "scripts/fleetart/cut", "frontend/src/assets/fleet"
SHIPS = {"opus": "hw-mother-A1", "sonnet": "hw-sonnet-5c", "haiku": "hw-haiku-4",
         "glm": "hw-glm-1", "qwen": "hw-qwen-1", "kimi": "hw-kimi-1"}
CAPTAINS = {"opus": "captain-opus", "sonnet": "captain-sonnet", "haiku": "captain-haiku",
            "glm": "captain-glm", "qwen": "captain-qwen", "kimi": "captain-kimi-3"}
CUTOUT_ENC = ["derelict"]                    # clean silhouette: background removed
# Kept with their dark sky and screen-blended in CSS: background removal ate the swarm's bugs and half the gate ring.
BLEND_ENC = ["storm", "anomaly", "swarm", "gate"]
SHIP_STYLE = (", rendered in cel-shaded painterly anime sci-fi style with clean ink lines, soft rim light, "
              "Homeworld-inspired design, three-quarter view, the nose points to the RIGHT and the engine is on the LEFT, "
              "small in frame with lots of empty space, plain dark space background, no text, no logos, no people")
NEUTRAL = "neutral light grey and white plating, no coloured trim, white engine glow"
CRAFT = {
    "scout": "a tiny recon scout drone, teardrop hull with a single large sensor eye at the front and two thin antenna whiskers",
    "pathfinder": "a small pathfinder corvette that projects a holographic star-map grid in front of its nose, flat hull with sensor fins",
    "tug": "a small utility tug craft with two mechanical grabber arms holding a crate, stubby armoured hull",
    "sentinel": "a small sentinel frigate with a big circular scanner lens on its nose casting a fan of light, swept wings",
    "wing": "a squadron of three small arrowhead fighters in tight echelon formation",
}
SPACE = {
    "deep": "deep space panorama, dense starfield with a faint blue galaxy band, very dark, calm",
    "nebula": "space panorama with a large soft violet and teal nebula and scattered bright stars",
    "planet": "space panorama with a huge ringed gas giant planet low on the horizon, dark sky, stars",
}
BG_STYLE = ", painterly anime sci-fi background art, cel-shaded, wide establishing shot, empty, no ships, no text, no people"
DAMAGE = ("Same ship, same hull shape, same colours, same pose and same framing. Add heavy battle damage: scorched and "
          "cracked armour plates, a few hull breaches venting sparks and smoke, one engine flickering. Still flying. "
          "Same cel-shaded anime style. No text.")
GALAXY = ("a vast spiral galaxy seen at a shallow angle, glowing golden core, blue and violet spiral arms with dust lanes, "
          "scattered star clusters, very dark edges and lots of empty dark space")


def jobs():
    j = []
    for id, src in SHIPS.items():
        j.append({"kind": "rmbg", "src": f"{RAW}/{src}.png", "out": f"{CUT}/ship-{id}.png"})
        j.append({"kind": "edit", "base": f"{RAW}/{src}.png", "out": f"{RAW}/ship-{id}-damaged.png",
                  "rmbg_to": f"{CUT}/ship-{id}-damaged.png", "style": "", "prompt": DAMAGE})
    for k, p in CRAFT.items():
        j.append({"kind": "t2i", "out": f"{RAW}/craft-{k}.png", "rmbg_to": f"{CUT}/craft-{k}.png", "aspect": "16:9",
                  "style": SHIP_STYLE, "prompt": f"{p}, {NEUTRAL}"})
    for k in CUTOUT_ENC:
        j.append({"kind": "rmbg", "src": f"{RAW}/hw-enc-{k}.png", "out": f"{CUT}/enc-{k}.png"})
    for k, p in SPACE.items():
        j.append({"kind": "t2i", "out": f"{CUT}/bg-{k}.png", "aspect": "16:9", "style": BG_STYLE, "prompt": p})
    json.dump(j, open("scripts/fleetart/jobs.json", "w"), indent=1)
    print(f"{len(j)} jobs -> scripts/fleetart/jobs.json")


def jobs7c():
    """Phase 7c only: station cutouts + galaxy backdrop (keeps 7a's jobs.json untouched)."""
    j = [{"kind": "rmbg", "src": f"{RAW}/hw-station-{n}.png", "out": f"{CUT}/station-{n}.png"} for n in (1, 2, 3)]
    j.append({"kind": "t2i", "out": f"{CUT}/bg-galaxy.png", "aspect": "16:9", "style": BG_STYLE, "prompt": GALAXY})
    json.dump(j, open("scripts/fleetart/jobs-7c.json", "w"), indent=1)
    print(f"{len(j)} jobs -> scripts/fleetart/jobs-7c.json")


def stage():
    os.makedirs(CUT, exist_ok=True)
    for id, src in CAPTAINS.items():
        shutil.copy(f"{RAW}/{src}.png", f"{CUT}/captain-{id}.png")
    for k in BLEND_ENC:
        shutil.copy(f"{RAW}/hw-enc-{k}.png", f"{CUT}/enc-{k}.png")
    shutil.copy(f"{RAW}/vortex-1.png", f"{CUT}/fx-vortex.png")  # 7c: Babylon 5 jump point (owner pick 1)
    print("staged captains and blended encounters")


def width_of(name):
    if name == "bg-galaxy":
        return 1600
    if name.startswith("bg-"):
        return 960
    if name.startswith("captain-"):
        return 160
    if name.startswith("fx-"):
        return 320
    if name.startswith("craft-"):
        return 200
    return 480


def build():
    srcs = sorted(glob.glob(f"{CUT}/*.png"))
    os.makedirs(OUT, exist_ok=True)
    for q in (72, 64, 56):
        for src in srcs:
            name = os.path.basename(src)[:-4]
            im = Image.open(src).convert("RGBA")
            bbox = im.getchannel("A").point(lambda a: 255 if a > 24 else 0).getbbox()  # ignore faint specks
            if not bbox:
                sys.exit(f"{src}: empty after cutout")
            im = im.crop(bbox)
            w = width_of(name)
            if im.width > w:
                im = im.resize((w, max(1, im.height * w // im.width)), Image.LANCZOS)
            if name.startswith(("bg-", "captain-", "fx-")) or name[4:] in BLEND_ENC:
                im = im.convert("RGB")
            im.save(f"{OUT}/{name}.webp", "WEBP", quality=q, method=6)
        total = sum(os.path.getsize(f) for f in glob.glob(f"{OUT}/*"))
        print(f"quality {q}: {len(srcs)} built, pack total {total // 1024} KB")
        if total <= 3 * 1024 * 1024:
            return
    sys.exit("fleet pack is over 3 MB even at quality 56")


def sheet():
    files = sorted(glob.glob(f"{OUT}/*.webp"))
    cell, cols = 240, 6
    rows = (len(files) + cols - 1) // cols
    out = Image.new("RGB", (cols * cell, rows * cell), (40, 42, 50))
    for i, f in enumerate(files):
        im = Image.open(f).convert("RGBA")
        im.thumbnail((cell - 8, cell - 8))
        out.paste(im, ((i % cols) * cell + 4, (i // cols) * cell + 4), im)
    out.save("scripts/fleetart/contact-sheet.jpg", quality=80)
    print(f"sheet: {len(files)} images")


if __name__ == "__main__":
    {"jobs": jobs, "jobs7c": jobs7c, "stage": stage, "build": build, "sheet": sheet}[sys.argv[1]]()
