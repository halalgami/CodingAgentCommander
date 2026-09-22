package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/imagegen"
)

// The pack generation run.
//
// Every image costs the user real money, so the invariants here are about spend
// and about not destroying what was bought:
//
//   - The run slot is claimed under the lock, never checked-then-claimed. Two
//     concurrent starts would otherwise both pass the check and the loser's
//     goroutine would bill images against a run nothing can cancel or save.
//   - A billed image is counted as billed even when it arrives as an error.
//     ErrBlackImage is a silent safety rejection: HTTP 200, real bytes, real
//     charge. Booking it as unbilled would under-report the user's spend.
//   - Nothing deletes staged art except an explicit discard. A finished run
//     whose art has not been saved BLOCKS a new run rather than being cleared
//     to make room, because the alternative is deleting images the user paid
//     for to make space for images they are about to pay for.
//
// Generation is sequential. Provider rate limits differ per plan, cancellation
// should leave a predictable spend, and concurrency buys little against the
// burst-429 risk it creates.

// Seams. The imagegen registry panics on a duplicate id by design, so tests
// cannot install a fake by registering one; they swap these instead.
var (
	packGenLookup  = imagegen.Lookup
	packGenLoadKey = imagegen.LoadKey
)

// packGenRunSeq names runs. Monotonic rather than random so a log or an event
// stream reads in order.
var packGenRunSeq atomic.Int64

// Run status values. A run is terminal in every state except running, and a
// terminal run holding unsaved art is what blocks the next one.
const (
	packGenRunning   = "running"
	packGenDone      = "done"
	packGenCancelled = "cancelled"
	packGenFailed    = "failed"
)

// PackGenItem is one requested image and what became of it. Exported for the
// Wails binding; the frontend renders one card per item.
type PackGenItem struct {
	Slot   string `json:"slot"`
	When   string `json:"when"`
	Status string `json:"status"` // pending | running | done | failed | skipped
	Error  string `json:"error,omitempty"`
	// Preview is the media id this item's art is served under while it is still
	// staged. Empty until the image lands.
	Preview string `json:"preview,omitempty"`
}

// PackGenState is the whole run, as the UI sees it.
type PackGenState struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Status string        `json:"status"`
	Total  int           `json:"total"`
	Done   int           `json:"done"`
	Billed int           `json:"billed"`
	Items  []PackGenItem `json:"items"`
	Error  string        `json:"error,omitempty"`
	// Saved reports whether the art has been promoted into the packs folder.
	// A run that is finished but not saved is holding paid-for art hostage,
	// which is why it blocks the next run.
	Saved bool `json:"saved"`
	// HasArt reports whether the run holds any art at all — seeded or generated,
	// regardless of per-item status. A regenerate/add run is seeded with the
	// saved pack's art, so it finishes "done" holding art even when every
	// requested scene failed; the client drives Save/Discard visibility off this
	// rather than off the count of done items, which would be zero in that case.
	HasArt bool `json:"hasArt"`
}

// PackGenRequest is what the wizard submits.
type PackGenRequest struct {
	Name       string        `json:"name"`
	ProviderID string        `json:"providerId"`
	Model      string        `json:"model"`
	StyleID    string        `json:"styleId"`
	BasePath   string        `json:"basePath"`
	Slots      []PackGenSlot `json:"slots"`
	// Regenerate replaces slots inside an EXISTING pack rather than building a
	// new one. The staged folder is seeded from that pack, so untouched
	// variants survive and a failure leaves it exactly as it was.
	Regenerate bool `json:"regenerate"`
	// PackPath addresses the pack to regenerate, and is required when
	// Regenerate is set.
	//
	// It exists because Name is NOT an address. Name is a display string; the
	// folder is slug(Name), and the two only agree for packs this app
	// generated. An imported pack takes its folder from the zip's filename
	// (plus a -2 suffix on collision) while keeping the manifest's own name, so
	// regenerating one scene of it by name resolved to a DIFFERENT pack and
	// rewrote that instead.
	PackPath string `json:"packPath"`
	// Overwrite permits a NEW pack to replace an existing one of the same
	// slug. Without it that is refused: promotion deletes the pack it replaces,
	// and the wizard leaves the previous name pre-filled, so pressing Generate
	// twice destroyed the first pack without asking.
	Overwrite bool `json:"overwrite"`
}

