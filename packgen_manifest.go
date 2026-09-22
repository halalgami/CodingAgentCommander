package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The manifest writer.
//
// One requirement drives every decision here: a pack this code writes must load
// with ZERO warnings. Warnings exist to tell a human their manifest is wrong,
// so a generated pack that trips them is telling the user their own purchase is
// broken. Meeting that bar is not automatic — the loader warns about several
// things that a naive `json.Marshal(pack)` produces:
//
//   - `warnings` is a field on Pack but is NOT in packTopKeys, so marshalling
//     the loader's own struct emits a key the loader then calls unknown.
//   - `rarity: 0` is warned about explicitly ("rarity 0 never displays"), and
//     it is the zero value, so every variant would carry it.
//   - a missing or zero-sized `canvas` warns.
//   - a missing `schema` warns.
//
// So the writer has its own types with omitempty rather than reusing Pack. The
// round-trip test is the real specification; these types are how it passes.

type packManifestAnim struct {
	Strip  string `json:"strip"`
	Frames int    `json:"frames"`
	FPS    int    `json:"fps"`
}

type packManifestVariant struct {
	File   string  `json:"file"`
	Mood   string  `json:"mood,omitempty"`
	When   string  `json:"when,omitempty"`
	Weight float64 `json:"weight,omitempty"`
	Rarity float64 `json:"rarity,omitempty"`
	// Pointers, because 0 is a MEANINGFUL focus value (the left edge, the top
	// edge) and omitempty on a float64 would silently drop it. Same trap the
	// loader has to work around from the other side.
	FocusX *float64          `json:"focusX,omitempty"`
	FocusY *float64          `json:"focusY,omitempty"`
	Anim   *packManifestAnim `json:"anim,omitempty"`
	Meta   map[string]string `json:"meta,omitempty"`
}

type packManifest struct {
	Schema  int                              `json:"schema"`
	Name    string                           `json:"name"`
	Author  string                           `json:"author,omitempty"`
	License string                           `json:"license,omitempty"`
	Canvas  PackCanvas                       `json:"canvas"`
	Alpha   bool                             `json:"alpha"`
	Slots   map[string][]packManifestVariant `json:"slots"`
}

// packGenArt is one finished image on its way into a pack.
type packGenArt struct {
	Slot  string
	When  string
	Mood  string
	Image []byte
	MIME  string

	// Crop anchor, carried through a regeneration so a variant the user has
	// already nudged is not silently re-centred by rewriting the manifest.
	//
	// Pointers because the ZERO value is meaningful (the left edge, the top
	// edge) and is NOT the default — an absent focusX means 0.5. A plain
	// float64 here would have emitted focusX:0 on every freshly generated
	// variant and pinned all of them to the left.
	FocusX *float64
	FocusY *float64

	// Provenance, written into the variant's meta. The pack format reserved
	// these fields for exactly this purpose: a user who likes one image should
	// be able to see what produced it.
	Prompt      string
	Model       string
	Seed        int64
	GeneratedAt time.Time
}

// packGenAuthorNote is written as the license of every generated pack. It is
// not a legal claim; it is a reminder to the person who finds this folder in
// six months and no longer remembers where the art came from.
const packGenAuthorNote = "Generated in Commander. Rights follow your image provider's terms and the rights in the source portrait."

// packGenIdentityMethod records HOW identity was held, so a future version that
// changes technique can tell old packs from new ones.
const packGenIdentityMethod = "instruction-edit-from-base"

