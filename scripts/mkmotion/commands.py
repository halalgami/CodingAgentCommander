"""Pure argv builders for the external tools. Return lists (never shell
strings) so callers pass them straight to subprocess.run without a shell."""


def scale_pad_filter(w, h):
    """Fit inside WxH preserving aspect, then pad to exactly WxH with a
    TRANSPARENT background (0x00000000) so alpha survives the pad."""
    return (
        f"scale={w}:{h}:force_original_aspect_ratio=decrease,"
        f"pad={w}:{h}:(ow-iw)/2:(oh-ih)/2:color=0x00000000"
    )


def ffmpeg_extract_cmd(src, out_pattern, fps):
    """Extract frames at fps into out_pattern (e.g. .../f_%04d.png)."""
    return ["ffmpeg", "-y", "-i", src, "-vf", f"fps={fps}", out_pattern]


def img2webp_cmd(frames, out, fps, quality, loop):
    """Encode frames into an animated webp. -d is per-frame delay in ms."""
    delay = str(round(1000 / fps))
    cmd = ["img2webp", "-loop", str(loop), "-q", str(quality)]
    for f in frames:
        cmd += ["-d", delay, f]
    cmd += ["-o", out]
    return cmd
