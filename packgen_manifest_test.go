package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- fixtures --------------------------------------------------------------

// testPNG encodes a real PNG. The colour is a parameter because content ids are
// content hashes: two images with IDENTICAL bytes hash to the same id, and the
// loader drops the second as a duplicate. A fixture that writes the same bytes
// to several files therefore tests deduplication, not the pack — which is
// exactly the mistake that made an earlier version of this suite unable to
// pass. Every image here is byte-distinct unless a test says otherwise.
func testPNG(t *testing.T, w, h int, shade uint8, transparent bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	a := uint8(255)
	if transparent {
		a = 128
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: shade, G: uint8(x % 256), B: uint8(y % 256), A: a})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testJPEG(t *testing.T, w, h int, shade uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: shade, G: uint8(x % 256), B: uint8(y % 256), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pngHeaderClaiming builds a PNG signature plus a single IHDR chunk declaring
// arbitrary dimensions. The CRC must be CORRECT: Go verifies the chunk checksum
// before it reports the dimensions, so a zero CRC would be rejected as a
// corrupt file and the size guard under test would never run. (An earlier
// version of this test used a zero CRC and proved nothing.)
func pngHeaderClaiming(t *testing.T, w, h uint32) []byte {
	t.Helper()
	var body bytes.Buffer
	body.WriteString("IHDR")
	binary.Write(&body, binary.BigEndian, w)
	binary.Write(&body, binary.BigEndian, h)
	body.Write([]byte{8, 6, 0, 0, 0}) // 8-bit RGBA, no interlace

	var out bytes.Buffer
	out.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	binary.Write(&out, binary.BigEndian, uint32(body.Len()-4)) // length excludes the type
	out.Write(body.Bytes())
	binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(body.Bytes()))
	return out.Bytes()
}

func stagingDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	staging := filepath.Join(dir, "staging")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	return staging
}

// --- the acceptance test ---------------------------------------------------

