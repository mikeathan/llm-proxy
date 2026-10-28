// Package vulnmatch compares the software a scan found with the local vulnerability index. It is pure: no I/O, no
// clock. A match means the version nmap read falls inside a range NVD lists, never that the device is vulnerable (a
// distribution can fix a flaw without changing the version number), so callers must word it as "may be affected".
package vulnmatch

import (
	"net/url"
	"slices"
	"strings"

	"llm-proxy/internal/core/netaudit/vulnindex"
	"llm-proxy/models"
)

// Match is one CVE a scanned version falls inside the ranges of.
type Match struct {
	CVE   string
	Score float64
	// Exploited is true when CISA lists the CVE as exploited in the wild.
	Exploited bool
}

// Result is what Find learned about one port.
type Result struct {
	// Matches has one entry per CVE, exploited ones first, then by score (highest first), then by id.
	Matches []Match
	// Uncomparable counts CVEs that had a range for the product but whose version could not be compared with the scanned
	// one, and that did not match any other way. They are not reported as matches; the count lets a report say so.
	Uncomparable int
	// Versioned is true when at least one CPE carried a version to compare, whether or not the index knew the product.
	Versioned bool
	// Precise is true when at least one CPE's version could be placed in a release (not just "4"), so a comparison with the
	// ranges meant something. A port that is Versioned but not Precise was too coarse to compare.
	Precise bool
}

// maxCPEFields is the part letter plus six more components of a CPE 2.2 URI.
const maxCPEFields = 7

// Find matches the CPEs nmap printed for one port against the index. A CPE with no version never matches.
func Find(idx models.VulnIndex, cpes []string) Result {
	found := map[string]Match{}
	unclear := map[string]bool{}
	res := Result{}
	for _, cpe := range cpes {
		key, version, ok := parseCPE(cpe)
		if !ok {
			continue
		}
		res.Versioned = true
		res.Precise = res.Precise || !majorOnly(version)
		for _, r := range idx.Entries[key] {
			switch matched, comparable := inRange(version, r); {
			case matched:
				found[r.CVE] = better(found[r.CVE], r)
			case !comparable:
				unclear[r.CVE] = true
			}
		}
	}
	for id, m := range found {
		_, m.Exploited = idx.Exploited[id]
		res.Matches = append(res.Matches, m)
	}
	for id := range unclear {
		if _, hit := found[id]; !hit {
			res.Uncomparable++
		}
	}
	slices.SortFunc(res.Matches, byImportance)
	return res
}

// better keeps the higher score when the same CVE matches more than once.
func better(old Match, r models.VulnRange) Match {
	if old.CVE == "" || r.Score > old.Score {
		return Match{CVE: r.CVE, Score: r.Score}
	}
	return old
}

func byImportance(a, b Match) int {
	switch {
	case a.Exploited != b.Exploited:
		if a.Exploited {
			return -1
		}
		return 1
	case a.Score != b.Score:
		if a.Score > b.Score {
			return -1
		}
		return 1
	}
	return strings.Compare(a.CVE, b.CVE)
}

// parseCPE reads a CPE 2.2 URI ("cpe:/a:openbsd:openssh:9.6p1") into the index key and the version. It returns false
// for anything that is not one, and for a CPE with no usable version.
func parseCPE(cpe string) (key, version string, ok bool) {
	rest, found := strings.CutPrefix(cpe, "cpe:/")
	if !found {
		return "", "", false
	}
	f := strings.Split(rest, ":")
	if len(f) < 4 || len(f) > maxCPEFields || f[1] == "" || f[2] == "" {
		return "", "", false
	}
	v, err := url.PathUnescape(f[3])
	if err != nil || v == "" || v == "-" || v == "*" {
		return "", "", false
	}
	return vulnindex.Key(f[0], f[1], f[2]), v, true
}

// inRange reports whether version falls inside r. comparable is false when a bound or the exact version could not be
// compared with it; the range is then not a match.
func inRange(version string, r models.VulnRange) (matched, comparable bool) {
	if r.Exact != "" {
		if strings.EqualFold(r.Exact, version) {
			return true, true
		}
		c, ok := compareVersions(version, r.Exact)
		return ok && c == 0, ok
	}
	if majorOnly(version) {
		return familyInRange(version, r)
	}
	p, ok := placeAgainst(version, r)
	return ok && p == inside, ok
}

// where a version sits against a range.
type where int

const (
	inside where = iota
	below
	above
)

// placeAgainst puts version below, inside or above r's bounds; ok is false when a bound could not be compared.
func placeAgainst(version string, r models.VulnRange) (where, bool) {
	checks := []struct {
		limit string
		out   where
		holds func(c int) bool // true when version is on the wrong side of this bound
	}{
		{r.StartIncl, below, func(c int) bool { return c < 0 }},
		{r.StartExcl, below, func(c int) bool { return c <= 0 }},
		{r.EndIncl, above, func(c int) bool { return c > 0 }},
		{r.EndExcl, above, func(c int) bool { return c >= 0 }},
	}
	for _, b := range checks {
		if b.limit == "" {
			continue
		}
		c, ok := compareVersions(version, b.limit)
		if !ok {
			return inside, false
		}
		if b.holds(c) {
			return b.out, true
		}
	}
	return inside, true
}

// familyInRange handles a scanned version that is only a major number ("4": Samba is reported that way). The real release
// is somewhere in 4.0 to 4.x, so a range matches only if all of that family lies inside it, and is ruled out only if
// none of it can. Anything between (a range that covers part of the family) cannot be decided and is not a match.
func familyInRange(major string, r models.VulnRange) (matched, comparable bool) {
	first, last := major, major+familyEnd
	atFirst, ok1 := placeAgainst(first, r)
	atLast, ok2 := placeAgainst(last, r)
	switch {
	case !ok1 || !ok2:
		return false, false
	case atFirst == above || atLast == below:
		return false, true
	case atFirst == inside && atLast == inside:
		return true, true
	}
	return false, false
}

// familyEnd turns a major version into a stand-in for its newest possible release.
const familyEnd = ".999999999"

// majorOnly is true for a version that is a single number ("4"). Some programs are reported that way; the real release
// could be anywhere in that major version, so it cannot be placed against a range.
func majorOnly(version string) bool {
	tokens, ok := tokenize(version)
	return ok && len(tokens) == 1 && tokens[0].isNum()
}
