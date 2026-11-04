package netaudit

import (
	"errors"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/platform/hostwatch"
	"llm-proxy/models"
)

var flawsNow = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func notice(issued, severity string, cves ...string) models.UbuntuNotice {
	return models.UbuntuNotice{Issued: issued, Severity: severity, CVEs: cves}
}

func flawIndex() models.UbuntuIndex {
	return models.UbuntuIndex{
		Release: "noble",
		Notices: map[string]models.UbuntuNotice{
			"USN-1-1": notice("2026-01-01", "medium", "CVE-2026-0001"),
			"USN-2-1": notice("2026-02-01", "high", "CVE-2026-0002", "CVE-2026-0003"),
			"USN-3-1": notice("2026-11-04", "critical", "CVE-2026-0004"), // issued today
		},
		Packages: map[string][]models.UbuntuFix{
			"openssl": {{Fixed: "3.0.13-0ubuntu3.1", USN: "USN-1-1"}, {Fixed: "3.0.13-0ubuntu3.2", USN: "USN-2-1"}},
			"curl":    {{Fixed: "8.5.0-2ubuntu10.5", USN: "USN-1-1"}},
			"libfoo":  {{Fixed: "1.0-1", USN: "USN-3-1"}},
		},
	}
}

func TestMatchFlaws_APackageOlderThanTheFixIsListedWithTheHighestFixAndEveryFlaw(t *testing.T) {
	got := MatchFlaws(flawIndex(), map[string]string{"openssl": "3.0.13-0ubuntu3", "curl": "8.5.0-2ubuntu10.5", "bash": "5.2"}, nil, flawsNow)
	if len(got.Packages) != 1 || got.Packages[0].Name != "openssl" || got.Packages[0].FixedIn != "3.0.13-0ubuntu3.2" || len(got.Packages[0].USNs) != 2 {
		t.Fatalf("packages = %+v", got.Packages)
	}
	if got.Flaws != 3 || got.Severity != "high" {
		t.Errorf("flaws %d, severity %q", got.Flaws, got.Severity)
	}
}

func TestMatchFlaws_OnlyTheFixesAboveTheInstalledVersionCount(t *testing.T) {
	got := MatchFlaws(flawIndex(), map[string]string{"openssl": "3.0.13-0ubuntu3.1"}, nil, flawsNow)
	if len(got.Packages) != 1 || len(got.Packages[0].USNs) != 1 || got.Packages[0].USNs[0] != "USN-2-1" || got.Flaws != 2 {
		t.Errorf("result = %+v", got)
	}
	if none := MatchFlaws(flawIndex(), map[string]string{"openssl": "3.0.13-0ubuntu3.2"}, nil, flawsNow); len(none.Packages) != 0 || none.Flaws != 0 {
		t.Errorf("a package at the fixed version is current: %+v", none)
	}
	if none := MatchFlaws(flawIndex(), map[string]string{"openssl": "1:1.0"}, nil, flawsNow); len(none.Packages) != 0 {
		t.Errorf("a higher epoch is newer: %+v", none)
	}
}

func TestMatchFlaws_ANoticeIssuedInTheLastTwoDaysIsNotCountedYet(t *testing.T) {
	if got := MatchFlaws(flawIndex(), map[string]string{"libfoo": "0.9"}, nil, flawsNow); len(got.Packages) != 0 {
		t.Errorf("a notice from today was counted: %+v", got)
	}
	if got := MatchFlaws(flawIndex(), map[string]string{"libfoo": "0.9"}, nil, flawsNow.Add(72*time.Hour)); len(got.Packages) != 1 {
		t.Errorf("after the grace it counts: %+v", got)
	}
}

func TestMatchFlaws_AFlawOnCisasListIsCountedAndMakesItHigh(t *testing.T) {
	idx := flawIndex()
	n := idx.Notices["USN-1-1"]
	idx.Notices["USN-1-1"] = n
	got := MatchFlaws(idx, map[string]string{"curl": "8.5.0-2ubuntu10.4"}, map[string]string{"CVE-2026-0001": "2026-03-03"}, flawsNow)
	if got.Exploited != 1 || got.Severity != "kev" || got.Flaws != 1 {
		t.Errorf("result = %+v", got)
	}
}

func TestMatchFlaws_NamesTheBusiestPackagesFirstAndANoticeWithNoCveStillCounts(t *testing.T) {
	idx := models.UbuntuIndex{Notices: map[string]models.UbuntuNotice{"USN-9-1": notice("2026-01-01", "")}, Packages: map[string][]models.UbuntuFix{"zed": {{Fixed: "2", USN: "USN-9-1"}}}}
	got := MatchFlaws(idx, map[string]string{"zed": "1"}, nil, flawsNow)
	if len(got.Packages) != 1 || got.Flaws != 0 || got.Severity != "" {
		t.Errorf("result = %+v", got)
	}
}

func TestMatchFlaws_AnEmptyIndexOrListFindsNothing(t *testing.T) {
	if got := MatchFlaws(models.UbuntuIndex{}, map[string]string{"a": "1"}, nil, flawsNow); len(got.Packages) != 0 {
		t.Error("an empty index found packages")
	}
	if got := MatchFlaws(flawIndex(), nil, nil, flawsNow); len(got.Packages) != 0 {
		t.Error("an empty list found packages")
	}
}

