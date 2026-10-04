# mkmotion

Author-side tool: frames / video / gif -> animated WebP + poster PNG for a
Commander companion pack. Not part of the app build.

## Requires
- `ffmpeg` and `img2webp` (`brew install ffmpeg webp`)
- `--matte` additionally needs `rembg` (`python3 -m pip install rembg`)

## Use
    python3 mkmotion.py frames/            -o idle_a
    python3 mkmotion.py clip.mp4  --matte  -o work_a
    python3 mkmotion.py in.gif             -o bored_a

Output is lossy animated WebP (`img2webp -lossy`, alpha kept), ~20x smaller
than lossless. `-o` must match `[A-Za-z0-9][A-Za-z0-9_-]{0,63}` (it becomes a
path component). Input errors print one `mkmotion: ...` line and exit 2.
Outputs land in `work/<name>/out/<name>.webp` and `.png`. Copy both into your
pack folder, then in `manifest.json` set the variant's `file` to the poster and
add `"motion": "<name>.webp"`.

## Flags
`--canvas WxH` (default 832x1216) · `--fps` (12) · `--max-frames` (24) ·
`--quality` (80) · `--pingpong/--no-pingpong` · `--poster/--no-poster` ·
`--matte` · `--force` · `--work-dir`

## Notes
- Frame directories are **PNG-only** (`*.png`, sorted by name). Input is a
  directory, a video, or a `.gif` (no glob patterns).
- Changing any flag needs `--force`; without it an existing output is kept.
  Each forced run clears its stage dirs first, so no stale frames leak in.
- Outputs made before the lossy fix are lossless and huge (~25 MB for 24
  frames) — rerun them with `--force`.
- Size guidance: about 4 MB per 24-frame 832x1216 loop; consider
  `--max-frames 12`. With pingpong the loop has ~2x the frames you keep.
- Opaque input gets a transparent pad (not black bars); `.webm` is decoded
  with libvpx so its alpha survives.
- Option errors (`--quality` 0-100, `--fps` >0, `--max-frames` >=1,
  `--canvas` positive WxH) are rejected before any work.

Keep motion short and subtle (breath, blink, hair). Big loops blow the pack
bitmap budget. GIF is only an *input* here — the app still never serves `.gif`.
