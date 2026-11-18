package rooms

import (
	"errors"
	"testing"
	"time"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/netsel"
)

func summaryOf(m *machine, id string) (RoomSummary, bool) {
	for _, r := range m.Rooms() {
		if r.ID == id {
			return r, true
		}
	}
	return RoomSummary{}, false
}

// threeMachines: a (owner, first), b (a's own laptop), c (colleague), one room.
func threeMachines(t *testing.T) (a, b, c *machine, room string) {
	a, b, c = newMachine(t), newMachine(t), newMachine(t)
	a.Setup(true, "Hussain", "mini")
	r, err := a.CreateRoom("/p/ayina", "Ayina", "")
	if err != nil {
		t.Fatal(err)
	}
	pairOwn(t, a, b)
	invite(t, a, r.ID, c)
	waitFor(t, "room on b", func() bool { _, ok := summaryOf(b, r.ID); return ok })
	return a, b, c, r.ID
}

func TestCloseRoomPropagates(t *testing.T) {
	a, b, c, id := threeMachines(t)
	if err := a.CloseRoom(id); err != nil {
		t.Fatal(err)
	}
	for _, m := range []*machine{a, b, c} {
		waitFor(t, "closed everywhere", func() bool { s, _ := summaryOf(m, id); return s.Closed })
	}
	if err := c.Post(id, "note", "too late", "", nil); !errors.Is(err, errClosed) {
		t.Fatalf("post after close: %v", err)
	}
}

func TestCloseRoomNotOwner(t *testing.T) {
	_, _, c, id := threeMachines(t)
	if err := c.CloseRoom(id); err == nil {
		t.Fatal("a colleague closed the room")
	}
}

func TestLeaveRoom(t *testing.T) {
	a, _, c, id := threeMachines(t)
	if err := a.LeaveRoom(id); err == nil {
		t.Fatal("the owner left their own room")
	}
	cdev := c.Status().Device
	if err := c.LeaveRoom(id); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "c gone from a's peers", func() bool {
		v, _ := a.View(id)
		for _, p := range v.Peers {
			if p.Device == cdev {
				return false
			}
		}
		return true
	})
	if s, _ := summaryOf(c, id); !s.Closed {
		t.Fatalf("c's copy not read-only: %+v", s)
	}
	if err := c.Post(id, "note", "still here?", "", nil); err == nil {
		t.Fatal("posted after leaving")
	}
}

func TestRemoveLocalTombstoneBlocksAdoption(t *testing.T) {
	a, b := newMachine(t), newMachine(t)
	a.Setup(true, "Hussain", "mini")
	r, _ := a.CreateRoom("/p/ayina", "Ayina", "")
	pairOwn(t, a, b)
	waitFor(t, "room on b", func() bool { _, ok := summaryOf(b, r.ID); return ok })
	if err := b.RemoveLocal(r.ID); err != nil {
		t.Fatal(err)
	}
	a.Post(r.ID, "note", "after remove", "", nil)
	a.SyncAll()
	time.Sleep(1500 * time.Millisecond)
	if _, ok := summaryOf(b, r.ID); ok {
		t.Fatal("removed room came back")
	}
	if h := b.Hidden(); len(h) != 1 || h[0].ID != r.ID || h[0].Name != "Ayina" {
		t.Fatalf("hidden %+v", h)
	}
	b.Close()
	b.boot(t)
	if _, ok := summaryOf(b, r.ID); ok || len(b.Hidden()) != 1 {
		t.Fatal("tombstone lost on restart")
	}
	if err := b.RestoreLocal(r.ID); err != nil {
		t.Fatal(err)
	}
	a.SyncAll()
	waitFor(t, "restored with message", func() bool { return has(texts(t, b, r.ID), "after remove") })
}

func TestResetMakesNewIdentity(t *testing.T) {
	a := newMachine(t)
	a.Setup(true, "Hussain", "mini")
	a.CreateRoom("/p", "Ayina", "")
	before := a.Status().Device
	if err := a.Reset(); err != nil {
		t.Fatal(err)
	}
	if st := a.Status(); st.SetUp || len(a.Rooms()) != 0 {
		t.Fatalf("after reset %+v rooms %d", st, len(a.Rooms()))
	}
	if err := a.Setup(true, "Hussain", "mini"); err != nil {
		t.Fatal(err)
	}
	if after := a.Status().Device; after == "" || after == before {
		t.Fatalf("device id before %s after %s", before, after)
	}
}

func TestOnConfirmCalled(t *testing.T) {
	a, b := newMachine(t), newMachine(t)
	got := make(chan Pending, 1)
	a.OnConfirm = func(p Pending) { got <- p }
	a.Setup(true, "Hussain", "mini")
	b.Setup(false, "", "")
	code, _ := a.PairCode(netsel.LAN)
	cv, _ := b.Join(code.Code, "laptop", "")
	select {
	case p := <-got:
		if p.Name != "laptop" || p.Words[0] != cv.Words[0] {
			t.Fatalf("pending %+v", p)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("OnConfirm not called")
	}
	a.Confirm(false)
}

func TestCancelJoin(t *testing.T) {
	a, b := newMachine(t), newMachine(t)
	a.Setup(true, "Hussain", "mini")
	b.Setup(false, "", "")
	code, _ := a.PairCode(netsel.LAN)
	b.Join(code.Code, "laptop", "")
	waitFor(t, "pending", func() bool { return a.PendingConfirm() != nil })
	start := time.Now()
	b.CancelJoin()
	joined(t, b, "failed")
	if time.Since(start) > time.Second || b.JoinStatus().Error != "cancelled" {
		t.Fatalf("cancel took %v, status %+v", time.Since(start), b.JoinStatus())
	}
}

func TestMachinesListsMyOtherMachines(t *testing.T) {
	a, b := newMachine(t), newMachine(t)
	a.Setup(true, "Hussain", "mini")
	pairOwn(t, a, b)
	ms := a.Machines()
	if len(ms) != 1 || ms[0].Name != "laptop" || ms[0].Net != "lan" || !ms[0].Mine || ms[0].Device != b.Status().Device {
		t.Fatalf("machines %+v", ms)
	}
}
