package footprint

import (
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

var now = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

func cfg() models.FootprintConfig {
	return models.FootprintConfig{
		Domains:        []models.FootprintDomain{{Name: "example.com", Frequency: models.FootprintDaily}},
		Emails:         []models.FootprintEmail{{ID: "a1", Address: "jo@example.com", Frequency: models.FootprintWeekly}},
		CertificatesOn: true, ExpiryOn: true, MailRecordsOn: true, BreachesOn: true,
	}
}

func res(watch, check string, mod func(*models.FootprintResult)) models.FootprintResult {
	r := models.FootprintResult{Watch: watch, Check: check, Status: models.FootprintLooked, At: now, LookedAt: now}
	if mod != nil {
		mod(&r)
	}
	return r
}

func allFresh() []models.FootprintResult {
	return []models.FootprintResult{
		res("domain:example.com", models.FootprintCheckCerts, nil),
		res("domain:example.com", models.FootprintCheckExpiry, func(r *models.FootprintResult) { r.ExpiresAt = now.Add(400 * day) }),
		res("domain:example.com", models.FootprintCheckMail, func(r *models.FootprintResult) { r.SPF, r.DMARC = MailOK, MailOK }),
		res("email:a1", models.FootprintCheckBreach, nil),
	}
}

func checksOf(fs []models.ScanFinding) string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Device+" "+f.Check)
	}
	return strings.Join(out, ",")
}

func TestFindings_ACleanFootprintHasNoFindingsAndIsFullyChecked(t *testing.T) {
	fs, checked := Findings(cfg(), allFresh(), now)
	if len(fs) != 0 || !checked["domain:example.com"] || !checked["email:a1"] {
		t.Errorf("%v %v", fs, checked)
	}
}

func TestFindings_EachProblemIsWordedByTheCoreAndKeyedOnTheLabel(t *testing.T) {
	rs := allFresh()
	rs[0].NewCerts = 2
	rs[1].ExpiresAt = now.Add(10 * day)
	rs[2].SPF, rs[2].DMARC = SPFWeak, DMARCNone
	rs[3].Breaches = []models.FootprintBreach{{Name: "<b>Evil</b>"}}
	fs, _ := Findings(cfg(), rs, now)
	want := "domain:example.com domain_cert_new,domain:example.com domain_expiring,domain:example.com mail_spf_weak,domain:example.com mail_dmarc_none,email:a1 email_breached"
	if got := checksOf(fs); got != want {
		t.Errorf("got %s", got)
	}
	for _, f := range fs {
		if strings.Contains(f.Evidence, "Evil") || strings.Contains(f.Evidence, "@") || strings.Contains(f.Evidence, "<") {
			t.Errorf("outside text or the address reached a finding: %q", f.Evidence)
		}
		if f.Key() == "" || !strings.HasPrefix(f.Key(), f.Device+"|") {
			t.Errorf("key %q", f.Key())
		}
	}
	if fs[len(fs)-1].Device != "email:a1" {
		t.Errorf("the email label holds the opaque id")
	}
}

func TestFindings_AnExpiredDomainIsHigh(t *testing.T) {
	rs := allFresh()
	rs[1].ExpiresAt = now.Add(-day)
	fs, _ := Findings(cfg(), rs, now)
	if len(fs) != 1 || fs[0].Check != CheckDomainExpired || fs[0].Severity != severityHigh {
		t.Errorf("%v", fs)
	}
}

// A look that was not made, or is out of date, must not resolve what was found: the watch is not "checked", so the audit carries
// its earlier findings.
func TestFindings_AWatchWithAMissingOrStaleLookIsNotChecked(t *testing.T) {
	rs := allFresh()
	rs[2].LookedAt = now.Add(-3 * day) // mail look is older than twice a daily frequency
	_, checked := Findings(cfg(), rs, now)
	if checked["domain:example.com"] || !checked["email:a1"] {
		t.Errorf("checked: %v", checked)
	}
	_, checked = Findings(cfg(), rs[:1], now)
	if checked["domain:example.com"] {
		t.Errorf("a watch with a missing look is not checked: %v", checked)
	}
}

func TestFindings_AFailedAttemptKeepsTheLastGoodLookFresh(t *testing.T) {
	rs := allFresh()
	rs[2].Status, rs[2].Reason, rs[2].At = models.FootprintCouldNotLook, ReasonDown, now.Add(time.Hour)
	_, checked := Findings(cfg(), rs, now.Add(time.Hour))
	if !checked["domain:example.com"] {
		t.Error("one failed attempt within the window must not make the watch unknown")
	}
}

func TestFindings_ASwitchedOffServiceIsNotAskedFor(t *testing.T) {
	c := cfg()
	c.MailRecordsOn, c.BreachesOn = false, false
	rs := allFresh()[:2]
	fs, checked := Findings(c, rs, now)
	if len(fs) != 0 || !checked["domain:example.com"] || checked["email:a1"] {
		t.Errorf("%v %v", fs, checked)
	}
}
