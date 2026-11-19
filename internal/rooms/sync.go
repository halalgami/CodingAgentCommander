package rooms

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/ident"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/netsel"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/roomlog"
)

// Sync (spec §6.4) as rounds of two requests per room: I send my heads and
// get what I lack plus the other side's heads; then I send what it lacks.

const (
	protoSync = "/commander/rooms/1"
	maxBatch  = 4 << 20
)

type syncReq struct {
	Room    string            `json:"room"`
	Cert    ident.DeviceCert  `json:"cert"`
	Meta    *roomlog.RoomMeta `json:"meta,omitempty"` // to my own devices: lets them adopt the room
	Heads   map[string]int64  `json:"heads"`
	Entries []roomlog.Entry   `json:"entries,omitempty"`
}

type syncResp struct {
	Heads    map[string]int64 `json:"heads"`
	Entries  []roomlog.Entry  `json:"entries,omitempty"`
	Rejected int              `json:"rejected,omitempty"`
}

func (s *Service) myCert() (ident.DeviceCert, bool) {
	me := s.me.PeerID()
	for _, c := range s.me.Certs {
		if ident.PeerIDOf(c.Device) == me {
			return c, true
		}
	}
	return ident.DeviceCert{}, false
}

// lacking: entries I hold that heads doesn't, oldest first per lane, ≤ maxBatch.
func lacking(l *roomlog.Lanes, heads map[string]int64) []roomlog.Entry {
	var out []roomlog.Entry
	budget := maxBatch
	for dev, mine := range l.Heads() {
		if heads[dev] >= mine || budget <= 0 {
			continue
		}
		es := l.Since(dev, heads[dev], budget)
		for _, e := range es {
			b, _ := json.Marshal(e)
			budget -= len(b)
		}
		out = append(out, es...)
	}
	return out
}

// apply appends entries in Lamport order (a member.add lands before the new
// member's first lane entry); entries refused for admission get one retry
// after the room state is refreshed. Returns how many were new and rejected.
func (s *Service) apply(r *room, es []roomlog.Entry) (added, rejected int) {
	if r.gone {
		return 0, 0
	}
	es = append([]roomlog.Entry(nil), es...)
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].Lamport != es[j].Lamport {
			return es[i].Lamport < es[j].Lamport
		}
		return es[i].Device < es[j].Device
	})
	var retry []roomlog.Entry
	for pass := 0; pass < 2; pass++ {
		retry = retry[:0]
		for _, e := range es {
			switch err := r.lanes.Append(e); {
			case err == nil:
				added++
				if e.Kind == roomlog.KindMemberAdd || e.Kind == roomlog.KindRoomMeta {
					s.stateLocked(r)
				}
			case errors.Is(err, roomlog.ErrDuplicate):
			case errors.Is(err, roomlog.ErrGap) || pass == 0:
				retry = append(retry, e)
			default:
				rejected++
			}
		}
		if len(retry) == 0 {
			break
		}
		s.stateLocked(r)
		es = append([]roomlog.Entry(nil), retry...)
	}
	rejected += len(retry)
	s.stateLocked(r)
	return
}

// mayShareLocked: may device dev (of user) see room r? Never a revoked
// device; my own machines and current members otherwise. Checked on BOTH
// sides of a sync — a peer that claims to hold nothing gets nothing it may
// not have.
func (s *Service) mayShareLocked(r *room, dev string, user []byte) bool {
	st := s.stateLocked(r)
	if st.Revoked[dev] || len(user) == 0 {
		return false
	}
	if u, certified := st.Devices[dev]; certified && string(u) != string(user) {
		return false // the log certifies this device to someone else
	}
	if string(user) == string(s.me.User) || string(user) == string(r.st.Owner) {
		return true
	}
	_, ok := st.Members[string(user)]
	return ok
}

func (s *Service) handleSync(from peer.ID, net netsel.Network, raw []byte) ([]byte, error) {
	if n, ok := s.policy(from); !ok || n != net {
		return nil, errors.New("refused") // the gater already enforces this; belt and braces
	}
	var req syncReq
	if json.Unmarshal(raw, &req) != nil || !roomIDRe.MatchString(req.Room) {
		return nil, errors.New("bad request")
	}
	if ident.VerifyCert(req.Cert) != nil || ident.PeerIDOf(req.Cert.Device) != from.String() {
		return nil, errors.New("refused")
	}
	s.mu.Lock()
	if !s.setUp {
		s.mu.Unlock()
		return nil, errors.New("refused")
	}
	r := s.rooms[req.Room]
	own := string(req.Cert.User) == string(s.me.User)
	if r == nil && own && req.Meta != nil {
		r = s.adoptRoomLocked(roomInfo{ID: req.Room, Meta: *req.Meta}, "")
	}
	if r == nil || !s.mayShareLocked(r, from.String(), req.Cert.User) {
		s.mu.Unlock()
		return nil, errors.New("refused")
	}
	added, rejected := s.apply(r, req.Entries)
	s.rejected[from.String()] += rejected
	resp := syncResp{Heads: r.lanes.Heads(), Rejected: rejected}
	resp.Entries = lacking(r.lanes, req.Heads)
	s.observeLocked(from.String(), req.Room, req.Heads)
	if added > 0 {
		s.receiptsLocked(r, req.Entries)
	}
	needCert := r.lanes.Heads()[s.me.PeerID()] == 0
	s.mu.Unlock()
	if needCert {
		s.ensureCerts()
	}
	if added > 0 {
		s.changed()
		go s.gossip(from)
	}
	b, _ := json.Marshal(resp)
	return b, nil
}

