package rooms

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/ident"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/roomlog"
)

// Closing, leaving and removing a room (UX spec §7), resetting this
// machine's rooms identity (§8) and cancelling a join.

var (
	errClosed   = errors.New("this room is closed")
	errNotOwner = errors.New("only the room's owner can close it")
)

// tomb records a room removed from this machine. Restoring lets it be
// adopted again (from my machines, or by a new invite) and keeps it listed
// until it has arrived (final review I6).
type tomb struct {
	Name      string `json:"name"`
	Restoring bool   `json:"restoring,omitempty"`
}

// HiddenRoom is a room the user removed from this machine.
type HiddenRoom struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Restoring bool   `json:"restoring"` // waiting for the room to sync back
}

func (s *Service) closedLocked(r *room) bool {
	st := s.stateLocked(r)
	return r.st.Left || st.Closed != nil || st.Removed[string(s.me.User)]
}

func (s *Service) roomLocked(id string) (*room, error) {
	if !s.setUp {
		return nil, errNotSetUp
	}
	r := s.rooms[id]
	if r == nil {
		return nil, errors.New("no such room")
	}
	return r, nil
}

// after a write: tell peers and the UI.
func (s *Service) wrote() {
	go s.SyncAll()
	s.changed()
}

// CloseRoom ends the room for everyone. Owner only (any of the owner's machines).
func (s *Service) CloseRoom(id string) error {
	s.mu.Lock()
	r, err := s.roomLocked(id)
	if err == nil {
		switch st := s.stateLocked(r); {
		case string(st.Meta.Owner) != string(s.me.User):
			err = errNotOwner
		case st.Closed != nil:
			err = errClosed
		default:
			_, err = r.w.Write(roomlog.KindRoomClose, roomlog.Close{Heads: r.lanes.Heads()})
		}
	}
	s.mu.Unlock()
	if err == nil {
		s.wrote()
	}
	return err
}

// RenameRoom renames an open room for everyone. Owner only.
func (s *Service) RenameRoom(id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("give the room a name")
	}
	if !roomlog.ValidRoomName(name) {
		return errors.New("room names are limited to 80 characters")
	}
	s.mu.Lock()
	r, err := s.roomLocked(id)
	if err == nil {
		switch st := s.stateLocked(r); {
		case string(st.Meta.Owner) != string(s.me.User):
			err = errNotOwner
		case s.closedLocked(r):
			err = errClosed
		default:
			_, err = r.w.Write(roomlog.KindRoomRename, roomlog.Rename{Name: name})
		}
	}
	s.mu.Unlock()
	if err == nil {
		s.wrote()
	}
	return err
}

// closeSettledLocked: the room is closed and every other member device is
// known to hold the close, so there is nothing left to dial for.
func (s *Service) closeSettledLocked(r *room) bool {
	if r == nil {
		return false
	}
	st := s.stateLocked(r)
	if st.Closed == nil {
		return false
	}
	me := s.me.PeerID()
	obs := s.observedRoomLocked(r.id)
	for _, d := range s.memberDevicesLocked(st) {
		if d == me || d == st.Closed.Device {
			continue
		}
		// It must hold the close and everything the closer held with it,
		// or a message inside the close could never reach it (final review M3).
		if obs[d][st.Closed.Device] < st.Closed.Seq {
			return false
		}
		for lane, h := range st.Closed.Heads {
			if lane != d && obs[d][lane] < h {
				return false
			}
		}
	}
	return true
}

// LeaveRoom removes me from a colleague's room; my copy turns read-only.
func (s *Service) LeaveRoom(id string) error {
	s.mu.Lock()
	r, err := s.roomLocked(id)
	if err == nil {
		switch st := s.stateLocked(r); {
		case string(st.Meta.Owner) == string(s.me.User):
			err = errors.New("you own this room — close it instead")
		case r.st.Left:
			err = errClosed
		default:
			if _, err = r.w.Write(roomlog.KindMemberRemove, roomlog.Member{User: s.me.User, Heads: r.lanes.Heads()}); err == nil {
				r.st.Left = true
				s.saveRoomLocked(r)
			}
		}
	}
	s.mu.Unlock()
	if err == nil {
		s.wrote()
	}
	return err
}

