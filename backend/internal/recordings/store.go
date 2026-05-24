package recordings

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type RecordingStore struct {
	mu        sync.RWMutex
	recordDir string
	entries   map[string]RecordingMeta
}

func NewRecordingStore(recordDir string) (*RecordingStore, error) {
	s := &RecordingStore{
		recordDir: recordDir,
		entries:   make(map[string]RecordingMeta),
	}
	if err := s.Refresh(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *RecordingStore) RecordDir() string {
	return s.recordDir
}

func (s *RecordingStore) List() []RecordingMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RecordingMeta, 0, len(s.entries))
	for _, m := range s.entries {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.After(out[j].Timestamp)
	})
	return out
}

func (s *RecordingStore) ListByAutomation(name string) []RecordingMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []RecordingMeta
	for _, m := range s.entries {
		if m.AutomationName == name {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.After(out[j].Timestamp)
	})
	return out
}

func (s *RecordingStore) Get(id string) (*RecordingMeta, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.entries[id]
	if !ok {
		return nil, false
	}
	return &m, true
}

func (s *RecordingStore) Delete(id string) error {
	s.mu.Lock()
	m, ok := s.entries[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("recording %q not found", id)
	}
	delete(s.entries, id)
	s.mu.Unlock()

	if err := os.Remove(m.FilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete recording file %s: %w", m.FilePath, err)
	}
	return nil
}

func (s *RecordingStore) Refresh() error {
	entries := make(map[string]RecordingMeta)

	err := filepath.WalkDir(s.recordDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}

		rel, err := filepath.Rel(s.recordDir, path)
		if err != nil {
			return nil
		}

		parts := strings.SplitN(rel, string(filepath.Separator), 3)
		model := ""
		automation := ""
		if len(parts) >= 1 {
			model = parts[0]
		}
		if len(parts) >= 2 {
			automation = parts[1]
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		ts, sessionID := parseRecordingFilename(d.Name())

		id := rel
		entries[id] = RecordingMeta{
			ID:             id,
			Model:          model,
			AutomationName: automation,
			Timestamp:      ts,
			FilePath:       path,
			FileSize:       info.Size(),
			SessionID:      sessionID,
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("scan recordings: %w", err)
	}

	s.mu.Lock()
	s.entries = entries
	s.mu.Unlock()
	return nil
}

func parseRecordingFilename(name string) (time.Time, string) {
	base := strings.TrimSuffix(name, ".jsonl")
	parts := strings.SplitN(base, "_", 2)
	if len(parts) < 2 {
		return time.Time{}, ""
	}
	ts, err := time.Parse("20060102T150405Z", parts[0])
	if err != nil {
		return time.Time{}, ""
	}
	return ts, parts[1]
}
