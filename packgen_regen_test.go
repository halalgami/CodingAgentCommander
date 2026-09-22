package main

import (
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/imagegen"
)

// saveOnePack runs a full generation and saves it, so the regeneration tests
// start from a pack that really came out of this code rather than a fixture
// that only resembles one.
func saveOnePack(t *testing.T, f *packGenFixture, slots ...PackGenSlot) PackGenState {
	t.Helper()
	if len(slots) == 0 {
		slots = []PackGenSlot{{Slot: "idle"}, {Slot: "working"}, {Slot: "bored"}}
	}
	if _, err := f.app.StartPackGen(f.request(slots...)); err != nil {
		t.Fatal(err)
	}
	f.waitDone(t)
	got, err := f.app.SavePackGen()
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// failOnce is a provider reply that errors without producing an image.
func failOnce(*testing.T) (imagegen.Result, error) {
	return imagegen.Result{}, errors.New("simulated provider failure")
}

// failAfterBilling stands in for the failures that happen AFTER fal's submit
// returns 2xx — a result download that exhausts its retries, a response with no
// images, a body that will not decode. The image exists and is paid for; we
// just could not collect it.
func failAfterBilling(*testing.T) (imagegen.Result, error) {
	return imagegen.Result{Billed: true}, errors.New("fetch result: giving up after 3 attempts")
}

func regenRequest(f *packGenFixture, slots ...PackGenSlot) PackGenRequest {
	r := f.request(slots...)
	// f.request substitutes a default slot list when given none, which is right
	// for the happy path and wrong here: a test for "nothing selected" would
	// silently start a REAL run against the default slots. Assigning through
	// keeps the empty case empty.
	r.Slots = slots
	r.Regenerate = true
	r.BasePath = "" // must come from inside the pack
	// Address by PATH: name is a display string, not a location.
	if p, err := packGenPathsFor(r.Name); err == nil {
		r.PackPath = p.Live
	}
	return r
}

// The point of the whole feature: replacing one scene must not re-buy the rest.
func TestRegenerateOneSlotBillsForOneImage(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	before := f.prov.count()

	if _, err := f.app.StartPackGen(regenRequest(f, PackGenSlot{Slot: "working"})); err != nil {
		t.Fatal(err)
	}
	got := f.waitDone(t)

	if n := f.prov.count() - before; n != 1 {
		t.Errorf("regenerating one scene cost %d images, want 1", n)
	}
	if got.Billed != 1 || got.Total != 1 {
		t.Errorf("billed=%d total=%d, want 1/1", got.Billed, got.Total)
	}
}

// The untouched scenes must survive, and the replaced one must actually change.
func TestRegenerateReplacesOnlyTheRequestedSlot(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	paths, _ := packGenPathsFor("Test Pack")

	before, _ := loadPack(paths.Live)
	beforeIdle := before.Slots["idle"][0].ID
	beforeWorking := before.Slots["working"][0].ID

	if _, err := f.app.StartPackGen(regenRequest(f, PackGenSlot{Slot: "working"})); err != nil {
		t.Fatal(err)
	}
	f.waitDone(t)
	if _, err := f.app.SavePackGen(); err != nil {
		t.Fatal(err)
	}

	after, _ := loadPack(paths.Live)
	if len(after.Warnings) != 0 {
		t.Fatalf("regenerated pack has warnings:\n  %s", strings.Join(after.Warnings, "\n  "))
	}
	if after.Slots["idle"][0].ID != beforeIdle {
		t.Error("an untouched scene was replaced")
	}
	if after.Slots["working"][0].ID == beforeWorking {
		t.Error("the requested scene was not replaced")
	}
	// The decisive one: appending instead of replacing would leave BOTH images
	// in the slot, and the resolver would show the old one half the time.
	if n := len(after.Slots["working"]); n != 1 {
		t.Errorf("slot working has %d variants after regeneration, want 1 — the old image was kept alongside the new", n)
	}
	if len(after.Slots["bored"]) != 1 {
		t.Error("an untouched scene was lost")
	}
}

// A regeneration must not demand the idle image be re-bought just to fix
// another scene — which is the exact waste the feature exists to avoid.
func TestRegenerateDoesNotRequireIdle(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)

	if _, err := f.app.StartPackGen(regenRequest(f, PackGenSlot{Slot: "bored"})); err != nil {
		t.Fatalf("regenerating a non-idle scene was refused: %v", err)
	}
	f.waitDone(t)

	// The full-run validator still insists on it, because a NEW pack without
	// idle warns on every load.
	fresh := f.request(PackGenSlot{Slot: "bored"})
	if _, err := f.app.StartPackGen(fresh); err == nil {
		t.Error("a fresh run without idle should still be refused")
	}
}

