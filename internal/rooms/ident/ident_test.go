package ident

import (
	"github.com/zalando/go-keyring"

	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memRing map[string]string

func (m memRing) Get(svc, user string) (string, error) {
	v, ok := m[svc+"/"+user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}
func (m memRing) Set(svc, user, val string) error { m[svc+"/"+user] = val; return nil }

func TestCreateFirstKeepsPrivateKeysInKeyring(t *testing.T) {
	dir := t.TempDir()
	ring := memRing{}
	s := &Store{Dir: dir, Ring: ring}
	id, err := s.CreateFirst("Hussain", "mac-mini")
	if err != nil {
		t.Fatal(err)
	}
	if len(id.Certs) != 1 || VerifyCert(id.Certs[0]) != nil || !id.IsFirst {
		t.Fatalf("self cert: %+v", id)
	}
	if len(ring) != 2 {
		t.Fatalf("keyring entries %d", len(ring))
	}
	body, _ := os.ReadFile(filepath.Join(dir, "identity.json"))
	for _, v := range ring {
		if strings.Contains(string(body), v) {
			t.Fatal("private key in identity.json")
		}
	}
	got, ok, err := (&Store{Dir: dir, Ring: ring}).Load()
	if err != nil || !ok || got.Name != "Hussain" || got.PeerID() != id.PeerID() || got.PeerID() == "" {
		t.Fatalf("load %+v %v %v", got, ok, err)
	}
	if k, ok := s.UserKey(); !ok || k == nil {
		t.Fatal("first machine has no user key")
	}
}

func TestSignAndAdoptCertForSecondMachine(t *testing.T) {
	a := &Store{Dir: t.TempDir(), Ring: memRing{}}
	ida, _ := a.CreateFirst("H", "mini")
	b := &Store{Dir: t.TempDir(), Ring: memRing{}}
	devB, err := b.DevicePub()
	if err != nil {
		t.Fatal(err)
	}
	c, err := a.SignCert(devB, "laptop")
	if err != nil || VerifyCert(c) != nil {
		t.Fatalf("sign %v", err)
	}
	idb, err := b.Adopt(ida.User, ida.Name, append(ida.Certs, c))
	if err != nil || idb.IsFirst || len(idb.Certs) != 2 {
		t.Fatalf("adopt %+v %v", idb, err)
	}
	if _, ok := b.UserKey(); ok {
		t.Fatal("second machine must not hold the user key")
	}
	if _, err := b.SignCert(devB, "x"); err == nil {
		t.Fatal("second machine signed a cert")
	}
	// A cert for someone else's device can't be adopted as mine.
	if _, err := b.Adopt(ida.User, "H", ida.Certs); err == nil {
		t.Fatal("adopted certs that don't include this device")
	}
}

func TestTamperedCertFails(t *testing.T) {
	a := &Store{Dir: t.TempDir(), Ring: memRing{}}
	id, _ := a.CreateFirst("H", "mini")
	c := id.Certs[0]
	c.Name = "evil"
	if VerifyCert(c) == nil {
		t.Fatal("tampered cert verified")
	}
}

type brokenRing struct{ memRing }

func (brokenRing) Get(string, string) (string, error) { return "", errors.New("keychain locked") }

// Review M7: a locked keychain must never mint a new device key.
func TestKeychainErrorDoesNotReplaceKey(t *testing.T) {
	s := &Store{Dir: t.TempDir(), Ring: brokenRing{memRing{}}}
	if _, err := s.DeviceKey(); err == nil {
		t.Fatal("created a key while the keychain errored")
	}
}
