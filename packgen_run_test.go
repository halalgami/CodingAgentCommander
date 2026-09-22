package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/imagegen"
)

// --- fake provider ---------------------------------------------------------

// fakeImageProvider is installed through the packGenLookup seam rather than
// through imagegen.Register, which panics on a duplicate id by design. A
// package-level registry cannot be populated per-test without that panic taking
// down the whole binary on the second test that tries.
type fakeImageProvider struct {
	mu     sync.Mutex
	calls  int
	blockN int           // if >0, the Nth call (1-based) blocks until released
	block  chan struct{} // closed to release a blocked call
	// reply is consulted per call index (1-based); a missing entry is a plain
	// success with distinct bytes.
	reply map[int]func(t *testing.T) (imagegen.Result, error)
	t     *testing.T
	seen  []imagegen.Request
}

func newFakeProvider(t *testing.T) *fakeImageProvider {
	return &fakeImageProvider{t: t, reply: map[int]func(*testing.T) (imagegen.Result, error){}, block: make(chan struct{})}
}

func (f *fakeImageProvider) ID() string { return "fake" }
func (f *fakeImageProvider) Models() []imagegen.Model {
	return []imagegen.Model{{ID: "fake/model", Label: "Fake", Mode: imagegen.ModeEdit}}
}

func (f *fakeImageProvider) Generate(ctx context.Context, key string, r imagegen.Request) (imagegen.Result, error) {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.seen = append(f.seen, r)
	fn := f.reply[n]
	blocks := f.blockN == n
	f.mu.Unlock()

	if blocks {
		select {
		case <-f.block:
		case <-ctx.Done():
			return imagegen.Result{}, ctx.Err()
		}
	}
	if ctx.Err() != nil {
		return imagegen.Result{}, ctx.Err()
	}
	if fn != nil {
		return fn(f.t)
	}
	// Distinct bytes per call: content ids are content hashes, so identical
	// bytes would be deduplicated and the test would measure the wrong thing.
	//
	// Billed mirrors the real provider's contract: it reports what the PROVIDER
	// charged, not whether the caller got a usable image. A fake that left it
	// false on success — or true only when there is an image — would restore
	// the "error means free" model that under-reported spend.
	return imagegen.Result{
		Image: testPNG(f.t, 24, 40, uint8(100+n), false),
		MIME:  "image/png", Seed: int64(n), Model: "fake/model", Billed: true,
	}, nil
}

func (f *fakeImageProvider) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// --- harness ---------------------------------------------------------------

type packGenFixture struct {
	app      *App
	prov     *fakeImageProvider
	emitter  *fakeEmitter
	basePath string
}

func newPackGenFixture(t *testing.T) *packGenFixture {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	if err := os.MkdirAll(companionPacksDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "base.png")
	if err := os.WriteFile(base, testPNG(t, 32, 48, 7, false), 0o644); err != nil {
		t.Fatal(err)
	}

	prov := newFakeProvider(t)
	origLookup, origKey := packGenLookup, packGenLoadKey
	packGenLookup = func(id string) (imagegen.Provider, bool) {
		if id == "fake" {
			return prov, true
		}
		return nil, false
	}
	packGenLoadKey = func(string) string { return "test-key" }
	t.Cleanup(func() { packGenLookup, packGenLoadKey = origLookup, origKey })

	a := NewApp()
	em := &fakeEmitter{}
	a.emitter = em
	return &packGenFixture{app: a, prov: prov, emitter: em, basePath: base}
}

func (f *packGenFixture) request(slots ...PackGenSlot) PackGenRequest {
	if len(slots) == 0 {
		slots = []PackGenSlot{{Slot: "idle"}, {Slot: "working"}}
	}
	return PackGenRequest{
		Name: "Test Pack", ProviderID: "fake", Model: "fake/model",
		StyleID: "whimsical", BasePath: f.basePath, Slots: slots,
	}
}

