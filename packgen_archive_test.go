package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// zipOf builds an archive from name -> contents, in order.
func zipOf(t *testing.T, dest string, entries [][2]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return dest
}

func goodManifest(t *testing.T) string {
	t.Helper()
	return `{"schema":1,"name":"Imported","canvas":{"w":8,"h":8,"scale":1,"anchor":"top-center"},
	  "alpha":false,"slots":{"idle":[{"file":"idle.png"}]}}`
}

// --- the attack this file exists for --------------------------------------

// "Zip slip": an entry name that escapes the extraction directory. Go's
// archive/zip does not sanitise names, so this is entirely on us.
func TestImportRefusesEscapingEntryNames(t *testing.T) {
	names := []string{
		"../escaped.png",
		"../../escaped.png",
		"../../../../../../tmp/escaped.png",
		`..\..\escaped.png`,
		"/etc/passwd",
		`C:\Windows\System32\evil.png`,
		"sub/idle.png",
		`sub\idle.png`,
		"./idle.png",
		"..",
		".",
	}
	for _, n := range names {
		t.Run(n, func(t *testing.T) {
			if packZipEntryAllowed(n) {
				t.Fatalf("%q was allowed; it must be refused, not sanitised", n)
			}
		})
	}
}

// The guard REFUSES rather than sanitising. filepath.Base("../../x") is "x",
// so a sanitising implementation would quietly turn an attack into an accepted
// file — and the archive would import "successfully".
func TestEscapingEntriesAreDroppedNotFlattened(t *testing.T) {
	f := newPackGenFixture(t)
	dir := t.TempDir()
	png := string(testPNG(t, 8, 8, 1, false))
	src := zipOf(t, filepath.Join(dir, "evil.zip"), [][2]string{
		{"manifest.json", goodManifest(t)},
		{"idle.png", png},
		{"../../../../escaped.png", "pwned"},
		{"sub/nested.png", png},
	})

	out, err := f.app.importPackZip(src)
	if err != nil {
		t.Fatal(err)
	}
	// Nothing escaped, in either direction: not outside the folder...
	root := companionPacksDir()
	for _, probe := range []string{
		filepath.Join(filepath.Dir(root), "escaped.png"),
		filepath.Join(root, "escaped.png"),
		filepath.Join(root, "..", "escaped.png"),
	} {
		if _, err := os.Stat(probe); err == nil {
			t.Errorf("an entry escaped to %s", probe)
		}
	}
	// ...and not flattened INTO it under a harmless-looking name.
	for _, gone := range []string{"escaped.png", "nested.png"} {
		if _, err := os.Stat(filepath.Join(out, gone)); err == nil {
			t.Errorf("%s was flattened into the pack instead of refused", gone)
		}
	}
	// The legitimate files still arrived.
	for _, want := range []string{"manifest.json", "idle.png"} {
		if _, err := os.Stat(filepath.Join(out, want)); err != nil {
			t.Errorf("%s was not extracted: %v", want, err)
		}
	}
}

// A pack is a manifest plus images. A file we would never serve has no reason
// to be written to disk at all.
func TestImportDropsFilesAPackCannotContain(t *testing.T) {
	f := newPackGenFixture(t)
	dir := t.TempDir()
	png := string(testPNG(t, 8, 8, 1, false))
	src := zipOf(t, filepath.Join(dir, "mixed.zip"), [][2]string{
		{"manifest.json", goodManifest(t)},
		{"idle.png", png},
		{"README.md", "hello"},
		{"install.sh", "#!/bin/sh\nrm -rf /"},
		{".DS_Store", "junk"},
		{"._idle.png", "resource fork"},
		{"idle.gif", "gif89a"},
	})

	out, err := f.app.importPackZip(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"README.md", "install.sh", ".DS_Store", "._idle.png", "idle.gif"} {
		if _, err := os.Stat(filepath.Join(out, gone)); err == nil {
			t.Errorf("%s was extracted; a pack cannot contain it", gone)
		}
	}
	entries, _ := os.ReadDir(out)
	if len(entries) != 2 {
		var got []string
		for _, e := range entries {
			got = append(got, e.Name())
		}
		t.Errorf("extracted %v, want just the manifest and the image", got)
	}
}

// --- refusals that protect the user's existing packs -----------------------

