package netaudit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type checkInClock struct{ last time.Time }

func (c *checkInClock) LastCheckIn() (time.Time, error) { return c.last, nil }
func (c *checkInClock) MarkCheckIn(at time.Time) error  { c.last = at; return nil }

func TestCheckIn_SendsOnceADayOnlyWhenOnAndSaysWhatStopped(t *testing.T) {
	now := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	on, hist := false, &checkInClock{}
	var sent []string
	var stalls []MonitorStall
	c := NewCheckIn(CheckInDeps{
		Enabled: func() bool { return on }, History: hist, Now: func() time.Time { return now },
		Stalled: func() []MonitorStall { return stalls },
		Notify:  func(_ context.Context, msg string) error { sent = append(sent, msg); return nil },
	})
	c.Tick(t.Context())
	if len(sent) != 0 {
		t.Fatal("off sends nothing")
	}
	on = true
	c.Tick(t.Context())
	c.Tick(t.Context())
	if len(sent) != 1 || !strings.Contains(sent[0], "every monitor that is switched on is working") {
		t.Fatalf("sent = %q; want one all-quiet note", sent)
	}
	now = now.Add(23*time.Hour + 45*time.Minute) // within the hour's slack, so the note does not drift later each day
	stalls = []MonitorStall{{ID: MonitorDNS, Words: "the DNS comparison"}}
	c.Tick(t.Context())
	if len(sent) != 2 || !strings.Contains(sent[1], "the DNS comparison") {
		t.Fatalf("sent = %q; want the next day's note naming the stopped part", sent)
	}
}

func TestCheckIn_AFailedSendIsNotMarked(t *testing.T) {
	hist := &checkInClock{}
	reported := 0
	c := NewCheckIn(CheckInDeps{
		Enabled: func() bool { return true }, History: hist, Stalled: func() []MonitorStall { return nil },
		Notify:  func(context.Context, string) error { return errors.New("down") },
		OnError: func(error) { reported++ },
	})
	c.Tick(t.Context())
	c.Tick(t.Context())
	if !hist.last.IsZero() {
		t.Error("a check-in that was not sent is tried again at the next tick")
	}
	if reported != 1 {
		t.Errorf("reported %d times; a failure that repeats every tick is reported once", reported)
	}
}
