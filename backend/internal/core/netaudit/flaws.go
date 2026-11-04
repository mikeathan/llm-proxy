package netaudit

import (
	"slices"
	"strings"
	"time"

	"llm-proxy/internal/core/netaudit/ubuntuoval"
	"llm-proxy/models"
)

// CheckHostPackagesVulnerable is installed packages that are older than the version that fixed a published security flaw.
const CheckHostPackagesVulnerable = "host_packages_vulnerable"

const (
	// flawGrace is how long a notice may be new before a package behind it is counted: a fix takes a day or so to reach the
	// mirrors and the computer's own automatic updates, and a finding in that window would only be noise.
	flawGrace = 48 * time.Hour
	// maxFlawNames is how many package names the finding lists; the count is always the whole number.
	maxFlawNames = 10
)

// FlawPackage is one installed package that is behind a fix.
type FlawPackage struct {
	Name      string
	Installed string
	// FixedIn is the highest version among the fixes the package is missing.
	FixedIn string
	USNs    []string
	// Exploited is how many of the flaws CISA lists as exploited.
	Exploited int
}

// FlawResult is the judgement of the installed packages against Ubuntu's notices.
type FlawResult struct {
	Packages []FlawPackage
	// Flaws is the number of different flaws the missing fixes cover, and Exploited how many of those CISA lists.
	Flaws     int
	Exploited int
	// Severity is the worst severity among them: "kev" when one is on CISA's list, else "critical", "high", "medium", "low" or "".
	Severity string
}

var flawRank = map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4, "kev": 5}

// MatchFlaws finds the installed packages that are older than a version that fixed a published flaw, comparing versions as dpkg
// does. A notice issued within flawGrace of now is not counted yet. exploited is CISA's list of flaws seen exploited (CVE id to
// date), and may be nil. Packages the notices do not mention are not judged.
func MatchFlaws(idx models.UbuntuIndex, installed map[string]string, exploited map[string]string, now time.Time) FlawResult {
	var res FlawResult
	all := map[string]bool{}
	for name, version := range installed {
		pkg, flaws := matchPackage(idx, name, version, exploited, now)
		if len(pkg.USNs) == 0 {
			continue
		}
		res.Packages = append(res.Packages, pkg)
		for _, cve := range flaws {
			all[cve] = true
		}
		for _, usn := range pkg.USNs {
			if sev := idx.Notices[usn].Severity; flawRank[sev] > flawRank[res.Severity] {
				res.Severity = sev
			}
		}
	}
	for cve := range all {
		if _, ok := exploited[cve]; ok {
			res.Exploited++
		}
	}
	res.Flaws = len(all)
	if res.Exploited > 0 {
		res.Severity = "kev"
	}
	slices.SortFunc(res.Packages, func(a, b FlawPackage) int {
		switch {
		case a.Exploited != b.Exploited:
			return b.Exploited - a.Exploited
		case len(a.USNs) != len(b.USNs):
			return len(b.USNs) - len(a.USNs)
		}
		return strings.Compare(a.Name, b.Name)
	})
	return res
}

// matchPackage is the fixes one package is missing and the flaws they cover.
func matchPackage(idx models.UbuntuIndex, name, version string, exploited map[string]string, now time.Time) (FlawPackage, []string) {
	pkg := FlawPackage{Name: name, Installed: version}
	var cves []string
	seen := map[string]bool{}
	for _, fix := range idx.Packages[name] {
		notice, ok := idx.Notices[fix.USN]
		if !ok || ubuntuoval.Compare(version, fix.Fixed) >= 0 || tooNew(notice.Issued, now) || seen[fix.USN] {
			continue
		}
		seen[fix.USN] = true
		pkg.USNs = append(pkg.USNs, fix.USN)
		if pkg.FixedIn == "" || ubuntuoval.Compare(fix.Fixed, pkg.FixedIn) > 0 {
			pkg.FixedIn = fix.Fixed
		}
		cves = append(cves, notice.CVEs...)
	}
	slices.Sort(pkg.USNs)
	slices.Sort(cves)
	cves = slices.Compact(cves)
	for _, cve := range cves {
		if _, ok := exploited[cve]; ok {
			pkg.Exploited++
		}
	}
	return pkg, cves
}

