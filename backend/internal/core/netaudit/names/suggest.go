package names

import (
	"context"
	"net/netip"
	"slices"
	"sync"
	"time"

	"llm-proxy/models"
)

// The cost of naming is bounded on every side: a few questions at a time, a few per look, each device asked rarely, and a
// silent one asked less and less often.
const (
	maxInFlight  = 4
	maxPerLook   = 32
	maxEntries   = 512
	answerRetry  = 7 * 24 * time.Hour
	firstMissGap = 6 * time.Hour
	maxRetryWait = 7 * 24 * time.Hour
)

// Asker asks one device what it calls itself. It returns an error for a device that did not give a usable answer.
type Asker func(ctx context.Context, addr netip.Addr) (string, error)

// Store keeps the suggestions. A missing file is an empty list.
type Store interface {
	Read() (models.NameSuggestions, error)
	Write(models.NameSuggestions) error
}

// Candidate is a device worth asking: one the operator has not named, with an IPv4 address.
type Candidate struct {
	MAC  string
	Addr netip.Addr
}

// Suggester asks unnamed devices for their names, a few at a time and rarely, and keeps the answers for the Devices card
// to offer. It never names a device by itself: the operator confirms.
type Suggester struct {
	mu    sync.Mutex
	store Store
	ask   Asker
	on    func() bool
	now   func() time.Time
}

// NewSuggester builds the service over a store and an asker.
func NewSuggester(store Store, ask Asker) *Suggester {
	return &Suggester{store: store, ask: ask, now: time.Now}
}

// WithEnabled makes the suggester ask, and offer what it learned, only while on returns true. Without it, it is always on.
func (s *Suggester) WithEnabled(on func() bool) *Suggester {
	s.on = on
	return s
}

func (s *Suggester) enabled() bool { return s.on == nil || s.on() }

// retryAfter is how long to wait after a question before asking the same device again.
func retryAfter(e models.NameSuggestion) time.Duration {
	if e.Misses == 0 {
		return answerRetry
	}
	wait := firstMissGap
	for i := 1; i < e.Misses && wait < maxRetryWait; i++ {
		wait *= 2
	}
	return min(wait, maxRetryWait)
}

// Look asks the devices that are due and writes what it learned, once. Devices that are no longer candidates (named, gone,
// or without an address) are forgotten. A stopped context ends it at once; what was learned until then is still kept.
func (s *Suggester) Look(ctx context.Context, candidates []Candidate) error {
	if !s.enabled() {
		return nil
	}
	s.mu.Lock()
	stored, err := s.store.Read()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	now := s.now()
	entries := map[string]models.NameSuggestion{}
	for _, e := range stored.Entries {
		entries[e.MAC] = e
	}
	changed := forget(entries, candidates)
	due := dueDevices(entries, candidates, now)
	for r := range s.askAll(ctx, due) {
		e := entries[r.MAC]
		e.MAC, e.TriedAt = r.MAC, now
		if r.name != "" {
			e.Name, e.AnsweredAt, e.Misses = r.name, now, 0
		} else {
			e.Misses++
		}
		entries[r.MAC] = e
		changed = true
	}
	if !changed {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.Write(models.NameSuggestions{Entries: bounded(entries)})
}

// forget drops every entry whose device is not a candidate now, and says whether it dropped any.
func forget(entries map[string]models.NameSuggestion, candidates []Candidate) bool {
	keep := map[string]bool{}
	for _, c := range candidates {
		keep[c.MAC] = true
	}
	dropped := false
	for mac := range entries {
		if !keep[mac] {
			delete(entries, mac)
			dropped = true
		}
	}
	return dropped
}

// dueDevices are the candidates whose retry time has come, the longest-waiting first, at most maxPerLook.
func dueDevices(entries map[string]models.NameSuggestion, candidates []Candidate, now time.Time) []Candidate {
	var due []Candidate
	for _, c := range candidates {
		if e, known := entries[c.MAC]; !known || now.Sub(e.TriedAt) >= retryAfter(e) {
			due = append(due, c)
		}
	}
	slices.SortStableFunc(due, func(a, b Candidate) int { return entries[a.MAC].TriedAt.Compare(entries[b.MAC].TriedAt) })
	if len(due) > maxPerLook {
		due = due[:maxPerLook]
	}
	return due
}

type result struct {
	MAC  string
	name string
}

// askAll puts the questions, at most maxInFlight at once, and stops handing out new ones when ctx ends.
func (s *Suggester) askAll(ctx context.Context, due []Candidate) <-chan result {
	out := make(chan result, len(due))
	go func() {
		defer close(out)
		var wg sync.WaitGroup
		slots := make(chan struct{}, maxInFlight)
		for _, c := range due {
			if ctx.Err() != nil {
				break
			}
			slots <- struct{}{}
			wg.Add(1)
			go func() {
				defer func() { <-slots; wg.Done() }()
				name, err := s.ask(ctx, c.Addr)
				if err != nil {
					name = ""
				}
				out <- result{MAC: c.MAC, name: name}
			}()
		}
		wg.Wait()
	}()
	return out
}

// bounded returns the entries, the most recently asked first, at most maxEntries.
func bounded(entries map[string]models.NameSuggestion) []models.NameSuggestion {
	out := make([]models.NameSuggestion, 0, len(entries))
	for _, e := range entries {
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b models.NameSuggestion) int {
		if c := b.TriedAt.Compare(a.TriedAt); c != 0 {
			return c
		}
		return compareMAC(a.MAC, b.MAC)
	})
	if len(out) > maxEntries {
		out = out[:maxEntries]
	}
	return out
}

func compareMAC(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// For is the name a device last gave, or empty. It reads the store, so it is for one device; All is for a list.
func (s *Suggester) For(mac string) string { return s.All()[mac] }

// All is every name a device has given, by hardware address.
func (s *Suggester) All() map[string]string {
	if !s.enabled() {
		return map[string]string{}
	}
	s.mu.Lock()
	stored, err := s.store.Read()
	s.mu.Unlock()
	out := map[string]string{}
	if err != nil {
		return out
	}
	for _, e := range stored.Entries {
		if e.Name != "" {
			out[e.MAC] = e.Name
		}
	}
	return out
}
