package nmap

import (
	"errors"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"llm-proxy/internal/core/netaudit"
)

// req builds a scan request for the two test targets.
func req(mode netaudit.Mode, ports string, thorough, prefiltered bool) netaudit.ScanRequest {
	return netaudit.ScanRequest{Targets: oneTarget, Mode: mode, Ports: ports, Thorough: thorough, Prefiltered: prefiltered}
}

var oneTarget = []netip.Addr{netip.MustParseAddr("192.168.1.5"), netip.MustParseAddr("192.168.1.6")}

// allowedFlags is every flag nmap may be started with. A flag outside this set is a change to review.
var allowedFlags = map[string]bool{
	"-n": true, "--unprivileged": true, "-oX": true, "-T3": true, "--max-retries": true, "--host-timeout": true,
	"--max-rate": true, "-sn": true, "-sT": true, "-Pn": true, "--open": true, "-sV": true, "--version-intensity": true,
	"--top-ports": true, "-p": true, "-T4": true, "-PS22,80,443,445,5000,8080": true,
}

// forbiddenFlags must never appear: they read or write files, run scripts, route through proxies or move the data dir.
var forbiddenFlags = []string{"-iL", "-iR", "--script", "--script-args", "--datadir", "--proxies", "-oN", "-oG", "-oA", "-oS", "--resume", "--excludefile", "-A", "-O", "--traceroute"}

func TestArgs_OnlyTableFlagsAndNoForbiddenOnes(t *testing.T) {
	for _, mode := range netaudit.Modes() {
		for _, ports := range []string{"", "22,80,8000-8100"} {
			for _, thorough := range []bool{false, true} {
				args, err := Args(req(mode, ports, thorough, false))
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
}

func TestArgs_EveryModeIsCoveredByTheTable(t *testing.T) {
	for _, mode := range netaudit.Modes() {
		if _, ok := modeFlags[mode]; !ok {
			t.Errorf("mode %s has no flags in the table", mode)
		}
	}
}

func TestArgs_XMLToStdoutNoDNSNoRawSockets(t *testing.T) {
	for _, thorough := range []bool{false, true} {
		args, _ := Args(req(netaudit.ModePorts, "", thorough, false))
		joined := " " + strings.Join(args, " ") + " "
		for _, want := range []string{" -n ", " --unprivileged ", " -oX - ", " -sT "} {
			if !strings.Contains(joined, want) {
				t.Errorf("thorough=%v: args missing %q: %s", thorough, want, joined)
			}
		}
		if args[len(args)-2] != "192.168.1.5" || args[len(args)-1] != "192.168.1.6" {
			t.Errorf("targets must be the trailing addresses, got %v", args[len(args)-2:])
		}
	}
}

// A quick scan of addresses the service already swept skips nmap's own liveness check and scans the common ports; a
// thorough one scans every address whether or not it answered, on the top ports, more gently; only a quick scan that
// was not swept lets nmap look for live addresses itself.
func TestArgs_WhoDecidesWhichAddressesAreLive(t *testing.T) {
	prefiltered, _ := Args(req(netaudit.ModePorts, "", false, true))
	if !slices.Contains(prefiltered, "-Pn") || !slices.Contains(prefiltered, "-T4") || slices.Contains(prefiltered, "-PS22,80,443,445,5000,8080") {
		t.Errorf("prefiltered: %v", prefiltered)
	}
	if i := slices.Index(prefiltered, "-p"); i < 0 || prefiltered[i+1] != "21,22,23,25,53,80,110,139,143,443,445,515,631,1883,3306,3389,5000,5353,5432,5900,8000,8080,8443,9100" {
		t.Errorf("quick must scan the shared default ports: %v", prefiltered)
	}

	unswept, _ := Args(req(netaudit.ModePorts, "", false, false))
	if slices.Contains(unswept, "-Pn") || !slices.Contains(unswept, "-PS22,80,443,445,5000,8080") {
		t.Errorf("an unswept quick scan lets nmap find live hosts: %v", unswept)
	}

	slow, _ := Args(req(netaudit.ModePorts, "", true, false))
	if !slices.Contains(slow, "-Pn") || !slices.Contains(slow, "--top-ports") || !slices.Contains(slow, "-T3") {
		t.Errorf("thorough: %v", slow)
	}
	if i := slices.Index(slow, "--max-rate"); slow[i+1] != "300" {
		t.Errorf("thorough must stay gentle: %v", slow)
	}
}

func TestArgs_DiscoverUsesTheSharedLivenessPortsInBothProfiles(t *testing.T) {
	for _, thorough := range []bool{false, true} {
		args, _ := Args(req(netaudit.ModeDiscover, "", thorough, false))
		if !slices.Contains(args, "-sn") || !slices.Contains(args, "-PS22,80,443,445,5000,8080") || slices.Contains(args, "-Pn") {
			t.Errorf("thorough=%v: %v", thorough, args)
		}
	}
}

func TestArgs_DiscoverTakesNoPorts(t *testing.T) {
	args, _ := Args(req(netaudit.ModeDiscover, "22", false, false))
	if slices.Contains(args, "-p") || slices.Contains(args, "--top-ports") {
		t.Errorf("discover must not scan ports: %v", args)
	}
}

func TestArgs_RefusesWhatCouldBecomeAFlag(t *testing.T) {
	for _, ports := range []string{"-iL /etc/passwd", "22 --script vuln", "22,-oN x", "1-2-3", ";"} {
		if _, err := Args(req(netaudit.ModePorts, ports, false, false)); !errors.Is(err, netaudit.ErrBadPorts) {
			t.Errorf("ports %q: err = %v, want ErrBadPorts", ports, err)
		}
	}
	if _, err := Args(req("vuln", "", false, false)); !errors.Is(err, netaudit.ErrBadMode) {
		t.Errorf("unknown mode: %v", err)
	}
	if _, err := Args(netaudit.ScanRequest{Mode: netaudit.ModePorts}); err == nil {
		t.Error("no targets must be refused")
	}
	if _, err := Args(netaudit.ScanRequest{Mode: netaudit.ModePorts, Targets: []netip.Addr{netip.MustParseAddr("fd00::1")}}); err == nil {
		t.Error("an IPv6 target must be refused")
	}
}
