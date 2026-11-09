package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// captureWindowFileName is the last capture_window summary, one file in the application's data folder, replaced whole by each run.
const captureWindowFileName = "network-capture-window.json"

const captureWindowLabel = "last capture summary"

// CaptureWindowStore keeps the last capture_window summary.
type CaptureWindowStore struct{ path string }

// NewCaptureWindowStore keeps the summary in dir. Nothing is read or created until Read or Write.
func NewCaptureWindowStore(dir string) *CaptureWindowStore {
	return &CaptureWindowStore{path: filepath.Join(dir, captureWindowFileName)}
}

// Read returns the last summary; a missing file is the zero value (At is zero), not an error. An undecodable file is set aside as *.corrupt.
func (s *CaptureWindowStore) Read() (models.CaptureSummary, error) {
	return readJSONFile[models.CaptureSummary](s.path, captureWindowLabel)
}

// Write replaces the summary atomically.
func (s *CaptureWindowStore) Write(c models.CaptureSummary) error {
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to encode the %s: %w", captureWindowLabel, err)
	}
	if err := storage.WriteAtomic(s.path, "network-capture-window-*.json.tmp", data, storage.ClassUserContent); err != nil {
		return fmt.Errorf("failed to save the %s: %w", captureWindowLabel, err)
	}
	return nil
}

// Delete removes the summary and any copy set aside as corrupt.
func (s *CaptureWindowStore) Delete() error {
	for _, path := range []string{s.path, s.path + corruptSuffix} {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("failed to delete the %s: %w", captureWindowLabel, err)
		}
	}
	return nil
}
