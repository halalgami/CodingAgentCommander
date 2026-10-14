package store

import (
	"path/filepath"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/parse"
)

func TestRecordUnlocksOnceAndPersist(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	s, _ := Create(dir, Consent{}, clock())
	id := repoID("/repo")
	s.Apply(id, "/t/a.jsonl", Cursor{SessionID: "a"}, false, []parse.Event{turn("u1")}, 0)
	if got := s.RecordUnlocks(id.Key, []Unlock{{ID: "first_blood", At: 5, SessionID: "a"}}); len(got) != 1 {
		t.Fatalf("first record %v", got)
	}
	if got := s.RecordUnlocks(id.Key, []Unlock{{ID: "first_blood", At: 9}}); len(got) != 0 {
		t.Fatal("recorded twice")
	}
	if got := s.RecordAccountUnlocks([]Unlock{{ID: "commander_10", At: 3}}); len(got) != 1 {
		t.Fatal("account unlock not recorded")
	}
	s.Close()
	s2, _ := Open(dir, clock())
	defer s2.Close()
	p, _ := s2.Project(id.Key)
	if len(p.Unlocks) != 1 || p.Unlocks[0].At != 5 {
		t.Fatalf("project unlocks %v", p.Unlocks)
	}
	if a := s2.AccountUnlocks(); len(a) != 1 || a[0].ID != "commander_10" {
		t.Fatalf("account unlocks %v", a)
	}
}

func TestUnlocksSurviveResetSession(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	s, _ := Create(dir, Consent{}, clock())
	defer s.Close()
	id := repoID("/repo")
	s.Apply(id, "/t/a.jsonl", Cursor{SessionID: "a"}, false, []parse.Event{turn("u1")}, 0)
	s.RecordUnlocks(id.Key, []Unlock{{ID: "marathon", SessionID: "a"}})
	s.ResetSession(id.Key, "a")
	if p, _ := s.Project(id.Key); len(p.Unlocks) != 1 {
		t.Fatal("re-derivation removed an unlock")
	}
	if ss := s.Sessions(id.Key); len(ss) != 0 {
		t.Fatalf("sessions after reset %d", len(ss))
	}
}
