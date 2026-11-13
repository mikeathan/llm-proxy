// Package session holds the bounded sessions (Constitution I.17, ADR-0054). Only the composition root builds a Store; the route handler reads
// the types and holds the interface it needs.
//
// A bounded session is the one way an action may run without a person's step-up for every request (Constitution I.17). A person confirms the
// passphrase once and approves an exact list: which stack may ask, which actions with which checked parameters, for how long, and how many
// times. While the session lives, the core serves a request from that stack that matches an entry of the list exactly, and nothing else. It is
// held in memory only (a restart ends it), has a hard longest life, a hard use count and a hard limit on how many may be open, and anyone
// signed in can end it at once. It carries no token: the authority is "this stack, this exact request, while the session lives", so there is
// nothing to leak and a stack cannot lend it. A session never raises what the helper allows: the helper still checks its own three gates and
// its own scope on every call.
package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

const (
	// MaxTTL is the longest a session may last, whatever was asked.
	MaxTTL = 12 * time.Hour
	// MaxUses is the most requests one session may serve, whatever was asked.
	MaxUses = 5000
	// MaxRequests is the most entries one approved list may hold.
	MaxRequests = 64
	// DefaultMaxOpen is how many sessions may be open at once.
	DefaultMaxOpen = 4
	// endedKept is how many finished sessions are remembered for the person to see what happened.
	endedKept = 10
)

// bind is what a session entry is held to: the action and the canonical parameters, length-prefixed so no split of the two can collide.
func bind(action string, canonical []byte) [sha256.Size]byte {
	h := sha256.New()
	fmt.Fprintf(h, "%d:%s|%d:", len(action), action, len(canonical))
	h.Write(canonical)
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

// Typed failures; match them with errors.Is.
var (
	// ErrNotCovered means no open session covers this stack's request: the request is not on any approved list, the list's time is up, or its
	// uses are spent.
	ErrNotCovered = errors.New("no open session covers this request")
	// ErrBad means the session asked for is malformed or asks for more than the hard limits.
	ErrBad = errors.New("the session is not valid")
	// ErrTooMany means as many sessions are open as may be.
	ErrTooMany = errors.New("too many open sessions")
	// ErrNoSuch means no open session has that id.
	ErrNoSuch = errors.New("no such session")
)

// Request is one entry of an approved list: an action and the canonical form of its checked parameters.
type Request struct {
	Action    string
	Canonical []byte
}

// Spec is what a person approved. The caller has already checked every request against the catalogue.
type Spec struct {
	// Stack is the one stack that may use it (the port the session is served through names its stack).
	Stack string
	// Label is the person's own name for it, shown in the list; it is data, never an instruction.
	Label    string
	TTL      time.Duration
	MaxUses  int
	Requests []Request
}

// View is what a list shows. It holds nothing a request could be forged from.
type View struct {
	ID       string     `json:"id"`
	Stack    string     `json:"stack"`
	Label    string     `json:"label"`
	Started  time.Time  `json:"started"`
	Expires  time.Time  `json:"expires"`
	Uses     int        `json:"uses"`
	MaxUses  int        `json:"max_uses"`
	LastUsed *time.Time `json:"last_used,omitempty"`
	Requests []Listing  `json:"requests"`
	// Ended is set on a finished session: why it ended ("ended by you", "time was up", "uses were spent", "the action was switched off").
	Ended string `json:"ended,omitempty"`
}

// Listing is one approved entry, as shown.
type Listing struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
}

// Options configure Sessions. Zero values take the defaults; Now is injected so time is testable; Log is told when a session begins
// and ends, and is the audit trail.
type Options struct {
	MaxOpen int
	Now     func() time.Time
	Log     func(msg string, kv ...any)
}

type session struct {
	view     View
	bindings map[[sha256.Size]byte]bool
}

// Store holds the open sessions. It is in memory only: a restart ends every one.
type Store struct {
	mu    sync.Mutex
	opts  Options
	open  map[string]*session
	ended []View
}

// New returns an empty store.
func New(o Options) *Store {
	if o.MaxOpen <= 0 {
		o.MaxOpen = DefaultMaxOpen
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Log == nil {
		o.Log = func(string, ...any) {}
	}
	return &Store{opts: o, open: map[string]*session{}}
}

// Begin opens a session for what the person approved. It refuses a list that is empty or over the limits, a time over MaxTTL, a use
// count over MaxUses, and a store that already holds as many as may be open; it never raises a limit to fit the request.
func (s *Store) Begin(spec Spec) (View, error) {
	if err := spec.check(); err != nil {
		return View{}, err
	}
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return View{}, fmt.Errorf("session id: %w", err)
	}
	now := s.opts.Now()
	sess := spec.open(hex.EncodeToString(raw), now)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(now)
	if len(s.open) >= s.opts.MaxOpen {
		return View{}, ErrTooMany
	}
	s.open[sess.view.ID] = sess
	s.opts.Log("a bounded session began", "session", sess.view.ID, "stack", spec.Stack, "requests", len(sess.view.Requests), "until", sess.view.Expires, "max_uses", spec.MaxUses)
	return sess.view, nil
}