// waitFor polls until cond holds or the deadline passes. The run is a
// goroutine, so there is no completion channel to wait on from a binding's
// point of view — which is exactly how the UI sees it too.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func (f *packGenFixture) waitDone(t *testing.T) PackGenState {
	t.Helper()
	waitFor(t, "the run to finish", func() bool {
		return f.app.PackGenStatus().Status != packGenRunning
	})
	return f.app.PackGenStatus()
}

// --- happy path ------------------------------------------------------------

func TestPackGenHappyPath(t *testing.T) {
	f := newPackGenFixture(t)
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	got := f.waitDone(t)

	if got.Status != packGenDone {
		t.Fatalf("status = %q (%s)", got.Status, got.Error)
	}
	if got.Done != 2 || got.Billed != 2 || got.Total != 2 {
		t.Errorf("done=%d billed=%d total=%d, want 2/2/2", got.Done, got.Billed, got.Total)
	}
	for _, it := range got.Items {
		if it.Status != "done" {
			t.Errorf("slot %q: status %q (%s)", it.Slot, it.Status, it.Error)
		}
		if it.Preview == "" {
			t.Errorf("slot %q has no preview id", it.Slot)
		}
	}

	// Staged, not live: nothing reaches the packs folder until the user saves.
	paths, _ := packGenPathsFor("Test Pack")
	if !packGenIsDir(paths.Staging) {
		t.Fatal("no staging folder")
	}
	if packGenIsDir(paths.Live) {
		t.Error("the run wrote into the live pack folder before being saved")
	}
	p, _ := loadPack(paths.Staging)
	if len(p.Warnings) != 0 {
		t.Errorf("staged pack has warnings:\n  %s", strings.Join(p.Warnings, "\n  "))
	}
}

// The prompt the provider receives must carry the locks. This asserts the
// composed prompt actually reaches the wire rather than being rebuilt or
// dropped somewhere in the run.
func TestPackGenSendsTheComposedPrompt(t *testing.T) {
	f := newPackGenFixture(t)
	if _, err := f.app.StartPackGen(f.request(PackGenSlot{Slot: "idle"})); err != nil {
		t.Fatal(err)
	}
	f.waitDone(t)

	f.prov.mu.Lock()
	defer f.prov.mu.Unlock()
	if len(f.prov.seen) != 1 {
		t.Fatalf("provider saw %d requests", len(f.prov.seen))
	}
	r := f.prov.seen[0]
	if r.Mode != imagegen.ModeEdit {
		t.Errorf("mode = %q; the spike settled on instruction editing", r.Mode)
	}
	if !r.Portrait {
		t.Error("portrait not requested")
	}
	if len(r.Base) == 0 {
		t.Error("no base image sent; every slot must derive from the base (star topology)")
	}
	style, _ := packGenStyleByID("whimsical")
	for _, want := range []string{packGenIdentityLock, packGenFramingLock, style.Text, packGenSlotStaging["idle"]} {
		if !strings.Contains(r.Prompt, want) {
			t.Errorf("prompt is missing a lock:\n%s", r.Prompt)
		}
	}
}

// --- billing ---------------------------------------------------------------

// An all-black frame is a silent safety rejection: HTTP 200, real bytes, a real
// charge. Booking it as unbilled would tell the user they spent less than they
// did — the whole point of showing a spend figure.
func TestBlackImageIsCountedAsBilled(t *testing.T) {
	f := newPackGenFixture(t)
	f.prov.reply[1] = func(*testing.T) (imagegen.Result, error) {
		// A black frame is HTTP 200 with real bytes: generated, and charged.
		return imagegen.Result{Billed: true}, fmt.Errorf("generate: %w", imagegen.ErrBlackImage)
	}
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	got := f.waitDone(t)

	if got.Billed != 2 {
		t.Errorf("billed = %d, want 2 — the black frame was still charged for", got.Billed)
	}
	if got.Done != 1 {
		t.Errorf("done = %d, want 1", got.Done)
	}
	if got.Items[0].Status != "failed" {
		t.Errorf("the black frame was kept: %q", got.Items[0].Status)
	}
	// A partial pack is legal and must still be savable.
	if got.Status != packGenDone {
		t.Errorf("status = %q; one bad slot must not fail the run", got.Status)
	}
}

