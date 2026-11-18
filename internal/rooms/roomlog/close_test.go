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
