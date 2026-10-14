package codecity

import (
	"fmt"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/metrics/scan"
)

var now = time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
var info = store.ProjectInfo{Key: "k1", Label: "app", Root: "/r", ScannedAt: 5}

func files(n int, dir string, code func(i int) int) map[string]scan.FileMetrics {
	m := map[string]scan.FileMetrics{}
	for i := 0; i < n; i++ {
		m[fmt.Sprintf("%s/f%04d.go", dir, i)] = scan.FileMetrics{Lang: "Go", Code: code(i), Complexity: i % 30}
	}
	return m
}

func TestTopFilesSelectedAndRestSlabbed(t *testing.T) {
	m := files(3000, "big", func(i int) int { return 3000 - i })
	v := Build(info, m, nil, nil, now)
	if len(v.Files) != Base {
		t.Fatalf("files %d, want %d", len(v.Files), Base)
	}
	if v.Files[0].Path != "big/f0000.go" {
		t.Fatalf("largest first: %s", v.Files[0].Path)
	}
	if len(v.Slabs) != 1 || v.Slabs[0].Folder != "big" || v.Slabs[0].Files != 2400 {
		t.Fatalf("slabs %+v", v.Slabs)
	}
}

func TestHotspotsRecentAndLiveAlwaysKeptUnderTheHardCap(t *testing.T) {
	m := files(1000, "a", func(i int) int { return 1000 - i })
	m["a/f0900.go"] = scan.FileMetrics{Code: 1, Flagged: true, Hotspot: 9}
	touch := map[string]store.FileTouch{"a/f0950.go": {Edits: 2, LastAt: now.Add(-time.Hour).UnixMilli()}}
	v := Build(info, m, touch, []string{"a/f0999.go"}, now)
	got := map[string]bool{}
	for _, f := range v.Files {
		got[f.Path] = true
	}
	for _, p := range []string{"a/f0900.go", "a/f0950.go", "a/f0999.go"} {
		if !got[p] {
			t.Fatalf("%s dropped", p)
		}
	}
	if len(v.Files) != Base+3 {
		t.Fatalf("files %d", len(v.Files))
	}
	many := files(1000, "b", func(i int) int { return 1000 - i })
	for p, f := range many {
		f.Flagged = true
		many[p] = f
	}
	if v := Build(info, many, nil, nil, now); len(v.Files) != HardCap {
		t.Fatalf("hard cap: %d", len(v.Files))
	}
}

func TestFoldersFoldBelowDepthFourAndSlabsCap(t *testing.T) {
	m := map[string]scan.FileMetrics{"a/b/c/d/e/f/deep.go": {Code: 10}, "top.go": {Code: 5}}
	v := Build(info, m, nil, nil, now)
	folders := map[string]bool{}
	for _, f := range v.Files {
		folders[f.Folder] = true
	}
	if !folders["a/b/c/d"] || !folders[""] {
		t.Fatalf("folders %+v", v.Files)
	}
	wide := map[string]scan.FileMetrics{}
	for i := 0; i < 400; i++ {
		for j := 0; j < 3; j++ {
			wide[fmt.Sprintf("d%03d/f%d.go", i, j)] = scan.FileMetrics{Code: 1}
		}
	}
	for i := 0; i < 700; i++ {
		wide[fmt.Sprintf("big%03d.go", i)] = scan.FileMetrics{Code: 100}
	}
	v = Build(info, wide, nil, nil, now)
	if len(v.Slabs) > MaxSlabs {
		t.Fatalf("slabs %d", len(v.Slabs))
	}
	total := len(v.Files)
	for _, s := range v.Slabs {
		total += s.Files
	}
	if total != len(wide) {
		t.Fatalf("files lost: %d of %d", total, len(wide))
	}
}

func TestGhostNewAndHostilePaths(t *testing.T) {
	m := map[string]scan.FileMetrics{"kept.go": {Code: 3}}
	touch := map[string]store.FileTouch{
		"gone.go": {Edits: 1, LastAt: now.Add(-time.Hour).UnixMilli()},
		"old.go":  {Edits: 1, LastAt: now.Add(-60 * 24 * time.Hour).UnixMilli()},
		"../x.go": {Edits: 1, LastAt: now.UnixMilli()},
		"/etc/p":  {Edits: 1, LastAt: now.UnixMilli()},
	}
	v := Build(store.ProjectInfo{Key: "k1", Root: "/r", ScannedAt: now.UnixMilli()}, m, touch, []string{"fresh.go", "/abs/y.go"}, now)
	by := map[string]File{}
	for _, f := range v.Files {
		by[f.Path] = f
	}
	if !by["gone.go"].Ghost || !by["fresh.go"].New || by["kept.go"].Ghost {
		t.Fatalf("files %+v", v.Files)
	}
	for _, bad := range []string{"old.go", "../x.go", "/etc/p", "/abs/y.go"} {
		if _, ok := by[bad]; ok {
			t.Fatalf("%s should be dropped", bad)
		}
	}
	if by["kept.go"].Touch != (Touch{}) {
		t.Fatal("untouched file has a touch")
	}
}

func TestNoMetricsAndMissing(t *testing.T) {
	touch := map[string]store.FileTouch{"a.go": {Edits: 1, LastAt: now.UnixMilli()}}
	v := Build(store.ProjectInfo{Key: "k", Missing: true}, nil, touch, []string{"b.go"}, now)
	if !v.NoMetrics || !v.Missing {
		t.Fatalf("%+v", v)
	}
	if len(v.Files) != 1 || !v.Files[0].New || v.Files[0].Path != "b.go" {
		t.Fatalf("never scanned: only live files, as new: %+v", v.Files)
	}
}

// Final review I4: a touched file the scan skipped is not a "deleted" ruin.
func TestUnscannedTouchesAreNotGhosts(t *testing.T) {
	scanned := store.ProjectInfo{Key: "k", Root: "/r", ScannedAt: now.Add(-2 * time.Hour).UnixMilli()}
	m := map[string]scan.FileMetrics{"kept.go": {Code: 3}}
	touch := map[string]store.FileTouch{
		"package-lock.json": {Reads: 4, LastAt: now.Add(-3 * time.Hour).UnixMilli()},
		"created.go":        {Writes: 1, LastAt: now.Add(-time.Hour).UnixMilli()},
		"removed.go":        {Edits: 2, LastAt: now.Add(-3 * time.Hour).UnixMilli()},
	}
	by := map[string]File{}
	for _, f := range Build(scanned, m, touch, nil, now).Files {
		by[f.Path] = f
	}
	if _, ok := by["package-lock.json"]; ok {
		t.Fatal("read-only touch of an unscanned file should be dropped")
	}
	if f := by["created.go"]; !f.New || f.Ghost {
		t.Fatalf("written after the scan → new: %+v", f)
	}
	if f := by["removed.go"]; !f.Ghost {
		t.Fatalf("edited before the scan and gone from it → ghost: %+v", f)
	}
}
