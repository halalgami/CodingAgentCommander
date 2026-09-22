package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Regenerating ONE variant of a saved pack.
//
// Re-running six images because one of them came out wrong is the obvious way
// to waste money, and the obvious fix — edit the live pack in place — is the
// one that can destroy it. So regeneration reuses the path that already works:
// copy the saved pack into staging, replace only what was asked for, and
// promote. Every guarantee the full run has (a manifest that loads clean,
// previews, cancel, nothing deleted except by discard) applies unchanged, and
// a failed regeneration leaves the saved pack exactly as it was.

// packGenBaseStem names the base portrait kept INSIDE the pack.
//
// Without it, regenerating a slot months later means finding the original file
// again — and if it has moved or been deleted, the pack can never be extended,
// only replaced. The underscore keeps it out of the way, and loadPack never
// enumerates the directory (it reads only manifest-referenced files), so an
// unreferenced file here is ignored rather than warned about or served.
const packGenBaseStem = "_base"

// writePackBase copies the base portrait into the pack folder.
func writePackBase(dir string, base []byte) error {
	facts, err := packGenImageFacts(base)
	if err != nil {
		return fmt.Errorf("the base portrait is not a usable image: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, packGenBaseStem+facts.Ext), base, 0o644)
}

// findPackBase returns the stored base portrait's bytes, if the pack has one.
func findPackBase(dir string) ([]byte, bool) {
	for _, ext := range []string{".png", ".jpg", ".jpeg"} {
		p := filepath.Join(dir, packGenBaseStem+ext)
		if raw, err := readFileLimited(p, packGenMaxBaseBytes); err == nil {
			if _, err := packGenImageFacts(raw); err == nil {
				return raw, true
			}
		}
	}
	return nil, false
}

// loadPackGenArts rebuilds the run's art list from a pack already on disk, so a
// regeneration can rewrite the whole manifest from one place — including the
// variants it is NOT touching. Reconstructing them rather than merging into the
// existing manifest means there is exactly one code path that writes a
// manifest, and therefore one place where "loads with zero warnings" is true.
//
// It reads through loadPack rather than parsing the manifest directly, so a
// variant the loader would drop is dropped here too: a regeneration cannot
// resurrect art that does not load.
func loadPackGenArts(dir string) ([]packGenArt, error) {
	p, ids := loadPack(dir)
	if len(ids) == 0 {
		return nil, fmt.Errorf("no usable images found in %s", dir)
	}
	var out []packGenArt
	for slot, variants := range p.Slots {
		for _, v := range variants {
			path, ok := ids[v.ID]
			if !ok {
				continue
			}
			raw, err := readFileLimited(path, packGenMaxBaseBytes)
			if err != nil {
				return nil, fmt.Errorf("reading %s: %w", v.File, err)
			}
			out = append(out, packGenArt{
				Slot: slot, When: v.When, Mood: v.Mood,
				Image: raw, MIME: packGenMIME(strings.ToLower(filepath.Ext(path))),
				Prompt:      v.Meta["prompt"],
				Model:       v.Meta["model"],
				Seed:        parseSeed(v.Meta["seed"]),
				GeneratedAt: parseStamp(v.Meta["generatedAt"]),
				// Only carried when the author actually chose something. The
				// loader materialises its defaults (0.5 / 0) onto every
				// variant, so copying them unconditionally would stamp
				// focusX/focusY onto every entry of a regenerated manifest.
				FocusX: nonDefaultFocus(v.FocusX, 0.5),
				FocusY: nonDefaultFocus(v.FocusY, 0),
			})
		}
	}
	packGenSortArts(out)
	return out, nil
}

// nonDefaultFocus returns a pointer only when the value differs from what an
// absent key would have produced.
func nonDefaultFocus(v, def float64) *float64 {
	if v == def {
		return nil
	}
	out := v
	return &out
}

func parseSeed(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func parseStamp(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// copyPackTree copies a pack folder's files into dst.
//
// Flat by design: a pack is a manifest plus images in one directory, and
// recursing would follow whatever a user put in a subfolder. Symlinks are
// skipped rather than followed for the same reason resolvePackFile refuses
// them — a pack is a third-party archive, and archives preserve symlinks.
func copyPackTree(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// PackVariantInfo is one saved variant, as the browser shows it.
type PackVariantInfo struct {
	Slot    string  `json:"slot"`
	When    string  `json:"when"`
	Mood    string  `json:"mood,omitempty"`
	File    string  `json:"file"`
	Preview string  `json:"preview"` // media id, served at /media/pack/<id>
	FocusX  float64 `json:"focusX"`
	FocusY  float64 `json:"focusY"`

	// Provenance, straight from the variant's meta. Empty for a hand-authored
	// pack, which is a fact worth showing rather than hiding.
	Prompt      string `json:"prompt,omitempty"`
	Model       string `json:"model,omitempty"`
	Seed        string `json:"seed,omitempty"`
	GeneratedAt string `json:"generatedAt,omitempty"`
}

// PackBrowse is the saved pack, for the browser.
type PackBrowse struct {
	Name     string            `json:"name"`
	Path     string            `json:"path"`
	HasBase  bool              `json:"hasBase"`
	Variants []PackVariantInfo `json:"variants"`
	Warnings []string          `json:"warnings"`
	// Missing is true when the configured folder is gone. The UI shows that as
	// a specific state rather than as an empty pack, which would read as "your
	// art vanished" instead of "the folder moved".
	Missing bool `json:"missing"`
}

// PackMissing reports that a pack is configured but could not be loaded — the
// folder was deleted or renamed outside the app. Distinct from "no pack
// configured", which is a normal state, and from a pack that simply has no
// usable art.
//
// It exists because loadPack never errors, so the binding used to return a
// perfectly valid struct describing nothing and the UI could not tell the two
// apart.

// BrowseCompanionPack lists the ACTIVE pack's variants with their provenance.
//
// HasBase decides whether regeneration is offered at all: a pack saved before
// the base was kept inside it, or one assembled by hand, has nothing to
// regenerate from, and offering a button that cannot work is worse than not
// offering it.
func (a *App) BrowseCompanionPack() (PackBrowse, error) {
	dir := a.GetCompanionConfig().PackPath
	if dir == "" {
		return PackBrowse{}, fmt.Errorf("no companion pack configured")
	}
	p, ids := loadPack(dir)
	_, hasBase := findPackBase(dir)
	missing := !packGenIsDir(dir)

	// Both slices are initialised, never left nil. A nil slice marshals to
	// `null`, and the frontend reads `saved.variants.length` — so a pack that
	// is configured but unloadable (folder deleted or renamed outside the app)
	// threw in the browser instead of rendering an empty state.
	out := PackBrowse{
		Name: p.Name, Path: dir, HasBase: hasBase,
		Variants: []PackVariantInfo{},
		Warnings: []string{},
		Missing:  missing,
	}
	if len(p.Warnings) > 0 {
		out.Warnings = p.Warnings
	}
	for _, slot := range packGenSlotOrder {
		for _, v := range p.Slots[slot] {
			if _, ok := ids[v.ID]; !ok {
				continue
			}
			out.Variants = append(out.Variants, PackVariantInfo{
				Slot: slot, When: v.When, Mood: v.Mood, File: v.File, Preview: v.ID,
				FocusX: v.FocusX, FocusY: v.FocusY,
				Prompt: v.Meta["prompt"], Model: v.Meta["model"],
				Seed: v.Meta["seed"], GeneratedAt: v.Meta["generatedAt"],
			})
		}
	}
	return out, nil
}

// SetPackVariantFocus rewrites one variant's crop anchor in the SAVED pack.
//
// This is the cheap half of "the art is not centred": the model composes each
// image independently, so one slot sits off-centre and another does not, and
// nudging the crop costs nothing where regenerating costs another image. It
// edits the live manifest directly — the only in-place pack write in the
// codebase — because it changes two numbers and touches no image, so the
// staging dance would be ceremony. The file is rewritten atomically.
func (a *App) SetPackVariantFocus(slot, when, mood string, focusX, focusY float64) error {
	dir := a.GetCompanionConfig().PackPath
	if dir == "" {
		return fmt.Errorf("no companion pack configured")
	}
	if focusX < 0 || focusX > 1 || focusY < 0 || focusY > 1 {
		return fmt.Errorf("focus must be between 0 and 1")
	}
	path := filepath.Join(dir, "manifest.json")
	raw, err := readFileLimited(path, 8<<20)
	if err != nil {
		return err
	}
	// Decoded into a generic map, not the writer's structs: a hand-authored
	// pack may carry keys this build has never heard of, and rewriting through
	// a typed struct would silently delete them.
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("manifest.json is not valid JSON: %w", err)
	}
	slots, _ := doc["slots"].(map[string]any)
	list, _ := slots[slot].([]any)
	var found bool
	for _, item := range list {
		v, ok := item.(map[string]any)
		if !ok {
			continue
		}
		// Match on mood as well as condition. loadPack calls several variants
		// per (slot, when) "the normal authoring pattern", and the browser
		// renders each as its own row — so keying on `when` alone silently
		// moved the crop of a DIFFERENT row than the one the user dragged.
		vw, _ := v["when"].(string)
		vm, _ := v["mood"].(string)
		if vw != when || vm != mood {
			continue
		}
		v["focusX"] = focusX
		v["focusY"] = focusY
		found = true
		break
	}
	if !found {
		return fmt.Errorf("no matching variant in slot %q (condition %q, mood %q)", slot, when, mood)
	}
	buf, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	if err := writeFileAtomic(path, buf); err != nil {
		return err
	}
	_, err = a.LoadCompanionPack()
	return err
}

// atomicRename is a seam, for the same reason promoteRename is one: the only
// interesting failure is the rename itself failing after the temp file is
// written, and there is no portable way to make a filesystem refuse exactly
// that from a test.
var atomicRename = os.Rename

// writeFileAtomic writes via a temp file in the same directory then renames, so
// an interrupted write cannot leave a half-written manifest — which would make
// the pack unloadable and take the art with it.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".manifest-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // no-op once the rename below succeeds
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return atomicRename(name, path)
}

// PackSummary is one pack in the library.
type PackSummary struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Folder string `json:"folder"`
	Scenes int    `json:"scenes"`
	// Thumb is the media id of the pack's idle art, served at /media/pack/<id>.
	// Empty for a pack with no usable idle image, which the UI shows as a
	// placeholder rather than a broken picture.
	Thumb    string `json:"thumb"`
	HasBase  bool   `json:"hasBase"`
	Active   bool   `json:"active"`
	Warnings int    `json:"warnings"`
}

// ListCompanionPacks enumerates every pack in the packs folder.
//
// Thumbnails are registered in their own id map rather than the active pack's:
// packPaths is REPLACED wholesale on every load, so merging them there would
// make the library's pictures vanish the moment any pack was selected — the
// same trap the staged previews already had to avoid.
func (a *App) ListCompanionPacks() []PackSummary {
	root := companionPacksDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	active := a.GetCompanionConfig().PackPath
	activeReal, _ := filepath.EvalSymlinks(active)

	lib := map[string]string{}
	var out []PackSummary
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Transient folders belong to a run in flight, not to the library.
		// Listing them would offer the user a half-written pack to select.
		if strings.HasSuffix(e.Name(), packGenStagingSuffix) || strings.HasSuffix(e.Name(), packGenOldSuffix) {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
			continue
		}
		p, ids := loadPack(dir)
		_, hasBase := findPackBase(dir)

		sum := PackSummary{
			Name: p.Name, Path: dir, Folder: e.Name(),
			HasBase: hasBase, Warnings: len(p.Warnings),
		}
		if sum.Name == "" {
			sum.Name = e.Name()
		}
		for _, vs := range p.Slots {
			sum.Scenes += len(vs)
		}
		if idle := p.Slots["idle"]; len(idle) > 0 {
			sum.Thumb = idle[0].ID
		}
		for id, path := range ids {
			lib[id] = path
		}
		if real, err := filepath.EvalSymlinks(dir); err == nil && activeReal != "" && real == activeReal {
			sum.Active = true
		}
		out = append(out, sum)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })

	a.packMu.Lock()
	a.packLibrary = lib
	a.packMu.Unlock()
	return out
}

