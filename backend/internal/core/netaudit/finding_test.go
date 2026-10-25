package netaudit

import (
	"net/netip"
	"testing"
)

func TestFindingsOf_OneFindingPerOpenPort(t *testing.T) {
	hosts := []Host{
		{Addr: netip.MustParseAddr("192.168.1.5"), Up: true, Ports: []Port{
			{Number: 22, Proto: "tcp", State: "open", Service: "ssh", Product: "OpenSSH", Version: "8.9"},
			{Number: 23, Proto: "tcp", State: "closed"},
			{Number: 80, Proto: "tcp", State: "open", Service: "http"},
		}},
		{Addr: netip.MustParseAddr("192.168.1.6"), Up: true},
	}
	got := FindingsOf(hosts)
	if len(got) != 2 {
		t.Fatalf("findings = %+v", got)
	}
	if got[0].Key() != "192.168.1.5|tcp|22|open_port" || got[0].Version != "OpenSSH 8.9" || got[0].Severity != SeverityInfo {
		t.Errorf("first finding = %+v", got[0])
	}
	if got[1].Version != "" {
		t.Errorf("no product and no version must give an empty version, got %q", got[1].Version)
	}
}

func TestFinding_KeyIgnoresWording(t *testing.T) {
	a := Finding{Device: "10.0.0.1", Port: 22, Proto: "tcp", Check: CheckOpenPort, Evidence: "x"}
	b := Finding{Device: "10.0.0.1", Port: 22, Proto: "tcp", Check: CheckOpenPort, Evidence: "y", Version: "9"}
	if a.Key() != b.Key() {
		t.Error("the key must identify what a finding is about, not how it is worded")
	}
}
