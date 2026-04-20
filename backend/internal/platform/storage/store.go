package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			// If file doesn't exist, we start with a zero value
			var zero T
			s.data = &zero
			return nil
		}
		return fmt.Errorf("failed to read store file at %s: %w", s.path, err)
	}

	var val T
	if err := json.Unmarshal(data, &val); err != nil {
		return fmt.Errorf("failed to parse store file at %s: %w", s.path, err)
	}

	s.data = &val
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
func (s *Store[T]) Update(fn func(*T)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data == nil {
		var zero T
		s.data = &zero
	}

	fn(s.data)

	return s.saveLocked()
}

// saveLocked writes the data to a temporary file and renames it.
// mu must be locked.
func (s *Store[T]) saveLocked() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal store data: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create store directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, filepath.Base(s.path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write store data: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("failed to rename store file: %w", err)
	}

	// Notify listeners
	dataCopy := *s.data
	fmt.Printf(" [STORAGE] Atomically updated %s\n", filepath.Base(s.path))
	for _, l := range s.listeners {
		l(dataCopy)
	}

	return nil
}

// Path returns the path of the store.
func (s *Store[T]) Path() string {
	return s.path
}
