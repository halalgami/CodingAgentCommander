package p2p

import (
	"context"
	"crypto/rand"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/netsel"
)

// policy is a mutable peer → network table.
type policy struct {
	mu  sync.Mutex
	m   map[peer.ID]netsel.Network
	pai netsel.Network
}

func (p *policy) Of(id peer.ID) (netsel.Network, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n, ok := p.m[id]
	return n, ok
}
func (p *policy) Pairing() (netsel.Network, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pai, p.pai != ""
}
func (p *policy) set(id peer.ID, n netsel.Network) { p.mu.Lock(); p.m[id] = n; p.mu.Unlock() }

func testNode(t *testing.T) (*Node, *policy) {
	t.Helper()
	k, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	pol := &policy{m: map[peer.ID]netsel.Network{}}
	n, err := New(Config{Key: k, ListenIPs: []string{"127.0.0.1"}, Port: 0, Policy: pol.Of, Pairing: pol.Pairing})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { n.Close() })
	return n, pol
}

// dial a → b and report whether a connection survives on BOTH sides. A
// listener rejects after the handshake, so the dialer's Connect can return
// nil for a connection that is closed a moment later.
func dial(a, b *Node) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Dial(ctx, b.ID(), b.Addrs()); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	_, okA := a.Conn(b.ID())
	_, okB := b.Conn(a.ID())
	if !okA || !okB {
		return errors.New("connection closed by the other side")
	}
	return nil
}

func TestKnownPeerOnChosenNetworkConnects(t *testing.T) {
	a, pa := testNode(t)
	b, pb := testNode(t)
	pa.set(b.ID(), netsel.LAN) // loopback counts as LAN
	pb.set(a.ID(), netsel.LAN)
	if err := dial(a, b); err != nil {
		t.Fatal(err)
	}
	if n, ok := a.Conn(b.ID()); !ok || n != netsel.LAN {
		t.Fatalf("conn network %v %v", n, ok)
	}
}

func TestWrongNetworkRefusedBothWays(t *testing.T) {
	a, pa := testNode(t)
	b, pb := testNode(t)
	pa.set(b.ID(), netsel.Tailscale) // a will only reach b over Tailscale
	pb.set(a.ID(), netsel.LAN)
	if err := dial(a, b); err == nil {
		t.Fatal("a dialled b over the wrong network")
	}
	if err := dial(b, a); err == nil {
		t.Fatal("a accepted b over the wrong network")
	}
}

func TestUnknownPeerOnlyWhilePairingOpen(t *testing.T) {
	a, pa := testNode(t)
	b, pb := testNode(t)
	pb.set(a.ID(), netsel.LAN)
	if err := dial(b, a); err == nil {
		t.Fatal("stranger accepted with no pairing open")
	}
	pa.mu.Lock()
	pa.pai = netsel.LAN
	pa.mu.Unlock()
	b2, pb2 := testNode(t)
	pb2.set(a.ID(), netsel.LAN)
	if err := dial(b2, a); err != nil {
		t.Fatalf("pairing open on LAN: %v", err)
	}
	pa.mu.Lock()
	pa.pai = netsel.Tailscale
	pa.mu.Unlock()
	b3, pb3 := testNode(t)
	pb3.set(a.ID(), netsel.LAN)
	if err := dial(b3, a); err == nil {
		t.Fatal("pairing open for Tailscale accepted a LAN stranger")
	}
}

func TestNoRelayAddressesOrDials(t *testing.T) {
	a, pa := testNode(t)
	b, _ := testNode(t)
	pa.set(b.ID(), netsel.LAN)
	for _, x := range a.Addrs() {
		if _, err := x.ValueForProtocol(ma.P_CIRCUIT); err == nil {
			t.Fatalf("relay address advertised: %s", x)
		}
	}
	circ := ma.StringCast("/ip4/127.0.0.1/tcp/1/p2p/" + a.ID().String() + "/p2p-circuit")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.Dial(ctx, b.ID(), []ma.Multiaddr{circ}); err == nil {
		t.Fatal("dialled through a relay address")
	}
}

func TestRequestResponseAndCaps(t *testing.T) {
	a, pa := testNode(t)
	b, pb := testNode(t)
	pa.set(b.ID(), netsel.LAN)
	pb.set(a.ID(), netsel.LAN)
	b.Handle("/t/1", func(peer.ID) int { return 1024 }, func(from peer.ID, net netsel.Network, req []byte) ([]byte, error) {
		if from != a.ID() || net != netsel.LAN {
			return nil, errors.New("wrong caller")
		}
		return append([]byte("echo:"), req...), nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Dial(ctx, b.ID(), b.Addrs()); err != nil {
		t.Fatal(err)
	}
	got, err := a.Request(ctx, b.ID(), "/t/1", []byte("hi"), 1024)
	if err != nil || string(got) != "echo:hi" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := a.Request(ctx, b.ID(), "/t/1", make([]byte, 2048), 1024); err == nil {
		t.Fatal("oversized request answered")
	}
}

// Review I3: an unknown peer gets no room to send big frames.
func TestUnknownPeerFrameLimit(t *testing.T) {
	a, pa := testNode(t)
	b, pb := testNode(t)
	pb.set(a.ID(), netsel.LAN)
	pa.mu.Lock()
	pa.pai = netsel.LAN // a pairing window lets strangers connect
	pa.mu.Unlock()
	a.Handle("/t/big", func(p peer.ID) int {
		if _, known := pa.Of(p); known {
			return 1 << 20
		}
		return 0
	}, func(peer.ID, netsel.Network, []byte) ([]byte, error) { return []byte("ok"), nil })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b.Dial(ctx, a.ID(), a.Addrs())
	if _, err := b.Request(ctx, a.ID(), "/t/big", make([]byte, 1000), 1024); err == nil {
		t.Fatal("stranger's frame accepted")
	}
	pa.set(b.ID(), netsel.LAN)
	if _, err := b.Request(ctx, a.ID(), "/t/big", make([]byte, 1000), 1024); err != nil {
		t.Fatalf("known peer refused: %v", err)
	}
}

// Review I6: a refused attempt on the wrong network is remembered for the UI.
func TestRefusedAttemptRecorded(t *testing.T) {
	a, pa := testNode(t)
	b, pb := testNode(t)
	pa.set(b.ID(), netsel.Tailscale)
	pb.set(a.ID(), netsel.LAN)
	dial(b, a)
	if n, ok := a.Refused(b.ID()); !ok || n != netsel.LAN {
		t.Fatalf("refused attempt %v %v", n, ok)
	}
}

func init() { netsel.AllowLoopback = true }
