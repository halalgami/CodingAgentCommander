package roomlog

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/ident"
)

// State is the room as replayed from its entries (spec §5.5, §6).
type State struct {
	Meta     RoomMeta
	Name     string            // Meta.Name, or the owner's latest rename
	Members  map[string]string // user key (raw bytes as string) → role
	Devices  map[string][]byte // device peer id → user key
	Names    map[string]string // device peer id → device name from its cert
	Revoked  map[string]bool   // device peer id
	Accepted []Entry           // entries that passed membership, in order
	Closed   *ClosedInfo       // set by the owner's room.close
	Removed  map[string]bool   // users removed or who left (final in v1)
	// Live: running sessions by "device|session", merged from status entries
	// (session.ended removes one). Phase 2, spec §8.1.
	Live map[string]Status
	// Claims: thread ref → active claims, in log order (spec §8.3). More than
	// one device = a conflict a human resolves.
	Claims map[string][]ClaimInfo
	// PreRevoked: device → the user who retired it before it had a
	// certificate in this room (device-level revocation, rooms made later).
	PreRevoked map[string]map[string]bool // device → signing users
}

// ClaimInfo is one active claim on a thread.
type ClaimInfo struct {
	Device  string `json:"device"`
	Session string `json:"session,omitempty"`
	Lamport int64  `json:"lamport"`
	Time    string `json:"time"`
}

// mergeStatus folds a session event into the live record.
func mergeStatus(old, n Status) Status {
	if old.Session == "" {
		old.Session = n.Session
	}
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&old.Label, n.Label)
	set(&old.Model, n.Model)
	set(&old.Machine, n.Machine)
	set(&old.Branch, n.Branch)
	set(&old.Sha, n.Sha)
	set(&old.State, n.State)
	set(&old.Since, n.Since)
	set(&old.Title, n.Title)
	switch n.Event {
	case EvStarted, EvCommit:
		old.Dirty = n.Dirty
		if n.Event == EvCommit {
			old.Pushed, old.Subject = n.Pushed, n.Subject
		}
	case EvPush:
		old.Pushed = true
	case EvFinished:
		old.Summary, old.Turns, old.DurationS, old.Pushed = n.Summary, n.Turns, n.DurationS, n.Pushed
		if old.State == "" || old.State == "working" {
			old.State = "idle"
		}
	case EvState:
		old.Listening = n.Listening
	}
	old.Event = n.Event
	return old
}

