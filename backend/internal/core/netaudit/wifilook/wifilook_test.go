package wifilook

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"llm-proxy/internal/platform/progexec"
)

// The sample is the owner's real output (vertex, 2026-11-06), with the neighbours' names and hardware addresses replaced.
const sample = `home:AA\:BB\:CC\:00\:00\:01:55:WPA2
home-Guest:92\:BB\:CC\:00\:00\:02:50:WPA2 WPA3
home-2G:AA\:BB\:CC\:00\:00\:03:49:WPA2
:86\:00\:00\:00\:00\:05:47:WPA2
Free Cafe:62\:00\:00\:00\:00\:06:42:
Office-X:7A\:00\:00\:00\:00\:07:24:WPA2 802.1X
Old Router:AA\:00\:00\:00\:00\:08:30:WEP
Mixed:AA\:00\:00\:00\:00\:09:61:WPA1 WPA2
`

func TestParse_ReadsTheRealShapeOfNmclisTerseOutput(t *testing.T) {
	got := Parse([]byte(sample))
	if len(got) != 8 {
		t.Fatalf("parsed %d access points, want 8: %+v", len(got), got)
	}
	want := []AP{
		{SSID: "home", BSSID: "aa:bb:cc:00:00:01", Signal: 55, Security: WPA2},
		{SSID: "home-Guest", BSSID: "92:bb:cc:00:00:02", Signal: 50, Security: WPA3},
		{SSID: "home-2G", BSSID: "aa:bb:cc:00:00:03", Signal: 49, Security: WPA2},
		{SSID: "", BSSID: "86:00:00:00:00:05", Signal: 47, Security: WPA2},
		{SSID: "Free Cafe", BSSID: "62:00:00:00:00:06", Signal: 42, Security: Open},
		{SSID: "Office-X", BSSID: "7a:00:00:00:00:07", Signal: 24, Security: WPA2},
		{SSID: "Old Router", BSSID: "aa:00:00:00:00:08", Signal: 30, Security: WEP},
		{SSID: "Mixed", BSSID: "aa:00:00:00:00:09", Signal: 61, Security: WPA2},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParse_AnEscapedColonInANameIsPartOfTheName(t *testing.T) {
	got := Parse([]byte(`a\:b\\c:AA\:BB\:CC\:00\:00\:01:10:WPA2` + "\n"))
	if len(got) != 1 || got[0].SSID != `a:b\c` {
		t.Fatalf("got %+v", got)
	}
}

// Anyone within radio range chooses these bytes.
func TestParse_TreatsEveryRowAsUntrusted(t *testing.T) {
	long := strings.Repeat("x", 33)
	for name, line := range map[string]string{
		"too few fields":                     `home:AA\:BB\:CC\:00\:00\:01:55`,
		"too many fields":                    `home:AA\:BB\:CC\:00\:00\:01:55:WPA2:extra`,
		"a hardware address that is not one": `home:ZZ\:BB\:CC\:00\:00\:01:55:WPA2`,
		"a short hardware address":           `home:AA\:BB\:CC:55:WPA2`,
		"no signal":                          `home:AA\:BB\:CC\:00\:00\:01::WPA2`,
		"a signal out of range":              `home:AA\:BB\:CC\:00\:00\:01:900:WPA2`,
		"empty":                              ``,
	} {
		if got := Parse([]byte(line + "\n")); len(got) != 0 {
			t.Errorf("%s: parsed %+v, want the row dropped", name, got)
		}
	}
	// A name that cannot be a real one is kept as a hidden network (empty name) so it can never match the operator's.
	for name, ssid := range map[string]string{"longer than 32 bytes": long, "control character": "a\x01b", "newline escape": "a\x7fb", "invalid UTF-8": "a\xffb"} {
		got := Parse([]byte(ssid + `:AA\:BB\:CC\:00\:00\:01:55:WPA2` + "\n"))
		if len(got) != 1 || got[0].SSID != "" {
			t.Errorf("%s: got %+v, want a hidden network", name, got)
		}
	}
}

func TestParse_IsBounded(t *testing.T) {
	var b strings.Builder
	for i := 0; i < MaxAPs+300; i++ {
		b.WriteString("n:AA\\:BB\\:CC\\:00\\:00\\:01:10:WPA2\n")
	}
	if got := Parse([]byte(b.String())); len(got) != MaxAPs {
		t.Fatalf("parsed %d, want the bound of %d", len(got), MaxAPs)
	}
}

func TestSecurity_RanksTheWeakestFirst(t *testing.T) {
	if !(Open.Rank() < WEP.Rank() && WEP.Rank() < WPA.Rank() && WPA.Rank() < WPA2.Rank() && WPA2.Rank() < WPA3.Rank()) {
		t.Fatal("ranks must rise from open to WPA3")
	}
}

type fakeRunner struct {
	out  string
	err  error
	got  progexec.Invocation
	prog progexec.Program
}

func (f *fakeRunner) Run(_ context.Context, p progexec.Program, inv progexec.Invocation) (progexec.Output, error) {
	f.prog, f.got = p, inv
	return progexec.Output{Stdout: []byte(f.out)}, f.err
}

// What the guarded run is allowed: fixed arguments written here, a clean environment, no TCP, Unix sockets (D-Bus) and nothing a
// model, a device or an operator can shape.
func TestList_RunsNmcliWithFixedArgumentsNoNetworkAndOnlyUnixSockets(t *testing.T) {
	r := &fakeRunner{out: sample}
	aps, err := New(r).List(context.Background())
	if err != nil || len(aps) != 8 {
		t.Fatalf("aps = %d err = %v", len(aps), err)
	}
	inv := r.got
	if r.prog.Binary != "nmcli" {
		t.Errorf("program = %q", r.prog.Binary)
	}
	if got := strings.Join(inv.Args, " "); got != "-t -f SSID,BSSID,SIGNAL,SECURITY device wifi list --rescan no" {
		t.Errorf("args = %q: they are fixed, read-only, and never ask NetworkManager to scan", got)
	}
	if !inv.NoNetwork || !inv.AllowUnixSockets {
		t.Errorf("invocation = %+v, want TCP denied and Unix sockets allowed", inv)
	}
	if inv.Env == nil || len(inv.ReadPaths) != 0 || inv.Timeout <= 0 || inv.MaxOutput <= 0 {
		t.Errorf("invocation = %+v: a clean environment, no extra read path, a time and an output limit", inv)
	}
	for _, kv := range inv.Env {
		if strings.HasPrefix(kv, "DBUS") || strings.HasPrefix(kv, "NM") {
			t.Errorf("environment holds %q: nothing may steer where nmcli connects", kv)
		}
	}
	if _, err := os.Stat(inv.Root); err == nil {
		t.Errorf("the private run folder %s was left behind", inv.Root)
	}
}

func TestList_APassThatFailsIsAnErrorNeverAnEmptyList(t *testing.T) {
	r := &fakeRunner{err: errors.New("nmcli failed: exit status 1: NetworkManager is not running")}
	aps, err := New(r).List(context.Background())
	if err == nil || aps != nil {
		t.Fatalf("aps = %v err = %v: a failed look must not read as 'no networks nearby'", aps, err)
	}
}
