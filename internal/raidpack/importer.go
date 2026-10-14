package raidpack

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	maxEntries = 160 // the starter kit alone is 129 (126 stems + manifest, csv, README)
	maxTotal   = 256 << 20
)

var slugBad = regexp.MustCompile(`[^a-z0-9]+`)

// FreeDir returns root/<slug of want>, or -2, -3 … if taken. Never existing.
func FreeDir(root, want string) (string, error) {
	slug := strings.Trim(slugBad.ReplaceAllString(strings.ToLower(want), "-"), "-")
	if slug == "" {
		slug = "raid-pack"
	}
	if len(slug) > 40 {
		slug = slug[:40]
	}
	c := filepath.Join(root, slug)
	for i := 2; ; i++ {
		if _, err := os.Lstat(c); os.IsNotExist(err) {
			return c, nil
		}
		if i > 99 {
			return "", fmt.Errorf("too many packs named like %q", slug)
		}
		c = filepath.Join(root, fmt.Sprintf("%s-%d", slug, i))
	}
}

type entry struct {
	name string
	open func() (io.ReadCloser, error)
}

// importEntries writes every acceptable entry into a NEW folder under root.
// On any hard error, or when no heroine image was imported, the folder is removed.
func importEntries(root, want string, ents []entry) (string, []string, error) {
	if len(ents) > maxEntries {
		return "", nil, fmt.Errorf("%d files is more than a heroine pack holds (max %d)", len(ents), maxEntries)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", nil, err
	}
	dir, err := FreeDir(root, want)
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	var warn []string
	var total int64
	images := 0
	fail := func(e error) (string, []string, error) { os.RemoveAll(dir); return "", warn, e }
	for _, e := range ents {
		if strings.ContainsAny(e.name, `/\`) || strings.HasPrefix(e.name, ".") {
			if !strings.HasPrefix(filepath.Base(e.name), ".") {
				warn = append(warn, e.name+": in a sub-folder; ignored (put files at the top level)")
			}
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.name))
		stem := strings.TrimSuffix(e.name, filepath.Ext(e.name))
		if e.name == "manifest.json" {
			rc, err := e.open()
			if err != nil {
				return fail(err)
			}
			b, err := io.ReadAll(io.LimitReader(rc, 1<<20))
			rc.Close()
			if err != nil {
				return fail(err)
			}
			if err := writeAtomic(filepath.Join(dir, "manifest.json"), b); err != nil {
				return fail(err)
			}
			continue
		}
		if silent(e.name) {
			continue
		}
		if !ImageExt[ext] || !ValidStem(stem) {
			warn = append(warn, e.name+": not a heroine image name; ignored")
			continue
		}
		rc, err := e.open()
		if err != nil {
			return fail(err)
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxFile+1))
		rc.Close()
		if err != nil {
			return fail(err)
		}
		if total += int64(len(b)); total > maxTotal {
			return fail(fmt.Errorf("pack is over %d MiB", maxTotal>>20))
		}
		if err := Store(dir, stem, strings.NewReader(string(b))); err != nil {
			warn = append(warn, fmt.Sprintf("%s: %v; skipped", e.name, err))
			continue
		}
		images++
	}
	if images == 0 {
		return fail(errors.New("no heroine images found (names like opus-stance.png; see the starter kit)"))
	}
	return dir, warn, nil
}

func ImportZip(root, zipPath string) (string, []string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", nil, fmt.Errorf("that file is not a readable zip archive: %w", err)
	}
	defer zr.Close()
	var ents []entry
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !f.Mode().IsRegular() {
			continue
		}
		f := f
		ents = append(ents, entry{name: f.Name, open: f.Open})
	}
	want := strings.TrimSuffix(filepath.Base(zipPath), filepath.Ext(zipPath))
	return importEntries(root, want, ents)
}

func ImportFolder(root, src string) (string, []string, error) {
	des, err := os.ReadDir(src)
	if err != nil {
		return "", nil, err
	}
	var ents []entry
	for _, d := range des {
		if !d.Type().IsRegular() {
			continue
		}
		p := filepath.Join(src, d.Name())
		ents = append(ents, entry{name: d.Name(), open: func() (io.ReadCloser, error) { return os.Open(p) }})
	}
	return importEntries(root, filepath.Base(src), ents)
}
