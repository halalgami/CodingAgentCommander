package main

// Image-pack loading for the holo-room companion kind.
//
// Go is authoritative for parsing, schema, id assignment, existence checks,
// extension whitelisting and slot/condition validation; it returns
// {pack, warnings} over a binding and NEVER fails the app (spec §3, §5.10).
// The frontend appends only post-decode facts (intrinsic size, working set) to
// the same list, so there is exactly one authority for the ids — i.e. for the
// URLs Go serves versus the URLs JS requests.
//
// This whole file is deleted by scripts/export-public.sh.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// packSchemaMajor is the manifest schema this build understands. A pack
// declaring a HIGHER major refuses to load, with a user-visible warning;
// minor/absent/non-integer values load as 1 (§5.10).
const packSchemaMajor = 1

// packSlots is the frozen slot vocabulary (§5.1): six foreground, four
// background. This list is the pack format's public API — what every image is
// named for and what project B generates against. It cannot change once art
// exists. Order is authoring/display order, NOT precedence (precedence is the
// ladder in slots.js).
var packSlots = []string{
	"idle", "working", "done", "awaiting", "error", "bored",
	"bg_idle", "bg_running", "bg_finished", "bg_error",
}

// packConditions maps each named condition to its class (§5.5). Ambient
// conditions join the base pool while they hold; event conditions fire once
// per rising edge. The class is what stops lateNight flashing for 12s every
// ten minutes instead of simply being the night's art.
var packConditions = map[string]string{
	"lateNight":     "ambient",
	"weekend":       "ambient",
	"marathon":      "ambient",
	"longIdle":      "ambient",
	"freshInstall":  "ambient",
	"firstRunOfDay": "event",
	"century":       "event",
	"streak":        "event",
}

// packCondSlots is the machine-readable slot x condition compatibility matrix
// (§5.5). A condition that cannot hold while its slot is showing is dead art:
// longIdle is unreachable in `idle` because `bored` outranks it after five
// minutes; marathon needs something running; century and streak are finish
// events. Background slots appear nowhere because §5.8 routes them past the
// resolver entirely — first variant, no eggs, no mood.
var packCondSlots = map[string][]string{
	"lateNight":    {"idle", "working", "done", "awaiting", "error", "bored"},
	"weekend":      {"idle", "working", "done", "awaiting", "error", "bored"},
	"freshInstall": {"idle", "working", "done", "awaiting", "error", "bored"},
	"longIdle":     {"working", "done", "awaiting", "error", "bored"},
	"marathon":     {"working"},
	// firstRunOfDay is a greeting, and the first launch of a day does not stay
	// idle: a user who opens Commander and immediately launches a session
	// should still get it. Restricting it to `idle` would silently clear the
	// condition and the art would never display — the failure mode this matrix
	// exists to prevent. Kept in sync with SLOT_CONDITIONS in pack.js.
	"firstRunOfDay": {"idle", "working", "done", "awaiting", "error", "bored"},
	"century":       {"done", "awaiting"},
	"streak":        {"done", "awaiting"},
}

// packMoods matches reactions.js exactly, `relaxed` included — it is the most
// common value (§5.2). `sad` is legal in a manifest but unreachable in v1
// because quota is deferred (§6.4), and is NOT warned about.
var packMoods = map[string]bool{
	"neutral": true, "relaxed": true, "happy": true, "sad": true, "surprised": true,
}

// packMediaExt is the served-extension whitelist. GIF is excluded: 256 colours
// with dithering bands badly on the dark gradients this aesthetic is made of,
// and it carries no real alpha (§5.3).
var packMediaExt = map[string]bool{".png": true, ".webp": true, ".jpg": true, ".jpeg": true}

type PackAnim struct {
	Strip  string `json:"strip"`
	Frames int    `json:"frames"`
	FPS    int    `json:"fps"`
}

// PackCanvas describes the authored frame. W and H are FILE pixels and Scale
// is the authoring DPI factor, so effective CSS size is W/Scale and the render
// rule is drawnDevicePx <= W (§4.3).
type PackCanvas struct {
	W      int    `json:"w"`
	H      int    `json:"h"`
	Scale  int    `json:"scale"`
	Anchor string `json:"anchor"`
}

