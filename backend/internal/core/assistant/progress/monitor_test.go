package progress

import (
	"fmt"
	"strings"
	"testing"
)

// hit is one search hit with a snippet long enough to carry its own shingles.
func hit(id string) string {
	words := make([]string, 12)
	for i := range words {
		words[i] = fmt.Sprintf("%s%d", id, i)
	}
	return fmt.Sprintf(`{"title":"Title %s","url":"https://example.test/%s","snippet":%q}`, id, id, strings.Join(words, " "))
}

func results(ids ...string) []byte {
	hits := make([]string, len(ids))
	for i, id := range ids {
		hits[i] = hit(id)
	}
	return []byte("[" + strings.Join(hits, ",") + "]")
}

func testMonitor() *Monitor {
	return New(Config{NoticeAfter: 2, StrongAfter: 4, LowGain: 0.2, RewordedGain: 0.5, SameArgs: 0.6})
}

func read(tool string) Call  { return Call{Tool: tool, ReadOnly: true} }
func write(tool string) Call { return Call{Tool: tool} }
func query(tool, q string) Call {
	return Call{Tool: tool, Args: fmt.Sprintf(`{"query":%q}`, q), ReadOnly: true}
}
func fetch(url string) Call {
	return Call{Tool: "fetch", Args: fmt.Sprintf(`{"url":%q}`, url), ReadOnly: true}
}
func fresh(i int) []byte {
	return []byte(fmt.Sprintf(`"page %d has its own words number %d about topic %d"`, i, i*7, i*13))
}
func observeN(m *Monitor, c Call, r []byte, n int) (d Decision) {
	for range n {
		d = m.Observe(c, r)
	}
	return d
}

func TestMonitor_NewInformationIsProgress(t *testing.T) {
	m := testMonitor()
	for _, ids := range [][]string{{"a", "b", "c", "d", "e"}, {"f", "g", "h"}, {"a", "i", "j", "k"}} {
		d := m.Observe(read("t"), results(ids...))
		if d.Level != None || d.Stale != 0 {
			t.Fatalf("results %v carry new sources: want no staleness, got %+v", ids, d)
		}
	}
}

func TestMonitor_RepeatedResultsRaiseTheNoticeOnceThenTheStrongOne(t *testing.T) {
	m := testMonitor()
	m.Observe(read("t"), results("a", "b", "c", "d", "e"))
	var raised []Level
	for range 6 {
		if d := m.Observe(read("t"), results("a", "b", "c", "d", "e")); d.Changed {
			raised = append(raised, d.Level)
		}
	}
	if len(raised) != 2 || raised[0] != Notice || raised[1] != Strong {
		t.Fatalf("each level is raised once, in order: got %v", raised)
	}
}

func TestMonitor_NoticeComesAtTheConfiguredStaleCount(t *testing.T) {
	m := testMonitor()
	m.Observe(read("t"), results("a", "b"))
	if d := m.Observe(read("t"), results("a", "b")); d.Level != None || d.Stale != 1 {
		t.Fatalf("one stale result is no reason to speak: %+v", d)
	}
	d := m.Observe(read("t"), results("a", "b"))
	if d.Level != Notice || !d.Changed || d.Stale != 2 || d.Tool != "t" {
		t.Fatalf("second stale result raises the notice: %+v", d)
	}
}

func TestMonitor_ShortIdenticalResultsAreStale(t *testing.T) {
	m := testMonitor()
	r := []byte(`"Sheffield: ☀️  +8°C\n"`)
	if d := observeN(m, read("t"), r, 3); d.Level != Notice {
		t.Fatalf("a short result seen three times is not new: %+v", d)
	}
}

func TestMonitor_AnIdenticalErrorRepeatedIsStale(t *testing.T) {
	m := testMonitor()
	e := []byte(`{"error":"server returned unexpected status: 503 Service Unavailable"}`)
	if d := observeN(m, read("t"), e, 3); d.Level != Notice {
		t.Fatalf("the same failure again is no progress: %+v", d)
	}
}

func TestMonitor_ProgressResetsAndReArms(t *testing.T) {
	m := testMonitor()
	observeN(m, read("t"), results("a", "b"), 3) // notice
	if d := m.Observe(read("t"), results("x", "y")); d.Level != None || d.Stale != 0 {
		t.Fatalf("new information resets: %+v", d)
	}
	if d := observeN(m, read("t"), results("x", "y"), 2); d.Level != Notice || !d.Changed {
		t.Fatalf("the notice must be raised again after progress: %+v", d)
	}
}

// A call that changes something (a write, a command) is progress, whatever it returns.
func TestMonitor_AMutatingCallResetsEveryStreak(t *testing.T) {
	m := testMonitor()
	observeN(m, read("a"), results("a"), 3)
	observeN(m, read("b"), results("b"), 3)
	if d := m.Observe(write("w"), []byte(`"ok"`)); d.Level != None || d.Stale != 0 {
		t.Fatalf("a mutating call is progress: %+v", d)
	}
	if d := m.Observe(read("a"), results("a")); d.Stale != 1 {
		t.Fatalf("the streak restarts from one: %+v", d)
	}
}

