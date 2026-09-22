package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Pack export and import, as a single .zip.
//
// Export is easy. IMPORT is the most dangerous code in this file, because a zip
// is a list of file names chosen by whoever built it, and the classic attack
// ("zip slip") is an entry called "../../../../.ssh/authorized_keys" that
// escapes the extraction directory. Go's archive/zip does NOT sanitise names.
//
// So extraction here treats every entry name as hostile: an entry may be a
// single path segment, on a whitelist of names a pack is allowed to contain,
// and nothing else. It is not sanitised into safety — it is REFUSED. A pack is
// a flat folder, so an entry with a separator in it is either an attack or a
// file we would not have served anyway, and silently flattening it would turn a
// suspicious archive into an accepted one.

const (
	// packZipMaxEntries bounds the file COUNT. A pack is a manifest, a base and
	// a handful of images; anything near this is not a pack.
	packZipMaxEntries = 64
	// packZipMaxFile and packZipMaxTotal bound the UNCOMPRESSED size. Zip stores
	// a declared size that a hostile archive can lie about, so both are enforced
	// while copying rather than trusted from the header.
	packZipMaxFile  = 64 << 20  // 64 MiB, matching the base-portrait cap
	packZipMaxTotal = 512 << 20 // 512 MiB across the whole archive
)