type PackVariant struct {
	ID        string            `json:"id"`
	File      string            `json:"file"`
	Mood      string            `json:"mood"`
	Rarity    float64           `json:"rarity"`
	When      string            `json:"when"`
	Weight    float64           `json:"weight"`
	FocusX    float64           `json:"focusX"`
	FocusY    float64           `json:"focusY"`
	HeadScale float64           `json:"headScale"`
	Anim      *PackAnim         `json:"anim"`
	Meta      map[string]string `json:"meta"`
	Motion    string            `json:"motion"`
	MotionID  string            `json:"motionId"`
}

type Pack struct {
	Schema   int                      `json:"schema"`
	Name     string                   `json:"name"`
	Author   string                   `json:"author"`
	License  string                   `json:"license"`
	Canvas   PackCanvas               `json:"canvas"`
	Alpha    bool                     `json:"alpha"`
	Slots    map[string][]PackVariant `json:"slots"`
	Warnings []string                 `json:"warnings"`
}

func (p *Pack) warnf(format string, args ...any) {
	p.Warnings = append(p.Warnings, fmt.Sprintf(format, args...))
}

func packSlotKnown(slot string) bool {
	for _, s := range packSlots {
		if s == slot {
			return true
		}
	}
	return false
}

// packCondAllowed reports whether cond can ever hold while slot is showing.
func packCondAllowed(cond, slot string) bool {
	for _, s := range packCondSlots[cond] {
		if s == slot {
			return true
		}
	}
	return false
}

// packTopKeys / packVariantKeys are the keys this build understands. Unknown
// keys are ignored with a warning rather than rejected, so project B can add
// fields an older Commander still loads (§5.10).
var packTopKeys = map[string]bool{
	"schema": true, "name": true, "author": true, "license": true,
	"canvas": true, "alpha": true, "slots": true,
}
var packVariantKeys = map[string]bool{
	"id": true, "file": true, "mood": true, "rarity": true, "when": true,
	"weight": true, "focusX": true, "focusY": true, "headScale": true,
	"anim": true, "meta": true, "motion": true,
}

// loadPack parses dir/manifest.json, validates it, assigns every variant a
// stable id and returns the pack plus the id -> absolute path map the media
// handler serves from. It never returns an error: a pack that is wrong in
// every possible way still yields a usable (possibly empty) Pack carrying
// warnings, because a decorative feature must not be able to fail the app.
func loadPack(dir string) (Pack, map[string]string) {
	p := Pack{Schema: packSchemaMajor, Slots: map[string][]PackVariant{}}
	ids := map[string]string{}

	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		p.warnf("manifest.json could not be read: %v", err)
		return p, ids
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		p.warnf("manifest.json is not valid JSON and was ignored: %v", err)
		return p, ids
	}

	// Schema first: a future major is the only thing that stops the load.
	schema := packSchemaMajor
	if r, ok := raw["schema"]; ok {
		var n json.Number
		if err := json.Unmarshal(r, &n); err != nil {
			p.warnf("schema is not a number; treating this pack as schema 1")
		} else if i, err := n.Int64(); err == nil {
			schema = int(i)
		} else if f, err := n.Float64(); err == nil {
			schema = int(f)
			p.warnf("schema %v is not an integer; treating this pack as schema %d", n, schema)
		}
	} else {
		p.warnf("no schema declared; treating this pack as schema 1")
	}
	p.Schema = schema
	if schema > packSchemaMajor {
		p.warnf("this pack declares schema %d, which is newer than this version of Commander understands — update Commander to use it", schema)
		return p, ids
	}

	for _, k := range sortedKeys(raw) {
		if !packTopKeys[k] {
			p.warnf("unknown manifest key %q was ignored", k)
		}
	}

	// Typed decode. A type mismatch fills what it can and warns; it is not
	// fatal, for the same reason nothing else here is. Warnings accumulated so
	// far (schema, unknown top-level keys) must survive the decode even though
	// the manifest itself might carry its own "warnings" key — the author does
	// not get to inject warnings, but they must not erase ours either.
	preDecodeWarnings := p.Warnings
	decodeErr := json.Unmarshal(b, &p)
	p.Schema = schema
	p.Warnings = preDecodeWarnings
	if decodeErr != nil {
		p.warnf("part of manifest.json had unexpected types and was ignored: %v", decodeErr)
	}
	if p.Slots == nil {
		p.Slots = map[string][]PackVariant{}
	}

	if _, ok := raw["canvas"]; !ok || p.Canvas.W <= 0 || p.Canvas.H <= 0 {
		p.warnf("canvas is missing or has no size; layout will fall back to each image's intrinsic size")
	}
	if p.Canvas.Scale <= 0 {
		p.Canvas.Scale = 1
	}
	if p.Canvas.Anchor == "" {
		p.Canvas.Anchor = "bottom-center"
	}
	// NOTE: opaque art is deliberately NOT warned about. An earlier design
	// rendered the companion as a room with its own floor and vignette drawn
	// BEHIND the figure, where a black matte showed as a rectangle. The sidebar
	// host draws art opaquely into an opaque panel, so `alpha:false` is correct
	// there — and every hosted generator returns opaque images, so warning about
	// it would fire on every generated pack while describing a floor that is no
	// longer rendered.

	validatePackSlots(&p, dir, raw, ids)
	return p, ids
}

