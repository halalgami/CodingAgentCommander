package roomlog

import (
	"github.com/zalando/go-keyring"

	"crypto/ed25519"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/ident"
)

type ring map[string]string

func (m ring) Get(s, u string) (string, error) {
	if v, ok := m[s+u]; ok {
		return v, nil
	}
	return "", keyring.ErrNotFound
}
func (m ring) Set(s, u, v string) error { m[s+u] = v; return nil }
func (m ring) Delete(s, u string) error { delete(m, s+u); return nil }

// dev is one device with its own lane writer.
type dev struct {
	st   *ident.Store
	id   ident.Identity
	w    *Writer
	lane *Lanes
}

func newUser(t *testing.T, name string) *dev {
	t.Helper()
	st := &ident.Store{Dir: t.TempDir(), Ring: ring{}}
	id, err := st.CreateFirst(name, name+"-pc")
	if err != nil {
		t.Fatal(err)
	}
	k, _ := st.DeviceKey()
	l := &Lanes{Dir: t.TempDir(), Room: "r1"}
	return &dev{st: st, id: id, w: &Writer{Room: "r1", Key: k, Lanes: l}, lane: l}
}

func (d *dev) post(t *testing.T, kind string, body any) Entry {
	t.Helper()
	e, err := d.w.Write(kind, body)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// sync copies every entry b lacks from a.
func syncTo(t *testing.T, from, to *dev) {
	t.Helper()
	for lane, head := range from.lane.Heads() {
		for _, e := range from.lane.Since(lane, to.lane.Heads()[lane], 1<<30) {
			if err := to.lane.Append(e); err != nil {
				t.Fatalf("append %s/%d: %v", lane, e.Seq, err)
			}
		}
		_ = head
	}
}

func TestSignVerifyAndTamper(t *testing.T) {
	a := newUser(t, "a")
	e := a.post(t, KindMsg, Msg{Kind: "note", Text: "hi"})
	if err := Verify(e); err != nil {
		t.Fatal(err)
	}
	e.Body = json.RawMessage(`{"kind":"note","text":"evil"}`)
	if Verify(e) == nil {
		t.Fatal("tampered entry verified")
	}
}

func TestAppendRejectsGapDupOversizeWrongRoom(t *testing.T) {
	a := newUser(t, "a")
	e1 := a.post(t, KindMsg, Msg{Kind: "note", Text: "1"})
	e2 := a.post(t, KindMsg, Msg{Kind: "note", Text: "2"})
	b := newUser(t, "b")
	if err := b.lane.Append(e2); !errors.Is(err, ErrGap) {
		t.Fatalf("gap: %v", err)
	}
	b.lane.Append(e1)
	if err := b.lane.Append(e1); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("dup: %v", err)
	}
	if _, err := a.w.Write(KindMsg, Msg{Kind: "note", Text: strings.Repeat("x", 70<<10)}); err == nil {
		t.Fatal("oversized entry written")
	}
	other := &Lanes{Dir: t.TempDir(), Room: "r2"}
	if err := other.Append(e1); err == nil {
		t.Fatal("wrong room accepted")
	}
	// Lanes reload from disk.
	re := &Lanes{Dir: a.lane.Dir, Room: "r1"}
	if re.Heads()[e1.Device] != 2 {
		t.Fatalf("reload heads %v", re.Heads())
	}
}

func TestLamportOrderAcrossLanes(t *testing.T) {
	a, b := newUser(t, "a"), newUser(t, "b")
	a.post(t, KindMsg, Msg{Kind: "note", Text: "a1"})
	syncTo(t, a, b)
	b.w.Lanes = b.lane
	eb := b.post(t, KindMsg, Msg{Kind: "note", Text: "b1"})
	if eb.Lamport <= 1 {
		t.Fatalf("lamport %d not after seen", eb.Lamport)
	}
	all := b.lane.All()
	if len(all) != 2 || all[0].Lamport > all[1].Lamport {
		t.Fatalf("order %+v", all)
	}
}

// owner a creates the room, invites b; b posts; a removes b; b's later post ignored.
func TestReplayMembership(t *testing.T) {
	a, b := newUser(t, "a"), newUser(t, "b")
	c := newUser(t, "c")
	newRoom(t, a, b, c)
	a.post(t, KindMemberAdd, Member{User: b.id.User, Role: "member"})
	syncTo(t, a, b)
	b.post(t, KindDeviceCert, b.id.Certs[0])
	b.post(t, KindMsg, Msg{Kind: "note", Text: "while member"})
	syncTo(t, b, a)
	a.post(t, KindMemberRemove, Member{User: b.id.User, Heads: a.lane.Heads()})
	syncTo(t, a, b)
	b.post(t, KindMsg, Msg{Kind: "note", Text: "after removal"})
	syncTo(t, b, a)
	st := Replay(a.lane.All())
	if st.Meta.Name != "Ayina" || st.Members[string(a.id.User)] != "owner" {
		t.Fatalf("meta/owner %+v", st)
	}
	if _, ok := st.Members[string(b.id.User)]; ok {
		t.Fatal("removed member still a member")
	}
	var texts []string
	for _, e := range st.Accepted {
		if e.Kind == KindMsg {
			var m Msg
			json.Unmarshal(e.Body, &m)
			texts = append(texts, m.Text)
		}
	}
	if strings.Join(texts, ",") != "while member" {
		t.Fatalf("accepted msgs %v", texts)
	}
	// A stranger who never got member.add is not accepted at all.
	syncTo(t, a, c)
	c.post(t, KindDeviceCert, c.id.Certs[0])
	c.post(t, KindMsg, Msg{Kind: "note", Text: "stranger"})
	syncTo(t, c, a)
	for _, e := range Replay(a.lane.All()).Accepted {
		if e.Device == c.id.PeerID() {
			t.Fatal("stranger entry accepted")
		}
	}
}

