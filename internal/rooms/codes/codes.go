// Package codes makes the pairing and invite codes a user copies between
// machines (rooms spec §5.2–5.3) and the 4 comparison words both sides show.
package codes

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

const (
	KindPair   = "pair"
	KindInvite = "invite"
	prefix     = "CMDR-"
)

// Code is what a pairing or invite code carries.
type Code struct {
	Kind    string   `json:"k"`
	Peer    string   `json:"p"` // host device peer id
	Addrs   []string `json:"a"` // host addresses on Net only
	Net     string   `json:"n"` // the network the user chose (D10)
	Secret  []byte   `json:"s"` // one-time secret
	Expires int64    `json:"e"` // unix seconds
	Room    string   `json:"r,omitempty"`
	Name    string   `json:"rn,omitempty"`
	Project string   `json:"rp,omitempty"`
}

var enc = base32.StdEncoding.WithPadding(base32.NoPadding)

func Encode(c Code) string {
	body, _ := json.Marshal(c)
	sum := sha256.Sum256(body)
	return prefix + enc.EncodeToString(append(body, sum[:4]...))
}

func Decode(s string, now time.Time) (Code, error) {
	s = strings.ToUpper(strings.Join(strings.Fields(s), ""))
	if !strings.HasPrefix(s, prefix) {
		return Code{}, errors.New("not a Commander code")
	}
	raw, err := enc.DecodeString(strings.TrimPrefix(s, prefix))
	if err != nil || len(raw) < 5 {
		return Code{}, errors.New("the code is damaged — copy it again")
	}
	body, tail := raw[:len(raw)-4], raw[len(raw)-4:]
	sum := sha256.Sum256(body)
	if !bytes.Equal(sum[:4], tail) {
		return Code{}, errors.New("the code is damaged — copy it again")
	}
	var c Code
	if err := json.Unmarshal(body, &c); err != nil || (c.Kind != KindPair && c.Kind != KindInvite) || c.Peer == "" || len(c.Secret) < 16 {
		return Code{}, errors.New("the code is damaged — copy it again")
	}
	if now.Unix() > c.Expires {
		return Code{}, errors.New("the code has expired — ask for a new one")
	}
	return c, nil
}

// Words are the 4 comparison words both machines show; they depend on both
// peer ids and the secret, in either order.
func Words(a, b string, secret []byte) [4]string {
	ids := []string{a, b}
	sort.Strings(ids)
	h := sha256.New()
	h.Write([]byte(ids[0] + "\x00" + ids[1] + "\x00"))
	h.Write(secret)
	sum := h.Sum(nil)
	var w [4]string
	for i := range w {
		w[i] = wordlist[sum[i]]
	}
	return w
}
