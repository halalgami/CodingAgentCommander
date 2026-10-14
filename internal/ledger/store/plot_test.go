package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
	"github.com/halalgami/CodingAgentCommander/internal/repoid"
)

func plotStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ledger")
	st, err := Create(dir, Consent{StoreTitles: true}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st, dir
}

func pid(root string) repoid.ID {
	return repoid.ID{Key: repoid.KeyOf("path:" + root), Identity: "path:" + root, Root: root, Label: filepath.Base(root)}
}

func touch(st *Store, root string, at int64) {
	st.Apply(pid(root), root+"/t.jsonl", Cursor{SessionID: "s-" + root}, false, []parse.Event{{Kind: parse.Prompt, At: at}}, 0)
}

func plots(st *Store) map[string]int {
	out := map[string]int{}
	for _, p := range st.Projects() {
		out[p.Root] = p.Plot
	}
	return out
}

func TestPlotsAreAssignedOnceAndStable(t *testing.T) {
	st, _ := plotStore(t)
	touch(st, "/a", 1)
	touch(st, "/b", 2)
	touch(st, "/c", 3)
	if p := plots(st); p["/a"] != 1 || p["/b"] != 2 || p["/c"] != 3 {
		t.Fatalf("plots %v", p)
	}
	touch(st, "/a", 9) // more activity never moves a district
	if p := plots(st); p["/a"] != 1 {
		t.Fatalf("plot moved: %v", p)
	}
}

// Review Focus 1.
func TestForgetFreesPlotAndMergeKeepsDestination(t *testing.T) {
	st, _ := plotStore(t)
	touch(st, "/a", 1)
	touch(st, "/b", 2)
	touch(st, "/c", 3)
	if err := st.Forget(pid("/b").Key); err != nil {
		t.Fatal(err)
	}
	touch(st, "/d", 4)
	if p := plots(st); p["/d"] != 2 || p["/a"] != 1 || p["/c"] != 3 {
		t.Fatalf("freed plot not reused, or others moved: %v", p)
	}
	st.Merge(pid("/c").Key, pid("/a"))
	if p := plots(st); p["/a"] != 1 {
		t.Fatalf("merge destination lost its plot: %v", p)
	}
}

func TestPlotsSurviveReopenAndOldFilesGetPlots(t *testing.T) {
	st, dir := plotStore(t)
	touch(st, "/a", 1)
	touch(st, "/b", 2)
	st.mu.Lock()
	st.projects[pid("/b").Key].Plot = 0 // as written before plots existed
	st.dirty[pid("/b").Key] = true
	st.mu.Unlock()
	if err := st.Flush(); err != nil {
		t.Fatal(err)
	}
	st.Close()
	re, err := Open(dir, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer re.Close()
	if p := plots(re); p["/a"] != 1 || p["/b"] != 2 {
		t.Fatalf("after reopen %v", p)
	}
}

func applyEdit(st *Store, root, file string) string {
	id := pid(root)
	st.Apply(id, root+"/t.jsonl", Cursor{SessionID: "s-" + root}, false, []parse.Event{
		{Kind: parse.ToolUse, At: 1000, ToolID: "t1", ToolName: "Edit", Target: filepath.Base(file), FilePath: file},
	}, 0)
	return id.Key
}

func TestTouchesAndRootsAreCopies(t *testing.T) {
	st, _ := plotStore(t)
	key := applyEdit(st, "/p1", "/p1/app.go")
	tc := st.Touches(key)
	if tc["app.go"].Edits != 1 {
		t.Fatalf("touches %+v", tc)
	}
	tc["app.go"] = FileTouch{}
	if st.Touches(key)["app.go"].Edits != 1 {
		t.Fatal("Touches returned the live map")
	}
	if st.Roots()[key] != "/p1" {
		t.Fatalf("roots %+v", st.Roots())
	}
}