func TestDelivery(t *testing.T) {
	a, b := newUser(t, "a"), newUser(t, "b")
	newRoom(t, a, b)
	a.post(t, KindMemberAdd, Member{User: b.id.User, Role: "member"})
	m := a.post(t, KindMsg, Msg{Kind: "note", Text: "x"})
	devs := []string{a.id.PeerID(), b.id.PeerID()}
	if d := Delivery(m, devs, a.lane.All(), nil); d[b.id.PeerID()] != Queued {
		t.Fatalf("before sync %v", d)
	}
	// a has seen b's heads cover the message, but no receipt yet → relayed.
	seen := map[string]map[string]int64{b.id.PeerID(): {a.id.PeerID(): m.Seq}}
	if d := Delivery(m, devs, a.lane.All(), seen); d[b.id.PeerID()] != Relayed {
		t.Fatalf("observed %v", d)
	}
	syncTo(t, a, b)
	b.post(t, KindDeviceCert, b.id.Certs[0])
	b.post(t, KindReceipt, Receipt{Lane: a.id.PeerID(), Seq: m.Seq, State: "delivered"})
	syncTo(t, b, a)
	if d := Delivery(m, devs, a.lane.All(), nil); d[b.id.PeerID()] != Delivered {
		t.Fatalf("receipt %v", d)
	}
	b.post(t, KindReceipt, Receipt{Lane: a.id.PeerID(), Seq: m.Seq, State: "seen"})
	syncTo(t, b, a)
	if d := Delivery(m, devs, a.lane.All(), nil); d[b.id.PeerID()] != Seen {
		t.Fatalf("seen %v", d)
	}
}

// ---- security review fixes ----

// newRoom: owner a creates room id bound to its user key; returns the writer room id.
func newRoom(t *testing.T, a *dev, others ...*dev) string {
	t.Helper()
	meta, room := NewRoom(a.id.User, "Ayina", "")
	for _, d := range append([]*dev{a}, others...) {
		d.w.Room, d.lane.Room = room, room
	}
	a.post(t, KindDeviceCert, a.id.Certs[0])
	a.post(t, KindRoomMeta, meta)
	return room
}

func TestForgedMetaCannotTakeOwnership(t *testing.T) {
	a, m := newUser(t, "a"), newUser(t, "mallory")
	newRoom(t, a, m)
	a.post(t, KindMemberAdd, Member{User: m.id.User})
	syncTo(t, a, m)
	m.post(t, KindDeviceCert, m.id.Certs[0])
	forged := RoomMeta{Name: "mine", Owner: m.id.User, Nonce: []byte("x")}
	e := Entry{Room: m.w.Room, Device: m.id.PeerID(), Seq: 2, Lamport: 1, Time: "t", Kind: KindRoomMeta}
	e.Body, _ = json.Marshal(forged)
	k, _ := m.st.DeviceKey()
	Sign(&e, k)
	m.lane.Append(e)
	syncTo(t, m, a)
	st := Replay(a.lane.All())
	if st.Members[string(a.id.User)] != "owner" || st.Members[string(m.id.User)] == "owner" || st.Meta.Name != "Ayina" {
		t.Fatalf("ownership taken: %+v", st.Members)
	}
}

func TestLamportMustRiseWithinALane(t *testing.T) {
	a := newUser(t, "a")
	newRoom(t, a)
	k, _ := a.st.DeviceKey()
	for _, lam := range []int64{0, 2} { // 2 == previous (meta has lamport 2)
		e := Entry{Room: a.w.Room, Device: a.id.PeerID(), Seq: 3, Lamport: lam, Time: "t", Kind: KindMsg, Body: json.RawMessage(`{}`)}
		Sign(&e, k)
		if err := a.lane.Append(e); err == nil {
			t.Fatalf("lamport %d accepted", lam)
		}
	}
}

