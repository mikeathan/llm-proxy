package grant

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newTestStore(c *clock, o Options) *Store {
	o.Now = c.Now
	return NewStore(o)
}

func TestAGrantWorksOnceForItsActionAndParameters(t *testing.T) {
	s := NewStore(Options{})
	token, err := s.Mint("scan_udp", []byte(`{"target":"192.168.1.5"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 32 {
		t.Errorf("a grant is 128 bits, got %q", token)
	}
	if err := s.Consume(token, "scan_udp", []byte(`{"target":"192.168.1.5"}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Consume(token, "scan_udp", []byte(`{"target":"192.168.1.5"}`)); !errors.Is(err, ErrUnknown) {
		t.Errorf("a second use must fail, got %v", err)
	}
}

func TestAGrantIsBoundToTheActionAndTheParameters(t *testing.T) {
	cases := map[string]struct{ action, params string }{
		"another action":     {"os_detect", `{"target":"192.168.1.5"}`},
		"another target":     {"scan_udp", `{"target":"192.168.1.6"}`},
		"extra parameters":   {"scan_udp", `{"target":"192.168.1.5","ports":[53]}`},
		"the split moved":    {"scan_udp{", `"target":"192.168.1.5"}`},
		"empty parameters":   {"scan_udp", ``},
		"an empty action":    {"", `{"target":"192.168.1.5"}`},
		"the action as data": {"", `scan_udp{"target":"192.168.1.5"}`},
	}
	for name, c := range cases {
		s := NewStore(Options{})
		token, _ := s.Mint("scan_udp", []byte(`{"target":"192.168.1.5"}`))
		if err := s.Consume(token, c.action, []byte(c.params)); !errors.Is(err, ErrMismatch) {
			t.Errorf("%s: got %v", name, err)
		}
		if err := s.Consume(token, "scan_udp", []byte(`{"target":"192.168.1.5"}`)); !errors.Is(err, ErrUnknown) {
			t.Errorf("%s: a wrong attempt must use the grant up, got %v", name, err)
		}
	}
}

func TestAGrantExpires(t *testing.T) {
	c := &clock{now: time.Unix(1000, 0)}
	s := newTestStore(c, Options{TTL: 10 * time.Second})
	token, _ := s.Mint("a", nil)
	c.now = c.now.Add(10 * time.Second)
	if err := s.Consume(token, "a", nil); !errors.Is(err, ErrExpired) {
		t.Errorf("at the expiry instant the grant is dead, got %v", err)
	}
	token, _ = s.Mint("a", nil)
	c.now = c.now.Add(9 * time.Second)
	if err := s.Consume(token, "a", nil); err != nil {
		t.Errorf("just before the expiry the grant works, got %v", err)
	}
}

func TestAGuessedGrantFails(t *testing.T) {
	s := NewStore(Options{})
	for _, guess := range []string{"", "0", "00000000000000000000000000000000"} {
		if err := s.Consume(guess, "a", nil); !errors.Is(err, ErrUnknown) {
			t.Errorf("%q: got %v", guess, err)
		}
	}
}

func TestTheStoreStaysBoundedAndForgetsTheExpired(t *testing.T) {
	c := &clock{now: time.Unix(1000, 0)}
	s := newTestStore(c, Options{TTL: time.Minute, MaxOutstanding: 3})
	for range 3 {
		if _, err := s.Mint("a", nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Mint("a", nil); !errors.Is(err, ErrTooMany) {
		t.Errorf("a full store must refuse, got %v", err)
	}
	c.now = c.now.Add(time.Minute)
	if n := s.Outstanding(); n != 0 {
		t.Errorf("expired grants must be swept, %d left", n)
	}
	if _, err := s.Mint("a", nil); err != nil {
		t.Errorf("room must come back once they expire, got %v", err)
	}
}

func TestTwoGrantsForTheSameRequestAreDistinct(t *testing.T) {
	s := NewStore(Options{})
	a, _ := s.Mint("a", []byte("p"))
	b, _ := s.Mint("a", []byte("p"))
	if a == b {
		t.Fatal("grants must be random")
	}
	if err := s.Consume(a, "a", []byte("p")); err != nil {
		t.Error(err)
	}
	if err := s.Consume(b, "a", []byte("p")); err != nil {
		t.Errorf("using one grant must not use the other up: %v", err)
	}
}

func TestOnlyOneOfManyConcurrentUsesSucceeds(t *testing.T) {
	s := NewStore(Options{})
	token, _ := s.Mint("a", []byte("p"))
	var wg sync.WaitGroup
	var mu sync.Mutex
	won := 0
	for range 32 {
		wg.Go(func() {
			if s.Consume(token, "a", []byte("p")) == nil {
				mu.Lock()
				won++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if won != 1 {
		t.Errorf("%d uses succeeded, want exactly 1", won)
	}
}
