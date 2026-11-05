package app

import (
	"net"
	"net/netip"
	"strconv"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/decoy"
	"llm-proxy/internal/core/netaudit/routerlog"
	"llm-proxy/internal/platform/hostwatch"
	"llm-proxy/internal/platform/neighbors"
)

// firewallSource reads the facts the firewall plan is built from when its guide is shown: the listeners from the kernel's tables,
// the declared networks, the router's address, this program's own port and the switches of its own sockets.
type firewallSource struct {
	listeners func() ([]hostwatch.Listener, error)
	networks  func() []string
	gateway   func() netip.Addr
	bind      func() string
	decoysOn  func() bool
	routerOn  func() bool
}

func (f firewallSource) facts() (netaudit.FirewallFacts, error) {
	listening, err := f.listeners()
	if err != nil {
		return netaudit.FirewallFacts{}, err
	}
	out := netaudit.FirewallFacts{Listeners: listening, Networks: f.networks(), Gateway: f.gateway(), AppPort: networkPort(f.bind())}
	if f.decoysOn() {
		for _, s := range decoy.Specs {
			out.Decoys = append(out.Decoys, s.Number)
		}
	}
	if f.routerOn() {
		out.RouterLogPort = routerlog.Port
	}
	return out, nil
}

// networkPort is the port of a bind address that other computers can reach, or 0 when it is this computer's only.
func networkPort(bind string) int {
	host, port, err := net.SplitHostPort(bind)
	if err != nil {
		return 0
	}
	if a, err := netip.ParseAddr(host); host == "localhost" || (err == nil && a.IsLoopback()) {
		return 0
	}
	n, _ := strconv.Atoi(port)
	return n
}

// netauditGuides gives findings the guides built for this computer (the firewall plan).
func (s *AppServices) netauditGuides() netaudit.HostGuides {
	n := s.netaudit
	src := firewallSource{
		listeners: hostwatch.Proc{Root: "/proc"}.Listeners,
		networks: func() []string {
			var out []string
			for _, r := range n.Scope.View().Ranges {
				out = append(out, r.CIDR)
			}
			return out
		},
		gateway:  cachedGateway(neighborSource{reader: neighbors.NewReader()}, gatewayTTL, time.Now),
		bind:     func() string { return bindAddress(s.AppCtx.GetSystem()) },
		decoysOn: n.DecoyConfig.Enabled,
		routerOn: n.RouterLogConfig.Enabled,
	}
	return netaudit.HostGuides{Facts: src.facts}
}
