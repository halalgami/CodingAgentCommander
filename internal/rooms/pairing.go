package rooms

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/codes"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/ident"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/netsel"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/roomlog"
)

const (
	protoPair   = "/commander/pair/1"
	pairTTL     = 10 * time.Minute
	inviteTTL   = 24 * time.Hour
	confirmWait = 150 * time.Second // < the 3-minute stream deadline (review minor 5)
)

// window is an open pairing or invite on one network.
type window struct {
	kind    string
	secret  []byte
	net     netsel.Network
	exp     time.Time
	room    string
	used    bool
	pending *pending
}

type pending struct {
	peer   peer.ID
	name   string
	words  [4]string
	decide chan bool
	until  time.Time
}

type joinState struct {
	Kind   string    `json:"kind"`
	Net    string    `json:"net"` // the network the code's maker chose (D10: shown)
	Words  [4]string `json:"words"`
	Status string    `json:"status"` // waiting|done|failed
	Error  string    `json:"error,omitempty"`
	Room   string    `json:"room,omitempty"`
}

type pairReq struct {
	Secret []byte            `json:"secret"`
	Device []byte            `json:"device"`
	Name   string            `json:"name"`
	Cert   *ident.DeviceCert `json:"cert,omitempty"` // invite: the joiner's own device cert
}

type roomInfo struct {
	ID   string           `json:"id"`
	Meta roomlog.RoomMeta `json:"meta"`
}

type pairResp struct {
	Cert  *ident.DeviceCert  `json:"cert,omitempty"` // the host's own device cert
	User  []byte             `json:"user,omitempty"`
	Name  string             `json:"name,omitempty"`
	Certs []ident.DeviceCert `json:"certs,omitempty"`
	Rooms []roomInfo         `json:"rooms,omitempty"`
}

// CodeView is what the UI shows for an open code.
type CodeView struct {
	Code    string   `json:"code"`
	Net     string   `json:"net"`
	Expires int64    `json:"expires"`
	Words   []string `json:"words,omitempty"`
}

func (s *Service) pairingNet() (netsel.Network, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.win == nil || s.win.used || s.now().After(s.win.exp) {
		return "", false
	}
	return s.win.net, true
}

func secret() []byte {
	b := make([]byte, 16)
	rand.Read(b)
	return b
}