// A transport or auth failure produced no image, so it must NOT be billed.
func TestTransportFailureIsNotBilled(t *testing.T) {
	f := newPackGenFixture(t)
	f.prov.reply[1] = func(*testing.T) (imagegen.Result, error) {
		// Fails BEFORE the submit lands, so nothing was generated or charged.
		return imagegen.Result{}, errors.New("dial tcp: connection refused")
	}
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	got := f.waitDone(t)
	if got.Billed != 1 {
		t.Errorf("billed = %d, want 1 — a failed connection produced no image", got.Billed)
	}
}

// Moderation returns real bytes the caller must not keep, but the provider
// still charged for producing them.
func TestContentFilteredIsBilledAndNotKept(t *testing.T) {
	f := newPackGenFixture(t)
	f.prov.reply[1] = func(t *testing.T) (imagegen.Result, error) {
		return imagegen.Result{
			Image: testPNG(t, 8, 8, 9, false), MIME: "image/png",
			FinishReason: imagegen.FinishContentFiltered, Billed: true,
		}, nil
	}
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	got := f.waitDone(t)

	if got.Billed != 2 {
		t.Errorf("billed = %d, want 2", got.Billed)
	}
	if got.Items[0].Status != "failed" || !strings.Contains(got.Items[0].Error, "flagged") {
		t.Errorf("flagged image not refused: %+v", got.Items[0])
	}
	paths, _ := packGenPathsFor("Test Pack")
	p, _ := loadPack(paths.Staging)
	if len(p.Slots["idle"]) != 0 {
		t.Error("flagged bytes were written into the pack")
	}
}

// A 200 carrying an HTML error page must not become a .png the webview renders
// as a broken image with nothing saying why.
func TestNonImageResponseIsRefused(t *testing.T) {
	f := newPackGenFixture(t)
	f.prov.reply[1] = func(*testing.T) (imagegen.Result, error) {
		return imagegen.Result{Image: []byte("<html>402 Payment Required</html>"),
			MIME: "image/png", Billed: true}, nil
	}
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	got := f.waitDone(t)
	if got.Items[0].Status != "failed" {
		t.Errorf("non-image accepted: %+v", got.Items[0])
	}
	if got.Done != 1 {
		t.Errorf("done = %d, want 1", got.Done)
	}
}

// --- the single run slot ---------------------------------------------------

