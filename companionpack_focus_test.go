package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// focusX/focusY decide where cover-fit crops, which decides whether the face is
// in frame. Both are float64, so an ABSENT key and an explicit 0 both decode to
// 0 — and 0 is meaningful here (the left edge, the top edge). Getting the
// default wrong pins every variant that omits focusX to the left edge.
func writeFocusPack(t *testing.T, variant string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	pack := filepath.Join(dir, "pack")
	if err := os.MkdirAll(pack, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pack, "idle.png"), testPNG(t, 8, 8, 1, false), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":1,"name":"f","canvas":{"w":8,"h":8,"scale":1,"anchor":"top-center"},
	  "alpha":false,"slots":{"idle":[` + variant + `]}}`
	if err := os.WriteFile(filepath.Join(pack, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return pack
}

func TestFocusDefaultsWhenAbsent(t *testing.T) {
	p, _ := loadPack(writeFocusPack(t, `{"file":"idle.png"}`))
	if len(p.Warnings) != 0 {
		t.Fatalf("warnings: %s", strings.Join(p.Warnings, "; "))
	}
	v := p.Slots["idle"][0]
	if v.FocusX != 0.5 {
		t.Errorf("absent focusX defaulted to %v, want 0.5 — 0 would pin the art to the left edge", v.FocusX)
	}
	if v.FocusY != 0 {
		t.Errorf("absent focusY defaulted to %v, want 0 (portraits are anchored at the top)", v.FocusY)
	}
}

// An explicit 0 is a real authoring choice and must survive.
func TestExplicitZeroFocusIsHonoured(t *testing.T) {
	p, _ := loadPack(writeFocusPack(t, `{"file":"idle.png","focusX":0,"focusY":0}`))
	if len(p.Warnings) != 0 {
		t.Fatalf("warnings: %s", strings.Join(p.Warnings, "; "))
	}
	if got := p.Slots["idle"][0].FocusX; got != 0 {
		t.Errorf("explicit focusX 0 became %v — absent and zero were conflated", got)
	}
}

func TestFocusIsCarriedAndClamped(t *testing.T) {
	p, _ := loadPack(writeFocusPack(t, `{"file":"idle.png","focusX":0.72,"focusY":0.15}`))
	if len(p.Warnings) != 0 {
		t.Fatalf("warnings: %s", strings.Join(p.Warnings, "; "))
	}
	v := p.Slots["idle"][0]
	if v.FocusX != 0.72 || v.FocusY != 0.15 {
		t.Errorf("focus = %v/%v, want 0.72/0.15", v.FocusX, v.FocusY)
	}

	out, _ := loadPack(writeFocusPack(t, `{"file":"idle.png","focusX":1.8,"focusY":-3}`))
	ov := out.Slots["idle"][0]
	if ov.FocusX != 1 || ov.FocusY != 0 {
		t.Errorf("out-of-range focus clamped to %v/%v, want 1/0", ov.FocusX, ov.FocusY)
	}
	if len(out.Warnings) != 2 {
		t.Errorf("want a warning per clamped field, got %d: %s", len(out.Warnings), strings.Join(out.Warnings, "; "))
	}
}

// A generated pack sets no focus at all, so it must keep rendering exactly as
// it did before this field was honoured.
func TestGeneratedPackGetsTheCentredDefault(t *testing.T) {
	dir := stagingDir(t)
	if _, err := writePackManifest(dir, "x", []packGenArt{
		{Slot: "idle", Image: testPNG(t, 24, 40, 5, false)},
	}); err != nil {
		t.Fatal(err)
	}
	p, _ := loadPack(dir)
	if len(p.Warnings) != 0 {
		t.Fatalf("warnings: %s", strings.Join(p.Warnings, "; "))
	}
	if v := p.Slots["idle"][0]; v.FocusX != 0.5 || v.FocusY != 0 {
		t.Errorf("generated variant focus = %v/%v, want 0.5/0", v.FocusX, v.FocusY)
	}
}
