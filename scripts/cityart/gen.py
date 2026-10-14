#!/usr/bin/env python3
"""fal.ai client for the Al Modon city art pack (dashboard phase 5a).

  gen.py t2i  <out.png> "<prompt>" [aspect]          text-to-image (nano-banana-2)
  gen.py edit <out.png> <base.png> "<prompt>"        image edit (nano-banana-2/edit)
  gen.py rmbg <out.png> <in.png>                     background removal (birefnet)
  gen.py batch <jobs.json>                           run many jobs in parallel

The key is read from the file named by FAL_KEY_FILE and never printed.
Every prompt gets the shared city STYLE suffix (a job may set "style" to replace it); .webp bases are converted to PNG first.
"""
import base64, json, sys, urllib.request, concurrent.futures as cf, os, time

KEYFILE = os.environ.get("FAL_KEY_FILE")
if not KEYFILE:
    sys.exit("set FAL_KEY_FILE to the file holding your fal key")
_raw = open(KEYFILE).read().strip()
KEY = _raw.split("=", 1)[1].strip().strip('"') if "=" in _raw.split(":")[0] else _raw

STYLE = (", isometric miniature diorama, tilt-shift toy model look, soft warm late-afternoon light, "
         "consistent 45-degree isometric camera from the front-left, centered, isolated on a plain light grey "
         "seamless background, no ground plate, no text, no logos")

T2I = "fal-ai/nano-banana-2"
EDIT = "fal-ai/nano-banana-2/edit"
RMBG = "fal-ai/birefnet/v2"


def call(model, body, timeout=300):
    req = urllib.request.Request(
        "https://fal.run/" + model,
        data=json.dumps(body).encode(),
        headers={"Authorization": "Key " + KEY, "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return json.load(r)
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"{model} HTTP {e.code}: {e.read()[:400].decode(errors='replace')}")


def save(url, out):
    os.makedirs(os.path.dirname(out) or ".", exist_ok=True)
    if url.startswith("data:"):
        open(out, "wb").write(base64.b64decode(url.split(",", 1)[1]))
    else:
        urllib.request.urlretrieve(url, out)


def datauri(path):
    if path.endswith(".webp"):
        from PIL import Image
        import io
        buf = io.BytesIO()
        Image.open(path).convert("RGBA").save(buf, "PNG")
        return "data:image/png;base64," + base64.b64encode(buf.getvalue()).decode()
    return "data:image/png;base64," + base64.b64encode(open(path, "rb").read()).decode()


def t2i(out, prompt, aspect="1:1"):
    r = call(T2I, {"prompt": prompt, "num_images": 1, "aspect_ratio": aspect,
                   "output_format": "png", "safety_tolerance": "4"})
    save(r["images"][0]["url"], out)


def edit(out, base, prompt, aspect=None):
    body = {"prompt": prompt, "image_urls": [datauri(base)], "num_images": 1,
            "output_format": "png", "safety_tolerance": "4"}
    if aspect:
        body["aspect_ratio"] = aspect
    r = call(EDIT, body)
    save(r["images"][0]["url"], out)


def rmbg(out, src):
    r = call(RMBG, {"image_url": datauri(src), "model": "General Use (Heavy)",
                    "operating_resolution": "1024x1024", "output_format": "png"})
    save(r["image"]["url"], out)


def run(job):
    t = time.time()
    kind = job["kind"]
    try:
        if kind == "t2i":
            t2i(job["out"], job["prompt"] + job.get("style", STYLE), job.get("aspect", "1:1"))
        elif kind == "edit":
            edit(job["out"], job["base"], job["prompt"] + job.get("style", STYLE), job.get("aspect"))
        elif kind == "rmbg":
            rmbg(job["out"], job["src"])
        if job.get("rmbg_to"):
            rmbg(job["rmbg_to"], job["out"])
        return f"ok   {job['out']} ({time.time()-t:.0f}s)"
    except Exception as e:
        return f"FAIL {job['out']}: {e}"


if __name__ == "__main__":
    cmd = sys.argv[1]
    if cmd == "batch":
        jobs = json.load(open(sys.argv[2]))
        with cf.ThreadPoolExecutor(int(os.environ.get("PAR", "6"))) as ex:
            for line in ex.map(run, jobs):
                print(line, flush=True)
    elif cmd == "t2i":
        print(run({"kind": "t2i", "out": sys.argv[2], "prompt": sys.argv[3],
                   "aspect": sys.argv[4] if len(sys.argv) > 4 else "1:1"}))
    elif cmd == "edit":
        print(run({"kind": "edit", "out": sys.argv[2], "base": sys.argv[3], "prompt": sys.argv[4]}))
    elif cmd == "rmbg":
        print(run({"kind": "rmbg", "out": sys.argv[2], "src": sys.argv[3]}))