func (s *Service) openWindow(kind string, n netsel.Network, room string, ttl time.Duration) (CodeView, error) {
	addrs := s.addrsOn(n)
	if len(addrs) == 0 {
		if n == netsel.Tailscale {
			return CodeView{}, errors.New("this machine has no Tailscale address — is Tailscale running?")
		}
		return CodeView{}, errors.New("this machine has no Local network address")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w := &window{kind: kind, secret: secret(), net: n, exp: s.now().Add(ttl), room: room}
	c := codes.Code{Kind: kind, Peer: s.node.ID().String(), Addrs: addrs, Net: string(n), Secret: w.secret, Expires: w.exp.Unix()}
	if room != "" {
		st := s.stateLocked(s.rooms[room])
		c.Room, c.Name, c.Project = room, st.Name, st.Meta.Project
	}
	s.win = w
	s.updateMDNSLocked()
	return CodeView{Code: codes.Encode(c), Net: string(n), Expires: w.exp.Unix()}, nil
}

// PairCode opens a 10-minute window for one of my machines to join (first machine only).
func (s *Service) PairCode(n netsel.Network) (CodeView, error) {
	s.mu.Lock()
	first := s.setUp && s.me.IsFirst
	s.mu.Unlock()
	if !first {
		return CodeView{}, errors.New("add machines from your first Commander machine")
	}
	return s.openWindow(codes.KindPair, n, "", pairTTL)
}

// InviteCode opens a 24-hour single-use invite to a room (owner only).
func (s *Service) InviteCode(roomID string, n netsel.Network) (CodeView, error) {
	s.mu.Lock()
	r := s.rooms[roomID]
	ok := r != nil && s.setUp
	var owner bool
	if ok {
		owner = string(s.stateLocked(r).Meta.Owner) == string(s.me.User)
	}
	s.mu.Unlock()
	if !ok {
		return CodeView{}, errors.New("no such room")
	}
	if !owner {
		return CodeView{}, errors.New("only the room's owner can invite")
	}
	return s.openWindow(codes.KindInvite, n, roomID, inviteTTL)
}

// Pending is the confirmation the host must give (words to compare).
type Pending struct {
	Kind  string   `json:"kind"`
	Name  string   `json:"name"`
	Words []string `json:"words"`
	Net   string   `json:"net"`            // the network the code was made for (D10: shown)
	Room  string   `json:"room,omitempty"` // invite: the room's name
	Until int64    `json:"until"`          // unix seconds the check stays open
}

func (s *Service) PendingConfirm() *Pending {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.win == nil || s.win.pending == nil {
		return nil
	}
	return s.pendingViewLocked()
}

// Confirm answers the pending pairing/invite on the host.
func (s *Service) Confirm(ok bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.win == nil || s.win.pending == nil {
		return errors.New("nothing to confirm")
	}
	select {
	case s.win.pending.decide <- ok:
	default:
	}
	return nil
}

// CancelCode closes an open window.
func (s *Service) CancelCode() {
	s.mu.Lock()
	s.win = nil
	s.updateMDNSLocked()
	s.mu.Unlock()
	s.changed()
}

func (s *Service) handlePair(from peer.ID, net netsel.Network, raw []byte) ([]byte, error) {
	var req pairReq
	if json.Unmarshal(raw, &req) != nil || ident.PeerIDOf(req.Device) != from.String() {
		return nil, errors.New("bad request")
	}
	s.mu.Lock()
	w := s.win
	if w == nil || w.used || s.now().After(w.exp) || net != w.net || subtle.ConstantTimeCompare(w.secret, req.Secret) != 1 || w.pending != nil {
		s.mu.Unlock()
		return nil, errors.New("refused")
	}
	if w.kind == codes.KindInvite && (req.Cert == nil || ident.VerifyCert(*req.Cert) != nil || string(req.Cert.Device) != string(req.Device)) {
		s.mu.Unlock()
		return nil, errors.New("refused")
	}
	p := &pending{peer: from, name: strings.TrimSpace(req.Name), words: codes.Words(s.node.ID().String(), from.String(), w.secret), decide: make(chan bool, 1)}
	w.pending = p
	p.until = s.now().Add(confirmWait)
	onConfirm := s.OnConfirm
	pv := *s.pendingViewLocked()
	s.mu.Unlock()
	s.changed()
	if onConfirm != nil {
		go onConfirm(pv)
	}

	ok := false
	select {
	case ok = <-p.decide:
	case <-time.After(confirmWait):
	}
	s.mu.Lock()
	defer func() { s.updateMDNSLocked(); s.mu.Unlock(); s.changed() }()
	if s.win != w {
		return nil, errors.New("cancelled")
	}
	s.win = nil
	if !ok {
		return nil, errors.New("declined")
	}
	w.used = true
	var resp pairResp
	switch w.kind {
	case codes.KindPair:
		c, err := s.ids.SignCert(req.Device, p.name)
		if err != nil {
			return nil, err
		}
		if s.me, err = s.ids.AddCert(c); err != nil {
			return nil, err
		}
		s.devices[from.String()] = DevicePref{Net: net, Name: p.name, User: s.me.User}
		resp = pairResp{User: s.me.User, Name: s.me.Name, Certs: s.me.Certs}
		for id, r := range s.rooms {
			resp.Rooms = append(resp.Rooms, roomInfo{ID: id, Meta: s.stateLocked(r).Meta})
		}
	case codes.KindInvite:
		r := s.rooms[w.room]
		if r == nil {
			return nil, errors.New("room gone")
		}
		if _, err := r.w.Write(roomlog.KindMemberAdd, roomlog.Member{User: req.Cert.User, Role: "member"}); err != nil {
			return nil, err
		}
		s.devices[from.String()] = DevicePref{Net: net, Name: p.name, User: req.Cert.User}
		resp = pairResp{Rooms: []roomInfo{{ID: w.room, Meta: s.stateLocked(r).Meta}}}
		if c, ok := s.myCert(); ok {
			resp.Cert = &c
		}
	}
	s.saveDevicesLocked()
	b, _ := json.Marshal(resp)
	return b, nil
}

// JoinStatus is the joiner's progress after entering a code.
func (s *Service) JoinStatus() *joinState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.join == nil {
		return nil
	}
	j := *s.join
	return &j
}

