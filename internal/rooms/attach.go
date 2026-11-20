package rooms

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/ident"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/netsel"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/roomlog"
)

// Attachments (spec §6.3, §6.4 step 4): a message names files by sha256; the
// bytes sit in <room>/attachments/<sha> and travel on request, only to a
// member, only when an accepted message in that room names the hash.

const (
	protoBlob       = "/commander/rooms/blob/1"
	attachKeep      = 7 * 24 * time.Hour
	attachStoreCap  = 500 << 20
	blobReqLimit    = 8 << 10
	blobFetchBudget = 30 * time.Second
	perPeerFetch    = 8 * time.Second // a peer that never answers costs this much, not the budget (review I3)
	maxBlobServing  = 4
)

type blobReq struct {
	Room string           `json:"room"`
	Cert ident.DeviceCert `json:"cert"`
	SHA  string           `json:"sha256"`
}

func (s *Service) attachDir(roomID string) string { return filepath.Join(s.Dir, roomID, "attachments") }

// StoreAttachment copies a regular file (never a symlink) of at most 2 MB into
// the room's attachment store and describes it for a message.
func (s *Service) StoreAttachment(roomID, path string) (roomlog.Attachment, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return roomlog.Attachment{}, err
	}
	if !fi.Mode().IsRegular() {
		return roomlog.Attachment{}, fmt.Errorf("%s is not a regular file", filepath.Base(path))
	}
	f, err := os.Open(path)
	if err != nil {
		return roomlog.Attachment{}, err
	}
	defer f.Close()
	// The file opened must be the one checked: a swap for a link between the
	// two calls is refused (review I1).
	if ofi, err := f.Stat(); err != nil || !os.SameFile(fi, ofi) {
		return roomlog.Attachment{}, fmt.Errorf("%s changed while it was being attached", filepath.Base(path))
	}
	return s.StoreAttachmentFrom(roomID, filepath.Base(path), f)
}

// StoreAttachmentFrom stores at most 2 MB read from an open file.
func (s *Service) StoreAttachmentFrom(roomID, name string, r io.Reader) (roomlog.Attachment, error) {
	b, err := io.ReadAll(io.LimitReader(r, roomlog.MaxAttachment+1))
	if err != nil {
		return roomlog.Attachment{}, err
	}
	if len(b) > roomlog.MaxAttachment {
		return roomlog.Attachment{}, fmt.Errorf("%s is over 2 MB", roomlog.AttachName(name))
	}
	s.mu.Lock()
	_, err = s.roomLocked(roomID)
	s.mu.Unlock()
	if err != nil {
		return roomlog.Attachment{}, err
	}
	sum := sha256.Sum256(b)
	sha := hex.EncodeToString(sum[:])
	if err := s.putBlob(roomID, sha, b); err != nil {
		return roomlog.Attachment{}, err
	}
	return roomlog.Attachment{Name: roomlog.AttachName(name), Size: int64(len(b)), SHA: sha}, nil
}

