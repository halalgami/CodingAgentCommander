package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/halalgami/CodingAgentCommander/internal/raidpack"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed raidkit/heroine-prompts.csv
var raidPrompts []byte

const mediaRaidPrefix = "/media/raid/"

var raidTint = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type RaidPackView struct {
	Dir      string                      `json:"dir"`
	Name     string                      `json:"name"`
	Files    map[string]string           `json:"files"`
	Heroines map[string]raidpack.Heroine `json:"heroines"`
	Warnings []string                    `json:"warnings"`
}

type RaidPackSummary struct {
	Dir    string `json:"dir"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
	Active bool   `json:"active"`
}

func raidPacksDir() string   { return filepath.Join(filepath.Dir(configPath()), "raidpacks") }
func raidConfigPath() string { return filepath.Join(filepath.Dir(configPath()), "raid.json") }

func readRaidConfig() string {
	var c struct {
		PackDir string `json:"packDir"`
	}
	if b, err := os.ReadFile(raidConfigPath()); err == nil {
		json.Unmarshal(b, &c)
	}
	return c.PackDir
}

func writeRaidConfig(dir string) error {
	b, _ := json.Marshal(map[string]string{"packDir": dir})
	if err := os.MkdirAll(filepath.Dir(raidConfigPath()), 0o755); err != nil {
		return err
	}
	tmp := raidConfigPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, raidConfigPath())
}

// inRaidPacks reports whether dir is a direct child of raidpacks/ (symlinks resolved).
func inRaidPacks(dir string) bool {
	root, err := filepath.EvalSymlinks(raidPacksDir())
	if err != nil {
		return false
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	return filepath.Dir(real) == root
}

// raidView loads the active pack, rebuilds the id map and returns the view.
func (a *App) raidView() RaidPackView {
	a.raidMu.Lock()
	dir := a.raidDir
	a.raidMu.Unlock()
	v := RaidPackView{Files: map[string]string{}, Heroines: map[string]raidpack.Heroine{}}
	ids := map[string]string{}
	if dir != "" {
		p := raidpack.Load(dir)
		v.Dir, v.Name, v.Heroines, v.Warnings = dir, p.Name, p.Heroines, p.Warnings
		for stem, path := range p.Files {
			b, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			sum := sha256.Sum256(b)
			id := stem + "-" + hex.EncodeToString(sum[:4])
			ids[id] = path
			v.Files[stem] = mediaRaidPrefix + id
		}
	}
	a.raidMu.Lock()
	a.raidPaths = ids
	a.raidMu.Unlock()
	return v
}

func (a *App) raidChanged() RaidPackView {
	v := a.raidView()
	a.emitter.Emit("raid:pack", v)
	return v
}

func (a *App) setRaidDir(dir string) error {
	if err := writeRaidConfig(dir); err != nil {
		return err
	}
	a.raidMu.Lock()
	a.raidDir = dir
	a.raidMu.Unlock()
	return nil
}

// RaidPackInfo returns the active pack; an empty view means bundled art.
func (a *App) RaidPackInfo() RaidPackView {
	a.raidOpMu.Lock()
	defer a.raidOpMu.Unlock()
	return a.raidView()
}

func (a *App) ListRaidPacks() []RaidPackSummary {
	ents, _ := os.ReadDir(raidPacksDir())
	a.raidMu.Lock()
	active := a.raidDir
	a.raidMu.Unlock()
	var out []RaidPackSummary
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(raidPacksDir(), e.Name())
		p := raidpack.Load(dir)
		out = append(out, RaidPackSummary{Dir: dir, Name: p.Name, Count: len(p.Files), Active: dir == active})
	}
	return out
}

func (a *App) SelectRaidPack(dir string) (RaidPackView, error) {
	a.raidOpMu.Lock()
	defer a.raidOpMu.Unlock()
	if !inRaidPacks(dir) {
		return RaidPackView{}, fmt.Errorf("not a heroine pack folder")
	}
	if err := a.setRaidDir(dir); err != nil {
		return RaidPackView{}, err
	}
	return a.raidChanged(), nil
}

// ClearRaidPack goes back to the bundled art; packs stay on disk.
func (a *App) ClearRaidPack() (RaidPackView, error) {
	a.raidOpMu.Lock()
	defer a.raidOpMu.Unlock()
	if err := a.setRaidDir(""); err != nil {
		return RaidPackView{}, err
	}
	return a.raidChanged(), nil
}

func (a *App) DeleteRaidPack(dir string) error {
	a.raidOpMu.Lock()
	defer a.raidOpMu.Unlock()
	if !inRaidPacks(dir) {
		return fmt.Errorf("not a heroine pack folder")
	}
	a.raidMu.Lock()
	active := a.raidDir
	a.raidMu.Unlock()
	if filepath.Clean(dir) == filepath.Clean(active) {
		return fmt.Errorf("that pack is in use; choose Use built-in first")
	}
	return os.RemoveAll(dir)
}

// ensureRaidPack returns the active pack folder, creating and activating
// "My heroines" when none is active (spec R4).
func (a *App) ensureRaidPack() (string, error) {
	a.raidMu.Lock()
	dir := a.raidDir
	a.raidMu.Unlock()
	if dir != "" {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			return dir, nil
		}
	}
	if err := os.MkdirAll(raidPacksDir(), 0o755); err != nil {
		return "", err
	}
	dir, err := raidpack.FreeDir(raidPacksDir(), "my-heroines")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := raidpack.WriteManifest(dir, raidpack.Manifest{Name: "My heroines"}); err != nil {
		return "", err
	}
	return dir, a.setRaidDir(dir)
}

func (a *App) ReplaceRaidImage(stem string) (RaidPackView, error) {
	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:   "Choose an image for " + stem,
		Filters: []wruntime.FileFilter{{DisplayName: "Images (*.png, *.webp, *.jpg)", Pattern: "*.png;*.webp;*.jpg;*.jpeg"}},
	})
	if err != nil || path == "" {
		return a.raidView(), err
	}
	return a.ReplaceRaidImageFrom(stem, path)
}

// ReplaceRaidImageFrom stores the image at path as stem in the active pack.
// Everything is checked before a pack is created, so a refusal writes nothing.
func (a *App) ReplaceRaidImageFrom(stem, path string) (RaidPackView, error) {
	a.raidOpMu.Lock()
	defer a.raidOpMu.Unlock()
	if !raidpack.ValidStem(stem) {
		return RaidPackView{}, fmt.Errorf("%q is not a heroine image", stem)
	}
	if !raidpack.ImageExt[strings.ToLower(filepath.Ext(path))] {
		return RaidPackView{}, fmt.Errorf("drop a PNG, JPEG or WebP image")
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return RaidPackView{}, fmt.Errorf("that is not an image file")
	}
	if fi.Size() > raidpack.MaxFile {
		return RaidPackView{}, fmt.Errorf("image is over 32 MiB")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return RaidPackView{}, err
	}
	// Normalise into a scratch folder first so a bad image never creates a pack.
	tmp, err := os.MkdirTemp("", "raidimg")
	if err != nil {
		return RaidPackView{}, err
	}
	defer os.RemoveAll(tmp)
	if err := raidpack.Store(tmp, stem, bytes.NewReader(b)); err != nil {
		return RaidPackView{}, err
	}
	dir, err := a.ensureRaidPack()
	if err != nil {
		return RaidPackView{}, err
	}
	out, err := os.ReadFile(filepath.Join(tmp, stem+".png"))
	if err != nil {
		return RaidPackView{}, err
	}
	if err := raidpack.Store(dir, stem, bytes.NewReader(out)); err != nil {
		return RaidPackView{}, err
	}
	return a.raidChanged(), nil
}

func (a *App) ResetRaidImage(stem string) (RaidPackView, error) {
	a.raidOpMu.Lock()
	defer a.raidOpMu.Unlock()
	a.raidMu.Lock()
	dir := a.raidDir
	a.raidMu.Unlock()
	if dir == "" {
		return a.raidView(), nil
	}
	if err := raidpack.Remove(dir, stem); err != nil {
		return RaidPackView{}, err
	}
	return a.raidChanged(), nil
}

func (a *App) SetRaidHeroine(id, name, tint string) (RaidPackView, error) {
	a.raidOpMu.Lock()
	defer a.raidOpMu.Unlock()
	if !raidpack.ValidID(id) {
		return RaidPackView{}, fmt.Errorf("%q is not a heroine", id)
	}
	if tint != "" && !raidTint.MatchString(tint) {
		return RaidPackView{}, fmt.Errorf("colour must be #rrggbb")
	}
	dir, err := a.ensureRaidPack()
	if err != nil {
		return RaidPackView{}, err
	}
	m, _ := raidpack.ReadManifest(dir)
	if m.Heroines == nil {
		m.Heroines = map[string]raidpack.Heroine{}
	}
	h := raidpack.Heroine{Name: strings.TrimSpace(name), Tint: tint}
	if h == (raidpack.Heroine{}) {
		delete(m.Heroines, id)
	} else {
		m.Heroines[id] = h
	}
	if err := raidpack.WriteManifest(dir, m); err != nil {
		return RaidPackView{}, err
	}
	v := a.raidChanged()
	return v, nil
}

func (a *App) ImportRaidPack() (RaidPackView, error) {
	src, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:   "Import a heroine pack",
		Filters: []wruntime.FileFilter{{DisplayName: "Zip archive (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || src == "" {
		return a.raidView(), err
	}
	dir, warn, err := raidpack.ImportZip(raidPacksDir(), src)
	return a.afterImport(dir, warn, err)
}

func (a *App) ChooseRaidFolder() (RaidPackView, error) {
	src, err := wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{Title: "Choose a folder of heroine images"})
	if err != nil || src == "" {
		return a.raidView(), err
	}
	dir, warn, err := raidpack.ImportFolder(raidPacksDir(), src)
	return a.afterImport(dir, warn, err)
}

func (a *App) afterImport(dir string, warn []string, err error) (RaidPackView, error) {
	a.raidOpMu.Lock()
	defer a.raidOpMu.Unlock()
	if err != nil {
		return RaidPackView{}, err
	}
	if err := a.setRaidDir(dir); err != nil {
		return RaidPackView{}, err
	}
	v := a.raidChanged()
	v.Warnings = append(warn, v.Warnings...)
	return v, nil
}

// ExportRaidStarter writes the starter kit zip; returns the saved path ("" on cancel).
func (a *App) ExportRaidStarter() (string, error) {
	path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title: "Export heroine starter kit", DefaultFilename: "heroine-starter-kit.zip",
	})
	if err != nil || path == "" {
		return "", err
	}
	bundled, err := fs.Sub(assets, "frontend/dist/assets")
	if err != nil {
		return "", err
	}
	a.raidMu.Lock()
	dir := a.raidDir
	a.raidMu.Unlock()
	p := raidpack.Pack{Files: map[string]string{}}
	if dir != "" {
		p = raidpack.Load(dir)
	}
	var buf bytes.Buffer
	if err := raidpack.WriteKit(&buf, p, bundled, raidPrompts); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, buf.Bytes(), 0o644)
}

// serveRaidMedia: the id is a map key, never a path (same rules as servePackMedia).
func (a *App) serveRaidMedia(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, mediaRaidPrefix)
	if id == "" || strings.ContainsAny(id, `/\`) {
		http.NotFound(w, r)
		return
	}
	a.raidMu.Lock()
	path, ok := a.raidPaths[id]
	a.raidMu.Unlock()
	if !ok || !raidpack.ImageExt[strings.ToLower(filepath.Ext(path))] {
		http.NotFound(w, r)
		return
	}
	if fi, err := os.Lstat(path); err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	fi, _ := f.Stat()
	http.ServeContent(w, r, filepath.Base(path), fi.ModTime(), f)
}
