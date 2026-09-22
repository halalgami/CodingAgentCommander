#!/usr/bin/env python3
"""mkmotion — turn frames / video / gif into an animated WebP + poster PNG
for a Commander companion pack. See README.md. Phase 1 (no --ai)."""
import argparse, glob, os, shutil, sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import plan, commands, run as runner


def _list_frames(src):
    if os.path.isdir(src):
        files = sorted(glob.glob(os.path.join(src, "*.png")))
    else:
        files = sorted(glob.glob(src))
    return files


def _fit_frames(src_dir, dst_dir, w, h):
    """Scale+pad every PNG in src_dir into dst_dir at exactly WxH (alpha kept)."""
    os.makedirs(dst_dir, exist_ok=True)
    out = []
    for i, f in enumerate(sorted(glob.glob(os.path.join(src_dir, "*.png")))):
        o = os.path.join(dst_dir, f"f_{i:04d}.png")
        runner.run(["ffmpeg", "-y", "-i", f, "-vf",
                    commands.scale_pad_filter(w, h), o])
        out.append(o)
    return out


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

    missing = runner.check_tools(["ffmpeg", "img2webp"])
    if missing:
        print(f"missing required tools: {', '.join(missing)} — `brew install ffmpeg webp`", file=sys.stderr)
        return 2

    try:
        w, h = (int(x) for x in a.canvas.lower().split("x"))
    except ValueError:
        print(f"--canvas must be WxH, got {a.canvas!r}", file=sys.stderr)
        return 2

    kind = plan.detect_input_kind(a.input)
    out = plan.stage_outputs(a.work_dir, a.name)
    raw_frames_dir = out["frames_dir"] + "_raw"

    if not plan.needs_run(out["webp"], a.force):
        print(f"{out['webp']} already exists; pass --force to regenerate")
        return 0

    try:
        # 1) get raw frames
        os.makedirs(raw_frames_dir, exist_ok=True)
        if kind == "frames":
            for i, f in enumerate(_list_frames(a.input)):
                shutil.copy(f, os.path.join(raw_frames_dir, f"f_{i:04d}.png"))
        else:  # video or gif
            runner.run(commands.ffmpeg_extract_cmd(
                a.input, os.path.join(raw_frames_dir, "f_%04d.png"), a.fps))
        raw = sorted(glob.glob(os.path.join(raw_frames_dir, "*.png")))
        if not raw:
            print("no frames produced from input", file=sys.stderr)
            return 1

        # 2) matte (optional)
        src_dir = raw_frames_dir
        if a.matte:
            import matte  # lazy: only needs rembg when --matte is used
            src_dir = out["matte_dir"]
            matte.matte_dir(raw_frames_dir, src_dir)

        # 3) canvas fit
        fitted = _fit_frames(src_dir, out["frames_dir"], w, h)

        # 4) cap + pingpong
        capped, dropped = plan.cap_frames(fitted, a.max_frames)
        if dropped:
            print(f"note: dropped {dropped} frame(s) to stay under --max-frames {a.max_frames}")
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
        print(e, file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
