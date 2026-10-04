import io, os, sys, shutil, subprocess, tempfile, unittest
from contextlib import redirect_stderr, redirect_stdout
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
import mkmotion

HAVE = bool(shutil.which("ffmpeg") and shutil.which("img2webp"))
needs_tools = unittest.skipUnless(HAVE, "ffmpeg/img2webp not installed")


def _has_libvpx():
    if not shutil.which("ffmpeg"):
        return False
    out = subprocess.run(["ffmpeg", "-hide_banner", "-encoders"], capture_output=True, text=True).stdout
    return "libvpx-vp9" in out


def run_main(argv):
    err, out = io.StringIO(), io.StringIO()
    with redirect_stderr(err), redirect_stdout(out):
        rc = mkmotion.main(argv)
    return rc, out.getvalue(), err.getvalue()


def make_frames(d, n, size="4x4", alpha=None):
    os.makedirs(d, exist_ok=True)
    colors = ["red", "green", "blue", "yellow", "white", "black"]
    for i in range(n):
        c = colors[i % len(colors)]
        src = (f"color=c={c}@{alpha}:s={size}:d=1,format=rgba" if alpha
               else f"color=c={c}:s={size}:d=1")
        subprocess.run(["ffmpeg", "-y", "-f", "lavfi", "-i", src, "-frames:v", "1",
                        os.path.join(d, f"f_{i:04d}.png")], check=True, capture_output=True)
    return d


def webp_info(path):
    if not shutil.which("webpmux"):
        raise unittest.SkipTest("webpmux not installed")
    return subprocess.run(["webpmux", "-info", path], capture_output=True, text=True).stdout


def frame_rows(info):
    rows = []
    for line in info.splitlines():
        parts = line.split()
        if parts and parts[0].endswith(":") and parts[0][:-1].isdigit():
            rows.append(parts)
    return rows


def pixel00(png):
    raw = subprocess.run(["ffmpeg", "-v", "error", "-i", png, "-f", "rawvideo",
                          "-pix_fmt", "rgba", "-"], capture_output=True, check=True).stdout
    return tuple(raw[:4])


class Validation(unittest.TestCase):
    def check(self, argv, needle):
        rc, _, err = run_main(argv)
        self.assertEqual(rc, 2, err)
        self.assertTrue(err.startswith("mkmotion: "), err)
        self.assertIn(needle, err)
        self.assertNotIn("Traceback", err)

    def test_bad_options(self):
        with tempfile.TemporaryDirectory() as d:
            self.check([d, "-o", "x", "--quality", "150"], "--quality")
            self.check([d, "-o", "x", "--fps", "0"], "--fps")
            self.check([d, "-o", "x", "--max-frames", "0"], "--max-frames")
            self.check([d, "-o", "x", "--canvas", "0x5"], "--canvas")

    def test_bad_name_refused_and_nothing_written(self):
        for bad in ["../evil", "a\n"]:
            with tempfile.TemporaryDirectory() as d:
                work = os.path.join(d, "w")
                self.check([d, "-o", bad, "--work-dir", work], "invalid name")
                self.assertFalse(os.path.exists(work))
                self.assertFalse(os.path.exists(os.path.join(d, "evil")))

    def test_evil_name_leaves_nothing_beside_workdir(self):
        with tempfile.TemporaryDirectory() as d:
            work = os.path.join(d, "inner")
            self.check([d, "-o", "../evil", "--work-dir", work], "invalid name")
            self.assertFalse(os.path.exists(os.path.join(d, "evil")))
            self.assertFalse(os.path.exists(work))

    def test_missing_input(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "nope.mp4")
            self.check([p, "-o", "x", "--work-dir", d], f"input not found: {p}")

    def test_unsupported_input(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "empty.txt")
            open(p, "w").close()
            self.check([p, "-o", "x", "--work-dir", d], "unsupported input")

    def test_frame_dir_without_png(self):
        with tempfile.TemporaryDirectory() as d:
            open(os.path.join(d, "a.jpg"), "w").close()
            self.check([d, "-o", "x", "--work-dir", os.path.join(d, "w")], "PNG-only")


