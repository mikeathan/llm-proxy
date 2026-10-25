package nmap

import (
	"errors"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"llm-proxy/internal/core/netaudit"
)

var oneTarget = []netip.Addr{netip.MustParseAddr("192.168.1.5"), netip.MustParseAddr("192.168.1.6")}

// allowedFlags is every flag nmap may be started with. A flag outside this set is a change to review.
var allowedFlags = map[string]bool{
	"-n": true, "--unprivileged": true, "-oX": true, "-T3": true, "--max-retries": true, "--host-timeout": true,
	"--max-rate": true, "-sn": true, "-sT": true, "-Pn": true, "--open": true, "-sV": true, "--version-intensity": true,
	"--top-ports": true, "-p": true,
}

// forbiddenFlags must never appear: they read or write files, run scripts, route through proxies or move the data dir.
var forbiddenFlags = []string{"-iL", "-iR", "--script", "--script-args", "--datadir", "--proxies", "-oN", "-oG", "-oA", "-oS", "--resume", "--excludefile", "-A", "-O", "--traceroute"}

func TestArgs_OnlyTableFlagsAndNoForbiddenOnes(t *testing.T) {
	for _, mode := range netaudit.Modes() {
		for _, ports := range []string{"", "22,80,8000-8100"} {
			args, err := Args(mode, oneTarget, ports)
			if err != nil {
				t.Fatalf("%s: %v", mode, err)
			}
			for _, a := range args {
				if a == "-" || !strings.HasPrefix(a, "-") { // "-" is the value of -oX: stdout
					continue
				}
				if !allowedFlags[a] {
					t.Errorf("%s: flag %q is not in the allowed set", mode, a)
				}
				if slices.Contains(forbiddenFlags, a) {
					t.Errorf("%s: forbidden flag %q", mode, a)
				}
			}
		}
	}
}

func TestArgs_EveryModeIsCoveredByTheTable(t *testing.T) {
	for _, mode := range netaudit.Modes() {
		if _, ok := modeFlags[mode]; !ok {
			t.Errorf("mode %s has no flags in the table", mode)
		}
	}
}

func TestArgs_XMLToStdoutNoDNSNoRawSockets(t *testing.T) {
	args, _ := Args(netaudit.ModePorts, oneTarget, "")
	joined := " " + strings.Join(args, " ") + " "
	for _, want := range []string{" -n ", " --unprivileged ", " -oX - ", " --top-ports 100 ", " -sT "} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q: %s", want, joined)
		}
	}
	if args[len(args)-2] != "192.168.1.5" || args[len(args)-1] != "192.168.1.6" {
		t.Errorf("targets must be the trailing addresses, got %v", args[len(args)-2:])
	}
}

func TestArgs_DiscoverTakesNoPorts(t *testing.T) {
	args, _ := Args(netaudit.ModeDiscover, oneTarget, "22")
	if slices.Contains(args, "-p") || slices.Contains(args, "--top-ports") {
		t.Errorf("discover must not scan ports: %v", args)
	}
}

func TestArgs_RefusesWhatCouldBecomeAFlag(t *testing.T) {
	for _, ports := range []string{"-iL /etc/passwd", "22 --script vuln", "22,-oN x", "1-2-3", ";"} {
		if _, err := Args(netaudit.ModePorts, oneTarget, ports); !errors.Is(err, netaudit.ErrBadPorts) {
			t.Errorf("ports %q: err = %v, want ErrBadPorts", ports, err)
		}
	}
	if _, err := Args("vuln", oneTarget, ""); !errors.Is(err, netaudit.ErrBadMode) {
		t.Errorf("unknown mode: %v", err)
	}
	if _, err := Args(netaudit.ModePorts, nil, ""); err == nil {
		t.Error("no targets must be refused")
	}
	if _, err := Args(netaudit.ModePorts, []netip.Addr{netip.MustParseAddr("fd00::1")}, ""); err == nil {
		t.Error("an IPv6 target must be refused")
	}
}
