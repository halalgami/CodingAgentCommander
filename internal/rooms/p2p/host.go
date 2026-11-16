// Package p2p is Commander's room transport (rooms spec §7, D4/D10): libp2p
// with TCP + QUIC, Noise and yamux, and nothing that reaches strangers — no
// relay, no hole punching, no NAT mapping, no public DHT. A connection gater
// lets a known device in only over the network the user chose for it, and an
// unknown device only while a pairing or invite is open on that network.
package p2p

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/halalgami/CodingAgentCommander/internal/rooms/netsel"
)

// MDNSService is the local-network discovery name.
const MDNSService = "_commander-rooms._udp"

type Config struct {
	Key       crypto.PrivKey
	ListenIPs []string // default 0.0.0.0 and ::
	Port      int      // 0 = random
	// Policy: the network the user chose for a known device.
	Policy func(peer.ID) (netsel.Network, bool)
	// Pairing: the network of an open pairing/invite window, if any.
	Pairing func() (netsel.Network, bool)
	// OnFound: a device seen by mDNS on the local network.
	OnFound func(peer.AddrInfo)
}

type Node struct {
	cfg     Config
	h       host.Host
	mu      sync.Mutex
	mdns    mdns.Service
	refused map[peer.ID]netsel.Network // last wrong-network attempt per known device
}

// Refused reports the network of the last attempt refused for id because it
// came over a network the user did not choose for it (shown in the UI).
func (n *Node) Refused(id peer.ID) (netsel.Network, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	v, ok := n.refused[id]
	return v, ok
}

func New(cfg Config) (*Node, error) {
	if cfg.Policy == nil || cfg.Pairing == nil || cfg.Key == nil {
		return nil, errors.New("p2p: key, policy and pairing are required")
	}
	ips := cfg.ListenIPs
	if len(ips) == 0 {
		ips = []string{"0.0.0.0", "::"}
	}
	var listen []string
	for _, ip := range ips {
		fam := "ip4"
		if containsColon(ip) {
			fam = "ip6"
		}
		listen = append(listen, fmt.Sprintf("/%s/%s/tcp/%d", fam, ip, cfg.Port), fmt.Sprintf("/%s/%s/udp/%d/quic-v1", fam, ip, cfg.Port))
	}
	n := &Node{cfg: cfg}
	h, err := libp2p.New(
		libp2p.Identity(cfg.Key),
		libp2p.ListenAddrStrings(listen...),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(libp2pquic.NewTransport),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Muxer(yamux.ID, yamux.DefaultTransport),
		libp2p.DisableRelay(),
		libp2p.ConnectionGater(gater{n}),
	)
	if err != nil {
		return nil, err
	}
	n.h = h
	return n, nil
}

func containsColon(s string) bool {
	for _, c := range s {
		if c == ':' {
			return true
		}
	}
	return false
}

func (n *Node) Host() host.Host       { return n.h }
func (n *Node) ID() peer.ID           { return n.h.ID() }
func (n *Node) Addrs() []ma.Multiaddr { return n.h.Addrs() }

func (n *Node) Close() error {
	n.SetMDNS(false)
	return n.h.Close()
}

// allowed: may we talk to p over addr's network?
func (n *Node) allowed(p peer.ID, addr ma.Multiaddr) bool {
	net, ok := netsel.OfAddr(addr)
	if !ok {
		return false
	}
	if want, known := n.cfg.Policy(p); known {
		return net == want
	}
	if want, open := n.cfg.Pairing(); open {
		return net == want
	}
	return false
}

// Dial connects to a device using only its addresses on the chosen network.
func (n *Node) Dial(ctx context.Context, id peer.ID, addrs []ma.Multiaddr) error {
	var ok []ma.Multiaddr
	for _, a := range addrs {
		if n.allowed(id, a) {
			ok = append(ok, a)
		}
	}
	if len(ok) == 0 {
		return errors.New("no address on the chosen network")
	}
	n.h.Peerstore().AddAddrs(id, ok, peerstore.TempAddrTTL)
	return n.h.Connect(ctx, peer.AddrInfo{ID: id, Addrs: ok})
}

// Conn reports the network of a live connection to id.
func (n *Node) Conn(id peer.ID) (netsel.Network, bool) {
	for _, c := range n.h.Network().ConnsToPeer(id) {
		if net, ok := netsel.OfAddr(c.RemoteMultiaddr()); ok {
			return net, true
		}
	}
	return "", false
}

// SetMDNS turns local-network discovery on or off. On only while some device
// uses the Local network or a Local-network pairing is open.
func (n *Node) SetMDNS(on bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if on == (n.mdns != nil) {
		return
	}
	if !on {
		n.mdns.Close()
		n.mdns = nil
		return
	}
	s := mdns.NewMdnsService(n.h, MDNSService, notifee{n})
	if s.Start() == nil {
		n.mdns = s
	}
}

type notifee struct{ n *Node }

func (f notifee) HandlePeerFound(pi peer.AddrInfo) {
	if pi.ID == f.n.ID() || f.n.cfg.OnFound == nil {
		return
	}
	pi.Addrs = netsel.Filter(pi.Addrs, netsel.LAN)
	if len(pi.Addrs) > 0 {
		f.n.cfg.OnFound(pi)
	}
}

// gater enforces D10 on every dial and every inbound connection.
type gater struct{ n *Node }

func (g gater) InterceptPeerDial(p peer.ID) bool {
	if _, known := g.n.cfg.Policy(p); known {
		return true
	}
	_, open := g.n.cfg.Pairing()
	return open
}
func (g gater) InterceptAddrDial(p peer.ID, a ma.Multiaddr) bool { return g.n.allowed(p, a) }
func (g gater) InterceptAccept(c network.ConnMultiaddrs) bool {
	_, ok := netsel.OfAddr(c.RemoteMultiaddr())
	return ok // public addresses never get past the handshake
}
func (g gater) InterceptSecured(_ network.Direction, p peer.ID, c network.ConnMultiaddrs) bool {
	if g.n.allowed(p, c.RemoteMultiaddr()) {
		g.n.mu.Lock()
		delete(g.n.refused, p)
		g.n.mu.Unlock()
		return true
	}
	if got, ok := netsel.OfAddr(c.RemoteMultiaddr()); ok {
		if _, known := g.n.cfg.Policy(p); known {
			g.n.mu.Lock()
			if g.n.refused == nil {
				g.n.refused = map[peer.ID]netsel.Network{}
			}
			g.n.refused[p] = got
			g.n.mu.Unlock()
		}
	}
	return false
}
func (g gater) InterceptUpgraded(network.Conn) (bool, control.DisconnectReason) { return true, 0 }
