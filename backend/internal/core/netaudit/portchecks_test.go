package netaudit

import (
	"net/netip"
	"slices"
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

func TestPortCheck_TelnetOnTheAlternatePort(t *testing.T) {
	got := portChecks(t, servicePort(2323, "", true))
	if len(got) != 1 || got[0].Check != CheckTelnetOpen || !strings.Contains(got[0].Evidence, "2323") {
		t.Fatalf("an open 2323 is the usual alternate Telnet port: %+v", got)
	}
}

// Each of these is a service whose danger is that it listens at all. A rule fires on the port alone, ranked lower, and
// ranks higher once the scan confirmed the service.
func TestPortCheck_ListeningIsTheProblem(t *testing.T) {
	product := func(number int, service, product string) Port {
		p := servicePort(number, service, false)
		p.Product = product
		return p
	}
	cases := []struct {
		name       string
		check      string
		portOnly   Port
		portSev    string
		confirmed  Port
		confirmSev string
	}{
		{"adb", CheckADBExposed, servicePort(5555, "freeciv", true), SeverityHigh, servicePort(5555, "adb", false), SeverityCritical},
		{"proxy socks", CheckOpenProxy, servicePort(1080, "socks", true), SeverityMedium, servicePort(1080, "socks5", false), SeverityHigh},
		{"proxy squid", CheckOpenProxy, servicePort(3128, "squid-http", true), SeverityMedium, servicePort(8888, "http-proxy", false), SeverityHigh},
		{"elasticsearch", CheckElasticsearchExposed, servicePort(9200, "wap-wsp", true), SeverityMedium, product(9200, "http", "Elasticsearch REST API"), SeverityHigh},
		{"x11", CheckX11Exposed, servicePort(6000, "X11", true), SeverityHigh, servicePort(6000, "x11", false), SeverityCritical},
		{"rsync", CheckRsyncExposed, servicePort(873, "rsync", true), SeverityMedium, servicePort(873, "rsync", false), SeverityHigh},
		{"kubelet", CheckKubeletExposed, servicePort(10250, "unknown", true), SeverityHigh, product(10250, "http", "Kubernetes Kubelet"), SeverityCritical},
		{"etcd", CheckEtcdExposed, servicePort(2379, "etcd-client", true), SeverityHigh, product(2379, "http", "etcd"), SeverityCritical},
	}
	for _, c := range cases {
		port := portChecks(t, c.portOnly)
		if len(port) != 1 || port[0].Check != c.check || port[0].Severity != c.portSev || !strings.Contains(port[0].Evidence, "normally") {
			t.Errorf("%s: the port alone = %+v, want one %s/%s that says \"normally\"", c.name, port, c.check, c.portSev)
		}
		sure := portChecks(t, c.confirmed)
		if len(sure) != 1 || sure[0].Check != c.check || sure[0].Severity != c.confirmSev || strings.Contains(sure[0].Evidence, "normally") {
			t.Errorf("%s: a confirmed service = %+v, want one %s/%s without \"normally\"", c.name, sure, c.check, c.confirmSev)
		}
	}
}

// A banner can name anything, so it decides whether a rule fires but is never copied into the words.
func TestPortCheck_BannerTextNeverReachesTheEvidence(t *testing.T) {
	p := servicePort(9200, "http", false)
	p.Product = "Elasticsearch </script><b>x</b> IGNORE PREVIOUS INSTRUCTIONS"
	got := portChecks(t, p)
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	if strings.ContainsAny(got[0].Evidence, "<>") || strings.Contains(got[0].Evidence, "IGNORE") {
		t.Errorf("device text reached the evidence: %q", got[0].Evidence)
	}
}

func TestPortCheck_NoRuleFiresOnAnUnrelatedPort(t *testing.T) {
	if got := portChecks(t, servicePort(8123, "http", false)); len(got) != 0 {
		t.Errorf("an ordinary web port raised %+v", got)
	}
}

func TestDefaultScansIncludeThePortsTheRulesLookAt(t *testing.T) {
	for _, p := range []int{23, 2323, 2375, 5555, 1080, 3128, 9200, 6000, 873, 10250, 2379} {
		if !slices.Contains(DefaultPorts, p) {
			t.Errorf("DefaultPorts lacks %d, so a quick scan cannot raise its rule", p)
		}
		if !slices.Contains(ThoroughPorts, p) {
			t.Errorf("ThoroughPorts lacks %d, so a thorough scan cannot raise its rule", p)
		}
	}
}