// An import must never write into an existing pack. Overwriting one would
// destroy art with no way back.
func TestImportNeverOverwritesAnExistingPack(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	paths, _ := packGenPathsFor("Test Pack")
	before, _ := loadPack(paths.Live)

	dir := t.TempDir()
	src := zipOf(t, filepath.Join(dir, paths.Slug+".zip"), [][2]string{
		{"manifest.json", goodManifest(t)},
		{"idle.png", string(testPNG(t, 8, 8, 9, false))},
	})

	out, err := f.app.importPackZip(src)
	if err != nil {
		t.Fatal(err)
	}
	if out == paths.Live {
		t.Fatal("the import wrote into the existing pack")
	}
	after, _ := loadPack(paths.Live)
	if after.Slots["idle"][0].ID != before.Slots["idle"][0].ID {
		t.Error("the existing pack's art changed")
	}
	if len(after.Slots) != len(before.Slots) {
		t.Error("the existing pack's shape changed")
	}
}

// A failed extraction must leave nothing behind. A half-written folder appears
// in the library and reads as a corrupt pack rather than as a failed import.
func TestFailedImportLeavesNoFolderBehind(t *testing.T) {
	f := newPackGenFixture(t)
	dir := t.TempDir()
	// Over the per-file cap: the copy is bounded while streaming, because the
	// size declared in the header is attacker-controlled.
	big := strings.Repeat("x", int(packZipMaxFile)+1024)
	src := zipOf(t, filepath.Join(dir, "huge.zip"), [][2]string{
		{"manifest.json", goodManifest(t)},
		{"idle.png", big},
	})

	if _, err := f.app.importPackZip(src); err == nil {
		t.Fatal("want an error for an oversized entry")
	}
	entries, err := os.ReadDir(companionPacksDir())
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "huge") {
			t.Errorf("a failed import left %s behind", e.Name())
		}
	}
}

func TestImportRefusals(t *testing.T) {
	f := newPackGenFixture(t)
	dir := t.TempDir()

	t.Run("not a zip", func(t *testing.T) {
		p := filepath.Join(dir, "notzip.zip")
		if err := os.WriteFile(p, []byte("this is prose"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := f.app.importPackZip(p); err == nil ||
			!strings.Contains(err.Error(), "not a readable zip") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("no manifest", func(t *testing.T) {
		src := zipOf(t, filepath.Join(dir, "nomanifest.zip"), [][2]string{
			{"idle.png", string(testPNG(t, 8, 8, 1, false))},
		})
		if _, err := f.app.importPackZip(src); err == nil ||
			!strings.Contains(err.Error(), "no manifest.json") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("too many entries", func(t *testing.T) {
		many := [][2]string{{"manifest.json", goodManifest(t)}}
		for i := 0; i < packZipMaxEntries+1; i++ {
			many = append(many, [2]string{fmt.Sprintf("f%d.png", i), "x"})
		}
		src := zipOf(t, filepath.Join(dir, "many.zip"), many)
		if _, err := f.app.importPackZip(src); err == nil ||
			!strings.Contains(err.Error(), "more than a pack contains") {
			t.Fatalf("got %v", err)
		}
	})
}

// --- the round trip --------------------------------------------------------

// The point of the feature: a pack exported and re-imported must be the same
// pack, and must still load clean.
func TestExportImportRoundTrip(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f, PackGenSlot{Slot: "idle"}, PackGenSlot{Slot: "working"},
		PackGenSlot{Slot: "bored", When: "lateNight"})
	paths, _ := packGenPathsFor("Test Pack")
	before, _ := loadPack(paths.Live)

	dest := filepath.Join(t.TempDir(), "backup.zip")
	if err := writePackZip(paths.Live, dest); err != nil {
		t.Fatal(err)
	}

	out, err := f.app.importPackZip(dest)
	if err != nil {
		t.Fatal(err)
	}
	after, warnings := loadPack(out)
	if len(after.Warnings) != 0 {
		t.Fatalf("the imported pack warns:\n  %s", strings.Join(after.Warnings, "\n  "))
	}
	_ = warnings

	if after.Name != before.Name {
		t.Errorf("name = %q, want %q", after.Name, before.Name)
	}
	if len(after.Slots) != len(before.Slots) {
		t.Fatalf("got %d slots, want %d", len(after.Slots), len(before.Slots))
	}
	for slot, vs := range before.Slots {
		if len(after.Slots[slot]) != len(vs) {
			t.Errorf("slot %q: %d variants, want %d", slot, len(after.Slots[slot]), len(vs))
		}
		for i, v := range vs {
			// Ids are content hashes, so equal ids prove the BYTES survived —
			// stronger than comparing file names.
			if after.Slots[slot][i].ID != v.ID {
				t.Errorf("slot %q variant %d changed content", slot, i)
			}
		}
	}
	// The stored base must survive, or the round trip silently costs the pack
	// its ability to regenerate a single scene.
	if _, ok := findPackBase(out); !ok {
		t.Error("the exported pack lost its stored base portrait")
	}
}

// Exporting must not follow a symlink out of the pack, for the same reason the
// loader refuses to serve through one: a pack is shared.
func TestExportSkipsSymlinks(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(goodManifest(t)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "idle.png"), testPNG(t, 8, 8, 1, false), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "sneaky.png")); err != nil {
		t.Skip("this platform does not allow symlinks: " + err.Error())
	}

	dest := filepath.Join(t.TempDir(), "out.zip")
	if err := writePackZip(dir, dest); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, ent := range zr.File {
		if ent.Name == "sneaky.png" {
			t.Error("a symlink was followed into the archive, publishing a file from outside the pack")
		}
	}
}

// A failed export must not leave a truncated archive sitting where a backup is
// supposed to be — the one place a user will not look closely.
func TestExportRefusesAnEmptyPack(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "empty.zip")
	if err := writePackZip(dir, dest); err == nil {
		t.Fatal("want an error when there is nothing exportable")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("a failed export left an archive behind")
	}
	// And no temp file either.
	entries, _ := os.ReadDir(filepath.Dir(dest))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".pack-") {
			t.Errorf("a failed export left %s behind", e.Name())
		}
	}
}

