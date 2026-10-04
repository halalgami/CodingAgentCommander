import os, sys, tempfile, unittest
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
from plan import detect_input_kind, build_frame_order, cap_frames, stage_outputs, needs_run


class DetectInputKind(unittest.TestCase):
    def test_directory_is_frames(self):
        with tempfile.TemporaryDirectory() as d:
            self.assertEqual(detect_input_kind(d), "frames")

    def test_mp4_is_video(self):
        self.assertEqual(detect_input_kind("clip.mp4"), "video")
        self.assertEqual(detect_input_kind("CLIP.MOV"), "video")
        self.assertEqual(detect_input_kind("x.webm"), "video")

    def test_gif_is_gif(self):
        self.assertEqual(detect_input_kind("in.gif"), "gif")

    def test_unknown_raises(self):
        with self.assertRaises(ValueError):
            detect_input_kind("notes.txt")


class FrameOrder(unittest.TestCase):
    def test_pingpong_appends_reversed_middle(self):
        # forward then back, without repeating the two endpoints
        self.assertEqual(build_frame_order(["a", "b", "c", "d"], True),
                         ["a", "b", "c", "d", "c", "b"])

    def test_pingpong_off_is_identity(self):
        self.assertEqual(build_frame_order(["a", "b", "c"], False), ["a", "b", "c"])

    def test_pingpong_noop_under_three_frames(self):
        self.assertEqual(build_frame_order(["a", "b"], True), ["a", "b"])

    def test_cap_keeps_all_when_under_limit(self):
        self.assertEqual(cap_frames(["a", "b", "c"], 24), (["a", "b", "c"], 0))

    def test_cap_evenly_samples_and_reports_dropped(self):
        frames = [str(i) for i in range(10)]
        kept, dropped = cap_frames(frames, 5)
        self.assertEqual(len(kept), 5)
        self.assertEqual(dropped, 5)
        self.assertEqual(kept[0], "0")            # first always kept
        self.assertTrue(all(f in frames for f in kept))
        self.assertEqual(kept, sorted(kept, key=lambda x: int(x)))  # order preserved

    def test_cap_frames_one_keeps_first(self):
        self.assertEqual(cap_frames(["a", "b", "c"], 1), (["a"], 2))


class Stages(unittest.TestCase):
    def test_stage_outputs_layout(self):
        out = stage_outputs("/w", "idle_a")
        self.assertEqual(out["clip"], "/w/idle_a/10_clip.mp4")
        self.assertEqual(out["frames_dir"], "/w/idle_a/20_frames")
        self.assertEqual(out["matte_dir"], "/w/idle_a/30_matte")
        self.assertEqual(out["webp"], "/w/idle_a/out/idle_a.webp")
        self.assertEqual(out["poster"], "/w/idle_a/out/idle_a.png")

    def test_needs_run_true_when_missing(self):
        self.assertTrue(needs_run("/does/not/exist.webp", force=False))

    def test_needs_run_false_when_present_and_not_forced(self):
        with tempfile.NamedTemporaryFile(suffix=".webp") as tf:
            self.assertFalse(needs_run(tf.name, force=False))

    def test_force_always_runs(self):
        with tempfile.NamedTemporaryFile(suffix=".webp") as tf:
            self.assertTrue(needs_run(tf.name, force=True))


if __name__ == "__main__":
    unittest.main()


class NameAndUpscale(unittest.TestCase):
    def test_validate_name(self):
        from plan import validate_name
        self.assertEqual(validate_name("idle_a-1"), "idle_a-1")
        for bad in ["../evil", "a/b", "", ".", "..", "-x", "a" * 65, "a\n"]:
            with self.assertRaises(ValueError, msg=bad):
                validate_name(bad)

    def test_is_upscale(self):
        from plan import is_upscale
        self.assertTrue(is_upscale(100, 200, 832, 1216))
        self.assertFalse(is_upscale(1920, 1080, 832, 1216))  # landscape mp4
        self.assertFalse(is_upscale(100, 2000, 832, 1216))
        self.assertFalse(is_upscale(832, 1216, 832, 1216))
        self.assertFalse(is_upscale(2000, 3000, 832, 1216))

    def test_validate_options(self):
        from plan import validate_options
        self.assertEqual(validate_options(80, 12, 24, "832x1216"), (832, 1216))
        for args in [(150, 12, 24, "8x8"), (-1, 12, 24, "8x8"), (80, 0, 24, "8x8"),
                     (80, 12, 0, "8x8"), (80, 12, 24, "8"), (80, 12, 24, "0x8"),
                     (80, 12, 24, "axb")]:
            with self.assertRaises(ValueError, msg=str(args)):
                validate_options(*args)