func (s *Service) saveTombsLocked() error {
	return writeJSON(filepath.Join(s.Dir, "tombstones.json"), s.tombs)
}

// RemoveLocal deletes this machine's copy and keeps sync from bringing it back.
func (s *Service) RemoveLocal(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.roomLocked(id)
	if err != nil {
		return err
	}
	s.tombs[id] = tomb{Name: s.stateLocked(r).Name}
	if err := s.saveTombsLocked(); err != nil {
		delete(s.tombs, id)
		return err
	}
	r.gone = true
	delete(s.rooms, id)
	os.RemoveAll(filepath.Join(s.Dir, id))
	go s.changed()
	return nil
}

// RestoreLocal lets the room sync back from the other machines.
func (s *Service) RestoreLocal(id string) error {
	s.mu.Lock()
	t, ok := s.tombs[id]
	if !ok {
		s.mu.Unlock()
		return errors.New("that room is not hidden")
	}
	os.RemoveAll(filepath.Join(s.Dir, id)) // anything a late sync left behind
	t.Restoring = true
	s.tombs[id] = t
	err := s.saveTombsLocked()
	s.mu.Unlock()
	if err == nil {
		go s.DialAll()
		s.changed()
	}
	return err
}

// Hidden lists the rooms removed from this machine.
func (s *Service) Hidden() []HiddenRoom {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []HiddenRoom{}
	for id, t := range s.tombs {
		out = append(out, HiddenRoom{ID: id, Name: t.Name, Restoring: t.Restoring})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Reset forgets rooms on this machine: identity, keys, rooms, devices.
// Afterwards rooms are not set up; Setup starts again with a new device.
func (s *Service) Reset() error {
	s.Close()
	s.mu.Lock()
	ids := s.ids
	for _, r := range s.rooms {
		r.gone = true // in-flight syncs must not write into the fresh setup
	}
	s.rooms, s.devices = map[string]*room{}, map[string]DevicePref{}
	s.mu.Unlock()
	if ids != nil {
		if err := ids.Reset(); err != nil {
			return err
		}
	}
	ents, _ := os.ReadDir(s.Dir)
	for _, e := range ents {
		os.RemoveAll(filepath.Join(s.Dir, e.Name()))
	}
	s.mu.Lock()
	s.node, s.stop, s.stopped, s.win, s.join, s.joinPeer = nil, nil, false, nil, nil, nil
	s.me, s.setUp, s.joining = ident.Identity{}, false, false
	s.mu.Unlock()
	err := s.Open()
	s.changed()
	return err
}

// CancelJoin stops a join this machine started.
func (s *Service) CancelJoin() {
	s.mu.Lock()
	stop := s.joinStop
	s.joinStop = nil
	js := s.join
	s.mu.Unlock()
	if stop != nil {
		stop()
	}
	s.finishJoin(js, errors.New("cancelled"), "")
}

// Machines lists my other machines, whatever rooms they are in (UX spec §8).
func (s *Service) Machines() []PeerView {
	ts := s.tailscaleUp()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []PeerView{}
	me := s.me.PeerID()
	for d, pref := range s.devices {
		if d == me || len(s.me.User) == 0 || string(pref.User) != string(s.me.User) {
			continue
		}
		pv := PeerView{Device: d, Name: pref.Name, Mine: true, Net: string(pref.Net), LastSeen: pref.LastSeen}
		pv.State, pv.Reason = s.peerStateLocked(d, pref, ts)
		if id, err := peer.Decode(d); err == nil && s.node != nil && pref.Net != "" {
			if got, refused := s.node.Refused(id); refused && got != pref.Net {
				pv.Tried = string(got)
			}
		}
		out = append(out, pv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
