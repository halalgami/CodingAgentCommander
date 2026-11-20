// Package roomlog is the room log (rooms spec §6): per-device signed lanes,
// Lamport order across lanes, membership replay and delivery states (§9).
package roomlog

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/canon"
	"github.com/halalgami/CodingAgentCommander/internal/rooms/ident"
)

// Entry kinds used in phase 1.
const (
	KindDeviceCert   = "device.cert"
	KindDeviceRevoke = "device.revoke"
	KindRoomMeta     = "room.meta"
	KindMemberAdd    = "member.add"
	KindMemberRemove = "member.remove"
	KindAddr         = "addr"
	KindMsg          = "msg"
	KindReceipt      = "receipt"
	KindRoomClose    = "room.close"    // owner ends the room for everyone (UX spec §7)
	KindRoomRename   = "room.rename"   // owner renames the room (UX spec §5)
	KindStatus       = "status"        // automatic session/git status (spec §8.1)
	KindClaim        = "claim"         // a device takes a handoff/question thread (§8.3)
	KindClaimRelease = "claim.release" // that device gives it back
	KindProfile      = "profile"       // the author's user display name (spec §10.4 "from <user>/<machine>")
)

const (
	MaxEntry = 64 << 10 // spec §12
	MaxBody  = 16 << 10 // message text, spec §6.3
	MaxLanes = 256
)

type Entry struct {
	Room    string          `json:"room"`
	Device  string          `json:"device"`
	Seq     int64           `json:"seq"`
	Lamport int64           `json:"lamport"`
	Time    string          `json:"time"`
	Kind    string          `json:"kind"`
	Body    json.RawMessage `json:"body"`
	Sig     []byte          `json:"sig,omitempty"`
}

// Ref names an entry: "<device>:<seq>".
func (e Entry) Ref() string { return fmt.Sprintf("%s:%d", e.Device, e.Seq) }

// RoomMeta is the room's genesis. The room id is bound to the owner's user
// key and a nonce (RoomIDOf), so nobody else can write a valid one.
type RoomMeta struct {
	Name    string `json:"name"`
	Project string `json:"project,omitempty"`
	Owner   []byte `json:"owner"`
	Nonce   []byte `json:"nonce"`
}

// RoomIDOf derives the room id from its owner and nonce.
func RoomIDOf(owner, nonce []byte) string {
	h := sha256.Sum256(append(append([]byte("commander-room\x00"), owner...), nonce...))
	return hex.EncodeToString(h[:16])
}

// NewRoom makes a fresh room genesis for owner.
func NewRoom(owner []byte, name, project string) (RoomMeta, string) {
	nonce := make([]byte, 16)
	rand.Read(nonce)
	return RoomMeta{Name: name, Project: project, Owner: owner, Nonce: nonce}, RoomIDOf(owner, nonce)
}

// Member adds or removes a user. On removal, Heads records the lane heads the
// remover held: the removed user's entries beyond them are never accepted,
// whatever Lamport time they claim (no backdating past a removal).
type Member struct {
	User  []byte           `json:"user"`
	Role  string           `json:"role,omitempty"`
	Heads map[string]int64 `json:"heads,omitempty"`
}

// Revoke retires one of the author's own devices; entries from it beyond Head
// are ignored. Sig is the user key over revokeBytes(Device, Head).
type Revoke struct {
	Device []byte `json:"device"`
	Head   int64  `json:"head"`
	Sig    []byte `json:"sig"`
}

func revokeBytes(device []byte, head int64) []byte {
	b := append([]byte("commander-revoke\x00"), device...)
	return binary.BigEndian.AppendUint64(b, uint64(head))
}

// SignRevoke builds a revocation of device (held up to head) with the user key.
func SignRevoke(user ed25519.PrivateKey, device []byte, head int64) Revoke {
	return Revoke{Device: device, Head: head, Sig: ed25519.Sign(user, revokeBytes(device, head))}
}

type Msg struct {
	Kind     string            `json:"kind"` // note|question|handoff|result|ack
	Text     string            `json:"text"`
	Parent   string            `json:"parent,omitempty"`   // thread ref
	Fields   map[string]string `json:"fields,omitempty"`   // handoff/result fields
	Attach   []Attachment      `json:"attach,omitempty"`   // content-addressed files (§6.3)
	Mentions []string          `json:"mentions,omitempty"` // device ids, highlight + notify only (§8.2)
}

// Attachment names a file by content; the bytes travel separately, fetched
// by hash from any member that holds them (§6.4 step 4).
type Attachment struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	SHA  string `json:"sha256"`
}

// Attachment caps (spec §6.3).
const (
	MaxAttachment  = 2 << 20
	MaxAttachTotal = 8 << 20
	MaxAttachCount = 8
	MaxMentions    = 16
	MaxUserName    = 40
	maxAttachName  = 100
)

// cleanText drops control characters (keeps nothing multi-line).
func cleanText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == 0x7f || r == '\u2028' || r == '\u2029' {
			return -1
		}
		return r
	}, s)
}

// AttachName reduces a name to a safe base name: no directories, no control
// characters, at most 100 characters, never "", "." or "..".
func AttachName(n string) string {
	n = strings.ReplaceAll(n, "\\", "/")
	if i := strings.LastIndex(n, "/"); i >= 0 {
		n = n[i+1:]
	}
	n = strings.TrimSpace(cleanText(n))
	if r := []rune(n); len(r) > maxAttachName {
		n = string(r[:maxAttachName])
	}
	if n == "" || n == "." || n == ".." {
		return "file"
	}
	return n
}

