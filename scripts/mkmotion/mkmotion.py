#!/usr/bin/env python3
"""mkmotion — turn frames / video / gif into an animated WebP + poster PNG
for a Commander companion pack. See README.md. Phase 1 (no --ai)."""
import argparse, glob, os, shutil, sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import plan, commands, run as runner


def _list_frames(src):
    return sorted(glob.glob(os.path.join(glob.escape(src), "*.png")))


def _fit_frames(files, dst_dir, w, h):
    """Scale+pad every PNG in files into dst_dir at exactly WxH (alpha kept)."""
    os.makedirs(dst_dir, exist_ok=True)
    out = []
    for i, f in enumerate(files):
        o = os.path.join(dst_dir, f"f_{i:04d}.png")
        runner.run(["ffmpeg", "-y", "-i", f, "-vf",
                    commands.scale_pad_filter(w, h), o])
        out.append(o)
    return out


def _fail(msg, code=2):
    print(f"mkmotion: {msg}", file=sys.stderr)
    return code


def main(argv):
    ap = argparse.ArgumentParser(prog="mkmotion")
    ap.add_argument("input", help="frame directory, video (.mp4/.mov/.webm), or .gif")
    ap.add_argument("-o", "--name", required=True, help="output basename (usually the slot/variant)")
    ap.add_argument("--work-dir", default="work")
    ap.add_argument("--canvas", default="832x1216")
    ap.add_argument("--fps", type=int, default=12)
    ap.add_argument("--max-frames", type=int, default=24)
    ap.add_argument("--quality", type=int, default=80)
    ap.add_argument("--pingpong", dest="pingpong", action="store_true", default=True)
    ap.add_argument("--no-pingpong", dest="pingpong", action="store_false")
    ap.add_argument("--poster", dest="poster", action="store_true", default=True)
    ap.add_argument("--no-poster", dest="poster", action="store_false")
    ap.add_argument("--matte", action="store_true", help="remove background per frame (needs rembg)")
    ap.add_argument("--force", action="store_true")
    a = ap.parse_args(argv)

    try:
        w, h = plan.validate_options(a.quality, a.fps, a.max_frames, a.canvas)
        plan.validate_name(a.name)
        if not os.path.exists(a.input):
            raise ValueError(f"input not found: {a.input}")
        kind = plan.detect_input_kind(a.input)
        if kind == "frames" and not _list_frames(a.input):
            raise ValueError(f"no .png frames in {a.input} (frame directories are PNG-only)")
    except ValueError as e:
        return _fail(e)

    missing = runner.check_tools(["ffmpeg", "img2webp"])
    if missing:
        return _fail(f"missing required tools: {', '.join(missing)} — `brew install ffmpeg webp`")

    out = plan.stage_outputs(a.work_dir, a.name)
    raw_frames_dir = out["frames_dir"] + "_raw"

    if not plan.needs_run(out["webp"], a.force):
        print(f"{out['webp']} already exists; pass --force to regenerate")
        return 0

    try:
        # fresh run: stale stage frames from an earlier/failed run must not leak in
        for d in (raw_frames_dir, out["frames_dir"], out["matte_dir"]):
            shutil.rmtree(d, ignore_errors=True)
        for f in (out["webp"], out["poster"]):
            if os.path.exists(f):
                os.remove(f)

        # 1) get raw frames
        os.makedirs(raw_frames_dir, exist_ok=True)
        if kind == "frames":
            for i, f in enumerate(_list_frames(a.input)):
                shutil.copy(f, os.path.join(raw_frames_dir, f"f_{i:04d}.png"))
        else:  # video or gif
            runner.run(commands.ffmpeg_extract_cmd(
                a.input, os.path.join(raw_frames_dir, "f_%04d.png"), a.fps))
        raw = sorted(glob.glob(os.path.join(glob.escape(raw_frames_dir), "*.png")))
        if not raw:
            return _fail("no frames produced from input")

        try:
            sw, sh = plan.png_size(raw[0])
            if plan.is_upscale(sw, sh, w, h):
                print(f"note: input is {sw}x{sh}, upscaling to {w}x{h} — expect softness")
        except (ValueError, OSError):
            shutil.rmtree(d, ignore_errors=True)

        # 2) cap first so dropped frames are never matted or fitted
        capped_raw, dropped = plan.cap_frames(raw, a.max_frames)
        if dropped:
            print(f"note: dropped {dropped} frame(s) to stay under --max-frames {a.max_frames}")

        # 3) matte (optional)
        src_files = capped_raw
        if a.matte:
            import matte  # lazy: only needs rembg when --matte is used
            src_files = matte.matte_files(capped_raw, out["matte_dir"])

        # 4) canvas fit, then pingpong
        capped = _fit_frames(src_files, out["frames_dir"], w, h)
        order = plan.build_frame_order(capped, a.pingpong)

        # 5) encode + poster
        os.makedirs(os.path.dirname(out["webp"]), exist_ok=True)
        runner.run(commands.img2webp_cmd(order, out["webp"], a.fps, a.quality, 0))
        if a.poster:
            shutil.copy(capped[0], out["poster"])

        print(f"wrote {out['webp']}")
        if a.poster:
            print(f"wrote {out['poster']}")
        return 0
    except RuntimeError as e:
        return _fail(e, 1)
    except OSError as e:
        return _fail(e)


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
