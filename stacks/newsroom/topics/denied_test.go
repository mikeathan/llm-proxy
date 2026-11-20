package topics

import (
	"context"
	"strings"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

type stubAnalyzer struct{ err error }

func (s stubAnalyzer) Analyze(context.Context, Input) (Analysis, error) { return Analysis{}, s.err }

// The feed always records a change. When the stack's daily limit of model calls is spent the items are announced as new with no comparison and the row
// says so, instead of the topic going quiet until midnight.
func TestASpentModelLimitStillRecordsTheItemsInTheFeed(t *testing.T) {
	topic := feedTopic()
	topic.Channel = ""
	h := newHarness(t, topic)
	h.fetch.bodies[feedURL] = rss([2]string{"First", "https://news.example/1"})
	h.run.Run(context.Background(), mustGet(t, h))
	h.now = h.now.Add(2 * time.Hour)
	h.fetch.bodies[feedURL] = rss([2]string{"First", "https://news.example/1"}, [2]string{"Second", "https://news.example/2"})
	h.run.Analyzer = stubAnalyzer{err: stacksdk.Errorf(stacksdk.CodeDenied, "limit")}

	o := h.run.Run(context.Background(), mustGet(t, h))
	if o.Status != StatusInFeed || o.New != 1 || !strings.Contains(o.Detail, "not compared") {
		t.Fatalf("outcome = %+v", o)
	}
	if cards := h.store.Feed(h.now, nil, 0, Filter{}); len(cards) != 1 || cards[0].Updates != 1 {
		t.Errorf("the item must be in the feed, got %+v", cards)
	}
}

// With no model chosen the core refuses the same way, and the row says why in the core's words.
func TestNoModelChosenAnnouncesItemsAsNewAndSaysWhy(t *testing.T) {
	h := newHarness(t, feedTopic())
	h.fetch.bodies[feedURL] = rss([2]string{"First", "https://news.example/1"})
	h.run.Run(context.Background(), mustGet(t, h))
	h.now = h.now.Add(2 * time.Hour)
	h.fetch.bodies[feedURL] = rss([2]string{"First", "https://news.example/1"}, [2]string{"Second", "https://news.example/2"})
	h.run.Analyzer = stubAnalyzer{err: stacksdk.Errorf(stacksdk.CodeDenied, "no model is chosen")}
	if o := h.run.Run(context.Background(), mustGet(t, h)); o.New != 1 || !strings.Contains(o.Detail, "not compared: no model is chosen") {
		t.Fatalf("outcome = %+v", o)
	}
}

// Any other failure of the model step stops the run and records nothing, so the next one tries again.
func TestATransientModelFailureStillRecordsNothingAndRetries(t *testing.T) {
	h := newHarness(t, feedTopic())
	h.fetch.bodies[feedURL] = rss([2]string{"First", "https://news.example/1"})
	h.run.Run(context.Background(), mustGet(t, h))
	h.now = h.now.Add(2 * time.Hour)
	h.fetch.bodies[feedURL] = rss([2]string{"First", "https://news.example/1"}, [2]string{"Second", "https://news.example/2"})
	h.run.Analyzer = stubAnalyzer{err: stacksdk.Errorf(stacksdk.CodeUnavailable, "down")}
	if o := h.run.Run(context.Background(), mustGet(t, h)); o.Status != StatusAnalysisFailed {
		t.Fatalf("outcome = %+v", o)
	}
	if cards := h.store.Feed(h.now, nil, 0, Filter{}); len(cards) != 0 {
		t.Errorf("nothing is recorded, got %+v", cards)
	}
}

func TestEraseForgetsEverythingAndNumbersFromOneAgain(t *testing.T) {
	h := newHarness(t, feedTopic())
	h.fetch.bodies[feedURL] = rss([2]string{"First", "https://news.example/1"})
	h.run.Run(context.Background(), mustGet(t, h))
	if err := h.store.Erase(); err != nil {
		t.Fatal(err)
	}
	if len(h.store.List()) != 0 || h.store.SeenCount(h.id) != 0 {
		t.Errorf("topics = %v, seen = %d", h.store.List(), h.store.SeenCount(h.id))
	}
	made, err := h.store.Create(feedTopic(), h.now)
	if err != nil || made.ID != 1 {
		t.Errorf("created = %+v, %v", made, err)
	}
}