// check holds the spec to the hard limits.
func (spec Spec) check() error {
	switch {
	case spec.Stack == "" || len(spec.Label) > 80:
		return fmt.Errorf("%w: a stack and a label of at most 80 characters", ErrBad)
	case spec.TTL <= 0 || spec.TTL > MaxTTL:
		return fmt.Errorf("%w: the time must be more than 0 and at most %s", ErrBad, MaxTTL)
	case spec.MaxUses < 1 || spec.MaxUses > MaxUses:
		return fmt.Errorf("%w: the uses must be between 1 and %d", ErrBad, MaxUses)
	case len(spec.Requests) < 1 || len(spec.Requests) > MaxRequests:
		return fmt.Errorf("%w: the list must hold between 1 and %d requests", ErrBad, MaxRequests)
	}
	for _, r := range spec.Requests {
		if r.Action == "" || len(r.Canonical) == 0 {
			return fmt.Errorf("%w: a request has no action or no parameters", ErrBad)
		}
	}
	return nil
}

// open builds the session for a checked spec, listing each distinct entry once.
func (spec Spec) open(id string, now time.Time) *session {
	sess := &session{bindings: map[[sha256.Size]byte]bool{}}
	for _, r := range spec.Requests {
		key := bind(r.Action, r.Canonical)
		if !sess.bindings[key] {
			sess.view.Requests = append(sess.view.Requests, Listing{Action: r.Action, Params: slices.Clone(r.Canonical)})
		}
		sess.bindings[key] = true
	}
	sess.view.ID, sess.view.Stack, sess.view.Label = id, spec.Stack, spec.Label
	sess.view.Started, sess.view.Expires, sess.view.MaxUses = now, now.Add(spec.TTL), spec.MaxUses
	return sess
}

// Use serves one request from the stack: it needs an open session of that stack whose list holds this action and these canonical parameters,
// whose time is not up and whose uses are not spent, and it counts one use. A request no session covers is ErrNotCovered, whatever the reason.
func (s *Store) Use(stack, action string, canonical []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.opts.Now()
	s.sweep(now)
	want := bind(action, canonical)
	for _, id := range s.ids() {
		sess := s.open[id]
		if sess.view.Stack != stack || !sess.bindings[want] {
			continue
		}
		sess.view.Uses++
		sess.view.LastUsed = &now
		if sess.view.Uses >= sess.view.MaxUses {
			s.finish(sess, "its uses were spent")
		}
		return nil
	}
	return ErrNotCovered
}

// End closes one session at once. It reports ErrNoSuch when none is open under that id.
func (s *Store) End(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(s.opts.Now())
	sess, ok := s.open[id]
	if !ok {
		return ErrNoSuch
	}
	s.finish(sess, "ended by a person")
	return nil
}

// EndAll closes every open session and returns how many there were.
func (s *Store) EndAll(reason string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.open)
	for _, id := range s.ids() {
		s.finish(s.open[id], reason)
	}
	return n
}

// EndAction closes every session whose list holds the action, because the person switched the action off: a session never outlives the switch.
func (s *Store) EndAction(action string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, id := range s.ids() {
		sess := s.open[id]
		if slices.ContainsFunc(sess.view.Requests, func(r Listing) bool { return r.Action == action }) {
			s.finish(sess, "the action was switched off")
			n++
		}
	}
	return n
}

// Open lists the open sessions, oldest first, then the last few finished ones with the reason each ended.
func (s *Store) Open() (open, ended []View) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(s.opts.Now())
	for _, id := range s.ids() {
		open = append(open, s.open[id].view)
	}
	return open, slices.Clone(s.ended)
}

// ids lists the open ids oldest first, so a request is served by the oldest session that covers it and a list reads in order.
func (s *Store) ids() []string {
	ids := make([]string, 0, len(s.open))
	for id := range s.open {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b string) int { return s.open[a].view.Started.Compare(s.open[b].view.Started) })
	return ids
}

func (s *Store) sweep(now time.Time) {
	for _, id := range s.ids() {
		if sess := s.open[id]; !now.Before(sess.view.Expires) {
			s.finish(sess, "its time was up")
		}
	}
}

func (s *Store) finish(sess *session, reason string) {
	delete(s.open, sess.view.ID)
	v := sess.view
	v.Ended = reason
	s.ended = append(s.ended, v)
	if len(s.ended) > endedKept {
		s.ended = s.ended[len(s.ended)-endedKept:]
	}
	s.opts.Log("a bounded session ended", "session", v.ID, "stack", v.Stack, "reason", reason, "uses", v.Uses)
}