// ValidSHA: 64 lowercase hex characters.
func ValidSHA(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// ValidAttachments keeps the well-formed attachments within the caps, with
// cleaned names. Whatever a peer signs, this is all anyone here acts on.
func ValidAttachments(as []Attachment) []Attachment {
	var out []Attachment
	var total int64
	for _, a := range as {
		if len(out) == MaxAttachCount {
			break
		}
		if !ValidSHA(a.SHA) || a.Size < 0 || a.Size > MaxAttachment || total+a.Size > MaxAttachTotal {
			continue
		}
		total += a.Size
		a.Name = AttachName(a.Name)
		out = append(out, a)
	}
	return out
}

// Profile is the body of a profile entry: the author's user display name.
type Profile struct {
	Name string `json:"name"`
}

// UserName cleans a display name: one line, at most MaxUserName characters.
func UserName(n string) string {
	n = strings.TrimSpace(cleanText(n))
	if r := []rune(n); len(r) > MaxUserName {
		n = strings.TrimSpace(string(r[:MaxUserName]))
	}
	return n
}

// Close is the body of a room.close entry.
// Heads are the lane heads the closer held: anything a device wrote beyond
// them is after the close, whatever Lamport time it claims.
type Close struct {
	Reason string           `json:"reason,omitempty"`
	Heads  map[string]int64 `json:"heads,omitempty"`
}

// Rename is the body of a room.rename entry.
type Rename struct {
	Name string `json:"name"`
}

// MaxRoomName bounds a room name in characters; maxRoomNameBytes keeps
// multi-byte names bounded too.
const (
	MaxRoomName      = 80
	maxRoomNameBytes = 4 * MaxRoomName
)

// ValidRoomName: non-empty after trimming, at most MaxRoomName characters.
func ValidRoomName(n string) bool {
	return n != "" && len(n) <= maxRoomNameBytes && utf8.RuneCountInString(n) <= MaxRoomName
}

// Status events (spec §8.1). They are data: shown and given to agents through
// room_status, never inserted into a session as prompt text.
const (
	EvStarted  = "session.started"
	EvTitle    = "session.title"
	EvState    = "session.state"
	EvFinished = "session.finished"
	EvEnded    = "session.ended"
	EvCommit   = "git.commit"
	EvPush     = "git.push"
)

// Status is the body of a status entry. Session is a per-device session key.
type Status struct {
	Event     string `json:"event"`
	Session   string `json:"session"`
	Label     string `json:"label,omitempty"`
	Model     string `json:"model,omitempty"`
	Machine   string `json:"machine,omitempty"`
	Branch    string `json:"branch,omitempty"`
	Sha       string `json:"sha,omitempty"`
	Dirty     bool   `json:"dirty,omitempty"`
	State     string `json:"state,omitempty"` // working | idle | needs approval | errored
	Since     string `json:"since,omitempty"`
	Title     string `json:"title,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Turns     int64  `json:"turns,omitempty"`
	DurationS int64  `json:"durationS,omitempty"`
	Pushed    bool   `json:"pushed,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Subject   string `json:"subject,omitempty"`
	Listening string `json:"listening,omitempty"` // thread a room_wait is open on
}

// Claim is the body of claim and claim.release entries (spec §8.3).
type Claim struct {
	Thread  string `json:"thread"`
	Session string `json:"session,omitempty"`
}

type Receipt struct {
	Lane  string `json:"lane"`
	Seq   int64  `json:"seq"`
	State string `json:"state"` // delivered (cumulative: holds lane up to seq) | seen | picked-up (that entry)
}

type Addr struct {
	Addrs []string `json:"addrs"`
}

func signBytes(e Entry) ([]byte, error) {
	e.Sig = nil
	return canon.Marshal(e)
}

// Sign fills e.Sig with the device key.
func Sign(e *Entry, key ed25519.PrivateKey) error {
	b, err := signBytes(*e)
	if err != nil {
		return err
	}
	e.Sig = ed25519.Sign(key, b)
	return nil
}

func devicePub(id string) (ed25519.PublicKey, error) {
	p, err := peer.Decode(id)
	if err != nil {
		return nil, err
	}
	k, err := p.ExtractPublicKey()
	if err != nil {
		return nil, err
	}
	raw, err := k.Raw()
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("device key is not ed25519")
	}
	return ed25519.PublicKey(raw), nil
}

// Verify checks the entry's signature against its device id.
func Verify(e Entry) error {
	pub, err := devicePub(e.Device)
	if err != nil {
		return fmt.Errorf("bad device id: %w", err)
	}
	b, err := signBytes(e)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, b, e.Sig) {
		return errors.New("entry signature invalid")
	}
	return nil
}

// Writer appends this device's own entries.
type Writer struct {
	Room  string
	Key   ed25519.PrivateKey
	Lanes *Lanes
	Now   func() time.Time
}

func (w *Writer) Device() string { return ident.PeerIDOf(w.Key.Public().(ed25519.PublicKey)) }

func (w *Writer) Write(kind string, body any) (Entry, error) {
	b, err := canon.Marshal(body)
	if err != nil {
		return Entry{}, err
	}
	now := time.Now
	if w.Now != nil {
		now = w.Now
	}
	w.Lanes.mu.Lock()
	w.Lanes.loadLocked()
	dev := w.Device()
	e := Entry{Room: w.Room, Device: dev, Seq: w.Lanes.headLocked(dev) + 1, Lamport: w.Lanes.lamport + 1,
		Time: now().UTC().Format(time.RFC3339), Kind: kind, Body: b}
	w.Lanes.mu.Unlock()
	if err := Sign(&e, w.Key); err != nil {
		return Entry{}, err
	}
	return e, w.Lanes.Append(e)
}
