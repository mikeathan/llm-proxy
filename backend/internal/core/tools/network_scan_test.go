package tools

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

func scanTestTools(cfg models.NetworkGuardrailsConfig) *NetworkTools {
	return NewNetworkTools(func(ctx context.Context) models.NetworkGuardrailsConfig { return cfg }, &mockLogger{})
}

func drain(jobs chan string) []string {
	close(jobs)
	var got []string
	for j := range jobs {
		got = append(got, j)
	}
	return got
}

func TestPortList_AcceptsArraysCommaTextAndRejectsTheRest(t *testing.T) {
	cases := []struct {
		in      string
		want    []int
		wantErr bool
	}{
		{`[22,80]`, []int{22, 80}, false},
		{`"22, 80,443"`, []int{22, 80, 443}, false},
		{`"22,abc,80"`, []int{22, 80}, false}, // a bad entry is skipped, the good ones stay
		{`[]`, []int{}, false},
		{`{"a":1}`, nil, true},
		{`7`, nil, true},
	}
	for _, c := range cases {
		var p PortList
		err := json.Unmarshal([]byte(c.in), &p)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", c.in, err, c.wantErr)
			continue
		}
		if !c.wantErr && !equalInts(p, c.want) {
			t.Errorf("%s: got %v, want %v", c.in, []int(p), c.want)
		}
	}
	var p PortList
	if err := p.UnmarshalJSON(nil); err != nil || p != nil {
		t.Errorf("empty input must be a no-op, got %v, %v", p, err)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestResolveAndQueueTargets_QueuesAllowedHostsAndSubnetsOnly(t *testing.T) {
	n := scanTestTools(models.NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true, BlockedIPs: []string{"192.168.1.3"}})
	jobs := make(chan string, 64)

	n.resolveAndQueueTargets(context.Background(), " 192.168.1.9 ,, 10.0.0.0/30, 8.8.8.8, 127.0.0.1, 192.168.1.0/xx", n.configProvider(context.Background()), jobs)
	got := drain(jobs)

	// the host, then the /30 without its network address; the internet, loopback and unparsable entries are dropped
	want := []string{"192.168.1.9", "10.0.0.1", "10.0.0.2", "10.0.0.3"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("queued %v, want %v", got, want)
	}
}

func TestResolveAndQueueTargets_ASubnetIsCappedAndBlockedIPsAreSkipped(t *testing.T) {
	n := scanTestTools(models.NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true, BlockedIPs: []string{"10.0.0.5"}})
	jobs := make(chan string, 4096)
	n.resolveAndQueueTargets(context.Background(), "10.0.0.0/16", n.configProvider(context.Background()), jobs)
	got := drain(jobs)
	if len(got) < 1000 || len(got) > 1025 {
		t.Errorf("a /16 must be capped near 1024 addresses, queued %d", len(got))
	}
	for _, ip := range got {
		if ip == "10.0.0.5" {
			t.Error("a blocked address was queued")
		}
	}
}

func TestResolveAndQueueTargets_StopsWhenTheContextEnds(t *testing.T) {
	n := scanTestTools(models.NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	jobs := make(chan string) // unbuffered and never read: only the context can end the send
	done := make(chan struct{})
	go func() {
		n.resolveAndQueueTargets(ctx, "192.168.1.1,192.168.1.2,10.0.0.0/24", n.configProvider(ctx), jobs)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("resolveAndQueueTargets kept blocking after the context ended")
	}
}

func TestScanLocalNetwork_RefusesWhenLanIsBlockedOrTheTargetIsBad(t *testing.T) {
	blocked := scanTestTools(models.NetworkGuardrailsConfig{Enabled: true})
	if _, err := blocked.ScanLocalNetwork(context.Background(), ScanArgs{Targets: "192.168.1.1"}); err == nil {
		t.Error("a scan with LAN access blocked must fail")
	}
	open := scanTestTools(models.NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true})
	if _, err := open.ScanLocalNetwork(context.Background(), ScanArgs{Targets: "10.0.0.0/99"}); err == nil {
		t.Error("an invalid target must fail")
	}
}

func TestScanLocalNetwork_NothingFoundSaysSo(t *testing.T) {
	n := scanTestTools(models.NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true})
	out, err := n.ScanLocalNetwork(context.Background(), ScanArgs{IP: "192.0.2.1", Ports: PortList{1}, TimeoutMs: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Network Scan () of 192.0.2.1 completed") || !strings.Contains(out, "No active services identified.") {
		t.Errorf("unexpected report: %q", out)
	}
}

func TestScanTargetOf_PrefersTheCanonicalFieldThenTheAliases(t *testing.T) {
	cases := []struct {
		args ScanArgs
		want string
	}{
		{ScanArgs{Targets: "a", Target: "b", IP: "c", CIDR: "d"}, "a"},
		{ScanArgs{Target: "b", IP: "c", CIDR: "d"}, "b"},
		{ScanArgs{IP: "c", CIDR: "d"}, "c"},
		{ScanArgs{CIDR: "d"}, "d"},
		{ScanArgs{}, ""},
	}
	for _, c := range cases {
		if got := scanTargetOf(c.args); got != c.want {
			t.Errorf("%+v: got %q, want %q", c.args, got, c.want)
		}
	}
}