type packGenRun struct {
	id    string
	name  string
	paths packGenPaths

	cancel context.CancelFunc

	mu         sync.Mutex
	status     string
	billed     int
	items      []PackGenItem
	arts       []packGenArt
	failure    string
	saved      bool
	regenerate bool

	// done closes when runPackGen returns. Without it, DiscardPackGen deleted
	// the staging folder and released the run slot while the goroutine was
	// still writing into that folder — so a new run for the same name reused
	// the surviving directory and the old goroutine's final flush overwrote the
	// new run's manifest with the discarded run's art. Every individual access
	// was correctly locked, so -race saw nothing: the defect was the absence of
	// a join, not a data race.
	done chan struct{}
}

// snapshot copies the run's state under the lock. The caller receives a value
// it owns, so it can be marshalled or emitted without holding anything.
func (r *packGenRun) snapshot() PackGenState {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]PackGenItem, len(r.items))
	copy(items, r.items)
	var done int
	for _, it := range items {
		if it.Status == "done" {
			done++
		}
	}
	return PackGenState{
		ID: r.id, Name: r.name, Status: r.status, Total: len(items),
		Done: done, Billed: r.billed, Items: items, Error: r.failure, Saved: r.saved,
		HasArt: len(r.arts) > 0,
	}
}

// active reports whether this run still owns the single run slot. A run holds
// it while generating AND after finishing, until its art is saved or discarded.
func (r *packGenRun) active() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status == packGenRunning || (!r.saved && len(r.arts) > 0)
}

