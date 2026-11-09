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

// hostTrafficFileName is the last capture_host summary, one file in the application's data folder, replaced whole by each look. Only the
// summary is kept (Constitution I.13): no frame, no payload, one file.
const hostTrafficFileName = "network-host-traffic.json"

const hostTrafficLabel = "last host traffic summary"

// HostTrafficStore keeps the last capture_host summary.
type HostTrafficStore struct{ path string }

// NewHostTrafficStore keeps the summary in dir. Nothing is read or created until Read or Write.
func NewHostTrafficStore(dir string) *HostTrafficStore {
	return &HostTrafficStore{path: filepath.Join(dir, hostTrafficFileName)}
}

// Read returns the last summary; a missing file is the zero value (At is zero), not an error. An undecodable file is set aside as *.corrupt and reported as an error once.
func (s *HostTrafficStore) Read() (models.HostTraffic, error) {
	return readJSONFile[models.HostTraffic](s.path, hostTrafficLabel)
}

// Write replaces the summary atomically.
func (s *HostTrafficStore) Write(t models.HostTraffic) error {
	data, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("failed to encode the %s: %w", hostTrafficLabel, err)
	}
	if err := storage.WriteAtomic(s.path, "network-host-traffic-*.json.tmp", data, storage.ClassUserContent); err != nil {
		return fmt.Errorf("failed to save the %s: %w", hostTrafficLabel, err)
	}
	return nil
}

// Delete removes the summary and any copy set aside as corrupt.
func (s *HostTrafficStore) Delete() error {
	for _, path := range []string{s.path, s.path + corruptSuffix} {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("failed to delete the %s: %w", hostTrafficLabel, err)
		}
	}
	return nil
}
