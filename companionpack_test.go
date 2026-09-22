package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// hasWarning reports whether any warning contains every one of subs.
func hasWarning(warnings []string, subs ...string) bool {
	for _, w := range warnings {
		all := true
		for _, s := range subs {
			if !strings.Contains(w, s) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// The slot vocabulary is the pack format's public API (§5.1) and the one
// artifact that cannot change once art exists. Freeze it in a test.
func TestPackVocabularyIsFrozen(t *testing.T) {
	want := []string{
		"idle", "working", "done", "awaiting", "error", "bored",
		"bg_idle", "bg_running", "bg_finished", "bg_error",
	}
	if len(packSlots) != len(want) {
		t.Fatalf("slot count changed: %v", packSlots)
	}
	for i, s := range want {
		if packSlots[i] != s {
			t.Fatalf("slot %d = %q, want %q", i, packSlots[i], s)
		}
	}
	wantClass := map[string]string{
		"lateNight": "ambient", "weekend": "ambient", "marathon": "ambient",
		"longIdle": "ambient", "freshInstall": "ambient",
		"firstRunOfDay": "event", "century": "event", "streak": "event",
	}
	if len(packConditions) != len(wantClass) {
		t.Fatalf("condition count changed: %v", packConditions)
	}
	for name, class := range wantClass {
		if packConditions[name] != class {
			t.Errorf("%s class = %q, want %q", name, packConditions[name], class)
		}
	}
}

// The slot x condition matrix is machine-readable so project B can filter on
// it and never spend money on art that cannot display (§5.5).
func TestPackConditionSlotMatrix(t *testing.T) {
	cases := []struct {
		cond, slot string
		want       bool
	}{
		{"longIdle", "idle", false}, // bored outranks idle after 5 min
		{"longIdle", "bored", true},
		{"marathon", "working", true},
		{"marathon", "idle", false}, // nothing is running
		{"century", "done", true},
		{"century", "awaiting", true},
		{"century", "working", false},
		{"streak", "done", true},
		{"lateNight", "idle", true},
		{"lateNight", "bg_idle", false}, // background art never runs the resolver
		{"firstRunOfDay", "idle", true},
		// A greeting must survive the user launching a session immediately;
		// see the matrix comment. This intentionally mirrors pack.js.
		{"firstRunOfDay", "done", true},
		{"firstRunOfDay", "working", true},
		{"firstRunOfDay", "bg_idle", false},
	}
	for _, tc := range cases {
		if got := packCondAllowed(tc.cond, tc.slot); got != tc.want {
			t.Errorf("packCondAllowed(%q, %q) = %v, want %v", tc.cond, tc.slot, got, tc.want)
		}
	}
	// Every condition must name only frozen slots, or the matrix is a typo farm.
	for cond, slots := range packCondSlots {
		if _, ok := packConditions[cond]; !ok {
			t.Errorf("matrix names unknown condition %q", cond)
		}
		for _, s := range slots {
			if !packSlotKnown(s) {
				t.Errorf("condition %q names unknown slot %q", cond, s)
			}
		}
	}
}

func TestLoadPackGood(t *testing.T) {
	p, ids := loadPack(filepath.Join("testdata", "packs", "good"))

	if len(p.Warnings) != 0 {
		t.Fatalf("a valid pack must produce no warnings, got %v", p.Warnings)
	}
	if p.Schema != 1 || p.Name != "Fixture Good" || !p.Alpha {
		t.Fatalf("header wrong: %+v", p)
	}
	if p.Canvas.W != 832 || p.Canvas.H != 1216 || p.Canvas.Scale != 2 || p.Canvas.Anchor != "bottom-center" {
		t.Fatalf("canvas wrong: %+v", p.Canvas)
	}
	if len(p.Slots["idle"]) != 2 || len(p.Slots["working"]) != 1 || len(p.Slots["bg_running"]) != 1 {
		t.Fatalf("slots wrong: %+v", p.Slots)
	}
	v := p.Slots["idle"][0]
	if v.ID != "idle-a" || v.Weight != 1 || v.FocusY != 0.28 {
		t.Fatalf("variant defaults wrong: %+v", v)
	}
	if v.Anim == nil || v.Anim.Frames != 12 || v.Anim.FPS != 8 {
		t.Fatalf("anim wrong: %+v", v.Anim)
	}
	// Every declared id, plus the sprite strip, must be servable.
	for _, want := range []string{"idle-a", "idle-a-strip", "idle-secret", "work-a", "bgr"} {
		path, ok := ids[want]
		if !ok {
			t.Fatalf("id %q missing from the media map: %v", want, ids)
		}
		if !filepath.IsAbs(path) {
			t.Errorf("id %q maps to a relative path %q", want, path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("id %q maps to an unreadable path: %v", want, err)
		}
	}
}

func TestLoadPackAssignsContentHashIDs(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "a.png")
	if err := os.WriteFile(png, []byte("not really a png, but stable bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":1,"canvas":{"w":8,"h":8,"scale":1},"alpha":true,
	  "slots":{"idle":[{"file":"a.png"}]}}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	p1, ids1 := loadPack(dir)
	p2, _ := loadPack(dir)
	id := p1.Slots["idle"][0].ID
	if id == "" {
		t.Fatal("a variant with no id must get a content hash")
	}
	if id != p2.Slots["idle"][0].ID {
		t.Fatal("content-hash ids must be stable across loads — they are the cache key in the URL")
	}
	if len(id) != 16 {
		t.Errorf("id %q should be a 16-char hash prefix", id)
	}
	if _, ok := ids1[id]; !ok {
		t.Errorf("hashed id not in the media map: %v", ids1)
	}
}

func TestLoadPackWarnings(t *testing.T) {
	p, ids := loadPack(filepath.Join("testdata", "packs", "warnings"))

	checks := []struct {
		name string
		subs []string
	}{
		{"non-integer schema", []string{"schema", "1"}},
		{"unknown top-level key", []string{"colourway"}},
		{"missing canvas", []string{"canvas"}},
		{"unknown slot", []string{"dancing"}},
		{"missing file", []string{"missing.png"}},
		{"bad extension", []string{"notes.txt"}},
		{"unknown variant key", []string{"tint"}},
		// Distinctive text, not just "rarity"+"0": that pair is ALSO satisfied
		// by "rarity 0.9 is outside (0, 0.5] and was clamped" (contains both
		// substrings), so a generic match here would pass even with the
		// explicit-rarity-0 branch deleted entirely.
		{"rarity zero", []string{"hash-me", "rarity 0 never displays"}},
		{"rarity out of range", []string{"rarity", "0.9"}},
		{"unknown condition", []string{"fullMoon"}},
		{"incompatible slot/condition", []string{"longIdle", "idle"}},
		{"condition in a background slot", []string{"century", "bg_idle"}},
		{"unknown mood", []string{"smug"}},
		{"simultaneous ambient conditions", []string{"lateNight", "weekend"}},
	}
	for _, c := range checks {
		if !hasWarning(p.Warnings, c.subs...) {
			t.Errorf("missing warning for %s (wanted all of %v)\ngot:\n  %s",
				c.name, c.subs, strings.Join(p.Warnings, "\n  "))
		}
	}
	// Warnings are never fatal: the loadable art still loads.
	if len(p.Slots["idle"]) == 0 {
		t.Error("warnings must not empty a slot that has usable variants")
	}
	if _, ok := p.Slots["dancing"]; ok {
		t.Error("an unknown slot must be dropped, not kept")
	}
	for _, dead := range []string{"gone", "not-an-image"} {
		if _, ok := ids[dead]; ok {
			t.Errorf("unusable variant %q must not be servable", dead)
		}
	}
	// A cleared condition must actually be cleared, not just warned about.
	var hashMeKept bool
	for _, v := range p.Slots["idle"] {
		if v.ID == "unreachable" && v.When != "" {
			t.Error("an incompatible condition must be cleared so the resolver never sees it")
		}
		if v.ID == "loud" && v.Rarity > 0.5 {
			t.Errorf("out-of-range rarity must be clamped, got %v", v.Rarity)
		}
		if v.ID == "hash-me" {
			hashMeKept = true
			if v.Rarity != 0 {
				t.Error("rarity 0 must be cleared into the base pool, not left as a dead roll")
			}
		}
	}
	// The fixture also has several OTHER idle variants (nightly, weekendly,
	// unreachable, no-such-cond, odd-mood) that omit `rarity` entirely — an
	// implicit, un-declared rarity of 0. The explicit-rarity-0 warning must
	// fire for hash-me alone: if the code that distinguishes "declared 0"
	// from "absent" regresses (e.g. drops the presence check and matches on
	// the zero VALUE instead of the raw key), every one of those would also
	// warn, and this count would climb past 1.
	if !hashMeKept {
		t.Fatal("hash-me must land in the idle slot's base pool, not be dropped")
	}
	n := 0
	for _, w := range p.Warnings {
		if strings.Contains(w, "never displays") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("want exactly 1 \"never displays\" warning (hash-me only), got %d:\n  %s", n, strings.Join(p.Warnings, "\n  "))
	}
}

func TestLoadPackNeverFatal(t *testing.T) {
	t.Run("bad json", func(t *testing.T) {
		p, ids := loadPack(filepath.Join("testdata", "packs", "badjson"))
		if !hasWarning(p.Warnings, "JSON") {
			t.Fatalf("want a JSON warning, got %v", p.Warnings)
		}
		if p.Slots == nil || len(ids) != 0 {
			t.Fatalf("a broken manifest must yield an empty, non-nil pack: %+v", p)
		}
	})
	t.Run("missing manifest", func(t *testing.T) {
		p, _ := loadPack(t.TempDir())
		if !hasWarning(p.Warnings, "manifest.json") {
			t.Fatalf("want a manifest warning, got %v", p.Warnings)
		}
	})
	t.Run("future major refuses", func(t *testing.T) {
		p, ids := loadPack(filepath.Join("testdata", "packs", "future"))
		if !hasWarning(p.Warnings, "newer") {
			t.Fatalf("want a schema-too-new warning, got %v", p.Warnings)
		}
		if len(ids) != 0 {
			t.Fatal("a refused pack must serve nothing")
		}
	})
}

// Two variants can legitimately reuse one image file — the obvious authoring
// shortcut of pointing `done` and `awaiting` at the same idle frame — without
// either declaring an `id`. The id is content-addressed, so this collides on
// id with NO malice and NO information loss intended: a shared URL is not a
// reason to delete art. Both variants must survive.
func TestLoadPackDuplicateIDSameFileIsKept(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.png"), []byte("shared art"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":1,"canvas":{"w":8,"h":8,"scale":1},"alpha":true,
	  "slots":{
	    "idle":[{"file":"a.png"}],
	    "done":[{"file":"a.png"}],
	    "awaiting":[{"file":"a.png"}]
	  }}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	p, ids := loadPack(dir)
	if hasWarning(p.Warnings, "duplicate id") {
		t.Errorf("reusing one file across slots must not warn about a duplicate id: %v", p.Warnings)
	}
	if len(p.Slots["idle"]) != 1 || len(p.Slots["done"]) != 1 || len(p.Slots["awaiting"]) != 1 {
		t.Fatalf("every slot reusing the same file must keep its variant: idle=%d done=%d awaiting=%d",
			len(p.Slots["idle"]), len(p.Slots["done"]), len(p.Slots["awaiting"]))
	}
	id := p.Slots["idle"][0].ID
	if id == "" {
		t.Fatal("content-hash id must still be assigned")
	}
	if p.Slots["done"][0].ID != id || p.Slots["awaiting"][0].ID != id {
		t.Fatalf("all three variants share one file, so they must share one id: idle=%q done=%q awaiting=%q",
			id, p.Slots["done"][0].ID, p.Slots["awaiting"][0].ID)
	}
	if _, ok := ids[id]; !ok {
		t.Errorf("the shared id must still be servable: %v", ids)
	}
}

// A declared id can only collide across two DIFFERENT files, and that is a
// genuine authoring mistake: the second variant must still be dropped, with
// a warning.
func TestLoadPackDuplicateIDDifferentFileWarnsAndDrops(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.png"), []byte("art a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.png"), []byte("art b, different bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":1,"canvas":{"w":8,"h":8,"scale":1},"alpha":true,
	  "slots":{
	    "idle":[{"id":"shared","file":"a.png"}],
	    "done":[{"id":"shared","file":"b.png"}]
	  }}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	p, ids := loadPack(dir)
	if !hasWarning(p.Warnings, "duplicate id", "shared") {
		t.Errorf("a declared id reused across two different files must warn, got %v", p.Warnings)
	}
	// Slots are walked in SORTED order (validatePackSlots), not manifest
	// order, so "done" (< "idle" lexicographically) is processed first and
	// wins the id; "idle" is the later claimant and is dropped.
	if len(p.Slots["idle"]) != 0 || len(p.Slots["done"]) != 1 {
		t.Fatalf("the first-processed slot keeps the id, the later one is dropped: idle=%d done=%d",
			len(p.Slots["idle"]), len(p.Slots["done"]))
	}
	abs, err := filepath.Abs(filepath.Join(dir, "b.png"))
	if err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatal(err)
	}
	if ids["shared"] != real {
		t.Errorf("the surviving id must still point at the FIRST file, got %q want %q", ids["shared"], real)
	}
}

// A sprite-strip's id ("<variant id>-strip") shares the same namespace as
// every other media id, so it must go through the same clash check the main
// id assignment does. Order must not matter: whichever variant is validated
// first keeps its id, the second collides and is warned about, but neither
// direction may silently redirect an existing id to the wrong file.
func TestLoadPackStripIDClashIsDetected(t *testing.T) {
	newFixture := func(t *testing.T, animFirst bool) string {
		t.Helper()
		dir := t.TempDir()
		for _, f := range []string{"x.png", "strip.png", "explicit.png"} {
			if err := os.WriteFile(filepath.Join(dir, f), []byte("bytes-"+f), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		animVariant := `{"id":"x","file":"x.png","anim":{"strip":"strip.png","frames":4,"fps":6}}`
		explicitVariant := `{"id":"x-strip","file":"explicit.png"}`
		order := explicitVariant + "," + animVariant
		if animFirst {
			order = animVariant + "," + explicitVariant
		}
		manifest := `{"schema":1,"canvas":{"w":8,"h":8,"scale":1},"alpha":true,
		  "slots":{"idle":[` + order + `]}}`
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	realPath := func(t *testing.T, dir, name string) string {
		t.Helper()
		abs, err := filepath.Abs(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		real, err := filepath.EvalSymlinks(abs)
		if err != nil {
			t.Fatal(err)
		}
		return real
	}

	t.Run("explicit id-strip registered first", func(t *testing.T) {
		dir := newFixture(t, false)
		p, ids := loadPack(dir)
		if !hasWarning(p.Warnings, "x-strip") {
			t.Errorf("want a warning naming the colliding id, got %v", p.Warnings)
		}
		// The first registrant (the explicit variant's own file) must win —
		// NOT silently be overwritten by the strip.
		if ids["x-strip"] != realPath(t, dir, "explicit.png") {
			t.Errorf("x-strip must still point at explicit.png, got %q", ids["x-strip"])
		}
		var xKept bool
		for _, v := range p.Slots["idle"] {
			if v.ID == "x" {
				xKept = true
				if v.Anim != nil {
					t.Error("the colliding anim must be dropped, not silently served under the wrong id")
				}
			}
		}
		if !xKept {
			t.Error("variant x itself must still be kept, just without its animation")
		}
	})

	t.Run("anim strip registered first", func(t *testing.T) {
		dir := newFixture(t, true)
		p, ids := loadPack(dir)
		if !hasWarning(p.Warnings, "duplicate id", "x-strip") {
			t.Errorf("want a duplicate-id warning naming x-strip, got %v", p.Warnings)
		}
		// The strip (registered first) must keep the id — not be overwritten.
		if ids["x-strip"] != realPath(t, dir, "strip.png") {
			t.Errorf("x-strip must still point at strip.png, got %q", ids["x-strip"])
		}
		var xKept bool
		for _, v := range p.Slots["idle"] {
			if v.ID == "x" {
				xKept = true
				// v.Anim.Strip carries the raw manifest string (the resolved
				// absolute path lives only in the ids map, keyed by
				// "x-strip"), so check the field the JSON declared.
				if v.Anim == nil || v.Anim.Strip != "strip.png" {
					t.Errorf("variant x must keep its own animation when it registered its strip id first: %+v", v.Anim)
				}
			}
		}
		if !xKept {
			t.Error("variant x must be kept")
		}
		var explicitKept bool
		for _, v := range p.Slots["idle"] {
			if v.ID == "x-strip" {
				explicitKept = true
			}
		}
		if explicitKept {
			t.Error("the second claimant of x-strip must be dropped")
		}
	})
}

// Two variants can share one `when` condition — the normal way to give a
// single condition several weighted variants — and must not trigger the
// "simultaneous ambient conditions" warning, which exists for genuinely
// DIFFERENT conditions that can both hold at once.
func TestLoadPackSharedAmbientConditionDoesNotWarn(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.png"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.png"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":1,"canvas":{"w":8,"h":8,"scale":1},"alpha":true,
	  "slots":{"idle":[
	    {"id":"night-a","file":"a.png","when":"lateNight"},
	    {"id":"night-b","file":"b.png","when":"lateNight"}
	  ]}}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	p, _ := loadPack(dir)
	if hasWarning(p.Warnings, "can all hold at once") {
		t.Errorf("two variants sharing one condition must not warn about simultaneity: %v", p.Warnings)
	}
	if len(p.Slots["idle"]) != 2 {
		t.Fatalf("both weighted variants for the shared condition must be kept, got %d", len(p.Slots["idle"]))
	}
	for _, v := range p.Slots["idle"] {
		if v.When != "lateNight" {
			t.Errorf("condition must survive uncleared for a shared-condition variant, got %q", v.When)
		}
	}
}

func TestLoadCompanionPackBinding(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	a := NewApp()

	if _, err := a.LoadCompanionPack(); err == nil {
		t.Fatal("no pack configured must be an error the UI can branch on")
	}

	abs, err := filepath.Abs(filepath.Join("testdata", "packs", "good"))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.saveAndSyncCompanion(func(c *CompanionConfig) { c.PackPath = abs }); err != nil {
		t.Fatal(err)
	}
	p, err := a.LoadCompanionPack()
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Slots["idle"]) != 2 {
		t.Fatalf("binding returned the wrong pack: %+v", p.Slots)
	}
	a.packMu.Lock()
	n := len(a.packPaths)
	a.packMu.Unlock()
	if n != 5 {
		t.Fatalf("the media map should hold 5 ids after a load, got %d", n)
	}
}

// TestLoadPackRefusesEscapingFile covers the path-escape guard in
// resolvePackFile. A manifest is user-supplied data, so a `file` that climbs
// out of the pack folder must be dropped with a warning rather than resolved:
// the id map it would otherwise populate is what the media handler serves
// from, and an entry pointing at an arbitrary absolute path would turn a pack
// into a file-disclosure primitive.
func TestLoadPackRefusesEscapingFile(t *testing.T) {
	root := t.TempDir()
	// A real, readable file OUTSIDE the pack — the thing an escape would reach.
	secret := filepath.Join(root, "secret.png")
	if err := os.WriteFile(secret, []byte("outside the pack"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "pack")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(dir, "ok.png")
	if err := os.WriteFile(inside, []byte("inside the pack"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A symlinked FILE, inside the pack folder, pointing at a file outside it.
	linkedFile := filepath.Join(dir, "link.png")
	if err := os.Symlink(secret, linkedFile); err != nil {
		t.Skip("this platform does not allow symlinks: " + err.Error())
	}

	// A symlinked DIRECTORY, inside the pack folder, pointing at a directory
	// outside it that contains a real, readable file.
	outsideDir := filepath.Join(root, "outside-dir")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideDirFile := filepath.Join(outsideDir, "x.png")
	if err := os.WriteFile(outsideDirFile, []byte("outside via a symlinked dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkedDir := filepath.Join(dir, "sub")
	if err := os.Symlink(outsideDir, linkedDir); err != nil {
		t.Fatal(err)
	}

	manifest := `{"schema":1,"canvas":{"w":8,"h":8,"scale":1},"alpha":true,
	  "slots":{"idle":[
	    {"id":"climb","file":"../secret.png"},
	    {"id":"absolute","file":"/etc/hosts"},
	    {"id":"sneaky","file":"sub/../../secret.png"},
	    {"id":"symlinked-file","file":"link.png"},
	    {"id":"symlinked-dir","file":"sub/x.png"},
	    {"id":"fine","file":"ok.png"}
	  ]}}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	p, ids := loadPack(dir)

	// Every escaping variant is gone; the legitimate one survives.
	var kept []string
	for _, v := range p.Slots["idle"] {
		kept = append(kept, v.ID)
	}
	if !slices.Equal(kept, []string{"fine"}) {
		t.Errorf("kept variants = %v, want only [fine]", kept)
	}
	for _, bad := range []string{"climb", "absolute", "sneaky", "symlinked-file", "symlinked-dir"} {
		if path, ok := ids[bad]; ok {
			t.Errorf("escaping variant %q reached the media map as %q", bad, path)
		}
	}
	// Nothing the handler can serve may live outside the pack folder. Resolve
	// symlinks on the pack dir itself before comparing: on macOS t.TempDir()
	// sits under /var, which is ITSELF a symlink to /private/var, so a
	// lexical Rel against the un-resolved dir would spuriously flag every
	// entry (resolvePackFile now stores the EvalSymlinks-resolved path).
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	for id, path := range ids {
		rel, err := filepath.Rel(realDir, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Errorf("media map entry %q -> %q escapes the pack folder", id, path)
		}
	}
	if len(p.Warnings) < 5 {
		t.Errorf("expected a warning per dropped variant, got %v", p.Warnings)
	}
}

// A broken symlink (target does not exist) must warn-and-drop, not crash the
// load — the same as a genuinely missing file, since to the pack author the
// two are indistinguishable.
func TestLoadPackBrokenSymlinkWarnsAndDrops(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.png")
	if err := os.Symlink(filepath.Join(dir, "does-not-exist.png"), broken); err != nil {
		t.Skip("this platform does not allow symlinks: " + err.Error())
	}
	manifest := `{"schema":1,"canvas":{"w":8,"h":8,"scale":1},"alpha":true,
	  "slots":{"idle":[{"id":"broken","file":"broken.png"}]}}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	p, ids := loadPack(dir)
	if _, ok := ids["broken"]; ok {
		t.Error("a broken symlink must not reach the media map")
	}
	if !hasWarning(p.Warnings, "broken.png") {
		t.Errorf("want a warning naming broken.png, got %v", p.Warnings)
	}
}

// loadedApp is an App with the `good` fixture pack loaded, for handler tests.
func loadedApp(t *testing.T) (*App, Pack) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	a := NewApp()
	abs, err := filepath.Abs(filepath.Join("testdata", "packs", "good"))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.saveAndSyncCompanion(func(c *CompanionConfig) { c.PackPath = abs }); err != nil {
		t.Fatal(err)
	}
	p, err := a.LoadCompanionPack()
	if err != nil {
		t.Fatal(err)
	}
	return a, p
}

func TestServePackMediaServesKnownID(t *testing.T) {
	a, _ := loadedApp(t)
	w := httptest.NewRecorder()
	a.servePackMedia(w, httptest.NewRequest("GET", mediaPackPrefix+"idle-a", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Body.Len() == 0 {
		t.Fatal("expected image bytes")
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "image/png") {
		t.Errorf("content-type = %q, want image/png", ct)
	}
	if w.Header().Get("Last-Modified") == "" {
		t.Error("ServeContent should set Last-Modified")
	}
	if w.Header().Get("Accept-Ranges") != "bytes" {
		t.Error("ServeContent should advertise range support")
	}
}

// The handler must never turn a request path into a filesystem path. Every
// one of these is a lookup miss, not a traversal that happens to be blocked.
func TestServePackMediaRefusesTraversal(t *testing.T) {
	a, _ := loadedApp(t)
	for _, id := range []string{
		"../../../../etc/passwd",
		"..%2F..%2Fetc%2Fpasswd",
		"/etc/passwd",
		"idle-a/../../../../etc/passwd",
		"",           // the bare prefix
		"idle_a.png", // the real FILE name is not an id
	} {
		w := httptest.NewRecorder()
		a.servePackMedia(w, httptest.NewRequest("GET", mediaPackPrefix+id, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("id %q: want 404, got %d (%d bytes)", id, w.Code, w.Body.Len())
		}
		if strings.Contains(w.Body.String(), "root:") {
			t.Fatalf("id %q leaked file content", id)
		}
	}
}

// Files can vanish between load and serve, so the handler re-stats every time
// rather than trusting the map (§6.5).
func TestServePackMediaVanishedFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	packDir := filepath.Join(dir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(packDir, "idle.png")
	if err := os.WriteFile(img, []byte("bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":1,"canvas":{"w":8,"h":8,"scale":1},"alpha":true,
	  "slots":{"idle":[{"id":"only","file":"idle.png"}]}}`
	if err := os.WriteFile(filepath.Join(packDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	if err := a.saveAndSyncCompanion(func(c *CompanionConfig) { c.PackPath = packDir }); err != nil {
		t.Fatal(err)
	}
	if _, err := a.LoadCompanionPack(); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	a.servePackMedia(w, httptest.NewRequest("GET", mediaPackPrefix+"only", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("precondition: want 200, got %d", w.Code)
	}

	if err := os.Remove(img); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	a.servePackMedia(w, httptest.NewRequest("GET", mediaPackPrefix+"only", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("a vanished file must 404, got %d", w.Code)
	}
}

// A stale id whose map entry points at a non-whitelisted extension must still
// be refused at serve time, not only at load time.
func TestServePackMediaExtensionWhitelistAtServeTime(t *testing.T) {
	a, _ := loadedApp(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "payload.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho pwned\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.packMu.Lock()
	a.packPaths["smuggled"] = script
	a.packMu.Unlock()

	w := httptest.NewRecorder()
	a.servePackMedia(w, httptest.NewRequest("GET", mediaPackPrefix+"smuggled", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 for a non-image extension, got %d: %s", w.Code, w.Body.String())
	}
}

// A pack folder is user data that can change after load. If whatever sits at
// an already-mapped path is replaced by a symlink AFTER the load-time
// containment check ran, os.Open would happily follow it — this is the
// serve-time TOCTOU half of the symlink-escape fix, distinct from the
// load-time containment check covered by TestLoadPackRefusesEscapingFile.
func TestServePackMediaRefusesSymlinkPlantedAfterLoad(t *testing.T) {
	a, _ := loadedApp(t)

	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.png")
	if err := os.WriteFile(secret, []byte("outside the pack, planted after load"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Simulate the map already pointing at a path that, right now, IS a
	// symlink to something outside the pack — this is what a TOCTOU swap
	// looks like from servePackMedia's point of view, whether or not that
	// exact path held a plain file at load time.
	dir := t.TempDir()
	planted := filepath.Join(dir, "was-a-file.png")
	if err := os.Symlink(secret, planted); err != nil {
		t.Skip("this platform does not allow symlinks: " + err.Error())
	}
	a.packMu.Lock()
	a.packPaths["swapped"] = planted
	a.packMu.Unlock()

	w := httptest.NewRecorder()
	a.servePackMedia(w, httptest.NewRequest("GET", mediaPackPrefix+"swapped", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 for a path that is a symlink at serve time, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "outside the pack") {
		t.Fatal("symlinked-in content leaked through the handler")
	}

	// A legitimate, non-symlink entry must still serve fine — the guard must
	// not be so broad it breaks ordinary files.
	w2 := httptest.NewRecorder()
	a.servePackMedia(w2, httptest.NewRequest("GET", mediaPackPrefix+"idle-a", nil))
	if w2.Code != http.StatusOK {
		t.Fatalf("a plain, non-symlink entry must still serve: got %d", w2.Code)
	}
}

func TestSetCompanionPackValidatesManifest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	a := NewApp()

	t.Run("folder without a manifest is refused", func(t *testing.T) {
		empty := filepath.Join(dir, "empty")
		if err := os.MkdirAll(empty, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := a.setCompanionPack(empty); err == nil {
			t.Fatal("expected an error for a folder with no manifest.json")
		}
		if a.GetCompanionConfig().PackPath != "" {
			t.Fatalf("a rejected pick must not be persisted: %q", a.GetCompanionConfig().PackPath)
		}
	})

	t.Run("a file rather than a folder is refused", func(t *testing.T) {
		f := filepath.Join(dir, "a-file.png")
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := a.setCompanionPack(f); err == nil {
			t.Fatal("expected an error for a file")
		}
	})

	t.Run("a real pack is accepted, persisted and loaded", func(t *testing.T) {
		abs, err := filepath.Abs(filepath.Join("testdata", "packs", "good"))
		if err != nil {
			t.Fatal(err)
		}
		name, err := a.setCompanionPack(abs)
		if err != nil {
			t.Fatal(err)
		}
		if name != "good" {
			t.Errorf("want the folder's base name, got %q", name)
		}
		if a.GetCompanionConfig().PackPath != abs {
			t.Fatalf("not persisted in memory: %+v", a.GetCompanionConfig())
		}
		if loadCompanionConfig().PackPath != abs {
			t.Fatal("not persisted to disk")
		}
		a.packMu.Lock()
		n := len(a.packPaths)
		a.packMu.Unlock()
		if n == 0 {
			t.Error("picking a pack should load it, so the media map is ready before the first request")
		}
	})

	t.Run("cancel leaves the config alone", func(t *testing.T) {
		before := a.GetCompanionConfig().PackPath
		if _, err := a.setCompanionPack(""); err == nil {
			t.Fatal("expected an error for an empty path")
		}
		if a.GetCompanionConfig().PackPath != before {
			t.Fatal("config changed on an empty pick")
		}
	})
}

func TestCompanionPacksDirSitsBesideTheConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	if got, want := companionPacksDir(), filepath.Join(dir, "packs"); got != want {
		t.Fatalf("packs dir = %q, want %q", got, want)
	}
}

// The handler is also reachable through the mux main.go registers.
func TestPackMediaHandlerRoutes(t *testing.T) {
	a, _ := loadedApp(t)
	srv := httptest.NewServer(a.packMediaHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + mediaPackPrefix + "bgr")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 through the mux, got %d", resp.StatusCode)
	}

	resp2, err := http.Get(srv.URL + "/some/other/path")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("unrelated paths must 404, got %d", resp2.StatusCode)
	}
}

func TestClearCompanionPack(t *testing.T) {
	a, _ := loadedApp(t)
	if a.GetCompanionConfig().PackPath == "" {
		t.Fatal("loadedApp should start with a pack configured")
	}

	if err := a.ClearCompanionPack(); err != nil {
		t.Fatalf("ClearCompanionPack: %v", err)
	}
	if got := a.GetCompanionConfig().PackPath; got != "" {
		t.Fatalf("PackPath = %q after clear, want empty", got)
	}
	// The id map must go too, or /media/pack/<id> keeps serving art from a pack
	// the user just disowned.
	a.packMu.Lock()
	n := len(a.packPaths)
	a.packMu.Unlock()
	if n != 0 {
		t.Fatalf("%d ids still registered after clear", n)
	}
	// LoadCompanionPack must now report "no pack configured", which is the
	// branch the UI uses to show the set-up affordance (§5.11).
	if _, err := a.LoadCompanionPack(); err == nil {
		t.Fatal("LoadCompanionPack succeeded after a clear")
	}
	// Clearing twice is not an error.
	if err := a.ClearCompanionPack(); err != nil {
		t.Fatalf("second ClearCompanionPack: %v", err)
	}
}

// Opaque art must load cleanly. Every hosted image generator returns opaque
// images, so a warning here would fire on every generated pack — and it would
// describe a room floor and vignette that the sidebar host does not render.
func TestOpaqueArtDoesNotWarn(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.png"), []byte("bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":1,"canvas":{"w":8,"h":8,"scale":1},"alpha":false,
	  "slots":{"idle":[{"file":"a.png"}]}}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	p, _ := loadPack(dir)
	for _, w := range p.Warnings {
		if strings.Contains(w, "alpha") {
			t.Errorf("opaque art warned: %q", w)
		}
	}
}

// A variant may carry an optional animated `motion` file alongside its still
// `file` poster. loadPack must register it under a derived id
// (<variant>-strip's sibling, `<variant>-motion`) and recognise `motion` as a
// known manifest key.
func TestPackMotionRegisteredAndServed(t *testing.T) {
	dir := t.TempDir()
	// a valid still and a valid (any-bytes) webp; loadPack only checks
	// extension + existence + containment, not pixel validity.
	if err := os.WriteFile(filepath.Join(dir, "idle.png"), []byte("not really a png, but stable bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "idle.webp"), []byte("RIFF....WEBP"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":1,"name":"m","canvas":{"w":832,"h":1216},
	  "slots":{"idle":[{"id":"idle-a","file":"idle.png","motion":"idle.webp"}]}}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	p, ids := loadPack(dir)
	v := p.Slots["idle"][0]
	if v.MotionID != "idle-a-motion" {
		t.Fatalf("MotionID = %q, want idle-a-motion", v.MotionID)
	}
	got, ok := ids[v.MotionID]
	if !ok || filepath.Base(got) != "idle.webp" {
		t.Fatalf("motion id not registered to idle.webp: %q ok=%v", got, ok)
	}
	for _, w := range p.Warnings {
		if strings.Contains(w, "unknown key \"motion\"") {
			t.Fatalf("motion should be a known key, got warning: %s", w)
		}
	}
}