func TestFlawFinding_SaysHowManyPackagesAndFlawsNamesTenAndTheSeverityFollowsTheWorstFlaw(t *testing.T) {
	var names []string
	for _, n := range "abcdefghijklm" {
		names = append(names, "pkg-"+string(n))
	}
	f := flawFinding(models.HostFlaws{Packages: 13, Names: names[:10], Flaws: 42, Exploited: 0, Severity: "high"})
	for _, want := range []string{"13 installed packages", "42 published security flaws", "pkg-a", "pkg-j", "and 3 more"} {
		if !strings.Contains(f.Evidence, want) {
			t.Errorf("evidence lacks %q: %s", want, f.Evidence)
		}
	}
	if f.Check != CheckHostPackagesVulnerable || f.Device != HostDevice || f.Severity != SeverityMedium || f.Port != 0 {
		t.Errorf("finding = %+v", f)
	}
	if hi := flawFinding(models.HostFlaws{Packages: 1, Names: []string{"curl"}, Flaws: 1, Exploited: 1, Severity: "kev"}); hi.Severity != SeverityHigh || !strings.Contains(hi.Evidence, "CISA") || !strings.Contains(hi.Evidence, "1 installed package ") {
		t.Errorf("a flaw attackers use is high and says so: %+v", hi)
	}
	if lo := flawFinding(models.HostFlaws{Packages: 2, Names: []string{"a", "b"}, Flaws: 2, Severity: "medium"}); lo.Severity != SeverityLow {
		t.Errorf("a medium flaw is low here: %+v", lo)
	}
	if _, ok := GuideFor(CheckHostPackagesVulnerable); !ok || CheckTitle(CheckHostPackagesVulnerable) == "" {
		t.Error("the check needs a title and a guide")
	}
}

type fakeJudge struct {
	result FlawResult
	ok     bool
}

func (f fakeJudge) Judge(map[string]string) (FlawResult, bool) { return f.result, f.ok }

func flawLook(t *testing.T, src *fakeHost, store *memHostStore, j FlawJudge) HostLook {
	t.Helper()
	look, err := newWatcher(src, store).WithFlaws(j).Look(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return look
}

func flawFindings(look HostLook) []Finding {
	var out []Finding
	for _, f := range look.Findings {
		if f.Check == CheckHostPackagesVulnerable {
			out = append(out, f)
		}
	}
	return out
}

func TestHostWatch_PackagesBehindTheirFixesAreOneFindingAndAnUpToDateComputerIsNone(t *testing.T) {
	src := &fakeHost{packages: map[string]string{"openssl": "3.0.13-0ubuntu3"}}
	judge := fakeJudge{ok: true, result: FlawResult{Packages: []FlawPackage{{Name: "openssl", USNs: []string{"USN-1-1"}}}, Flaws: 2, Severity: "high"}}
	if got := flawFindings(flawLook(t, src, &memHostStore{}, judge)); len(got) != 1 {
		t.Fatalf("findings = %+v", got)
	}
	if got := flawFindings(flawLook(t, src, &memHostStore{}, fakeJudge{ok: true})); len(got) != 0 {
		t.Errorf("nothing is behind: %+v", got)
	}
}

func TestHostWatch_WithNoSecurityDataOrAnUnsupportedReleaseNothingIsSaidAboutPackages(t *testing.T) {
	src := &fakeHost{packages: map[string]string{"openssl": "1"}}
	if got := flawFindings(flawLook(t, src, &memHostStore{}, fakeJudge{ok: false})); len(got) != 0 {
		t.Errorf("findings = %+v", got)
	}
	if got := flawFindings(integrityLook(t, &fakeHost{}, &memHostStore{})); len(got) != 0 {
		t.Errorf("without a judge nothing is judged: %+v", got)
	}
}

func TestHostWatch_APackageListThatCannotBeReadKeepsTheEarlierFindingAndIsCounted(t *testing.T) {
	store := &memHostStore{}
	src := &fakeHost{packages: map[string]string{"openssl": "1"}}
	judge := fakeJudge{ok: true, result: FlawResult{Packages: []FlawPackage{{Name: "openssl"}}, Flaws: 2, Severity: "high"}}
	flawLook(t, src, store, judge)
	src.packages, src.packagesErr = nil, errors.New("permission denied")
	look := flawLook(t, src, store, judge)
	if got := flawFindings(look); len(got) != 1 || look.Unreadable == 0 {
		t.Errorf("findings %+v, unreadable %d", got, look.Unreadable)
	}
	src.packagesErr = nil
	src.packages = map[string]string{"openssl": "9"}
	if got := flawFindings(flawLook(t, src, store, fakeJudge{ok: true})); len(got) != 0 {
		t.Errorf("a clean judgement resolves it: %+v", got)
	}
	src.packages, src.packagesErr = nil, errors.New("again")
	if got := flawFindings(flawLook(t, src, store, judge)); len(got) != 0 {
		t.Errorf("after a clean judgement nothing is carried: %+v", got)
	}
}

var _ = hostwatch.Integrity{}

func TestPackageFlaws_JudgesOnlyWhenTheDataIsForThisComputersRelease(t *testing.T) {
	judge := func(idx models.UbuntuIndex, release string) (FlawResult, bool) {
		return PackageFlaws{
			Index: func() models.UbuntuIndex { return idx }, Release: func() string { return release },
			Exploited: func() map[string]string { return map[string]string{"CVE-2026-0001": "2026-03-03"} }, Now: func() time.Time { return flawsNow },
		}.Judge(map[string]string{"curl": "8.5.0-2ubuntu10.4"})
	}
	if res, ok := judge(flawIndex(), "noble"); !ok || len(res.Packages) != 1 || res.Exploited != 1 {
		t.Errorf("matching release: %+v, %v", res, ok)
	}
	if _, ok := judge(flawIndex(), "jammy"); ok {
		t.Error("data for another release was used")
	}
	if _, ok := judge(flawIndex(), ""); ok {
		t.Error("a computer that is not Ubuntu was judged")
	}
	if _, ok := judge(models.UbuntuIndex{Release: "noble"}, "noble"); ok {
		t.Error("an empty index was used to say everything is fine")
	}
}