// Two imports of the same archive must produce two packs, not a collision or
// an overwrite.
func TestImportingTwiceMakesTwoPacks(t *testing.T) {
	f := newPackGenFixture(t)
	dir := t.TempDir()
	src := zipOf(t, filepath.Join(dir, "twice.zip"), [][2]string{
		{"manifest.json", goodManifest(t)},
		{"idle.png", string(testPNG(t, 8, 8, 3, false))},
	})

	first, err := f.app.importPackZip(src)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.app.importPackZip(src)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("the second import reused the first folder")
	}
	for _, d := range []string{first, second} {
		if p, _ := loadPack(d); len(p.Warnings) != 0 {
			t.Errorf("%s warns: %s", d, strings.Join(p.Warnings, "; "))
		}
	}
}

func TestPackZipEntryAllowedAcceptsWhatAPackContains(t *testing.T) {
	for _, ok := range []string{
		"manifest.json", "idle.png", "working.webp", "done.jpg", "bored.jpeg",
		"idle-latenight.png", packGenBaseStem + ".png", packGenBaseStem + ".jpg",
	} {
		if !packZipEntryAllowed(ok) {
			t.Errorf("%q should be allowed", ok)
		}
	}
}

// A zip can DESCRIBE a symlink: the entry's mode carries the link bit and its
// body is the target path. Extracting one would plant a link inside the pack
// that the media handler then has to refuse at serve time — and on an older
// build, or a different consumer of the folder, it would not be refused at all.
//
// zip.Writer.Create only ever produces regular entries, so this has to be built
// with an explicit header. That is precisely why the plain round-trip tests
// could not catch it.
func TestImportRefusesSymlinkEntries(t *testing.T) {
	f := newPackGenFixture(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "linky.zip")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, body string, mode os.FileMode) {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	add("manifest.json", goodManifest(t), 0o644)
	add("idle.png", string(testPNG(t, 8, 8, 1, false)), 0o644)
	// The body of a symlink entry is its target.
	add("sneaky.png", "/etc/passwd", os.ModeSymlink|0o777)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := f.app.importPackZip(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(filepath.Join(out, "sneaky.png")); err == nil {
		t.Errorf("a symlink entry was extracted (mode %v)", fi.Mode())
	}
	// The real files still arrived, so the refusal is targeted rather than the
	// whole import failing.
	if p, _ := loadPack(out); len(p.Slots["idle"]) != 1 {
		t.Error("refusing the symlink also lost the legitimate art")
	}
}
