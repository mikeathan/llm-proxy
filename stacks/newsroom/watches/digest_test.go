package watches

import (
	"strings"
	"testing"
	"time"
)

// A digest is the model's own short headline and one-or-two-sentence summary of an item. It is optional: an answer without one changes nothing.
func TestParseAnalysisKeepsCheckedDigests(t *testing.T) {
	long := strings.Repeat("w", 400)
	raw := `{"digests":[{"item":1,"headline":"  GPT-Live-1.1 adds voice  ","summary":"OpenAI shipped a full-duplex voice model."},{"item":9,"headline":"nope","summary":"not shown"},{"item":2,"headline":"","summary":""},{"item":3,"headline":"` + long + `","summary":"` + long + `"}]}`
	a, err := parseAnalysis(raw, 3, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if d := a.Digests[1]; d.Headline != "GPT-Live-1.1 adds voice" || d.Summary != "OpenAI shipped a full-duplex voice model." {
		t.Errorf("digest 1: %+v", d)
	}
	if _, ok := a.Digests[9]; ok {
		t.Error("a digest must name a shown item")
	}
	if _, ok := a.Digests[2]; ok {
		t.Error("an empty digest is not kept")
	}
	if d := a.Digests[3]; len([]rune(d.Headline)) > maxHeadlineRune || len([]rune(d.Summary)) > maxSummaryRune {
		t.Errorf("digest 3 is not bounded: %d %d", len([]rune(d.Headline)), len([]rune(d.Summary)))
	}
}

// A judged answer with verdicts and no digests still filters: the digests are only the nicer title.
func TestAJudgedAnswerWithoutDigestsStillFilters(t *testing.T) {
	a, err := parseAnalysis(`{"verdicts":[{"item":1,"why":"x","relevant":true},{"item":2,"why":"y","relevant":false}]}`, 2, 0, true)
	if err != nil || !a.Judged || len(a.Matches) != 1 || len(a.Digests) != 0 {
		t.Fatalf("%+v %v", a, err)
	}
}

func TestAnAnswerWithoutDigestsStillParses(t *testing.T) {
	a, err := parseAnalysis(`{"state":"s"}`, 2, 0, false)
	if err != nil || len(a.Digests) != 0 {
		t.Fatalf("%+v %v", a, err)
	}
}

func TestThePromptAsksForADigestOnEveryItem(t *testing.T) {
	if !strings.Contains(system, "digests") || !strings.Contains(system, "headline") || !strings.Contains(system, "summary") {
		t.Error("the system prompt must ask for digests")
	}
}

func TestTheFeedEntryKeepsTheHeadlineSummaryAndTheOriginalTitleAndLink(t *testing.T) {
	m := &scriptedModel{answers: []string{`{}`, `{"verdicts":[{"item":1,"why":"a voice model","relevant":true}],"digests":[{"item":1,"headline":"GPT-Live-1.1 listens while it talks","summary":"A full-duplex voice model in the API."}]}`}}
	h := relevanceHarness(t, "new AI models", m)
	h.fetch.bodies[feedURL] = itemsRSS("seed")
	h.tick(t, 0)
	h.forgetFirstFindings()
	h.fetch.bodies[feedURL] = itemsRSS("seed", "OpenAI GPT-Live-1.1 listens while it talks and a very long original post title")
	h.tick(t, time.Hour)
	cur := h.store.Feed(h.now, time.UTC, 0, Filter{})[0].Watches[0].Current
	if cur.Headline != "GPT-Live-1.1 listens while it talks" || cur.Summary != "A full-duplex voice model in the API." {
		t.Errorf("digest not stored: %+v", cur)
	}
	if !strings.HasPrefix(cur.Title, "OpenAI GPT-Live-1.1") || cur.Link == "" || cur.Why != "a voice model" {
		t.Errorf("the original title, link and reason must be kept: %+v", cur)
	}
}