// Every other rule still applies, or the two validators drift and the drift is
// only ever discovered by paying for art that cannot display.
func TestRegenerateStillRefusesImpossibleCombinations(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)

	cases := []struct {
		name  string
		slots []PackGenSlot
		want  string
	}{
		{"unreachable condition", []PackGenSlot{{Slot: "bored", When: "marathon"}}, "cannot display"},
		{"background slot", []PackGenSlot{{Slot: "bg_idle"}}, "not generated"},
		{"unknown slot", []PackGenSlot{{Slot: "dancing"}}, "unknown slot"},
		{"nothing selected", nil, "nothing selected"},
		{"two ambients on one slot", []PackGenSlot{
			{Slot: "bored", When: "lateNight"}, {Slot: "bored", When: "weekend"},
		}, "both be true at once"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.app.StartPackGen(regenRequest(f, tc.slots...))
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

// The base portrait is kept inside the pack so a scene can be regenerated
// later without the user finding the original file again — and if it has moved
// or been deleted, the pack would otherwise only ever be replaceable.
func TestGeneratedPackStoresItsBasePortrait(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	paths, _ := packGenPathsFor("Test Pack")

	if _, ok := findPackBase(paths.Live); !ok {
		t.Fatal("the saved pack has no stored base portrait")
	}
	// It must not become a variant: loadPack reads only manifest-referenced
	// files, so it should be invisible to the loader and never served.
	p, ids := loadPack(paths.Live)
	if len(p.Warnings) != 0 {
		t.Errorf("the stored base produced warnings:\n  %s", strings.Join(p.Warnings, "\n  "))
	}
	for id, path := range ids {
		if strings.Contains(filepath.Base(path), packGenBaseStem) {
			t.Errorf("the stored base portrait is being served as %s", id)
		}
	}
}

// Deleting the original base file must not stop a regeneration: the whole point
// of storing it is that the pack no longer depends on the user's filesystem.
func TestRegenerateWorksAfterTheOriginalBaseIsGone(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	if err := os.Remove(f.basePath); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.StartPackGen(regenRequest(f, PackGenSlot{Slot: "working"})); err != nil {
		t.Fatalf("regeneration needed the original file: %v", err)
	}
	if got := f.waitDone(t); got.Status != packGenDone {
		t.Errorf("status = %q (%s)", got.Status, got.Error)
	}
}

// A pack with no stored base cannot be regenerated from, and saying so beats
// offering a button that fails after the user presses it.
func TestRegenerateRefusesAPackWithNoStoredBase(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	paths, _ := packGenPathsFor("Test Pack")
	for _, ext := range []string{".png", ".jpg", ".jpeg"} {
		_ = os.Remove(filepath.Join(paths.Live, packGenBaseStem+ext))
	}

	_, err := f.app.StartPackGen(regenRequest(f, PackGenSlot{Slot: "working"}))
	if err == nil || !strings.Contains(err.Error(), "no stored base portrait") {
		t.Fatalf("got %v", err)
	}
	if browse, err := f.app.BrowseCompanionPack(); err != nil {
		t.Fatal(err)
	} else if browse.HasBase {
		t.Error("the browser still advertises regeneration for a pack that cannot do it")
	}
}

func TestRegenerateRefusesWhenNothingIsSaved(t *testing.T) {
	f := newPackGenFixture(t)
	_, err := f.app.StartPackGen(regenRequest(f, PackGenSlot{Slot: "working"}))
	if err == nil {
		t.Fatal("want an error when there is nothing to regenerate")
	}
	// Now refused at path resolution rather than later, which is both earlier
	// and more accurate — the folder genuinely is not there.
	if !strings.Contains(err.Error(), "no longer exists") &&
		!strings.Contains(err.Error(), "no saved pack") {
		t.Fatalf("refused for an unexpected reason: %v", err)
	}
}

// A failed regeneration must leave the saved pack untouched. It is the only
// operation that writes over art the user already has.
func TestFailedRegenerationLeavesTheSavedPackIntact(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	paths, _ := packGenPathsFor("Test Pack")
	before, _ := loadPack(paths.Live)

	f.prov.reply[f.prov.count()+1] = failOnce
	if _, err := f.app.StartPackGen(regenRequest(f, PackGenSlot{Slot: "working"})); err != nil {
		t.Fatal(err)
	}
	f.waitDone(t)

	after, _ := loadPack(paths.Live)
	if len(after.Warnings) != 0 {
		t.Fatalf("warnings: %s", strings.Join(after.Warnings, "; "))
	}
	if after.Slots["working"][0].ID != before.Slots["working"][0].ID {
		t.Error("a FAILED regeneration replaced the saved image")
	}
	if len(after.Slots) != len(before.Slots) {
		t.Error("a failed regeneration changed the saved pack's shape")
	}
}

// --- the browser ----------------------------------------------------------

func TestBrowseListsEveryVariantWithItsProvenance(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f, PackGenSlot{Slot: "idle"}, PackGenSlot{Slot: "bored"},
		PackGenSlot{Slot: "bored", When: "lateNight"})

	got, err := f.app.BrowseCompanionPack()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Variants) != 3 {
		t.Fatalf("got %d variants, want 3: %+v", len(got.Variants), got.Variants)
	}
	if !got.HasBase {
		t.Error("a generated pack must advertise that it can be regenerated")
	}
	for _, v := range got.Variants {
		if v.Preview == "" {
			t.Errorf("%s has no preview id, so the browser cannot show it", v.Slot)
		}
		if v.Prompt == "" || v.Model == "" || v.GeneratedAt == "" {
			t.Errorf("%s is missing provenance: %+v", v.Slot, v)
		}
	}
	// A conditioned variant must be distinguishable from its base sibling, or
	// regenerating one would silently target the other.
	var night int
	for _, v := range got.Variants {
		if v.Slot == "bored" && v.When == "lateNight" {
			night++
		}
	}
	if night != 1 {
		t.Errorf("the lateNight variant is not listed distinctly (found %d)", night)
	}
}

