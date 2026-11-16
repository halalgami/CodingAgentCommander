package netsel

import (
	"net"
	"testing"

	ma "github.com/multiformats/go-multiaddr"
)

func TestOf(t *testing.T) {
	cases := map[string]Network{
		"100.83.12.118": Tailscale, "100.64.0.1": Tailscale, "fd7a:115c:a1e0::ea3a:c76": Tailscale,
		"192.168.100.249": LAN, "10.0.0.2": LAN, "172.16.5.4": LAN, "169.254.1.1": LAN,
		"fe80::1": LAN, "fd00::1": LAN, "127.0.0.1": LAN, "::1": LAN,
	}
	for ip, want := range cases {
		if got, ok := Of(net.ParseIP(ip)); !ok || got != want {
			t.Fatalf("%s → %v %v, want %v", ip, got, ok, want)
		}
	}
	for _, ip := range []string{"8.8.8.8", "2001:1a40::1", "100.128.0.1", "172.32.0.1"} {
		if n, ok := Of(net.ParseIP(ip)); ok {
			t.Fatalf("%s classified as %v", ip, n)
		}
	}
}

func TestFilter(t *testing.T) {
	var in []ma.Multiaddr
	for _, s := range []string{
		"/ip4/192.168.1.2/tcp/4001", "/ip4/100.83.12.118/udp/4001/quic-v1", "/ip4/8.8.8.8/tcp/1",
		"/ip4/31.171.131.209/udp/4001/quic-v1/p2p/12D3KooWDMGosykAMPuYZRAnUZi2G1MTM2kFdMs1BfyhMAtB3GBx/p2p-circuit",
		"/dns4/example.com/tcp/1",
	} {
		in = append(in, ma.StringCast(s))
	}
	lan := Filter(in, LAN)
	if len(lan) != 1 || lan[0].String() != "/ip4/192.168.1.2/tcp/4001" {
		t.Fatalf("lan %v", lan)
	}
	ts := Filter(in, Tailscale)
	if len(ts) != 1 || ts[0].String() != "/ip4/100.83.12.118/udp/4001/quic-v1" {
		t.Fatalf("ts %v", ts)
	}
	if !Valid("lan") || !Valid("tailscale") || Valid("internet") {
		t.Fatal("Valid")
	}
}

func TestParseStatus(t *testing.T) {
	js := []byte(`{"BackendState":"Running","Self":{"HostName":"mini","TailscaleIPs":["100.83.12.118"]},
	 "Peer":{"k1":{"HostName":"wafas-grace-desktop","OS":"windows","TailscaleIPs":["100.70.24.39","fd7a::1"],"Online":true},
	         "k2":{"HostName":"ipad","OS":"iOS","TailscaleIPs":["100.1.1.1"],"Online":false}}}`)
	up, peers, err := parseStatus(js)
	if err != nil || !up || len(peers) != 2 {
		t.Fatalf("%v %v %+v", up, err, peers)
	}
	if peers[0].Name != "ipad" || peers[1].Name != "wafas-grace-desktop" || !peers[1].Online || peers[1].IPs[0] != "100.70.24.39" {
		t.Fatalf("peers %+v", peers)
	}
	if up, _, _ := parseStatus([]byte(`{"BackendState":"Stopped"}`)); up {
		t.Fatal("stopped reported up")
	}
}

// Review M2: loopback is LAN only when explicitly allowed (tests).
func TestLoopbackOnlyWhenAllowed(t *testing.T) {
	AllowLoopback = false
	defer func() { AllowLoopback = true }()
	if _, ok := Of(net.ParseIP("127.0.0.1")); ok {
		t.Fatal("loopback classified in production")
	}
	if _, ok := Of(net.ParseIP("::1")); ok {
		t.Fatal("::1 classified in production")
	}
}

func init() { AllowLoopback = true }
