package session

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newStore(t *testing.T) (*Store, *fakeClock, *[]string) {
	t.Helper()
	c := &fakeClock{time.Date(2026, 11, 13, 20, 0, 0, 0, time.UTC)}
	var logs []string
	return New(Options{Now: c.now, Log: func(msg string, _ ...any) { logs = append(logs, msg) }}), c, &logs
}

func spec() Spec {
	return Spec{Stack: "radio", Label: "overnight", TTL: 8 * time.Hour, MaxUses: 3, Requests: []Request{
		{Action: "radio_listen", Canonical: []byte(`{"iface":"w","channel":1,"seconds":20}`)},
		{Action: "radio_listen", Canonical: []byte(`{"iface":"w","channel":6,"seconds":20}`)},
	}}
}

func TestASessionServesExactlyWhatWasApproved(t *testing.T) {
	s, _, _ := newStore(t)
	if _, err := s.Begin(spec()); err != nil {
		t.Fatal(err)
	}
	if err := s.Use("radio", "radio_listen", []byte(`{"iface":"w","channel":6,"seconds":20}`)); err != nil {
		t.Errorf("an approved request: %v", err)
	}
	for name, c := range map[string]struct {
		stack, action string
		canon         string
	}{
		"another channel":  {"radio", "radio_listen", `{"iface":"w","channel":11,"seconds":20}`},
		"a longer window":  {"radio", "radio_listen", `{"iface":"w","channel":1,"seconds":30}`},
		"another adapter":  {"radio", "radio_listen", `{"iface":"x","channel":1,"seconds":20}`},
		"another action":   {"radio", "radio_probe", `{"iface":"w","channel":1,"seconds":20}`},
		"another stack":    {"network", "radio_listen", `{"iface":"w","channel":1,"seconds":20}`},
		"a different body": {"radio", "radio_listen", `{}`},
	} {
		if err := s.Use(c.stack, c.action, []byte(c.canon)); !errors.Is(err, ErrNotCovered) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestASessionEndsWhenItsTimeIsUp(t *testing.T) {
	s, c, _ := newStore(t)
	if _, err := s.Begin(spec()); err != nil {
		t.Fatal(err)
	}
	c.t = c.t.Add(8*time.Hour - time.Second)
	if err := s.Use("radio", "radio_listen", []byte(`{"iface":"w","channel":1,"seconds":20}`)); err != nil {
		t.Fatalf("a second before the end: %v", err)
	}
	c.t = c.t.Add(time.Second)
	if err := s.Use("radio", "radio_listen", []byte(`{"iface":"w","channel":1,"seconds":20}`)); !errors.Is(err, ErrNotCovered) {
		t.Errorf("at the end: %v", err)
	}
	open, ended := s.Open()
	if len(open) != 0 || len(ended) != 1 || ended[0].Ended != "its time was up" || ended[0].Uses != 1 {
		t.Errorf("open %v ended %+v", open, ended)
	}
}

func TestASessionEndsWhenItsUsesAreSpent(t *testing.T) {
	s, _, _ := newStore(t)
	if _, err := s.Begin(spec()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := s.Use("radio", "radio_listen", []byte(`{"iface":"w","channel":1,"seconds":20}`)); err != nil {
			t.Fatalf("use %d: %v", i, err)
		}
	}
	if err := s.Use("radio", "radio_listen", []byte(`{"iface":"w","channel":1,"seconds":20}`)); !errors.Is(err, ErrNotCovered) {
		t.Errorf("a fourth use: %v", err)
	}
	if _, ended := s.Open(); len(ended) != 1 || ended[0].Ended != "its uses were spent" {
		t.Errorf("%+v", ended)
	}
}

func TestAPersonEndsASessionAtOnce(t *testing.T) {
	s, _, logs := newStore(t)
	v, err := s.Begin(spec())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.End(v.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Use("radio", "radio_listen", []byte(`{"iface":"w","channel":1,"seconds":20}`)); !errors.Is(err, ErrNotCovered) {
		t.Errorf("after the end: %v", err)
	}
	if err := s.End(v.ID); !errors.Is(err, ErrNoSuch) {
		t.Errorf("ending twice: %v", err)
	}
	if got := *logs; len(got) != 2 || got[0] != "a bounded session began" || got[1] != "a bounded session ended" {
		t.Errorf("the audit trail: %v", got)
	}
}

func TestSwitchingAnActionOffEndsEverySessionThatHoldsIt(t *testing.T) {
	s, _, _ := newStore(t)
	if _, err := s.Begin(spec()); err != nil {
		t.Fatal(err)
	}
	other := spec()
	other.Requests = []Request{{Action: "radio_probe", Canonical: []byte(`{}`)}}
	if _, err := s.Begin(other); err != nil {
		t.Fatal(err)
	}
	if n := s.EndAction("radio_listen"); n != 1 {
		t.Errorf("ended %d", n)
	}
	if open, _ := s.Open(); len(open) != 1 || open[0].Requests[0].Action != "radio_probe" {
		t.Errorf("%+v", open)
	}
	if n := s.EndAll("the service is stopping"); n != 1 {
		t.Errorf("ended %d", n)
	}
}

func TestBeginRefusesWhatExceedsTheHardLimits(t *testing.T) {
	s, _, _ := newStore(t)
	cases := map[string]func(*Spec){
		"no stack":        func(p *Spec) { p.Stack = "" },
		"too long":        func(p *Spec) { p.TTL = MaxTTL + time.Second },
		"no time":         func(p *Spec) { p.TTL = 0 },
		"too many uses":   func(p *Spec) { p.MaxUses = MaxUses + 1 },
		"no uses":         func(p *Spec) { p.MaxUses = 0 },
		"empty list":      func(p *Spec) { p.Requests = nil },
		"a request blank": func(p *Spec) { p.Requests = []Request{{Action: "radio_listen"}} },
		"long label":      func(p *Spec) { p.Label = string(make([]byte, 81)) },
		"list too long": func(p *Spec) {
			p.Requests = nil
			for i := 0; i <= MaxRequests; i++ {
				p.Requests = append(p.Requests, Request{Action: "a", Canonical: []byte{byte(i), 1}})
			}
		},
	}
	for name, mutate := range cases {
		p := spec()
		mutate(&p)
		if _, err := s.Begin(p); !errors.Is(err, ErrBad) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if open, _ := s.Open(); len(open) != 0 {
		t.Error("a refused session must not be open")
	}
}

func TestOnlyAFewSessionsMayBeOpenAndAnEndedOneFreesASlot(t *testing.T) {
	s, _, _ := newStore(t)
	var first View
	for i := 0; i < DefaultMaxOpen; i++ {
		v, err := s.Begin(spec())
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = v
		}
	}
	if _, err := s.Begin(spec()); !errors.Is(err, ErrTooMany) {
		t.Fatalf("%v", err)
	}
	if err := s.End(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Begin(spec()); err != nil {
		t.Errorf("a slot is free: %v", err)
	}
}

func TestTheViewHoldsNothingARequestCouldBeForgedFromAndRememberedEndsAreBounded(t *testing.T) {
	s, _, _ := newStore(t)
	for i := 0; i < endedKept+5; i++ {
		v, err := s.Begin(spec())
		if err != nil {
			t.Fatal(err)
		}
		_ = s.End(v.ID)
	}
	if _, ended := s.Open(); len(ended) != endedKept {
		t.Errorf("%d finished sessions kept, want %d", len(ended), endedKept)
	}
}

func TestSessionsAreSafeToUseFromManyGoroutines(t *testing.T) {
	s, _, _ := newStore(t)
	p := spec()
	p.MaxUses = 100
	if _, err := s.Begin(p); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	served := 0
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.Use("radio", "radio_listen", []byte(`{"iface":"w","channel":1,"seconds":20}`)) == nil {
				mu.Lock()
				served++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if served != 100 {
		t.Errorf("served %d, the cap is 100", served)
	}
}
