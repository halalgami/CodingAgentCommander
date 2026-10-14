// Package codecity builds the bounded file-level model of one project for the
// dashboard's CodeCity zoom (phase 5b): the largest files, every hotspot, every
// recently touched and every live file, with the rest summarised per folder.
package codecity

import (
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/metrics/scan"
)

const (
	Base     = 600
	HardCap  = 800
	MaxSlabs = 200
	MaxDepth = 4
	MaxPath  = 300
	recent   = 7 * 24 * time.Hour
)

type CodeView struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	Locked    bool     `json:"locked"`
	NoMetrics bool     `json:"noMetrics"`
	Missing   bool     `json:"missing"`
	ScannedAt int64    `json:"scannedAt"`
	Folders   []Folder `json:"folders"`
	Files     []File   `json:"files"`
	Slabs     []Slab   `json:"slabs"`
}

type Folder struct {
	Path string `json:"path"`
	Code int    `json:"code"`
}

type File struct {
	Path       string  `json:"path"`
	Folder     string  `json:"folder"`
	Lang       string  `json:"lang"`
	Code       int     `json:"code"`
	Complexity int     `json:"complexity"`
	Todos      int     `json:"todos"`
	Hotspot    float64 `json:"hotspot"`
	Flagged    bool    `json:"flagged"`
	Test       bool    `json:"test"`
	Ghost      bool    `json:"ghost"`
	New        bool    `json:"new"`
	Touch      Touch   `json:"touch"`
}

type Touch struct {
	Reads  int64 `json:"reads"`
	Edits  int64 `json:"edits"`
	Writes int64 `json:"writes"`
	LastAt int64 `json:"lastAt"`
}

type Slab struct {
	Folder string `json:"folder"`
	Files  int    `json:"files"`
	Code   int    `json:"code"`
}

// Valid reports whether p is a clean relative path we may send to the UI.
func Valid(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") || utf8.RuneCountInString(p) > MaxPath {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "" {
			return false
		}
	}
	return true
}

// FolderOf is p's directory, folded to MaxDepth segments ("" = root).
func FolderOf(p string) string {
	d := path.Dir(p)
	if d == "." {
		return ""
	}
	if segs := strings.Split(d, "/"); len(segs) > MaxDepth {
		return strings.Join(segs[:MaxDepth], "/")
	}
	return d
}

func Build(info store.ProjectInfo, metrics map[string]scan.FileMetrics, touches map[string]store.FileTouch, live []string, now time.Time) CodeView {
	v := CodeView{Key: info.Key, Label: info.Label, Missing: info.Missing, ScannedAt: info.ScannedAt,
		NoMetrics: len(metrics) == 0, Folders: []Folder{}, Files: []File{}, Slabs: []Slab{}}
	cutoff := now.Add(-recent).UnixMilli()

	mk := func(p string, m scan.FileMetrics) File {
		t := touches[p]
		return File{Path: p, Folder: FolderOf(p), Lang: m.Lang, Code: m.Code, Complexity: m.Complexity, Todos: m.Todos,
			Hotspot: m.Hotspot, Flagged: m.Flagged, Test: m.Test,
			Touch: Touch{Reads: t.Reads, Edits: t.Edits, Writes: t.Writes, LastAt: t.LastAt}}
	}

	// Every valid metrics file, largest first (path breaks ties).
	all := make([]string, 0, len(metrics))
	for p := range metrics {
		if Valid(p) {
			all = append(all, p)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		a, b := metrics[all[i]].Code, metrics[all[j]].Code
		return a > b || (a == b && all[i] < all[j])
	})

	chosen := map[string]bool{}
	var out []File
	add := func(f File) bool {
		if chosen[f.Path] || len(out) >= HardCap {
			return false
		}
		chosen[f.Path] = true
		out = append(out, f)
		return true
	}

	// Extras first so a crane never loses its file to the cap; they are
	// ordered back into the list by size afterwards.
	for _, p := range live {
		if !Valid(p) {
			continue
		}
		if m, ok := metrics[p]; ok {
			add(mk(p, m))
		} else {
			f := mk(p, scan.FileMetrics{})
			f.New = true
			add(f)
		}
	}
	if !v.NoMetrics {
		var flagged, touched []string
		for _, p := range all {
			if metrics[p].Flagged {
				flagged = append(flagged, p)
			}
		}
		sort.SliceStable(flagged, func(i, j int) bool { return metrics[flagged[i]].Hotspot > metrics[flagged[j]].Hotspot })
		for p, t := range touches {
			if t.LastAt >= cutoff && Valid(p) {
				touched = append(touched, p)
			}
		}
		sort.Slice(touched, func(i, j int) bool {
			a, b := touches[touched[i]].LastAt, touches[touched[j]].LastAt
			return a > b || (a == b && touched[i] < touched[j])
		})
		for _, p := range flagged {
			add(mk(p, metrics[p]))
		}
		for _, p := range touched {
			if m, ok := metrics[p]; ok {
				add(mk(p, m))
				continue
			}
			// Not in the scan: Claude changed it before the scan → it was
			// deleted (ghost); changed since → new. Read-only touches of files
			// the scan skips (lockfiles, ignored, generated) are not shown.
			t := touches[p]
			if t.Edits+t.Writes == 0 {
				continue
			}
			f := mk(p, scan.FileMetrics{})
			if t.LastAt > info.ScannedAt {
				f.New = true
			} else {
				f.Ghost = true
			}
			add(f)
		}
		extras := len(out)
		for _, p := range all {
			if len(out)-extras >= Base {
				break
			}
			add(mk(p, metrics[p]))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Code > out[j].Code || (out[i].Code == out[j].Code && out[i].Path < out[j].Path)
	})
	v.Files = append(v.Files, out...)

	// Folder totals over every file; leftovers become one slab per folder.
	code := map[string]int{}
	slabs := map[string]*Slab{}
	for _, p := range all {
		m := metrics[p]
		fo := FolderOf(p)
		code[fo] += m.Code
		if !chosen[p] {
			s := slabs[fo]
			if s == nil {
				s = &Slab{Folder: fo}
				slabs[fo] = s
			}
			s.Files++
			s.Code += m.Code
		}
	}
	for _, f := range out {
		if _, ok := code[f.Folder]; !ok {
			code[f.Folder] = 0
		}
	}
	for fo, c := range code {
		v.Folders = append(v.Folders, Folder{Path: fo, Code: c})
	}
	sort.Slice(v.Folders, func(i, j int) bool { return v.Folders[i].Path < v.Folders[j].Path })

	ss := make([]Slab, 0, len(slabs))
	for _, s := range slabs {
		ss = append(ss, *s)
	}
	sort.Slice(ss, func(i, j int) bool {
		return ss[i].Code > ss[j].Code || (ss[i].Code == ss[j].Code && ss[i].Folder < ss[j].Folder)
	})
	if len(ss) > MaxSlabs {
		rest := Slab{Folder: ""}
		for _, s := range ss[MaxSlabs-1:] {
			rest.Files += s.Files
			rest.Code += s.Code
		}
		ss = ss[:MaxSlabs-1]
		merged := false
		for i := range ss {
			if ss[i].Folder == "" {
				ss[i].Files += rest.Files
				ss[i].Code += rest.Code
				merged = true
			}
		}
		if !merged {
			ss = append(ss, rest)
		}
	}
	sort.Slice(ss, func(i, j int) bool { return ss[i].Folder < ss[j].Folder })
	v.Slabs = append(v.Slabs, ss...)
	return v
}
