package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"llm-proxy/internal/platform/storage"
)

// Record is everything the sign-in keeps, in one file. Every secret in it is a hash: the passphrase (argon2id), the
// setup code, the script tokens and the session ids (SHA-256 of 256-bit random values), so reading the file, as another
// OS user or an unjailed agent could, gives nothing to replay.
type Record struct {
	Passphrase string     `json:"passphrase,omitempty"`
	SetupCode  *SetupCode `json:"setup_code,omitempty"`
	Tokens     []Token    `json:"tokens,omitempty"`
	Sessions   []Session  `json:"sessions,omitempty"`
	Events     []Event    `json:"events,omitempty"`
}

// Store keeps the Record in a file. It notices a change made by another process (the command line) and reads again.
type Store struct {
	path string

	mu    sync.Mutex
	rec   Record
	mtime time.Time
	size  int64
	known bool
}

func OpenStore(path string) *Store { return &Store{path: path} }

// View passes a copy of the record to fn, after reading the file again if it changed on disk.
func (s *Store) View(fn func(Record)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refresh(); err != nil {
		return err
	}
	fn(clone(s.rec))
	return nil
}

// Update changes the record under the lock and writes it atomically (0600). When fn fails nothing is changed or written.
func (s *Store) Update(fn func(*Record) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refresh(); err != nil {
		return err
	}
	next := clone(s.rec)
	if err := fn(&next); err != nil {
		return err
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode the sign-in record: %w", err)
	}
	if err := storage.WriteAtomic(s.path, "auth-*.json.tmp", data, storage.ClassSecret); err != nil {
		return fmt.Errorf("failed to save the sign-in record: %w", err)
	}
	s.rec = next
	s.remember()
	return nil
}

// refresh reads the file when it is new or changed. A missing file is an empty record; an unreadable one is an error, so
// a damaged file can never read as "sign-in is off". The damaged file is left where it is for the operator.
func (s *Store) refresh() error {
	fi, err := os.Stat(s.path)
	if os.IsNotExist(err) {
		s.rec, s.known = Record{}, false
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read the sign-in record: %w", err)
	}
	if s.known && fi.ModTime().Equal(s.mtime) && fi.Size() == s.size {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("failed to read the sign-in record: %w", err)
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return fmt.Errorf("the sign-in record %s is damaged and was left as it is: %w", s.path, err)
	}
	s.rec = rec
	s.mtime, s.size, s.known = fi.ModTime(), fi.Size(), true
	return nil
}

func (s *Store) remember() {
	if fi, err := os.Stat(s.path); err == nil {
		s.mtime, s.size, s.known = fi.ModTime(), fi.Size(), true
	}
}

func clone(r Record) Record {
	r.Tokens = append([]Token(nil), r.Tokens...)
	r.Sessions = append([]Session(nil), r.Sessions...)
	r.Events = append([]Event(nil), r.Events...)
	if r.SetupCode != nil {
		c := *r.SetupCode
		r.SetupCode = &c
	}
	return r
}
