"""Per-frame background removal. rembg is imported lazily so the phase-1 core
never requires it — only `--matte` pulls it in."""
import glob, os


def _load_rembg():
    try:
        from rembg import remove  # noqa: WPS433 (deliberate lazy import)
        return remove
    except Exception as e:  # ImportError, or a broken partial install
        raise RuntimeError(
            "background removal needs rembg: `python3 -m pip install rembg` "
            f"(import failed: {e})"
        )


def matte_dir(src_dir, dst_dir):
    """Write an alpha-cut PNG per frame in src_dir into dst_dir."""
    return matte_files(sorted(glob.glob(os.path.join(glob.escape(src_dir), "*.png"))), dst_dir)


def matte_files(files, dst_dir):
    """Write an alpha-cut PNG for each file in files into dst_dir."""
    remove = _load_rembg()
    os.makedirs(dst_dir, exist_ok=True)
    outs = []
    for f in files:
        o = os.path.join(dst_dir, os.path.basename(f))
        with open(f, "rb") as fh:
            data = remove(fh.read())
        with open(o, "wb") as fh:
            fh.write(data)
        outs.append(o)
    return outs
