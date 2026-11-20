package topics

import (
	"context"
	"sync"
	"testing"
	"time"
)

// A manual run from the screen can arrive while the timer's pass is reading the same topic. Both would find the same items unseen, so without
// the runner's lock the notice would go out twice.
func TestTwoRunsAtOnceSendOneNotice(t *testing.T) {
	h := newHarness(t, feedTopic())
	h.fetch.bodies[feedURL] = rss([2]string{"First", "https://news.example/1"})
	if o := h.run.Run(context.Background(), mustGet(t, h)); o.Status != StatusBaseline {
		t.Fatalf("first run = %+v", o)
	}
	h.now = h.now.Add(2 * time.Hour)
	h.fetch.bodies[feedURL] = rss([2]string{"First", "https://news.example/1"}, [2]string{"Second", "https://news.example/2"})

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.run.Run(context.Background(), mustGet(t, h))
		}()
	}
	wg.Wait()
	if len(h.sent) != 1 {
		t.Errorf("the one new item must be sent once, got %d notices", len(h.sent))
	}
}

func TestARunForADeletedTopicDoesNotRecordAnything(t *testing.T) {
	h := newHarness(t, feedTopic())
	topic := mustGet(t, h)
	if err := h.store.Delete(h.id); err != nil {
		t.Fatal(err)
	}
	h.fetch.bodies[feedURL] = rss([2]string{"First", "https://news.example/1"})
	if o := h.run.Run(context.Background(), topic); o.Status != StatusFailing || h.fetch.calls != 0 {
		t.Errorf("run = %+v, fetches = %d", o, h.fetch.calls)
	}
}

func mustGet(t *testing.T, h *harness) Topic {
	t.Helper()
	topic, err := h.store.Get(h.id)
	if err != nil {
		t.Fatal(err)
	}
	return topic
}
