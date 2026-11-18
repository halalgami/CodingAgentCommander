package rooms

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/roomlog"
)

// Final review I2: a seen receipt must name a message of that room.
func TestMarkSeenRefusesForeignRef(t *testing.T) {
	a := newMachine(t)
	a.Setup(true, "H", "mini")
	r, _ := a.CreateRoom("/p", "Ayina", "")
	if err := a.MarkSeen(r.ID, "12D3KooWnotinroom:7"); err == nil {
		t.Fatal("seen receipt for a ref that is not in the room")
	}
}

// Final review I5: leaving on one of my machines makes all of them read-only.
func TestLeaveSpreadsToMyOtherMachines(t *testing.T) {
	a, c, c2 := newMachine(t), newMachine(t), newMachine(t)
	a.Setup(true, "H", "mini")
	r, _ := a.CreateRoom("/p", "Ayina", "")
	invite(t, a, r.ID, c)
	pairOwn(t, c, c2)
	waitFor(t, "room on c2", func() bool { _, ok := summaryOf(c2, r.ID); return ok })
	if err := c.LeaveRoom(r.ID); err != nil {
		t.Fatal(err)
	}
	c.SyncAll()
	waitFor(t, "c2 read-only", func() bool { s, _ := summaryOf(c2, r.ID); return s.Closed })
	if err := c2.Post(r.ID, "note", "ghost", "", nil); err == nil {
		t.Fatal("c2 posted after its user left")
	}
}

// Final review I6/I7: restoring a colleague's room keeps it listed until it
// arrives, and a new invite to a hidden room joins cleanly.
func TestHiddenColleagueRoomRestoredByInvite(t *testing.T) {
	a, c := newMachine(t), newMachine(t)
	a.Setup(true, "H", "mini")
	r, _ := a.CreateRoom("/p", "Ayina", "")
	invite(t, a, r.ID, c)
	if err := c.RemoveLocal(r.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.RestoreLocal(r.ID); err != nil {
		t.Fatal(err)
	}
	if h := c.Hidden(); len(h) != 1 || !h[0].Restoring {
		t.Fatalf("restoring room vanished from the list: %+v", h)
	}
	invite(t, a, r.ID, c) // fails today: "could not prove it belongs to the room's owner"
	if _, ok := summaryOf(c, r.ID); !ok || len(c.Hidden()) != 0 {
		t.Fatalf("not restored by the invite; hidden %+v", c.Hidden())
	}
}

// Final review I8: a sync round that finishes after RemoveLocal must not
// recreate the room's files.
func TestRemovedRoomNotRecreatedByLateSync(t *testing.T) {
	a, b := newMachine(t), newMachine(t)
	a.Setup(true, "H", "mini")
	rs, _ := a.CreateRoom("/p", "Ayina", "")
	pairOwn(t, a, b)
	waitFor(t, "room on b", func() bool { _, ok := summaryOf(b, rs.ID); return ok })
	b.mu.Lock()
	stale := b.rooms[rs.ID]
	b.mu.Unlock()
	if err := b.RemoveLocal(rs.ID); err != nil {
		t.Fatal(err)
	}
	a.Post(rs.ID, "note", "late", "", nil)
	a.mu.Lock()
	es := a.rooms[rs.ID].lanes.All()
	a.mu.Unlock()
	b.mu.Lock()
	b.apply(stale, es) // what an in-flight roundWith would do
	b.mu.Unlock()
	if _, err := os.Stat(filepath.Join(b.dir, rs.ID)); err == nil {
		t.Fatal("removed room's folder recreated by a late sync")
	}
	_ = roomlog.KindMsg
}