// StartPackGen begins a generation run. Everything that can fail cheaply —
// naming, slot validation, reading the base image, finding the provider and its
// key — happens BEFORE the run slot is claimed, so a rejected request never
// disturbs a run in flight.
func (a *App) StartPackGen(req PackGenRequest) (PackGenState, error) {
	paths, err := packGenPathsFor(req.Name)
	if err != nil {
		return PackGenState{}, err
	}
	if req.Regenerate {
		// Address the pack by PATH. Deriving it from the display name is how a
		// regeneration ends up rewriting a different pack.
		if strings.TrimSpace(req.PackPath) == "" {
			return PackGenState{}, fmt.Errorf("no pack given to regenerate")
		}
		if paths, err = packGenPathsAt(req.PackPath); err != nil {
			return PackGenState{}, err
		}
	}
	// A regeneration replaces part of an existing pack, so the "must include
	// idle" rule does not apply — the saved pack already has one, and demanding
	// it here would mean re-buying the idle image to fix any other slot.
	if req.Regenerate {
		if err := validateRegenSlots(req.Slots); err != nil {
			return PackGenState{}, err
		}
	} else if err := validatePackGenSlots(req.Slots); err != nil {
		return PackGenState{}, err
	}
	style, ok := packGenStyleByID(req.StyleID)
	if !ok {
		return PackGenState{}, fmt.Errorf("unknown style %q", req.StyleID)
	}
	// A new pack must never silently replace one that exists. promotePack
	// deletes what it replaces, and saveRun leaves the name pre-filled, so the
	// obvious second run lands on the first pack's folder.
	if !req.Regenerate && !req.Overwrite && packGenIsDir(paths.Live) {
		return PackGenState{}, fmt.Errorf(
			"a pack called %q already exists (%s). Saving would replace it and its images cannot be recovered — rename this one, or confirm the replacement.",
			paths.Slug, paths.Live)
	}

	var base []byte
	var seeded []packGenArt
	if req.Regenerate {
		if !packGenIsDir(paths.Live) {
			return PackGenState{}, fmt.Errorf("there is no saved pack called %q to regenerate", paths.Slug)
		}
		var ok bool
		if base, ok = findPackBase(paths.Live); !ok {
			return PackGenState{}, fmt.Errorf(
				"this pack has no stored base portrait, so a single scene cannot be regenerated from it")
		}
		if seeded, err = loadPackGenArts(paths.Live); err != nil {
			return PackGenState{}, err
		}
	} else if base, err = readFileLimited(req.BasePath, packGenMaxBaseBytes); err != nil {
		return PackGenState{}, fmt.Errorf("reading the base portrait: %w", err)
	}
	baseFacts, err := packGenImageFacts(base)
	if err != nil {
		return PackGenState{}, fmt.Errorf("the base portrait could not be read as an image: %w", err)
	}
	prov, ok := packGenLookup(req.ProviderID)
	if !ok {
		return PackGenState{}, fmt.Errorf("unknown image provider %q", req.ProviderID)
	}
	model := req.Model
	if model == "" && len(prov.Models()) > 0 {
		model = prov.Models()[0].ID
	}
	if !packGenModelOffered(prov, model) {
		return PackGenState{}, fmt.Errorf("%s does not offer the model %q", prov.ID(), model)
	}
	// Read at the last possible moment and never stored on the run: a key on a
	// long-lived struct is a key in every heap dump for the life of the run.
	key := packGenLoadKey(prov.ID())
	if key == "" {
		return PackGenState{}, fmt.Errorf("no API key saved for %s; add one in Settings first", prov.ID())
	}

	items := make([]PackGenItem, len(req.Slots))
	for i, s := range req.Slots {
		items[i] = PackGenItem{Slot: s.Slot, When: s.When, Status: "pending"}
	}
	run := &packGenRun{
		id:     fmt.Sprintf("packgen-%d", packGenRunSeq.Add(1)),
		name:   req.Name,
		paths:  paths,
		status: packGenRunning,
		items:  items,
		done:   make(chan struct{}),
		// Seeded with the saved pack's art so the manifest is rewritten from
		// ONE place, including the variants this run is not touching. There is
		// exactly one function that writes a manifest, so exactly one place
		// where "loads with zero warnings" has to hold.
		arts:       seeded,
		regenerate: req.Regenerate,
	}

	// Claim the slot under the lock. Checking and then assigning would let two
	// concurrent starts both pass, and the loser's goroutine would go on
	// spending money against a run no binding can reach.
	a.packGenMu.Lock()
	if prev := a.packGen; prev != nil && prev.active() {
		a.packGenMu.Unlock()
		if prev.snapshot().Status == packGenRunning {
			return PackGenState{}, fmt.Errorf("a pack is already being generated")
		}
		return PackGenState{}, fmt.Errorf("the previous run has unsaved images; save or discard them first")
	}
	ctx, cancel := context.WithCancel(context.Background())
	run.cancel = cancel
	a.packGen = run
	a.packGenMu.Unlock()

	go a.runPackGen(ctx, run, prov, key, model, base, baseFacts, style.Text, req.Slots)
	return run.snapshot(), nil
}

func packGenModelOffered(p imagegen.Provider, model string) bool {
	for _, m := range p.Models() {
		if m.ID == model {
			return true
		}
	}
	return false
}

