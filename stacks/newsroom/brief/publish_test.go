package brief

import (
	"context"
	"strings"
	"testing"
	"time"

	stacksdk "llm-proxy-stacks/sdk"
)

// publishRig is the collector rig with a pool already collected, and a publisher over the same store and clock.
func publishRig(t *testing.T, edition func(*Edition)) (*rig, *Publisher, *fakeModel) {
	t.Helper()
	r := newRig(t, edition)
	r.tickAt(t, 5, 0) // collects the topic into the pool at 05:00
	m := &fakeModel{}
	return r, &Publisher{Timing: r.col.Timing, Store: r.store, Model: m}, m
}

func (r *rig) at(h, m int) { r.now = time.Date(2026, 11, 21, h, m, 0, 0, time.UTC) }

func tick(t *testing.T, p *Publisher) []Made {
	t.Helper()
	out, err := p.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// An edition is written once for each ready time: not before the lead, once inside it, and never again for the same time.
func TestPublisher_WritesOnceStartingBeforeReadyBy(t *testing.T) {
	r, p, _ := publishRig(t, nil)
	r.at(6, 30)
	if got := tick(t, p); len(got) != 0 {
		t.Fatalf("too early (the lead is %s): %+v", PublishLead, got)
	}
	r.at(6, 45)
	got := tick(t, p)
	if len(got) != 1 {
		t.Fatalf("inside the lead the edition is written: %+v", got)
	}
	is, err := r.store.Issue(got[0].IssueID)
	if err != nil || len(is.Sections) != 1 || len(is.Sections[0].Stories) == 0 || is.ReadyFor.Hour() != 7 {
		t.Fatalf("stored edition: %+v, %v", is, err)
	}
	r.at(6, 55)
	if again := tick(t, p); len(again) != 0 {
		t.Errorf("an edition already written for 07:00 is not written again: %+v", again)
	}
	r.at(7, 30)
	if again := tick(t, p); len(again) != 0 {
		t.Errorf("nor after its ready time: %+v", again)
	}
}

// A service that was stopped at ready time still writes the edition when it comes back within the catch-up window, and not after it.
func TestPublisher_CatchesUpAfterAStopButNotForever(t *testing.T) {
	r, p, _ := publishRig(t, nil)
	r.at(9, 30)
	if got := tick(t, p); len(got) != 1 {
		t.Fatalf("2.5 hours late is inside the catch-up: %+v", got)
	}
	r2, p2, _ := publishRig(t, nil)
	r2.at(11, 0)
	if got := tick(t, p2); len(got) != 0 {
		t.Errorf("4 hours late is past the catch-up of %s: %+v", PublishCatchUp, got)
	}
}

func TestPublisher_AnEditionSwitchedOffIsNotWritten(t *testing.T) {
	r, p, _ := publishRig(t, func(e *Edition) { e.Enabled = false })
	r.at(6, 50)
	if got := tick(t, p); len(got) != 0 {
		t.Errorf("a disabled edition: %+v", got)
	}
}

// What one edition showed is not in the next one made inside a day, and a manual edition has no ready time so it never blocks the scheduled one.
func TestPublisher_TheNextEditionDoesNotRepeatTheLast(t *testing.T) {
	r, p, _ := publishRig(t, nil)
	r.at(6, 0)
	first, err := p.MakeNow(context.Background(), 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Sections[0].Stories) == 0 || !first.ReadyFor.IsZero() {
		t.Fatalf("manual edition: %+v", first)
	}
	r.at(6, 50)
	got := tick(t, p)
	if len(got) != 1 {
		t.Fatalf("the scheduled edition is still written: %+v", got)
	}
	second, _ := r.store.Issue(got[0].IssueID)
	if n := len(second.Sections[0].Stories); n != 0 {
		t.Errorf("every story was shown an hour ago and nothing new reported it: %d shown again", n)
	}
}

func TestPublisher_AMissingEditionIsNotFound(t *testing.T) {
	_, p, _ := publishRig(t, nil)
	if _, err := p.MakeNow(context.Background(), 999, nil); err == nil {
		t.Error("no such edition")
	}
}

// The made editions and the shown record survive a restart, and old ones are dropped.
func TestStore_IssuesPersistAndAreRetained(t *testing.T) {
	r, p, _ := publishRig(t, nil)
	r.at(6, 0)
	is, err := p.MakeNow(context.Background(), 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	back, err := NewStore(r.store.dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := back.Issues(2); len(got) != 1 || got[0].ID != is.ID || len(back.Snapshot().Shown) == 0 {
		t.Fatalf("after a restart: %+v", got)
	}
	later := r.now.Add(IssueRetention + time.Hour)
	if _, err := back.SaveIssue(Issue{EditionID: 2, MadeAt: later}, later); err != nil {
		t.Fatal(err)
	}
	if got := back.Issues(0); len(got) != 1 || got[0].ID == is.ID {
		t.Errorf("an edition past its retention is dropped: %+v", got)
	}
}

// "Home" is stored on the issue only when the edition asks, and is read after the sections are written: the model is never shown it.
func TestPublisher_HomeBlockIsStoredAndNeverShownToTheModel(t *testing.T) {
	r, p, m := publishRig(t, func(e *Edition) { e.Home = true })
	line := stacksdk.HomeLine{ID: "network_online", Count: 3, Words: "SECRET-HOME-WORDS"}
	p.Home = func(context.Context) []stacksdk.HomeLine { return []stacksdk.HomeLine{line} }
	r.at(6, 45)
	is, err := p.MakeNow(context.Background(), r.store.Editions()[0].ID, nil)
	if err != nil || len(is.Home) != 1 || is.Home[0] != line {
		t.Fatalf("home block: %+v, %v", is.Home, err)
	}
	if len(m.prompts) == 0 {
		t.Fatal("the model was never asked, so this proves nothing")
	}
	for _, prompt := range m.prompts {
		if strings.Contains(prompt, "SECRET-HOME-WORDS") || strings.Contains(prompt, "network_online") {
			t.Errorf("the model saw the home facts: %q", prompt)
		}
	}
	r2, p2, _ := publishRig(t, nil)
	p2.Home = func(context.Context) []stacksdk.HomeLine { return []stacksdk.HomeLine{line} }
	r2.at(6, 45)
	if is, _ := p2.MakeNow(context.Background(), r2.store.Editions()[0].ID, nil); len(is.Home) != 0 {
		t.Errorf("an edition that did not ask has no block: %+v", is.Home)
	}
}

// "Your watches" is in an edition only when the edition asks for it, and it covers the time since the edition before it.
func TestPublisher_WatchesBlockOnlyWhenAsked(t *testing.T) {
	var windows [][2]time.Time
	hook := func(since, until time.Time) []WatchLine {
		windows = append(windows, [2]time.Time{since, until})
		return []WatchLine{{Watch: "AI news", New: 2, Headline: "A model shipped"}}
	}
	r, p, _ := publishRig(t, func(e *Edition) { e.Watches = true })
	p.Watches = hook
	r.at(6, 45)
	first, err := p.MakeNow(context.Background(), r.store.Editions()[0].ID, nil)
	if err != nil || len(first.Watches) != 1 || first.Watches[0].Headline != "A model shipped" {
		t.Fatalf("the block is missing: %+v, %v", first.Watches, err)
	}
	if got := windows[0][1].Sub(windows[0][0]); got != WatchesWindow {
		t.Errorf("the first edition looks back %s, want %s", got, WatchesWindow)
	}
	r.at(8, 0)
	if _, err := p.MakeNow(context.Background(), r.store.Editions()[0].ID, nil); err != nil {
		t.Fatal(err)
	}
	if !windows[1][0].Equal(first.MadeAt) {
		t.Errorf("the next edition starts where the last was made: %v, want %v", windows[1][0], first.MadeAt)
	}

	r2, p2, _ := publishRig(t, nil)
	p2.Watches = hook
	r2.at(6, 45)
	if is, _ := p2.MakeNow(context.Background(), r2.store.Editions()[0].ID, nil); len(is.Watches) != 0 {
		t.Errorf("an edition that did not ask has no block: %+v", is.Watches)
	}
}
