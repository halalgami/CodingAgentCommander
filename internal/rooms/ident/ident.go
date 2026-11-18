// Package ident holds a Commander's room identity (rooms spec §4, §5.1–5.2):
// a device key per installation and a user key on the user's first machine.
// Private keys live only in the OS keychain; identity.json holds public keys
// and device certificates.
package ident

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/zalando/go-keyring"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/canon"
)

const service = "commander-rooms"

// Keyring is the OS keychain (fake in tests).
type Keyring interface {
	Get(service, user string) (string, error)
	Set(service, user, value string) error
	Delete(service, user string) error
}

type osRing struct{}

func (osRing) Get(s, u string) (string, error) { return keyring.Get(s, u) }
func (osRing) Set(s, u, v string) error        { return keyring.Set(s, u, v) }
func (osRing) Delete(s, u string) error        { return keyring.Delete(s, u) }

// OSKeyring is the real keychain.
var OSKeyring Keyring = osRing{}

// DeviceCert says: user key User vouches for device key Device.
type DeviceCert struct {
	User   []byte `json:"user"`
	Device []byte `json:"device"`
	Name   string `json:"name"`
	Issued string `json:"issued"`
	Sig    []byte `json:"sig,omitempty"`
}

func certBytes(c DeviceCert) ([]byte, error) {
	c.Sig = nil
	return canon.Marshal(c)
}

func VerifyCert(c DeviceCert) error {
	if len(c.User) != ed25519.PublicKeySize || len(c.Device) != ed25519.PublicKeySize {
		return errors.New("bad key size")
	}
	b, err := certBytes(c)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(c.User), b, c.Sig) {
		return errors.New("certificate signature invalid")
	}
	return nil
}

// Identity is identity.json.
type Identity struct {
	User    []byte       `json:"user"`
	Device  []byte       `json:"device"`
	Name    string       `json:"name"`
	IsFirst bool         `json:"isFirst"`
	Certs   []DeviceCert `json:"certs"`
}

// PeerIDOf is the libp2p peer ID of a device public key.
func PeerIDOf(pub []byte) string {
	k, err := crypto.UnmarshalEd25519PublicKey(pub)
	if err != nil {
		return ""
	}
	id, err := peer.IDFromPublicKey(k)
	if err != nil {
		return ""
	}
	return id.String()
}

func (id Identity) PeerID() string { return PeerIDOf(id.Device) }

// Store reads and writes the identity under Dir.
type Store struct {
	Dir  string
	Ring Keyring
	Now  func() time.Time
	mu   sync.Mutex
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Store) path() string { return filepath.Join(s.Dir, "identity.json") }

func (s *Store) Load() (Identity, bool, error) {
	b, err := os.ReadFile(s.path())
	if errors.Is(err, os.ErrNotExist) {
		return Identity{}, false, nil
	}
	if err != nil {
		return Identity{}, false, err
	}
	var id Identity
	if err := json.Unmarshal(b, &id); err != nil {
		return Identity{}, false, fmt.Errorf("identity.json: %w", err)
	}
	return id, true, nil
}

func (s *Store) save(id Identity) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(id, "", "  ")
	tmp := s.path() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path())
}

func (s *Store) key(name string, create bool) (ed25519.PrivateKey, error) {
	v, err := s.Ring.Get(service, name)
	if err == nil {
		seed, err := base64.StdEncoding.DecodeString(v)
		if err != nil || len(seed) != ed25519.SeedSize {
			return nil, fmt.Errorf("keychain %s key is corrupt", name)
		}
		return ed25519.NewKeyFromSeed(seed), nil
	}
	if !create || !errors.Is(err, keyring.ErrNotFound) {
		return nil, err // a locked or failing keychain must never mint a new identity
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := s.Ring.Set(service, name, base64.StdEncoding.EncodeToString(priv.Seed())); err != nil {
		return nil, fmt.Errorf("keychain: %w", err)
	}
	return priv, nil
}

// DeviceKey returns this installation's device key, creating it once.
func (s *Store) DeviceKey() (ed25519.PrivateKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.key("device", true)
}

func (s *Store) DevicePub() ([]byte, error) {
	k, err := s.DeviceKey()
	if err != nil {
		return nil, err
	}
	return k.Public().(ed25519.PublicKey), nil
}

// UserKey is present only on the user's first machine.
func (s *Store) UserKey() (ed25519.PrivateKey, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, err := s.key("user", false)
	return k, err == nil
}

// CreateFirst makes this the user's first machine.
func (s *Store) CreateFirst(name, deviceName string) (Identity, error) {
	dev, err := s.DevicePub()
	if err != nil {
		return Identity{}, err
	}
	s.mu.Lock()
	user, err := s.key("user", true)
	s.mu.Unlock()
	if err != nil {
		return Identity{}, err
	}
	id := Identity{User: user.Public().(ed25519.PublicKey), Device: dev, Name: name, IsFirst: true}
	c, err := s.sign(user, dev, deviceName)
	if err != nil {
		return Identity{}, err
	}
	id.Certs = []DeviceCert{c}
	return id, s.save(id)
}

func (s *Store) sign(user ed25519.PrivateKey, dev []byte, name string) (DeviceCert, error) {
	c := DeviceCert{User: user.Public().(ed25519.PublicKey), Device: dev, Name: name, Issued: s.now().UTC().Format(time.RFC3339)}
	b, err := certBytes(c)
	if err != nil {
		return DeviceCert{}, err
	}
	c.Sig = ed25519.Sign(user, b)
	return c, nil
}

// SignCert vouches for another device of this user (first machine only).
func (s *Store) SignCert(devicePub []byte, name string) (DeviceCert, error) {
	user, ok := s.UserKey()
	if !ok {
		return DeviceCert{}, errors.New("only your first Commander machine can add machines")
	}
	if len(devicePub) != ed25519.PublicKeySize {
		return DeviceCert{}, errors.New("bad device key")
	}
	return s.sign(user, devicePub, name)
}

// Adopt stores the identity a first machine handed this device at pairing.
// The certs must include a valid one for this device under that user.
func (s *Store) Adopt(user []byte, name string, certs []DeviceCert) (Identity, error) {
	dev, err := s.DevicePub()
	if err != nil {
		return Identity{}, err
	}
	mine := false
	for _, c := range certs {
		if err := VerifyCert(c); err != nil || string(c.User) != string(user) {
			return Identity{}, errors.New("a certificate does not belong to this user")
		}
		if string(c.Device) == string(dev) {
			mine = true
		}
	}
	if !mine {
		return Identity{}, errors.New("no certificate for this machine")
	}
	id := Identity{User: user, Device: dev, Name: name, Certs: certs}
	return id, s.save(id)
}

// AddCert records a newly certified device of mine (first machine, after pairing).
func (s *Store) AddCert(c DeviceCert) (Identity, error) {
	id, ok, err := s.Load()
	if err != nil || !ok {
		return Identity{}, errors.New("no identity")
	}
	if VerifyCert(c) != nil || string(c.User) != string(id.User) {
		return Identity{}, errors.New("certificate not from this user")
	}
	id.Certs = append(id.Certs, c)
	return id, s.save(id)
}

// Reset forgets this machine's rooms identity: both keys leave the keychain
// and identity.json goes. The next setup mints a new device (UX spec §8).
func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range []string{"device", "user"} {
		if err := s.Ring.Delete(service, k); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return fmt.Errorf("keychain: %w", err)
		}
	}
	if err := os.Remove(s.path()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
