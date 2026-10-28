package nmap

import (
	"errors"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

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
	"--max-rate": true, "-sn": true, "-sT": true, "-Pn": true, "-sV": true, "--version-intensity": true,
	"--top-ports": true, "-p": true, "-T4": true, "--script": true, "--script-timeout": true, "--min-hostgroup": true, "-PS22,80,443,445,5000,8080": true,
}

// forbiddenFlags must never appear: they read or write files, run scripts, route through proxies or move the data dir.
var forbiddenFlags = []string{"-iL", "-iR", "--script-args", "--script-updatedb", "--script-trace", "--script-help", "--datadir", "--proxies", "-oN", "-oG", "-oA", "-oS", "--resume", "--excludefile", "-A", "-O", "--traceroute"}

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
	if i := slices.Index(prefiltered, "-p"); i < 0 || prefiltered[i+1] != "21,22,23,25,53,80,110,139,143,443,445,515,631,1883,3306,3389,5000,5353,5432,5900,8000,8080,8443,9100,2375,11211,27017" {
		t.Errorf("quick must scan the shared default ports: %v", prefiltered)
	}

	unswept, _ := Args(req(netaudit.ModePorts, "", false, false))
	if slices.Contains(unswept, "-Pn") || !slices.Contains(unswept, "-PS22,80,443,445,5000,8080") {
		t.Errorf("an unswept quick scan lets nmap find live hosts: %v", unswept)
	}

	slow, _ := Args(req(netaudit.ModePorts, "", true, false))
	if !slices.Contains(slow, "-Pn") || !slices.Contains(slow, "-T3") {
		t.Errorf("thorough: %v", slow)
	}
	// nmap ignores --top-ports when -p is also given, so the list is explicit; the ports the checks need must be on it.
	if slices.Contains(slow, "--top-ports") {
		t.Errorf("thorough must not rely on --top-ports: %v", slow)
	}
	i := slices.Index(slow, "-p")
	if i < 0 {
		t.Fatalf("thorough names its ports: %v", slow)
	}
	scanned := strings.Split(slow[i+1], ",")
	for _, need := range []string{"22", "23", "445", "2375", "11211", "27017"} {
		if !slices.Contains(scanned, need) {
			t.Errorf("thorough must scan port %s: %v", need, scanned)
		}
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

func hostTimeoutOf(t *testing.T, r netaudit.ScanRequest) string {
	t.Helper()
	args, err := Args(r)
	if err != nil {
		t.Fatal(err)
	}
	return args[slices.Index(args, "--host-timeout")+1]
}

func TestArgs_PerDeviceTimeIsTheDepthsStandardUnlessTheOperatorOrARetryChangesIt(t *testing.T) {
	quickReq, thoroughReq := req(netaudit.ModePorts, "", false, false), req(netaudit.ModePorts, "", true, false)
	if got := hostTimeoutOf(t, quickReq); got != "60s" {
		t.Errorf("quick standard = %s, want 60s", got)
	}
	if got := hostTimeoutOf(t, thoroughReq); got != "180s" {
		t.Errorf("thorough standard = %s, want 180s", got)
	}
	thoroughReq.HostTimeout = 5 * time.Minute
	if got := hostTimeoutOf(t, thoroughReq); got != "300s" {
		t.Errorf("the operator's choice replaces the standard: %s", got)
	}
	thoroughReq.HostTimeoutFactor = 2
	if got := hostTimeoutOf(t, thoroughReq); got != "600s" {
		t.Errorf("a retry doubles whichever applies: %s", got)
	}
	quickReq.HostTimeoutFactor = 2
	if got := hostTimeoutOf(t, quickReq); got != "120s" {
		t.Errorf("a retry doubles the standard too: %s", got)
	}
}

func TestRunLimit_NeverLongerThanTheEnginesOwn(t *testing.T) {
	if runLimit(netaudit.ScanRequest{}) != scanTimeout || runLimit(netaudit.ScanRequest{Timeout: time.Hour}) != scanTimeout {
		t.Error("the engine's limit is the ceiling")
	}
	if got := runLimit(netaudit.ScanRequest{Timeout: 2 * time.Minute}); got != 2*time.Minute {
		t.Errorf("a shorter caller bound applies, got %v", got)
	}
}

func TestArgs_OnlyAChecksScanRunsScriptsAndOnlyTheReviewedOnes(t *testing.T) {
	for _, mode := range netaudit.Modes() {
		args, err := Args(req(mode, "", true, false))
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		i := slices.Index(args, "--script")
		if mode != netaudit.ModeChecks {
			if i >= 0 {
				t.Errorf("%s must not run scripts: %v", mode, args)
			}
			continue
		}
		if i < 0 || args[i+1] != strings.Join(netaudit.CheckScriptNames(), ",") {
			t.Fatalf("checks must run exactly the reviewed list, got %v", args)
		}
		if j := slices.Index(args, "--script-timeout"); j < 0 || args[j+1] != "30s" {
			t.Errorf("each script is time-limited: %v", args)
		}
		if !slices.Contains(args, "-sV") {
			t.Errorf("scripts pick their ports by service, so checks include service detection: %v", args)
		}
	}
}

func TestArgs_NoScriptNameEverComesFromTheRequest(t *testing.T) {
	r := req(netaudit.ModeChecks, "22,80", false, false)
	args, _ := Args(r)
	for _, bad := range []string{"--script=ftp-brute", "ftp-brute", "default", "vuln", "all"} {
		if slices.Contains(args, bad) {
			t.Errorf("%q must never appear: %v", bad, args)
		}
	}
	if _, err := Args(req(netaudit.ModeChecks, "22;--script=ftp-brute", false, false)); err == nil {
		t.Error("a port list that could become a flag must still be refused in checks mode")
	}
}

func TestArgs_AllTheLiveDevicesAreScannedTogetherNotInRoundsOfFive(t *testing.T) {
	many := make([]netip.Addr, 0, 100)
	for i := 1; i <= 100; i++ {
		many = append(many, netip.AddrFrom4([4]byte{192, 168, 50, byte(i)}))
	}
	value := func(args []string) string { return args[slices.Index(args, "--min-hostgroup")+1] }
	args, err := Args(netaudit.ScanRequest{Targets: many[:8], Mode: netaudit.ModeServices, Prefiltered: true})
	if err != nil || value(args) != "8" {
		t.Fatalf("eight live devices go in one group of eight: %v %v", value(args), err)
	}
	args, _ = Args(netaudit.ScanRequest{Targets: many, Mode: netaudit.ModeServices, Prefiltered: true})
	if value(args) != "64" {
		t.Errorf("a large group is capped at 64: %s", value(args))
	}
	args, _ = Args(netaudit.ScanRequest{Targets: many[:1], Mode: netaudit.ModeServices, Prefiltered: true})
	if slices.Contains(args, "--min-hostgroup") {
		t.Errorf("one device needs no grouping: %v", args)
	}
	args, _ = Args(netaudit.ScanRequest{Targets: many[:8], Mode: netaudit.ModeDiscover})
	if !slices.Contains(args, "--min-hostgroup") {
		t.Errorf("a discover scan is grouped too: %v", args)
	}
}

func TestArgs_ChecksGetMoreTimePerDeviceBecauseScriptsRunInsideIt(t *testing.T) {
	thorough := req(netaudit.ModeServices, "", true, false)
	checks := req(netaudit.ModeChecks, "", true, false)
	if got := hostTimeoutOf(t, thorough); got != "180s" {
		t.Errorf("services standard = %s", got)
	}
	if got := hostTimeoutOf(t, checks); got != "300s" {
		t.Errorf("checks standard = %s, want 300s (180s + 120s for the scripts)", got)
	}
	checks.HostTimeout = 5 * time.Minute
	if got := hostTimeoutOf(t, checks); got != "300s" {
		t.Errorf("the operator's choice is used as given: %s", got)
	}
}

func TestArgs_DoNotHideDevicesThatHaveNothingOpen(t *testing.T) {
	for _, mode := range netaudit.Modes() {
		args, _ := Args(req(mode, "", true, false))
		if slices.Contains(args, "--open") {
			t.Errorf("%s: --open makes nmap leave a device with no open ports out of its output, and the audit would call it unscanned: %v", mode, args)
		}
	}
}