func TestScanPortsAndTimeout(t *testing.T) {
	if got := scanPorts(ScanArgs{Ports: PortList{9, 10}}); !equalInts(got, []int{9, 10}) {
		t.Errorf("explicit ports: %v", got)
	}
	if got := scanPorts(ScanArgs{}); len(got) != 5 {
		t.Errorf("discovery mode scans 5 ports, got %v", got)
	}
	if got := scanPorts(ScanArgs{Mode: "deep"}); len(got) != 20 {
		t.Errorf("deep mode scans 20 ports, got %v", got)
	}
	if got := scanTimeout(ScanArgs{}); got != 500*time.Millisecond {
		t.Errorf("default timeout %v", got)
	}
	if got := scanTimeout(ScanArgs{TimeoutMs: 40, Timeout: 90}); got != 40*time.Millisecond {
		t.Errorf("timeout_ms wins: %v", got)
	}
	if got := scanTimeout(ScanArgs{Timeout: 90}); got != 90*time.Millisecond {
		t.Errorf("alias timeout: %v", got)
	}
}

func TestServiceBanner_TrimsFlattensAndShortens(t *testing.T) {
	if got := serviceBanner([]byte("SSH-2.0-OpenSSH\nsecond line\n")); got != "SSH-2.0-OpenSSH second line" {
		t.Errorf("got %q", got)
	}
	long := serviceBanner([]byte(strings.Repeat("x", 200)))
	if len(long) != 64 || !strings.HasSuffix(long, "...") {
		t.Errorf("a long banner is cut to 64 characters: %q (%d)", long, len(long))
	}
}

func TestProbePort_ReadsABannerOrReportsNothingWhenClosed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no loopback listener")
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte("HELLO banner\n"))
			c.Close()
		}
	}()
	host, portText, _ := net.SplitHostPort(ln.Addr().String())
	port := 0
	for _, ch := range portText {
		port = port*10 + int(ch-'0')
	}

	n := scanTestTools(models.NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true})
	plan := scanPlan{timeout: 300 * time.Millisecond, dialer: &net.Dialer{Timeout: 300 * time.Millisecond}}
	if info, ok := n.probePort(context.Background(), plan, host, port); !ok || info != "HELLO banner" {
		t.Errorf("open port: %q, %v", info, ok)
	}
	ln.Close()
	if _, ok := n.probePort(context.Background(), plan, host, port); ok {
		t.Error("a closed port must report nothing")
	}
}

func bannerListener(t *testing.T, banner string) (host string, port int, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no loopback listener")
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte(banner))
			c.Close()
		}
	}()
	host, portText, _ := net.SplitHostPort(ln.Addr().String())
	for _, ch := range portText {
		port = port*10 + int(ch-'0')
	}
	return host, port, func() { ln.Close() }
}

func runWorker(t *testing.T, ctx context.Context, plan scanPlan, ips ...string) []string {
	t.Helper()
	n := scanTestTools(models.NetworkGuardrailsConfig{Enabled: true, AllowLanAccess: true})
	jobs := make(chan string, len(ips))
	for _, ip := range ips {
		jobs <- ip
	}
	close(jobs)
	results := make(chan string, 64)
	n.scanWorker(ctx, plan, jobs, results)
	close(results)
	var got []string
	for r := range results {
		got = append(got, r)
	}
	return got
}

func TestScanWorker_DiscoveryReportsTheFirstOpenPortDeepReportsAll(t *testing.T) {
	host, portA, stopA := bannerListener(t, "A-service\n")
	_, portB, stopB := bannerListener(t, "B-service\n")
	defer stopA()
	defer stopB()
	plan := scanPlan{ports: []int{portA, portB}, timeout: 300 * time.Millisecond, dialer: &net.Dialer{Timeout: 300 * time.Millisecond}}

	got := runWorker(t, context.Background(), plan, host)
	if len(got) != 1 || !strings.Contains(got[0], "A-service") {
		t.Errorf("discovery mode: %v", got)
	}

	plan.deep = true
	got = runWorker(t, context.Background(), plan, host)
	if len(got) != 2 || !strings.Contains(got[1], "B-service") {
		t.Errorf("deep mode: %v", got)
	}
}

func TestScanWorker_StopsWhenTheContextEnds(t *testing.T) {
	host, port, stop := bannerListener(t, "x\n")
	defer stop()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	plan := scanPlan{ports: []int{port}, timeout: 200 * time.Millisecond, dialer: &net.Dialer{Timeout: 200 * time.Millisecond}}
	if got := runWorker(t, ctx, plan, host, host); len(got) != 0 {
		t.Errorf("a cancelled scan reported %v", got)
	}
}
