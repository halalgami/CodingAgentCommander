package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/ledger/store"
	"github.com/halalgami/CodingAgentCommander/internal/rooms"
)

func TestRoomForPrefersNewestOpenRoom(t *testing.T) {
	p := ProjectRoom{Identity: "rootcommit:x"}
	list := []rooms.RoomSummary{
		{ID: "old", Project: "rootcommit:x", Last: "2026-11-01T00:00:00Z"},
		{ID: "new", Project: "rootcommit:x", Last: "2026-11-18T00:00:00Z"},
		{ID: "closed", Project: "rootcommit:x", Last: "2026-11-19T00:00:00Z", Closed: true},
	}
	if r, _ := roomFor(list, p); r.ID != "new" {
		t.Fatalf("picked %q", r.ID)
	}
}

func TestNonGitFolderRecheckedLater(t *testing.T) {
	old := nonGitRecheck
	nonGitRecheck = 0
	defer func() { nonGitRecheck = old }()
	dir := realpath(t, t.TempDir())
	if p := resolveProject(dir); p.Identity != "" {
		t.Fatalf("plain folder has identity %q", p.Identity)
	}
	committedRepo(t, dir)
	if p := resolveProject(dir); !strings.HasPrefix(p.Identity, "rootcommit:") {
		t.Fatalf("after git init still %+v", p)
	}
}

func TestGitHeadBySessionOnly(t *testing.T) {
	a := setUpRooms(t)
	dir := committedRepo(t, filepath.Join(realpath(t, t.TempDir()), "p"))
	addRoomSession(a, "w1", dir)
	h, err := a.GitHead("w1")
	if err != nil || h.Branch == "" || len(h.Sha) != 40 {
		t.Fatalf("head %+v err %v", h, err)
	}
	if _, err := a.GitHead("nope"); err == nil {
		t.Fatal("unknown session accepted")
	}
}

func TestRecentProjectsNewestFirstWithRooms(t *testing.T) {
	a := setUpRooms(t)
	base := realpath(t, t.TempDir())
	p1 := committedRepo(t, filepath.Join(base, "one"))
	p2 := committedRepo(t, filepath.Join(base, "two"))
	r, err := a.ShareProject(p2)
	if err != nil {
		t.Fatal(err)
	}
	infos := []store.ProjectInfo{
		{Key: "k1", Root: p1, Label: "one", LastSeen: 1},
		{Key: "k2", Root: p2, Label: "two", LastSeen: 2},
		{Key: "k3", Root: filepath.Join(base, "gone"), Label: "gone", LastSeen: 3, Missing: true},
	}
	got := recentProjects(infos, a.Rooms(), 8)
	if len(got) != 2 || got[0].Label != "two" || got[0].RoomID != r.ID || got[1].RoomID != "" {
		t.Fatalf("recent %+v", got)
	}
	if p := projectByKey(infos, a.Rooms(), "k2"); p.RoomID != r.ID || p.Root != p2 {
		t.Fatalf("by key %+v", p)
	}
	if p := projectByKey(infos, a.Rooms(), "zz"); p.Root != "" {
		t.Fatalf("unknown key %+v", p)
	}
}