func sortedKeys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// validatePackSlots walks every slot, drops what cannot be shown, normalises
// what can, assigns ids and fills the media map. Iteration is over sorted slot
// names so the warning list is deterministic and diffable.
func validatePackSlots(p *Pack, dir string, raw map[string]json.RawMessage, ids map[string]string) {
	// Second, untyped view of the slots so unknown VARIANT keys can be named.
	rawSlots := map[string][]map[string]json.RawMessage{}
	if r, ok := raw["slots"]; ok {
		_ = json.Unmarshal(r, &rawSlots)
	}

	names := make([]string, 0, len(p.Slots))
	for name := range p.Slots {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, slot := range names {
		if !packSlotKnown(slot) {
			p.warnf("unknown slot %q was ignored (valid slots: %s)", slot, strings.Join(packSlots, ", "))
			delete(p.Slots, slot)
			continue
		}
		background := strings.HasPrefix(slot, "bg_")
		var kept []PackVariant
		var ambient []string

		for i, v := range p.Slots[slot] {
			label := v.ID
			if label == "" {
				label = v.File
			}
			if i < len(rawSlots[slot]) {
				for _, k := range sortedKeys(rawSlots[slot][i]) {
					if !packVariantKeys[k] {
						p.warnf("slot %q variant %q: unknown key %q was ignored", slot, label, k)
					}
				}
			}

			path, ok := resolvePackFile(p, dir, slot, label, v.File)
			if !ok {
				continue
			}

			// Identity. A declared id wins; anything else gets a content hash,
			// which is also the cache key in the served URL.
			if v.ID != "" && !packIDSafe(v.ID) {
				p.warnf("slot %q variant %q: id contains characters that cannot appear in a URL; using a content hash instead", slot, label)
				v.ID = ""
			}
			if v.ID == "" {
				h, err := packContentID(path)
				if err != nil {
					p.warnf("slot %q variant %q could not be read and was dropped: %v", slot, label, err)
					continue
				}
				v.ID = h
			}
			// The id is content-addressed by default (packContentID), so two
			// variants that legitimately share one image file — the most
			// obvious authoring shortcut, e.g. the same idle frame reused for
			// both `done` and `awaiting` — collide on id here without either
			// side declaring one. That is not a reason to delete art: reuse
			// the id and keep both variants. Only a DIFFERENT file mapping to
			// the same id (which can only happen with an explicitly declared
			// `id`) is a genuine clash worth dropping and warning about.
			if existing, clash := ids[v.ID]; clash && existing != path {
				p.warnf("slot %q variant %q: duplicate id %q; the later variant was dropped", slot, label, v.ID)
				continue
			}

			if v.Mood != "" && !packMoods[v.Mood] {
				p.warnf("slot %q variant %q: unknown mood %q was ignored", slot, label, v.Mood)
				v.Mood = ""
			}
			switch {
			case v.Rarity == 0: // untagged: joins the base pool
			case v.Rarity < 0 || v.Rarity > 0.5:
				p.warnf("slot %q variant %q: rarity %v is outside (0, 0.5] and was clamped", slot, label, v.Rarity)
				v.Rarity = clampF(v.Rarity, 0, 0.5)
			}
			if r, ok := variantRawNumber(rawSlots, slot, i, "rarity"); ok && r == 0 {
				p.warnf("slot %q variant %q: rarity 0 never displays; the variant was moved into the base pool", slot, label)
				v.Rarity = 0
			}
			if v.Weight <= 0 {
				v.Weight = 1
			}

			// Focus is the authored crop anchor, in 0..1 of the image. The host
			// renders art object-fit:cover, so a composition whose subject sits
			// off-centre is cropped off-centre; this is the only knob that
			// corrects it, and a generator cannot set it without knowing where
			// the face is.
			//
			// The raw check is load-bearing. Both fields are float64, so an
			// ABSENT key and an explicit 0 both decode to 0 — and 0 is a
			// meaningful value here (the left edge, the top edge). Without
			// this, every variant that omits focusX would be pinned to the left
			// edge instead of centred. Same reasoning as `rarity` above.
			if _, ok := variantRawNumber(rawSlots, slot, i, "focusX"); !ok {
				v.FocusX = 0.5
			} else if v.FocusX < 0 || v.FocusX > 1 {
				p.warnf("slot %q variant %q: focusX %v is outside 0..1 and was clamped", slot, label, v.FocusX)
				v.FocusX = clampF(v.FocusX, 0, 1)
			}
			if _, ok := variantRawNumber(rawSlots, slot, i, "focusY"); !ok {
				// Top, not centre: these are portraits, and the face is at the
				// top. This matches the host's previous hard-coded anchor, so
				// every existing pack renders exactly as it did before.
				v.FocusY = 0
			} else if v.FocusY < 0 || v.FocusY > 1 {
				p.warnf("slot %q variant %q: focusY %v is outside 0..1 and was clamped", slot, label, v.FocusY)
				v.FocusY = clampF(v.FocusY, 0, 1)
			}

			if v.When != "" {
				switch {
				case packConditions[v.When] == "":
					p.warnf("slot %q variant %q: unknown condition %q was ignored (valid: %s)", slot, label, v.When, strings.Join(sortedCondNames(), ", "))
					v.When = ""
				case background:
					p.warnf("slot %q variant %q: condition %q was ignored — background slots render their first variant only, with no eggs and no mood", slot, label, v.When)
					v.When = ""
				case !packCondAllowed(v.When, slot):
					p.warnf("slot %q variant %q: condition %q can never hold while %q is showing, so this art would never display", slot, label, v.When, slot)
					v.When = ""
				case packConditions[v.When] == "ambient":
					ambient = append(ambient, v.When)
				}
			}

			validatePackAnim(p, dir, slot, label, &v, ids)
			validatePackMotion(p, dir, slot, label, &v, ids)
			ids[v.ID] = path
			kept = append(kept, v)
		}

		if len(ambient) > 1 {
			sort.Strings(ambient)
			// Giving one condition several weighted variants is the normal
			// authoring pattern (e.g. three different `when: "lateNight"`
			// variants), and must not warn — only genuinely DIFFERENT
			// conditions that can be simultaneously true are worth flagging.
			uniq := ambient[:1]
			for _, c := range ambient[1:] {
				if c != uniq[len(uniq)-1] {
					uniq = append(uniq, c)
				}
			}
			if len(uniq) > 1 {
				p.warnf("slot %q declares ambient conditions %s, which can all hold at once; the pick between them is weighted, never manifest order", slot, strings.Join(uniq, ", "))
			}
		}
		p.Slots[slot] = kept
	}

	if len(p.Slots["idle"]) == 0 {
		p.warnf("no usable variants in the required `idle` slot; the room will show the built-in placeholder")
	}
}

// resolvePackFile whitelists the extension and confirms the file exists,
// returning its absolute, symlink-resolved path. File paths inside a
// manifest must resolve relative to the pack folder and must not be able to
// escape it.
//
// Containment is checked twice: once lexically (cheap, catches the common
// "../" and absolute-path cases without touching the filesystem) and once
// after filepath.EvalSymlinks on both sides. The lexical check alone is not
// a real guarantee — filepath.Abs/Rel compare strings and do not resolve
// symlinks, so a `.png` symlink (or a symlink somewhere in the path, e.g. a
// symlinked subdirectory) whose target lives outside the pack would pass it
// under its link name and then be served same-origin into the deck webview.
// Packs are third-party archives that users download and share, and
// archives preserve symlinks, so this is not a hypothetical.
//
// The path returned is the fully resolved real path (not the original,
// possibly-symlinked one), which also makes servePackMedia's later os.Lstat
// symlink check meaningful: a legitimate pack that used a symlink to share
// one file across two variants still serves correctly, because what got
// registered is the real file, not the link.
func resolvePackFile(p *Pack, dir, slot, label, file string) (string, bool) {
	if file == "" {
		p.warnf("slot %q variant %q has no file and was dropped", slot, label)
		return "", false
	}
	ext := strings.ToLower(filepath.Ext(file))
	if !packMediaExt[ext] {
		p.warnf("slot %q variant %q: %q is not a servable image type and was dropped (allowed: .png, .webp, .jpg, .jpeg)", slot, label, file)
		return "", false
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		p.warnf("slot %q variant %q: %q could not be resolved and was dropped: %v", slot, label, file, err)
		return "", false
	}
	abs, err := filepath.Abs(filepath.Join(absDir, file))
	if err != nil {
		p.warnf("slot %q variant %q: %q could not be resolved and was dropped: %v", slot, label, file, err)
		return "", false
	}
	rel, err := filepath.Rel(absDir, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		p.warnf("slot %q variant %q: %q resolves outside the pack folder and was dropped", slot, label, file)
		return "", false
	}
	realDir, err := filepath.EvalSymlinks(absDir)
	if err != nil {
		p.warnf("slot %q variant %q: the pack folder could not be resolved and was dropped: %v", slot, label, err)
		return "", false
	}
	// A broken symlink, or a plain missing file, both fail here — same
	// warning either way, since from the author's side they look identical.
	realAbs, err := filepath.EvalSymlinks(abs)
	if err != nil {
		p.warnf("slot %q variant %q: %q is missing from the pack folder and was dropped: %v", slot, label, file, err)
		return "", false
	}
	relReal, err := filepath.Rel(realDir, realAbs)
	if err != nil || relReal == ".." || strings.HasPrefix(relReal, ".."+string(filepath.Separator)) {
		p.warnf("slot %q variant %q: %q resolves outside the pack folder and was dropped", slot, label, file)
		return "", false
	}
	fi, err := os.Stat(realAbs)
	if err != nil || fi.IsDir() {
		p.warnf("slot %q variant %q: %q is missing from the pack folder and was dropped", slot, label, file)
		return "", false
	}
	return realAbs, true
}

// validatePackAnim clamps the sprite-strip limits and registers the strip as
// its own media id (<variant>-strip) so the page can address it.
func validatePackAnim(p *Pack, dir, slot, label string, v *PackVariant, ids map[string]string) {
	if v.Anim == nil {
		return
	}
	strip, ok := resolvePackFile(p, dir, slot, label, v.Anim.Strip)
	if !ok {
		v.Anim = nil
		return
	}
	if v.Anim.Frames < 1 || v.Anim.Frames > 24 {
		p.warnf("slot %q variant %q: %d animation frames is outside 1-24 and was clamped", slot, label, v.Anim.Frames)
		v.Anim.Frames = clampI(v.Anim.Frames, 1, 24)
	}
	if v.Anim.FPS < 1 || v.Anim.FPS > 12 {
		p.warnf("slot %q variant %q: %d fps is outside 1-12 and was clamped", slot, label, v.Anim.FPS)
		v.Anim.FPS = clampI(v.Anim.FPS, 1, 12)
	}
	// The strip shares the same id namespace as every other media id (a
	// variant could declare `id: "x-strip"` explicitly), so it must go
	// through the same clash check the main id assignment does — otherwise
	// whichever of the two is validated second silently overwrites the
	// other's map entry with no warning, and a request for that id serves
	// the wrong image.
	stripID := v.ID + "-strip"
	if existing, clash := ids[stripID]; clash && existing != strip {
		p.warnf("slot %q variant %q: sprite-strip id %q collides with another media id; the animation was dropped", slot, label, stripID)
		v.Anim = nil
		return
	}
	ids[stripID] = strip
}

// validatePackMotion resolves the optional animated `motion` file and registers
// it under a derived id, mirroring validatePackAnim's `-strip` convention. The
// still `file` remains the poster shown when motion is disallowed.
func validatePackMotion(p *Pack, dir, slot, label string, v *PackVariant, ids map[string]string) {
	if v.Motion == "" {
		return
	}
	path, ok := resolvePackFile(p, dir, slot, label, v.Motion)
	if !ok {
		v.Motion = ""
		return
	}
	id := v.ID + "-motion"
	if existing, clash := ids[id]; clash && existing != path {
		p.warnf("slot %q variant %q: motion id %q collides with another media id; motion was dropped", slot, label, id)
		v.Motion = ""
		return
	}
	ids[id] = path
	v.MotionID = id
}

// packContentID is the default identity: the first 16 hex chars of the file's
// SHA-256. Cache correctness comes from this hash being in the URL, not from
// cache headers, which the WebKit scheme handler is not demonstrably honouring.
func packContentID(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// packIDSafe keeps ids to characters that survive a URL path segment intact —
// and, as a side effect, guarantees no id can ever contain a separator.
func packIDSafe(id string) bool {
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return id != "" && id != "." && id != ".."
}

func sortedCondNames() []string {
	out := make([]string, 0, len(packConditions))
	for k := range packConditions {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// variantRawNumber reads one numeric key straight from the untyped manifest,
// which is the only way to tell "rarity: 0" from "rarity absent".
func variantRawNumber(rawSlots map[string][]map[string]json.RawMessage, slot string, i int, key string) (float64, bool) {
	if i >= len(rawSlots[slot]) {
		return 0, false
	}
	r, ok := rawSlots[slot][i][key]
	if !ok {
		return 0, false
	}
	var f float64
	if json.Unmarshal(r, &f) != nil {
		return 0, false
	}
	return f, true
}

// companionPacksDir is where packs are expected to live: beside config.toml,
// which configPath() already redirects under COMMANDER_CONFIG, so tests and
// alternate profiles get their own. Packs are user data and are never
// committed (§11).
func companionPacksDir() string {
	return filepath.Join(filepath.Dir(configPath()), "packs")
}

// setCompanionPack validates a chosen folder (must be a directory containing
// manifest.json), persists it, and loads it so the media map is populated
// before the page issues its first request. Validation failures (empty path,
// not a directory, no manifest.json) all happen BEFORE anything is written,
// so those leave the config unchanged. Persistence itself is not rolled
// back: PackPath is saved before LoadCompanionPack() runs, so a failure from
// that call (in practice unreachable here — dir was just validated non-empty
// and LoadCompanionPack's only error case is an empty PackPath) would still
// leave the new path persisted. Returns the folder's base name for the
// Settings label.
func (a *App) setCompanionPack(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("no folder chosen")
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("unreadable: %w", err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("a pack is a folder, not a file")
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		return "", fmt.Errorf("%s has no manifest.json", filepath.Base(dir))
	}
	if err := a.saveAndSyncCompanion(func(c *CompanionConfig) { c.PackPath = dir }); err != nil {
		return "", err
	}
	if _, err := a.LoadCompanionPack(); err != nil {
		return "", err
	}
	return filepath.Base(dir), nil
}

// ClearCompanionPack forgets the configured pack and drops the served id map.
// Dropping the map matters: /media/pack/<id> is an id-map lookup, so leaving it
// populated would keep serving art from a pack the user just disowned.
//
// Clearing an already-clear config is not an error — the UI's Clear button is
// idempotent and must not surface a toast for a no-op.
func (a *App) ClearCompanionPack() error {
	if err := a.saveAndSyncCompanion(func(c *CompanionConfig) { c.PackPath = "" }); err != nil {
		return err
	}
	a.packMu.Lock()
	a.packPaths = nil
	a.packMu.Unlock()
	return nil
}

// PickCompanionPack opens a native directory picker, validates and persists
// the choice. Modelled on PickFolder (app.go:1537), not on the avatar kind's
// model picker, which is a single-file dialog. Cancel returns ("", nil).
func (a *App) PickCompanionPack() (string, error) {
	_ = os.MkdirAll(companionPacksDir(), 0o755) // so the dialog opens somewhere useful
	dir, err := wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:            "Choose a companion pack folder",
		DefaultDirectory: companionPacksDir(),
	})
	if err != nil || dir == "" { // cancel -> ""
		return "", err
	}
	return a.setCompanionPack(dir)
}

// LoadCompanionPack parses the configured pack and rebuilds the media map.
// The parsed pack crosses a Wails binding rather than HTTP, so the manifest
// itself never travels over a URL and no origin question arises (§6.5).
//
// The ONLY error case is "no pack configured", which the UI branches on to
// show the built-in placeholder (§5.11). Every other problem is a warning.
func (a *App) LoadCompanionPack() (Pack, error) {
	dir := a.GetCompanionConfig().PackPath
	if dir == "" {
		return Pack{Slots: map[string][]PackVariant{}}, fmt.Errorf("no companion pack configured")
	}
	p, ids := loadPack(dir)
	a.packMu.Lock()
	a.packPaths = ids
	a.packMu.Unlock()
	return p, nil
}

// mediaPackPrefix is DELIBERATELY neutral. main.go registers this route and
// main.go is neither overridden nor deleted by the public export, whose leak
// tripwire greps file content — a route literal naming the feature there would
// abort the export (§8.1).
const mediaPackPrefix = "/media/pack/"

// packMediaHandler is the asset-server fallback: wails calls it for any GET
// the embedded assets answer with os.ErrNotExist, on macOS (wails://wails),
// Windows (the WebView2 request filter) and `wails dev` alike. Requests are
// same-origin, so there is no CORS problem and no loopback port to discover.
func (a *App) packMediaHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(mediaPackPrefix, a.servePackMedia)
	return mux
}

// servePackMedia serves one pack image by id.
//
// The id is a MAP KEY, never a path: the request cannot name a file, only an
// entry the app itself registered while loading a manifest, and ids are
// restricted to URL-safe characters at load time so no key can contain a
// separator. The extension whitelist is re-checked here and the file is
// re-stat'ed on every serve, because a pack folder is user data that can
// change under us between load and request. http.ServeContent gives correct
// Last-Modified, Range and Content-Type handling for free.
func (a *App) servePackMedia(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, mediaPackPrefix)
	if id == "" || strings.ContainsAny(id, `/\`) {
		http.NotFound(w, r)
		return
	}
	a.packMu.Lock()
	path, ok := a.packPaths[id]
	if !ok {
		// Art from a pack that is still being generated, so the wizard can show
		// it before it is saved. Both maps are populated by loadPack, so a
		// preview path carries exactly the same containment guarantees, and
		// every check below applies to it unchanged.
		path, ok = a.packGenPreviews[id]
	}
	if !ok {
		// Thumbnails of packs in the library that are not the active one.
		path, ok = a.packLibrary[id]
	}
	a.packMu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !packMediaExt[strings.ToLower(filepath.Ext(path))] {
		http.NotFound(w, r)
		return
	}
	// A pack folder is user data that can change under us between load and
	// request. resolvePackFile stores the symlink-resolved real path, so a
	// symlink found here can only be one planted AFTER load, at a path that
	// used to be (and was validated as) a plain file — a classic TOCTOU. Lstat
	// (which does NOT follow the final symlink, unlike Stat) is what lets us
	// tell the difference and refuse to serve through it.
	if fi, err := os.Lstat(path); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, filepath.Base(path), fi.ModTime(), f)
}
