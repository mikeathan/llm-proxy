package automation

import (
	"strings"
	"testing"
	"time"

	"llm-proxy/models"
)

// The two reports below are RECONSTRUCTED from the shape of the 2026-10-12 vertex
// heartbeat audit (five items in run 1; the same items again in run 2, one of them
// under a different source link), not copies of the real run history, which is not
// in the repo. They pin what buildDigest does today, so the Topic Watch plan
// (docs/PLANS/autonomy/topic-watch.md) knows what the URL-keyed ledger can and
// cannot do before a changed-fact step is built on top of it.
const watchRun1 = `Five items found this run.

- Qwen4 released with open weights — https://qwen.ai/blog/qwen4
- Llama 5 announced — https://ai.meta.com/blog/llama-5/?utm_source=feed
- Mistral cuts API prices — https://mistral.ai/news/pricing
- GPT-5.5 retirement date set — https://orcarouter.ai/news/gpt-55-retirement
- Gemma 4 preview — https://ai.google.dev/gemma4

Run notes: 3 searches.`

const watchRun2 = `Five items found this run.

- Qwen4 released with open weights — https://qwen.ai/blog/qwen4#intro
- Llama 5 announced — https://ai.meta.com/blog/llama-5/
- Mistral cuts API prices — https://mistral.ai/news/pricing
- GPT-5.5 retirement date set — https://trustloud.com/gpt-55-retired
- Gemma 4 preview — https://ai.google.dev/gemma4

Run notes: 4 searches.`

func TestBuildDigest_ReplayOfTwoRuns(t *testing.T) {
	t0 := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	cfg := models.NotifyConfig{Connector: "tg", Dedup: true}

	run1 := buildDigest(watchRun1, nil, cfg, t0)
	if len(run1.NewItems) != 5 {
		t.Fatalf("run 1 recorded %d items, want 5: %v", len(run1.NewItems), run1.NewItems)
	}

	// A send succeeded: the framework records run 1's items.
	ledger := mergeSeen(nil, run1.NewItems, t0, cfg.RetentionDays())
	run2 := buildDigest(watchRun2, ledger, cfg, t0.Add(6*time.Hour))

	for _, suppressed := range []string{"Qwen4", "Llama 5", "Mistral", "Gemma 4"} {
		if strings.Contains(run2.Message, suppressed) {
			t.Errorf("%s came back under the same canonical link and must be suppressed:\n%s", suppressed, run2.Message)
		}
	}

	// Known limit (heartbeat-seen-ledger.md): the same event under a new source URL is
	// a new key, so it is sent twice. If this starts failing, identity got smarter:
	// update the Topic Watch plan, do not just flip the assertion.
	if !strings.Contains(run2.Message, "GPT-5.5 retirement") || len(run2.NewItems) != 1 {
		t.Errorf("different-link repeat should still be sent once more; NewItems=%v\n%s", run2.NewItems, run2.Message)
	}

	// Known limit: the model's framing prose survives when rows are dropped.
	for _, prose := range []string{"Five items found this run.", "Run notes: 4 searches."} {
		if !strings.Contains(run2.Message, prose) {
			t.Errorf("framing line %q is expected to survive today:\n%s", prose, run2.Message)
		}
	}
}

func TestBuildDigest_FailedSendRecordsNothing(t *testing.T) {
	t0 := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	cfg := models.NotifyConfig{Connector: "tg", Dedup: true}

	// buildDigest only proposes NewItems; a failed send never merges them, so the
	// same report is a full digest again on the next run.
	first := buildDigest(watchRun1, nil, cfg, t0)
	again := buildDigest(watchRun1, nil, cfg, t0.Add(6*time.Hour))
	if first.Message != again.Message || len(again.NewItems) != 5 {
		t.Errorf("an unrecorded run must be retried whole; NewItems=%d", len(again.NewItems))
	}
}
