package rooms

import (
	"github.com/zalando/go-keyring"
	"strings"

	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/codes"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/netsel"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/roomlog"
)

type memRing struct {
	mu sync.Mutex
	m  map[string]string
}

func (r *memRing) Get(s, u string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.m[s+u]; ok {
		return v, nil
	}
	return "", keyring.ErrNotFound
}
func (r *memRing) Set(s, u, v string) error { r.mu.Lock(); r.m[s+u] = v; r.mu.Unlock(); return nil }
func (r *memRing) Delete(s, u string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[s+u]; !ok {
		return keyring.ErrNotFound
	}
	delete(r.m, s+u)
	return nil
}

type machine struct {
	*Service
	dir  string
	ring *memRing
}

func newMachine(t *testing.T) *machine {
	t.Helper()
	m := &machine{dir: t.TempDir(), ring: &memRing{m: map[string]string{}}}
	m.boot(t)
	return m
}

func (m *machine) boot(t *testing.T) {
	t.Helper()
	m.Service = &Service{Dir: m.dir, Ring: m.ring, ListenIPs: []string{"127.0.0.1"}, TailscaleUp: func() bool { return false }, TailPeers: func() []netsel.TailPeer { return nil }}
	if err := m.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func confirmNext(t *testing.T, host *machine, ok bool) []string {
	t.Helper()
	waitFor(t, "pending confirm", func() bool { return host.PendingConfirm() != nil })
	w := host.PendingConfirm().Words
	host.Confirm(ok)
	return w
}

func joined(t *testing.T, m *machine, status string) {
	t.Helper()
	waitFor(t, "join "+status, func() bool { j := m.JoinStatus(); return j != nil && j.Status == status })
}

// pairOwn: a becomes the first machine, b pairs with it on the LAN.
func pairOwn(t *testing.T, a, b *machine) {
	t.Helper()
	if !a.Status().SetUp {
		if err := a.Setup(true, "Hussain", "mini"); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Setup(false, "", ""); err != nil {
		t.Fatal(err)
	}
	code, err := a.PairCode(netsel.LAN)
	if err != nil {
		t.Fatal(err)
	}
	cv, err := b.Join(code.Code, "laptop", "")
	if err != nil {
		t.Fatal(err)
	}
	words := confirmNext(t, a, true)
	if len(words) != 4 || words[0] != cv.Words[0] || words[3] != cv.Words[3] {
		t.Fatalf("words differ: host %v joiner %v", words, cv.Words)
	}
	joined(t, b, "done")
}

func invite(t *testing.T, owner *machine, room string, c *machine) {
	t.Helper()
	if !c.Status().SetUp {
		if err := c.Setup(true, "Colleague", "cpc"); err != nil {
			t.Fatal(err)
		}
	}
	code, err := owner.InviteCode(room, netsel.LAN)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Join(code.Code, "cpc", "/c/ayina"); err != nil {
		t.Fatal(err)
	}
	confirmNext(t, owner, true)
	joined(t, c, "done")
}

func texts(t *testing.T, m *machine, room string) []string {
	v, err := m.View(room)
	if err != nil {
		return nil
	}
	var out []string
	for _, th := range v.Threads {
		out = append(out, th.Root.Text)
	}
	return out
}

func has(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func deliveryFor(t *testing.T, m *machine, room, text, dev string) string {
	v, _ := m.View(room)
	for _, th := range v.Threads {
		if th.Root.Text == text {
			return th.Root.Delivery[dev]
		}
	}
	return ""
}

func TestNotSetUpTouchesNothing(t *testing.T) {
	m := newMachine(t)
	if m.Status().SetUp || m.node != nil || len(m.ring.m) != 0 || len(m.Rooms()) != 0 {
		t.Fatalf("not set up but node=%v keyring=%d", m.node != nil, len(m.ring.m))
	}
}

func TestPairOwnMachineAndRoomsFollow(t *testing.T) {
	a, b := newMachine(t), newMachine(t)
	a.Setup(true, "Hussain", "mini")
	r, err := a.CreateRoom("/p/ayina", "Ayina", "github.com/x/ayina")
	if err != nil {
		t.Fatal(err)
	}
	pairOwn(t, a, b)
	st := b.Status()
	if !st.SetUp || st.First || st.Name != "Hussain" {
		t.Fatalf("b status %+v", st)
	}
	waitFor(t, "room on b", func() bool { return len(b.Rooms()) == 1 && b.Rooms()[0].Name == "Ayina" })
	if b.Rooms()[0].Folder != "" {
		t.Fatal("folder binding must stay local")
	}
	a.Post(r.ID, "note", "hello laptop", "", nil)
	waitFor(t, "message on b", func() bool { return has(texts(t, b, r.ID), "hello laptop") })
	// Restart b: identity and rooms survive.
	b.Close()
	b.boot(t)
	if !b.Status().SetUp || len(b.Rooms()) != 1 {
		t.Fatalf("after restart %+v", b.Status())
	}
}

func TestPairDeclinedAndCodeSingleUse(t *testing.T) {
	a, b := newMachine(t), newMachine(t)
	a.Setup(true, "H", "mini")
	b.Setup(false, "", "")
	code, _ := a.PairCode(netsel.LAN)
	b.Join(code.Code, "laptop", "")
	confirmNext(t, a, false)
	joined(t, b, "failed")
	if b.Status().SetUp {
		t.Fatal("declined pairing set b up")
	}
	b.Join(code.Code, "laptop", "")
	joined(t, b, "failed") // window closed after the decline
}

func TestInvitePostAndDelivery(t *testing.T) {
	a, c := newMachine(t), newMachine(t)
	a.Setup(true, "H", "mini")
	r, _ := a.CreateRoom("/p", "Ayina", "")
	invite(t, a, r.ID, c)
	cs := c.Rooms()
	if len(cs) != 1 || cs[0].Folder != "/c/ayina" || cs[0].Owner {
		t.Fatalf("c rooms %+v", cs)
	}
	a.Post(r.ID, "handoff", "run the windows tests", "", map[string]string{"goal": "green"})
	waitFor(t, "message on c", func() bool { return has(texts(t, c, r.ID), "run the windows tests") })
	cdev := c.Status().Device
	waitFor(t, "delivered to c", func() bool { return deliveryFor(t, a, r.ID, "run the windows tests", cdev) == "delivered" })
	v, _ := c.View(r.ID)
	c.MarkSeen(r.ID, v.Threads[0].Root.Ref)
	waitFor(t, "seen by c", func() bool { return deliveryFor(t, a, r.ID, "run the windows tests", cdev) == "seen" })
	av, _ := a.View(r.ID)
	if len(av.Peers) != 1 || av.Peers[0].State != "direct (Local network)" || av.Peers[0].Net != "lan" || av.Peers[0].Mine {
		t.Fatalf("a peers %+v", av.Peers)
	}
	// c replies in the thread.
	c.Post(r.ID, "result", "all green", v.Threads[0].Root.Ref, map[string]string{"outcome": "pass"})
	waitFor(t, "reply on a", func() bool {
		av, _ := a.View(r.ID)
		return len(av.Threads) == 1 && len(av.Threads[0].Replies) == 1
	})
}

func TestQueuedRelayedDelivered(t *testing.T) {
	a, b, c := newMachine(t), newMachine(t), newMachine(t)
	a.Setup(true, "H", "mini")
	r, _ := a.CreateRoom("/p", "Ayina", "")
	pairOwn(t, a, b)
	invite(t, a, r.ID, c)
	waitFor(t, "b knows c", func() bool { v, _ := b.View(r.ID); return len(v.Peers) == 2 })
	cdev := c.Status().Device
	b.Close()
	c.Close()
	a.Post(r.ID, "note", "while away", "", nil)
	if a.QueuedCount() != 1 {
		t.Fatalf("queued %d", a.QueuedCount())
	}
	if got := deliveryFor(t, a, r.ID, "while away", cdev); got != "queued" {
		t.Fatalf("c state %q", got)
	}
	b.boot(t) // b comes back and dials a
	waitFor(t, "relayed via b", func() bool { return deliveryFor(t, a, r.ID, "while away", cdev) == "relayed" })
	if a.QueuedCount() != 0 {
		t.Fatalf("queued after relay %d", a.QueuedCount())
	}
	c.boot(t)
	waitFor(t, "delivered to c", func() bool { return deliveryFor(t, a, r.ID, "while away", cdev) == "delivered" })
}

func TestWrongNetworkStopsSync(t *testing.T) {
	a, c := newMachine(t), newMachine(t)
	a.Setup(true, "H", "mini")
	r, _ := a.CreateRoom("/p", "Ayina", "")
	invite(t, a, r.ID, c)
	if err := c.SetDeviceNetwork(a.Status().Device, "internet"); err == nil {
		t.Fatal("accepted an unknown network")
	}
	c.SetDeviceNetwork(a.Status().Device, "tailscale") // c now only talks to a over Tailscale
	a.Post(r.ID, "note", "not over the LAN", "", nil)
	time.Sleep(1500 * time.Millisecond)
	if has(texts(t, c, r.ID), "not over the LAN") {
		t.Fatal("synced over a network the user did not choose")
	}
	v, _ := c.View(r.ID)
	if len(v.Peers) != 1 || v.Peers[0].Net != "tailscale" || v.Peers[0].State != "unreachable" || v.Peers[0].Reason == "" {
		t.Fatalf("peer %+v", v.Peers)
	}
	c.SetDeviceNetwork(a.Status().Device, "lan")
	waitFor(t, "sync back on LAN", func() bool { return has(texts(t, c, r.ID), "not over the LAN") })
}

func TestSyncRefusesNonMembers(t *testing.T) {
	a, d := newMachine(t), newMachine(t)
	a.Setup(true, "H", "mini")
	r, _ := a.CreateRoom("/p", "Ayina", "")
	d.Setup(true, "Mallory", "evil")
	dev := d.Status().Device
	a.SetDeviceNetwork(dev, "lan") // a knows the device but it is not in the room
	cert, _ := d.myCert()
	body, _ := json.Marshal(syncReq{Room: r.ID, Cert: cert, Heads: map[string]int64{}})
	id, _ := peer.Decode(dev)
	if _, err := a.handleSync(id, netsel.LAN, body); err == nil {
		t.Fatal("non-member got the room")
	}
	if _, err := a.handleSync(id, netsel.Tailscale, body); err == nil {
		t.Fatal("wrong network accepted")
	}
}

func TestPostValidation(t *testing.T) {
	a := newMachine(t)
	a.Setup(true, "H", "mini")
	r, _ := a.CreateRoom("/p", "Ayina", "")
	for _, c := range []struct{ kind, text string }{{"spam", "x"}, {"note", "  "}, {"note", string(make([]byte, 17<<10))}} {
		if a.Post(r.ID, c.kind, c.text, "", nil) == nil {
			t.Fatalf("accepted %q", c.kind)
		}
	}
	if _, err := filepath.Abs(a.dir); err != nil {
		t.Fatal(err)
	}
}

// Security review: a colleague in room 1 must never be offered room 2, even
// by a tampered peer that pretends to hold nothing.
func TestSyncOffersOnlySharedRooms(t *testing.T) {
	a, c := newMachine(t), newMachine(t)
	a.Setup(true, "H", "mini")
	r1, _ := a.CreateRoom("/p1", "Shared", "")
	r2, _ := a.CreateRoom("/p2", "Private", "")
	a.Post(r2.ID, "note", "secret plans", "", nil)
	invite(t, a, r1.ID, c)
	// c turns malicious: answers every sync with empty heads and records what it is sent.
	var mu sync.Mutex
	got := map[string]int{}
	c.node.Handle(protoSync, func(peer.ID) int { return maxBatch + (1 << 20) }, func(from peer.ID, _ netsel.Network, raw []byte) ([]byte, error) {
		var req syncReq
		json.Unmarshal(raw, &req)
		mu.Lock()
		got[req.Room] += 1 + len(req.Entries)
		mu.Unlock()
		b, _ := json.Marshal(syncResp{Heads: map[string]int64{}})
		return b, nil
	})
	a.syncPeer(mustPeer(t, c.Status().Device))
	mu.Lock()
	defer mu.Unlock()
	if got[r2.ID] != 0 {
		t.Fatalf("room 2 offered to a non-member: %v", got)
	}
	if got[r1.ID] == 0 {
		t.Fatal("room 1 not synced at all (test broken)")
	}
}

func mustPeer(t *testing.T, s string) peer.ID {
	id, err := peer.Decode(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Security review: a revoked machine of my own user is refused too.
func TestRevokedOwnDeviceRefused(t *testing.T) {
	a, b := newMachine(t), newMachine(t)
	a.Setup(true, "H", "mini")
	r, _ := a.CreateRoom("/p", "Ayina", "")
	pairOwn(t, a, b)
	waitFor(t, "b in room", func() bool { return len(b.Rooms()) == 1 })
	bdev := b.Status().Device
	uk, _ := a.ids.UserKey()
	bpub, _ := b.ids.DevicePub()
	a.mu.Lock()
	a.rooms[r.ID].w.Write(roomlog.KindDeviceRevoke, roomlog.SignRevoke(uk, bpub, a.rooms[r.ID].lanes.Heads()[bdev]))
	a.mu.Unlock()
	cert, _ := b.myCert()
	body, _ := json.Marshal(syncReq{Room: r.ID, Cert: cert, Heads: map[string]int64{}})
	if _, err := a.handleSync(mustPeer(t, bdev), netsel.LAN, body); err == nil {
		t.Fatal("revoked own device was served")
	}
}

// Security review: an invite host must prove it is the owner's machine.
func TestJoinRequiresOwnerCert(t *testing.T) {
	ri := roomInfo{}
	owner := []byte("01234567890123456789012345678901")
	ri.Meta.Owner, ri.Meta.Nonce = owner, []byte("n")
	ri.ID = roomlog.RoomIDOf(owner, ri.Meta.Nonce)
	a := newMachine(t)
	a.Setup(true, "H", "mini")
	host := a.Status().Device
	cert, _ := a.myCert()
	if hostIsOwner(pairResp{Cert: &cert}, host, ri) {
		t.Fatal("a cert for a different user passed as the owner's")
	}
	ri.Meta.Owner = cert.User
	ri.ID = roomlog.RoomIDOf(cert.User, ri.Meta.Nonce)
	if !hostIsOwner(pairResp{Cert: &cert}, host, ri) {
		t.Fatal("the owner's own cert rejected")
	}
	if hostIsOwner(pairResp{}, host, ri) || hostIsOwner(pairResp{Cert: &cert}, "12D3KooWother", ri) {
		t.Fatal("missing cert or wrong device accepted")
	}
}

func init() { netsel.AllowLoopback = true }

// ---- final review fixes ----

func TestFailedJoinLeavesNoTrust(t *testing.T) {
	a, b := newMachine(t), newMachine(t)
	a.Setup(true, "H", "mini")
	b.Setup(false, "", "")
	code, _ := a.PairCode(netsel.LAN)
	b.Join(code.Code, "laptop", "")
	confirmNext(t, a, false)
	joined(t, b, "failed")
	b.mu.Lock()
	_, known := b.devices[a.Status().Device]
	b.mu.Unlock()
	if known {
		t.Fatal("a failed join left the host as a known device")
	}
}

func TestCodeCannotFlipKnownDeviceNetwork(t *testing.T) {
	a, c := newMachine(t), newMachine(t)
	a.Setup(true, "H", "mini")
	r, _ := a.CreateRoom("/p", "Ayina", "")
	invite(t, a, r.ID, c)
	forged := codes.Code{Kind: codes.KindInvite, Peer: a.Status().Device, Addrs: []string{"/ip4/100.83.12.118/tcp/4011"}, Net: "tailscale",
		Secret: []byte("0123456789abcdef"), Expires: time.Now().Add(time.Hour).Unix(), Room: r.ID}
	if _, err := c.Join(codes.Encode(forged), "cpc", ""); err == nil {
		t.Fatal("a code switched a known machine's network")
	}
	c.mu.Lock()
	net := c.devices[a.Status().Device].Net
	c.mu.Unlock()
	if net != netsel.LAN {
		t.Fatalf("network changed to %q", net)
	}
}

func TestRejectedEntriesCountedPerPeer(t *testing.T) {
	a, c := newMachine(t), newMachine(t)
	a.Setup(true, "H", "mini")
	r, _ := a.CreateRoom("/p", "Ayina", "")
	invite(t, a, r.ID, c)
	cert, _ := c.myCert()
	bad := roomlog.Entry{Room: r.ID, Device: c.Status().Device, Seq: 99, Lamport: 5, Time: "t", Kind: roomlog.KindMsg, Body: json.RawMessage(`{}`)}
	body, _ := json.Marshal(syncReq{Room: r.ID, Cert: cert, Heads: map[string]int64{}, Entries: []roomlog.Entry{bad}})
	if _, err := a.handleSync(mustPeer(t, c.Status().Device), netsel.LAN, body); err != nil {
		t.Fatal(err)
	}
	v, _ := a.View(r.ID)
	if len(v.Peers) != 1 || v.Peers[0].Rejected == 0 {
		t.Fatalf("rejected not counted: %+v", v.Peers)
	}
}

func TestWrongNetworkAttemptShownWithReason(t *testing.T) {
	a, c := newMachine(t), newMachine(t)
	a.Setup(true, "H", "mini")
	r, _ := a.CreateRoom("/p", "Ayina", "")
	invite(t, a, r.ID, c)
	c.SetDeviceNetwork(a.Status().Device, "tailscale")
	a.DialAll() // a still reaches out over the LAN; c refuses and remembers
	waitFor(t, "reason on c", func() bool {
		v, _ := c.View(r.ID)
		return len(v.Peers) == 1 && strings.Contains(v.Peers[0].Reason, "Local network")
	})
	if v, _ := c.View(r.ID); v.Peers[0].Tried != "lan" {
		t.Fatalf("tried = %q, want lan (the panel offers that switch, UX §6)", v.Peers[0].Tried)
	}
}