// writePackManifest writes every image plus manifest.json into dir, which must
// already exist and should be a staging folder. Returns the number of image
// files actually written, which is not necessarily len(arts) — see the
// deduplication note below.
func writePackManifest(dir, displayName string, arts []packGenArt) (int, error) {
	if len(arts) == 0 {
		return 0, fmt.Errorf("no art to write")
	}
	if !packGenIsDir(dir) {
		return 0, fmt.Errorf("staging folder %s does not exist", dir)
	}

	slots := map[string][]packManifestVariant{}
	byDigest := map[string]string{} // content digest -> file name already written
	usedNames := map[string]bool{}
	var canvas PackCanvas
	var alpha bool
	var written int

	for _, a := range arts {
		if !packSlotKnown(a.Slot) {
			return written, fmt.Errorf("unknown slot %q", a.Slot)
		}
		facts, err := packGenImageFacts(a.Image)
		if err != nil {
			return written, fmt.Errorf("slot %q: %w", a.Slot, err)
		}

		// Deduplicate by content, not by slot. Two variants pointing at the
		// SAME file are explicitly supported by the loader (it reuses the id
		// and keeps both), but two DIFFERENT files with identical bytes hash
		// to the same content id and the loader drops the second as a
		// duplicate. A provider that serves a cached response for two similar
		// prompts would otherwise silently cost the user a slot.
		digest := hex.EncodeToString(sha256Bytes(a.Image))
		name, seen := byDigest[digest]
		if !seen {
			name = packGenFileName(a, facts.Ext, usedNames)
			if err := os.WriteFile(filepath.Join(dir, name), a.Image, 0o644); err != nil {
				return written, fmt.Errorf("writing %s: %w", name, err)
			}
			usedNames[name] = true
			byDigest[digest] = name
			written++
		}

		// The canvas is pack-wide but the images need not agree on size, so the
		// first one seen defines it. It is advisory: the sidebar host renders
		// object-fit:cover, and the loader only warns when it is absent or
		// zero-sized.
		if canvas.W == 0 {
			canvas = PackCanvas{W: facts.W, H: facts.H, Scale: 1, Anchor: "top-center"}
		}
		// alpha describes the PACK, so any transparent image makes it true.
		// Derived from the GENERATED art rather than assumed: the hosted models
		// return opaque frames today, but declaring alpha:false over an image
		// that does have transparency would composite a black rectangle.
		alpha = alpha || facts.HasAlpha

		v := packManifestVariant{
			File: name,
			Mood: a.Mood,
			When: a.When,
			Meta: packGenMeta(a),
		}
		// Carried only when set, so a fresh pack's manifest stays free of fields
		// nobody chose and keeps rendering from the loader's defaults.
		v.FocusX, v.FocusY = a.FocusX, a.FocusY
		slots[a.Slot] = append(slots[a.Slot], v)
	}

	m := packManifest{
		Schema:  packSchemaMajor,
		Name:    strings.TrimSpace(displayName),
		Author:  "Commander",
		License: packGenAuthorNote,
		Canvas:  canvas,
		Alpha:   alpha,
		Slots:   slots,
	}
	if m.Name == "" {
		m.Name = "Generated pack"
	}

	// Indented and newline-terminated: this file is meant to be opened, read
	// and hand-edited by someone extending their own pack.
	buf, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return written, fmt.Errorf("encoding manifest: %w", err)
	}
	buf = append(buf, '\n')
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), buf, 0o644); err != nil {
		return written, fmt.Errorf("writing manifest.json: %w", err)
	}
	return written, nil
}

func packGenMeta(a packGenArt) map[string]string {
	// PackVariant.Meta is map[string]string, so every value must be a string —
	// a numeric seed written as a JSON number silently fails to unmarshal and
	// the whole meta object is lost.
	meta := map[string]string{
		"tool":           "Commander",
		"identityMethod": packGenIdentityMethod,
	}
	if a.Model != "" {
		meta["model"] = a.Model
	}
	if a.Prompt != "" {
		meta["prompt"] = a.Prompt
	}
	if a.Seed != 0 {
		meta["seed"] = fmt.Sprint(a.Seed)
	}
	if !a.GeneratedAt.IsZero() {
		meta["generatedAt"] = a.GeneratedAt.UTC().Format(time.RFC3339)
	}
	return meta
}

// packGenFileName builds a readable, collision-free file name. Readability
// matters because the folder is the hand-editing surface for anyone who wants
// to swap one image out.
func packGenFileName(a packGenArt, ext string, used map[string]bool) string {
	parts := []string{a.Slot}
	if a.When != "" {
		parts = append(parts, a.When)
	}
	if a.Mood != "" {
		parts = append(parts, a.Mood)
	}
	// The slot and condition vocabularies are closed and already safe, but the
	// name still goes through the slug whitelist so this function cannot become
	// a path-injection point if a future caller passes something else.
	base, err := packGenSlug(strings.Join(parts, "-"))
	if err != nil {
		base = "variant"
	}
	name := base + ext
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
	return name
}