@needs_tools
class Runs(unittest.TestCase):
    def test_upscale_note_once_alpha_and_lossy(self):
        with tempfile.TemporaryDirectory() as d:
            frames = make_frames(os.path.join(d, "fr"), 3, "4x4", alpha=0.5)
            rc, out, err = run_main([frames, "-o", "up", "--work-dir", os.path.join(d, "w"),
                                     "--canvas", "8x8", "--fps", "6"])
            self.assertEqual(rc, 0, err)
            self.assertEqual((out + err).count("note: input is 4x4, upscaling to 8x8"), 1)
            info = webp_info(os.path.join(d, "w/up/out/up.webp"))
            rows = frame_rows(info)
            self.assertEqual(rows[0][3], "yes", info)  # alpha column of frame 1
            self.assertEqual(rows[0][-1], "lossy", info)

    def test_no_upscale_note_for_large_landscape(self):
        with tempfile.TemporaryDirectory() as d:
            frames = make_frames(os.path.join(d, "fr"), 2, "16x8")
            rc, out, err = run_main([frames, "-o", "ls", "--work-dir", os.path.join(d, "w"),
                                     "--canvas", "8x12"])
            self.assertEqual(rc, 0, err)
            self.assertNotIn("upscaling", out + err)

    def test_opaque_landscape_gets_transparent_pad(self):
        with tempfile.TemporaryDirectory() as d:
            frames = make_frames(os.path.join(d, "fr"), 2, "16x8")
            rc, _, err = run_main([frames, "-o", "ls", "--work-dir", os.path.join(d, "w"),
                                   "--canvas", "16x16"])
            self.assertEqual(rc, 0, err)
            self.assertEqual(pixel00(os.path.join(d, "w/ls/out/ls.png"))[3], 0)

    def test_force_rerun_with_fewer_frames_has_no_stale_frames(self):
        with tempfile.TemporaryDirectory() as d:
            work = os.path.join(d, "w")
            base = ["-o", "s", "--work-dir", work, "--canvas", "8x8", "--no-pingpong", "--force"]
            self.assertEqual(run_main([make_frames(os.path.join(d, "a"), 4)] + base)[0], 0)
            webp = os.path.join(work, "s/out/s.webp")
            self.assertEqual(len(frame_rows(webp_info(webp))), 4)
            self.assertEqual(run_main([make_frames(os.path.join(d, "b"), 2)] + base)[0], 0)
            self.assertEqual(len(frame_rows(webp_info(webp))), 2)

    def test_dir_with_bracket_in_path(self):
        with tempfile.TemporaryDirectory() as d:
            frames = make_frames(os.path.join(d, "fr[1]"), 2)
            rc, _, err = run_main([frames, "-o", "b", "--work-dir", os.path.join(d, "w"),
                                   "--canvas", "8x8"])
            self.assertEqual(rc, 0, err)

    def test_matte_runs_only_on_kept_frames(self):
        import matte
        calls = []
        orig = matte._load_rembg
        matte._load_rembg = lambda: (lambda data: calls.append(1) or data)
        try:
            with tempfile.TemporaryDirectory() as d:
                frames = make_frames(os.path.join(d, "fr"), 6)
                rc, _, err = run_main([frames, "-o", "m", "--work-dir", os.path.join(d, "w"),
                                       "--canvas", "8x8", "--max-frames", "3", "--matte"])
                self.assertEqual(rc, 0, err)
                self.assertEqual(len(calls), 3)
        finally:
            matte._load_rembg = orig

    def test_unwritable_work_dir_is_one_line(self):
        with tempfile.TemporaryDirectory() as d:
            frames = make_frames(os.path.join(d, "fr"), 2)
            blocker = os.path.join(d, "file")
            open(blocker, "w").close()
            rc, _, err = run_main([frames, "-o", "x", "--work-dir", os.path.join(blocker, "w")])
            self.assertEqual(rc, 2)
            self.assertTrue(err.startswith("mkmotion: "), err)
            self.assertNotIn("Traceback", err)

    @unittest.skipUnless(_has_libvpx(), "ffmpeg lacks libvpx")
    def test_webm_alpha_survives(self):
        with tempfile.TemporaryDirectory() as d:
            clip = os.path.join(d, "c.webm")
            subprocess.run(["ffmpeg", "-y", "-f", "lavfi", "-i",
                            "color=c=red@0.5:s=16x16:d=1:r=4,format=yuva420p",
                            "-c:v", "libvpx-vp9", "-pix_fmt", "yuva420p", clip],
                           check=True, capture_output=True)
            rc, _, err = run_main([clip, "-o", "w", "--work-dir", os.path.join(d, "w"),
                                   "--canvas", "16x16"])
            self.assertEqual(rc, 0, err)
            self.assertLess(pixel00(os.path.join(d, "w/w/out/w.png"))[3], 255)
