# City art pack (Al Modon)

Generates the dashboard city sprites with fal.ai, then cuts, trims and ships them as webp.

    FAL_KEY_FILE=/path/to/fal.env PAR=6 python3 scripts/cityart/gen.py batch scripts/cityart/jobs.json
    python3 scripts/cityart/post.py build   # cut/*.png -> frontend/src/assets/city/*.webp (<= 512 px)
    python3 scripts/cityart/post.py sheet   # scripts/cityart/contact-sheet.png for review

`FAL_KEY_FILE` names a file holding the fal key (`FAL_KEY=...` or the bare key); the key is never printed.
`raw/`, `cut/` and the contact sheet are git-ignored; only the webp files under `frontend/src/assets/city/`
are committed, and only after the contact sheet is approved.