// tooNew reports whether a notice was issued (YYYY-MM-DD) less than flawGrace before now; a date that cannot be read is old.
func tooNew(issued string, now time.Time) bool {
	t, err := time.Parse("2006-01-02", issued)
	return err == nil && now.Sub(t) < flawGrace
}

// flawsOf is what the host watch keeps of a judgement: counts, and the names of the ten packages to look at first.
func flawsOf(res FlawResult) models.HostFlaws {
	out := models.HostFlaws{Packages: len(res.Packages), Flaws: res.Flaws, Exploited: res.Exploited, Severity: res.Severity}
	for i, p := range res.Packages {
		if i == maxFlawNames {
			break
		}
		out.Names = append(out.Names, p.Name)
	}
	return out
}

// flawFinding is the finding for a judgement that found packages behind. Severity: high when a flaw is on CISA's list of
// exploited flaws, medium for a critical or high one, low otherwise. The words are fixed; the names are plain package names.
func flawFinding(h models.HostFlaws) Finding {
	verb := "are"
	if h.Packages == 1 {
		verb = "is"
	}
	text := plural(h.Packages, "installed package", "installed packages") + " " + verb + " older than the version that fixed a published security flaw"
	if h.Flaws > 0 {
		text += " (" + plural(h.Flaws, "published security flaw", "published security flaws")
		if h.Exploited > 0 {
			text += ", " + plural(h.Exploited, "of them", "of them") + " on CISA's list of flaws attackers are using"
		}
		text += ")"
	}
	if len(h.Names) > 0 {
		text += ": " + strings.Join(h.Names, ", ")
		if more := h.Packages - len(h.Names); more > 0 {
			text += ", and " + plural(more, "more", "more")
		}
	}
	text += ". Updates that only an Ubuntu Pro subscription installs are not counted"
	severity := SeverityLow
	switch h.Severity {
	case "kev":
		severity = SeverityHigh
	case "critical", "high":
		severity = SeverityMedium
	}
	return Finding{Device: HostDevice, Check: CheckHostPackagesVulnerable, Severity: severity, Evidence: text}
}

// FlawJudge judges the installed packages. ok is false when there is nothing to judge them against: no data downloaded, or the
// data is for another release than this computer's.
type FlawJudge interface {
	Judge(installed map[string]string) (FlawResult, bool)
}

// PackageFlaws is the judge over the downloaded notices and CISA's list.
type PackageFlaws struct {
	Index     func() models.UbuntuIndex
	Exploited func() map[string]string
	// Release is this computer's Ubuntu release codename, or "" when it is not Ubuntu.
	Release func() string
	Now     func() time.Time
}

// Judge matches the packages when the data is for this computer's release.
func (p PackageFlaws) Judge(installed map[string]string) (FlawResult, bool) {
	idx := p.Index()
	if len(idx.Notices) == 0 || idx.Release == "" || idx.Release != p.Release() {
		return FlawResult{}, false
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	var exploited map[string]string
	if p.Exploited != nil {
		exploited = p.Exploited()
	}
	return MatchFlaws(idx, installed, exploited, now), true
}

func init() {
	checkTitles[CheckHostPackagesVulnerable] = "Installed packages are behind a security fix"
	remediations[CheckHostPackagesVulnerable] = "Install the waiting updates (sudo apt update, then sudo apt upgrade), and restart the services that use the updated libraries or the computer if it asks for it."
	guides[CheckHostPackagesVulnerable] = packagesVulnerableGuide
}

var packagesVulnerableGuide = FixGuide{
	Steps: []string{
		"Update the package list and install the updates: the packages named in the finding are the ones to look at first.",
		"If a package stays behind after the update, its fix may be one that only an Ubuntu Pro subscription installs (the program does not count those), or the package may come from another source than Ubuntu's archive. Check which source it came from.",
		"Restart the services that use an updated library, or the computer if it says a restart is pending.",
		"Turn on automatic security updates so this stays short.",
	},
	Commands: []GuideCommand{
		{platformLinux, "sudo apt update\nsudo apt list --upgradable 2>/dev/null | head -30\nsudo apt upgrade"},
	},
	Risk: "An upgrade can restart a service and can change a package you rely on; read what apt proposes before you confirm it.",
	Undo: "Install the earlier version of a package again with apt install <package>=<version>.",
}
