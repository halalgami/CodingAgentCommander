// Package rooms is Commander rooms (spec docs/superpowers/specs/2026-11-15-
// commander-rooms-design.md): identity, rooms, the signed room log, pairing,
// invites and peer-to-peer sync over the network the user chose (D10).
package rooms

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/ident"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/netsel"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/p2p"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/roomlog"
)

// DevicePref is what this machine knows about another device: above all the
// network the user chose for it (D10). Net "" = not chosen yet → never dialled.
type DevicePref struct {
	Net      netsel.Network `json:"net"`
	Name     string         `json:"name,omitempty"`
	User     []byte         `json:"user,omitempty"`
	Addrs    []string       `json:"addrs,omitempty"`
	LastSeen int64          `json:"lastSeen,omitempty"`
	Revoked  bool           `json:"revoked,omitempty"` // one of my machines I removed: never dialled or shared with
}

type roomState struct {
	Folder string          `json:"folder"`
	Seen   map[string]bool `json:"seen"`
	// Owner is the owner's user key, known from the room id's genesis before
	// any entry has synced (the id is bound to it, see roomlog.RoomIDOf).
	Owner []byte `json:"owner"`
	// Left: I left this colleague's room; my copy is read-only (UX spec §7).
	Left bool `json:"left,omitempty"`
	// Phase 3–5, all local to this machine:
	Dismissed map[string]bool `json:"dismissed,omitempty"` // inbox items dismissed here
	Settings  RoomSettings    `json:"settings"`
	Used      map[string]int  `json:"used,omitempty"`    // thread → automatic deliveries used (§10.4)
	Granted   map[string]int  `json:"granted,omitempty"` // thread → extra deliveries the user allowed
	Agent     []int64         `json:"agent,omitempty"`   // ms times of agent posts in the last hour
	Drafts    []Draft         `json:"drafts,omitempty"`  // agent posts held by the rate limit
}

type room struct {
	id    string
	lanes *roomlog.Lanes
	w     *roomlog.Writer
	st    roomState

	cacheKey int64          // sum of lane heads when cache was built
	cache    *roomlog.State // replayed state (review I8)
	ix       roomlog.Receipts

	umu   sync.Mutex
	users map[string]bool // users whose devices may open a lane (Admit)

	gone bool // removed from this machine; in-flight syncs must not touch it (final review I8)

	told map[string]bool // msg refs already announced (or history at open)

	named map[string]roomlog.Attachment // sha → attachment accepted messages name (review I2)
}

// stateLocked replays the room once per change of its lanes.
func (s *Service) stateLocked(r *room) roomlog.State {
	var key int64
	for _, h := range r.lanes.Heads() {
		key += h + 1
	}
	if r.cache == nil || key != r.cacheKey {
		st := roomlog.Replay(r.lanes.All())
		r.cache, r.cacheKey, r.ix = &st, key, roomlog.IndexReceipts(st.Accepted)
		r.named = nil // rebuilt on demand (namedLocked)
		users := map[string]bool{string(r.st.Owner): true, string(s.me.User): true}
		for u := range st.Members {
			users[u] = true
		}
		r.umu.Lock()
		r.users = users
		r.umu.Unlock()
	}
	return *r.cache
}

// admit: a new lane opens only with a certified device of the owner, a
// member or me (review I1: junk lanes can't fill the 256 slots).
func (r *room) admit(first roomlog.Entry) bool {
	if first.Kind != roomlog.KindDeviceCert {
		return false
	}
	var c ident.DeviceCert
	if json.Unmarshal(first.Body, &c) != nil || ident.VerifyCert(c) != nil || ident.PeerIDOf(c.Device) != first.Device {
		return false
	}
	r.umu.Lock()
	defer r.umu.Unlock()
	return r.users[string(c.User)]
}

type settings struct {
	Port int `json:"port"`
}

