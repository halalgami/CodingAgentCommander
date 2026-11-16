// Package netsel decides which network an address belongs to (rooms spec D10:
// the user chooses the network, explicitly — Local network or Tailscale — and
// Commander never uses any other).
package netsel

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"time"

	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/halalgami/CodingAgentCommander/internal/proc"
)

type Network string

const (
	LAN       Network = "lan"
	Tailscale Network = "tailscale"
)

func Valid(s string) bool { return s == string(LAN) || s == string(Tailscale) }

// Label is the user-facing name.
func (n Network) Label() string {
	if n == Tailscale {
		return "Tailscale"
	}
	return "Local network"
}

func cidrs(s ...string) []*net.IPNet {
	var out []*net.IPNet
	for _, c := range s {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}

var (
	tailnet = cidrs("100.64.0.0/10", "fd7a:115c:a1e0::/48")
	lan     = cidrs("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "fe80::/10", "fc00::/7")
	loop    = cidrs("127.0.0.0/8", "::1/128")
)

// AllowLoopback counts loopback as the Local network. Tests only: in the app,
// traffic arriving on loopback (an SSH forward, Tailscale userspace mode) may
// really come from anywhere, so it belongs to no network.
var AllowLoopback = false

func in(ip net.IP, ns []*net.IPNet) bool {
	for _, n := range ns {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// Of classifies an IP. Public addresses belong to no network Commander uses.
func Of(ip net.IP) (Network, bool) {
	if ip == nil {
		return "", false
	}
	if in(ip, tailnet) { // before LAN: fd7a:… is inside fc00::/7
		return Tailscale, true
	}
	if in(ip, lan) || (AllowLoopback && in(ip, loop)) {
		return LAN, true
	}
	return "", false
}

// OfAddr classifies a multiaddr; relayed and DNS addresses belong to none.
func OfAddr(a ma.Multiaddr) (Network, bool) {
	if _, err := a.ValueForProtocol(ma.P_CIRCUIT); err == nil {
		return "", false
	}
	ip, err := manet.ToIP(a)
	if err != nil {
		return "", false
	}
	return Of(ip)
}

// Filter keeps the addresses on network n.
func Filter(addrs []ma.Multiaddr, n Network) []ma.Multiaddr {
	var out []ma.Multiaddr
	for _, a := range addrs {
		if got, ok := OfAddr(a); ok && got == n {
			out = append(out, a)
		}
	}
	return out
}

// TailPeer is one machine on the user's tailnet.
type TailPeer struct {
	Name   string   `json:"name"`
	OS     string   `json:"os"`
	IPs    []string `json:"ips"`
	Online bool     `json:"online"`
}

func parseStatus(b []byte) (bool, []TailPeer, error) {
	var st struct {
		BackendState string
		Peer         map[string]struct {
			HostName     string
			OS           string
			TailscaleIPs []string
			Online       bool
		}
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return false, nil, err
	}
	var out []TailPeer
	for _, p := range st.Peer {
		out = append(out, TailPeer{Name: p.HostName, OS: p.OS, IPs: p.TailscaleIPs, Online: p.Online})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return st.BackendState == "Running", out, nil
}

func tailscaleBin() string {
	if p, err := exec.LookPath("tailscale"); err == nil {
		return p
	}
	for _, p := range []string{"/Applications/Tailscale.app/Contents/MacOS/Tailscale", `C:\Program Files\Tailscale\tailscale.exe`} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if runtime.GOOS == "linux" {
		return "/usr/bin/tailscale"
	}
	return ""
}

// Status reads `tailscale status --json`. up=false when Tailscale is absent
// or not running.
func Status(ctx context.Context) (up bool, peers []TailPeer) {
	bin := tailscaleBin()
	if bin == "" {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := proc.Hide(exec.CommandContext(ctx, bin, "status", "--json")).Output()
	if err != nil {
		return false, nil
	}
	up, peers, err = parseStatus(out)
	if err != nil {
		return false, nil
	}
	return up, peers
}
