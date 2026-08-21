package winstate

import (
	"os"
	"path/filepath"
	"testing"
)

func tmpPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "windows.json")
}

func TestSetSurvivesReopen(t *testing.T) {
	p := tmpPath(t)
	s := Open(p)
	if err := s.Set("@5", Record{Model: "claude-opus-5"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Update("@5", func(r *Record) { r.RemoteControl = true }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// Durability across an app restart is the whole point — this replaced a tmux
	// option that lived in the server and survived restarts.
	got, ok := Open(p).Get("@5")
	if !ok || got.Model != "claude-opus-5" || !got.RemoteControl {
		t.Errorf("after reopen: %+v (found=%v)", got, ok)
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Error("temp file left behind by the atomic write")
	}
}

func TestRecordsAreIndependentPerWindow(t *testing.T) {
	// The bug this package exists for: a tmux window option written for one
	// window was read back by every window. Two ids must not share a record.
	s := Open(tmpPath(t))
	if err := s.Set("@1", Record{Model: "glm-5.3"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("@2", Record{Model: "claude-opus-5"}); err != nil {
		t.Fatal(err)
	}
	a, _ := s.Get("@1")
	b, _ := s.Get("@2")
	if a.Model != "glm-5.3" || b.Model != "claude-opus-5" {
		t.Errorf("records bled together: @1=%+v @2=%+v", a, b)
	}
}

func TestSetIsAuthoritativeForAReusedID(t *testing.T) {
	s := Open(tmpPath(t))
	_ = s.Set("@1", Record{Model: "glm-5.3", RemoteControl: true})
	// A launch replaces the record wholesale; a stale RemoteControl flag must not
	// survive onto the new session.
	_ = s.Set("@1", Record{Model: "claude-opus-5"})
	got, _ := s.Get("@1")
	if got.Model != "claude-opus-5" || got.RemoteControl {
		t.Errorf("Set did not replace the record: %+v", got)
	}
}

func TestPruneDropsOnlyDeadWindows(t *testing.T) {
	p := tmpPath(t)
	s := Open(p)
	for _, id := range []string{"@1", "@2", "@3"} {
		_ = s.Set(id, Record{Model: "m-" + id})
	}
	n, err := s.Prune([]string{"@2"})
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 2 {
		t.Errorf("pruned %d, want 2", n)
	}
	if _, ok := s.Get("@2"); !ok {
		t.Error("live window was pruned")
	}
	for _, gone := range []string{"@1", "@3"} {
		if _, ok := s.Get(gone); ok {
			t.Errorf("%s survived the prune", gone)
		}
	}
	// Pruning to nothing is the tmux-server-restart case: no window ids are
	// live, so no new window can inherit a dead one's model.
	if n, _ := s.Prune(nil); n != 1 {
		t.Errorf("second prune removed %d, want 1", n)
	}
	if n, _ := s.Prune(nil); n != 0 {
		t.Errorf("pruning an empty store reported %d changes", n)
	}
	if _, ok := Open(p).Get("@2"); ok {
		t.Error("prune did not persist: @2 was back after reopening the file")
	}
}

func TestDelete(t *testing.T) {
	s := Open(tmpPath(t))
	_ = s.Set("@1", Record{Model: "m"})
	if err := s.Delete("@1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := s.Get("@1"); ok {
		t.Error("record survived Delete")
	}
	if err := s.Delete("@nope"); err != nil {
		t.Errorf("deleting an absent record errored: %v", err)
	}
}

func TestOpenToleratesMissingAndCorruptFiles(t *testing.T) {
	if _, ok := Open(filepath.Join(t.TempDir(), "absent.json")).Get("@1"); ok {
		t.Error("a missing file yielded a record")
	}
	p := tmpPath(t)
	if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Open(p)
	if _, ok := s.Get("@1"); ok {
		t.Error("a corrupt file yielded a record")
	}
	// A corrupt file must not be fatal, and must still be writable afterwards.
	if err := s.Set("@1", Record{Model: "m"}); err != nil {
		t.Errorf("Set after a corrupt read: %v", err)
	}
	if got, ok := Open(p).Get("@1"); !ok || got.Model != "m" {
		t.Errorf("store not usable after a corrupt read: %+v", got)
	}
}

func TestEmptyPathNeverWrites(t *testing.T) {
	dir := t.TempDir()
	s := Open("")
	if err := s.Set("@1", Record{Model: "m"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got, ok := s.Get("@1"); !ok || got.Model != "m" {
		t.Errorf("memory-only store did not keep the record: %+v", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("memory-only store wrote files: %v", entries)
	}
}

func TestBlankWindowIDIsIgnored(t *testing.T) {
	s := Open(tmpPath(t))
	if err := s.Set("", Record{Model: "m"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Update("", func(r *Record) { r.Model = "m" }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, ok := s.Get(""); ok {
		t.Error("a blank window id was stored")
	}
}