// Two concurrent starts must produce exactly one run. A check-then-claim would
// let both pass, and the loser's goroutine would bill images against a run that
// no binding can cancel, save or even see.
func TestConcurrentStartsClaimTheSlotExactlyOnce(t *testing.T) {
	f := newPackGenFixture(t)
	f.prov.blockN = 1 // hold the first call so both starts race on a live run

	const n = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	var okCount int
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.app.StartPackGen(f.request()); err == nil {
				mu.Lock()
				okCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if okCount != 1 {
		t.Fatalf("%d starts succeeded, want exactly 1", okCount)
	}
	close(f.prov.block)
	got := f.waitDone(t)

	// The decisive assertion: only ONE run's worth of images was paid for.
	if f.prov.count() != 2 {
		t.Errorf("provider called %d times, want 2 — a losing start billed images anyway", f.prov.count())
	}
	if got.Billed != 2 {
		t.Errorf("billed = %d, want 2", got.Billed)
	}
}

// Art that has been generated but not saved was paid for. A new run must wait
// rather than clear it: the alternative is deleting images the user bought to
// make room for images they are about to buy.
func TestUnsavedRunBlocksTheNextOne(t *testing.T) {
	f := newPackGenFixture(t)
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	f.waitDone(t)

	_, err := f.app.StartPackGen(f.request())
	if err == nil {
		t.Fatal("a second run started over unsaved art")
	}
	if !strings.Contains(err.Error(), "unsaved") {
		t.Errorf("unhelpful error: %v", err)
	}
	if f.prov.count() != 2 {
		t.Errorf("provider called %d times; the blocked run spent money", f.prov.count())
	}

	// Discarding releases the slot.
	if err := f.app.DiscardPackGen(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatalf("the slot was not released by discard: %v", err)
	}
	f.waitDone(t)
}

// Every rejection must happen BEFORE the slot is claimed, or a bad request
// would lock out good ones.
func TestRejectedRequestsNeverClaimTheSlot(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*PackGenRequest)
		want string
	}{
		{"unusable name", func(r *PackGenRequest) { r.Name = "..." }, "no letters or digits"},
		{"no idle slot", func(r *PackGenRequest) { r.Slots = []PackGenSlot{{Slot: "working"}} }, "idle slot is required"},
		{"unknown style", func(r *PackGenRequest) { r.StyleID = "nope" }, "unknown style"},
		{"missing base", func(r *PackGenRequest) { r.BasePath = "/nonexistent/base.png" }, "reading the base portrait"},
		{"unknown provider", func(r *PackGenRequest) { r.ProviderID = "nope" }, "unknown image provider"},
		{"model the provider does not offer", func(r *PackGenRequest) { r.Model = "other/model" }, "does not offer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPackGenFixture(t)
			req := f.request()
			tc.mut(&req)

			_, err := f.app.StartPackGen(req)
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
			if f.app.PackGenStatus().ID != "" {
				t.Error("a rejected request claimed the run slot")
			}
			if f.prov.count() != 0 {
				t.Error("a rejected request reached the provider")
			}
			// The slot must still be usable.
			if _, err := f.app.StartPackGen(f.request()); err != nil {
				t.Fatalf("the slot was locked by a rejected request: %v", err)
			}
			f.waitDone(t)
		})
	}
}