// A pack this code writes must load with ZERO warnings. Warnings tell a human
// their manifest is wrong; firing them on art the user just paid for tells them
// their own purchase is broken. This is the requirement the writer exists to
// meet, so it is asserted against the real loader rather than against the
// writer's own output.
func TestGeneratedPackLoadsWithNoWarnings(t *testing.T) {
	dir := stagingDir(t)

	var arts []packGenArt
	shade := uint8(10)
	for _, slot := range packGenSlotOrder {
		arts = append(arts, packGenArt{
			Slot: slot, Image: testPNG(t, 24, 40, shade, false),
			Prompt: "prompt for " + slot, Model: "fal-ai/nano-banana-2/edit",
			Seed: 4242, GeneratedAt: time.Unix(1756512000, 0),
		})
		shade += 20
	}
	// Conditioned variants exercise the parts of the loader most likely to
	// warn: the condition vocabulary and the slot x condition matrix.
	arts = append(arts,
		packGenArt{Slot: "bored", When: "lateNight", Image: testPNG(t, 24, 40, 200, false)},
		packGenArt{Slot: "done", When: "century", Mood: "happy", Image: testPNG(t, 24, 40, 210, false)},
	)

	if _, err := writePackManifest(dir, "Acceptance Pack", arts); err != nil {
		t.Fatal(err)
	}

	p, ids := loadPack(dir)
	if len(p.Warnings) != 0 {
		t.Fatalf("generated pack produced %d warnings:\n  %s",
			len(p.Warnings), strings.Join(p.Warnings, "\n  "))
	}
	if len(ids) != len(arts) {
		t.Errorf("got %d served ids, want %d", len(ids), len(arts))
	}
	for _, slot := range packGenSlotOrder {
		if len(p.Slots[slot]) == 0 {
			t.Errorf("slot %q is empty after loading", slot)
		}
	}
	if len(p.Slots["bored"]) != 2 || len(p.Slots["done"]) != 2 {
		t.Errorf("conditioned variants lost: bored=%d done=%d",
			len(p.Slots["bored"]), len(p.Slots["done"]))
	}
	if p.Name != "Acceptance Pack" {
		t.Errorf("name = %q", p.Name)
	}
	// Every id must resolve to a file that actually exists inside the folder.
	// The loader returns symlink-resolved paths, and macOS puts temp dirs under
	// /var -> /private/var, so the comparison has to resolve too.
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	for id, path := range ids {
		if filepath.Dir(path) != realDir {
			t.Errorf("id %s resolves outside the pack: %s", id, path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("id %s: %v", id, err)
		}
	}
}

// The writer's own struct must not emit keys the loader calls unknown. This is
// asserted on the JSON rather than only through the warning count, so the
// failure names the offending key.
func TestManifestEmitsOnlyKeysTheLoaderKnows(t *testing.T) {
	dir := stagingDir(t)
	arts := []packGenArt{{Slot: "idle", Image: testPNG(t, 8, 8, 1, false)}}
	if _, err := writePackManifest(dir, "x", arts); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	for k := range top {
		if !packTopKeys[k] {
			t.Errorf("manifest emits top-level key %q, which the loader warns about", k)
		}
	}
	var doc struct {
		Slots map[string][]map[string]json.RawMessage `json:"slots"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for slot, vs := range doc.Slots {
		for _, v := range vs {
			for k := range v {
				if !packVariantKeys[k] {
					t.Errorf("slot %q emits variant key %q, which the loader warns about", slot, k)
				}
			}
			// rarity is the specific trap: it is the zero value AND the loader
			// warns that rarity 0 never displays, so it must be omitted.
			if _, present := v["rarity"]; present {
				t.Errorf("slot %q emits rarity, which warns when zero", slot)
			}
		}
	}
}

// --- content deduplication -------------------------------------------------

// Two different files with identical bytes hash to the same content id, and the
// loader drops the second. Writing one file and referencing it twice is the
// path the loader explicitly supports, and it is not hypothetical: a provider
// serving a cached response for two similar prompts would otherwise silently
// cost the user a slot they paid for.
func TestIdenticalImagesShareOneFileAndBothSurvive(t *testing.T) {
	dir := stagingDir(t)
	same := testPNG(t, 16, 16, 77, false)
	arts := []packGenArt{
		{Slot: "idle", Image: same},
		{Slot: "done", Image: same},
		{Slot: "working", Image: testPNG(t, 16, 16, 99, false)},
	}

	written, err := writePackManifest(dir, "dedup", arts)
	if err != nil {
		t.Fatal(err)
	}
	if written != 2 {
		t.Errorf("wrote %d files, want 2 (the duplicate should be shared)", written)
	}

	p, _ := loadPack(dir)
	if len(p.Warnings) != 0 {
		t.Fatalf("duplicate content warned:\n  %s", strings.Join(p.Warnings, "\n  "))
	}
	for _, slot := range []string{"idle", "done", "working"} {
		if len(p.Slots[slot]) != 1 {
			t.Errorf("slot %q has %d variants, want 1", slot, len(p.Slots[slot]))
		}
	}
	if p.Slots["idle"][0].ID != p.Slots["done"][0].ID {
		t.Error("shared art did not share an id")
	}
}

// --- facts read from the bytes ---------------------------------------------

func TestCanvasAndAlphaComeFromTheGeneratedArt(t *testing.T) {
	t.Run("opaque", func(t *testing.T) {
		dir := stagingDir(t)
		arts := []packGenArt{{Slot: "idle", Image: testPNG(t, 37, 61, 5, false)}}
		if _, err := writePackManifest(dir, "x", arts); err != nil {
			t.Fatal(err)
		}
		p, _ := loadPack(dir)
		if p.Canvas.W != 37 || p.Canvas.H != 61 {
			t.Errorf("canvas = %dx%d, want 37x61", p.Canvas.W, p.Canvas.H)
		}
		if p.Canvas.Anchor != "top-center" {
			t.Errorf("anchor = %q; the sidebar host anchors art at the top", p.Canvas.Anchor)
		}
		if p.Alpha {
			t.Error("alpha declared true for fully opaque art")
		}
	})

	t.Run("transparent", func(t *testing.T) {
		dir := stagingDir(t)
		arts := []packGenArt{{Slot: "idle", Image: testPNG(t, 20, 20, 5, true)}}
		if _, err := writePackManifest(dir, "x", arts); err != nil {
			t.Fatal(err)
		}
		p, _ := loadPack(dir)
		if !p.Alpha {
			t.Error("alpha declared false over art that has transparency")
		}
	})

	t.Run("one transparent image makes the pack transparent", func(t *testing.T) {
		dir := stagingDir(t)
		// Deliberately DIFFERENT sizes. With both at 20x20 the canvas half of
		// this test asserted nothing — first-wins and last-wins produced
		// identical output, and a mutation to "last image wins" passed.
		arts := []packGenArt{
			{Slot: "idle", Image: testPNG(t, 24, 40, 5, false)},
			{Slot: "done", Image: testPNG(t, 48, 20, 6, true)},
		}
		if _, err := writePackManifest(dir, "x", arts); err != nil {
			t.Fatal(err)
		}
		p, _ := loadPack(dir)
		if !p.Alpha {
			t.Error("alpha is a pack-wide claim; one transparent image must set it")
		}
		// The canvas comes from the FIRST image in order, not the last.
		if p.Canvas.W != 24 || p.Canvas.H != 40 {
			t.Errorf("canvas = %dx%d, want the first image's 24x40", p.Canvas.W, p.Canvas.H)
		}
	})
}

// The extension decides what the loader serves and what the webview renders, so
// it must come from the bytes and not from whatever the provider claimed.
func TestExtensionComesFromSniffingNotTheDeclaredMIME(t *testing.T) {
	dir := stagingDir(t)
	arts := []packGenArt{
		{Slot: "idle", Image: testJPEG(t, 16, 16, 50), MIME: "image/png"},
		{Slot: "done", Image: testPNG(t, 16, 16, 51, false), MIME: "image/jpeg"},
	}
	if _, err := writePackManifest(dir, "x", arts); err != nil {
		t.Fatal(err)
	}
	p, _ := loadPack(dir)
	if len(p.Warnings) != 0 {
		t.Fatalf("warnings:\n  %s", strings.Join(p.Warnings, "\n  "))
	}
	if got := p.Slots["idle"][0].File; !strings.HasSuffix(got, ".jpg") {
		t.Errorf("JPEG bytes written as %q despite a png Content-Type", got)
	}
	if got := p.Slots["done"][0].File; !strings.HasSuffix(got, ".png") {
		t.Errorf("PNG bytes written as %q despite a jpeg Content-Type", got)
	}
}

func TestPackGenImageFactsRejectsBadInput(t *testing.T) {
	cases := []struct {
		name, wantErr string
		raw           []byte
	}{
		{"empty", "not a decodable image", nil},
		{"not an image", "not a decodable image", []byte("this is a prose apology, not a PNG")},
		{"html error page", "not a decodable image", []byte("<html><body>402 Payment Required</body></html>")},
		{"absurd side", "implausible image dimensions", pngHeaderClaiming(t, 100000, 10)},
		{"pixel bomb", "too large", pngHeaderClaiming(t, 8000, 8000)},
		// Go's PNG decoder rejects this itself, before the writer's own
		// dimension guard is reached. The guard stays because it is not
		// PNG-specific, but the honest assertion here is that the bytes are
		// refused, not which layer refused them.
		{"zero size", "not a decodable image", pngHeaderClaiming(t, 0, 10)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := packGenImageFacts(tc.raw)
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// The bomb fixture must be rejected for its SIZE, not because Go found it
// corrupt — otherwise the guard under test never runs and the test passes for
// the wrong reason.
func TestPixelBombFixtureIsAValidPNGHeader(t *testing.T) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(pngHeaderClaiming(t, 8000, 8000)))
	if err != nil {
		t.Fatalf("fixture is not a decodable header, so the size guard is never reached: %v", err)
	}
	if format != "png" || cfg.Width != 8000 || cfg.Height != 8000 {
		t.Fatalf("fixture decoded as %s %dx%d", format, cfg.Width, cfg.Height)
	}
}

// --- meta ------------------------------------------------------------------

// PackVariant.Meta is map[string]string. A numeric seed written as a JSON
// number does not unmarshal into it, and the whole meta object is silently
// lost — provenance vanishes with no warning anywhere.
func TestProvenanceSurvivesTheRoundTrip(t *testing.T) {
	dir := stagingDir(t)
	when := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	arts := []packGenArt{{
		Slot: "idle", Image: testPNG(t, 8, 8, 3, false),
		Prompt: "a very specific prompt", Model: "fal-ai/nano-banana-2/edit",
		Seed: 987654321, GeneratedAt: when,
	}}
	if _, err := writePackManifest(dir, "x", arts); err != nil {
		t.Fatal(err)
	}

	p, _ := loadPack(dir)
	if len(p.Warnings) != 0 {
		t.Fatalf("warnings:\n  %s", strings.Join(p.Warnings, "\n  "))
	}
	meta := p.Slots["idle"][0].Meta
	if meta == nil {
		t.Fatal("meta was lost entirely — check that every value is a string")
	}
	for k, want := range map[string]string{
		"tool":           "Commander",
		"identityMethod": packGenIdentityMethod,
		"model":          "fal-ai/nano-banana-2/edit",
		"prompt":         "a very specific prompt",
		"seed":           "987654321",
		"generatedAt":    "2026-08-30T12:00:00Z",
	} {
		if meta[k] != want {
			t.Errorf("meta[%q] = %q, want %q", k, meta[k], want)
		}
	}

	// Absent provenance must not become empty strings that look like data.
	dir2 := stagingDir(t)
	if _, err := writePackManifest(dir2, "y", []packGenArt{{Slot: "idle", Image: testPNG(t, 8, 8, 4, false)}}); err != nil {
		t.Fatal(err)
	}
	p2, _ := loadPack(dir2)
	for _, k := range []string{"model", "prompt", "seed", "generatedAt"} {
		if _, present := p2.Slots["idle"][0].Meta[k]; present {
			t.Errorf("meta[%q] present when nothing was supplied", k)
		}
	}
}

// --- file naming and ordering ----------------------------------------------

func TestFileNamesAreReadableAndUnique(t *testing.T) {
	dir := stagingDir(t)
	arts := []packGenArt{
		{Slot: "idle", Image: testPNG(t, 8, 8, 1, false)},
		{Slot: "bored", When: "lateNight", Image: testPNG(t, 8, 8, 2, false)},
		{Slot: "done", When: "century", Mood: "happy", Image: testPNG(t, 8, 8, 3, false)},
	}
	if _, err := writePackManifest(dir, "x", arts); err != nil {
		t.Fatal(err)
	}
	p, _ := loadPack(dir)
	want := map[string]string{
		"idle":  "idle.png",
		"bored": "bored-latenight.png",
		"done":  "done-century-happy.png",
	}
	for slot, w := range want {
		if got := p.Slots[slot][0].File; got != w {
			t.Errorf("slot %q file = %q, want %q", slot, got, w)
		}
	}
}

// Two images that want the same file name but hold DIFFERENT bytes must not
// collapse into one file. validatePackGenSlots refuses this set upstream, but
// the writer is a separate function and silently overwriting here would destroy
// an image the user paid for.
func TestSameNameDifferentBytesKeepsBothImages(t *testing.T) {
	dir := stagingDir(t)
	arts := []packGenArt{
		{Slot: "idle", Image: testPNG(t, 8, 8, 1, false)},
		{Slot: "idle", Image: testPNG(t, 8, 8, 2, false)},
	}
	written, err := writePackManifest(dir, "x", arts)
	if err != nil {
		t.Fatal(err)
	}
	if written != 2 {
		t.Fatalf("wrote %d files, want 2 — one image was overwritten", written)
	}
	p, ids := loadPack(dir)
	if len(p.Warnings) != 0 {
		t.Fatalf("warnings:\n  %s", strings.Join(p.Warnings, "\n  "))
	}
	if len(ids) != 2 {
		t.Errorf("got %d served ids, want 2", len(ids))
	}
	if a, b := p.Slots["idle"][0].File, p.Slots["idle"][1].File; a == b {
		t.Errorf("both variants point at %q; one image was lost", a)
	}
}

// The manifest is the hand-editing surface for anyone extending their pack, so
// it must not be padded with empty fields that look like settings.
func TestManifestOmitsEmptyFields(t *testing.T) {
	dir := stagingDir(t)
	if _, err := writePackManifest(dir, "x", []packGenArt{
		{Slot: "idle", Image: testPNG(t, 8, 8, 1, false)},
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Slots map[string][]map[string]json.RawMessage `json:"slots"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for slot, vs := range doc.Slots {
		for _, v := range vs {
			for k, val := range v {
				if string(val) == `""` || string(val) == "0" || string(val) == "null" {
					t.Errorf("slot %q emits empty %q as %s", slot, k, val)
				}
			}
		}
	}
}

func TestPackGenSortArtsPutsIdleFirstAndBaseBeforeVariants(t *testing.T) {
	arts := []packGenArt{
		{Slot: "bored", When: "lateNight"},
		{Slot: "done"},
		{Slot: "idle"},
		{Slot: "bored"},
	}
	packGenSortArts(arts)
	got := make([]string, len(arts))
	for i, a := range arts {
		got[i] = a.Slot + "/" + a.When
	}
	want := []string{"idle/", "done/", "bored/", "bored/lateNight"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

// --- refusals --------------------------------------------------------------

func TestWritePackManifestRefusals(t *testing.T) {
	t.Run("no art", func(t *testing.T) {
		if _, err := writePackManifest(stagingDir(t), "x", nil); err == nil {
			t.Fatal("want an error")
		}
	})
	t.Run("missing staging folder", func(t *testing.T) {
		dir := filepath.Join(stagingDir(t), "nope")
		_, err := writePackManifest(dir, "x", []packGenArt{{Slot: "idle", Image: testPNG(t, 8, 8, 1, false)}})
		if err == nil || !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("unknown slot", func(t *testing.T) {
		_, err := writePackManifest(stagingDir(t), "x", []packGenArt{
			{Slot: "dancing", Image: testPNG(t, 8, 8, 1, false)},
		})
		if err == nil || !strings.Contains(err.Error(), "unknown slot") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("undecodable art names its slot", func(t *testing.T) {
		_, err := writePackManifest(stagingDir(t), "x", []packGenArt{
			{Slot: "idle", Image: testPNG(t, 8, 8, 1, false)},
			{Slot: "done", Image: []byte("not an image")},
		})
		if err == nil || !strings.Contains(err.Error(), `slot "done"`) {
			t.Fatalf("got %v", err)
		}
	})
	// A pack with no name still has to be identifiable in the picker.
	t.Run("blank name", func(t *testing.T) {
		dir := stagingDir(t)
		if _, err := writePackManifest(dir, "   ", []packGenArt{
			{Slot: "idle", Image: testPNG(t, 8, 8, 1, false)},
		}); err != nil {
			t.Fatal(err)
		}
		p, _ := loadPack(dir)
		if strings.TrimSpace(p.Name) == "" {
			t.Error("pack has no name")
		}
	})
}
