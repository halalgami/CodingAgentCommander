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
	"time"

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
	Kind   string            `json:"kind"` // note|question|handoff|result|ack
	Text   string            `json:"text"`
	Parent string            `json:"parent,omitempty"` // thread ref
	Fields map[string]string `json:"fields,omitempty"` // handoff/result fields
}

type Receipt struct {
	Lane  string `json:"lane"`
	Seq   int64  `json:"seq"`
	State string `json:"state"` // delivered (cumulative: holds lane up to seq) | seen (that entry)
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