// packZipEntryAllowed reports whether a zip entry name may be extracted.
//
// The whitelist is by NAME, not merely by extension: a pack contains a
// manifest, image files, and the stored base portrait. Anything else — a
// README, a .DS_Store, a script — is dropped rather than written, because a
// file we do not serve is a file we have no reason to put on disk.
func packZipEntryAllowed(name string) bool {
	// Any separator, drive letter or traversal segment is refused outright.
	// This is the zip-slip guard, and it is deliberately a refusal rather than
	// a sanitisation: filepath.Base("../../x") is "x", which would turn an
	// attack into a silently accepted file.
	if name == "" || name != filepath.Base(name) {
		return false
	}
	if strings.ContainsAny(name, `/\:`) || name == "." || name == ".." {
		return false
	}
	if strings.HasPrefix(name, ".") && name != "." {
		// Dotfiles are not part of a pack, and "._foo" resource forks arrive in
		// every archive made on a Mac.
		return false
	}
	if name == "manifest.json" {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	if strings.HasPrefix(name, packGenBaseStem+".") {
		return ext == ".png" || ext == ".jpg" || ext == ".jpeg"
	}
	return packMediaExt[ext]
}

// ExportCompanionPack writes the active pack to a .zip the user chooses.
// Returns the path written, or "" if they cancelled.
func (a *App) ExportCompanionPack() (string, error) {
	dir := a.GetCompanionConfig().PackPath
	if dir == "" {
		return "", fmt.Errorf("no companion pack configured")
	}
	suggested := filepath.Base(dir) + ".zip"
	dest, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:           "Export the companion pack",
		DefaultFilename: suggested,
		Filters:         []wruntime.FileFilter{{DisplayName: "Zip archive (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || dest == "" { // cancel -> ""
		return "", err
	}
	if strings.ToLower(filepath.Ext(dest)) != ".zip" {
		dest += ".zip"
	}
	if err := writePackZip(dir, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// writePackZip archives a pack folder's regular files, flat.
//
// Written to a temp file and renamed, so a failure part-way through cannot
// leave a truncated archive sitting at the destination looking like a backup.
func writePackZip(dir, dest string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".pack-*.zip.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	zw := zip.NewWriter(tmp)
	var written int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			tmp.Close()
			return err
		}
		// Symlinks are not followed, for the same reason the loader refuses to
		// serve through one: a pack is shared, and following a link would pull
		// a file from outside it into the archive.
		if !info.Mode().IsRegular() {
			continue
		}
		if !packZipEntryAllowed(e.Name()) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			tmp.Close()
			return err
		}
		w, err := zw.Create(e.Name())
		if err != nil {
			tmp.Close()
			return err
		}
		if _, err := w.Write(raw); err != nil {
			tmp.Close()
			return err
		}
		written++
	}
	if written == 0 {
		tmp.Close()
		return fmt.Errorf("there is nothing in %s to export", filepath.Base(dir))
	}
	if err := zw.Close(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dest)
}

// ImportCompanionPack extracts a .zip into the packs folder and selects it.
// Returns the new pack's display name, or "" if the user cancelled.
func (a *App) ImportCompanionPack() (string, error) {
	src, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:   "Import a companion pack",
		Filters: []wruntime.FileFilter{{DisplayName: "Zip archive (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || src == "" { // cancel -> ""
		return "", err
	}
	dir, err := a.importPackZip(src)
	if err != nil {
		return "", err
	}
	return a.setCompanionPack(dir)
}

// importPackZip extracts src into a NEW folder under the packs directory and
// returns that folder. It never writes into an existing pack: a name collision
// picks a free one, because an import that silently overwrote a pack would
// destroy art the user cannot get back.
func (a *App) importPackZip(src string) (string, error) {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return "", fmt.Errorf("that file is not a readable zip archive: %w", err)
	}
	defer zr.Close()

	if len(zr.File) > packZipMaxEntries {
		return "", fmt.Errorf("that archive has %d files, which is far more than a pack contains", len(zr.File))
	}
	// Refuse BEFORE writing anything: a half-extracted pack in the packs folder
	// would show up in the library as a broken entry.
	var hasManifest bool
	for _, f := range zr.File {
		if f.Name == "manifest.json" {
			hasManifest = true
		}
	}
	if !hasManifest {
		return "", fmt.Errorf("that archive has no manifest.json, so it is not a companion pack")
	}

	root := companionPacksDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	dir, err := freePackDir(root, strings.TrimSuffix(filepath.Base(src), filepath.Ext(src)))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	if err := extractPackZip(zr, dir); err != nil {
		// Leave nothing behind. A partial folder here is worse than no import:
		// it appears in the library, loads with warnings, and looks like the
		// user's pack is corrupt rather than that the import failed.
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

func extractPackZip(zr *zip.ReadCloser, dir string) error {
	var total int64
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Mode carries symlink and device bits. A zip can describe a symlink,
		// and extracting one would plant a link inside the pack that the media
		// handler then has to refuse at serve time — better never to write it.
		if !f.Mode().IsRegular() {
			continue
		}
		if !packZipEntryAllowed(f.Name) {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("reading %s from the archive: %w", f.Name, err)
		}
		// The DECLARED size is attacker-controlled, so the cap is enforced while
		// copying. LimitReader at cap+1 makes an over-long entry detectable
		// rather than silently truncated into a corrupt image.
		out, err := os.Create(filepath.Join(dir, f.Name))
		if err != nil {
			rc.Close()
			return err
		}
		n, err := io.Copy(out, io.LimitReader(rc, packZipMaxFile+1))
		rc.Close()
		closeErr := out.Close()
		if err != nil {
			return fmt.Errorf("extracting %s: %w", f.Name, err)
		}
		if closeErr != nil {
			return closeErr
		}
		if n > packZipMaxFile {
			return fmt.Errorf("%s in that archive is larger than %d MB", f.Name, packZipMaxFile>>20)
		}
		total += n
		if total > packZipMaxTotal {
			return fmt.Errorf("that archive expands to more than %d MB", packZipMaxTotal>>20)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		// Reachable when the only manifest entry was a directory or a symlink,
		// both of which are skipped above.
		return fmt.Errorf("that archive has no usable manifest.json")
	}
	return nil
}

// freePackDir returns a folder under root that does not exist yet, derived from
// the archive's name. Never returns an existing directory: overwriting a pack
// on import would destroy art with no way back.
func freePackDir(root, want string) (string, error) {
	slug, err := packGenSlug(want)
	if err != nil {
		slug = "imported-pack"
	}
	candidate := filepath.Join(root, slug)
	for i := 2; packGenIsDir(candidate) || fileExists(candidate); i++ {
		if i > 99 {
			return "", fmt.Errorf("too many packs already named like %q", slug)
		}
		candidate = filepath.Join(root, fmt.Sprintf("%s-%d", slug, i))
	}
	return candidate, nil
}

func fileExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
