package raidpack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Heroine struct {
	Name string `json:"name,omitempty"`
	Tint string `json:"tint,omitempty"`
}

type Manifest struct {
	Schema   int                `json:"schema"`
	Name     string             `json:"name"`
	Heroines map[string]Heroine `json:"heroines,omitempty"`
}

// Pack is a loaded folder. Files maps stem → symlink-free real path.
type Pack struct {
	Dir      string
	Name     string
	Files    map[string]string
	Heroines map[string]Heroine
	Warnings []string
}

var tintRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

// ReadManifest reads dir/manifest.json; a missing file is an empty manifest.
// Bad fields are dropped with a warning; the result is always usable.
func ReadManifest(dir string) (Manifest, []string) {
	var warn []string
	m := Manifest{Schema: 1}
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, nil
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{Schema: 1}, []string{"manifest.json is not valid JSON; ignored"}
	}
	m.Name = clip(m.Name, 40)
	clean := map[string]Heroine{}
	for id, h := range m.Heroines {
		if !validID(id) {
			warn = append(warn, fmt.Sprintf("manifest: unknown heroine %q", id))
			continue
		}
		h.Name = clip(h.Name, 24)
		if h.Tint != "" && !tintRe.MatchString(h.Tint) {
			warn = append(warn, fmt.Sprintf("manifest: %s tint %q is not #rrggbb", id, h.Tint))
			h.Tint = ""
		}
		if h != (Heroine{}) {
			clean[id] = h
		}
	}
	m.Heroines = clean
	return m, warn
}

// WriteManifest writes dir/manifest.json atomically, applying the same limits.
func WriteManifest(dir string, m Manifest) error {
	m.Schema = 1
	m.Name = clip(m.Name, 40)
	for id, h := range m.Heroines {
		h.Name = clip(h.Name, 24)
		m.Heroines[id] = h
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, "manifest.json"), b)
}

// writeAtomic writes through a fresh temp file in the same folder, so two
// writers never share a scratch name and a stray "<path>.tmp" cannot block it.
func writeAtomic(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op after a successful rename
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// silent files are expected in a pack (or left by Finder) and never warned about.
func silent(name string) bool {
	l := strings.ToLower(name)
	return strings.HasPrefix(name, ".") || name == "manifest.json" || strings.HasPrefix(l, "readme") || strings.HasSuffix(l, ".csv") || strings.HasSuffix(l, ".tmp")
}

// Load reads a pack folder. It never fails: problems become warnings.
func Load(dir string) Pack {
	p := Pack{Dir: dir, Name: filepath.Base(dir), Files: map[string]string{}, Heroines: map[string]Heroine{}}
	ents, err := os.ReadDir(dir)
	if err != nil {
		p.Warnings = append(p.Warnings, fmt.Sprintf("cannot read %s: %v", filepath.Base(dir), err))
		return p
	}
	m, warn := ReadManifest(dir)
	p.Warnings = append(p.Warnings, warn...)
	if m.Schema > 1 {
		p.Warnings = append(p.Warnings, fmt.Sprintf("manifest schema %d is newer than this app understands; pack not used", m.Schema))
		return p
	}
	if m.Name != "" {
		p.Name = m.Name
	}
	p.Heroines = m.Heroines
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		if silent(name) {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		if !ImageExt[ext] || !ValidStem(stem) {
			p.Warnings = append(p.Warnings, fmt.Sprintf("%s: not a heroine image name; ignored", name))
			continue
		}
		path := filepath.Join(dir, name)
		fi, err := os.Lstat(path)
		if err != nil || !fi.Mode().IsRegular() {
			p.Warnings = append(p.Warnings, fmt.Sprintf("%s: not a regular file; ignored", name))
			continue
		}
		if _, dup := p.Files[stem]; dup {
			p.Warnings = append(p.Warnings, fmt.Sprintf("%s: another file already provides %s; ignored", name, stem))
			continue
		}
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			continue
		}
		p.Files[stem] = real
	}
	return p
}