func TestBackdatedEntriesAfterRemovalIgnored(t *testing.T) {
	a, b := newUser(t, "a"), newUser(t, "b")
	newRoom(t, a, b)
	a.post(t, KindMemberAdd, Member{User: b.id.User})
	syncTo(t, a, b)
	b.post(t, KindDeviceCert, b.id.Certs[0])
	syncTo(t, b, a)
	for i := 0; i < 3; i++ { // the room moves on; Lamport climbs past b's last entry
		a.post(t, KindMsg, Msg{Kind: "note", Text: "owner"})
	}
	a.post(t, KindMemberRemove, Member{User: b.id.User, Heads: a.lane.Heads()})
	// b, removed, writes an entry whose Lamport sorts before the removal.
	k, _ := b.st.DeviceKey()
	last := b.lane.Since(b.id.PeerID(), 0, 1<<20)[0].Lamport
	e := Entry{Room: b.w.Room, Device: b.id.PeerID(), Seq: 2, Lamport: last + 1, Time: "t", Kind: KindMsg}
	e.Body, _ = json.Marshal(Msg{Kind: "note", Text: "backdated"})
	Sign(&e, k)
	if err := b.lane.Append(e); err != nil {
		t.Fatal(err)
	}
	syncTo(t, b, a)
	for _, x := range Replay(a.lane.All()).Accepted {
		if x.Kind == KindMsg && x.Device == b.id.PeerID() {
			t.Fatal("backdated entry from a removed member accepted")
		}
	}
}

func TestRevokeOnlyOwnDevices(t *testing.T) {
	a, b := newUser(t, "a"), newUser(t, "b")
	newRoom(t, a, b)
	a.post(t, KindMemberAdd, Member{User: b.id.User})
	syncTo(t, a, b)
	b.post(t, KindDeviceCert, b.id.Certs[0])
	// b signs a's device key with b's own user key and "revokes" it.
	uk, _ := b.st.UserKey()
	b.post(t, KindDeviceRevoke, Revoke{Device: a.id.Device, Sig: ed25519.Sign(uk, revokeBytes(a.id.Device, 0))})
	syncTo(t, b, a)
	if Replay(a.lane.All()).Revoked[a.id.PeerID()] {
		t.Fatal("a member revoked someone else's machine")
	}
}

// ---- final review fixes ----

func TestLamportOverflowRejected(t *testing.T) {
	a := newUser(t, "a")
	newRoom(t, a)
	k, _ := a.st.DeviceKey()
	e := Entry{Room: a.w.Room, Device: a.id.PeerID(), Seq: 3, Lamport: math.MaxInt64, Time: "t", Kind: KindMsg, Body: json.RawMessage(`{}`)}
	Sign(&e, k)
	if err := a.lane.Append(e); err == nil {
		t.Fatal("absurd lamport accepted")
	}
	if _, err := a.w.Write(KindMsg, Msg{Kind: "note", Text: "still writable"}); err != nil {
		t.Fatal(err)
	}
}

func TestNewLaneNeedsAdmission(t *testing.T) {
	a, x := newUser(t, "a"), newUser(t, "x")
	newRoom(t, a, x)
	a.lane.Admit = func(first Entry) bool { return first.Kind == KindDeviceCert && first.Device == a.id.PeerID() }
	k, _ := x.st.DeviceKey()
	e := Entry{Room: a.w.Room, Device: x.id.PeerID(), Seq: 1, Lamport: 1, Time: "t", Kind: KindMsg, Body: json.RawMessage(`{}`)}
	Sign(&e, k)
	if err := a.lane.Append(e); err == nil {
		t.Fatal("unadmitted lane stored")
	}
}

func TestTornLineTruncatedOnLoad(t *testing.T) {
	a := newUser(t, "a")
	newRoom(t, a)
	f := filepath.Join(a.lane.Dir, a.id.PeerID()+".jsonl")
	fh, _ := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0o600)
	fh.WriteString(`{"room":"torn`)
	fh.Close()
	re := &Lanes{Dir: a.lane.Dir, Room: a.w.Room}
	k, _ := a.st.DeviceKey()
	w := &Writer{Room: a.w.Room, Key: k, Lanes: re}
	if _, err := w.Write(KindMsg, Msg{Kind: "note", Text: "after tear"}); err != nil {
		t.Fatal(err)
	}
	again := &Lanes{Dir: a.lane.Dir, Room: a.w.Room}
	if again.Heads()[a.id.PeerID()] != 3 {
		t.Fatalf("entry after a torn line lost on reload: heads %v", again.Heads())
	}
}

func TestRevokedOwnerDeviceCannotRemoveAfterCut(t *testing.T) {
	a, b := newUser(t, "a"), newUser(t, "b")
	newRoom(t, a, b)
	a.post(t, KindMemberAdd, Member{User: b.id.User})
	syncTo(t, a, b)
	b.post(t, KindDeviceCert, b.id.Certs[0])
	syncTo(t, b, a)
	uk, _ := a.st.UserKey()
	// a's device revokes itself (stolen) at its current head, then keeps writing.
	head := a.lane.Heads()[a.id.PeerID()]
	a.post(t, KindDeviceRevoke, SignRevoke(uk, a.id.Device, head+1))
	a.post(t, KindMemberRemove, Member{User: b.id.User, Heads: a.lane.Heads()})
	if _, ok := Replay(a.lane.All()).Members[string(b.id.User)]; !ok {
		t.Fatal("a removal written past a revoke cut-off took effect")
	}
}
