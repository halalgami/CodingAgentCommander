"""Pure argv builders for the external tools. Return lists (never shell
strings) so callers pass them straight to subprocess.run without a shell."""


def scale_pad_filter(w, h):
    """Fit inside WxH preserving aspect, then pad to exactly WxH with a
    TRANSPARENT background (0x00000000) so alpha survives the pad."""
    return (
        f"format=rgba,"
        f"scale={w}:{h}:force_original_aspect_ratio=decrease,"
        f"pad={w}:{h}:(ow-iw)/2:(oh-ih)/2:color=0x00000000"
    )


def ffmpeg_extract_cmd(src, out_pattern, fps):
    """Extract frames at fps into out_pattern (e.g. .../f_%04d.png). .webm needs
    the libvpx decoder or the native one drops the alpha channel."""
    pre = ["-c:v", "libvpx-vp9"] if src.lower().endswith(".webm") else []
    return ["ffmpeg", "-y"] + pre + ["-i", src, "-vf", f"fps={fps}", out_pattern]


def img2webp_cmd(frames, out, fps, quality, loop):
    """Encode frames into an animated webp. -d is per-frame delay in ms.
    -lossy is required: img2webp defaults to lossless, where -q is only
    effort and the file is ~20x larger. Alpha is kept."""
    delay = str(round(1000 / fps))
    cmd = ["img2webp", "-loop", str(loop), "-lossy", "-q", str(quality)]
    for f in frames:
        cmd += ["-d", delay, f]
    cmd += ["-o", out]
    return cmd