// ClosedInfo records who closed the room and when (UX spec §7).
type ClosedInfo struct {
	Lamport int64  `json:"lamport"`
	Device  string `json:"device"`
	Seq     int64  `json:"seq"`
	// Heads: the lanes the closer held; nil for a close written before 1.5b.
	Heads map[string]int64 `json:"heads,omitempty"`
	Time  string           `json:"time"`
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
	s := State{Members: map[string]string{}, Devices: map[string][]byte{}, Names: map[string]string{}, Revoked: map[string]bool{},
		Live: map[string]Status{}, Claims: map[string][]ClaimInfo{}, PreRevoked: map[string]map[string]bool{}}

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
		// Only a user's own key can retire that user's own machine. A device
		// with no certificate in this room yet can be retired ahead of time
		// (device-level revocation in rooms made after it): if it ever shows
		// up certified to this user, it is already out.
		if (ok && string(du) != string(u)) || !ed25519.Verify(ed25519.PublicKey(u), revokeBytes(r.Device, r.Head), r.Sig) {
			continue
		}
		if !ok {
			// Not certified here (yet): pending, bound to the signer. It only
			// takes effect once the device turns up certified to that same
			// user, so nobody can pre-revoke another user's machine.
			if s.PreRevoked[pid] == nil {
				s.PreRevoked[pid] = map[string]bool{}
			}
			s.PreRevoked[pid][string(u)] = true // every signer (security review: no squatting)
			continue
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

	// The close is fixed before ordering too: an entry written after it can
	// claim any Lamport time, but not a seq inside the closer's heads.
	var closeHeads map[string]int64
	for _, e := range entries {
		u, ok := s.Devices[e.Device]
		var c Close
		if !ok || e.Kind != KindRoomClose || past(e) || string(u) != owner || json.Unmarshal(e.Body, &c) != nil {
			continue
		}
		s.Closed, closeHeads = &ClosedInfo{Lamport: e.Lamport, Device: e.Device, Seq: e.Seq, Time: e.Time, Heads: c.Heads}, c.Heads
		break
	}
	afterClose := func(e Entry) bool {
		if s.Closed == nil || (e.Kind != KindMsg && e.Kind != KindMemberAdd && e.Kind != KindRoomRename &&
			e.Kind != KindStatus && e.Kind != KindClaim && e.Kind != KindClaimRelease) {
			return false
		}
		if closeHeads != nil && e.Seq > closeHeads[e.Device] {
			return true
		}
		return e.Lamport >= s.Closed.Lamport
	}

	// Pass 2: membership in order, entries past a cut-off never count.
	msgs := map[string]bool{} // accepted msg refs (claims must name one)
	s.Name = s.Meta.Name
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
		if afterClose(e) {
			continue // nothing new is said, admitted or renamed after the close
		}
		switch e.Kind {
		case KindMsg:
			msgs[e.Ref()] = true
		case KindStatus:
			var st Status
			if json.Unmarshal(e.Body, &st) != nil || st.Session == "" || len(st.Session) > 128 {
				continue
			}
			key := e.Device + "|" + st.Session
			if st.Event == EvEnded {
				delete(s.Live, key)
			} else {
				s.Live[key] = mergeStatus(s.Live[key], st)
			}
		case KindClaim, KindClaimRelease:
			var c Claim
			if json.Unmarshal(e.Body, &c) != nil || !msgs[c.Thread] {
				continue
			}
			cur := s.Claims[c.Thread][:0:0]
			for _, ci := range s.Claims[c.Thread] {
				if ci.Device != e.Device {
					cur = append(cur, ci)
				}
			}
			if e.Kind == KindClaim {
				cur = append(cur, ClaimInfo{Device: e.Device, Session: c.Session, Lamport: e.Lamport, Time: e.Time})
			}
			if len(cur) == 0 {
				delete(s.Claims, c.Thread)
			} else {
				s.Claims[c.Thread] = cur
			}
		case KindRoomClose:
			if s.Closed == nil || e.Device != s.Closed.Device || e.Seq != s.Closed.Seq {
				continue
			}
		case KindRoomRename:
			var r Rename
			if string(author) != owner || json.Unmarshal(e.Body, &r) != nil {
				continue
			}
			n := strings.TrimSpace(r.Name)
			if !ValidRoomName(n) {
				continue
			}
			s.Name = n
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
	s.Removed = removed
	return s
}

// Delivery states (spec §9; picked-up comes in phase 3).
type DeliveryState string

const (
	Queued    DeliveryState = "queued"
	Relayed   DeliveryState = "relayed"
	Delivered DeliveryState = "delivered"
	Seen      DeliveryState = "seen"
	PickedUp  DeliveryState = "picked-up"
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
	pick map[string]map[string]bool  // ref → device that picked it up
}

// PickedUpBy reports whether device took the message into a session.
func (ix Receipts) PickedUpBy(ref, device string) bool { return ix.pick[ref][device] }

func IndexReceipts(accepted []Entry) Receipts {
	ix := Receipts{held: map[string]map[string]int64{}, seen: map[string]map[string]bool{}, pick: map[string]map[string]bool{}}
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
		if r.State == "picked-up" {
			ref := fmt.Sprintf("%s:%d", r.Lane, r.Seq)
			if ix.pick[ref] == nil {
				ix.pick[ref] = map[string]bool{}
			}
			ix.pick[ref][e.Device] = true
		}
		if r.State == "seen" || r.State == "picked-up" {
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
		case ix.pick[m.Ref()][d]:
			out[d] = PickedUp
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
