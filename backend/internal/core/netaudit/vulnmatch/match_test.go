package vulnmatch

import (
	"testing"

	"llm-proxy/models"
)

func index(entries map[string][]models.VulnRange, exploited map[string]string) models.VulnIndex {
	return models.VulnIndex{Entries: entries, Exploited: exploited}
}

func ids(r Result) []string {
	var out []string
	for _, m := range r.Matches {
		out = append(out, m.CVE)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFind_RangeBoundsOpenAndClosed(t *testing.T) {
	idx := index(map[string][]models.VulnRange{
		"a:openbsd:openssh": {
			{CVE: "CVE-2024-0001", EndExcl: "9.8"},
			{CVE: "CVE-2024-0002", StartIncl: "9.6", EndIncl: "9.6p1"},
			{CVE: "CVE-2024-0003", StartExcl: "9.6p1", EndExcl: "9.9"},
			{CVE: "CVE-2024-0004", StartIncl: "9.7"},
			{CVE: "CVE-2024-0005", Exact: "9.6p1"},
			{CVE: "CVE-2024-0006", Exact: "9.5"},
			{CVE: "CVE-2024-0007"},
		},
	}, nil)
	got := ids(Find(idx, []string{"cpe:/a:openbsd:openssh:9.6p1"}))
	want := []string{"CVE-2024-0001", "CVE-2024-0002", "CVE-2024-0005", "CVE-2024-0007"}
	if !equal(sorted(got), want) {
		t.Fatalf("matches = %v, want %v", got, want)
	}
}

func sorted(in []string) []string {
	out := append([]string(nil), in...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestFind_NoVersionNeverMatches(t *testing.T) {
	idx := index(map[string][]models.VulnRange{
		"a:protocol_labs:go-ipfs": {{CVE: "CVE-2024-0001"}},
		"o:linux:linux_kernel":    {{CVE: "CVE-2024-0002", EndExcl: "6.1"}},
	}, nil)
	for _, cpe := range []string{
		"cpe:/a:protocol_labs:go-ipfs",
		"cpe:/o:linux:linux_kernel",
		"cpe:/o:linux:linux_kernel:",
		"cpe:/o:linux:linux_kernel:-",
	} {
		r := Find(idx, []string{cpe})
		if len(r.Matches) != 0 || r.Uncomparable != 0 {
			t.Errorf("%s: got %+v, want nothing", cpe, r)
		}
	}
}

func TestFind_UnknownProductAndMalformedCPEsMatchNothing(t *testing.T) {
	idx := index(map[string][]models.VulnRange{"a:x:y": {{CVE: "CVE-2024-0001"}}}, nil)
	for _, cpe := range []string{"", "cpe:/", "cpe:/a:x", "http://x", "cpe:/z:x:y:1", "cpe:/a:other:y:1", "cpe:/a:x:y:1:2:3:4:5:6:7"} {
		if r := Find(idx, []string{cpe}); len(r.Matches) != 0 {
			t.Errorf("%q: got %+v", cpe, r)
		}
	}
}

func TestFind_KeyIsCaseAndEncodingInsensitive(t *testing.T) {
	idx := index(map[string][]models.VulnRange{"a:f5:nginx+plus": {{CVE: "CVE-2024-0001", EndExcl: "2"}}}, nil)
	if got := ids(Find(idx, []string{"cpe:/a:F5:Nginx%2bPlus:1.5"})); !equal(got, []string{"CVE-2024-0001"}) {
		t.Fatalf("got %v", got)
	}
}

func TestFind_UncomparableIsCountedNotMatched(t *testing.T) {
	idx := index(map[string][]models.VulnRange{
		"a:x:y": {
			{CVE: "CVE-2024-0001", EndExcl: "2.0"},
			{CVE: "CVE-2024-0001", EndExcl: "3.0"},
			{CVE: "CVE-2024-0002", EndExcl: "2.0"},
			{CVE: "CVE-2024-0003", EndExcl: "9.0"},
		},
	}, nil)
	r := Find(idx, []string{"cpe:/a:x:y:1.0-final"})
	if len(r.Matches) != 0 {
		t.Fatalf("matches = %v", ids(r))
	}
	if r.Uncomparable != 3 {
		t.Fatalf("Uncomparable = %d, want 3 (distinct CVEs)", r.Uncomparable)
	}
}

func TestFind_UncomparableRangeOfAMatchedCVEIsNotCounted(t *testing.T) {
	idx := index(map[string][]models.VulnRange{
		"a:x:y": {{CVE: "CVE-2024-0001", Exact: "weird-build"}, {CVE: "CVE-2024-0001", EndExcl: "2.0"}},
	}, nil)
	r := Find(idx, []string{"cpe:/a:x:y:1.5"})
	if got := ids(r); !equal(got, []string{"CVE-2024-0001"}) || r.Uncomparable != 0 {
		t.Fatalf("got %v, uncomparable %d", got, r.Uncomparable)
	}
}

func TestFind_MergesDuplicatesAcrossCPEsAndRanges(t *testing.T) {
	idx := index(map[string][]models.VulnRange{
		"a:x:y": {{CVE: "CVE-2024-0001", Score: 5, EndExcl: "2"}, {CVE: "CVE-2024-0001", Score: 7, EndExcl: "3"}},
		"a:x:z": {{CVE: "CVE-2024-0001", Score: 9, EndExcl: "2"}},
	}, nil)
	r := Find(idx, []string{"cpe:/a:x:y:1", "cpe:/a:x:z:1", "cpe:/a:x:y:1"})
	if len(r.Matches) != 1 || r.Matches[0].Score != 9 {
		t.Fatalf("got %+v, want one match with the highest score 9", r.Matches)
	}
}

func TestFind_OrdersExploitedFirstThenScoreThenID(t *testing.T) {
	idx := index(map[string][]models.VulnRange{
		"a:x:y": {
			{CVE: "CVE-2024-0003", Score: 9.8},
			{CVE: "CVE-2024-0002", Score: 5.0},
			{CVE: "CVE-2024-0001", Score: 5.0},
			{CVE: "CVE-2024-0004", Score: 4.0},
		},
	}, map[string]string{"CVE-2024-0004": "2024-05-01"})
	r := Find(idx, []string{"cpe:/a:x:y:1"})
	want := []string{"CVE-2024-0004", "CVE-2024-0003", "CVE-2024-0001", "CVE-2024-0002"}
	if got := ids(r); !equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	if !r.Matches[0].Exploited || r.Matches[1].Exploited {
		t.Fatalf("exploited flags wrong: %+v", r.Matches)
	}
}

func TestFind_EmptyIndexAndNoCPEs(t *testing.T) {
	if r := Find(models.VulnIndex{}, []string{"cpe:/a:x:y:1"}); len(r.Matches) != 0 {
		t.Fatalf("got %+v", r)
	}
	if r := Find(index(map[string][]models.VulnRange{"a:x:y": {{CVE: "CVE-2024-0001"}}}, nil), nil); len(r.Matches) != 0 {
		t.Fatalf("got %+v", r)
	}
}

func TestFind_VersionedSaysWhetherAnyCPEHadAVersionToCompare(t *testing.T) {
	idx := index(map[string][]models.VulnRange{"a:x:y": {{CVE: "CVE-2024-0001", EndExcl: "2"}}}, nil)
	cases := map[string]bool{
		"cpe:/a:x:y:5":       true, // a version, no match
		"cpe:/a:unknown:z:1": true, // a version, product not in the index
		"cpe:/a:x:y":         false,
		"cpe:/o:linux:linux": false,
		"not a cpe":          false,
	}
	for cpe, want := range cases {
		if got := Find(idx, []string{cpe}).Versioned; got != want {
			t.Errorf("%s: Versioned = %v, want %v", cpe, got, want)
		}
	}
}

// nmap reports some programs by their major version only (Samba as "4"). That could be 4.0 or 4.20, so it must not be
// treated as 4.0 and matched against every range that starts there.
func TestFind_AMajorVersionAloneIsTooCoarseForARange(t *testing.T) {
	idx := index(map[string][]models.VulnRange{
		"a:samba:samba": {
			{CVE: "CVE-2022-0001", StartIncl: "4.0.0", EndExcl: "4.15.13"},
			{CVE: "CVE-2022-0002", EndExcl: "4.17.0"},
			{CVE: "CVE-2022-0003", Exact: "4"},
		},
	}, nil)
	got := Find(idx, []string{"cpe:/a:samba:samba:4"})
	if !equal(ids(got), []string{"CVE-2022-0003"}) {
		t.Errorf("matches = %v, want only the exact one", ids(got))
	}
	if got.Uncomparable != 2 || !got.Versioned {
		t.Errorf("uncomparable %d versioned %v, want the two ranges counted as not comparable", got.Uncomparable, got.Versioned)
	}
	other := index(map[string][]models.VulnRange{"a:samba:samba": {{CVE: "CVE-2010-0001", EndExcl: "3.6"}, {CVE: "CVE-2030-0001", StartIncl: "5.0"}}}, nil)
	if r := Find(other, []string{"cpe:/a:samba:samba:4"}); len(r.Matches) != 0 || r.Uncomparable != 0 {
		t.Errorf("ranges wholly below or above the major version are ruled out, not counted: %+v", r)
	}
	if fine := Find(idx, []string{"cpe:/a:samba:samba:4.13"}); !equal(ids(fine), []string{"CVE-2022-0001", "CVE-2022-0002"}) {
		t.Errorf("a major.minor version is fine to compare: %v", ids(fine))
	}
}

func TestFind_PreciseSaysWhetherAVersionCouldBePlacedInARelease(t *testing.T) {
	idx := index(map[string][]models.VulnRange{"a:x:y": {{CVE: "CVE-2024-0001", EndExcl: "5"}}}, nil)
	for cpe, want := range map[string]bool{"cpe:/a:x:y:4": false, "cpe:/a:x:y:4.2": true, "cpe:/a:x:y:4.2p1": true, "cpe:/a:x:y": false} {
		if got := Find(idx, []string{cpe}).Precise; got != want {
			t.Errorf("%s: Precise = %v, want %v", cpe, got, want)
		}
	}
	if got := Find(idx, []string{"cpe:/a:x:y:4", "cpe:/a:x:y:4.2"}).Precise; !got {
		t.Error("one precise version among the CPEs is enough")
	}
}