func (s *Service) putBlob(roomID, sha string, b []byte) error {
	dir := s.attachDir(roomID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	dst := filepath.Join(dir, sha)
	if _, err := os.Stat(dst); err == nil {
		now := time.Now()
		os.Chtimes(dst, now, now) // used again: keep it another week
		return nil
	}
	tmp, err := os.CreateTemp(dir, ".blob-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	tmp.Close()
	if err := os.Rename(tmp.Name(), dst); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	evictAttachments(dir, time.Now(), attachStoreCap)
	return nil
}

// AttachmentPath is the local copy of an attachment, if this machine has it.
func (s *Service) AttachmentPath(roomID, sha string) (string, bool) {
	if !roomlog.ValidSHA(sha) || !roomIDRe.MatchString(roomID) {
		return "", false
	}
	p := filepath.Join(s.attachDir(roomID), sha)
	if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
		return p, true
	}
	return "", false
}

// namedLocked: the attachment an accepted message of r names with this
// hash, from an index built once per replayed state.
func (s *Service) namedLocked(r *room, sha string) (roomlog.Attachment, bool) {
	st := s.stateLocked(r)
	if r.named == nil {
		r.named = map[string]roomlog.Attachment{}
		for _, e := range st.Accepted {
			if e.Kind != roomlog.KindMsg {
				continue
			}
			var m roomlog.Msg
			if json.Unmarshal(e.Body, &m) != nil {
				continue
			}
			for _, a := range roomlog.ValidAttachments(m.Attach) {
				r.named[a.SHA] = a
			}
		}
	}
	a, ok := r.named[sha]
	return a, ok
}

// FetchAttachment returns the local path of an attachment a message in the
// room names, asking connected members for it when this machine lacks it.
// The bytes are checked against the hash before they are kept.
func (s *Service) FetchAttachment(ctx context.Context, roomID, sha string) (string, error) {
	if !roomlog.ValidSHA(sha) {
		return "", errors.New("not an attachment id")
	}
	s.mu.Lock()
	r, err := s.roomLocked(roomID)
	if err != nil {
		s.mu.Unlock()
		return "", err
	}
	a, ok := s.namedLocked(r, sha)
	if !ok {
		s.mu.Unlock()
		return "", errors.New("no message in this room has that attachment")
	}
	if p, have := s.AttachmentPath(roomID, sha); have {
		s.mu.Unlock()
		return p, nil
	}
	node := s.node
	cert, certOK := s.myCert()
	var ids []peer.ID
	if node != nil && certOK {
		for dev, d := range s.devices {
			user := d.User
			if len(user) == 0 {
				user, _ = s.stateLocked(r).UserOf(dev)
			}
			id, err := peer.Decode(dev)
			if err != nil || !s.mayShareLocked(r, dev, user) {
				continue
			}
			if _, up := node.Conn(id); up {
				ids = append(ids, id)
			}
		}
	}
	s.mu.Unlock()
	if len(ids) == 0 {
		return "", errors.New("no machine that has this attachment is connected")
	}
	ctx, cancel := context.WithTimeout(ctx, blobFetchBudget)
	defer cancel()
	req, _ := json.Marshal(blobReq{Room: roomID, Cert: cert, SHA: sha})
	for _, id := range ids {
		pctx, pcancel := context.WithTimeout(ctx, perPeerFetch)
		b, err := node.Request(pctx, id, protoBlob, req, roomlog.MaxAttachment)
		pcancel()
		if err != nil {
			continue
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != sha || int64(len(b)) != a.Size {
			continue // wrong bytes: try the next machine
		}
		if err := s.putBlob(roomID, sha, b); err != nil {
			return "", err
		}
		s.changed()
		p, _ := s.AttachmentPath(roomID, sha)
		return p, nil
	}
	return "", errors.New("no connected machine could send this attachment (it may have expired — attachments are kept 7 days)")
}

// handleBlob serves an attachment to a member of the room, and only one an
// accepted message names (no probing the store for other files).
func (s *Service) handleBlob(from peer.ID, net netsel.Network, raw []byte) ([]byte, error) {
	if n, ok := s.policy(from); !ok || n != net {
		return nil, errors.New("refused")
	}
	var req blobReq
	if json.Unmarshal(raw, &req) != nil || !roomIDRe.MatchString(req.Room) || !roomlog.ValidSHA(req.SHA) {
		return nil, errors.New("bad request")
	}
	if ident.VerifyCert(req.Cert) != nil || ident.PeerIDOf(req.Cert.Device) != from.String() {
		return nil, errors.New("refused")
	}
	s.blobOnce.Do(func() { s.blobSem = make(chan struct{}, maxBlobServing) })
	select {
	case s.blobSem <- struct{}{}:
		defer func() { <-s.blobSem }()
	default:
		return nil, errors.New("busy") // at most 4 at once: ask again later
	}
	s.mu.Lock()
	r := s.rooms[req.Room]
	if r == nil || !s.setUp || !s.mayShareLocked(r, from.String(), req.Cert.User) {
		s.mu.Unlock()
		return nil, errors.New("refused")
	}
	_, named := s.namedLocked(r, req.SHA)
	s.mu.Unlock()
	if !named {
		return nil, errors.New("refused")
	}
	p, ok := s.AttachmentPath(req.Room, req.SHA)
	if !ok {
		return nil, errors.New("not here")
	}
	b, err := os.ReadFile(p)
	if err != nil || len(b) > roomlog.MaxAttachment {
		return nil, errors.New("not here")
	}
	return b, nil
}

// evictAttachments removes attachments older than a week, then the oldest
// until the store is under capBytes. Only files named by a hash are touched.
func evictAttachments(dir string, now time.Time, capBytes int64) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type f struct {
		p    string
		size int64
		mod  time.Time
	}
	var fs []f
	var total int64
	for _, e := range ents {
		if !roomlog.ValidSHA(e.Name()) {
			continue
		}
		fi, err := e.Info()
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if now.Sub(fi.ModTime()) > attachKeep {
			os.Remove(p)
			continue
		}
		fs = append(fs, f{p, fi.Size(), fi.ModTime()})
		total += fi.Size()
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].mod.Before(fs[j].mod) })
	for _, x := range fs {
		if total <= capBytes {
			break
		}
		os.Remove(x.p)
		total -= x.size
	}
}