func TestMonitor_NumbersAreInformation(t *testing.T) {
	m := testMonitor()
	m.Observe(read("t"), []byte(`{"temp_c":9.3,"humidity":64,"wind_kph":15.8,"gust_kph":31.7}`))
	if d := m.Observe(read("t"), []byte(`{"temp_c":11.2,"humidity":71,"wind_kph":9.1,"gust_kph":20.4}`)); d.Stale != 0 {
		t.Fatalf("different readings are new information: %+v", d)
	}
}

func TestMonitor_DistinctCountsOnlyUsefulResults(t *testing.T) {
	m := testMonitor()
	m.Observe(read("t"), results("a"))
	m.Observe(read("t"), results("b"))
	m.Observe(read("t"), results("a"))
	m.Observe(read("t"), results("b"))
	if d := m.Observe(read("t"), results("a")); d.Distinct != 2 {
		t.Fatalf("two useful results so far, got %+v", d)
	}
}

// A page fetched between two searches is new information, but it says nothing about whether the searches are
// getting anywhere: each tool's streak is its own.
func TestMonitor_AnInterleavedToolDoesNotResetAnothersStreak(t *testing.T) {
	m := testMonitor()
	m.Observe(read("search"), results("a", "b"))
	m.Observe(read("search"), results("a", "b")) // stale 1
	m.Observe(read("page"), fresh(1))            // new, other tool
	d := m.Observe(read("search"), results("a", "b"))
	if d.Level != Notice || d.Stale != 2 || d.Tool != "search" {
		t.Fatalf("the search streak must survive a fetch in between: %+v", d)
	}
}

// The measured run reworded its search each time, so each result shared hits with the last and was part new (38 %
// and 64 % new): too much to be stale on its own. A call whose arguments mostly repeat an earlier one is judged
// by a stricter bar.
func TestMonitor_ARewordedCallNeedsMoreNewInformation(t *testing.T) {
	m := testMonitor()
	m.Observe(query("search", "weather forecast sheffield monday 19 october 2026"), results("a", "b", "c", "d", "e"))
	// Same content apart from one new hit in five (~20 % new): a different question would carry it as progress.
	if d := m.Observe(query("search", "sheffield weather forecast 19 october 2026 temperature"), results("a", "b", "c", "d", "f")); d.Stale != 1 {
		t.Fatalf("a reworded query returning mostly the same hits is stale: %+v", d)
	}
	if d := m.Observe(query("search", "sheffield weather forecast 19 october 2026 monday rain"), results("a", "b", "c", "g", "h")); d.Stale != 2 || d.Level != Notice {
		t.Fatalf("and again, two in a row: %+v", d)
	}
}

func TestMonitor_ADifferentQueryWithPartlyNewResultsIsProgress(t *testing.T) {
	m := testMonitor()
	m.Observe(query("search", "weather forecast sheffield"), results("a", "b", "c", "d", "e"))
	if d := m.Observe(query("search", "best pizza near leeds station"), results("a", "f", "g", "h", "i")); d.Stale != 0 {
		t.Fatalf("a different question with new hits is progress: %+v", d)
	}
}

// Reading different files returns different content: similar paths must not read as a reworded call.
func TestMonitor_SimilarArgumentsWithNewContentAreProgress(t *testing.T) {
	m := testMonitor()
	for i := range 6 {
		d := m.Observe(fetch(fmt.Sprintf("https://example.test/docs/page%d", i)), fresh(i))
		if d.Stale != 0 {
			t.Fatalf("page %d has new content: %+v", i, d)
		}
	}
}

func TestConfigForSteps_ScalesWithTheBudget(t *testing.T) {
	small, mid, large := ForSteps(10), ForSteps(25), ForSteps(200)
	if small.StrongAfter != minStrongAfter || mid.StrongAfter != 4 || large.StrongAfter != maxStrongAfter {
		t.Errorf("strong thresholds %d/%d/%d for 10/25/200 steps", small.StrongAfter, mid.StrongAfter, large.StrongAfter)
	}
	if small.NoticeAfter != noticeAfter || small.NoticeAfter >= small.StrongAfter {
		t.Errorf("the notice must come first: %+v", small)
	}
}

func TestDecisionNote_StatesTheFactsAndOnlyWhenRaised(t *testing.T) {
	if (Decision{Level: Notice, Changed: false, Stale: 2, Tool: "search"}).Note() != "" {
		t.Error("a level already announced must not repeat")
	}
	n := Decision{Level: Notice, Changed: true, Stale: 2, Distinct: 3, Tool: "search"}.Note()
	if !strings.Contains(n, "search") || !strings.Contains(n, "2") || !strings.Contains(n, "3") || !strings.HasPrefix(strings.TrimSpace(n), "[progress note") {
		t.Errorf("notice text = %q", n)
	}
	s := Decision{Level: Strong, Changed: true, Stale: 4, Tool: "search"}.Note()
	if !strings.Contains(s, "4") || !strings.Contains(s, "search") || !strings.Contains(strings.ToLower(s), "answer") {
		t.Errorf("strong text = %q", s)
	}
}