// Service owns everything rooms does on this machine.
type Service struct {
	Dir       string
	Ring      ident.Keyring
	Now       func() time.Time
	ListenIPs []string // tests: 127.0.0.1
	OnChange  func()
	// OnConfirm is told when a machine waits for the 4-word check here.
	OnConfirm func(Pending)
	// TailscaleUp reports whether Tailscale runs (stubbed in tests).
	TailscaleUp func() bool
	// TailPeers lists the tailnet's machines (stubbed in tests).
	TailPeers func() []netsel.TailPeer
	blobOnce  sync.Once
	blobSem   chan struct{} // blob requests served at once (review I2)
	tailMu    sync.Mutex
	tailAt    time.Time
	tailCache []netsel.TailPeer
	// OnMessage is told about each message another device wrote, once, when
	// it is first accepted here (notifications, listening). Not for history.
	OnMessage func(Incoming)
	// OnNotify shows a native notification: a handoff or question that
	// mentions this machine (spec §11). Status never notifies.
	OnNotify func(title, body string)

	mu       sync.Mutex
	ids      *ident.Store
	me       ident.Identity
	setUp    bool
	joining  bool
	node     *p2p.Node
	devices  map[string]DevicePref
	rooms    map[string]*room
	win      *window
	join     *joinState
	observed map[string]map[string]map[string]int64 // device → room → heads
	rejected map[string]int                         // device → entries refused from it (review I7)
	joinPeer *joinPeer                              // a code's host, trusted only for the join
	running  map[string]bool                        // device → sync in progress
	again    map[string]bool                        // device → sync again when done
	stop     chan struct{}
	stopped  bool
	tombs    map[string]tomb // removed from this machine
	joinStop context.CancelFunc
}

var roomIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) changed() {
	if s.OnChange != nil {
		go s.OnChange()
	}
}

// tailPeers is cached for 30 s: dialling every unreachable machine must not
// run `tailscale status` each time.
func (s *Service) tailPeers() []netsel.TailPeer {
	if s.TailPeers != nil {
		return s.TailPeers()
	}
	s.tailMu.Lock()
	defer s.tailMu.Unlock()
	if time.Since(s.tailAt) > 30*time.Second {
		_, s.tailCache = netsel.Status(context.Background())
		s.tailAt = time.Now()
	}
	return s.tailCache
}

func (s *Service) tailscaleUp() bool {
	if s.TailscaleUp != nil {
		return s.TailscaleUp()
	}
	up, _ := netsel.Status(context.Background())
	return up
}

// Open loads state and, when rooms are set up, starts the node. Nothing
// touches the keychain or the network until the user has set rooms up.
func (s *Service) Open() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Ring == nil {
		s.Ring = ident.OSKeyring
	}
	s.ids = &ident.Store{Dir: s.Dir, Ring: s.Ring, Now: s.Now}
	s.devices = map[string]DevicePref{}
	s.rooms = map[string]*room{}
	s.observed = map[string]map[string]map[string]int64{}
	s.running, s.again, s.rejected = map[string]bool{}, map[string]bool{}, map[string]int{}
	readJSON(filepath.Join(s.Dir, "devices.json"), &s.devices)
	s.tombs = map[string]tomb{}
	readJSON(filepath.Join(s.Dir, "tombstones.json"), &s.tombs)
	id, ok, err := s.ids.Load()
	if err != nil {
		return err
	}
	_, joinErr := os.Stat(filepath.Join(s.Dir, "joining"))
	s.joining = joinErr == nil
	if !ok && !s.joining {
		return nil
	}
	s.me, s.setUp = id, ok
	if ok {
		s.loadRoomsLocked()
	}
	return s.startLocked()
}

func readJSON(path string, v any) bool {
	b, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(b, v) == nil
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Service) saveDevicesLocked() { writeJSON(filepath.Join(s.Dir, "devices.json"), s.devices) }

func (s *Service) loadRoomsLocked() {
	ents, _ := os.ReadDir(s.Dir)
	for _, e := range ents {
		if _, gone := s.tombs[e.Name()]; e.IsDir() && roomIDRe.MatchString(e.Name()) && !gone {
			s.openRoomLocked(e.Name())
			go evictAttachments(s.attachDir(e.Name()), time.Now(), attachStoreCap)
		}
	}
}

func (s *Service) openRoomLocked(id string) *room {
	if r := s.rooms[id]; r != nil {
		return r
	}
	key, _ := s.ids.DeviceKey()
	l := &roomlog.Lanes{Dir: filepath.Join(s.Dir, id, "lanes"), Room: id}
	r := &room{id: id, lanes: l, w: &roomlog.Writer{Room: id, Key: key, Lanes: l, Now: s.Now}}
	readJSON(filepath.Join(s.Dir, id, "state.json"), &r.st)
	if r.st.Seen == nil {
		r.st.Seen = map[string]bool{}
	}
	if r.st.Dismissed == nil {
		r.st.Dismissed = map[string]bool{}
	}
	l.Admit = r.admit
	s.rooms[id] = r
	r.told = map[string]bool{}
	for _, e := range s.stateLocked(r).Accepted {
		if e.Kind == roomlog.KindMsg {
			r.told[e.Ref()] = true
		}
	}
	return r
}

