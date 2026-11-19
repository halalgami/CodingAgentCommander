package rooms

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/ident"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/netsel"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/roomlog"
)

type Status struct {
	SetUp       bool       `json:"setUp"`
	Joining     bool       `json:"joining"`
	First       bool       `json:"first"`
	Name        string     `json:"name"`
	Device      string     `json:"device"`
	TailscaleUp bool       `json:"tailscaleUp"`
	Machine     string     `json:"machine"` // this computer's name (filled by the app)
	Pending     *Pending   `json:"pending,omitempty"`
	Join        *joinState `json:"join,omitempty"`
}

func (s *Service) Status() Status {
	ts := s.tailscaleUp()
	s.mu.Lock()
	st := Status{SetUp: s.setUp, Joining: s.joining, First: s.me.IsFirst, Name: s.me.Name, TailscaleUp: ts}
	if s.node != nil {
		st.Device = s.node.ID().String()
	}
	s.mu.Unlock()
	st.Pending = s.PendingConfirm()
	st.Join = s.JoinStatus()
	return st
}

type RoomSummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Project string `json:"project"`
	Folder  string `json:"folder"`
	Owner   bool   `json:"owner"`
	Unread  int    `json:"unread"`
	Queued  int    `json:"queued"`
	// UX 1.5a: read-only state, folder binding, who is reachable now.
	Closed      bool   `json:"closed"`
	ClosedAt    string `json:"closedAt,omitempty"`
	ClosedBy    string `json:"closedBy,omitempty"`
	NeedsFolder bool   `json:"needsFolder"`
	Online      int    `json:"online"`
	Last        string `json:"last"` // newest message time, else the room's creation (1.5b)
}

var msgKinds = map[string]bool{"note": true, "question": true, "handoff": true, "result": true, "ack": true}

// CreateRoom starts a room for a project folder (spec §5.3).
func (s *Service) CreateRoom(folder, name, project string) (RoomSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.setUp {
		return RoomSummary{}, errNotSetUp
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return RoomSummary{}, errors.New("give the room a name")
	}
	cert, ok := s.myCert()
	if !ok {
		return RoomSummary{}, errors.New("this machine has no certificate")
	}
	meta, id := roomlog.NewRoom(s.me.User, name, project)
	r := s.openRoomLocked(id)
	if _, err := r.w.Write(roomlog.KindDeviceCert, cert); err != nil {
		return RoomSummary{}, err
	}
	if _, err := r.w.Write(roomlog.KindRoomMeta, meta); err != nil {
		return RoomSummary{}, err
	}
	r.st.Folder, r.st.Owner = folder, s.me.User
	s.saveRoomLocked(r)
	s.publishAddrsLocked()
	go s.SyncAll()
	go s.changed()
	return s.summaryLocked(r), nil
}

func (s *Service) BindFolder(roomID, folder string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rooms[roomID]
	if r == nil {
		return errors.New("no such room")
	}
	r.st.Folder = folder
	s.saveRoomLocked(r)
	go s.changed()
	return nil
}

func (s *Service) summaryLocked(r *room) RoomSummary {
	st := s.stateLocked(r)
	sum := RoomSummary{ID: r.id, Name: st.Name, Project: st.Meta.Project, Folder: r.st.Folder, Owner: string(st.Meta.Owner) == string(s.me.User)}
	me := s.me.PeerID()
	devs := s.memberDevicesLocked(st)
	sum.NeedsFolder = r.st.Folder == ""
	if st.Closed != nil {
		sum.Closed, sum.ClosedAt, sum.ClosedBy = true, st.Closed.Time, st.Names[st.Closed.Device]
	} else if r.st.Left || st.Removed[string(s.me.User)] {
		sum.Closed, sum.ClosedBy = true, "you left" // or were removed, possibly from another of my machines
	}
	for _, d := range devs {
		if id, err := peer.Decode(d); err == nil && d != me && s.node != nil {
			if _, ok := s.node.Conn(id); ok {
				sum.Online++
			}
		}
	}
	for _, e := range st.Accepted {
		if e.Kind == roomlog.KindRoomMeta && sum.Last == "" {
			sum.Last = e.Time
		}
		if e.Kind != roomlog.KindMsg {
			continue
		}
		if e.Time > sum.Last {
			sum.Last = e.Time
		}
		if e.Device != me {
			if !r.st.Seen[e.Ref()] {
				sum.Unread++
			}
			continue
		}
		if queuedOnly(roomlog.DeliveryOf(e, devs, r.ix, s.observedRoomLocked(r.id))) {
			sum.Queued++
		}
	}
	return sum
}

func queuedOnly(d map[string]roomlog.DeliveryState) bool {
	if len(d) == 0 {
		return false
	}
	for _, v := range d {
		if v != roomlog.Queued {
			return false
		}
	}
	return true
}

func (s *Service) observedRoomLocked(room string) map[string]map[string]int64 {
	out := map[string]map[string]int64{}
	for dev, rooms := range s.observed {
		if h := rooms[room]; h != nil {
			out[dev] = h
		}
	}
	return out
}

