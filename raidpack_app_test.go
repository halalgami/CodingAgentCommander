package main

import (
	"bytes"
	"image"
	"image/png"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/raidpack"
)

var raidHashed = regexp.MustCompile(`^(.+)-[A-Za-z0-9_-]{8}\.webp$`)

func raidApp(t *testing.T) (*App, *fakeEmitter, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	a := NewApp()
	fe := &fakeEmitter{}
	a.emitter = fe
	return a, fe, dir
}

func tinyPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	var b bytes.Buffer
	png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, w, h)))
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceCreatesFreshPack(t *testing.T) {
	a, fe, dir := raidApp(t)
	os.MkdirAll(filepath.Join(dir, "raidpacks", "my-heroines"), 0o755) // stale folder must not be reused
	src := filepath.Join(t.TempDir(), "me.png")
	tinyPNG(t, src, 40, 80)
	v, err := a.ReplaceRaidImageFrom("opus-stance", src)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(v.Dir) != "my-heroines-2" || v.Name != "My heroines" {
		t.Fatalf("pack %+v", v)
	}
	if !strings.HasPrefix(v.Files["opus-stance"], "/media/raid/opus-stance-") {
		t.Fatalf("url %q", v.Files["opus-stance"])
	}
	if got := a.RaidPackInfo(); got.Dir != v.Dir {
		t.Fatal("new pack not active / not persisted")
	}
	b := NewApp() // persisted in raid.json
	if b.RaidPackInfo().Dir != v.Dir {
		t.Fatal("raid.json not read back")
	}
	if len(fe.events) == 0 || fe.events[len(fe.events)-1] != "raid:pack" {
		t.Fatalf("events %v", fe.events)
	}
}

func TestReplaceFromRefusesBadFile(t *testing.T) {
	a, _, _ := raidApp(t)
	txt := filepath.Join(t.TempDir(), "x.txt")
	os.WriteFile(txt, []byte("hi"), 0o644)
	if _, err := a.ReplaceRaidImageFrom("opus-stance", txt); err == nil {
		t.Fatal("non-image extension accepted")
	}
	png := filepath.Join(t.TempDir(), "x.png")
	tinyPNG(t, png, 4, 4)
	if _, err := a.ReplaceRaidImageFrom("../../etc", png); err == nil {
		t.Fatal("bad stem accepted")
	}
	if _, err := a.ReplaceRaidImageFrom("opus-stance", filepath.Dir(png)); err == nil {
		t.Fatal("directory accepted")
	}
	if a.RaidPackInfo().Dir != "" {
		t.Fatal("a refused replace must not create a pack")
	}
}

func TestResetAndHeroineAndServe(t *testing.T) {
	a, _, _ := raidApp(t)
	src := filepath.Join(t.TempDir(), "me.png")
	tinyPNG(t, src, 10, 20)
	v, _ := a.ReplaceRaidImageFrom("kimi-hurt", src)
	url := v.Files["kimi-hurt"]
	rec := httptest.NewRecorder()
	a.packMediaHandler().ServeHTTP(rec, httptest.NewRequest("GET", url, nil))
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("serve %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, bad := range []string{"/media/raid/nope", "/media/raid/a/b", "/media/raid/"} {
		rec := httptest.NewRecorder()
		a.packMediaHandler().ServeHTTP(rec, httptest.NewRequest("GET", bad, nil))
		if rec.Code != 404 {
			t.Fatalf("%s → %d", bad, rec.Code)
		}
	}
	v, err := a.SetRaidHeroine("kimi", "Aiko", "#112233")
	if err != nil || v.Heroines["kimi"].Name != "Aiko" {
		t.Fatalf("heroine %v %+v", err, v.Heroines)
	}
	if _, err := a.SetRaidHeroine("kimi", "", "blue"); err == nil {
		t.Fatal("bad tint accepted")
	}
	v, _ = a.SetRaidHeroine("kimi", "", "")
	if _, ok := v.Heroines["kimi"]; ok {
		t.Fatal("empty name+tint must remove the entry")
	}
	v, err = a.ResetRaidImage("kimi-hurt")
	if err != nil || v.Files["kimi-hurt"] != "" {
		t.Fatalf("reset %v %+v", err, v.Files)
	}
	rec = httptest.NewRecorder()
	a.packMediaHandler().ServeHTTP(rec, httptest.NewRequest("GET", url, nil))
	if rec.Code != 404 {
		t.Fatal("old id still served after reset")
	}
}

