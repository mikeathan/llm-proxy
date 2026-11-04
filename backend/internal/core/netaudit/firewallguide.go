package netaudit

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"llm-proxy/internal/platform/hostwatch"
)

// FirewallFacts are what the firewall plan is built from, all of them facts the module already holds: the ports this computer
// listens on for other computers (not this program's own sockets), the declared scan networks, the router's address, and the
// sockets this program opens itself.
type FirewallFacts struct {
	Listeners []hostwatch.Listener
	Networks  []string
	Gateway   netip.Addr
	// AppPort is this program's own port, 0 when it listens on this computer only.
	AppPort int
	// Decoys are the decoy ports while the decoys are on; RouterLogPort the router log's port while it is on, else 0.
	Decoys        []int
	RouterLogPort int
}

// knownServices are the listeners the plan can name without a process name: the port alone says what they are on Ubuntu.
var knownServices = map[hostwatch.Listener]string{
	{Proto: "tcp", Port: 22}:   "SSH, so you keep your way in",
	{Proto: "udp", Port: 5353}: "mDNS, which lets devices on the network find this computer by name",
}

// firewallRule is one allow rule and the line that says why.
type firewallRule struct {
	from  string
	proto string
	port  int
	why   string
}

func (r firewallRule) command() string {
	return fmt.Sprintf("sudo ufw allow from %s to any port %d proto %s", r.from, r.port, r.proto)
}

// FirewallGuide is the guide for a switched-off firewall, built for this computer: deny what comes in by default, allow each
// service it can name from the declared networks only, SSH first, and switch on last. It suggests no rule for a port it cannot
// name, and says which those are; it never suggests a rule that opens a port to everyone. Nothing in it is run (ADR-0010).
func FirewallGuide(f FirewallFacts) FixGuide {
	nets := plainNetworks(f.Networks)
	rules, unknown := firewallRules(f, nets)
	cmds := []string{"sudo ufw default deny incoming", "sudo ufw default allow outgoing"}
	steps := []string{"These commands are built from what this computer listens on now and from your scan networks. Read them before you run them, one at a time, with a second session open."}
	if len(nets) == 0 {
		steps = append(steps, "No scan networks are declared, so no allow rule is suggested: a rule from everywhere would undo the firewall. Declare your network under the scan networks first, then open this guide again.")
	}
	for _, r := range rules {
		cmds = append(cmds, r.command())
		steps = append(steps, fmt.Sprintf("Allow %d/%s from %s: %s.", r.port, r.proto, r.from, r.why))
	}
	if len(unknown) > 0 {
		steps = append(steps, "Also listening, but the audit cannot tell what they are, so they get no rule and stop being reachable once the firewall is on: "+
			strings.Join(unknown, ", ")+". Find the program (sudo ss -lntup) and add a rule like the ones above for each one you use.")
	}
	steps = append(steps,
		"Switch the firewall on last, and check the rules it shows. Ports that Docker publishes bypass ufw: bind them to 127.0.0.1 in the container settings if they should not be reachable.",
		"If the router already blocks everything you do not forward and you accept that, accept the finding with a note instead.")
	cmds = append(cmds, "sudo ufw enable", "sudo ufw status numbered")
	return FixGuide{
		Steps:    steps,
		Commands: []GuideCommand{{platformLinux, strings.Join(cmds, "\n")}},
		Risk:     "Anything not allowed stops being reachable from other computers. If SSH is not in the list, switching on locks you out over SSH; keep a second session open and the computer's console within reach.",
		Undo:     "sudo ufw disable (keeps the rules); sudo ufw reset removes them all.",
	}
}

// firewallRules are the allow rules in order (SSH first, then this program, the services it can name, the decoys and the router's
// log) and the listeners it cannot name.
func firewallRules(f FirewallFacts, nets []string) ([]firewallRule, []string) {
	var rules []firewallRule
	fromNets := func(proto string, port int, why string) {
		for _, n := range nets {
			rules = append(rules, firewallRule{from: n, proto: proto, port: port, why: why})
		}
	}
	listening := slices.Clone(f.Listeners)
	slices.SortFunc(listening, func(a, b hostwatch.Listener) int { return a.Port - b.Port })
	ssh := hostwatch.Listener{Proto: "tcp", Port: 22}
	if slices.Contains(listening, ssh) {
		fromNets(ssh.Proto, ssh.Port, knownServices[ssh])
	}
	if f.AppPort > 0 {
		fromNets("tcp", f.AppPort, "this program's own page and API")
	}
	var unknown []string
	for _, l := range listening {
		why, known := knownServices[l]
		switch {
		case l == ssh || isOwnPort(f, l):
		case known:
			fromNets(l.Proto, l.Port, why)
		default:
			unknown = append(unknown, fmt.Sprintf("%d/%s", l.Port, l.Proto))
		}
	}
	for _, p := range f.Decoys {
		fromNets("tcp", p, "a decoy service, which must stay reachable to catch a device probing the network")
	}
	if f.RouterLogPort > 0 {
		if f.Gateway.IsValid() {
			rules = append(rules, firewallRule{from: f.Gateway.String(), proto: "udp", port: f.RouterLogPort, why: "the router's log, from the router only"})
		} else {
			fromNets("udp", f.RouterLogPort, "the router's log (the router's address is not known, so from the network)")
		}
	}
	return rules, unknown
}

// isOwnPort reports whether a listener is one of this program's own sockets, which have their own rules.
func isOwnPort(f FirewallFacts, l hostwatch.Listener) bool {
	switch {
	case l.Proto == "tcp" && (l.Port == f.AppPort || slices.Contains(f.Decoys, l.Port)):
		return true
	case l.Proto == "udp" && f.RouterLogPort > 0 && l.Port == f.RouterLogPort:
		return true
	}
	return false
}

// plainNetworks keeps the declared networks that parse as prefixes, in their canonical form, so nothing else reaches a command.
func plainNetworks(in []string) []string {
	var out []string
	for _, n := range in {
		if p, err := netip.ParsePrefix(strings.TrimSpace(n)); err == nil {
			out = append(out, p.Masked().String())
		}
	}
	return out
}

// HostGuides gives a finding its guide: the firewall plan built from this computer's facts for a switched-off firewall, the fixed
// guide for everything else.
type HostGuides struct {
	// Facts reads the firewall facts at the moment the guide is shown; an error falls back to the fixed guide.
	Facts func() (FirewallFacts, error)
}

// Guide returns the guide of a check.
func (g HostGuides) Guide(check string) (FixGuide, bool) {
	if check == CheckHostFirewallOff && g.Facts != nil {
		if f, err := g.Facts(); err == nil {
			return FirewallGuide(f), true
		}
	}
	return GuideFor(check)
}