// runPackGen generates every requested slot in order, writing the staging
// folder after each success so a crash leaves a loadable partial pack rather
// than nothing.
func (a *App) runPackGen(
	ctx context.Context, run *packGenRun, prov imagegen.Provider,
	key, model string, base []byte, baseFacts packGenFacts,
	styleText string, slots []PackGenSlot,
) {
	defer close(run.done)
	defer run.cancel()

	if err := os.MkdirAll(run.paths.Staging, 0o755); err != nil {
		run.finish(packGenFailed, fmt.Sprintf("could not create the staging folder: %v", err))
		a.emitPackGen(run)
		return
	}
	if run.regenerate {
		// Copy the saved pack in first, so a regeneration that fails on its
		// very first image still promotes to something complete.
		if err := copyPackTree(run.paths.Live, run.paths.Staging); err != nil {
			run.finish(packGenFailed, fmt.Sprintf("could not copy the saved pack: %v", err))
			a.emitPackGen(run)
			return
		}
	} else if err := writePackBase(run.paths.Staging, base); err != nil {
		// Not fatal: the pack is still usable, it just cannot be regenerated
		// from later, and saying so beats failing a run that would otherwise
		// have produced good art.
		run.setFailure(fmt.Sprintf("the base portrait could not be stored in the pack, so single scenes cannot be regenerated later: %v", err))
	}

	for i, s := range slots {
		if ctx.Err() != nil {
			run.skipRemaining(i)
			break
		}
		run.setItemStatus(i, "running", "")
		a.emitPackGen(run)

		res, err := prov.Generate(ctx, key, imagegen.Request{
			Mode: imagegen.ModeEdit,
			// The model the user actually picked. Until a second model existed
			// this was omitted and every run silently used the provider's
			// default, which made the picker in the wizard decorative.
			Model:    model,
			Base:     base,
			BaseMIME: packGenMIME(baseFacts.Ext),
			Prompt:   composePackPrompt(s, styleText, model),
			Portrait: true,
		})

		// Count the charge BEFORE deciding whether the bytes are usable, and
		// count it from the provider's own report rather than from the error.
		//
		// "An error means no charge" is false here: once fal's submit returns
		// 2xx the image exists and is paid for, so a failed result download, a
		// response with no images, or a body that will not decode are all
		// billed failures. Keying on errors.Is(ErrBlackImage) caught exactly
		// one of those and under-reported the rest.
		if res.Billed {
			run.countBilled()
		}

		// A cancel landing at the same moment as a SUCCESS used to discard the
		// image: the ctx check came first, so a paid-for result was dropped on
		// the floor. Keep anything that actually arrived — the run stops either
		// way, and recording it costs nothing.
		if ctx.Err() != nil && err == nil && res.FinishReason == "" && len(res.Image) > 0 {
			if rec := run.record(packGenArt{
				Slot: s.Slot, When: s.When, Image: res.Image, MIME: res.MIME,
				Prompt: composePackPrompt(s, styleText, model), Model: res.Model,
				Seed: res.Seed, GeneratedAt: time.Now(),
			}); rec == nil {
				_ = a.flushPackGen(run)
				run.setItemStatus(i, "done", "")
				run.skipRemaining(i + 1)
				a.emitPackGen(run)
				run.finish(packGenCancelled, "")
				a.emitPackGen(run)
				return
			}
		}

		switch {
		case ctx.Err() != nil:
			// Cancelled with nothing usable in hand. The charge above still
			// counts if the provider got far enough to bill.
			run.setItemStatus(i, "skipped", "")
			run.skipRemaining(i + 1)
			a.emitPackGen(run)
			run.finish(packGenCancelled, "")
			a.emitPackGen(run)
			return
		case errors.Is(err, imagegen.ErrBlackImage):
			// Named separately from a generic failure because it is the one
			// failure the user PAID for. "failed" alone reads as "the request
			// broke", and the user re-rolls expecting it to be free. It is not:
			// fal returns a filter rejection as HTTP 200 carrying a black
			// frame, and charges for it. Saying so is the difference between a
			// cost the user can decide about and one that just happens to them.
			run.setItemStatus(i, "failed",
				"rejected by the provider's safety filter, and charged for. "+
					"This model does that on roughly one image in six; nothing about "+
					"your prompt caused it. Regenerate this scene to try again.")
		case err != nil:
			// One failed slot does not fail the run: partial packs are legal,
			// and re-running five good images to recover one is expensive.
			run.setItemStatus(i, "failed", err.Error())
		case res.FinishReason != "":
			run.setItemStatus(i, "failed", fmt.Sprintf("the provider flagged this image (%s) and it was not kept", res.FinishReason))
		default:
			art := packGenArt{
				Slot: s.Slot, When: s.When, Image: res.Image, MIME: res.MIME,
				Prompt: composePackPrompt(s, styleText, model), Model: res.Model,
				Seed: res.Seed, GeneratedAt: time.Now(),
			}
			if art.Model == "" {
				art.Model = model
			}
			if err := run.record(art); err != nil {
				run.setItemStatus(i, "failed", err.Error())
				break
			}
			if err := a.flushPackGen(run); err != nil {
				run.setItemStatus(i, "failed", err.Error())
				break
			}
			run.setItemStatus(i, "done", "")
		}
		a.emitPackGen(run)
	}

	if run.usable() == 0 {
		run.finish(packGenFailed, "no images were generated")
	} else {
		run.finish(packGenDone, "")
	}
	a.emitPackGen(run)
}