func (s *Service) saveRoomLocked(r *room) { writeJSON(filepath.Join(s.Dir, r.id, "state.json"), r.st) }

func (s *Service) startLocked() error {
	if s.node != nil {
		return nil
	}
	dk, err := s.ids.DeviceKey()
	if err != nil {
		return err
	}
	key, err := crypto.UnmarshalEd25519PrivateKey(dk)
	if err != nil {
		return err
	}
	var st settings
	readJSON(filepath.Join(s.Dir, "settings.json"), &st)
	if st.Port == 0 && s.ListenIPs == nil {
		st.Port = 4011 // first try a stable port; random if taken
	}
	cfg := p2p.Config{Key: key, ListenIPs: s.ListenIPs, Port: st.Port, Policy: s.policy, Pairing: s.pairingNet, OnFound: s.found}
	n, err := p2p.New(cfg)
	if err != nil && st.Port != 0 {
		cfg.Port = 0
		n, err = p2p.New(cfg)
	}
	if err != nil {
		return err
	}
	s.node = n
	if s.ListenIPs == nil {
		for _, a := range n.Addrs() {
			if p, err := a.ValueForProtocol(ma.P_TCP); err == nil {
				var port int
				json.Unmarshal([]byte(p), &port)
				if port != 0 && port != st.Port {
					writeJSON(filepath.Join(s.Dir, "settings.json"), settings{Port: port})
				}
				break
			}
		}
	}
	n.Handle(protoBlob, func(p peer.ID) int {
		if _, known := s.policy(p); known {
			return blobReqLimit
		}
		return 0
	}, s.handleBlob)
	n.Handle(protoPair, func(peer.ID) int { return 64 << 10 }, s.handlePair)
	n.Handle(protoSync, func(p peer.ID) int {
		if _, known := s.policy(p); known { // strangers can't make us allocate (review I3)
			return maxBatch + (1 << 20)
		}
		return 0
	}, s.handleSync)
	n.Host().Network().Notify(&network.NotifyBundle{ConnectedF: func(_ network.Network, c network.Conn) {
		go s.syncPeer(c.RemotePeer())
	}})
	s.stop = make(chan struct{})
	go s.loop(s.stop)
	s.updateMDNSLocked()
	return nil
}

// Close stops the node.
func (s *Service) Close() {
	s.mu.Lock()
	if s.stop != nil && !s.stopped {
		close(s.stop)
		s.stopped = true
	}
	node := s.node
	s.node = nil
	s.mu.Unlock()
	// Outside the lock: closing the host waits for upgrades whose gater calls
	// take s.mu (review I4: shutdown deadlock).
	if node != nil {
		node.Close()
	}
}

// policy: the network the user chose for a device (D10).
func (s *Service) policy(id peer.ID) (netsel.Network, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[id.String()]
	if (!ok || d.Net == "") && s.joinPeer != nil && s.joinPeer.id == id {
		return s.joinPeer.net, true
	}
	if !ok || d.Net == "" {
		return "", false
	}
	return d.Net, true
}

// joinPeer: the host named in a code, trusted only while that join runs and
// only on the code's network (review I5). Persisted only on success.
type joinPeer struct {
	id    peer.ID
	net   netsel.Network
	addrs []string
}

func (s *Service) updateMDNSLocked() {
	if s.node == nil {
		return
	}
	on := s.win != nil && s.win.net == netsel.LAN
	for _, d := range s.devices {
		if d.Net == netsel.LAN {
			on = true
		}
	}
	s.node.SetMDNS(on)
}

// found: a device on the local network announced itself via mDNS.
func (s *Service) found(pi peer.AddrInfo) {
	s.mu.Lock()
	d, ok := s.devices[pi.ID.String()]
	if ok && d.Net == netsel.LAN {
		d.Addrs = addrStrings(pi.Addrs)
		s.devices[pi.ID.String()] = d
		s.saveDevicesLocked()
	}
	s.mu.Unlock()
	if ok && d.Net == netsel.LAN {
		s.dial(pi.ID)
	}
}

func addrStrings(as []ma.Multiaddr) []string {
	var out []string
	for _, a := range as {
		out = append(out, a.String())
	}
	sort.Strings(out)
	return out
}

