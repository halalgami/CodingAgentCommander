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

Outputs land in `work/<name>/out/<name>.webp` and `.png`. Copy both into your
pack folder, then in `manifest.json` set the variant's `file` to the poster and
add `"motion": "<name>.webp"`.

## Flags
`--canvas WxH` (default 832x1216) · `--fps` (12) · `--max-frames` (24) ·
`--quality` (80) · `--pingpong/--no-pingpong` · `--poster/--no-poster` ·
`--matte` · `--force` · `--work-dir`

Keep motion short and subtle (breath, blink, hair). Big loops blow the pack
bitmap budget. GIF is only an *input* here — the app still never serves `.gif`.
