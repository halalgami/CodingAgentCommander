package raidpack

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func makeZip(t *testing.T, files map[string][]byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "My Pack.zip")
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	for n, b := range files {
		w, _ := zw.Create(n)
		w.Write(b)
	}
	zw.Close()
	f.Close()
	return p
}

func TestImportZip(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "my-pack"), 0o755) // collision → my-pack-2
	z := makeZip(t, map[string][]byte{
		"opus-stance.png":     pngBytes(t, 50, 100),
		"opus-attack.jpg":     []byte("broken"),
		"manifest.json":       []byte(`{"schema":1,"name":"Zipped"}`),
		"sub/kimi-stance.png": pngBytes(t, 5, 5),
		"evil.sh":             []byte("x"),
	})
	dir, warn, err := ImportZip(root, z)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dir) != "my-pack-2" {
		t.Fatalf("dir %s", dir)
	}
	p := Load(dir)
	if p.Name != "Zipped" || len(p.Files) != 1 || !strings.HasSuffix(p.Files["opus-stance"], ".png") {
		t.Fatalf("pack %+v", p)
	}
	w := strings.Join(warn, "|")
	for _, want := range []string{"opus-attack.jpg", "sub/kimi-stance.png", "evil.sh"} {
		if !strings.Contains(w, want) {
			t.Errorf("warnings %q missing %q", w, want)
		}
	}
}

func TestImportZipLimitsAndCleanup(t *testing.T) {
	root := t.TempDir()
	many := map[string][]byte{}
	for i := 0; i < 161; i++ {
		many[filepath.Base(t.Name())+string(rune('a'+i%26))+strings.Repeat("x", i)+".png"] = []byte("x")
	}
	if _, _, err := ImportZip(root, makeZip(t, many)); err == nil || !strings.Contains(err.Error(), "max 160") {
		t.Fatalf("161 entries: want the entry cap, got %v", err)
	}
	if _, _, err := ImportZip(root, makeZip(t, map[string][]byte{"notes.txt": []byte("x")})); err == nil {
		t.Fatal("a zip with no heroine images must be refused")
	}
	if ents, _ := os.ReadDir(root); len(ents) != 0 {
		t.Fatalf("partial output left: %v", ents)
	}
}

func TestImportFolder(t *testing.T) {
	root, src := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(src, "haiku-stance.png"), pngBytes(t, 20, 40), 0o644)
	os.WriteFile(filepath.Join(src, "manifest.json"), []byte(`{"schema":1,"heroines":{"haiku":{"name":"Rin"}}}`), 0o644)
	dir, _, err := ImportFolder(root, src)
	if err != nil {
		t.Fatal(err)
	}
	p := Load(dir)
	if len(p.Files) != 1 || p.Heroines["haiku"].Name != "Rin" {
		t.Fatalf("pack %+v", p)
	}
}

func TestWriteKit(t *testing.T) {
	dir := t.TempDir()
	if err := Store(dir, "opus-stance", bytes.NewReader(pngBytes(t, 10, 20))); err != nil {
		t.Fatal(err)
	}
	WriteManifest(dir, Manifest{Name: "Mine"})
	p := Load(dir)
	bundled := fstest.MapFS{
		"opus-attack-AbC12_-x.webp": {Data: []byte("not decodable")}, // undecodable bundled file is skipped
		"bg-ruins-12345678.webp":    {Data: []byte("x")},
	}
	var buf bytes.Buffer
	if err := WriteKit(&buf, p, bundled, []byte("a,b\n")); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range zr.File {
		got[f.Name] = true
	}
	for _, want := range []string{"opus-stance.png", "manifest.json", "heroine-prompts.csv", "README.txt"} {
		if !got[want] {
			t.Errorf("kit missing %s (has %v)", want, got)
		}
	}
	if got["bg-ruins.png"] || got["opus-attack.png"] {
		t.Errorf("kit has files it should not: %v", got)
	}
	rc, _ := zr.Open("README.txt")
	b, _ := io.ReadAll(rc)
	if !strings.Contains(string(b), "opus-stance") || !strings.Contains(string(b), "transparent") {
		t.Fatalf("README: %s", b)
	}
}

// The starter kit must import back: every stem plus manifest, csv and README.
func TestKitRoundTrip(t *testing.T) {
	src := t.TempDir()
	for _, stem := range AllStems() {
		os.WriteFile(filepath.Join(src, stem+".png"), pngBytes(t, 4, 8), 0o644)
	}
	p := Load(src)
	if len(p.Files) != len(AllStems()) {
		t.Fatalf("fixture has %d files", len(p.Files))
	}
	kit := filepath.Join(t.TempDir(), "kit.zip")
	f, _ := os.Create(kit)
	if err := WriteKit(f, p, fstest.MapFS{}, []byte("a,b\n")); err != nil {
		t.Fatal(err)
	}
	f.Close()
	dir, _, err := ImportZip(t.TempDir(), kit)
	if err != nil {
		t.Fatalf("kit does not import back: %v", err)
	}
	if n := len(Load(dir).Files); n != len(AllStems()) {
		t.Fatalf("imported %d of %d", n, len(AllStems()))
	}
}

// Minors sweep: with no pack (built-in heroines) the kit read ./manifest.json
// from whatever the working directory was.
func TestWriteKitBuiltInIgnoresWorkingDirectory(t *testing.T) {
	wd := t.TempDir()
	if err := os.WriteFile(filepath.Join(wd, "manifest.json"), []byte(`{"schema":1,"name":"Stray"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(wd)
	var buf bytes.Buffer
	if err := WriteKit(&buf, Pack{Files: map[string]string{}}, fstest.MapFS{}, nil); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	f, err := zr.Open("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(f)
	if strings.Contains(string(b), "Stray") || !strings.Contains(string(b), "My heroines") {
		t.Fatalf("kit manifest = %s", b)
	}
}
