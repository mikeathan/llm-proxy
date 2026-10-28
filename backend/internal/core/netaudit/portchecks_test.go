package netaudit

import (
	"net/netip"
	"strings"
	"testing"
)

func servicePort(number int, service string, guessed bool) Port {
	return Port{Number: number, Proto: "tcp", State: "open", Service: service, Guessed: guessed}
}

func portChecks(t *testing.T, p Port) []Finding {
	t.Helper()
	var out []Finding
	for _, f := range FindingsOf([]Host{{Addr: netip.MustParseAddr("10.0.0.5"), Up: true, Ports: []Port{p}}}) {
		if f.Check != CheckOpenPort {
			out = append(out, f)
		}
	}
	return out
}

func TestPortCheck_TelnetOpen(t *testing.T) {
	confirmed := portChecks(t, servicePort(23, "telnet", false))
	guessed := portChecks(t, servicePort(23, "telnet", true))
	for _, got := range [][]Finding{confirmed, guessed} {
		if len(got) != 1 || got[0].Check != CheckTelnetOpen || got[0].Severity != SeverityHigh {
			t.Fatalf("port 23 = %+v, want one %s/high", got, CheckTelnetOpen)
		}
	}
	if !strings.Contains(guessed[0].Evidence, "normally") || strings.Contains(confirmed[0].Evidence, "normally") {
		t.Errorf("a guessed service must say so, a confirmed one must not:\n%q\n%q", guessed[0].Evidence, confirmed[0].Evidence)
	}
	if got := portChecks(t, servicePort(2323, "telnet", false)); len(got) != 1 {
		t.Errorf("a service nmap identified as Telnet on another port is still Telnet: %+v", got)
	}
}

func TestPortCheck_DockerAPIRanksByHowSureTheScanIs(t *testing.T) {
	sure := portChecks(t, servicePort(2375, "docker", false))
	port := portChecks(t, servicePort(2375, "docker", true))
	if len(sure) != 1 || sure[0].Check != CheckDockerAPI || sure[0].Severity != SeverityCritical {
		t.Errorf("a confirmed Docker API is critical: %+v", sure)
	}
	if len(port) != 1 || port[0].Severity != SeverityHigh {
		t.Errorf("an open 2375 that was only named from the port table is high: %+v", port)
	}
}

func TestPortCheck_OtherPortsAreQuiet(t *testing.T) {
	for _, p := range []Port{servicePort(22, "ssh", false), servicePort(80, "http", true), servicePort(8081, "blackice-icecap", true), servicePort(2376, "docker-s", false)} {
		if got := portChecks(t, p); len(got) != 0 {
			t.Errorf("%d/%s must not raise a port check: %+v", p.Number, p.Service, got)
		}
	}
}

func TestPortCheck_ClosedPortsAreIgnored(t *testing.T) {
	p := servicePort(23, "telnet", false)
	p.State = "closed"
	if got := portChecks(t, p); len(got) != 0 {
		t.Errorf("a closed port is not a finding: %+v", got)
	}
}