func TestServeRefusesSymlinkPlantedAfterLoad(t *testing.T) {
	a, _, _ := raidApp(t)
	src := filepath.Join(t.TempDir(), "me.png")
	tinyPNG(t, src, 10, 20)
	v, _ := a.ReplaceRaidImageFrom("opus-stance", src)
	p := filepath.Join(v.Dir, "opus-stance.png")
	os.Remove(p)
	os.Symlink(src, p)
	rec := httptest.NewRecorder()
	a.packMediaHandler().ServeHTTP(rec, httptest.NewRequest("GET", v.Files["opus-stance"], nil))
	if rec.Code != 404 {
		t.Fatalf("symlink served: %d", rec.Code)
	}
}

func TestActivePackVanished(t *testing.T) {
	a, _, _ := raidApp(t)
	src := filepath.Join(t.TempDir(), "me.png")
	tinyPNG(t, src, 10, 20)
	v, _ := a.ReplaceRaidImageFrom("opus-stance", src)
	os.RemoveAll(v.Dir)
	got := a.RaidPackInfo()
	if len(got.Files) != 0 || len(got.Warnings) == 0 {
		t.Fatalf("vanished pack: %+v", got)
	}
}

func TestListSelectClearDelete(t *testing.T) {
	a, _, dir := raidApp(t)
	src := filepath.Join(t.TempDir(), "me.png")
	tinyPNG(t, src, 10, 20)
	v1, _ := a.ReplaceRaidImageFrom("opus-stance", src)
	if err := a.DeleteRaidPack(v1.Dir); err == nil {
		t.Fatal("deleting the active pack must be refused")
	}
	if _, err := a.ClearRaidPack(); err != nil {
		t.Fatal(err)
	}
	v2, _ := a.ReplaceRaidImageFrom("glm-stance", src) // creates my-heroines-2
	list := a.ListRaidPacks()
	if len(list) != 2 {
		t.Fatalf("list %+v", list)
	}
	if _, err := a.SelectRaidPack(v1.Dir); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteRaidPack(v2.Dir); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteRaidPack(dir); err == nil {
		t.Fatal("a folder outside raidpacks must be refused")
	}
	if _, err := a.SelectRaidPack(t.TempDir()); err == nil {
		t.Fatal("selecting a folder outside raidpacks must be refused")
	}
}

// Wails runs each bound call on its own goroutine: quick replaces with no pack
// active must land in ONE pack, and the served id map must match the last view.
func TestConcurrentReplacesShareOnePack(t *testing.T) {
	for round := 0; round < 20; round++ {
		a, _, _ := raidApp(t)
		src := filepath.Join(t.TempDir(), "me.png")
		tinyPNG(t, src, 10, 20)
		stems := []string{"opus-stance", "kimi-stance", "glm-hurt", "qwen-await"}
		done := make(chan error, len(stems))
		for _, s := range stems {
			go func(s string) { _, err := a.ReplaceRaidImageFrom(s, src); done <- err }(s)
		}
		for range stems {
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		}
		v := a.RaidPackInfo()
		if len(v.Files) != len(stems) {
			t.Fatalf("round %d: active pack has %d of %d images (%v)", round, len(v.Files), len(stems), a.ListRaidPacks())
		}
	}
}

// The starter kit is built from the embedded frontend/dist/assets. Vite inlines
// small images as data: URIs and never writes them there, so every stem a pack
// can hold must exist as a real hashed file or the kit silently drops it.
func TestEmbeddedAssetsHaveEveryStem(t *testing.T) {
	sub, err := fs.Sub(assets, "frontend/dist/assets")
	if err != nil {
		t.Fatal(err)
	}
	ents, _ := fs.ReadDir(sub, ".")
	have := map[string]bool{}
	for _, e := range ents {
		if m := raidHashed.FindStringSubmatch(e.Name()); m != nil {
			have[m[1]] = true
		}
	}
	var missing []string
	for _, s := range raidpack.AllStems() {
		if !have[s] {
			missing = append(missing, s)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("%d stems not in the build (inlined by Vite?): %v", len(missing), missing)
	}
}

// Minors sweep: an unknown heroine id was written into the manifest.
func TestSetRaidHeroineRejectsUnknownID(t *testing.T) {
	a, _, _ := raidApp(t)
	if _, err := a.SetRaidHeroine("../evil", "x", ""); err == nil {
		t.Fatal("unknown id accepted")
	}
}

// Minors sweep: a dropped file was read whole before the 32 MiB limit applied.
func TestReplaceFromRefusesHugeFileWithoutReading(t *testing.T) {
	a, _, dir := raidApp(t)
	p := filepath.Join(dir, "big.png")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(64 << 20); err != nil {
		t.Fatal(err)
	}
	f.Close()
	// Unreadable: only a size check made before reading can name the real problem.
	if err := os.Chmod(p, 0o000); err != nil {
		t.Fatal(err)
	}
	_, err = a.ReplaceRaidImageFrom("opus-stance", p)
	if err == nil || !strings.Contains(err.Error(), "over 32 MiB") {
		t.Fatalf("want the size refusal, got %v", err)
	}
}
