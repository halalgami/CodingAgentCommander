import os, sys, unittest
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from commands import scale_pad_filter, ffmpeg_extract_cmd, img2webp_cmd


class Commands(unittest.TestCase):
    def test_scale_pad_preserves_alpha_and_centers(self):
        f = scale_pad_filter(832, 1216)
        self.assertIn("scale=832:1216:force_original_aspect_ratio=decrease", f)
        self.assertIn("pad=832:1216", f)
        self.assertIn("0x00000000", f)  # transparent pad, not black

    def test_extract_sets_fps_and_pattern(self):
        cmd = ffmpeg_extract_cmd("clip.mp4", "20_frames/f_%04d.png", 12)
        self.assertEqual(cmd[0], "ffmpeg")
        self.assertIn("clip.mp4", cmd)
        self.assertIn("fps=12", " ".join(cmd))
        self.assertIn("20_frames/f_%04d.png", cmd)

    def test_img2webp_infinite_loop_and_delay(self):
        cmd = img2webp_cmd(["a.png", "b.png"], "out.webp", fps=10, quality=80, loop=0)
        self.assertEqual(cmd[0], "img2webp")
        self.assertIn("-loop", cmd); self.assertIn("0", cmd)
        self.assertIn("-q", cmd); self.assertIn("80", cmd)
        self.assertIn("-d", cmd); self.assertIn("100", cmd)  # 1000/10 ms
        self.assertEqual(cmd[-2:], ["-o", "out.webp"])
        self.assertIn("a.png", cmd); self.assertIn("b.png", cmd)


class Lossy(unittest.TestCase):
    def test_lossy_precedes_first_frame(self):
        cmd = img2webp_cmd(["a.png", "b.png"], "out.webp", fps=10, quality=80, loop=0)
        self.assertIn("-lossy", cmd)
        self.assertLess(cmd.index("-lossy"), cmd.index("a.png"))
        self.assertLess(cmd.index("-lossy"), cmd.index("-d"))


class FilterAndWebm(unittest.TestCase):
    def test_filter_converts_to_rgba_before_pad(self):
        self.assertTrue(scale_pad_filter(8, 8).startswith("format=rgba,"))

    def test_webm_uses_libvpx_decoder_before_input(self):
        cmd = ffmpeg_extract_cmd("a.WEBM", "o_%04d.png", 12)
        self.assertLess(cmd.index("libvpx-vp9"), cmd.index("-i"))
        self.assertNotIn("libvpx-vp9", ffmpeg_extract_cmd("a.mp4", "o_%04d.png", 12))
