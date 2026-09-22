import os, sys, shutil, tempfile, subprocess, unittest
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

HAVE = shutil.which("ffmpeg") and shutil.which("img2webp")


@unittest.skipUnless(HAVE, "ffmpeg/img2webp not installed")
class Smoke(unittest.TestCase):
    def _frame(self, path, color):
        # 4x4 solid PNG via ffmpeg's lavfi source, kept tiny on purpose
        subprocess.run(["ffmpeg", "-y", "-f", "lavfi", "-i",
                        f"color=c={color}:s=4x4:d=1", "-frames:v", "1", path],
                       check=True, capture_output=True)

    def test_frames_to_webp_and_poster(self):
        with tempfile.TemporaryDirectory() as d:
            frames = os.path.join(d, "frames"); os.makedirs(frames)
            for i, c in enumerate(["red", "green", "blue"]):
                self._frame(os.path.join(frames, f"f_{i:04d}.png"), c)
            work = os.path.join(d, "work")
            import mkmotion
            rc = mkmotion.main([frames, "-o", "idle_a", "--work-dir", work,
                                "--canvas", "8x8", "--fps", "6"])
            self.assertEqual(rc, 0)
            self.assertTrue(os.path.exists(os.path.join(work, "idle_a/out/idle_a.webp")))
            self.assertTrue(os.path.exists(os.path.join(work, "idle_a/out/idle_a.png")))