func TestBrowseWithNoPackConfigured(t *testing.T) {
	f := newPackGenFixture(t)
	if _, err := f.app.BrowseCompanionPack(); err == nil {
		t.Error("want an error when no pack is configured")
	}
}

// --- focus editing --------------------------------------------------------

// The cheap half of "the art is not centred": nudging a crop costs nothing
// where regenerating costs an image.
func TestSetPackVariantFocusPersistsAndReloads(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)

	if err := f.app.SetPackVariantFocus("working", "", "", 0.72, 0.18); err != nil {
		t.Fatal(err)
	}
	got, err := f.app.BrowseCompanionPack()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, v := range got.Variants {
		if v.Slot == "working" {
			found = true
			if v.FocusX != 0.72 || v.FocusY != 0.18 {
				t.Errorf("focus = %v/%v, want 0.72/0.18", v.FocusX, v.FocusY)
			}
		}
		if v.Slot == "idle" && (v.FocusX != 0.5 || v.FocusY != 0) {
			t.Errorf("an untouched variant's focus changed to %v/%v", v.FocusX, v.FocusY)
		}
	}
	if !found {
		t.Fatal("the working variant vanished")
	}
	// The pack must still load clean after being edited in place.
	paths, _ := packGenPathsFor("Test Pack")
	p, _ := loadPack(paths.Live)
	if len(p.Warnings) != 0 {
		t.Errorf("editing focus made the pack warn:\n  %s", strings.Join(p.Warnings, "\n  "))
	}
}

// A crop the user nudged belongs to the SLOT, not to the image that happened to
// be in it, so replacing the image must not silently re-centre it.
func TestRegenerationKeepsAnEditedFocus(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	if err := f.app.SetPackVariantFocus("working", "", "", 0.8, 0.25); err != nil {
		t.Fatal(err)
	}

	if _, err := f.app.StartPackGen(regenRequest(f, PackGenSlot{Slot: "working"})); err != nil {
		t.Fatal(err)
	}
	f.waitDone(t)
	if _, err := f.app.SavePackGen(); err != nil {
		t.Fatal(err)
	}

	got, err := f.app.BrowseCompanionPack()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range got.Variants {
		if v.Slot == "working" && (v.FocusX != 0.8 || v.FocusY != 0.25) {
			t.Errorf("regeneration reset the crop to %v/%v", v.FocusX, v.FocusY)
		}
	}
}