// A base portrait that is not a decodable image must be refused before any
// money is spent, not after six failed generations.
func TestBaseImageIsValidatedBeforeSpending(t *testing.T) {
	f := newPackGenFixture(t)
	bad := filepath.Join(t.TempDir(), "bad.png")
	if err := os.WriteFile(bad, []byte("this is not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := f.request()
	req.BasePath = bad

	_, err := f.app.StartPackGen(req)
	if err == nil || !strings.Contains(err.Error(), "could not be read as an image") {
		t.Fatalf("got %v", err)
	}
	if f.prov.count() != 0 {
		t.Error("money was spent on a run with an unusable base image")
	}
}

// A missing key must be caught up front. The provider would reject every call,
// and on a paid endpoint an auth failure per slot is six wasted round trips.
func TestMissingKeyIsRefusedUpFront(t *testing.T) {
	f := newPackGenFixture(t)
	orig := packGenLoadKey
	packGenLoadKey = func(string) string { return "" }
	t.Cleanup(func() { packGenLoadKey = orig })

	_, err := f.app.StartPackGen(f.request())
	if err == nil || !strings.Contains(err.Error(), "no API key") {
		t.Fatalf("got %v", err)
	}
	if f.prov.count() != 0 {
		t.Error("a keyless run reached the provider")
	}
}

// --- cancellation ----------------------------------------------------------

// Cancelling stops the remaining SPEND. It must not throw away art already
// paid for.
func TestCancelKeepsWhatWasAlreadyGenerated(t *testing.T) {
	f := newPackGenFixture(t)
	f.prov.blockN = 2 // let slot 1 land, then hold slot 2

	slots := []PackGenSlot{{Slot: "idle"}, {Slot: "working"}, {Slot: "done"}}
	if _, err := f.app.StartPackGen(f.request(slots...)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the first image to land", func() bool {
		return f.app.PackGenStatus().Done >= 1
	})

	f.app.CancelPackGen()
	got := f.waitDone(t)

	if got.Status != packGenCancelled {
		t.Fatalf("status = %q", got.Status)
	}
	if got.Done != 1 {
		t.Errorf("done = %d, want 1", got.Done)
	}
	if f.prov.count() > 2 {
		t.Errorf("provider called %d times; cancellation did not stop the spend", f.prov.count())
	}
	if got.Items[2].Status != "skipped" {
		t.Errorf("the untouched slot is %q, want skipped", got.Items[2].Status)
	}

	// The paid-for image must still be on disk and loadable.
	paths, _ := packGenPathsFor("Test Pack")
	p, _ := loadPack(paths.Staging)
	if len(p.Slots["idle"]) != 1 {
		t.Fatal("cancellation destroyed art that was already paid for")
	}
	if len(p.Warnings) != 0 {
		t.Errorf("the partial pack does not load cleanly:\n  %s", strings.Join(p.Warnings, "\n  "))
	}
}

// Cancelling with nothing running must not panic or invent a run.
func TestCancelWithNoRun(t *testing.T) {
	f := newPackGenFixture(t)
	if got := f.app.CancelPackGen(); got.ID != "" {
		t.Errorf("got a run from nowhere: %+v", got)
	}
	if err := f.app.DiscardPackGen(); err != nil {
		t.Errorf("discarding nothing errored: %v", err)
	}
}

// --- incremental durability ------------------------------------------------

// The staging folder must LOAD after every image, not only at the end, so a
// crash mid-run leaves a usable partial pack rather than orphaned bytes.
func TestStagingLoadsAfterEachImage(t *testing.T) {
	f := newPackGenFixture(t)
	f.prov.blockN = 3
	slots := []PackGenSlot{{Slot: "idle"}, {Slot: "working"}, {Slot: "done"}}
	if _, err := f.app.StartPackGen(f.request(slots...)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "two images to land", func() bool { return f.app.PackGenStatus().Done >= 2 })

	paths, _ := packGenPathsFor("Test Pack")
	p, _ := loadPack(paths.Staging)
	if len(p.Warnings) != 0 {
		t.Errorf("mid-run pack has warnings:\n  %s", strings.Join(p.Warnings, "\n  "))
	}
	if len(p.Slots["idle"]) != 1 || len(p.Slots["working"]) != 1 {
		t.Errorf("mid-run pack is missing images: %+v", p.Slots)
	}

	close(f.prov.block)
	f.waitDone(t)
}

// --- save and discard ------------------------------------------------------

func TestSavePromotesAndSelectsThePack(t *testing.T) {
	f := newPackGenFixture(t)
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	f.waitDone(t)

	got, err := f.app.SavePackGen()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Saved {
		t.Error("state does not report the pack as saved")
	}
	paths, _ := packGenPathsFor("Test Pack")
	if !packGenIsDir(paths.Live) {
		t.Fatal("the pack was not promoted")
	}
	if packGenIsDir(paths.Staging) {
		t.Error("the staging folder survived the save")
	}
	if got := f.app.GetCompanionConfig().PackPath; got != paths.Live {
		t.Errorf("active pack = %q, want %q", got, paths.Live)
	}
	p, _ := loadPack(paths.Live)
	if len(p.Warnings) != 0 {
		t.Errorf("the saved pack has warnings:\n  %s", strings.Join(p.Warnings, "\n  "))
	}
}

func TestSaveRefusesWhileRunning(t *testing.T) {
	f := newPackGenFixture(t)
	f.prov.blockN = 1
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the run to reach the provider", func() bool { return f.prov.count() >= 1 })

	if _, err := f.app.SavePackGen(); err == nil {
		t.Error("saved a run that was still generating")
	}
	close(f.prov.block)
	f.waitDone(t)
}

func TestSaveWithNothingGenerated(t *testing.T) {
	f := newPackGenFixture(t)
	if _, err := f.app.SavePackGen(); err == nil {
		t.Error("want an error when there is no run")
	}

	// A run where every slot failed has nothing to promote.
	f.prov.reply[1] = func(*testing.T) (imagegen.Result, error) { return imagegen.Result{}, errors.New("boom") }
	f.prov.reply[2] = func(*testing.T) (imagegen.Result, error) { return imagegen.Result{}, errors.New("boom") }
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	got := f.waitDone(t)
	if got.Status != packGenFailed {
		t.Errorf("status = %q, want failed when nothing was generated", got.Status)
	}
	if _, err := f.app.SavePackGen(); err == nil {
		t.Error("saved a run with no images")
	}
}

func TestDiscardRemovesTheStagedPack(t *testing.T) {
	f := newPackGenFixture(t)
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	f.waitDone(t)
	paths, _ := packGenPathsFor("Test Pack")

	if err := f.app.DiscardPackGen(); err != nil {
		t.Fatal(err)
	}
	if packGenIsDir(paths.Staging) {
		t.Error("discard left the staged folder behind")
	}
	if f.app.PackGenStatus().ID != "" {
		t.Error("discard left the run in place")
	}
}

// Discard is the only path that deletes generated art. Saving must not, and a
// failed or cancelled run must not either — that is what makes retrying safe.
func TestOnlyDiscardDeletesArt(t *testing.T) {
	f := newPackGenFixture(t)
	f.prov.reply[2] = func(*testing.T) (imagegen.Result, error) { return imagegen.Result{}, errors.New("boom") }
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	f.waitDone(t)

	paths, _ := packGenPathsFor("Test Pack")
	if !packGenIsDir(paths.Staging) {
		t.Fatal("a partially failed run destroyed its own art")
	}
	p, _ := loadPack(paths.Staging)
	if len(p.Slots["idle"]) != 1 {
		t.Error("the successful image did not survive a failed sibling")
	}
}

// --- previews --------------------------------------------------------------

// The wizard shows staged art through the same media route the live pack uses,
// so the id must resolve there while the run is unsaved — and stop resolving
// once it is not.
func TestStagedPreviewsServeAndThenStop(t *testing.T) {
	f := newPackGenFixture(t)
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	got := f.waitDone(t)

	id := got.Items[0].Preview
	if id == "" {
		t.Fatal("no preview id")
	}
	srv := httptest.NewServer(f.app.packMediaHandler())
	defer srv.Close()

	res, err := srv.Client().Get(srv.URL + mediaPackPrefix + id)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("staged preview returned %d", res.StatusCode)
	}

	// After discard the id must stop resolving. Asserting only the 404 proved
	// nothing: discard also deletes the files, so the handler refuses on the
	// missing file whether or not the map was cleared — a mutation removing
	// dropStagedPreviewMap left the whole suite green. Inspect the MAP, which
	// is the thing under test, and keep the 404 as the user-visible corollary.
	if err := f.app.DiscardPackGen(); err != nil {
		t.Fatal(err)
	}
	f.app.packMu.Lock()
	_, stillMapped := f.app.packGenPreviews[id]
	f.app.packMu.Unlock()
	if stillMapped {
		t.Error("the staging preview map still points at a folder that was deleted")
	}
	res2, err := srv.Client().Get(srv.URL + mediaPackPrefix + id)
	if err != nil {
		t.Fatal(err)
	}
	res2.Body.Close()
	if res2.StatusCode != 404 {
		t.Errorf("a discarded preview still returns %d", res2.StatusCode)
	}
}