func (s *Service) memberDevicesLocked(st roomlog.State) []string {
	var out []string
	for d := range st.Devices {
		if st.MemberDevice(d) {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Service) Rooms() []RoomSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []RoomSummary{}
	for _, r := range s.rooms {
		out = append(out, s.summaryLocked(r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// MsgView is one message in a thread.
type MsgView struct {
	Ref      string            `json:"ref"`
	Parent   string            `json:"parent,omitempty"`
	Author   string            `json:"author"`
	Device   string            `json:"device"`
	Mine     bool              `json:"mine"`
	Kind     string            `json:"kind"`
	Text     string            `json:"text"`
	Fields   map[string]string `json:"fields,omitempty"`
	Time     string            `json:"time"`
	Seen     bool              `json:"seen"`
	Delivery map[string]string `json:"delivery,omitempty"` // device → state (my messages)
}

type Thread struct {
	Root    MsgView   `json:"root"`
	Replies []MsgView `json:"replies"`
	Last    string    `json:"last"`
}

// PeerView is one member device as this machine sees it (spec §9, D10).
type PeerView struct {
	Device   string `json:"device"`
	Name     string `json:"name"`
	Mine     bool   `json:"mine"` // one of my own machines
	Net      string `json:"net"`  // the network the user chose; "" = not chosen
	State    string `json:"state"`
	Reason   string `json:"reason,omitempty"`
	LastSeen int64  `json:"lastSeen,omitempty"`
	Head     int64  `json:"head"`
	Waiting  int    `json:"waiting"`         // my messages not yet on that device
	Rejected int    `json:"rejected"`        // entries from it this machine refused (review I7)
	Tried    string `json:"tried,omitempty"` // the other network it last reached us over (refused, D10)
}

type OutItem struct {
	Ref    string `json:"ref"`
	Text   string `json:"text"`
	Device string `json:"device"`
	Name   string `json:"name"`
	State  string `json:"state"`
}

type RoomView struct {
	RoomSummary
	Threads []Thread   `json:"threads"`
	Peers   []PeerView `json:"peers"`
	Outbox  []OutItem  `json:"outbox"`
}

func (s *Service) View(roomID string) (RoomView, error) {
	ts := s.tailscaleUp()
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rooms[roomID]
	if r == nil {
		return RoomView{}, errors.New("no such room")
	}
	st := s.stateLocked(r)
	me := s.me.PeerID()
	devs := s.memberDevicesLocked(st)
	obs := s.observedRoomLocked(r.id)
	heads := r.lanes.Heads()
	v := RoomView{RoomSummary: s.summaryLocked(r), Threads: []Thread{}, Peers: []PeerView{}, Outbox: []OutItem{}}
	name := func(dev string) string {
		if n := st.Names[dev]; n != "" {
			return n
		}
		if len(dev) > 8 {
			return dev[len(dev)-8:]
		}
		return dev
	}
	byRef := map[string]int{}
	waiting := map[string]int{}
	for _, e := range st.Accepted {
		if e.Kind != roomlog.KindMsg {
			continue
		}
		var m roomlog.Msg
		if json.Unmarshal(e.Body, &m) != nil {
			continue
		}
		mv := MsgView{Ref: e.Ref(), Parent: m.Parent, Author: name(e.Device), Device: e.Device, Mine: e.Device == me, Kind: m.Kind, Text: m.Text, Fields: m.Fields, Time: e.Time, Seen: e.Device == me || r.st.Seen[e.Ref()]}
		if mv.Mine {
			mv.Delivery = map[string]string{}
			for d, ds := range roomlog.DeliveryOf(e, devs, r.ix, obs) {
				mv.Delivery[d] = string(ds)
				if ds == roomlog.Queued || ds == roomlog.Relayed {
					waiting[d]++
					v.Outbox = append(v.Outbox, OutItem{Ref: mv.Ref, Text: clip(m.Text, 80), Device: d, Name: name(d), State: string(ds)})
				}
			}
		}
		if i, ok := byRef[m.Parent]; ok && m.Parent != "" {
			v.Threads[i].Replies = append(v.Threads[i].Replies, mv)
			v.Threads[i].Last = e.Time
			continue
		}
		byRef[mv.Ref] = len(v.Threads)
		v.Threads = append(v.Threads, Thread{Root: mv, Replies: []MsgView{}, Last: e.Time})
	}
	sort.SliceStable(v.Threads, func(i, j int) bool { return v.Threads[i].Last > v.Threads[j].Last })
	for _, d := range devs {
		if d == me {
			continue
		}
		pref := s.devices[d]
		u, _ := st.UserOf(d)
		pv := PeerView{Device: d, Name: name(d), Mine: string(u) == string(s.me.User), Net: string(pref.Net), LastSeen: pref.LastSeen, Head: heads[d], Waiting: waiting[d]}
		pv.State, pv.Reason = s.peerStateLocked(d, pref, ts)
		pv.Rejected = s.rejected[d]
		if id, err := peer.Decode(d); err == nil && s.node != nil && pref.Net != "" {
			if got, refused := s.node.Refused(id); refused && got != pref.Net {
				pv.Tried = string(got)
			}
		}
		v.Peers = append(v.Peers, pv)
	}
	return v, nil
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (s *Service) peerStateLocked(dev string, pref DevicePref, tailscaleUp bool) (string, string) {
	if pref.Net == "" {
		return "unreachable", "choose a network for this machine"
	}
	if id, err := peer.Decode(dev); err == nil && s.node != nil {
		if n, ok := s.node.Conn(id); ok {
			if n == netsel.Tailscale {
				return "direct (Tailscale)", ""
			}
			return "direct (Local network)", ""
		}
	}
	if id, err := peer.Decode(dev); err == nil && s.node != nil {
		if got, refused := s.node.Refused(id); refused && got != pref.Net {
			// D10: shown, and a switch suggested — never made.
			return "unreachable", fmt.Sprintf("it reached this machine over %s, but you chose %s for it — switch its network if that's right", got.Label(), pref.Net.Label())
		}
	}
	if pref.Net == netsel.Tailscale && !tailscaleUp {
		return "unreachable", "Tailscale is off on this machine"
	}
	if pref.LastSeen > 0 {
		return "offline", "not seen on " + pref.Net.Label() + " since " + time.UnixMilli(pref.LastSeen).Format("Jan 2 15:04")
	}
	return "offline", "not seen yet on " + pref.Net.Label()
}

// Post writes a human message into a room (spec §8.2).
func (s *Service) Post(roomID, kind, text, parent string, fields map[string]string) error {
	if !msgKinds[kind] {
		return errors.New("unknown message kind")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("write something first")
	}
	if len(text) > roomlog.MaxBody {
		return errors.New("messages are limited to 16 KB")
	}
	s.mu.Lock()
	r := s.rooms[roomID]
	if r == nil {
		s.mu.Unlock()
		return errors.New("no such room")
	}
	if s.closedLocked(r) {
		s.mu.Unlock()
		return errClosed
	}
	_, err := r.w.Write(roomlog.KindMsg, roomlog.Msg{Kind: kind, Text: text, Parent: parent, Fields: fields})
	s.mu.Unlock()
	if err != nil {
		return err
	}
	go s.SyncAll()
	s.changed()
	return nil
}

// MarkSeen records that a human here opened a message (a "seen" receipt).
func (s *Service) MarkSeen(roomID, ref string) error {
	s.mu.Lock()
	r := s.rooms[roomID]
	if r == nil || r.st.Seen[ref] {
		s.mu.Unlock()
		return nil
	}
	dev, seqStr, ok := strings.Cut(ref, ":")
	var seq int64
	if !ok || json.Unmarshal([]byte(seqStr), &seq) != nil {
		s.mu.Unlock()
		return errors.New("bad ref")
	}
	found := false
	for _, e := range s.stateLocked(r).Accepted {
		if e.Kind == roomlog.KindMsg && e.Ref() == ref {
			found = true
			break
		}
	}
	if !found {
		s.mu.Unlock()
		return errors.New("no such message in this room") // final review I2
	}
	r.st.Seen[ref] = true
	s.saveRoomLocked(r)
	_, err := r.w.Write(roomlog.KindReceipt, roomlog.Receipt{Lane: dev, Seq: seq, State: "seen"})
	s.mu.Unlock()
	go s.SyncAll()
	s.changed()
	return err
}

// QueuedCount: my messages that no other device holds yet (quit guard).
func (s *Service) QueuedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	me := s.me.PeerID()
	for _, r := range s.rooms {
		n += s.summaryLocked(r).Queued
		// A close of mine that no other machine holds yet counts too (1.5a review minor).
		if st := s.stateLocked(r); st.Closed != nil && st.Closed.Device == me {
			c := roomlog.Entry{Device: me, Seq: st.Closed.Seq}
			if queuedOnly(roomlog.DeliveryOf(c, s.memberDevicesLocked(st), r.ix, s.observedRoomLocked(r.id))) {
				n++
			}
		}
	}
	return n
}

// SetDeviceNetwork changes the network the user chose for a device (D10).
// Connections on the other network are closed at once.
func (s *Service) SetDeviceNetwork(dev, net string) error {
	if !netsel.Valid(net) {
		return errors.New("choose Local network or Tailscale")
	}
	id, err := peer.Decode(dev)
	if err != nil {
		return errors.New("bad device")
	}
	s.mu.Lock()
	d := s.devices[dev]
	if d.Net != netsel.Network(net) {
		d.Addrs = nil // addresses learnt on the other network don't apply
	}
	d.Net = netsel.Network(net)
	s.devices[dev] = d
	s.saveDevicesLocked()
	s.updateMDNSLocked()
	node := s.node
	s.mu.Unlock()
	if node != nil {
		node.Host().Network().ClosePeer(id)
	}
	go s.dial(id)
	s.changed()
	return nil
}

// certUser is a small helper for tests and bindings.
func certUser(c ident.DeviceCert) []byte { return c.User }
