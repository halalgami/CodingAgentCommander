import os, sys, importlib.util, tempfile, unittest
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
import matte

HAVE_REMBG = importlib.util.find_spec("rembg") is not None


class MatteDeps(unittest.TestCase):
    @unittest.skipIf(HAVE_REMBG, "rembg installed; exercising the missing-dep path only")
    def test_missing_rembg_raises_actionable(self):
        with tempfile.TemporaryDirectory() as d:
            with self.assertRaises(RuntimeError) as ctx:
                matte.matte_dir(d, os.path.join(d, "out"))
            self.assertIn("rembg", str(ctx.exception))


@unittest.skipUnless(HAVE_REMBG, "rembg not installed")
class MatteRun(unittest.TestCase):
    def test_produces_one_output_per_input(self):
        from PIL import Image
        with tempfile.TemporaryDirectory() as d:
            src = os.path.join(d, "src"); os.makedirs(src)
            for i in range(2):
                Image.new("RGB", (8, 8), (200, 30, 30)).save(os.path.join(src, f"f_{i:04d}.png"))
            outs = matte.matte_dir(src, os.path.join(d, "out"))
            self.assertEqual(len(outs), 2)
            self.assertTrue(all(os.path.exists(o) for o in outs))
