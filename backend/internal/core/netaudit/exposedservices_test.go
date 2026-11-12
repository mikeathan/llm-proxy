package netaudit

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

func exHost(addr string, ports ...Port) Host {
	return Host{Addr: netip.MustParseAddr(addr), Up: true, Ports: ports}
}

func open(n int, service, product, version string) Port {
	return Port{Number: n, Proto: "tcp", State: "open", Service: service, Product: product, Version: version}
}

func TestExposedServices_OnlyConfirmedRemoteAdminFileSharingAndDatabaseServices(t *testing.T) {
	guessed := open(445, "microsoft-ds", "", "")
	guessed.Guessed = true
	wrapped := open(3389, "tcpwrapped", "", "")
	filtered := open(22, "ssh", "OpenSSH", "9.6p1")
	filtered.State = "filtered"
	hosts := []Host{
		exHost("10.0.0.10", open(22, "ssh", "OpenSSH", "9.9p1"), open(139, "netbios-ssn", "Samba smbd", "4.6.2"), open(80, "http", "nginx", "1.24"), guessed, wrapped),
		exHost("10.0.0.2", open(3306, "mysql", "MariaDB", "10.11"), filtered),
		{Addr: netip.MustParseAddr("10.0.0.3"), Up: true, Unscanned: true, Ports: []Port{open(22, "ssh", "", "")}},
	}
	got := ExposedServices(hosts, nil)
	if len(got) != 3 {
		t.Fatalf("want ssh and SMB on .10 and MySQL on .2 only, got %+v", got)
	}
	if got[0].Device != "10.0.0.2" || got[0].Kind != KindDatabase || got[1].Port != 22 || got[1].Kind != KindRemoteAdmin || got[2].Port != 139 || got[2].Kind != KindFileSharing {
		t.Errorf("numeric address order (.2 before .10), then port, with each kind: %+v", got)
	}
	if got[1].Detail != "OpenSSH 9.9p1" {
		t.Errorf("the product and version the scan read: %q", got[1].Detail)
	}
}

func TestExposedServices_AnAcceptedFindingOnAPortTakesItOutButKnownVulnsDoNot(t *testing.T) {
	hosts := []Host{exHost("10.0.0.1", open(22, "ssh", "OpenSSH", "9.6p1"), open(445, "microsoft-ds", "Samba", "4.6.2"))}
	covered := CoveredPorts([]Finding{
		{Device: "10.0.0.1", Port: 22, Proto: "tcp", Check: CheckSSHPasswordLogin},
		{Device: "10.0.0.1", Port: 445, Proto: "tcp", Check: CheckKnownVulns},
	})
	got := ExposedServices(hosts, covered)
	if len(got) != 1 || got[0].Port != 445 {
		t.Errorf("the accepted SSH finding hides port 22; accepting the CVEs of port 445 does not hide the service: %+v", got)
	}
}

func TestExposedServices_AGuessedProductIsNotShown(t *testing.T) {
	p := open(22, "ssh", "Dropbear", "")
	p.ProductGuessed = true
	got := ExposedServices([]Host{exHost("10.0.0.1", p)}, nil)
	if len(got) != 1 || got[0].Detail != "" {
		t.Errorf("a guessed product is left out of the row: %+v", got)
	}
}

func TestRenderObserved_ListsTheExposedServicesOnEveryRunAndEscapesTheirText(t *testing.T) {
	banner := open(21, "ftp", "evil|pipe\nline", "1.0")
	hosts := []Host{exHost("10.0.0.1", banner)}
	exposed := ExposedServices(hosts, nil)
	for name, prev := range map[string]bool{"first scan": false, "later scan": true} {
		o := Observed{ScannedAt: ackNow, Result: Result{Engine: "nmap", Hosts: hosts, Findings: FindingsOf(hosts)}, Exposed: exposed}
		if prev {
			o.Previous = ackNow.Add(-time.Hour)
		}
		out := RenderObserved(o)
		if !strings.Contains(out, "### Exposed services worth reviewing") || !strings.Contains(out, "| 10.0.0.1 | 21/tcp | ftp |") {
			t.Errorf("%s: the exposed services are listed:\n%s", name, out)
		}
		if strings.Contains(out, "evil|pipe") || strings.Contains(out, "pipe\nline") {
			t.Errorf("%s: a banner must not break the table:\n%s", name, out)
		}
	}
	if out := RenderObserved(Observed{ScannedAt: ackNow, Result: Result{Engine: "nmap"}}); strings.Contains(out, "Exposed services") {
		t.Errorf("no exposed service means no section:\n%s", out)
	}
}

func TestRenderObserved_TheExposedListIsBoundedAndSaysSo(t *testing.T) {
	var list []ExposedService
	for i := range maxExposedRows + 5 {
		list = append(list, ExposedService{Device: "10.0.0.1", Port: 1000 + i, Proto: "tcp", Service: "ftp", Kind: KindFileSharing})
	}
	out := RenderObserved(Observed{ScannedAt: ackNow, Result: Result{Engine: "nmap"}, Exposed: list})
	if strings.Count(out, "| 10.0.0.1 |") != maxExposedRows || !strings.Contains(out, "and 5 more not listed") {
		t.Errorf("rows %d:\n%s", strings.Count(out, "| 10.0.0.1 |"), out)
	}
}