func (s *Service) observeLocked(dev, room string, heads map[string]int64) {
	if s.observed[dev] == nil {
		s.observed[dev] = map[string]map[string]int64{}
	}
	h := map[string]int64{}
	for k, v := range heads {
		h[k] = v
	}
	s.observed[dev][room] = h
}

// receiptsLocked: one cumulative "delivered" receipt per foreign lane that
// brought new messages (spec §8.4).
func (s *Service) receiptsLocked(r *room, es []roomlog.Entry) {
	me := s.me.PeerID()
	lanes := map[string]bool{}
	for _, e := range es {
		if e.Kind == roomlog.KindMsg && e.Device != me {
			lanes[e.Device] = true
		}
	}
	heads := r.lanes.Heads()
	for l := range lanes {
		r.w.Write(roomlog.KindReceipt, roomlog.Receipt{Lane: l, Seq: heads[l], State: "delivered"})
	}
}

// gossip: new entries arrived from one peer; pass them on to the others.
func (s *Service) gossip(except peer.ID) {
	s.mu.Lock()
	node := s.node
	s.mu.Unlock()
	if node == nil {
		return
	}
	for _, id := range node.Host().Network().Peers() {
		if id != except {
			go s.syncPeer(id)
		}
	}
}

// SyncAll runs a sync round with every connected peer.
func (s *Service) SyncAll() { s.gossip("") }

// syncPeer runs rounds for every shared room with one peer. Calls for the
// same peer coalesce: one runs, one more is queued.
func (s *Service) syncPeer(id peer.ID) {
	s.mu.Lock()
	if s.running[id.String()] {
		s.again[id.String()] = true
		s.mu.Unlock()
		return
	}
	s.running[id.String()] = true
	s.mu.Unlock()
	for {
		s.syncOnce(id)
		s.mu.Lock()
		if !s.again[id.String()] {
			delete(s.running, id.String())
			s.mu.Unlock()
			return
		}
		delete(s.again, id.String())
		s.mu.Unlock()
	}
}

func (s *Service) syncOnce(id peer.ID) {
	s.mu.Lock()
	node := s.node
	cert, ok := s.myCert()
	if node == nil || !s.setUp || !ok {
		s.mu.Unlock()
		return
	}
	if _, known := s.devices[id.String()]; !known {
		s.mu.Unlock()
		return
	}
	d := s.devices[id.String()]
	own := string(d.User) == string(s.me.User)
	var rooms []*room
	for _, r := range s.rooms {
		user := d.User
		if len(user) == 0 {
			user, _ = s.stateLocked(r).UserOf(id.String())
		}
		if s.mayShareLocked(r, id.String(), user) && !s.closeSettledLocked(r) {
			rooms = append(rooms, r)
		}
	}
	s.mu.Unlock()
	s.touch(id)
	moved := false
	for _, r := range rooms {
		for round := 0; round < 8; round++ {
			n, err := s.roundWith(node, id, r, cert, own)
			if err != nil || n == 0 {
				break
			}
			moved = true
		}
	}
	if moved {
		s.changed()
	}
}

// roundWith returns how many entries moved either way.
func (s *Service) roundWith(node interface {
	Request(context.Context, peer.ID, string, []byte, int) ([]byte, error)
}, id peer.ID, r *room, cert ident.DeviceCert, own bool) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req := syncReq{Room: r.id, Cert: cert, Heads: r.lanes.Heads()}
	if own {
		s.mu.Lock()
		m := s.stateLocked(r).Meta
		s.mu.Unlock()
		req.Meta = &m
	}
	body, _ := json.Marshal(req)
	raw, err := node.Request(ctx, id, protoSync, body, maxBatch+(1<<20))
	if err != nil {
		return 0, err
	}
	var resp syncResp
	if json.Unmarshal(raw, &resp) != nil {
		return 0, errors.New("bad response")
	}
	s.mu.Lock()
	added, rej := s.apply(r, resp.Entries)
	s.rejected[id.String()] += rej
	s.observeLocked(id.String(), r.id, resp.Heads)
	if added > 0 {
		s.receiptsLocked(r, resp.Entries)
	}
	send := lacking(r.lanes, resp.Heads)
	s.mu.Unlock()
	if len(send) > 0 {
		req2 := syncReq{Room: r.id, Cert: cert, Heads: r.lanes.Heads(), Entries: send}
		body, _ = json.Marshal(req2)
		if raw, err = node.Request(ctx, id, protoSync, body, maxBatch+(1<<20)); err == nil {
			var resp2 syncResp
			if json.Unmarshal(raw, &resp2) == nil {
				s.mu.Lock()
				s.observeLocked(id.String(), r.id, resp2.Heads)
				s.mu.Unlock()
			}
		}
	}
	return added + len(send), nil
}