// SelectCompanionPack makes one of the listed packs active.
//
// Thin on purpose: setCompanionPack already validates the folder, persists the
// choice and reloads the media map, and duplicating any of that here would give
// the library a second, weaker validation path.
func (a *App) SelectCompanionPack(dir string) (string, error) {
	return a.setCompanionPack(dir)
}

// DeleteCompanionPack removes a pack folder permanently.
//
// The only destructive operation on saved art in the app, so it is bounded
// hard: the path must resolve to a DIRECT CHILD of the packs directory. That
// is checked after EvalSymlinks on both sides, because a lexical check compares
// strings — a symlink inside the packs folder pointing at, say, a home
// directory would pass one and be deleted by RemoveAll.
//
// The caller confirms. This function does not, because a Go-side prompt cannot
// be shown and a binding that silently no-ops would be worse than one that
// deletes.
func (a *App) DeleteCompanionPack(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("no pack chosen")
	}
	root := companionPacksDir()
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("the packs folder could not be resolved: %w", err)
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("that pack no longer exists: %w", err)
	}
	// A direct child, not merely "somewhere underneath": nesting is not a thing
	// a pack folder does, and allowing it would let a deeper path through.
	// This also rules out the packs folder itself, since Dir(root) is never
	// root. An explicit second check for that case was unreachable, and
	// unreachable defensive code no test can exercise is false comfort.
	if filepath.Dir(realDir) != realRoot {
		return fmt.Errorf("refusing to delete %s: it is not a pack in %s", dir, root)
	}
	fi, err := os.Lstat(realDir)
	if err != nil || !fi.IsDir() {
		return fmt.Errorf("that pack is not a folder")
	}
	// Refuse anything without a manifest. A folder here that is not a pack is
	// something the user put there, and deleting it is not ours to do.
	if _, err := os.Stat(filepath.Join(realDir, "manifest.json")); err != nil {
		return fmt.Errorf("refusing to delete %s: it has no manifest.json, so it is not a pack",
			filepath.Base(realDir))
	}

	wasActive := false
	if active := a.GetCompanionConfig().PackPath; active != "" {
		if realActive, err := filepath.EvalSymlinks(active); err == nil && realActive == realDir {
			wasActive = true
		}
	}
	if err := os.RemoveAll(realDir); err != nil {
		return fmt.Errorf("removing the pack: %w", err)
	}
	// Deleting the ACTIVE pack must also disown it, or the config points at a
	// folder that is gone and the figure keeps serving from a dropped id map.
	if wasActive {
		if err := a.ClearCompanionPack(); err != nil {
			return fmt.Errorf("the pack was deleted but could not be deselected: %w", err)
		}
	}
	// Drop the library thumbnails: their ids now name files that do not exist.
	a.packMu.Lock()
	a.packLibrary = nil
	a.packMu.Unlock()
	return nil
}
