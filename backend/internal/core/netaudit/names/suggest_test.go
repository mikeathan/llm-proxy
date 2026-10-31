package names

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"llm-proxy/models"
)

var t0 = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

type memStore struct {
	s      models.NameSuggestions
	writes int
}

func (m *memStore) Read() (models.NameSuggestions, error) { return m.s, nil }
func (m *memStore) Write(s models.NameSuggestions) error  { m.s = s; m.writes++; return nil }

func cand(mac, ip string) Candidate { return Candidate{MAC: mac, Addr: netip.MustParseAddr(ip)} }

func suggester(store Store, ask Asker, now *time.Time) *Suggester {
	s := NewSuggester(store, ask)
	s.now = func() time.Time { return *now }
	return s
}

func answers(name string) Asker {
	return func(context.Context, netip.Addr) (string, error) { return name, nil }
}

func silent(context.Context, netip.Addr) (string, error) { return "", errors.New("no reply") }

func TestSuggester_AsksANewDeviceAndKeepsTheAnswer(t *testing.T) {
	store, now := &memStore{}, t0
	s := suggester(store, answers("Samsung"), &now)
	if err := s.Look(context.Background(), []Candidate{cand("aa:00:00:00:00:01", "192.168.1.5")}); err != nil {
		t.Fatal(err)
	}
	if got := s.For("aa:00:00:00:00:01"); got != "Samsung" {
		t.Errorf("suggestion = %q", got)
	}
	if e := store.s.Entries[0]; e.Misses != 0 || !e.AnsweredAt.Equal(t0) || store.writes != 1 {
		t.Errorf("entry = %+v, writes = %d", e, store.writes)
	}
}

func TestSuggester_DoesNotAskAgainBeforeTheRetryTime(t *testing.T) {
	store, now := &memStore{}, t0
	var asked atomic.Int32
	s := suggester(store, func(context.Context, netip.Addr) (string, error) { asked.Add(1); return "Samsung", nil }, &now)
	c := []Candidate{cand("aa:00:00:00:00:01", "192.168.1.5")}
	_ = s.Look(context.Background(), c)
	now = now.Add(6*24*time.Hour + 23*time.Hour)
	_ = s.Look(context.Background(), c)
	if asked.Load() != 1 {
		t.Fatalf("asked %d times inside the 7 days", asked.Load())
	}
	now = now.Add(2 * time.Hour)
	_ = s.Look(context.Background(), c)
	if asked.Load() != 2 {
		t.Errorf("asked %d times after 7 days, want 2", asked.Load())
	}
}

func TestSuggester_ASilentDeviceIsAskedLessAndLessOftenUpToAWeek(t *testing.T) {
	store, now := &memStore{}, t0
	var asked atomic.Int32
	s := suggester(store, func(ctx context.Context, a netip.Addr) (string, error) { asked.Add(1); return silent(ctx, a) }, &now)
	c := []Candidate{cand("aa:00:00:00:00:02", "192.168.1.6")}
	var waits []time.Duration
	for range 9 {
		before := asked.Load()
		_ = s.Look(context.Background(), c)
		if asked.Load() != before+1 {
			t.Fatal("a due device must be asked once per look")
		}
		wait := retryAfter(store.s.Entries[0])
		waits = append(waits, wait)
		now = now.Add(wait)
	}
	for i := 1; i < len(waits); i++ {
		if waits[i] < waits[i-1] {
			t.Fatalf("waits must not shrink: %v", waits)
		}
	}
	if waits[len(waits)-1] != maxRetryWait {
		t.Errorf("last wait = %v, want the cap %v: %v", waits[len(waits)-1], maxRetryWait, waits)
	}
}

func TestSuggester_SkipsNothingItShouldNotAskAndAsksAtMostFourAtOnce(t *testing.T) {
	store, now := &memStore{}, t0
	var inFlight, peak atomic.Int32
	var mu sync.Mutex
	ask := func(ctx context.Context, a netip.Addr) (string, error) {
		n := inFlight.Add(1)
		mu.Lock()
		peak.Store(max(peak.Load(), n))
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		inFlight.Add(-1)
		return "x", nil
	}
	s := suggester(store, ask, &now)
	var cs []Candidate
	for i := range 20 {
		cs = append(cs, cand("aa:00:00:00:00:"+string(rune('a'+i)), "192.168.1."+string(rune('0'+i%10))))
	}
	_ = s.Look(context.Background(), cs)
	if peak.Load() > maxInFlight {
		t.Errorf("peak in flight = %d, want at most %d", peak.Load(), maxInFlight)
	}
	if store.writes != 1 {
		t.Errorf("writes = %d: one look is one write", store.writes)
	}
}

