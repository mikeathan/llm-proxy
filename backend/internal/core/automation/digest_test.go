package automation

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

const briefReport = `As of: 2026-10-03 | Searches: 3 | Window: latest available

| Item | Date | Type | Why it matters | Source |
|---|---|---|---|---|
| GPT-6 | 2026-10-02 | model | Big jump in reasoning | [OpenAI](https://openai.com/blog/gpt-6?utm_source=x) |
| Qwen4 | 2026-10-01 | model | Open weights, strong coding | https://qwen.ai/blog/qwen4#intro |
| Gizmo | n/d | tool | No link row | |

Read: GPT-6 changes pricing.`

func TestCanonicalURL(t *testing.T) {
	cases := map[string]string{
		"https://OpenAI.com/blog/gpt-6/?utm_source=x&b=1#top": "https://openai.com/blog/gpt-6?b=1",
		"https://www.example.com/a":                           "https://example.com/a",
		"https://example.com/a.":                              "https://example.com/a",
		"not a url":                                           "not a url",
	}
	for in, want := range cases {
		if got := canonicalURL(in); got != want {
			t.Errorf("canonicalURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCanonicalURL_KeepsMeaningfulParams(t *testing.T) {
	// "ref" selects a different page on code hosts; only pure tracking params go.
	a := canonicalURL("https://github.com/a/b?ref=main")
	b := canonicalURL("https://github.com/a/b?ref=dev")
	if a == b {
		t.Errorf("distinct ?ref values must stay distinct, both became %q", a)
	}
}

func TestBuildDigest(t *testing.T) {
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	gptKey := canonicalURL("https://openai.com/blog/gpt-6")
	cfgDedup := models.NotifyConfig{Connector: "tg", Dedup: true}

	t.Run("formats table as list and records new items", func(t *testing.T) {
		d := buildDigest(briefReport, nil, cfgDedup, now)
		for _, want := range []string{"• GPT-6 — 2026-10-02 · model · Big jump in reasoning", "https://openai.com/blog/gpt-6?utm_source=x", "• Gizmo", "Read: GPT-6 changes pricing.", "As of: 2026-10-03"} {
			if !strings.Contains(d.Message, want) {
				t.Errorf("message missing %q:\n%s", want, d.Message)
			}
		}
		if strings.Contains(d.Message, "|---") {
			t.Errorf("table markup leaked into chat message:\n%s", d.Message)
		}
		if len(d.NewItems) != 2 || d.NewItems[gptKey] != "GPT-6" {
			t.Errorf("NewItems = %v, want GPT-6 and Qwen4 keyed by canonical URL", d.NewItems)
		}
	})

	t.Run("drops rows already reported", func(t *testing.T) {
		ledger := models.SeenLedger{gptKey: {Title: "GPT-6", At: now.Add(-24 * time.Hour)}}
		d := buildDigest(briefReport, ledger, cfgDedup, now)
		if strings.Contains(d.Message, "GPT-6 —") {
			t.Errorf("seen row was not dropped:\n%s", d.Message)
		}
		if !strings.Contains(d.Message, "Qwen4") || !strings.Contains(d.Message, "Gizmo") {
			t.Errorf("fresh rows must survive:\n%s", d.Message)
		}
		if _, ok := d.NewItems[gptKey]; ok {
			t.Error("a seen item must not be re-recorded as new")
		}
	})

	t.Run("expired ledger entries count as unseen", func(t *testing.T) {
		ledger := models.SeenLedger{gptKey: {Title: "GPT-6", At: now.Add(-61 * 24 * time.Hour)}}
		d := buildDigest(briefReport, ledger, cfgDedup, now)
		if !strings.Contains(d.Message, "• GPT-6") {
			t.Errorf("expired entry must not suppress the row:\n%s", d.Message)
		}
	})

	t.Run("nothing new stays silent unless SendEmpty", func(t *testing.T) {
		allSeen := models.SeenLedger{
			gptKey: {At: now},
			canonicalURL("https://qwen.ai/blog/qwen4"): {At: now},
		}
		report := strings.Replace(briefReport, "| Gizmo | n/d | tool | No link row | |\n", "", 1)
		if d := buildDigest(report, allSeen, cfgDedup, now); d.Message != "" {
			t.Errorf("expected no message, got %q", d.Message)
		}
		cfg := cfgDedup
		cfg.SendEmpty = true
		if d := buildDigest(report, allSeen, cfg, now); d.Message != "No new items since the last run." {
			t.Errorf("SendEmpty message = %q", d.Message)
		}
	})

	t.Run("dedup off keeps everything and records nothing", func(t *testing.T) {
		ledger := models.SeenLedger{gptKey: {At: now}}
		d := buildDigest(briefReport, ledger, models.NotifyConfig{Connector: "tg"}, now)
		if !strings.Contains(d.Message, "• GPT-6") || len(d.NewItems) != 0 {
			t.Errorf("dedup off must not filter or record: %q %v", d.Message, d.NewItems)
		}
	})

	t.Run("report without a table is sent whole", func(t *testing.T) {
		d := buildDigest("Nothing notable happened today.", nil, cfgDedup, now)
		if d.Message != "Nothing notable happened today." {
			t.Errorf("message = %q", d.Message)
		}
	})

	t.Run("duplicate link with an empty title is still sent once", func(t *testing.T) {
		report := "| Item | Source |\n|---|---|\n|  | https://x.com/a |\n|  | https://x.com/a |\n"
		d := buildDigest(report, nil, cfgDedup, now)
		if strings.Count(d.Message, "https://x.com/a") != 1 {
			t.Errorf("empty-title duplicate not collapsed:\n%s", d.Message)
		}
	})

	t.Run("stored titles are clipped and stripped of control characters", func(t *testing.T) {
		long := strings.Repeat("A", 500) + "\x07\x1b[31m"
		report := "| Item | Source |\n|---|---|\n| " + long + " | https://x.com/a |\n"
		d := buildDigest(report, nil, cfgDedup, now)
		title := d.NewItems[canonicalURL("https://x.com/a")]
		if n := len([]rune(title)); n == 0 || n > maxTitleRunes {
			t.Errorf("stored title has %d chars, want 1..%d", n, maxTitleRunes)
		}
		if strings.ContainsAny(title, "\x07\x1b") {
			t.Errorf("control characters survived: %q", title)
		}
	})

	t.Run("duplicate link inside one report is sent once", func(t *testing.T) {
		report := "| Item | Source |\n|---|---|\n| A | https://x.com/a |\n| A again | https://x.com/a/ |\n"
		d := buildDigest(report, nil, cfgDedup, now)
		if strings.Count(d.Message, "https://x.com/a") != 1 {
			t.Errorf("duplicate row not collapsed:\n%s", d.Message)
		}
	})

	const bulletReport = "Top items:\n" +
		"- [GPT-6 launches](https://openai.com/blog/gpt-6?utm_source=x) — big jump\n" +
		"  Context: reasoning gains.\n" +
		"- Qwen4 open weights: https://qwen.ai/blog/qwen4\n" +
		"- No link here, just a remark\n" +
		"\nRead: pricing changes."

	t.Run("link bullets are recorded and sent as written", func(t *testing.T) {
		d := buildDigest(bulletReport, nil, cfgDedup, now)
		for _, want := range []string{"[GPT-6 launches]", "Context: reasoning gains.", "Qwen4 open weights", "No link here", "Read: pricing changes."} {
			if !strings.Contains(d.Message, want) {
				t.Errorf("message missing %q:\n%s", want, d.Message)
			}
		}
		if len(d.NewItems) != 2 || !strings.Contains(d.NewItems[gptKey], "GPT-6 launches") {
			t.Errorf("NewItems = %v, want the two link bullets keyed by canonical URL", d.NewItems)
		}
	})

	t.Run("seen bullets are dropped with their indented details", func(t *testing.T) {
		ledger := models.SeenLedger{gptKey: {Title: "GPT-6", At: now.Add(-24 * time.Hour)}}
		d := buildDigest(bulletReport, ledger, cfgDedup, now)
		if strings.Contains(d.Message, "GPT-6") || strings.Contains(d.Message, "reasoning gains") {
			t.Errorf("seen bullet or its detail line survived:\n%s", d.Message)
		}
		for _, want := range []string{"Qwen4", "No link here"} {
			if !strings.Contains(d.Message, want) {
				t.Errorf("fresh content %q must survive:\n%s", want, d.Message)
			}
		}
	})

	t.Run("a nested link bullet under a kept bullet is its own item", func(t *testing.T) {
		nested := "- Releases https://x.com/releases\n  - Alpha https://x.com/alpha\n    note on alpha\n  - Beta https://x.com/beta\n"
		childKey := canonicalURL("https://x.com/alpha")
		first := buildDigest(nested, nil, cfgDedup, now)
		if _, ok := first.NewItems[childKey]; !ok || len(first.NewItems) != 3 {
			t.Errorf("NewItems = %v, want the parent and both nested links recorded", first.NewItems)
		}
		ledger := models.SeenLedger{childKey: {Title: "Alpha", At: now.Add(-24 * time.Hour)}}
		second := buildDigest(nested, ledger, cfgDedup, now)
		if strings.Contains(second.Message, "Alpha") || strings.Contains(second.Message, "note on alpha") {
			t.Errorf("seen nested bullet (or its detail) was re-sent:\n%s", second.Message)
		}
		for _, want := range []string{"Releases", "Beta"} {
			if !strings.Contains(second.Message, want) {
				t.Errorf("fresh content %q must survive:\n%s", want, second.Message)
			}
		}
	})

	t.Run("a report whose bullets were all seen stays silent unless SendEmpty", func(t *testing.T) {
		ledger := models.SeenLedger{gptKey: {At: now}, canonicalURL("https://qwen.ai/blog/qwen4"): {At: now}}
		if d := buildDigest(bulletReport, ledger, cfgDedup, now); d.Message != "" {
			t.Errorf("expected no message, got %q", d.Message)
		}
		cfg := cfgDedup
		cfg.SendEmpty = true
		if d := buildDigest(bulletReport, ledger, cfg, now); d.Message != noNewItemsMessage {
			t.Errorf("SendEmpty message = %q", d.Message)
		}
	})

	t.Run("bullets without a link are never recorded or dropped", func(t *testing.T) {
		d := buildDigest("- one\n- two\n1. three", nil, cfgDedup, now)
		if d.Message != "- one\n- two\n1. three" || len(d.NewItems) != 0 {
			t.Errorf("link-less bullets changed: %q %v", d.Message, d.NewItems)
		}
	})

	t.Run("the same link in two bullets is sent once", func(t *testing.T) {
		d := buildDigest("- A https://x.com/a\n2) A again https://x.com/a/", nil, cfgDedup, now)
		if strings.Count(d.Message, "https://x.com/a") != 1 {
			t.Errorf("duplicate bullet not collapsed:\n%s", d.Message)
		}
	})

	t.Run("a table row and a bullet share one ledger", func(t *testing.T) {
		report := "| Item | Source |\n|---|---|\n| A | https://x.com/a |\n\n- A again https://x.com/a\n"
		d := buildDigest(report, nil, cfgDedup, now)
		if strings.Count(d.Message, "https://x.com/a") != 1 || len(d.NewItems) != 1 {
			t.Errorf("table and bullet not deduplicated together:\n%s\n%v", d.Message, d.NewItems)
		}
	})

	t.Run("dedup off leaves bullets alone", func(t *testing.T) {
		ledger := models.SeenLedger{gptKey: {At: now}}
		d := buildDigest(bulletReport, ledger, models.NotifyConfig{Connector: "tg"}, now)
		if !strings.Contains(d.Message, "GPT-6 launches") || len(d.NewItems) != 0 {
			t.Errorf("dedup off must not filter or record: %q %v", d.Message, d.NewItems)
		}
	})
}

func TestLedgerHelpers(t *testing.T) {
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	ledger := models.SeenLedger{
		"old":  {Title: "Old", At: now.Add(-90 * 24 * time.Hour)},
		"mid":  {Title: "Mid", At: now.Add(-5 * 24 * time.Hour)},
		"new":  {Title: "New", At: now.Add(-1 * time.Hour)},
		"dupe": {Title: "New", At: now.Add(-2 * time.Hour)},
	}

	merged := mergeSeen(ledger, map[string]string{"fresh": "Fresh"}, now, 60)
	if _, ok := merged["old"]; ok {
		t.Error("entries past retention must be pruned")
	}
	if merged["fresh"].Title != "Fresh" || !merged["fresh"].At.Equal(now) {
		t.Errorf("new item not recorded: %+v", merged["fresh"])
	}

	capped := mergeSeen(models.SeenLedger{}, manyItems(maxSeenEntries+50), now, 60)
	if len(capped) != maxSeenEntries {
		t.Errorf("ledger grew to %d entries, want it capped at %d", len(capped), maxSeenEntries)
	}

	got := recentTitles(ledger, now, 60, 2)
	if len(got) != 2 || got[0] != "New" || got[1] != "Mid" {
		t.Errorf("recentTitles = %v, want [New Mid] (newest first, titles unique, capped)", got)
	}
}

func manyItems(n int) map[string]string {
	items := make(map[string]string, n)
	for i := 0; i < n; i++ {
		items[fmt.Sprintf("https://x.com/%d", i)] = "t"
	}
	return items
}
