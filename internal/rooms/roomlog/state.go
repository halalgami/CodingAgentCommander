package roomlog

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/ident"
)

// State is the room as replayed from its entries (spec §5.5, §6).
type State struct {
	Meta     RoomMeta
	Members  map[string]string // user key (raw bytes as string) → role
	Devices  map[string][]byte // device peer id → user key
	Names    map[string]string // device peer id → device name from its cert
	Revoked  map[string]bool   // device peer id
	Accepted []Entry           // entries that passed membership, in order
}

// UserOf returns the user key of a device, if certified.
func (s State) UserOf(dev string) ([]byte, bool) { u, ok := s.Devices[dev]; return u, ok }

// MemberDevice: a certified, unrevoked device of a current member.
func (s State) MemberDevice(dev string) bool {
	u, ok := s.Devices[dev]
	if !ok || s.Revoked[dev] {
		return false
	}
	_, m := s.Members[string(u)]
	return m
}

// Replay applies entries in order. Authorization happens here, not at
// Append, so the order lanes sync in never matters. Two things are fixed
// before ordering, because Lamport times are chosen by the writer and can be
// backdated: the owner (bound to the room id) and the cut-offs recorded by
// removals and revocations (seq-based, not time-based).
func Replay(entries []Entry) State {
	s := State{Members: map[string]string{}, Devices: map[string][]byte{}, Names: map[string]string{}, Revoked: map[string]bool{}}

	// Pass 1: certificates, the genuine genesis, removal and revoke cut-offs.
	for _, e := range entries {
		if e.Kind != KindDeviceCert {
			continue
		}
		var c ident.DeviceCert
		if json.Unmarshal(e.Body, &c) != nil || ident.VerifyCert(c) != nil || ident.PeerIDOf(c.Device) != e.Device {
			continue
		}
		if _, done := s.Devices[e.Device]; !done {
			s.Devices[e.Device] = c.User
			s.Names[e.Device] = c.Name
		}
	}
	cut := map[string]int64{} // device → last seq accepted from it
	past := func(e Entry) bool { c, ok := cut[e.Device]; return ok && e.Seq > c }
	// Revocations first: they bound everything else a device wrote, including
	// removals and the genesis (review I2).
	for _, e := range entries {
		u, ok := s.Devices[e.Device]
		if !ok || e.Kind != KindDeviceRevoke {
			continue
		}
		var r Revoke
		if json.Unmarshal(e.Body, &r) != nil {
			continue
		}
		pid := ident.PeerIDOf(r.Device)
		du, ok := s.Devices[pid]
		if !ok || string(du) != string(u) || !ed25519.Verify(ed25519.PublicKey(u), revokeBytes(r.Device, r.Head), r.Sig) {
			continue // only a user's own key can retire that user's own machine
		}
		s.Revoked[pid] = true
		if old, seen := cut[pid]; !seen || r.Head < old {
			cut[pid] = r.Head
		}
	}
	var genesis *Entry
	for i, e := range entries {
		var m RoomMeta
		if e.Kind != KindRoomMeta || past(e) || json.Unmarshal(e.Body, &m) != nil || RoomIDOf(m.Owner, m.Nonce) != e.Room {
			continue
		}
		if u, ok := s.Devices[e.Device]; ok && string(u) == string(m.Owner) {
			s.Meta, genesis = m, &entries[i]
			break
		}
	}
	if genesis == nil {
		return s
	}
	owner := string(s.Meta.Owner)
	removed := map[string]bool{} // users removed (final in v1)
	for _, e := range entries {
		u, ok := s.Devices[e.Device]
		if !ok || past(e) {
			continue
		}
		switch e.Kind {
		case KindMemberRemove:
			var m Member
			if json.Unmarshal(e.Body, &m) != nil || string(m.User) == owner || (string(u) != owner && string(u) != string(m.User)) {
				continue
			}
			removed[string(m.User)] = true
			for d, du := range s.Devices {
				if string(du) == string(m.User) {
					h := m.Heads[d]
					if old, seen := cut[d]; !seen || h < old {
						cut[d] = h
					}
				}
			}
		}
	}

	// Pass 2: membership in order, entries past a cut-off never count.
	s.Members[owner] = "owner"
	for _, e := range entries {
		if e.Kind == KindDeviceCert {
			continue
		}
		if e.Kind == KindRoomMeta {
			if e.Device == genesis.Device && e.Seq == genesis.Seq {
				s.Accepted = append(s.Accepted, e)
			}
			continue
		}
		if c, ok := cut[e.Device]; ok && e.Seq > c {
			continue
		}
		author, ok := s.Devices[e.Device]
		if !ok {
			continue
		}
		if _, member := s.Members[string(author)]; !member {
			continue
		}
		switch e.Kind {
		case KindMemberAdd:
			var m Member
			if string(author) != owner || json.Unmarshal(e.Body, &m) != nil || len(m.User) != ed25519.PublicKeySize {
				continue
			}
			if _, already := s.Members[string(m.User)]; !already {
				s.Members[string(m.User)] = "member"
			}
		case KindMemberRemove:
			var m Member
			if json.Unmarshal(e.Body, &m) != nil || string(m.User) == owner || (string(author) != owner && string(m.User) != string(author)) {
				continue
			}
			delete(s.Members, string(m.User))
		}
		s.Accepted = append(s.Accepted, e)
	}
	// Removed users stay out even if their add sorted after the removal.
	for u := range removed {
		delete(s.Members, u)
	}
	return s
}