// packGenFacts is what the writer needs to know about an image, read from the
// bytes rather than from what the provider claimed in a header.
type packGenFacts struct {
	W, H     int
	Ext      string
	HasAlpha bool
}

// packGenImageFacts decodes an image's header, then the image itself, within
// the same bounds imagegen uses. The format comes from sniffing, never from the
// provider's Content-Type: the extension decides what the loader will serve and
// what the webview will try to render, so a wrong one produces a broken image
// rather than an error.
func packGenImageFacts(raw []byte) (packGenFacts, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return packGenFacts{}, fmt.Errorf("not a decodable image: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > packGenMaxDim || cfg.Height > packGenMaxDim {
		return packGenFacts{}, fmt.Errorf("implausible image dimensions %dx%d", cfg.Width, cfg.Height)
	}
	if px := int64(cfg.Width) * int64(cfg.Height); px > packGenMaxPixels {
		return packGenFacts{}, fmt.Errorf("image too large: %d pixels", px)
	}

	var ext string
	switch format {
	case "png":
		ext = ".png"
	case "jpeg":
		ext = ".jpg"
	default:
		// Only PNG and JPEG decoders are registered, so this is unreachable
		// via image.DecodeConfig today; it exists so that registering another
		// format elsewhere cannot silently produce a file the pack loader's
		// extension whitelist then refuses to serve.
		return packGenFacts{}, fmt.Errorf("unsupported image format %q", format)
	}

	facts := packGenFacts{W: cfg.Width, H: cfg.Height, Ext: ext}
	if ext == ".png" {
		img, _, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			return packGenFacts{}, fmt.Errorf("decoding image: %w", err)
		}
		// Every stdlib image type that can carry alpha implements Opaque().
		// A type that does not is treated as opaque, which is the safe
		// direction: it means the host composites onto its own background
		// instead of trusting a transparency that may not exist.
		if o, ok := img.(interface{ Opaque() bool }); ok {
			facts.HasAlpha = !o.Opaque()
		}
	}
	return facts, nil
}

// Same bounds and same reasoning as internal/imagegen: the per-side cap rejects
// absurd aspect ratios early, and the pixel count is the actual memory bound.
const (
	packGenMaxDim    = 16384
	packGenMaxPixels = 40 << 20

	// packGenMaxBaseBytes caps a base portrait BEFORE it is decoded. The pixel
	// bound above is a bound on the decoded image, which does not help if the
	// file itself is a 4 GiB read: the process is already out of memory by the
	// time a decoder sees a header. Any real portrait is orders of magnitude
	// under this.
	packGenMaxBaseBytes = 64 << 20
)

// readFileLimited reads at most max bytes, and reports a file that exceeds the
// cap as an error rather than silently returning a truncated prefix — truncated
// image bytes would fail to decode with a message about corruption, sending the
// user to look for a problem with their file that does not exist.
func readFileLimited(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("%s is a folder, not an image file", filepath.Base(path))
	}
	if fi.Size() > max {
		return nil, fmt.Errorf("%s is %d MB, which is far larger than any portrait needs to be (limit %d MB)",
			filepath.Base(path), fi.Size()>>20, max>>20)
	}
	// LimitReader as well as the size check: the file can grow between the stat
	// and the read.
	return io.ReadAll(io.LimitReader(f, max))
}

func sha256Bytes(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

// packGenSortArts orders art the way the slots are presented, so a manifest
// read by a human lists idle first rather than in completion order.
func packGenSortArts(arts []packGenArt) {
	rank := map[string]int{}
	for i, s := range packGenSlotOrder {
		rank[s] = i
	}
	sort.SliceStable(arts, func(i, j int) bool {
		ri, rj := rank[arts[i].Slot], rank[arts[j].Slot]
		if ri != rj {
			return ri < rj
		}
		// Unconditioned variants first: they are the slot's base art.
		return arts[i].When == "" && arts[j].When != ""
	})
}