func TestSetPackVariantFocusRefusals(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)

	if err := f.app.SetPackVariantFocus("working", "", "", 1.5, 0); err == nil {
		t.Error("accepted a focus outside 0..1")
	}
	if err := f.app.SetPackVariantFocus("awaiting", "", "", 0.5, 0); err == nil {
		t.Error("accepted a slot that is not in the pack")
	}
	if err := f.app.SetPackVariantFocus("working", "lateNight", "", 0.5, 0); err == nil {
		t.Error("accepted a condition the pack does not have")
	}
}

// SetPackVariantFocus is the ONLY in-place write to a live pack, so a failure
// part-way through must leave the manifest exactly as it was. A half-written
// manifest does not load, and a pack that does not load takes its art with it.
func TestFocusWriteFailureLeavesTheManifestIntact(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	paths, _ := packGenPathsFor("Test Pack")
	manifest := filepath.Join(paths.Live, "manifest.json")

	before, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}

	orig := atomicRename
	atomicRename = func(string, string) error { return errors.New("simulated failure") }
	t.Cleanup(func() { atomicRename = orig })

	if err := f.app.SetPackVariantFocus("working", "", "", 0.7, 0.2); err == nil {
		t.Fatal("want an error")
	}
	after, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("a failed focus write modified the manifest")
	}
	// And it must not litter: a temp file left in a pack folder is a file the
	// user has to explain to themselves later.
	entries, err := os.ReadDir(paths.Live)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".manifest-") {
			t.Errorf("a failed write left %s behind", e.Name())
		}
	}
	// The pack still loads.
	if p, _ := loadPack(paths.Live); len(p.Warnings) != 0 {
		t.Errorf("warnings after a failed write: %s", strings.Join(p.Warnings, "; "))
	}
}

// A pack is a third-party archive and archives preserve symlinks, so copying
// one into staging must not follow it — the same reason resolvePackFile refuses
// to serve through one.
func TestCopyPackTreeSkipsSymlinks(t *testing.T) {
	src, dst := t.TempDir(), filepath.Join(t.TempDir(), "out")
	outside := filepath.Join(t.TempDir(), "secret.png")
	if err := os.WriteFile(outside, []byte("not part of the pack"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "idle.png"), testPNG(t, 8, 8, 1, false), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(src, "sneaky.png")); err != nil {
		t.Skip("this platform does not allow symlinks: " + err.Error())
	}

	if err := copyPackTree(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "idle.png")); err != nil {
		t.Errorf("a real file was not copied: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "sneaky.png")); err == nil {
		t.Error("a symlink was copied into the staged pack, pulling in a file from outside it")
	}
}

// --- the pack library -----------------------------------------------------

func TestListCompanionPacks(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)

	// A second pack, written directly so the test does not depend on the run
	// slot being free.
	second := filepath.Join(companionPacksDir(), "another")
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := writePackManifest(second, "Another", []packGenArt{
		{Slot: "idle", Image: testPNG(t, 12, 20, 44, false)},
	}); err != nil {
		t.Fatal(err)
	}

	got := f.app.ListCompanionPacks()
	if len(got) != 2 {
		t.Fatalf("got %d packs, want 2: %+v", len(got), got)
	}
	// Sorted by name, so the list does not reshuffle between calls.
	if got[0].Name != "Another" || got[1].Name != "Test Pack" {
		t.Errorf("order = %q, %q", got[0].Name, got[1].Name)
	}

	var activeCount int
	for _, p := range got {
		if p.Active {
			activeCount++
		}
		if p.Thumb == "" {
			t.Errorf("%s has no thumbnail", p.Name)
		}
		if p.Scenes == 0 {
			t.Errorf("%s reports no scenes", p.Name)
		}
	}
	if activeCount != 1 {
		t.Errorf("%d packs marked active, want exactly 1", activeCount)
	}
	// Only the generated one carries a stored portrait.
	for _, p := range got {
		if p.Name == "Test Pack" && !p.HasBase {
			t.Error("the generated pack should be regeneratable")
		}
		if p.Name == "Another" && p.HasBase {
			t.Error("a hand-written pack has no stored portrait")
		}
	}
}

