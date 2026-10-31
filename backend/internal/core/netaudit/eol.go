package netaudit

import (
	"strings"
	"time"
)

// CheckEndOfLife is the check name of a finding that a port runs a version series its makers no longer support, so a flaw
// found in it later will not be fixed upstream. A banner proves little and a distribution often keeps fixing an old
// number, so it is worded as "no longer supported by its makers", and a distribution build is rated lower.
const CheckEndOfLife = "end_of_life"

// endOfLifeEvidence is the finding's one line. It is fixed text: the software is named in the finding's own fields.
const endOfLifeEvidence = "The makers of this software no longer support this version, so a flaw found in it later will not be fixed upstream. A system's vendor may still patch it"

// eolSeries is one version series and the day its makers stopped fixing it. Key is the vendor and product of the CPE nmap
// printed, which is exact; Cycle is the series as the makers number it. Dates are from endoflife.date, read 2026-10-31.
// Only series that are long past and unambiguous are listed: a distribution that backports fixes (OpenSSL 3.0 on a long
// term release, for example) would make a version number misleading, so those are left out.
type eolSeries struct{ Key, Cycle, Ended string }

var endOfLifeSeries = []eolSeries{
	{"apache:http_server", "2.0", "2013-07-10"},
	{"apache:http_server", "2.2", "2017-07-11"},
	{"php:php", "5", "2018-12-31"},
	{"php:php", "7.0", "2019-01-10"},
	{"php:php", "7.1", "2019-12-01"},
	{"php:php", "7.2", "2020-11-30"},
	{"php:php", "7.3", "2021-12-06"},
	{"php:php", "7.4", "2022-11-28"},
	{"php:php", "8.0", "2023-11-26"},
	{"php:php", "8.1", "2025-12-31"},
	{"openssl:openssl", "1.0.2", "2019-12-31"},
	{"openssl:openssl", "1.1.0", "2019-09-11"},
	{"openssl:openssl", "1.1.1", "2023-09-11"},
	{"mysql:mysql", "5.5", "2018-12-31"},
	{"mysql:mysql", "5.6", "2021-02-28"},
	{"mysql:mysql", "5.7", "2023-10-31"},
	{"postgresql:postgresql", "9.6", "2021-11-11"},
	{"postgresql:postgresql", "10", "2022-11-10"},
	{"postgresql:postgresql", "11", "2023-11-09"},
	{"postgresql:postgresql", "12", "2024-11-21"},
	{"postgresql:postgresql", "13", "2025-11-13"},
}

// EndOfLifeFindings reports one finding for each open port whose recognised software is in a series that has ended. It is
// pure, reads the CPEs nmap printed (no lookup, no network), and leaves out a host the scan did not finish.
func EndOfLifeFindings(hosts []Host, now time.Time) []Finding {
	var out []Finding
	for _, h := range hosts {
		if h.Unscanned {
			continue
		}
		for _, p := range h.Ports {
			if p.State != "open" || !runsEndOfLife(p, now) {
				continue
			}
			severity := SeverityMedium
			if distroBuild.MatchString(p.Version) {
				severity = SeverityLow
			}
			out = append(out, Finding{
				Device: h.Addr.String(), Port: p.Number, Proto: p.Proto, Service: serviceLabel(p), Version: joinVersion(p),
				Check: CheckEndOfLife, Severity: severity, Evidence: endOfLifeEvidence,
			})
		}
	}
	return out
}

func runsEndOfLife(p Port, now time.Time) bool {
	for _, cpe := range p.CPEs {
		key, version, ok := appCPE(cpe)
		if !ok {
			continue
		}
		for _, s := range endOfLifeSeries {
			if s.Key == key && inSeries(version, s.Cycle) && endedBy(s.Ended, now) {
				return true
			}
		}
	}
	return false
}

// appCPE splits "cpe:/a:vendor:product:version[:...]" into "vendor:product" and the version; only application CPEs with a
// version count.
func appCPE(cpe string) (key, version string, ok bool) {
	parts := strings.Split(cpe, ":")
	if len(parts) < 5 || parts[0] != "cpe" || parts[1] != "/a" || parts[2] == "" || parts[3] == "" || parts[4] == "" {
		return "", "", false
	}
	return parts[2] + ":" + parts[3], parts[4], true
}

// inSeries is true when version is in the series: it equals the cycle or continues it with a dot, a dash or a letter
// (OpenSSL's 1.1.1w), never with another digit (PHP 7.40 is not 7.4).
func inSeries(version, cycle string) bool {
	rest, found := strings.CutPrefix(version, cycle)
	if !found {
		return false
	}
	if rest == "" {
		return true
	}
	c := rest[0]
	return c == '.' || c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func endedBy(ended string, now time.Time) bool {
	t, err := time.Parse("2006-01-02", ended)
	return err == nil && now.After(t)
}