// loop re-dials known devices on their chosen network every 30 s. It only
// ever talks to the user's own devices and room members on LAN / tailnet.
func (s *Service) loop(stop chan struct{}) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	s.DialAll()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.DialAll()
		}
	}
}

// DialAll connects to every known device not connected yet.
func (s *Service) DialAll() {
	s.mu.Lock()
	var ids []peer.ID
	for k, d := range s.devices {
		if d.Net == "" || d.Revoked {
			continue
		}
		if id, err := peer.Decode(k); err == nil {
			ids = append(ids, id)
		}
	}
	node := s.node
	s.mu.Unlock()
	for _, id := range ids {
		if node == nil {
			return
		}
		if _, ok := node.Conn(id); ok {
			go s.syncPeer(id)
			continue
		}
		s.dial(id)
	}
}

func (s *Service) dial(id peer.ID) {
	s.mu.Lock()
	node := s.node
	d := s.devices[id.String()]
	addrs := append([]string{}, d.Addrs...)
	if s.joinPeer != nil && s.joinPeer.id == id {
		addrs = append(addrs, s.joinPeer.addrs...)
	}
	tailnet := d.Net == netsel.Tailscale
	for _, r := range s.rooms {
		addrs = append(addrs, lastAddrs(r.lanes.All(), id.String())...)
	}
	s.mu.Unlock()
	if node == nil {
		return
	}
	if tailnet {
		addrs = append(addrs, tailnetCandidates(addrs, s.tailPeers())...)
	}
	var mas []ma.Multiaddr
	for _, a := range addrs {
		if m, err := ma.NewMultiaddr(a); err == nil {
			mas = append(mas, m)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if node.Dial(ctx, id, mas) == nil {
		s.touch(id)
	}
}

func lastAddrs(entries []roomlog.Entry, dev string) []string {
	var out []string
	for _, e := range entries {
		if e.Kind == roomlog.KindAddr && e.Device == dev {
			var a roomlog.Addr
			if json.Unmarshal(e.Body, &a) == nil {
				out = a.Addrs
				if len(out) > maxAddrs { // a peer can't make us dial a long list (phase-1 deferred)
					out = out[:maxAddrs]
				}
			}
		}
	}
	return out
}

func (s *Service) touch(id peer.ID) {
	s.mu.Lock()
	if d, ok := s.devices[id.String()]; ok {
		d.LastSeen = s.now().UnixMilli()
		if s.node != nil {
			for _, c := range s.node.Host().Network().ConnsToPeer(id) {
				d.Addrs = mergeAddr(d.Addrs, c.RemoteMultiaddr().String())
			}
		}
		s.devices[id.String()] = d
		s.saveDevicesLocked()
	}
	s.mu.Unlock()
}

const maxAddrs = 8

func mergeAddr(as []string, a string) []string {
	for _, x := range as {
		if x == a {
			return as
		}
	}
	as = append(as, a)
	if len(as) > maxAddrs {
		as = as[len(as)-maxAddrs:]
	}
	return as
}

// myAddrs: this node's addresses on LAN and tailnet only (never public).
func (s *Service) myAddrsLocked() []string {
	if s.node == nil {
		return nil
	}
	var out []string
	for _, a := range s.node.Addrs() {
		if _, ok := netsel.OfAddr(a); ok { // loopback only in tests (netsel.AllowLoopback)
			out = append(out, a.String())
		}
	}
	sort.Strings(out)
	return out
}

func (s *Service) addrsOn(n netsel.Network) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, a := range s.myAddrsLocked() {
		if got, ok := netsel.OfAddr(ma.StringCast(a)); ok && got == n {
			out = append(out, a)
		}
	}
	return out
}

// publishAddrs writes an addr entry in each room when the addresses changed.
func (s *Service) publishAddrsLocked() {
	s.publishProfileLocked()
	addrs := s.myAddrsLocked()
	if len(addrs) == 0 {
		return
	}
	for _, r := range s.rooms {
		if eq(lastAddrs(r.lanes.All(), s.me.PeerID()), addrs) {
			continue
		}
		r.w.Write(roomlog.KindAddr, roomlog.Addr{Addrs: addrs})
	}
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var errNotSetUp = errors.New("set up rooms first")

func (s *Service) myUser() []byte { return s.me.User }

func (s *Service) devicePub() ed25519.PublicKey {
	k, err := s.ids.DeviceKey()
	if err != nil {
		return nil
	}
	return k.Public().(ed25519.PublicKey)
}
