package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Store is a generic atomic JSON file store.
type Store[T any] struct {
	mu        sync.RWMutex
	path      string
	data      *T
	listeners []func(T)
}

// NewStore creates a new atomic JSON store for type T.
func NewStore[T any](path string) *Store[T] {
	return &Store[T]{
		path: path,
	}
}

// OnChange registers a callback to be called when the store data changes.
func (s *Store[T]) OnChange(fn func(T)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}

// Load reads the data from disk into memory.
func (s *Store[T]) Load() error {
	s.mu.Lock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			// If file doesn't exist, we start with a zero value
			var zero T
			s.data = &zero
			s.mu.Unlock()
			return nil
		}
		s.mu.Unlock()
		return fmt.Errorf("failed to read store file at %s: %w", s.path, err)
	}

	var val T
	if strings.HasSuffix(s.path, ".yml") || strings.HasSuffix(s.path, ".yaml") {
		if err := yaml.Unmarshal(data, &val); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("failed to parse yaml store file at %s: %w", s.path, err)
		}
	} else {
		if err := json.Unmarshal(data, &val); err != nil {
			s.mu.Unlock()
			return fmt.Errorf("failed to parse json store file at %s: %w", s.path, err)
		}
	}

	s.data = &val

	// Notify listeners after unlock
	dataCopy := *s.data
	s.mu.Unlock()
	for _, l := range s.listeners {
		l(dataCopy)
	}

	return nil
}

// Get returns a copy of the data.
func (s *Store[T]) Get() T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		var zero T
		return zero
	}
	// Return a shallow copy. For deep copy, caller must handle it or use a deepcopy lib.
	return *s.data
}

// Update modifies the data via a callback and persists it atomically.
func (s *Store[T]) Update(fn func(*T) error) error {
	s.mu.Lock()

	if s.data == nil {
		var zero T
		s.data = &zero
	}

	if err := fn(s.data); err != nil {
		s.mu.Unlock()
		return err
	}

	err := s.saveLocked()
	if err != nil {
		s.mu.Unlock()
		return err
	}

	// Notify listeners after unlock to avoid deadlocks
	dataCopy := *s.data
	s.mu.Unlock()
	for _, l := range s.listeners {
		l(dataCopy)
	}

	return nil
}

// saveLocked writes the data to a temporary file and renames it.
// mu must be locked.
func (s *Store[T]) saveLocked() error {
	var data []byte
	var err error

	if strings.HasSuffix(s.path, ".yml") || strings.HasSuffix(s.path, ".yaml") {
		data, err = yaml.Marshal(s.data)
	} else {
		data, err = json.MarshalIndent(s.data, "", "  ")
	}

	if err != nil {
		return fmt.Errorf("failed to marshal store data: %w", err)
	}

	if err := WriteAtomic(s.path, filepath.Base(s.path)+".*.tmp", data); err != nil {
		return fmt.Errorf("atomic write store file: %w", err)
	}

	fmt.Printf(" [STORAGE] Atomically updated %s\n", filepath.Base(s.path))

	return nil
}

// Path returns the path of the store.
func (s *Store[T]) Path() string {
	return s.path
}
