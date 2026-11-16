package codes

import (
	"strings"
	"testing"
	"time"
)

func TestRoundTripAndChecks(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	c := Code{Kind: KindInvite, Peer: "12D3KooWPeer", Addrs: []string{"/ip4/100.83.12.118/tcp/4001"}, Net: "tailscale",
		Secret: []byte("0123456789abcdef"), Expires: now.Add(time.Hour).Unix(), Room: "r1", Name: "Ayina", Project: "github.com/x/ayina"}
	s := Encode(c)
	if !strings.HasPrefix(s, "CMDR-") || strings.ContainsAny(s, " \n") {
		t.Fatalf("code %q", s)
	}
	got, err := Decode(s, now)
	if err != nil || got.Room != "r1" || got.Net != "tailscale" || string(got.Secret) != string(c.Secret) {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := Decode(strings.ToLower(s), now); err != nil {
		t.Fatalf("lower-case paste rejected: %v", err)
	}
	if _, err := Decode(" "+s+"\n", now); err != nil {
		t.Fatalf("whitespace rejected: %v", err)
	}
	bad := s[:len(s)-3] + "AAA"
	if _, err := Decode(bad, now); err == nil {
		t.Fatal("bad checksum accepted")
	}
	if _, err := Decode(s, now.Add(2*time.Hour)); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired: %v", err)
	}
	if _, err := Decode("hello", now); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestWords(t *testing.T) {
	w1 := Words("A", "B", []byte("s1"))
	w2 := Words("B", "A", []byte("s1"))
	w3 := Words("A", "B", []byte("s2"))
	if w1 != w2 {
		t.Fatalf("order-dependent: %v %v", w1, w2)
	}
	if w1 == w3 {
		t.Fatal("secret ignored")
	}
	for _, w := range w1 {
		if w == "" {
			t.Fatal("empty word")
		}
	}
	if len(wordlist) != 256 {
		t.Fatalf("wordlist %d", len(wordlist))
	}
}