// Join enters a pairing or invite code. It returns the 4 words at once; the
// result arrives later (JoinStatus, OnChange). folder binds an invited room.
func (s *Service) Join(code, name, folder string) (CodeView, error) {
	c, err := codes.Decode(code, s.now())
	if err != nil {
		return CodeView{}, err
	}
	if !netsel.Valid(c.Net) {
		return CodeView{}, errors.New("the code names an unknown network")
	}
	host, err := peer.Decode(c.Peer)
	if err != nil {
		return CodeView{}, errors.New("the code is damaged — copy it again")
	}
	s.mu.Lock()
	switch {
	case c.Kind == codes.KindPair && s.setUp:
		s.mu.Unlock()
		return CodeView{}, errors.New("this machine already has rooms set up")
	case c.Kind == codes.KindInvite && !s.setUp:
		s.mu.Unlock()
		return CodeView{}, errNotSetUp
	case s.node == nil:
		s.mu.Unlock()
		return CodeView{}, errNotSetUp
	case host == s.node.ID():
		s.mu.Unlock()
		return CodeView{}, errors.New("that code is from this machine")
	}
	net := netsel.Network(c.Net)
	// The user picked the network when making the code; only its addresses are used (D10).
	var keep []string
	for _, a := range c.Addrs {
		if m, err := ma.NewMultiaddr(a); err == nil {
			if got, ok := netsel.OfAddr(m); ok && got == net {
				keep = append(keep, a)
			}
		}
	}
	if d, known := s.devices[host.String()]; known && d.Net != "" && d.Net != net {
		s.mu.Unlock()
		// D10: a code never switches a known machine's network silently.
		return CodeView{}, fmt.Errorf("that machine is set to %s here; change it under Machines & network first", d.Net.Label())
	}
	if t, hidden := s.tombs[c.Room]; hidden && c.Kind == codes.KindInvite {
		t.Restoring = true // an explicit invite brings a hidden room back (final review I7)
		s.tombs[c.Room] = t
		s.saveTombsLocked()
	}
	if s.joinStop != nil {
		s.joinStop() // a new code replaces a join still running
	}
	s.joinPeer = &joinPeer{id: host, net: net, addrs: keep}
	s.updateMDNSLocked()
	words := codes.Words(host.String(), s.node.ID().String(), c.Secret)
	js := &joinState{Kind: c.Kind, Net: c.Net, Words: words, Status: "waiting"}
	s.join = js
	req := pairReq{Secret: c.Secret, Device: s.devicePub(), Name: name}
	if c.Kind == codes.KindInvite {
		for _, cert := range s.me.Certs {
			if string(cert.Device) == string(s.devicePub()) {
				cc := cert
				req.Cert = &cc
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.joinStop = cancel
	s.mu.Unlock()
	s.changed()
	go s.runJoin(ctx, js, c, host, req, folder)
	return CodeView{Net: c.Net, Expires: c.Expires, Words: words[:]}, nil
}

// finishJoin settles one join — only if it is still the current one and
// still waiting, so a cancelled or replaced join never overwrites a newer one.
func (s *Service) finishJoin(js *joinState, err error, room string) {
	s.mu.Lock()
	if js == nil || s.join != js || js.Status != "waiting" {
		s.mu.Unlock()
		return
	}
	s.joinPeer = nil
	{
		if err != nil {
			s.join.Status, s.join.Error = "failed", err.Error()
		} else {
			s.join.Status, s.join.Room = "done", room
		}
	}
	s.mu.Unlock()
	s.changed()
}

func (s *Service) runJoin(parent context.Context, js *joinState, c codes.Code, host peer.ID, req pairReq, folder string) {
	s.dial(host)
	body, _ := json.Marshal(req)
	ctx, cancel := context.WithTimeout(parent, confirmWait+30*time.Second)
	defer cancel()
	s.mu.Lock()
	node := s.node
	s.mu.Unlock()
	if node == nil {
		s.finishJoin(js, errors.New("rooms stopped"), "")
		return
	}
	raw, err := node.Request(ctx, host, protoPair, body, 1<<20)
	if parent.Err() != nil {
		return // CancelJoin already reported it
	}
	if err != nil {
		s.finishJoin(js, errors.New("the other machine did not accept — check the words and try a new code"), "")
		return
	}
	var resp pairResp
	if json.Unmarshal(raw, &resp) != nil {
		s.finishJoin(js, errors.New("unexpected answer"), "")
		return
	}
	s.mu.Lock()
	// The host becomes a known device only once it has proved itself (review I5).
	trusted := func(d DevicePref) DevicePref {
		if d.Net == "" && s.joinPeer != nil {
			d.Net, d.Addrs = s.joinPeer.net, s.joinPeer.addrs
		}
		return d
	}
	switch c.Kind {
	case codes.KindPair:
		id, err := s.ids.Adopt(resp.User, resp.Name, resp.Certs)
		if err != nil {
			s.mu.Unlock()
			s.finishJoin(js, err, "")
			return
		}
		s.me, s.setUp, s.joining = id, true, false
		os.Remove(filepath.Join(s.Dir, "joining"))
		d := trusted(s.devices[host.String()])
		d.Name = "first machine"
		for _, cert := range resp.Certs { // Adopt verified every cert against resp.User
			if ident.PeerIDOf(cert.Device) == host.String() {
				d.User, d.Name = resp.User, cert.Name
			}
		}
		s.devices[host.String()] = d
		for _, ri := range resp.Rooms {
			s.adoptRoomLocked(ri, "")
		}
	case codes.KindInvite:
		d := trusted(s.devices[host.String()])
		d.Name = c.Name + " (owner)"
		var adopted bool
		for _, ri := range resp.Rooms {
			if ri.ID == c.Room && hostIsOwner(resp, host.String(), ri) && s.adoptRoomLocked(ri, folder) != nil {
				d.User, adopted = resp.Cert.User, true
			}
		}
		if !adopted {
			s.mu.Unlock()
			s.finishJoin(js, errors.New("the inviting machine could not prove it belongs to the room's owner"), "")
			return
		}
		s.devices[host.String()] = d
	}
	s.saveDevicesLocked()
	s.mu.Unlock()
	s.syncPeer(host)
	s.ensureCerts()
	s.syncPeer(host)
	s.finishJoin(js, nil, c.Room)
}

// adoptRoomLocked creates a local copy of a room this device belongs to.
func (s *Service) adoptRoomLocked(ri roomInfo, folder string) *room {
	if !roomIDRe.MatchString(ri.ID) || roomlog.RoomIDOf(ri.Meta.Owner, ri.Meta.Nonce) != ri.ID {
		return nil
	}
	if t, hidden := s.tombs[ri.ID]; hidden && !t.Restoring {
		return nil // removed from this machine by the user (UX spec §7)
	}
	if _, hidden := s.tombs[ri.ID]; hidden {
		delete(s.tombs, ri.ID) // restore finished
		s.saveTombsLocked()
	}
	r := s.openRoomLocked(ri.ID)
	r.st.Owner = ri.Meta.Owner
	if folder != "" {
		r.st.Folder = folder
	}
	s.saveRoomLocked(r)
	return r
}

// ensureCerts: every room this device is in carries this device's cert in
// its own lane (after it has synced, so the Lamport time follows the add).
func (s *Service) ensureCerts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	me := s.me.PeerID()
	for _, r := range s.rooms {
		if r.lanes.Heads()[me] > 0 {
			continue
		}
		for _, c := range s.me.Certs {
			if ident.PeerIDOf(c.Device) == me {
				r.w.Write(roomlog.KindDeviceCert, c)
			}
		}
		// A room I just joined or adopted carries my standing revocations
		// too (review I1); only the machine with the user key can sign them.
		s.revokeIntoLocked(r)
	}
	s.publishAddrsLocked()
}

// Setup is the first-run choice. first=true makes this the user's first
// machine; first=false prepares to pair with it (node starts, no user key).
func (s *Service) Setup(first bool, name, deviceName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setUp {
		return errors.New("rooms are already set up")
	}
	if first {
		id, err := s.ids.CreateFirst(strings.TrimSpace(name), deviceName)
		if err != nil {
			return err
		}
		s.me, s.setUp = id, true
		os.Remove(filepath.Join(s.Dir, "joining"))
	} else {
		if _, err := s.ids.DeviceKey(); err != nil {
			return err
		}
		os.MkdirAll(s.Dir, 0o700)
		os.WriteFile(filepath.Join(s.Dir, "joining"), nil, 0o600)
		s.joining = true
	}
	err := s.startLocked()
	go s.changed()
	return err
}

// hostIsOwner: the invite host's own cert is valid, names the host device and
// belongs to the room's owner (whose key the room id is bound to).
func hostIsOwner(resp pairResp, host string, ri roomInfo) bool {
	c := resp.Cert
	return c != nil && ident.VerifyCert(*c) == nil && ident.PeerIDOf(c.Device) == host &&
		string(c.User) == string(ri.Meta.Owner) && roomlog.RoomIDOf(ri.Meta.Owner, ri.Meta.Nonce) == ri.ID
}

func (s *Service) pendingViewLocked() *Pending {
	w := s.win
	pv := &Pending{Kind: w.kind, Name: w.pending.name, Words: w.pending.words[:], Net: string(w.net), Until: w.pending.until.Unix()}
	if r := s.rooms[w.room]; r != nil {
		pv.Room = s.stateLocked(r).Name
	}
	return pv
}
