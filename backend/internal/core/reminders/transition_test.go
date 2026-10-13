package reminders

import (
	"errors"
	"testing"
	"time"
)

var (
	t0     = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	policy = Policy{LateWindow: 12 * time.Hour, LateGrace: 2 * time.Minute, MaxAttempts: 3,
		RetryDelays: []time.Duration{time.Minute, 5 * time.Minute}}
)

func oneOff(at time.Time) Reminder {
	return Reminder{ID: 1, Message: "x", FireAt: at, Timezone: "UTC", Status: StatusScheduled}
}

func daily(at time.Time) Reminder {
	r := oneOff(at)
	r.Cron = "0 9 * * *"
	return r
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name    string
		overdue time.Duration
		want    Timing
	}{
		{"on the dot", 0, OnTime},
		{"normal timer jitter", 2 * time.Minute, OnTime},
		{"just past the grace", 2*time.Minute + time.Second, Late},
		{"down for hours", 11 * time.Hour, Late},
		{"exactly the window", 12 * time.Hour, Late},
		{"past the window", 12*time.Hour + time.Second, Missed},
	}
	for _, tc := range cases {
		if got := policy.Classify(oneOff(t0), t0.Add(tc.overdue)); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestAdvance_Delivered(t *testing.T) {
	now := t0.Add(time.Second)

	step := policy.Advance(oneOff(t0), Delivered(OnTime), now)
	if step.Next.Status != StatusDone || step.Event != EventDue {
		t.Errorf("one-off on time: %+v", step)
	}
	if step := policy.Advance(oneOff(t0), Delivered(Late), now); step.Event != EventLate {
		t.Errorf("late delivery event = %q", step.Event)
	}

	step = policy.Advance(daily(t0), Delivered(OnTime), now)
	if step.Next.Status != StatusScheduled || !step.Next.FireAt.Equal(t0.AddDate(0, 0, 1)) {
		t.Errorf("recurring should move to tomorrow: %+v", step.Next)
	}

	r := oneOff(t0)
	r.Attempts, r.LastError = 2, "boom"
	if next := policy.Advance(r, Delivered(OnTime), now).Next; next.Attempts != 0 || next.LastError != "" {
		t.Errorf("a delivery clears the failure record: %+v", next)
	}
}

func TestAdvance_RecurringEndsAtUntil(t *testing.T) {
	r := daily(t0)
	until := t0.Add(time.Hour)
	r.Until = &until
	if step := policy.Advance(r, Delivered(OnTime), t0); step.Next.Status != StatusDone {
		t.Errorf("no occurrence left before until, want done: %+v", step.Next)
	}
}

func TestAdvance_Missed(t *testing.T) {
	now := t0.Add(20 * time.Hour)
	step := policy.Advance(oneOff(t0), Overdue, now)
	if step.Next.Status != StatusMissed || step.Event != EventMissed {
		t.Errorf("one-off: %+v", step)
	}
	step = policy.Advance(daily(t0), Overdue, now)
	if step.Next.Status != StatusScheduled || !step.Next.FireAt.After(now) || step.Event != EventMissed {
		t.Errorf("recurring skips to the next future time and still reports the miss: %+v", step)
	}
}

func TestAdvance_FailedRetriesThenGivesUp(t *testing.T) {
	r := oneOff(t0)
	now := t0
	var delays []time.Duration
	for i := 1; i <= 2; i++ {
		step := policy.Advance(r, Failed("down"), now)
		if step.Event != "" || step.Next.Status != StatusScheduled || step.Next.Attempts != i || step.Next.LastError != "down" {
			t.Fatalf("attempt %d should retry quietly: %+v", i, step)
		}
		delays = append(delays, step.Next.FireAt.Sub(now))
		r = step.Next
	}
	if delays[0] != time.Minute || delays[1] != 5*time.Minute {
		t.Errorf("retry delays = %v, want 1m then 5m", delays)
	}
	step := policy.Advance(r, Failed("down"), now)
	if step.Next.Status != StatusFailed || step.Event != EventFailed || step.Next.Attempts != 3 {
		t.Errorf("third failure ends it: %+v", step)
	}
}

func TestAdvance_FailedRecurringMovesOn(t *testing.T) {
	r := daily(t0)
	r.Attempts = 2
	step := policy.Advance(r, Failed("down"), t0)
	if step.Next.Status != StatusScheduled || step.Event != EventFailed || step.Next.Attempts != 0 || !step.Next.FireAt.After(t0) {
		t.Errorf("a recurring reminder reports the failure and goes on: %+v", step)
	}
}

func TestAdvance_RetryDelayBeyondTheList(t *testing.T) {
	p := policy
	p.MaxAttempts = 5
	r := oneOff(t0)
	r.Attempts = 3
	if got := p.Advance(r, Failed("x"), t0).Next.FireAt.Sub(t0); got != 5*time.Minute {
		t.Errorf("delay past the end of the list repeats the last, got %v", got)
	}
}

func TestAdvance_LongErrorIsTruncated(t *testing.T) {
	long := make([]byte, 1000)
	for i := range long {
		long[i] = 'a'
	}
	if got := policy.Advance(oneOff(t0), Failed(string(long)), t0).Next.LastError; len(got) != maxErrorLength {
		t.Errorf("error length = %d, want %d", len(got), maxErrorLength)
	}
}

func TestAdvance_DoesNotMutateItsInput(t *testing.T) {
	r := oneOff(t0)
	_ = policy.Advance(r, Failed("x"), t0)
	if r.Attempts != 0 || r.LastError != "" || r.Status != StatusScheduled {
		t.Errorf("input changed: %+v", r)
	}
}

func TestPause(t *testing.T) {
	got, err := Pause(daily(t0))
	if err != nil || got.Status != StatusPaused {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if _, err := Pause(oneOff(t0)); !errors.Is(err, ErrInvalid) {
		t.Errorf("a one-off cannot be paused (delete or reschedule it), err = %v", err)
	}
	paused := daily(t0)
	paused.Status = StatusPaused
	if _, err := Pause(paused); !errors.Is(err, ErrInvalid) {
		t.Errorf("an already paused reminder, err = %v", err)
	}
}

func TestResume_MovesToTheNextOccurrenceAfterNow(t *testing.T) {
	paused := daily(t0) // 09:00 daily, due 12 Oct
	paused.Status = StatusPaused
	now := t0.Add(72 * time.Hour).Add(time.Hour) // paused for three days, now 10:00 on the 15th
	got, err := Resume(paused, now)
	if err != nil || got.Status != StatusScheduled || !got.FireAt.Equal(time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %+v err=%v, want scheduled for the 16th (no replay of what was silenced)", got, err)
	}
	if _, err := Resume(daily(t0), now); !errors.Is(err, ErrInvalid) {
		t.Errorf("resuming a reminder that is not paused, err = %v", err)
	}
	ended := paused
	until := t0.Add(time.Hour)
	ended.Until = &until
	if _, err := Resume(ended, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("resuming one whose end date has passed, err = %v", err)
	}
}

func TestSkipNext(t *testing.T) {
	r := daily(t0.Add(24 * time.Hour)) // next is tomorrow 09:00
	got, err := SkipNext(r)
	if err != nil || got.Status != StatusScheduled || !got.FireAt.Equal(t0.Add(48*time.Hour)) {
		t.Fatalf("got %+v err=%v, want the occurrence after the next one", got, err)
	}
	if _, err := SkipNext(oneOff(t0)); !errors.Is(err, ErrInvalid) {
		t.Errorf("a one-off has nothing to skip to, err = %v", err)
	}
	last := daily(t0)
	until := t0.Add(time.Hour)
	last.Until = &until
	if _, err := SkipNext(last); !errors.Is(err, ErrInvalid) {
		t.Errorf("skipping the last occurrence, err = %v (delete it instead)", err)
	}
}

func TestAdvance_PartlyDeliveredClosesAndKeepsWhatFailed(t *testing.T) {
	step := policy.Advance(oneOff(t0), DeliveredPartly(OnTime, "slack: channel not found"), t0)
	if step.Next.Status != StatusDone || step.Event != EventPartial || step.Next.LastError != "slack: channel not found" {
		t.Errorf("one-off: %+v", step)
	}
	if step.Next.Attempts != 0 {
		t.Errorf("a partly delivered reminder is not retried, attempts = %d", step.Next.Attempts)
	}
}

func TestAdvance_PartlyDeliveredRecurringMovesOn(t *testing.T) {
	step := policy.Advance(daily(t0), DeliveredPartly(OnTime, "slack: down"), t0)
	if step.Next.Status != StatusScheduled || !step.Next.FireAt.After(t0) || step.Event != EventPartial {
		t.Errorf("recurring: %+v", step)
	}
	// The next clean delivery clears the failure.
	clean := policy.Advance(step.Next, Delivered(OnTime), step.Next.FireAt)
	if clean.Next.LastError != "" {
		t.Errorf("last_error = %q after a clean delivery", clean.Next.LastError)
	}
}