// flushPackGen rewrites the staging folder from everything recorded so far.
//
// It rewrites rather than appends because writePackManifest derives the whole
// manifest — canvas, alpha, deduplication — from the full set, and a manifest
// built incrementally would have to duplicate that logic. Re-writing a handful
// of small files six times is cheap next to one image generation, and it buys
// the property that a crash at any moment leaves a folder that LOADS.
func (a *App) flushPackGen(run *packGenRun) error {
	run.mu.Lock()
	arts := make([]packGenArt, len(run.arts))
	copy(arts, run.arts)
	name := run.name
	run.mu.Unlock()

	packGenSortArts(arts)
	if _, err := writePackManifest(run.paths.Staging, name, arts); err != nil {
		return err
	}
	a.refreshPackGenPreviews(run)
	return nil
}

func (r *packGenRun) record(art packGenArt) error {
	// Decode before recording. A provider that returns an HTML error page with
	// a 200 would otherwise be written into the pack as a .png the webview
	// renders as a broken image, with nothing anywhere saying why.
	if _, err := packGenImageFacts(art.Image); err != nil {
		return fmt.Errorf("the provider returned something that is not a usable image: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// UPSERT, not append. A regeneration is seeded with the saved pack's art,
	// so appending would leave the old image beside the new one — two variants
	// of the same (slot, condition), both displayable, picked between at
	// random. The user would see the image they just paid to replace come back
	// half the time.
	for i := range r.arts {
		if r.arts[i].Slot == art.Slot && r.arts[i].When == art.When && r.arts[i].Mood == art.Mood {
			// The crop the user nudged belongs to the SLOT, not to the image
			// that happened to be in it, so it survives a replacement.
			art.FocusX, art.FocusY = r.arts[i].FocusX, r.arts[i].FocusY
			r.arts[i] = art
			return nil
		}
	}
	r.arts = append(r.arts, art)
	return nil
}

// refreshPackGenPreviews republishes the staged folder's media ids so the
// wizard can display art before it is saved.
//
// It goes through loadPack rather than registering paths directly, which buys
// three things for free: the ids are the same content hashes the live pack
// uses, every path is symlink-resolved and containment-checked by
// resolvePackFile, and a preview only appears for art that actually LOADS — so
// the wizard cannot show an image that the saved pack would then drop.
func (a *App) refreshPackGenPreviews(run *packGenRun) {
	p, ids := loadPack(run.paths.Staging)

	a.packMu.Lock()
	a.packGenPreviews = ids
	a.packMu.Unlock()

	run.mu.Lock()
	defer run.mu.Unlock()
	for i := range run.items {
		for _, v := range p.Slots[run.items[i].Slot] {
			if v.When == run.items[i].When {
				run.items[i].Preview = v.ID
				break
			}
		}
	}
}

// dropStagedPreviewMap stops serving art from the staging folder. The ids
// themselves are NOT invalidated: promotion is a rename, so the bytes and
// therefore the content hashes are unchanged, and the same id resolves through
// packPaths once the promoted pack is loaded. Called after that load, so an id
// is continuously resolvable through one map or the other and the wizard's
// thumbnails do not flash broken at the moment of saving.
func (a *App) dropStagedPreviewMap() {
	a.packMu.Lock()
	a.packGenPreviews = nil
	a.packMu.Unlock()
}

// setFailure records a non-fatal problem without ending the run. The run's
// status is untouched: this is something the user should see, not a reason to
// throw away images that generated correctly.
func (r *packGenRun) setFailure(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failure == "" {
		r.failure = msg
	}
}

func (r *packGenRun) countBilled() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.billed++
}

func (r *packGenRun) setItemStatus(i int, status, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i < 0 || i >= len(r.items) {
		return
	}
	r.items[i].Status = status
	r.items[i].Error = msg
}

func (r *packGenRun) skipRemaining(from int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := from; i < len(r.items); i++ {
		if r.items[i].Status == "pending" || r.items[i].Status == "running" {
			r.items[i].Status = "skipped"
		}
	}
}

func (r *packGenRun) usable() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.arts)
}

