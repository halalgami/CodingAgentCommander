package raidpack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidStem(t *testing.T) {
	ok := []string{"opus-stance", "kimi-v2-hurt", "glm-legend-victory", "gear-familiar-qwen", "captain-kimi"}
	bad := []string{"", "opus", "Opus-stance", "opus-v3-hurt", "boss-hydra", "gear-banner", "../opus-stance", "opus-stance.png", "gear-familiar-claude", "captain-claude"}
	for _, s := range ok {
		if !ValidStem(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range bad {
		if ValidStem(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
	if n := len(AllStems()); n != 132 {
		t.Fatalf("AllStems = %d, want 132", n)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "opus-stance.png", "a")
	write(t, dir, "opus-attack.jpg", "b")
	write(t, dir, "opus-attack.png", "c") // duplicate stem: sorted first (.jpg) wins
	write(t, dir, "notes.txt", "x")       // unknown: warning
	write(t, dir, "README.txt", "x")      // silent
	write(t, dir, "heroine-prompts.csv", "x")
	write(t, dir, ".DS_Store", "x") // dot file: silent
	write(t, dir, "manifest.json", `{"schema":1,"name":"Mine","heroines":{"opus":{"name":"Layla","tint":"#c0392b"},"kimi":{"tint":"red"},"zed":{"name":"x"}}}`)
	if err := os.Symlink(filepath.Join(dir, "opus-stance.png"), filepath.Join(dir, "kimi-stance.png")); err != nil {
		t.Fatal(err)
	}
	p := Load(dir)
	if p.Name != "Mine" {
		t.Fatalf("name %q", p.Name)
	}
	if len(p.Files) != 2 || !strings.HasSuffix(p.Files["opus-attack"], "opus-attack.jpg") || p.Files["opus-stance"] == "" {
		t.Fatalf("files %v", p.Files)
	}
	if p.Heroines["opus"] != (Heroine{Name: "Layla", Tint: "#c0392b"}) {
		t.Fatalf("opus %+v", p.Heroines["opus"])
	}
	if _, ok := p.Heroines["kimi"]; ok {
		t.Fatal("bad tint must drop the entry's tint; empty entry dropped")
	}
	if _, ok := p.Heroines["zed"]; ok {
		t.Fatal("unknown heroine kept")
	}
	w := strings.Join(p.Warnings, "|")
	for _, want := range []string{"notes.txt", "opus-attack.png", "kimi-stance.png", "zed", "kimi"} {
		if !strings.Contains(w, want) {
			t.Errorf("warnings %q missing %q", w, want)
		}
	}
	if strings.Contains(w, "README") || strings.Contains(w, "DS_Store") || strings.Contains(w, "csv") {
		t.Errorf("silent files warned: %q", w)
	}
}

func TestLoadNoManifestAndFutureSchema(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "haiku-stance.webp", "a")
	p := Load(dir)
	if p.Name != filepath.Base(dir) || len(p.Files) != 1 {
		t.Fatalf("no-manifest pack: %+v", p)
	}
	write(t, dir, "manifest.json", `{"schema":2}`)
	p = Load(dir)
	if len(p.Files) != 0 || len(p.Warnings) == 0 {
		t.Fatalf("schema 2 must refuse the pack: %+v", p)
	}
	q := Load(filepath.Join(dir, "missing"))
	if len(q.Files) != 0 || len(q.Warnings) == 0 {
		t.Fatalf("missing dir: %+v", q)
	}
}

func TestManifestRoundTripAndLimits(t *testing.T) {
	dir := t.TempDir()
	long := strings.Repeat("n", 50)
	if err := WriteManifest(dir, Manifest{Schema: 1, Name: long, Heroines: map[string]Heroine{"glm": {Name: strings.Repeat("g", 30)}}}); err != nil {
		t.Fatal(err)
	}
	m, _ := ReadManifest(dir)
	if len([]rune(m.Name)) != 40 || len([]rune(m.Heroines["glm"].Name)) != 24 {
		t.Fatalf("limits not applied: %+v", m)
	}
}

// Minors sweep: a fixed "<path>.tmp" collided between concurrent writers and
// failed outright when that name was taken.
func TestWriteAtomicUsesAFreshTempFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "manifest.json")
	if err := os.Mkdir(p+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(p, []byte(`{}`)); err != nil {
		t.Fatalf("writeAtomic: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != `{}` {
		t.Fatalf("content = %q", b)
	}
}

func TestValidID(t *testing.T) {
	if !ValidID(IDs[0]) || ValidID("../x") || ValidID("") {
		t.Fatal("ValidID")
	}
}
