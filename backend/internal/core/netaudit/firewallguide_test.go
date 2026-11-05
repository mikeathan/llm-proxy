package netaudit

import (
	"net/netip"
	"strings"
	"testing"

	"llm-proxy/internal/platform/hostwatch"
)

// vertexFacts are the facts of the owner's server (2026-11-04): SSH, mDNS and one unknown port open to the network, this
// program on 4001, the three decoys and the router's log switched on, one declared network.
func vertexFacts() FirewallFacts {
	return FirewallFacts{
		Listeners:     []hostwatch.Listener{{Proto: "tcp", Port: 22}, {Proto: "udp", Port: 5353}, {Proto: "tcp", Port: 8080}},
		Networks:      []string{"192.168.50.0/24"},
		Gateway:       netip.MustParseAddr("192.168.50.1"),
		AppPort:       4001,
		Decoys:        []int{2222, 4445, 8023},
		RouterLogPort: 5514,
	}
}

func TestFirewallGuide_VertexGetsSSHFirstThenItsServicesThenTheSwitch(t *testing.T) {
	g := FirewallGuide(vertexFacts())
	if len(g.Commands) != 1 {
		t.Fatalf("commands = %+v", g.Commands)
	}
	lines := strings.Split(g.Commands[0].Command, "\n")
	want := []string{
		"sudo ufw default deny incoming",
		"sudo ufw default allow outgoing",
		"sudo ufw allow from 192.168.50.0/24 to any port 22 proto tcp",
		"sudo ufw allow from 192.168.50.0/24 to any port 4001 proto tcp",
		"sudo ufw allow from 192.168.50.0/24 to any port 5353 proto udp",
		"sudo ufw allow from 192.168.50.0/24 to any port 2222 proto tcp",
		"sudo ufw allow from 192.168.50.0/24 to any port 4445 proto tcp",
		"sudo ufw allow from 192.168.50.0/24 to any port 8023 proto tcp",
		"sudo ufw allow from 192.168.50.1 to any port 5514 proto udp",
		"sudo ufw enable",
		"sudo ufw status numbered",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("commands:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(strings.Join(g.Steps, " "), "8080/tcp") {
		t.Errorf("a port the audit cannot attribute must be named and left without a rule: %v", g.Steps)
	}
	if g.Undo == "" || g.Risk == "" {
		t.Error("the guide must say how to undo it and what it can break")
	}
}

func TestFirewallGuide_WithoutDeclaredNetworksItSaysSoAndSuggestsNoRuleFromEverywhere(t *testing.T) {
	f := vertexFacts()
	f.Networks = nil
	g := FirewallGuide(f)
	if strings.Contains(g.Commands[0].Command, "from any") || strings.Contains(g.Commands[0].Command, "allow 22") {
		t.Fatalf("no rule may open a port to everyone: %s", g.Commands[0].Command)
	}
	if !strings.Contains(strings.Join(g.Steps, " "), "scan networks") {
		t.Errorf("steps must say the networks are missing: %v", g.Steps)
	}
}

func TestFirewallGuide_ARouterLogWithoutAKnownGatewayIsAllowedFromTheNetworks(t *testing.T) {
	f := vertexFacts()
	f.Gateway = netip.Addr{}
	if !strings.Contains(FirewallGuide(f).Commands[0].Command, "sudo ufw allow from 192.168.50.0/24 to any port 5514 proto udp") {
		t.Fatal("the router's log must still get a rule when the gateway is unknown")
	}
}

func TestFirewallGuide_NetworksThatAreNotPlainCIDRsAreLeftOut(t *testing.T) {
	f := vertexFacts()
	f.Networks = []string{"192.168.50.0/24; rm -rf /"}
	if strings.Contains(FirewallGuide(f).Commands[0].Command, "rm -rf") {
		t.Fatal("only parsed prefixes may reach a command")
	}
}