// Saving must not make the wizard's thumbnails flash broken.
//
// Promotion is a rename, so the bytes — and therefore the content-hash ids —
// are unchanged. Every preview id is already in the live pack's map by the time
// the staged one is dropped, so the URLs the wizard is currently displaying
// keep resolving straight through the save with no reload.
func TestPreviewUrlsKeepWorkingAcrossSave(t *testing.T) {
	f := newPackGenFixture(t)
	if _, err := f.app.StartPackGen(f.request(PackGenSlot{Slot: "idle"})); err != nil {
		t.Fatal(err)
	}
	got := f.waitDone(t)
	id := got.Items[0].Preview
	if id == "" {
		t.Fatal("no preview id")
	}

	srv := httptest.NewServer(f.app.packMediaHandler())
	defer srv.Close()
	fetch := func() (int, []byte) {
		t.Helper()
		res, err := srv.Client().Get(srv.URL + mediaPackPrefix + id)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, b
	}

	code, before := fetch()
	if code != 200 {
		t.Fatalf("staged preview returned %d", code)
	}
	if _, err := f.app.SavePackGen(); err != nil {
		t.Fatal(err)
	}
	code, after := fetch()
	if code != 200 {
		t.Fatalf("the same preview URL returned %d after saving", code)
	}
	if string(before) != string(after) {
		t.Error("the preview URL now serves different bytes")
	}

	// And it is being served from the promoted pack, not from a stale staging
	// entry pointing at a folder that no longer exists.
	f.app.packMu.Lock()
	_, stagedStillMapped := f.app.packGenPreviews[id]
	_, liveMapped := f.app.packPaths[id]
	f.app.packMu.Unlock()
	if stagedStillMapped {
		t.Error("the staging map was not dropped after saving")
	}
	if !liveMapped {
		t.Error("the id is not served by the promoted pack")
	}
}

