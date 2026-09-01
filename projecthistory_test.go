package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordProjectOpenUpsertsByFolder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	a := NewApp()

	pa := filepath.Join(dir, "a")
	pb := filepath.Join(dir, "b")
	if err := os.MkdirAll(pa, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pb, 0o755); err != nil {
		t.Fatal(err)
	}

	a.recordProjectOpen(pa, "m1")
	a.recordProjectOpen(pa, "m2") // same folder, new model -> upsert, not a new row
	a.recordProjectOpen(pb, "m3")

	got := a.ListProjects()
	if len(got) != 2 {
		t.Fatalf("want 2 rows (one per folder), got %d: %+v", len(got), got)
	}
	var rowA *ProjectView
	for i := range got {
		if got[i].Folder == filepath.Clean(pa) {
			rowA = &got[i]
		}
	}
	if rowA == nil {
		t.Fatalf("folder a missing from %+v", got)
	}
	if rowA.OpenCount != 2 {
		t.Errorf("openCount = %d, want 2", rowA.OpenCount)
	}
	if rowA.LastModelID != "m2" {
		t.Errorf("lastModelID = %q, want m2", rowA.LastModelID)
	}
	if rowA.Label != "a" {
		t.Errorf("label = %q, want basename a", rowA.Label)
	}
	if rowA.Missing {
		t.Error("folder a exists on disk; Missing must be false")
	}
}

func TestListProjectsSortPinnedThenRecent(t *testing.T) {
	in := []ProjectEntry{
		{Folder: "/x", LastOpened: 100},
		{Folder: "/y", LastOpened: 300, Pinned: true},
		{Folder: "/z", LastOpened: 200},
	}
	out := sortProjects(in)
	if out[0].Folder != "/y" {
		t.Errorf("pinned must lead, got %q", out[0].Folder)
	}
	if out[1].Folder != "/z" || out[2].Folder != "/x" {
		t.Errorf("unpinned must be recency-desc, got %q,%q", out[1].Folder, out[2].Folder)
	}
}

func TestPinRemoveRename(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	a := NewApp()
	p := filepath.Join(dir, "proj")
	os.MkdirAll(p, 0o755)
	a.recordProjectOpen(p, "m1")
	key := filepath.Clean(p)

	if err := a.PinProject(key, true); err != nil {
		t.Fatal(err)
	}
	if !a.ListProjects()[0].Pinned {
		t.Error("pin not persisted")
	}

	if err := a.RenameProject(key, "My Project"); err != nil {
		t.Fatal(err)
	}
	if a.ListProjects()[0].Label != "My Project" {
		t.Error("rename not persisted")
	}
	if err := a.RenameProject(key, "  "); err != nil {
		t.Fatal(err)
	}
	if a.ListProjects()[0].Label != "proj" {
		t.Error("blank rename should reset to basename")
	}

	if err := a.RemoveProject(key); err != nil {
		t.Fatal(err)
	}
	if len(a.ListProjects()) != 0 {
		t.Error("remove failed")
	}
}

func TestImportProjectsIdempotentNoOpWhenNonEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	a := NewApp()
	if err := a.ImportProjects([]ProjectEntry{{Folder: "/one", LastModelID: "m", OpenCount: 1}}); err != nil {
		t.Fatal(err)
	}
	if len(a.ListProjects()) != 1 {
		t.Fatalf("first import should seed 1")
	}
	// Second import must be a no-op (store already has data).
	if err := a.ImportProjects([]ProjectEntry{{Folder: "/two"}, {Folder: "/three"}}); err != nil {
		t.Fatal(err)
	}
	if len(a.ListProjects()) != 1 {
		t.Error("import must no-op when store already non-empty")
	}
}

func TestLoadProjectHistoryCorruptFallsBackToEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COMMANDER_CONFIG", filepath.Join(dir, "config.toml"))
	os.WriteFile(projectHistoryPath(), []byte("{not json"), 0o644)
	if got := loadProjectHistory(); len(got.Projects) != 0 {
		t.Fatalf("corrupt file must yield empty history, got %+v", got)
	}
}

func TestPruneKeepsPinnedDropsOldestUnpinned(t *testing.T) {
	h := projectHistory{}
	// 600 unpinned + 3 pinned; oldest unpinned must be dropped beyond 500.
	for i := 0; i < 600; i++ {
		h.Projects = append(h.Projects, ProjectEntry{Folder: filepath.Clean("/u" + itoa(i)), LastOpened: int64(i)})
	}
	for i := 0; i < 3; i++ {
		h.Projects = append(h.Projects, ProjectEntry{Folder: filepath.Clean("/p" + itoa(i)), Pinned: true, LastOpened: 0})
	}
	pruned := pruneProjects(h)
	pinned, unpinned := 0, 0
	for _, e := range pruned.Projects {
		if e.Pinned {
			pinned++
		} else {
			unpinned++
		}
	}
	if pinned != 3 {
		t.Errorf("all pinned must survive, got %d", pinned)
	}
	if unpinned != 500 {
		t.Errorf("unpinned must cap at 500, got %d", unpinned)
	}
	// The very oldest unpinned (LastOpened 0) must be gone; a recent one kept.
	if projectsContain(pruned.Projects, filepath.Clean("/u0")) {
		t.Error("oldest unpinned /u0 should have been pruned")
	}
	if !projectsContain(pruned.Projects, filepath.Clean("/u599")) {
		t.Error("most-recent unpinned /u599 must be kept")
	}
}

// tiny helpers so the test doesn't pull in strconv noise inline
func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }
func projectsContain(ps []ProjectEntry, folder string) bool {
	for _, p := range ps {
		if p.Folder == folder {
			return true
		}
	}
	return false
}
