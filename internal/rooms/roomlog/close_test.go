package roomlog

import (
	"encoding/json"
	"testing"
)

// closeRoom builds owner a + member b, both certified, b a member.
func closeRoom(t *testing.T) (*dev, *dev) {
	t.Helper()
	a, b := newUser(t, "a"), newUser(t, "b")
	newRoom(t, a, b)
	a.post(t, KindMemberAdd, Member{User: b.id.User, Role: "member"})
	syncTo(t, a, b)
	b.post(t, KindDeviceCert, b.id.Certs[0])
	syncTo(t, b, a)
	return a, b
}

func msgTexts(st State) []string {
	var out []string
	for _, e := range st.Accepted {
		if e.Kind == KindMsg {
			var m Msg
			json.Unmarshal(e.Body, &m)
			out = append(out, m.Text)
		}
	}
	return out
}

func TestCloseByOwnerClosesRoom(t *testing.T) {
	a, _ := closeRoom(t)
	a.post(t, KindRoomClose, Close{})
	st := Replay(a.lane.All())
	if st.Closed == nil || st.Closed.Device != a.id.PeerID() {
		t.Fatalf("closed = %+v", st.Closed)
	}
}

func TestCloseByMemberIgnored(t *testing.T) {
	a, b := closeRoom(t)
	b.post(t, KindRoomClose, Close{})
	syncTo(t, b, a)
	if st := Replay(a.lane.All()); st.Closed != nil {
		t.Fatalf("member closed the room: %+v", st.Closed)
	}
}

func TestMsgAfterCloseNotAccepted(t *testing.T) {
	a, b := closeRoom(t)
	b.post(t, KindMsg, Msg{Kind: "note", Text: "before"})
	syncTo(t, b, a)
	a.post(t, KindRoomClose, Close{})
	syncTo(t, a, b)
	b.post(t, KindMsg, Msg{Kind: "note", Text: "after"})
	b.post(t, KindReceipt, Receipt{Lane: "x", Seq: 1, State: "seen"})
	syncTo(t, b, a)
	st := Replay(a.lane.All())
	if got := msgTexts(st); len(got) != 1 || got[0] != "before" {
		t.Fatalf("msgs %v", got)
	}
	receipt := false
	for _, e := range st.Accepted {
		if e.Kind == KindReceipt {
			receipt = true
		}
	}
	if !receipt {
		t.Fatal("receipt after close dropped")
	}
}

// backdate writes a signed entry on d's lane with a chosen Lamport time,
// the way a modified client could.
func backdate(t *testing.T, d *dev, kind string, body any, lamport int64) Entry {
	t.Helper()
	b, _ := json.Marshal(body)
	e := Entry{Room: d.w.Room, Device: d.id.PeerID(), Seq: d.lane.Heads()[d.id.PeerID()] + 1, Lamport: lamport, Time: "2026-11-19T00:00:00Z", Kind: kind, Body: b}
	if err := Sign(&e, d.w.Key); err != nil {
		t.Fatal(err)
	}
	if err := d.lane.Append(e); err != nil {
		t.Fatal(err)
	}
	return e
}

func lastLamport(d *dev) int64 {
	var l int64
	for _, e := range d.lane.All() {
		if e.Device == d.id.PeerID() {
			l = e.Lamport
		}
	}
	return l
}

func TestBackdatedMsgAfterCloseNotAccepted(t *testing.T) {
	a, b := closeRoom(t)
	for i := 0; i < 5; i++ {
		a.post(t, KindMsg, Msg{Kind: "note", Text: "owner"})
	}
	a.post(t, KindRoomClose, Close{Heads: a.lane.Heads()})
	syncTo(t, a, b)
	backdate(t, b, KindMsg, Msg{Kind: "note", Text: "backdated"}, lastLamport(b)+1)
	syncTo(t, b, a)
	for _, txt := range msgTexts(Replay(a.lane.All())) {
		if txt == "backdated" {
			t.Fatal("backdated message after the close was accepted")
		}
	}
}

func TestMsgSameLamportAsCloseNotAccepted(t *testing.T) {
	a, b := closeRoom(t)
	c := a.post(t, KindRoomClose, Close{})
	backdate(t, b, KindMsg, Msg{Kind: "note", Text: "tie"}, c.Lamport)
	syncTo(t, b, a)
	if got := msgTexts(Replay(a.lane.All())); len(got) != 0 {
		t.Fatalf("msgs %v", got)
	}
}

func TestRenameByOwnerOnly(t *testing.T) {
	a, b := closeRoom(t)
	if st := Replay(a.lane.All()); st.Name != "Ayina" {
		t.Fatalf("initial name %q", st.Name)
	}
	a.post(t, KindRoomRename, Rename{Name: "Commander"})
	b.post(t, KindRoomRename, Rename{Name: "Hijacked"})
	syncTo(t, b, a)
	if st := Replay(a.lane.All()); st.Name != "Commander" {
		t.Fatalf("name %q", st.Name)
	}
}

func TestRenameAfterCloseIgnored(t *testing.T) {
	a, _ := closeRoom(t)
	a.post(t, KindRoomClose, Close{Heads: a.lane.Heads()})
	a.post(t, KindRoomRename, Rename{Name: "Later"})
	if st := Replay(a.lane.All()); st.Name != "Ayina" {
		t.Fatalf("renamed after close: %q", st.Name)
	}
}

func TestRenameRejectsEmptyAndLong(t *testing.T) {
	a, _ := closeRoom(t)
	a.post(t, KindRoomRename, Rename{Name: "  "})
	a.post(t, KindRoomRename, Rename{Name: string(make([]byte, 81))})
	if st := Replay(a.lane.All()); st.Name != "Ayina" {
		t.Fatalf("name %q", st.Name)
	}
}
