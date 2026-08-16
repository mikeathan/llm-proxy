package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"llm-proxy/internal/platform/logging"
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

// Load reads the data from disk into memory. If the file content is unchanged
// from the in-memory value (e.g. the watcher reloading our own atomic write),
// listeners are NOT re-fired — this breaks the self-reload loop (C2).
func (s *Store[T]) Load() error {
	s.mu.Lock()

	val, err := readAndUnmarshal[T](s.path)
	if err != nil {
		if os.IsNotExist(err) {
			// If file doesn't exist, we start with a zero value
			var zero T
			changed := s.data == nil || !reflect.DeepEqual(s.data, &zero)
			s.data = &zero
			if !changed {
				s.mu.Unlock()
				return nil
			}
			dataCopy := deepCopy(&zero)
			listeners := append([]func(T){}, s.listeners...)
			s.mu.Unlock()
			s.notify(listeners, dataCopy)
			return nil
		}
		s.mu.Unlock()
		return fmt.Errorf("failed to read store file at %s: %w", s.path, err)
	}

	changed := s.data == nil || !reflect.DeepEqual(s.data, val)
	s.data = val
	if !changed {
		s.mu.Unlock()
		return nil
	}
	dataCopy := deepCopy(val)
	listeners := append([]func(T){}, s.listeners...)
	s.mu.Unlock()
	s.notify(listeners, dataCopy)
	return nil
}

// LoadQuiet reads the data from disk and replaces the in-memory value WITHOUT
// firing OnChange listeners. It is used by the reset lifecycle to swap files and
// reload stores without triggering a mid-reset Sync into the live runtime.
func (s *Store[T]) LoadQuiet() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	val, err := readAndUnmarshal[T](s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.data = nil
			return nil
		}
		return fmt.Errorf("failed to read store file at %s: %w", s.path, err)
	}
	s.data = val
	return nil
}

// readAndUnmarshal reads path and decodes it as YAML (for .yml/.yaml) or JSON.
// A read error is returned unwrapped so callers can os.IsNotExist it; parse
// errors are wrapped with the file path.
func readAndUnmarshal[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var val T
	if isYAMLPath(path) {
		if err := yaml.Unmarshal(data, &val); err != nil {
			return nil, fmt.Errorf("failed to parse yaml store file at %s: %w", path, err)
		}
	} else {
		if err := json.Unmarshal(data, &val); err != nil {
			return nil, fmt.Errorf("failed to parse json store file at %s: %w", path, err)
		}
	}
	return &val, nil
}

// Get returns a deep copy of the data (C1). Maps and pointers are cloned so a
// caller can never race a concurrent Update through a shared reference.
func (s *Store[T]) Get() T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data == nil {
		var zero T
		return zero
	}
	return deepCopy(s.data)
}

// GetProjected applies project to a shallow copy of the live data under the
// read lock, then deep-copies the projection (C1). It costs less than Get when
// the caller only needs a small projection of a large document — the whole
// document is never marshalled, only the projected value is.
func GetProjected[T any, R any](s *Store[T], project func(*T) R) R {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var src T
	if s.data != nil {
		src = *s.data
	}
	proj := project(&src)
	return deepCopy(&proj)
}

// Update modifies the data via a callback and persists it atomically. A failed
// callback or a failed save leaves the in-memory data unchanged (single-owner
// invariant: a failed write must leave the views untouched).
func (s *Store[T]) Update(fn func(*T) error) error {
	s.mu.Lock()

	if s.data == nil {
		var zero T
		s.data = &zero
	}

	before := deepCopy(s.data)
	if err := fn(s.data); err != nil {
		s.data = &before
		s.mu.Unlock()
		return err
	}

	if err := s.saveLocked(); err != nil {
		s.data = &before
		s.mu.Unlock()
		return err
	}

	dataCopy := deepCopy(s.data)
	listeners := append([]func(T){}, s.listeners...)
	s.mu.Unlock()
	s.notify(listeners, dataCopy)
	return nil
}

// notify invokes listeners with the given deep copy so callbacks never observe
// a half-updated document. It must be called with mu released; the listener
// snapshot must have been captured under the lock so a concurrent OnChange
// cannot race the iteration.
func (s *Store[T]) notify(listeners []func(T), dataCopy T) {
	for _, l := range listeners {
		l(dataCopy)
	}
}

// saveLocked writes the data to a temporary file and renames it.
// mu must be locked.
func (s *Store[T]) saveLocked() error {
	yamlPath := isYAMLPath(s.path)

	var data []byte
	var err error

	if yamlPath {
		data, err = yaml.Marshal(s.data)
	} else {
		data, err = json.MarshalIndent(s.data, "", "  ")
	}

	if err != nil {
		return fmt.Errorf("failed to marshal store data: %w", err)
	}

	class := ClassData
	if yamlPath {
		class = ClassConfig
	}
	if err := WriteAtomic(s.path, filepath.Base(s.path)+".*.tmp", data, class); err != nil {
		return fmt.Errorf("atomic write store file: %w", err)
	}

	logging.Debug("storage store atomically updated", "file", filepath.Base(s.path))

	return nil
}

// Path returns the path of the store.
func (s *Store[T]) Path() string {
	return s.path
}

func isYAMLPath(path string) bool {
	return strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")
}

// deepCopy returns a deep copy of v. Store payloads are JSON-serializable, so
// a JSON round-trip yields a fully independent value (maps and pointers cloned).
// If marshalling ever fails, v is returned unchanged rather than panicking.
func deepCopy[T any](v *T) T {
	var out T
	data, err := json.Marshal(v)
	if err != nil {
		return *v
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return *v
	}
	return out
}
