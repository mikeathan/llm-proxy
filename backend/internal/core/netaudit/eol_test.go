package netaudit

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"llm-proxy/models"
)

var eolNow = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

func hostWithPort(p Port) Host {
	p.State = "open"
	return Host{Addr: netip.MustParseAddr("192.168.1.20"), Up: true, Ports: []Port{p}}
}

func eolFindings(p Port) []Finding { return EndOfLifeFindings([]Host{hostWithPort(p)}, eolNow) }

func TestEndOfLife_FlagsAVersionSeriesItsMakersStoppedSupporting(t *testing.T) {
	cases := map[string]Port{
		"apache 2.2":    {Number: 80, Proto: "tcp", Service: "http", Product: "Apache httpd", Version: "2.2.34", CPEs: []string{"cpe:/a:apache:http_server:2.2.34"}},
		"php 7.4":       {Number: 80, Proto: "tcp", Service: "http", Product: "PHP", Version: "7.4.33", CPEs: []string{"cpe:/a:php:php:7.4.33"}},
		"openssl 1.1.1": {Number: 443, Proto: "tcp", Service: "https", CPEs: []string{"cpe:/a:openssl:openssl:1.1.1w"}},
		"mysql 5.7":     {Number: 3306, Proto: "tcp", Service: "mysql", CPEs: []string{"cpe:/a:mysql:mysql:5.7.44"}},
		"postgres 13":   {Number: 5432, Proto: "tcp", Service: "postgresql", CPEs: []string{"cpe:/a:postgresql:postgresql:13.2"}},
	}
	for name, p := range cases {
		got := eolFindings(p)
		if len(got) != 1 || got[0].Check != CheckEndOfLife || got[0].Severity != SeverityMedium || got[0].Port != p.Number {
			t.Errorf("%s: findings = %+v", name, got)
		}
	}
}

func TestEndOfLife_LeavesSupportedAndUnlistedSoftwareAlone(t *testing.T) {
	cases := map[string]Port{
		"apache 2.4":          {Number: 80, Proto: "tcp", CPEs: []string{"cpe:/a:apache:http_server:2.4.58"}},
		"php 8.3":             {Number: 80, Proto: "tcp", CPEs: []string{"cpe:/a:php:php:8.3.1"}},
		"php 7.40 is not 7.4": {Number: 80, Proto: "tcp", CPEs: []string{"cpe:/a:php:php:7.40.0"}},
		"openssl 3.0":         {Number: 443, Proto: "tcp", CPEs: []string{"cpe:/a:openssl:openssl:3.0.2"}},
		"postgres 14":         {Number: 5432, Proto: "tcp", CPEs: []string{"cpe:/a:postgresql:postgresql:14.1"}},
		"unlisted product":    {Number: 22, Proto: "tcp", CPEs: []string{"cpe:/a:openbsd:openssh:7.4p1"}},
		"no version":          {Number: 80, Proto: "tcp", CPEs: []string{"cpe:/a:apache:http_server"}},
		"no cpe":              {Number: 80, Proto: "tcp", Product: "Apache httpd", Version: "2.2.34"},
		"not an app cpe":      {Number: 80, Proto: "tcp", CPEs: []string{"cpe:/o:linux:linux_kernel:2.2.34"}},
	}
	for name, p := range cases {
		if got := eolFindings(p); len(got) != 0 {
			t.Errorf("%s: unexpected findings %+v", name, got)
		}
	}
}

func TestEndOfLife_OneFindingPerPortAndNoneForAHostTheScanDidNotFinish(t *testing.T) {
	p := Port{Number: 80, Proto: "tcp", CPEs: []string{"cpe:/a:apache:http_server:2.2.34", "cpe:/a:php:php:5.6.40"}}
	if got := eolFindings(p); len(got) != 1 {
		t.Errorf("two end-of-life programs on one port is still one finding, got %d", len(got))
	}
	h := hostWithPort(p)
	h.Unscanned = true
	if got := EndOfLifeFindings([]Host{h}, eolNow); len(got) != 0 {
		t.Errorf("an unfinished host has unknown ports: %+v", got)
	}
	closed := hostWithPort(p)
	closed.Ports[0].State = "closed"
	if got := EndOfLifeFindings([]Host{closed}, eolNow); len(got) != 0 {
		t.Errorf("a closed port runs nothing: %+v", got)
	}
}

func TestEndOfLife_AFinishedSupportDateInTheFutureIsNotYetAFinding(t *testing.T) {
	p := Port{Number: 5432, Proto: "tcp", CPEs: []string{"cpe:/a:postgresql:postgresql:13.2"}}
	if got := EndOfLifeFindings([]Host{hostWithPort(p)}, time.Date(2025, 11, 12, 0, 0, 0, 0, time.UTC)); len(got) != 0 {
		t.Errorf("support ended on 2025-11-13, so on the 12th there is nothing to report: %+v", got)
	}
}

func TestEndOfLife_ADistributionBuildIsLowerBecauseItOftenCarriesTheFixes(t *testing.T) {
	p := Port{Number: 80, Proto: "tcp", Product: "Apache httpd", Version: "2.2.15 ((CentOS))", CPEs: []string{"cpe:/a:apache:http_server:2.2.15"}}
	got := eolFindings(p)
	if len(got) != 1 || got[0].Severity != SeverityLow {
		t.Errorf("findings = %+v", got)
	}
}

func TestEndOfLife_TheEvidenceIsFixedWordsAndTheSoftwareStaysInItsOwnFields(t *testing.T) {
	p := Port{Number: 80, Proto: "tcp", Service: "http", Product: "Apache httpd", Version: "2.2.34", CPEs: []string{"cpe:/a:apache:http_server:2.2.34"}}
	got := eolFindings(p)[0]
	if got.Version != "Apache httpd 2.2.34" || got.Device != "192.168.1.20" {
		t.Errorf("finding = %+v", got)
	}
	if got.Evidence != endOfLifeEvidence {
		t.Errorf("evidence = %q: it must be the fixed sentence, with nothing the device sent", got.Evidence)
	}
}

func TestRunReporter_AnEndOfLifeServiceBecomesAFindingInTheAudit(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	port := Port{Number: 80, Proto: "tcp", State: "open", Service: "http", Product: "Apache httpd", Version: "2.2.34", CPEs: []string{"cpe:/a:apache:http_server:2.2.34"}}
	ctx, c := WithCollector(context.Background())
	c.Add(Result{Engine: "nmap", Hosts: []Host{{Addr: netip.MustParseAddr("192.168.1.20"), Up: true, Ports: []Port{port}}}})
	reporter(store, eolNow).Augment(ctx, "ws", "nightly", "")
	found := false
	for _, f := range store.data["ws/nightly"].Findings {
		found = found || f.Check == CheckEndOfLife
	}
	if !found {
		t.Errorf("baseline findings = %+v", store.data["ws/nightly"].Findings)
	}
}