func TestSuggester_ForgetsDevicesThatLeftTheListAndIsBounded(t *testing.T) {
	store, now := &memStore{}, t0
	s := suggester(store, answers("x"), &now)
	_ = s.Look(context.Background(), []Candidate{cand("aa:00:00:00:00:01", "192.168.1.5"), cand("aa:00:00:00:00:02", "192.168.1.6")})
	_ = s.Look(context.Background(), []Candidate{cand("aa:00:00:00:00:02", "192.168.1.6")})
	if len(store.s.Entries) != 1 || store.s.Entries[0].MAC != "aa:00:00:00:00:02" {
		t.Errorf("entries = %+v", store.s.Entries)
	}
	var many []Candidate
	for i := range maxEntries + 50 {
		many = append(many, cand(macOf(i), "192.168.1.9"))
	}
	_ = s.Look(context.Background(), many)
	if len(store.s.Entries) > maxEntries {
		t.Errorf("entries = %d, want at most %d", len(store.s.Entries), maxEntries)
	}
}

func macOf(i int) string {
	const hex = "0123456789abcdef"
	return "aa:00:00:00:" + string(hex[(i>>4)&15]) + string(hex[i&15]) + ":" + string(hex[(i>>8)&15]) + "0"
}

func TestSuggester_AStoppedLookEndsAtOnceAndWritesWhatItLearned(t *testing.T) {
	store, now := &memStore{}, t0
	ctx, cancel := context.WithCancel(context.Background())
	var asked atomic.Int32
	s := suggester(store, func(ctx context.Context, a netip.Addr) (string, error) {
		if asked.Add(1) == 1 {
			cancel()
		}
		return "x", nil
	}, &now)
	var cs []Candidate
	for i := range 30 {
		cs = append(cs, cand(macOf(i), "192.168.1.9"))
	}
	_ = s.Look(ctx, cs)
	if asked.Load() > maxInFlight+1 {
		t.Errorf("asked %d after the stop", asked.Load())
	}
}

func TestSuggester_ForReturnsNothingForAnUnknownDevice(t *testing.T) {
	now := t0
	if got := suggester(&memStore{}, answers("x"), &now).For("aa:00:00:00:00:09"); got != "" {
		t.Errorf("suggestion = %q", got)
	}
}

func TestSuggester_SwitchedOffAsksNobodyAndOffersNothing(t *testing.T) {
	store, now := &memStore{}, t0
	on := true
	var asked atomic.Int32
	s := suggester(store, func(context.Context, netip.Addr) (string, error) { asked.Add(1); return "Samsung", nil }, &now).WithEnabled(func() bool { return on })
	c := []Candidate{cand("aa:00:00:00:00:01", "192.168.1.5")}
	_ = s.Look(context.Background(), c)
	if s.For("aa:00:00:00:00:01") != "Samsung" {
		t.Fatal("on: the answer is kept and offered")
	}
	on = false
	now = now.Add(30 * 24 * time.Hour)
	_ = s.Look(context.Background(), c)
	if asked.Load() != 1 {
		t.Errorf("asked %d times: a switched-off suggester must not ask", asked.Load())
	}
	if got := s.All(); len(got) != 0 {
		t.Errorf("offered %v while off", got)
	}
	on = true
	if s.For("aa:00:00:00:00:01") != "Samsung" {
		t.Error("switching back on offers what was learned")
	}
}

const dismissMAC = "aa:00:00:00:00:01"

func TestSuggester_ADismissedNameIsNotOfferedAgainEvenWhenTheDeviceIsAskedAgain(t *testing.T) {
	store, now := &memStore{}, t0
	reply := "Samsung"
	s := suggester(store, func(context.Context, netip.Addr) (string, error) { return reply, nil }, &now)
	c := []Candidate{cand(dismissMAC, "192.168.1.5")}
	_ = s.Look(context.Background(), c)
	if err := s.Dismiss(dismissMAC); err != nil {
		t.Fatal(err)
	}
	if got := s.For(dismissMAC); got != "" {
		t.Fatalf("the rejected name is no longer offered: %q", got)
	}
	// The re-ask after a week gets the same answer; it stays hidden.
	now = now.Add(answerRetry + time.Hour)
	_ = s.Look(context.Background(), c)
	if got := s.For(dismissMAC); got != "" {
		t.Errorf("a rejected name stays rejected after the device answers again: %q", got)
	}
	// A different name later is a new suggestion.
	reply = "Living room TV"
	now = now.Add(answerRetry + time.Hour)
	_ = s.Look(context.Background(), c)
	if got := s.For(dismissMAC); got != "Living room TV" {
		t.Errorf("a different name is offered: %q", got)
	}
}

func TestSuggester_DismissWithNothingToDismissIsAnError(t *testing.T) {
	now := t0
	s := suggester(&memStore{}, silent, &now)
	_ = s.Look(context.Background(), []Candidate{cand(dismissMAC, "192.168.1.5")})
	for _, mac := range []string{dismissMAC, "aa:00:00:00:00:99"} {
		if err := s.Dismiss(mac); !errors.Is(err, ErrNoSuggestion) {
			t.Errorf("%s: err = %v, want ErrNoSuggestion", mac, err)
		}
	}
}
