package store

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
	"github.com/halalgami/CodingAgentCommander/internal/repoid"
)

func clock() func() time.Time {
	return func() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) }
}

func repoID(root string) repoid.ID {
	return repoid.ID{Key: repoid.KeyOf("rootcommit:abc"), Identity: "rootcommit:abc", Root: root, Label: filepath.Base(root), PathKey: repoid.KeyOf("path:" + root)}
}

func turn(uuid string) parse.Event {
	return parse.Event{Kind: parse.TurnDuration, At: time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC).UnixMilli(), UUID: uuid, Ms: 1000}
}

func TestNoConsentNoDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	if Enabled(dir) {
		t.Fatal("enabled without consent")
	}
	if _, err := Open(dir, clock()); err != ErrNoConsent {
		t.Fatalf("Open err = %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("Open must not create the ledger directory")
	}
}

func TestCreateApplyFlushReopen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	s, err := Create(dir, Consent{StoreTitles: true}, clock())
	if err != nil {
		t.Fatal(err)
	}
	id := repoID("/repo")
	cur := Cursor{Size: 10, Offset: 10, SessionID: "s1", Cwd: "/repo", ParserVersion: parse.ParserVersion}
	if !s.Apply(id, "/t/s1.jsonl", cur, false, []parse.Event{{Kind: parse.Title, Text: "T"}, turn("u1")}, 2) {
		t.Fatal("apply refused")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(filepath.Join(dir, "projects", id.Key+".json"))
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("project file mode %v", fi.Mode().Perm())
		}
		di, _ := os.Stat(dir)
		if di.Mode().Perm() != 0o700 {
			t.Fatalf("dir mode %v", di.Mode().Perm())
		}
	}
	s2, err := Open(dir, clock())
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	p, ok := s2.Project(id.Key)
	if !ok || p.Totals.ActiveMs != 1000 || p.Totals.Skipped != 2 || p.Sessions[0].Title != "T" {
		t.Fatalf("reopened: %+v", p)
	}
	if c, ok := s2.Cursor("/t/s1.jsonl"); !ok || c.Offset != 10 {
		t.Fatalf("cursor: %+v %v", c, ok)
	}
}

func TestSecondInstanceIsReadOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no lock on windows in phase 1")
	}
	dir := filepath.Join(t.TempDir(), "ledger")
	a, _ := Create(dir, Consent{}, clock())
	defer a.Close()
	b, err := Open(dir, clock())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if !b.ReadOnly() || a.ReadOnly() {
		t.Fatalf("readOnly a=%v b=%v", a.ReadOnly(), b.ReadOnly())
	}
}

func TestCorruptFileRestoredFromBackup(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	s, _ := Create(dir, Consent{}, clock())
	id := repoID("/repo")
	s.Apply(id, "/t/s1.jsonl", Cursor{Offset: 5, SessionID: "s1"}, false, []parse.Event{turn("u1")}, 0)
	s.Flush() // first write: no .bak yet (nothing to back up)
	s.Apply(id, "/t/s1.jsonl", Cursor{Offset: 9, SessionID: "s1"}, false, []parse.Event{turn("u2")}, 0)
	s.Flush() // second write backs up the first
	s.Close()
	path := filepath.Join(dir, "projects", id.Key+".json")
	os.WriteFile(path, []byte(`{"key":`), 0o600) // corrupt

	s2, err := Open(dir, clock())
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	p, ok := s2.Project(id.Key)
	if !ok || p.Totals.ActiveMs != 1000 {
		t.Fatalf("restored: %+v", p)
	}
	if c, _ := s2.Cursor("/t/s1.jsonl"); c.Offset != 5 {
		t.Fatalf("cursor must come from the same backup as the data: %+v", c)
	}
	if s2.LastError() == "" {
		t.Fatal("restore must be reported")
	}
	matches, _ := filepath.Glob(path + ".corrupt-*")
	if len(matches) != 1 {
		t.Fatal("corrupt file not kept aside")
	}
}

func TestExcludeForgetMergeKeyByRoot(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	s, _ := Create(dir, Consent{}, clock())
	defer s.Close()
	pathID := repoid.ID{Key: repoid.KeyOf("path:/repo"), Identity: "path:/repo", Root: "/repo", Label: "repo", PathKey: repoid.KeyOf("path:/repo")}
	s.Apply(pathID, "/t/a.jsonl", Cursor{SessionID: "a"}, false, []parse.Event{turn("u1")}, 0)

	if got, ok := s.KeyByRoot("/repo"); !ok || got.Key != pathID.Key {
		t.Fatalf("KeyByRoot = %+v %v", got, ok)
	}
	commitID := repoID("/repo")
	s.Merge(pathID.Key, commitID)
	if s.Has(pathID.Key) || !s.Has(commitID.Key) {
		t.Fatal("merge did not move the project")
	}
	if p, _ := s.Project(commitID.Key); p.Totals.ActiveMs != 1000 {
		t.Fatalf("merged totals: %+v", p.Totals)
	}
	if c, ok := s.Cursor("/t/a.jsonl"); !ok || c.SessionID != "a" {
		t.Fatal("cursor not carried by merge")
	}

	if err := s.SetConsent(Consent{Enabled: true, Excluded: []string{"/repo"}}); err != nil {
		t.Fatal(err)
	}
	if s.Has(commitID.Key) {
		t.Fatal("excluded project not purged")
	}
	if s.Apply(commitID, "/t/b.jsonl", Cursor{SessionID: "b"}, false, []parse.Event{turn("u9")}, 0) {
		t.Fatal("apply accepted an excluded root")
	}

	other := repoID("/other")
	other.Key = "other"
	s.Apply(other, "/t/c.jsonl", Cursor{SessionID: "c"}, false, []parse.Event{turn("u5")}, 0)
	s.Flush()
	if err := s.Forget("other"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "projects", "other.json")); !os.IsNotExist(err) {
		t.Fatal("forget left the file")
	}
}

func TestTitlesScrubbedWhenTitlesTurnedOff(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	s, _ := Create(dir, Consent{StoreTitles: true}, clock())
	id := repoID("/repo")
	s.Apply(id, "/t/s1.jsonl", Cursor{SessionID: "s1"}, false, []parse.Event{{Kind: parse.Title, Text: "Rotate prod DB password"}}, 0)
	s.Flush()
	if err := s.SetConsent(Consent{Enabled: true, StoreTitles: false}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	b, _ := os.ReadFile(filepath.Join(dir, "projects", id.Key+".json"))
	if strings.Contains(string(b), "Rotate prod DB password") {
		t.Fatal("title still on disk after titles were turned off")
	}
}

// Minors round 2 (phase 3 review): an unreadable profile.json was ignored in
// silence and the next flush overwrote it, losing every account achievement.
func TestCorruptProfileKeptAsideAndReported(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	s, _ := Create(dir, Consent{}, clock())
	s.Close()
	path := filepath.Join(dir, "profile.json")
	os.WriteFile(path, []byte(`{"unlocks":[`), 0o600)
	s2, err := Open(dir, clock())
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if s2.LastError() == "" {
		t.Fatal("unreadable profile not reported")
	}
	if m, _ := filepath.Glob(path + ".corrupt-*"); len(m) != 1 {
		t.Fatal("unreadable profile not kept aside")
	}
}
