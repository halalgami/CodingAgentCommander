package main

import (
	"errors"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/sidebar"
	"github.com/halalgami/CodingAgentCommander/internal/tmux"
)

func TestSidebarLayoutFollowsReconcile(t *testing.T) {
	a := NewApp()
	a.reconcile([]tmux.WindowState{{ID: "@1", Name: "A", Cwd: "/a"}, {ID: "@2", Name: "B", Cwd: "/b"}})
	got := a.SidebarLayout().Order[sidebar.Ungrouped]
	if len(got) != 2 || got[0] != "@1" || got[1] != "@2" {
		t.Fatalf("want [@1 @2], got %v", got)
	}
	a.reconcile([]tmux.WindowState{{ID: "@2", Name: "B", Cwd: "/b"}})
	got = a.SidebarLayout().Order[sidebar.Ungrouped]
	if len(got) != 1 || got[0] != "@2" {
		t.Fatalf("dead window kept: %v", got)
	}
}

func TestSetSidebarLayoutNormalizes(t *testing.T) {
	a := NewApp()
	if err := a.SetSidebarLayout(sidebar.Layout{Order: map[string][]string{sidebar.Ungrouped: {"@1", "@1"}}}); err != nil {
		t.Fatal(err)
	}
	if got := a.SidebarLayout().Order[sidebar.Ungrouped]; len(got) != 1 {
		t.Fatalf("duplicate kept: %v", got)
	}
}

type failingListHost struct{ tmux.Host }

func (failingListHost) List(string) ([]tmux.WindowState, error) {
	return nil, errors.New("tmux list-windows: fork/exec: resource temporarily unavailable")
}

// A failed list-windows must not be read as "every window is gone": that
// would wipe the user's hand-made sidebar arrangement.
func TestListSessionsErrorKeepsLayout(t *testing.T) {
	a := NewApp()
	a.reconcile([]tmux.WindowState{{ID: "@1", Name: "A", Cwd: "/a"}})
	_ = a.SetSidebarLayout(sidebar.Layout{
		Groups: []sidebar.Group{{ID: "g_1", Name: "Ayina"}},
		Order:  map[string][]string{"g_1": {"@1"}},
	})
	a.host = failingListHost{}
	a.ListSessions()
	if got := a.SidebarLayout().Order["g_1"]; len(got) != 1 || got[0] != "@1" {
		t.Fatalf("layout wiped by a failed list: %v", got)
	}
}
