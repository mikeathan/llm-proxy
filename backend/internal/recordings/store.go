package recordings

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"llm-proxy/internal/platform/storage"
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

	// Nested recordings (data/runs/{ws}/{model}/{task}/{ts}_{sid}/recording.jsonl)
	// live inside a run directory that also holds events.jsonl, run-meta.json
	// and final-report.md. Removing only the .jsonl would orphan those siblings,
	// so delete the whole run directory when applicable.
	if filepath.Base(m.FilePath) == runRecordingName {
		runDir := filepath.Dir(m.FilePath)
		if err := storage.RemoveScopedDir(s.recordDir, runDir); err != nil {
			return err
		}
	}
	return nil
}

// eventStreamName is the run's event stream, which sits beside its recording.jsonl in the run folder and is not a recording.
const eventStreamName = "events.jsonl"

func (s *RecordingStore) Refresh() error {
	entries := make(map[string]RecordingMeta)

	err := filepath.WalkDir(s.recordDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") || d.Name() == eventStreamName {
			return nil
		}

		rel, err := filepath.Rel(s.recordDir, path)
		if err != nil {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		meta := recordingLabels(rel)
		meta.ID, meta.FilePath, meta.FileSize = rel, path, info.Size()
		entries[rel] = meta
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

// runRecordingName is the recording inside a run folder.
const runRecordingName = "recording.jsonl"

// recordingLabels reads a recording's labels from its path under the store root. Two layouts are written today:
//
//	run folder (run logging): {workspace}/{model}/{task}/{timestamp}_{session}/recording.jsonl
//	flat file (--record, a run without a folder): {model}/{task}/{timestamp}_{session}.jsonl
func recordingLabels(rel string) RecordingMeta {
	parts := strings.Split(rel, string(filepath.Separator))
	var meta RecordingMeta
	if n := len(parts); n == 5 && parts[n-1] == runRecordingName {
		meta.WorkspaceID, meta.Model, meta.AutomationName = parts[0], parts[1], parts[2]
		meta.Timestamp, meta.SessionID = parseRecordingFilename(parts[3] + ".jsonl")
		return meta
	}
	if len(parts) >= 1 {
		meta.Model = parts[0]
	}
	if len(parts) >= 2 {
		meta.AutomationName = parts[1]
	}
	meta.Timestamp, meta.SessionID = parseRecordingFilename(parts[len(parts)-1])
	return meta
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
