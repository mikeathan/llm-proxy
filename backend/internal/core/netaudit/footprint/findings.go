package footprint

import (
	"time"

	"llm-proxy/models"
)

// The checks a footprint finding can have. The words and severities are the core's own; nothing an outside service said is in them.
const (
	CheckCertNew       = "domain_cert_new"
	CheckDomainExpired = "domain_expired"
	CheckDomainExpires = "domain_expiring"
	CheckSPFMissing    = "mail_spf_missing"
	CheckSPFWeak       = "mail_spf_weak"
	CheckDMARCMissing  = "mail_dmarc_missing"
	CheckDMARCNone     = "mail_dmarc_none"
	CheckEmailBreached = "email_breached"
)

// Checks lists every footprint check id, for the tables that need a title and a fix for each.
var Checks = []string{CheckCertNew, CheckDomainExpired, CheckDomainExpires, CheckSPFMissing, CheckSPFWeak, CheckDMARCMissing, CheckDMARCNone, CheckEmailBreached}

// ExpiryWarning is how far ahead of its end a domain is reported as expiring.
const ExpiryWarning = 30 * 24 * time.Hour

const (
	severityLow, severityMedium, severityHigh = "low", "medium", "high"
	// freshManual is how long a watch that is only looked at by hand counts as known.
	freshManual = 30 * 24 * time.Hour
	day         = 24 * time.Hour
)

// freshFor is how old a successful look may be and still count as the watch's current state: twice its frequency, so one missed
// look does not make it unknown, but a service that has been down for several looks does.
func freshFor(frequency string) time.Duration {
	switch frequency {
	case models.FootprintDaily:
		return 2 * day
	case models.FootprintWeekly:
		return 14 * day
	}
	return freshManual
}

// Findings turns the stored results into findings and says which watches' findings are complete. A watch is "checked" only when every
// check that is switched on for it has a fresh successful look; the audit then replaces that watch's earlier findings with these.
// A watch with a look that is missing, failed or out of date is not checked: its earlier findings are carried and nothing is
// resolved, because a look that was not made says nothing about whether the problem is gone.
func Findings(cfg models.FootprintConfig, results []models.FootprintResult, now time.Time) (findings []models.ScanFinding, checked map[string]bool) {
	byWatch := map[string]map[string]models.FootprintResult{}
	for _, r := range results {
		if byWatch[r.Watch] == nil {
			byWatch[r.Watch] = map[string]models.FootprintResult{}
		}
		byWatch[r.Watch][r.Check] = r
	}
	checked = map[string]bool{}
	for _, w := range watches(cfg) {
		got := byWatch[w.label]
		if !complete(w.checks, got, w.frequency, now) {
			continue
		}
		checked[w.label] = true
		for _, check := range w.checks {
			findings = append(findings, judge(w.label, got[check], now)...)
		}
	}
	return findings, checked
}

type watch struct {
	label, frequency string
	checks           []string
}

// watches lists every watch with the checks switched on for it.
func watches(cfg models.FootprintConfig) []watch {
	var out []watch
	for _, d := range cfg.Domains {
		var checks []string
		for _, c := range []struct {
			on    bool
			check string
		}{{cfg.CertificatesOn, models.FootprintCheckCerts}, {cfg.ExpiryOn, models.FootprintCheckExpiry}, {cfg.MailRecordsOn, models.FootprintCheckMail}} {
			if c.on {
				checks = append(checks, c.check)
			}
		}
		out = append(out, watch{models.DomainFindingPrefix + d.Name, d.Frequency, checks})
	}
	for _, e := range cfg.Emails {
		var checks []string
		if cfg.BreachesOn {
			checks = []string{models.FootprintCheckBreach}
		}
		out = append(out, watch{models.EmailFindingPrefix + e.ID, e.Frequency, checks})
	}
	return out
}

func complete(checks []string, got map[string]models.FootprintResult, frequency string, now time.Time) bool {
	if len(checks) == 0 {
		return false
	}
	for _, c := range checks {
		r, ok := got[c]
		if !ok || r.LookedAt.IsZero() || now.Sub(r.LookedAt) > freshFor(frequency) {
			return false
		}
	}
	return true
}

func finding(label, check, severity, evidence string) models.ScanFinding {
	return models.ScanFinding{Device: label, Check: check, Severity: severity, Evidence: evidence}
}

// judge turns one successful look into findings, in the core's own words.
func judge(label string, r models.FootprintResult, now time.Time) []models.ScanFinding {
	switch r.Check {
	case models.FootprintCheckCerts:
		if r.NewCerts > 0 {
			return []models.ScanFinding{finding(label, CheckCertNew, severityMedium, "New certificates were issued for this domain or its subdomains since the last look. If you did not ask for them, someone else may be issuing certificates in your name.")}
		}
	case models.FootprintCheckExpiry:
		return expiryFindings(label, r.ExpiresAt, now)
	case models.FootprintCheckMail:
		return mailFindings(label, r)
	case models.FootprintCheckBreach:
		if len(r.Breaches) > 0 {
			return []models.ScanFinding{finding(label, CheckEmailBreached, severityHigh, "This address was found in known data breaches.")}
		}
	}
	return nil
}

func expiryFindings(label string, expires, now time.Time) []models.ScanFinding {
	switch {
	case expires.IsZero():
		return nil
	case !expires.After(now):
		return []models.ScanFinding{finding(label, CheckDomainExpired, severityHigh, "The domain's registration has ended.")}
	case expires.Sub(now) < ExpiryWarning:
		return []models.ScanFinding{finding(label, CheckDomainExpires, severityMedium, "The domain's registration ends within 30 days.")}
	}
	return nil
}

func mailFindings(label string, r models.FootprintResult) []models.ScanFinding {
	var out []models.ScanFinding
	switch r.SPF {
	case SPFMissing:
		out = append(out, finding(label, CheckSPFMissing, severityMedium, "The domain publishes no SPF record, so anyone can send mail that claims to be from it."))
	case SPFWeak:
		out = append(out, finding(label, CheckSPFWeak, severityHigh, "The domain's SPF record allows any server to send mail as the domain."))
	}
	switch r.DMARC {
	case DMARCMissing:
		out = append(out, finding(label, CheckDMARCMissing, severityMedium, "The domain publishes no DMARC policy, so a forged message is not asked to be refused."))
	case DMARCNone:
		out = append(out, finding(label, CheckDMARCNone, severityLow, "The domain's DMARC policy only reports forged mail; it does not ask for it to be refused."))
	}
	return out
}