// Loading the live pack replaces packPaths wholesale. Previews live in their
// own map precisely so that does not wipe them mid-run.
func TestLoadingTheLivePackDoesNotDropStagedPreviews(t *testing.T) {
	f := newPackGenFixture(t)
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	got := f.waitDone(t)
	id := got.Items[0].Preview

	// Point the app at some other pack and load it.
	other := filepath.Join(companionPacksDir(), "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := writePackManifest(other, "Other", []packGenArt{
		{Slot: "idle", Image: testPNG(t, 12, 12, 3, false)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.setCompanionPack(other); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(f.app.packMediaHandler())
	defer srv.Close()
	res, err := srv.Client().Get(srv.URL + mediaPackPrefix + id)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Errorf("loading another pack dropped the staged preview (%d)", res.StatusCode)
	}
}

// --- events ----------------------------------------------------------------

func TestRunEmitsProgress(t *testing.T) {
	f := newPackGenFixture(t)
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	f.waitDone(t)

	f.emitter.mu.Lock()
	defer f.emitter.mu.Unlock()
	var n int
	for _, e := range f.emitter.events {
		if e == "packgen:progress" {
			n++
		}
	}
	// Two slots: running/done for each, plus the terminal emit.
	if n < 5 {
		t.Errorf("got %d progress events, want at least 5: %v", n, f.emitter.events)
	}
}

// A run with no emitter attached (a not-yet-started App) must not panic.
func TestRunWithoutAnEmitter(t *testing.T) {
	f := newPackGenFixture(t)
	f.app.emitter = nil
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	if got := f.waitDone(t); got.Status != packGenDone {
		t.Errorf("status = %q", got.Status)
	}
}

// The failure class that used to be reported as free. Once fal's submit returns
// 2xx the image is generated and charged; a failure collecting it is OUR
// problem, not a refund. Keying the counter on errors.Is(ErrBlackImage) caught
// one such path and missed the rest.
func TestPostSubmitFailuresAreStillBilled(t *testing.T) {
	f := newPackGenFixture(t)
	f.prov.reply[1] = func(*testing.T) (imagegen.Result, error) {
		return imagegen.Result{Billed: true}, errors.New("fetch result: giving up after 3 attempts")
	}
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	got := f.waitDone(t)

	if got.Billed != 2 {
		t.Errorf("billed = %d, want 2 — a result we failed to download was still generated and charged", got.Billed)
	}
	if got.Done != 1 {
		t.Errorf("done = %d, want 1", got.Done)
	}
}

// And the converse must stay true: a failure BEFORE the submit lands produced
// nothing and must not be counted, or the spend figure over-reports instead.
func TestPreSubmitFailuresAreNotBilled(t *testing.T) {
	f := newPackGenFixture(t)
	for i := 1; i <= 2; i++ {
		f.prov.reply[i] = func(*testing.T) (imagegen.Result, error) {
			return imagegen.Result{}, errors.New("dial tcp: connection refused")
		}
	}
	if _, err := f.app.StartPackGen(f.request()); err != nil {
		t.Fatal(err)
	}
	if got := f.waitDone(t); got.Billed != 0 {
		t.Errorf("billed = %d, want 0", got.Billed)
	}
}

// setFailure records something the user must see WITHOUT ending the run — and
// every successful run then called finish(done, "") straight over it, so the
// one warning it exists to deliver was never delivered. The user learned months
// later, when regeneration was refused, that single-scene fixes now cost a full
// re-buy.
func TestANonFatalWarningSurvivesTheRunFinishing(t *testing.T) {
	run := &packGenRun{status: packGenRunning, done: make(chan struct{})}
	run.setFailure("the base portrait could not be stored")

	run.finish(packGenDone, "")
	got := run.snapshot()
	if got.Status != packGenDone {
		t.Errorf("status = %q", got.Status)
	}
	if got.Error == "" {
		t.Fatal("the warning was wiped by a successful finish")
	}
	// A real failure still wins: it is the more important message.
	run2 := &packGenRun{status: packGenRunning, done: make(chan struct{})}
	run2.setFailure("a warning")
	run2.finish(packGenFailed, "no images were generated")
	if got := run2.snapshot().Error; got != "no images were generated" {
		t.Errorf("failure message = %q, want the terminal failure", got)
	}
}

// The warning's real path: the base cannot be stored, the run still succeeds,
// and the user is told the pack will not be regeneratable.
func TestAPackThatCouldNotStoreItsBaseSaysSo(t *testing.T) {
	f := newPackGenFixture(t)
	// Make the staging folder unwritable for the base only, by pre-creating a
	// DIRECTORY where writePackBase wants to put a file.
	paths, _ := packGenPathsFor("Test Pack")
	if err := os.MkdirAll(filepath.Join(paths.Staging, packGenBaseStem+".png"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.StartPackGen(f.request(PackGenSlot{Slot: "idle"})); err != nil {
		t.Fatal(err)
	}
	got := f.waitDone(t)

	if got.Status != packGenDone {
		t.Fatalf("the run failed instead of warning: %q / %s", got.Status, got.Error)
	}
	if got.Error == "" || !strings.Contains(got.Error, "regenerate") {
		t.Errorf("the user was not warned that single scenes cannot be regenerated: %q", got.Error)
	}
}

func TestSnapshotReportsHeldArtEvenWhenItemsFailed(t *testing.T) {
	// A regen/add run is seeded with the pack's art, so it holds art even if
	// every requested item failed. The client relies on this to surface
	// Save/Discard instead of stranding the user.
	r := &packGenRun{
		status: packGenDone,
		items:  []PackGenItem{{Slot: "bored", When: "", Status: "failed"}},
		arts:   []packGenArt{{Slot: "idle", When: ""}},
		done:   make(chan struct{}),
	}
	s := r.snapshot()
	if !s.HasArt {
		t.Fatal("HasArt = false, want true: seeded art is held regardless of item status")
	}
	if s.Done != 0 {
		t.Fatalf("Done = %d, want 0: no item is done", s.Done)
	}
}

func TestSnapshotReportsNoArtForEmptyRun(t *testing.T) {
	r := &packGenRun{
		status: packGenFailed,
		items:  []PackGenItem{{Slot: "idle", When: "", Status: "failed"}},
		done:   make(chan struct{}),
	}
	if r.snapshot().HasArt {
		t.Fatal("HasArt = true, want false: the run has no art")
	}
}
