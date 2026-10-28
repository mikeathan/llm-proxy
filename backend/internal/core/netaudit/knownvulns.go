package netaudit

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"llm-proxy/internal/core/netaudit/vulnmatch"
	"llm-proxy/models"
)

// CheckKnownVulns is the check name of a finding that a port's software version falls inside a range NVD lists for a
// CVE. It is worded "may be affected", never "is vulnerable": a banner version does not prove a flaw.
const CheckKnownVulns = "known_vulns"

// maxEvidenceIDs bounds how many CVE ids the evidence names; the finding itself keeps them all.
const maxEvidenceIDs = 5

// cveID is the only shape of id that may be printed. The ids come from the downloaded files, which are untrusted.
var cveID = regexp.MustCompile(`^CVE-\d{4}-\d{4,}$`)

// distroBuild finds a distribution's packaging marker in the version text nmap read ("9.6p1 Ubuntu 3ubuntu13.20"). Such
// a build usually carries fixes without changing the upstream number the CPE holds.
var distroBuild = regexp.MustCompile(`(?i)\b(ubuntu|debian|raspbian|fedora|centos|rhel|alpine)\b|\bdeb\d+u\d+|\.el\d+`)

// VulnCoverage says how much of a scan the vulnerability data could be compared with, so a report never implies more
// was checked than was.
type VulnCoverage struct {
	// Enabled is true when matching was asked for at all; nothing about vulnerabilities is reported otherwise.
	Enabled bool
	// Skipped is true when it was asked for but there was no vulnerability data to compare with.
	Skipped bool
	// Total is the open services on the addresses scanned to the end; Checked the ones that gave a version to compare;
	// Coarse the ones whose version was only a major number (Samba "4"), too vague to compare; Uncomparable the CVEs whose
	// version could not be compared.
	Total, Checked, Coarse, Uncomparable int
}

// KnownVulnFindings compares the software the scan recognised with the index: one finding per open port that may be
// affected by at least one CVE. It is pure. An address the engine did not finish is left out, because its ports are
// unknown.
func KnownVulnFindings(hosts []Host, idx models.VulnIndex) ([]Finding, VulnCoverage) {
	cov := VulnCoverage{Enabled: true}
	if len(idx.Entries) == 0 {
		return nil, cov
	}
	var out []Finding
	for _, h := range hosts {
		if h.Unscanned {
			continue
		}
		for _, p := range h.Ports {
			if p.State != "open" {
				continue
			}
			cov.Total++
			res := vulnmatch.Find(idx, p.CPEs)
			cov.Uncomparable += res.Uncomparable
			switch {
			case res.Precise || len(res.Matches) > 0:
				cov.Checked++
			case res.Versioned:
				cov.Coarse++
			}
			if f, ok := knownVulnFinding(h.Addr.String(), p, res); ok {
				out = append(out, f)
			}
		}
	}
	return out, cov
}

func knownVulnFinding(device string, p Port, res vulnmatch.Result) (Finding, bool) {
	var ids, exploited []string
	var top float64
	scores := map[string]float64{}
	for _, m := range res.Matches {
		if !cveID.MatchString(m.CVE) {
			continue
		}
		ids = append(ids, m.CVE)
		scores[m.CVE] = m.Score
		if m.Exploited {
			exploited = append(exploited, m.CVE)
		}
		top = max(top, m.Score)
	}
	if len(ids) == 0 {
		return Finding{}, false
	}
	distro := distroBuild.MatchString(p.Version)
	f := Finding{
		Device: device, Port: p.Number, Proto: p.Proto, Service: serviceLabel(p), Version: joinVersion(p),
		Check: CheckKnownVulns, Severity: vulnSeverity(top, len(exploited) > 0, distro),
		Evidence: vulnEvidence(ids, len(exploited), distro),
	}
	f.CVEs, f.Exploited, f.CVEScores = sortedCopy(ids), sortedCopy(exploited), scores
	return f, true
}

// vulnSeverity is high for a CVE CISA lists as exploited or a score of 9 or more, medium from 7, otherwise low; a
// distribution build lowers it one step.
func vulnSeverity(top float64, exploited, distro bool) string {
	level := 0
	switch {
	case exploited || top >= 9:
		level = 2
	case top >= 7:
		level = 1
	}
	if distro {
		level = max(level-1, 0)
	}
	return []string{SeverityLow, SeverityMedium, SeverityHigh}[level]
}

// vulnEvidence is the finding's one line, written here from counts and validated ids; ids is already in order of
// importance.
func vulnEvidence(ids []string, exploited int, distro bool) string {
	noun := "flaws"
	if len(ids) == 1 {
		noun = "flaw"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "The software on this port may be affected by %d known %s", len(ids), noun)
	if exploited > 0 {
		fmt.Fprintf(&b, " (%d actively exploited)", exploited)
	}
	listed := ids[:min(len(ids), maxEvidenceIDs)]
	b.WriteString(": " + strings.Join(listed, ", "))
	if more := len(ids) - len(listed); more > 0 {
		fmt.Fprintf(&b, " and %d more", more)
	}
	b.WriteString(".")
	if distro {
		b.WriteString(" This is a distribution build, so the flaws may already be fixed.")
	}
	return b.String()
}

func sortedCopy(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := slices.Clone(in)
	slices.Sort(out)
	return out
}

// VulnSource returns the vulnerability index to match against; empty until the data has been downloaded.
type VulnSource func() models.VulnIndex

// WithVulns makes the reporter add known_vulns findings to each scan. Without it, or when the audit's own settings turn
// matching off, nothing about vulnerabilities appears.
func (r *RunReporter) WithVulns(src VulnSource) *RunReporter {
	r.vulns = src
	return r
}

// addKnownVulns appends the known_vulns findings to the result and says how far the matching reached. The findings
// join the result before the comparison with the last scan, so they are diffed, accepted and kept like any other.
func (r *RunReporter) addKnownVulns(ctx context.Context, result *Result) VulnCoverage {
	if r.vulns == nil {
		return VulnCoverage{}
	}
	if settings, ok := models.ScanSettingsFrom(ctx); ok && !settings.KnownVulnsOn() {
		return VulnCoverage{}
	}
	idx := r.vulns()
	found, cov := KnownVulnFindings(result.Hosts, idx)
	cov.Skipped = len(idx.Entries) == 0
	result.Findings = append(result.Findings, found...)
	return cov
}

// writeCoverage states how much of the scan the vulnerability data was compared with.
func writeCoverage(b *strings.Builder, c VulnCoverage) {
	switch {
	case !c.Enabled:
	case c.Skipped:
		b.WriteString("\n> Note: Known-vulnerability matching was skipped: the vulnerability data has not been downloaded (Settings, Network scan).\n")
	default:
		fmt.Fprintf(b, "\nKnown vulnerabilities: compared %d of %d open services with the vulnerability data.", c.Checked, c.Total)
		if c.Coarse > 0 {
			fmt.Fprintf(b, " %d reported only a major version (for example just \"4\"), which is too coarse to compare, so they are not covered.", c.Coarse)
		}
		b.WriteString(" The rest gave no software version to compare.")
		if c.Uncomparable > 0 {
			fmt.Fprintf(b, " %d could not be compared and are not reported.", c.Uncomparable)
		}
		b.WriteString("\n")
	}
}

// coverageSummary is the counts-only clause for the connector summary.
func coverageSummary(c VulnCoverage) string {
	switch {
	case !c.Enabled:
		return ""
	case c.Skipped:
		return "; vulnerability matching skipped (no data)"
	}
	return fmt.Sprintf("; vulnerability data compared for %d of %d services", c.Checked, c.Total)
}