// A run's transient folders are not packs. Listing them would offer the user a
// half-written pack to select.
func TestListSkipsStagingAndBackupFolders(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	paths, _ := packGenPathsFor("Test Pack")

	for _, dir := range []string{paths.Staging, paths.Old} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := writePackManifest(dir, "Transient", []packGenArt{
			{Slot: "idle", Image: testPNG(t, 8, 8, 7, false)},
		}); err != nil {
			t.Fatal(err)
		}
	}

	for _, p := range f.app.ListCompanionPacks() {
		if strings.HasSuffix(p.Folder, packGenStagingSuffix) || strings.HasSuffix(p.Folder, packGenOldSuffix) {
			t.Errorf("the library lists the transient folder %q", p.Folder)
		}
	}
}

// A folder without a manifest is not a pack, and must not appear as a broken
// entry the user can select.
func TestListSkipsFoldersWithoutAManifest(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	if err := os.MkdirAll(filepath.Join(companionPacksDir(), "notapack"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range f.app.ListCompanionPacks() {
		if p.Folder == "notapack" {
			t.Error("a folder with no manifest was listed as a pack")
		}
	}
}

// Thumbnails must keep resolving after another pack is selected. packPaths is
// replaced wholesale on every load, which is exactly why the library has its
// own id map.
func TestLibraryThumbnailsSurviveSelectingAPack(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	second := filepath.Join(companionPacksDir(), "another")
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := writePackManifest(second, "Another", []packGenArt{
		{Slot: "idle", Image: testPNG(t, 12, 20, 44, false)},
	}); err != nil {
		t.Fatal(err)
	}

	list := f.app.ListCompanionPacks()
	var otherThumb string
	for _, p := range list {
		if !p.Active {
			otherThumb = p.Thumb
		}
	}
	if otherThumb == "" {
		t.Fatal("no inactive pack to check")
	}

	srv := httptest.NewServer(f.app.packMediaHandler())
	defer srv.Close()
	fetch := func() int {
		res, err := srv.Client().Get(srv.URL + mediaPackPrefix + otherThumb)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := fetch(); code != 200 {
		t.Fatalf("an inactive pack's thumbnail returned %d", code)
	}
	if _, err := f.app.SelectCompanionPack(second); err != nil {
		t.Fatal(err)
	}
	if code := fetch(); code != 200 {
		t.Errorf("selecting a pack dropped the library thumbnails (%d)", code)
	}
}

func TestSelectCompanionPackSwitchesTheActiveOne(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	second := filepath.Join(companionPacksDir(), "another")
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := writePackManifest(second, "Another", []packGenArt{
		{Slot: "idle", Image: testPNG(t, 12, 20, 44, false)},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := f.app.SelectCompanionPack(second); err != nil {
		t.Fatal(err)
	}
	if got := f.app.GetCompanionConfig().PackPath; got != second {
		t.Errorf("active pack = %q, want %q", got, second)
	}
	for _, p := range f.app.ListCompanionPacks() {
		if p.Active != (p.Path == second) {
			t.Errorf("%s active=%v", p.Name, p.Active)
		}
	}
	// The browser must now describe the newly selected pack.
	browse, err := f.app.BrowseCompanionPack()
	if err != nil {
		t.Fatal(err)
	}
	if browse.Name != "Another" {
		t.Errorf("browser still shows %q", browse.Name)
	}
}

// --- deleting a pack -------------------------------------------------------

// The only destructive operation on saved art, so the containment check is the
// test that matters. Lexical comparison is not enough: a symlink inside the
// packs folder pointing anywhere would pass it and be handed to RemoveAll.
func TestDeleteRefusesAnythingOutsideThePacksFolder(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	root := companionPacksDir()

	outside := t.TempDir()
	precious := filepath.Join(outside, "precious.txt")
	if err := os.WriteFile(precious, []byte("do not delete"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct{ name, path string }{
		{"an unrelated directory", outside},
		{"the packs folder itself", root},
		{"a traversal out of the packs folder", filepath.Join(root, "..", filepath.Base(outside))},
		{"a nested path", filepath.Join(root, "test-pack", "deeper")},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := f.app.DeleteCompanionPack(tc.path); err == nil {
				t.Fatalf("deleted %q", tc.path)
			}
		})
	}
	if _, err := os.Stat(precious); err != nil {
		t.Error("a file outside the packs folder was destroyed")
	}
	if _, err := os.Stat(root); err != nil {
		t.Error("the packs folder itself was destroyed")
	}
}

// A symlink inside the packs folder is the case a string comparison misses.
func TestDeleteRefusesASymlinkedPack(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(companionPacksDir(), "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("this platform does not allow symlinks: " + err.Error())
	}

	if err := f.app.DeleteCompanionPack(link); err == nil {
		t.Error("followed a symlink out of the packs folder and deleted through it")
	}
	if _, err := os.Stat(filepath.Join(outside, "manifest.json")); err != nil {
		t.Error("the symlink target was destroyed")
	}
}

// A folder without a manifest is something the user put there. Deleting it is
// not ours to do.
func TestDeleteRefusesAFolderThatIsNotAPack(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	notAPack := filepath.Join(companionPacksDir(), "my-reference-images")
	if err := os.MkdirAll(notAPack, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notAPack, "a.png"), testPNG(t, 8, 8, 1, false), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.app.DeleteCompanionPack(notAPack); err == nil {
		t.Error("deleted a folder with no manifest")
	}
	if _, err := os.Stat(notAPack); err != nil {
		t.Error("it was removed anyway")
	}
}

func TestDeleteRemovesTheRequestedPackAndNothingElse(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	keep := filepath.Join(companionPacksDir(), "keeper")
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := writePackManifest(keep, "Keeper", []packGenArt{
		{Slot: "idle", Image: testPNG(t, 10, 14, 21, false)},
	}); err != nil {
		t.Fatal(err)
	}
	paths, _ := packGenPathsFor("Test Pack")

	if err := f.app.DeleteCompanionPack(paths.Live); err != nil {
		t.Fatal(err)
	}
	if packGenIsDir(paths.Live) {
		t.Error("the pack was not removed")
	}
	if !packGenIsDir(keep) {
		t.Error("an unrelated pack was removed")
	}
	if got := f.app.ListCompanionPacks(); len(got) != 1 || got[0].Folder != "keeper" {
		t.Errorf("library = %+v, want just the keeper", got)
	}
}

// Deleting the ACTIVE pack must disown it too, or the config points at a folder
// that no longer exists and the figure keeps trying to serve from it.
func TestDeletingTheActivePackDeselectsIt(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	paths, _ := packGenPathsFor("Test Pack")
	if got := f.app.GetCompanionConfig().PackPath; got != paths.Live {
		t.Fatalf("fixture did not leave the pack active: %q", got)
	}

	if err := f.app.DeleteCompanionPack(paths.Live); err != nil {
		t.Fatal(err)
	}
	if got := f.app.GetCompanionConfig().PackPath; got != "" {
		t.Errorf("config still points at the deleted pack: %q", got)
	}
	// And the browser must report the absence rather than erroring on a
	// missing folder.
	if _, err := f.app.BrowseCompanionPack(); err == nil {
		t.Error("the browser still claims a pack is configured")
	}
}

func TestDeleteIsNotIdempotentlySilent(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	paths, _ := packGenPathsFor("Test Pack")
	if err := f.app.DeleteCompanionPack(paths.Live); err != nil {
		t.Fatal(err)
	}
	// Deleting it again must SAY it is gone rather than report success — a
	// silent no-op here reads to the caller as "deleted", which is how a UI
	// ends up claiming it removed something that was never there.
	if err := f.app.DeleteCompanionPack(paths.Live); err == nil {
		t.Error("deleting a nonexistent pack reported success")
	}
}

// --- addressing a pack by path, not by name --------------------------------

// The folder is slug(name) only for packs this app generated. An imported pack
// keeps the manifest's name while taking its folder from the archive filename,
// so regenerating one scene of it BY NAME resolved to a different pack and
// rewrote that one instead.
func TestRegenerateTargetsTheFolderNotTheDisplayName(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f) // packs/test-pack, manifest name "Test Pack"
	victim, _ := packGenPathsFor("Test Pack")
	before, _ := loadPack(victim.Live)

	// A SECOND pack in a differently-named folder, whose manifest claims the
	// same display name — exactly what importing a same-named pack produces.
	twin := filepath.Join(companionPacksDir(), "test-pack-2")
	if err := os.MkdirAll(twin, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := writePackManifest(twin, "Test Pack", []packGenArt{
		{Slot: "idle", Image: testPNG(t, 16, 22, 90, false)},
		{Slot: "working", Image: testPNG(t, 16, 22, 91, false)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writePackBase(twin, testPNG(t, 20, 28, 60, false)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.SelectCompanionPack(twin); err != nil {
		t.Fatal(err)
	}
	twinBefore, _ := loadPack(twin)

	req := regenRequest(f, PackGenSlot{Slot: "working"})
	req.Name = "Test Pack" // the display name both packs share
	req.PackPath = twin    // the one actually being browsed
	if _, err := f.app.StartPackGen(req); err != nil {
		t.Fatal(err)
	}
	f.waitDone(t)
	if _, err := f.app.SavePackGen(); err != nil {
		t.Fatal(err)
	}

	// The pack that was NOT being browsed must be untouched.
	after, _ := loadPack(victim.Live)
	if after.Slots["working"][0].ID != before.Slots["working"][0].ID {
		t.Error("regeneration rewrote a different pack that happened to share a display name")
	}
	// And the one that WAS must have changed.
	twinAfter, _ := loadPack(twin)
	if twinAfter.Slots["working"][0].ID == twinBefore.Slots["working"][0].ID {
		t.Error("the browsed pack was not regenerated")
	}
}

func TestRegenerateRefusesAPathOutsideThePacksFolder(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)

	// A FULLY VALID pack, just in the wrong place: manifest, art and a stored
	// base. Anything less and the refusal could come from a missing base rather
	// than from containment, and the test would pass without the guard.
	outside := t.TempDir()
	if _, err := writePackManifest(outside, "Elsewhere", []packGenArt{
		{Slot: "idle", Image: testPNG(t, 12, 16, 33, false)},
		{Slot: "working", Image: testPNG(t, 12, 16, 34, false)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writePackBase(outside, testPNG(t, 12, 16, 35, false)); err != nil {
		t.Fatal(err)
	}

	req := regenRequest(f, PackGenSlot{Slot: "working"})
	req.PackPath = outside
	_, err := f.app.StartPackGen(req)
	if err == nil {
		t.Fatal("accepted a pack path outside the packs folder")
	}
	// The SPECIFIC refusal, so this cannot pass because of some later failure.
	if !strings.Contains(err.Error(), "is not a pack in") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
	// And nothing was written near it.
	if packGenIsDir(outside + packGenStagingSuffix) {
		t.Error("a staging folder was created beside a rejected pack")
	}

	req.PackPath = ""
	if _, err := f.app.StartPackGen(req); err == nil {
		t.Fatal("accepted a regeneration with no pack given")
	}
}

// --- not silently replacing an existing pack -------------------------------

// promotePack deletes the pack it replaces, and saveRun leaves the name
// pre-filled — so the obvious second run landed on the first pack's folder and
// destroyed it without asking. This is the same protection importPackZip
// already had and generation did not.
func TestGeneratingOverAnExistingPackIsRefusedByDefault(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	paths, _ := packGenPathsFor("Test Pack")
	before, _ := loadPack(paths.Live)

	_, err := f.app.StartPackGen(f.request(PackGenSlot{Slot: "idle"}))
	if err == nil {
		t.Fatal("a second run replaced an existing pack without asking")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("unhelpful error: %v", err)
	}
	// The refusal must cost nothing and change nothing.
	if f.prov.count() != 3 {
		t.Errorf("the refused run reached the provider (%d calls)", f.prov.count())
	}
	after, _ := loadPack(paths.Live)
	if len(after.Slots) != len(before.Slots) {
		t.Error("the existing pack changed")
	}
}

// Names that differ only in case, spacing or punctuation slug onto the same
// folder, so the collision check has to be on the SLUG, not the name.
func TestOverwriteRefusalIsBySlugNotByDisplayName(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	for _, name := range []string{"Test Pack", "  TEST PACK!  ", "test-pack", "Test  Pack"} {
		req := f.request(PackGenSlot{Slot: "idle"})
		req.Name = name
		if _, err := f.app.StartPackGen(req); err == nil {
			t.Errorf("%q was allowed to replace the existing pack", name)
		}
	}
}

// The user can still replace a pack — they just have to mean it.
func TestOverwriteIsAllowedWhenConfirmed(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	paths, _ := packGenPathsFor("Test Pack")
	before, _ := loadPack(paths.Live)

	req := f.request(PackGenSlot{Slot: "idle"})
	req.Overwrite = true
	if _, err := f.app.StartPackGen(req); err != nil {
		t.Fatal(err)
	}
	f.waitDone(t)
	if _, err := f.app.SavePackGen(); err != nil {
		t.Fatal(err)
	}
	after, _ := loadPack(paths.Live)
	if after.Slots["idle"][0].ID == before.Slots["idle"][0].ID {
		t.Error("the confirmed replacement did not happen")
	}
}

// loadPack calls several variants per (slot, when) "the normal authoring
// pattern", and the browser renders each as its own row. Keying the focus edit
// on `when` alone moved a DIFFERENT row than the one the user dragged — and
// silently, since the row they touched simply did not change.
func TestFocusEditsTheVariantTheUserActuallyPicked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	pack := filepath.Join(companionPacksDir(), "hand-made")
	if err := os.MkdirAll(pack, 0o755); err != nil {
		t.Fatal(err)
	}
	// Two variants of the SAME slot and condition, distinguished only by mood.
	if _, err := writePackManifest(pack, "Hand made", []packGenArt{
		{Slot: "idle", Mood: "happy", Image: testPNG(t, 10, 14, 11, false)},
		{Slot: "idle", Mood: "sad", Image: testPNG(t, 10, 14, 12, false)},
	}); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	if _, err := a.setCompanionPack(pack); err != nil {
		t.Fatal(err)
	}

	if err := a.SetPackVariantFocus("idle", "", "sad", 0.9, 0.1); err != nil {
		t.Fatal(err)
	}
	got, err := a.BrowseCompanionPack()
	if err != nil {
		t.Fatal(err)
	}
	var checked int
	for _, v := range got.Variants {
		switch v.Mood {
		case "sad":
			checked++
			if v.FocusX != 0.9 || v.FocusY != 0.1 {
				t.Errorf("the variant the user picked was not moved: %v/%v", v.FocusX, v.FocusY)
			}
		case "happy":
			checked++
			if v.FocusX != 0.5 || v.FocusY != 0 {
				t.Errorf("a variant the user did not touch moved to %v/%v", v.FocusX, v.FocusY)
			}
		}
	}
	if checked != 2 {
		t.Fatalf("expected both variants in the browse output, saw %d", checked)
	}
}

// loadPack never errors, so a configured-but-missing folder used to produce a
// perfectly valid struct with a NIL variants slice — which marshals to `null`
// and made the browse tab throw on `saved.variants.length`. The user's real
// scenario: they moved or renamed the pack folder in Finder.
func TestBrowseReportsAMissingFolderRatherThanReturningNothing(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	paths, _ := packGenPathsFor("Test Pack")

	// Renamed outside the app — the config still points at the old path.
	if err := os.Rename(paths.Live, paths.Live+"-moved"); err != nil {
		t.Fatal(err)
	}

	got, err := f.app.BrowseCompanionPack()
	if err != nil {
		t.Fatalf("browsing a missing pack errored instead of describing it: %v", err)
	}
	if !got.Missing {
		t.Error("the folder is gone and the result does not say so")
	}
	if got.Variants == nil {
		t.Error("variants is nil; it marshals to null and the UI reads .length on it")
	}
	if got.Warnings == nil {
		t.Error("warnings is nil for the same reason")
	}
}

// The slices must be non-nil on the ordinary path too, or the guard only holds
// for the failure case it was written for.
func TestBrowseNeverReturnsNilSlices(t *testing.T) {
	f := newPackGenFixture(t)
	saveOnePack(t, f)
	got, err := f.app.BrowseCompanionPack()
	if err != nil {
		t.Fatal(err)
	}
	if got.Variants == nil || got.Warnings == nil {
		t.Errorf("nil slice on a healthy pack: variants=%v warnings=%v",
			got.Variants == nil, got.Warnings == nil)
	}
	if got.Missing {
		t.Error("a present pack was reported missing")
	}
}
