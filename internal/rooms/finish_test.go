package rooms

import (
	"errors"
	"testing"
)

func TestRenameRoomPropagates(t *testing.T) {
	a, b, c, id := threeMachines(t)
	if err := c.RenameRoom(id, "Nope"); !errors.Is(err, errNotOwner) {
		t.Fatalf("colleague rename: %v", err)
	}
	if err := a.RenameRoom(id, "  "); err == nil {
		t.Fatal("empty name accepted")
	}
	if err := a.RenameRoom(id, "Commander"); err != nil {
		t.Fatal(err)
	}
	for _, m := range []*machine{a, b, c} {
		waitFor(t, "renamed everywhere", func() bool { s, _ := summaryOf(m, id); return s.Name == "Commander" })
	}
	a.CloseRoom(id)
	if err := a.RenameRoom(id, "Later"); !errors.Is(err, errClosed) {
		t.Fatalf("rename after close: %v", err)
	}
}

func TestSummaryLastIsNewestMessage(t *testing.T) {
	a := newMachine(t)
	a.Setup(true, "H", "mini")
	r, _ := a.CreateRoom("/p", "Ayina", "")
	s0, _ := summaryOf(a, r.ID)
	if s0.Last == "" {
		t.Fatal("no Last for a new room")
	}
	a.Post(r.ID, "note", "hi", "", nil)
	if s1, _ := summaryOf(a, r.ID); s1.Last < s0.Last {
		t.Fatalf("Last went back: %q < %q", s1.Last, s0.Last)
	}
}

func TestQueuedCountIncludesUnsentClose(t *testing.T) {
	a, b := newMachine(t), newMachine(t)
	a.Setup(true, "H", "mini")
	r, _ := a.CreateRoom("/p", "Ayina", "")
	pairOwn(t, a, b)
	waitFor(t, "room on b", func() bool { _, ok := summaryOf(b, r.ID); return ok })
	b.Close()
	if err := a.CloseRoom(r.ID); err != nil {
		t.Fatal(err)
	}
	if n := a.QueuedCount(); n != 1 {
		t.Fatalf("queued %d, want the close counted", n)
	}
	b.boot(t)
	waitFor(t, "close delivered", func() bool { return a.QueuedCount() == 0 })
}

func TestStaleJoinCannotOverwriteNewer(t *testing.T) {
	m := newMachine(t)
	old := &joinState{Status: "waiting"}
	cur := &joinState{Status: "waiting"}
	m.mu.Lock()
	m.join = cur
	m.mu.Unlock()
	m.finishJoin(old, errors.New("cancelled"), "")
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.join.Status != "waiting" {
		t.Fatalf("an old join overwrote the current one: %+v", m.join)
	}
}

func TestClosedRoomSettles(t *testing.T) {
	a, b, c, id := threeMachines(t)
	settled := func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.closeSettledLocked(a.rooms[id])
	}
	if settled() {
		t.Fatal("an open room counts as settled")
	}
	a.CloseRoom(id)
	for _, m := range []*machine{b, c} {
		waitFor(t, "closed", func() bool { s, _ := summaryOf(m, id); return s.Closed })
	}
	a.SyncAll()
	waitFor(t, "settled on a", settled)
}

func TestSettledNeedsEverythingInsideTheClose(t *testing.T) {
	a, b, c, id := threeMachines(t)
	b.Post(id, "note", "held by the closer", "", nil)
	waitFor(t, "a holds b's note", func() bool { return has(texts(t, a, id), "held by the closer") })
	a.CloseRoom(id)
	for _, m := range []*machine{b, c} {
		waitFor(t, "closed", func() bool { s, _ := summaryOf(m, id); return s.Closed })
	}
	a.SyncAll()
	settled := func() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.closeSettledLocked(a.rooms[id]) }
	waitFor(t, "settled", settled)
	// c holds the close but (say) a sync cut off before b's lane arrived.
	a.mu.Lock()
	a.observed[c.Status().Device][id][b.Status().Device] = 0
	a.mu.Unlock()
	if settled() {
		t.Fatal("settled although c lacks a message inside the close")
	}
}

func TestRenameReachesHiddenName(t *testing.T) {
	a := newMachine(t)
	a.Setup(true, "H", "mini")
	r, _ := a.CreateRoom("/p", "Ayina", "")
	if err := a.RenameRoom(r.ID, "Commander"); err != nil {
		t.Fatal(err)
	}
	a.CloseRoom(r.ID)
	if err := a.RemoveLocal(r.ID); err != nil {
		t.Fatal(err)
	}
	if h := a.Hidden(); len(h) != 1 || h[0].Name != "Commander" {
		t.Fatalf("hidden %+v", h)
	}
}

func TestRenameCountsCharactersNotBytes(t *testing.T) {
	a := newMachine(t)
	a.Setup(true, "H", "mini")
	r, _ := a.CreateRoom("/p", "Ayina", "")
	name := ""
	for i := 0; i < 80; i++ {
		name += "ع"
	}
	if err := a.RenameRoom(r.ID, name); err != nil {
		t.Fatalf("80 Arabic letters refused: %v", err)
	}
	waitFor(t, "renamed", func() bool { s, _ := summaryOf(a, r.ID); return s.Name == name })
	if err := a.RenameRoom(r.ID, name+"ع"); err == nil {
		t.Fatal("81 characters accepted")
	}
}