// Delivery states (spec §9; picked-up comes in phase 3).
type DeliveryState string

const (
	Queued    DeliveryState = "queued"
	Relayed   DeliveryState = "relayed"
	Delivered DeliveryState = "delivered"
	Seen      DeliveryState = "seen"
)

// Delivery derives a message's state for each device other than its author.
// observed[device][lane] = the lane head that device was last seen holding.
func Delivery(m Entry, devices []string, entries []Entry, observed map[string]map[string]int64) map[string]DeliveryState {
	held := map[string]bool{}
	seen := map[string]bool{}
	for _, e := range entries {
		if e.Kind != KindReceipt || e.Device == m.Device {
			continue
		}
		var r Receipt
		if json.Unmarshal(e.Body, &r) != nil || r.Lane != m.Device {
			continue
		}
		if r.State == "seen" && r.Seq == m.Seq {
			seen[e.Device], held[e.Device] = true, true
		}
		if r.State == "delivered" && r.Seq >= m.Seq {
			held[e.Device] = true
		}
	}
	relayed := len(held) > 0
	for d, heads := range observed {
		if d != m.Device && heads[m.Device] >= m.Seq {
			relayed = true
		}
	}
	out := map[string]DeliveryState{}
	for _, d := range devices {
		if d == m.Device {
			continue
		}
		switch {
		case seen[d]:
			out[d] = Seen
		case held[d]:
			out[d] = Delivered
		case relayed:
			out[d] = Relayed
		default:
			out[d] = Queued
		}
	}
	return out
}

// Receipts indexes receipts once so delivery states don't rescan the log for
// every message (review I8). Build it from accepted entries only, so junk
// lanes can't fake a delivery (review minor 1).
type Receipts struct {
	held map[string]map[string]int64 // lane → device → highest delivered seq
	seen map[string]map[string]bool  // ref → device
}

func IndexReceipts(accepted []Entry) Receipts {
	ix := Receipts{held: map[string]map[string]int64{}, seen: map[string]map[string]bool{}}
	for _, e := range accepted {
		if e.Kind != KindReceipt {
			continue
		}
		var r Receipt
		if json.Unmarshal(e.Body, &r) != nil {
			continue
		}
		if ix.held[r.Lane] == nil {
			ix.held[r.Lane] = map[string]int64{}
		}
		if r.State == "seen" {
			ref := fmt.Sprintf("%s:%d", r.Lane, r.Seq)
			if ix.seen[ref] == nil {
				ix.seen[ref] = map[string]bool{}
			}
			ix.seen[ref][e.Device] = true
			ix.held[r.Lane][e.Device] = max(ix.held[r.Lane][e.Device], 0)
			continue
		}
		if r.State == "delivered" && r.Seq > ix.held[r.Lane][e.Device] {
			ix.held[r.Lane][e.Device] = r.Seq
		}
	}
	return ix
}

// DeliveryOf is Delivery over a prebuilt index.
func DeliveryOf(m Entry, devices []string, ix Receipts, observed map[string]map[string]int64) map[string]DeliveryState {
	seen := ix.seen[m.Ref()]
	held := map[string]bool{}
	for d, seq := range ix.held[m.Device] {
		if d != m.Device && (seq >= m.Seq || seen[d]) {
			held[d] = true
		}
	}
	for d := range seen {
		held[d] = true
	}
	relayed := len(held) > 0
	for d, heads := range observed {
		if d != m.Device && heads[m.Device] >= m.Seq {
			relayed = true
		}
	}
	out := map[string]DeliveryState{}
	for _, d := range devices {
		if d == m.Device {
			continue
		}
		switch {
		case seen[d]:
			out[d] = Seen
		case held[d]:
			out[d] = Delivered
		case relayed:
			out[d] = Relayed
		default:
			out[d] = Queued
		}
	}
	return out
}
