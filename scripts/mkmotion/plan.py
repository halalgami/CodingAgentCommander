"""Pure decision logic for mkmotion. No subprocess, no filesystem writes,
so every function here is unit-testable with the real CLI tools absent."""
import os, re, struct

NAME_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")


def validate_name(name):
    """Output name becomes a path component; refuse anything that could
    escape --work-dir."""
    if not NAME_RE.fullmatch(name):
        raise ValueError(f"invalid name {name!r}: use letters, digits, _ or - "
                         "(1-64 chars, starting with a letter or digit)")
    return name


def png_size(path):
    """(w, h) from a PNG's IHDR, stdlib only."""
    with open(path, "rb") as f:
        head = f.read(24)
    if head[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError(f"not a PNG: {path}")
    return struct.unpack(">II", head[16:24])


def is_upscale(src_w, src_h, w, h):
    """The fit filter uses force_original_aspect_ratio=decrease, so the source
    is enlarged only when BOTH its dimensions are smaller than the canvas."""
    return src_w < w and src_h < h


def parse_canvas(s):
    try:
        w, h = (int(x) for x in s.lower().split("x"))
    except ValueError:
        raise ValueError(f"--canvas must be WxH, got {s!r}")
    if w < 1 or h < 1:
        raise ValueError(f"--canvas must be positive, got {s!r}")
    return w, h


def validate_options(quality, fps, max_frames, canvas):
    """Return (w, h); raise ValueError (one-line message) on bad options."""
    if not 0 <= quality <= 100:
        raise ValueError(f"--quality must be 0-100, got {quality}")
    if fps < 1:
        raise ValueError(f"--fps must be > 0, got {fps}")
    if max_frames < 1:
        raise ValueError(f"--max-frames must be >= 1, got {max_frames}")
    return parse_canvas(canvas)


VIDEO_EXTS = {".mp4", ".mov", ".webm"}


def detect_input_kind(path):
    """frames (a directory), video (.mp4/.mov/.webm), or gif (.gif)."""
    if os.path.isdir(path):
        return "frames"
    ext = os.path.splitext(path)[1].lower()
    if ext in VIDEO_EXTS:
        return "video"
    if ext == ".gif":
        return "gif"
    raise ValueError(f"unsupported input {path!r}: pass a frame directory, a video (.mp4/.mov/.webm), or a .gif")


def build_frame_order(frames, pingpong):
    """Forward, then the middle frames reversed, so the loop has no seam.
    Endpoints are not repeated (a,b,c,d -> a,b,c,d,c,b). No-op below 3 frames."""
    if not pingpong or len(frames) < 3:
        return list(frames)
    return list(frames) + list(reversed(frames[1:-1]))


def cap_frames(frames, max_frames):
    """Keep at most max_frames, sampled evenly, first frame always kept,
    source order preserved. Returns (kept, dropped_count)."""
    n = len(frames)
    if n <= max_frames:
        return list(frames), 0
    if max_frames <= 1:
        return frames[:1], n - 1
    idx = sorted({round(i * (n - 1) / (max_frames - 1)) for i in range(max_frames)})
    kept = [frames[i] for i in idx]
    return kept, n - len(kept)


def stage_outputs(work_dir, name):
    base = os.path.join(work_dir, name)
    return {
        "clip": os.path.join(base, "10_clip.mp4"),
        "frames_dir": os.path.join(base, "20_frames"),
        "matte_dir": os.path.join(base, "30_matte"),
        "webp": os.path.join(base, "out", name + ".webp"),
        "poster": os.path.join(base, "out", name + ".png"),
    }


def needs_run(output_path, force):
    """A stage runs when forced or when its output is absent."""
    return force or not os.path.exists(output_path)
