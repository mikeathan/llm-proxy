// Package grant is the one-use permission to call the privileged helper (design network-privileged-helper-design.md section 6). A grant
// answers "was this a person's step-up click, inside the service": it is minted only for a request that already passed the core's step-up
// check, is bound to one action and the canonical form of its checked parameters, works once and expires quickly. The helper never sees
// it; it answers "was this the service" with its own caller check. Only the step-up route handler and the composition root import this
// package (an architecture test pins that).
package grant

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	tokenBytes = 16 // 128 bits

	// DefaultTTL is how long a grant lives: long enough for the call that follows the click, short enough that a leaked one is useless.
	DefaultTTL = 30 * time.Second
	// DefaultMaxOutstanding bounds the grants waiting to be used; a step-up click is human-paced, so a full store means abuse or a bug.
	DefaultMaxOutstanding = 64
)

// Typed failures; match them with errors.Is. A caller maps all of them to one refusal.
var (
	// ErrUnknown means the grant was never minted here, or was already used.
	ErrUnknown = errors.New("no such grant")
	// ErrExpired means the grant lived past its time.
	ErrExpired = errors.New("grant expired")
	// ErrMismatch means the grant was minted for another action or other parameters.
	ErrMismatch = errors.New("grant is for another request")
	// ErrTooMany means the store holds as many unused grants as it may.
	ErrTooMany = errors.New("too many unused grants")
)

// Options configure a Store. Zero values take the defaults; Now is injected so expiry is testable.
type Options struct {
	TTL            time.Duration
	MaxOutstanding int
	Now            func() time.Time
}

type entry struct {
	binding [sha256.Size]byte
	expires time.Time
}

// Store holds the grants that have been minted and not yet used. It is in memory only: a restart voids every grant.
type Store struct {
	mu      sync.Mutex
	opts    Options
	entries map[string]entry
}

// NewStore returns an empty Store.
func NewStore(o Options) *Store {
	if o.TTL <= 0 {
		o.TTL = DefaultTTL
	}
	if o.MaxOutstanding <= 0 {
		o.MaxOutstanding = DefaultMaxOutstanding
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Store{opts: o, entries: make(map[string]entry)}
}

// bind is what a grant is bound to: the action and the canonical parameters, length-prefixed so no split of the two can collide.
func bind(action string, canonical []byte) [sha256.Size]byte {
	h := sha256.New()
	fmt.Fprintf(h, "%d:%s|%d:", len(action), action, len(canonical))
	h.Write(canonical)
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

// Mint returns a new grant for the action and the canonical form of its checked parameters. Expired grants are dropped first, so the
// store cannot grow past MaxOutstanding unused entries.
func (s *Store) Mint(action string, canonical []byte) (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("mint grant: %w", err)
	}
	token := hex.EncodeToString(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.opts.Now()
	s.sweep(now)
	if len(s.entries) >= s.opts.MaxOutstanding {
		return "", ErrTooMany
	}
	s.entries[token] = entry{binding: bind(action, canonical), expires: now.Add(s.opts.TTL)}
	return token, nil
}

// Consume checks the grant against the action and canonical parameters and uses it up. The grant is gone after any attempt, right or
// wrong, so a guess cannot be retried against it.
func (s *Store) Consume(token, action string, canonical []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[token]
	delete(s.entries, token)
	switch want := bind(action, canonical); {
	case !ok:
		return ErrUnknown
	case !s.opts.Now().Before(e.expires):
		return ErrExpired
	case subtle.ConstantTimeCompare(e.binding[:], want[:]) != 1:
		return ErrMismatch
	}
	return nil
}

// Outstanding is the number of unused, unexpired grants.
func (s *Store) Outstanding() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(s.opts.Now())
	return len(s.entries)
}

func (s *Store) sweep(now time.Time) {
	for token, e := range s.entries {
		if !now.Before(e.expires) {
			delete(s.entries, token)
		}
	}
}