// checkMsg validates what can be checked without the lock: kind, text size,
// attachments present here and within the caps.
func (s *Service) checkMsg(roomID string, m *roomlog.Msg) error {
	if !msgKinds[m.Kind] {
		return errors.New("unknown message kind")
	}
	m.Text = strings.TrimSpace(m.Text)
	if m.Text == "" && len(m.Attach) == 0 {
		return errors.New("write something first")
	}
	size := len(m.Text)
	for k, v := range m.Fields {
		size += len(k) + len(v)
	}
	if size > roomlog.MaxBody {
		return errors.New("messages are limited to 16 KB")
	}
	if len(m.Attach) > roomlog.MaxAttachCount {
		return fmt.Errorf("at most %d attachments per message", roomlog.MaxAttachCount)
	}
	var total int64
	for i, a := range m.Attach {
		if len(roomlog.ValidAttachments([]roomlog.Attachment{a})) != 1 {
			return errors.New("bad attachment")
		}
		if _, ok := s.AttachmentPath(roomID, a.SHA); !ok {
			return fmt.Errorf("attachment %s is not on this machine", a.Name)
		}
		m.Attach[i].Name = roomlog.AttachName(a.Name)
		total += a.Size
	}
	if total > roomlog.MaxAttachTotal {
		return errors.New("attachments are limited to 8 MB per message")
	}
	if len(m.Mentions) > roomlog.MaxMentions {
		return errors.New("too many mentions")
	}
	return nil
}

// mentionsLocked keeps each mention once and refuses machines not in the room.
func mentionsLocked(st roomlog.State, m *roomlog.Msg) error {
	if len(m.Mentions) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var ms []string
	for _, d := range m.Mentions {
		if !st.MemberDevice(d) {
			return errors.New("you can only mention machines in this room")
		}
		if !seen[d] {
			seen[d] = true
			ms = append(ms, d)
		}
	}
	m.Mentions = ms
	return nil
}

// PostMsg writes a message with optional attachments (already stored here)
// and mentions (devices in the room).
func (s *Service) PostMsg(roomID string, m roomlog.Msg) (string, error) {
	if err := s.checkMsg(roomID, &m); err != nil {
		return "", err
	}
	s.mu.Lock()
	r := s.rooms[roomID]
	if r == nil {
		s.mu.Unlock()
		return "", errors.New("no such room")
	}
	if s.closedLocked(r) {
		s.mu.Unlock()
		return "", errClosed
	}
	if err := mentionsLocked(s.stateLocked(r), &m); err != nil {
		s.mu.Unlock()
		return "", err
	}
	e, err := r.w.Write(roomlog.KindMsg, m)
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	go s.SyncAll()
	s.changed()
	return e.Ref(), nil
}

// publishProfileLocked writes my display name into rooms that don't have it.
func (s *Service) publishProfileLocked() {
	n := roomlog.UserName(s.me.Name)
	if n == "" {
		return
	}
	for _, r := range s.rooms {
		if r.gone || s.closedLocked(r) || r.lanes.Heads()[s.me.PeerID()] == 0 {
			continue // no lane yet: the device cert goes first
		}
		if s.stateLocked(r).UserNames[string(s.me.User)] == n {
			continue
		}
		r.w.Write(roomlog.KindProfile, roomlog.Profile{Name: n})
	}
}

// tailnetCandidates (spec §7.2 step 2): every online tailnet machine's
// addresses with each transport/port this device was last seen on. Dialling
// a wrong host is harmless — libp2p checks the peer id.
func tailnetCandidates(known []string, peers []netsel.TailPeer) []string {
	var suffixes []string
	seen := map[string]bool{}
	for _, a := range known {
		// Only ports seen on a tailnet address, and never a relay path: a
		// peer's addr entry must not turn this into a tailnet port scan.
		m, err := ma.NewMultiaddr(a)
		if err != nil || strings.Contains(a, "p2p-circuit") {
			continue
		}
		if n, ok := netsel.OfAddr(m); !ok || n != netsel.Tailscale {
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(a, "/"), "/", 3)
		if len(parts) != 3 || (parts[0] != "ip4" && parts[0] != "ip6") {
			continue
		}
		if sfx := "/" + parts[2]; !seen[sfx] {
			seen[sfx] = true
			suffixes = append(suffixes, sfx)
		}
	}
	var out []string
	for _, p := range peers {
		if !p.Online {
			continue
		}
		for _, ip := range p.IPs {
			pfx := "/ip4/"
			if strings.Contains(ip, ":") {
				pfx = "/ip6/"
			}
			for _, sfx := range suffixes {
				out = append(out, pfx+ip+sfx)
			}
		}
	}
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}
