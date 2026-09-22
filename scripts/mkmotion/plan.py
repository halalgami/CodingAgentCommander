"""Pure decision logic for mkmotion. No subprocess, no filesystem writes,
so every function here is unit-testable with the real CLI tools absent."""
import os

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