func (r *packGenRun) finish(status, failure string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// A cancellation that arrives after the last slot already finished must not
	// relabel a completed run.
	if r.status != packGenRunning {
		return
	}
	r.status = status
	// Never blank an existing warning. setFailure records something the user
	// needs to know and does NOT end the run — and every successful run then
	// called finish(done, "") straight over it, so the one message it exists to
	// deliver ("the base portrait could not be stored, single scenes cannot be
	// regenerated later") was never seen.
	if failure != "" || r.failure == "" {
		r.failure = failure
	}
}

func packGenMIME(ext string) string {
	if ext == ".jpg" {
		return "image/jpeg"
	}
	return "image/png"
}

// --- bindings --------------------------------------------------------------

// PackGenStatus returns the current run, or a zero state when there is none.
func (a *App) PackGenStatus() PackGenState {
	a.packGenMu.Lock()
	run := a.packGen
	a.packGenMu.Unlock()
	if run == nil {
		return PackGenState{}
	}
	return run.snapshot()
}

// CancelPackGen stops a run in flight. Images already generated are KEPT: they
// are paid for, and the user cancelled the remaining spend, not the art.
func (a *App) CancelPackGen() PackGenState {
	a.packGenMu.Lock()
	run := a.packGen
	a.packGenMu.Unlock()
	if run == nil {
		return PackGenState{}
	}
	if run.cancel != nil {
		run.cancel()
	}
	return run.snapshot()
}

// SavePackGen promotes the staged pack and makes it the active one.
func (a *App) SavePackGen() (PackGenState, error) {
	a.packGenMu.Lock()
	run := a.packGen
	a.packGenMu.Unlock()
	if run == nil {
		return PackGenState{}, fmt.Errorf("there is no generated pack to save")
	}
	if s := run.snapshot(); s.Status == packGenRunning {
		return s, fmt.Errorf("the run is still generating; cancel it first or wait for it to finish")
	}
	if run.usable() == 0 {
		return run.snapshot(), fmt.Errorf("there are no images to save")
	}
	if err := promotePack(run.paths); err != nil {
		return run.snapshot(), err
	}

	run.mu.Lock()
	run.saved = true
	run.mu.Unlock()

	// Load the promoted pack FIRST, then drop the staging map. The ids are
	// content hashes and a rename does not change bytes, so every preview id is
	// already present in the live map by the time the staged one goes away —
	// the wizard's thumbnails keep resolving straight through the save.
	if _, err := a.setCompanionPack(run.paths.Live); err != nil {
		// The art is saved and on disk either way; only the "make it active"
		// half failed, and saying so is more useful than failing the save.
		// The staging map stays until the next run replaces it, which is
		// harmless and keeps the previews working meanwhile.
		return run.snapshot(), fmt.Errorf("the pack was saved but could not be selected: %w", err)
	}
	a.dropStagedPreviewMap()
	return run.snapshot(), nil
}

// DiscardPackGen deletes the staged pack. This is the ONLY path that removes
// generated art, and it exists only behind an explicit user action, because
// everything in that folder was paid for.
func (a *App) DiscardPackGen() error {
	a.packGenMu.Lock()
	run := a.packGen
	a.packGen = nil
	a.packGenMu.Unlock()
	if run == nil {
		return nil
	}
	if run.cancel != nil {
		run.cancel()
	}
	// WAIT for the goroutine before touching the folder. Cancelling only asks
	// it to stop; it may be inside writePackManifest right now, and deleting
	// the directory underneath it leaves a half-written folder that the next
	// run for the same name then adopts.
	if run.done != nil {
		<-run.done
	}
	a.dropStagedPreviewMap()
	if err := os.RemoveAll(run.paths.Staging); err != nil {
		return fmt.Errorf("removing the staged pack: %w", err)
	}
	return nil
}

// PackGenStyles lists the style presets for the wizard.
func (a *App) PackGenStyles() []PackGenStyle { return PackGenStyles }

// PackGenSlots lists the generatable slots with their default staging text.
func (a *App) PackGenSlots() []PackGenSlot {
	out := make([]PackGenSlot, 0, len(packGenSlotOrder))
	for _, s := range packGenSlotOrder {
		out = append(out, PackGenSlot{Slot: s, Staging: packGenSlotStaging[s]})
	}
	return out
}

func (a *App) emitPackGen(run *packGenRun) {
	if a.emitter != nil {
		a.emitter.Emit("packgen:progress", run.snapshot())
	}
}
