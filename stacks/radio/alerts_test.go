package radio

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

// captureEvents is the Events port of a test: it keeps the alerts the stack published.
type captureEvents struct {
	mu     sync.Mutex
	alerts []stacksdk.Alert
}

func (c *captureEvents) Publish(_ context.Context, e stacksdk.Event) error {
	var a stacksdk.Alert
	if e.Type == stacksdk.EventAlert && json.Unmarshal(e.Data, &a) == nil {
		c.mu.Lock()
		c.alerts = append(c.alerts, a)
		c.mu.Unlock()
	}
	return nil
}

func (c *captureEvents) got() []stacksdk.Alert {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]stacksdk.Alert(nil), c.alerts...)
}

func (r *listenRig) advance(d time.Duration) {
	r.mu.Lock()
	r.now = r.now.Add(d)
	r.mu.Unlock()
}

func TestAWatchOfADayWarnsOnceWhenAFifthOfItIsLeft(t *testing.T) {
	r := newRig(t, `{"enabled":true,"channels":[60]}`, okWindow)
	r.post(t, `{"adapter":"wlx00","hours":24}`)
	waitFor(t, "a wait", func() bool { return len(r.nap) == 1 })
	if n := len(r.ev.got()); n != 0 {
		t.Fatalf("nothing is wrong at the start: %v", r.ev.got())
	}
	r.advance(20 * time.Hour) // four hours left of 24: a fifth is 4.8
	r.step <- struct{}{}
	waitFor(t, "the next wait", func() bool { return len(r.nap) == 2 })
	got := r.ev.got()
	if len(got) != 1 || got[0].ID != AlertWatchEnding || got[0].Hours != 4 {
		t.Fatalf("want one ending alert for 4 hours, got %+v", got)
	}
	r.step <- struct{}{}
	waitFor(t, "a third wait", func() bool { return len(r.nap) == 3 })
	if n := len(r.ev.got()); n != 1 {
		t.Errorf("a watch warns once, got %d alerts", n)
	}
}

func TestAShortWatchDoesNotWarnItIsEnding(t *testing.T) {
	r := newRig(t, `{"enabled":true,"channels":[60]}`, okWindow)
	r.post(t, `{"adapter":"wlx00","hours":4}`)
	waitFor(t, "a wait", func() bool { return len(r.nap) == 1 })
	r.advance(3 * time.Hour)
	r.step <- struct{}{}
	waitFor(t, "the next wait", func() bool { return len(r.nap) == 2 })
	if got := r.ev.got(); len(got) != 0 {
		t.Errorf("a four-hour watch needs no reminder: %+v", got)
	}
}

func TestAWatchThatTheAdapterEndedEarlyTellsTheBellWithTheHoursLeft(t *testing.T) {
	// Four channels: enough windows in one cycle to reach the failure limit.
	r := newRig(t, `{"enabled":true,"channels":[1,6,11,60]}`, func(int, ListenParams) (stacksdk.PrivilegedResult, error) {
		return stacksdk.PrivilegedResult{}, stacksdk.Errorf(stacksdk.CodeUnavailable, "the adapter went away")
	})
	r.post(t, `{"adapter":"wlx00","hours":48}`)
	waitFor(t, "the run to end", func() bool { return !r.state(t).Listening })
	got := r.ev.got()
	if len(got) != 1 || got[0].ID != AlertWatchStopped || got[0].Hours != 48 {
		t.Fatalf("want one stopped alert with 48 hours left, got %+v", got)
	}
}

func TestARestartTellsTheBellOnceThatTheWatchWasStopped(t *testing.T) {
	r := newRig(t, `{"enabled":true,"channels":[60]}`, okWindow)
	r.post(t, `{"adapter":"wlx00","hours":48}`)
	waitFor(t, "a wait", func() bool { return len(r.nap) == 1 })
	r.advance(6 * time.Hour)
	r.restart(t)
	got := r.ev.got()
	if len(got) != 1 || got[0].ID != AlertWatchStopped || got[0].Hours != 42 {
		t.Fatalf("want one stopped alert with 42 hours left, got %+v", got)
	}
	r.restart(t)
	if n := len(r.ev.got()); n != 1 {
		t.Errorf("the same stop is told once, got %d", n)
	}
}

func TestAWatchTheOperatorStoppedTellsNoOne(t *testing.T) {
	r := newRig(t, `{"enabled":true,"channels":[60]}`, okWindow)
	r.post(t, `{"adapter":"wlx00","hours":48}`)
	waitFor(t, "a wait", func() bool { return len(r.nap) == 1 })
	if _, err := r.s.Handle(context.Background(), stacksdk.Request{Method: http.MethodDelete, Path: ListenPath}); err != nil {
		t.Fatal(err)
	}
	r.restart(t)
	if got := r.ev.got(); len(got) != 0 {
		t.Errorf("you stopped it yourself: %+v", got)
	}
}

func TestEveryAlertIDTheStackPublishesIsListed(t *testing.T) {
	ids := AlertIDs()
	if len(ids) != 2 || ids[0] != AlertWatchEnding || ids[1] != AlertWatchStopped {
		t.Errorf("%v", ids)
	}
}
