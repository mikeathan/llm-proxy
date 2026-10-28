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

// vulnIndexFileName is the local vulnerability data, one file in the application's data folder (outside the repository
// and every workspace): built by the importer, replaced whole.
const vulnIndexFileName = "vulnindex.json"

const vulnIndexLabel = "vulnerability data"

// VulnIndexStore keeps the vulnerability index in one file.
type VulnIndexStore struct {
	path string
}

// NewVulnIndexStore keeps the index in dir. Nothing is read or created until Read or Write.
func NewVulnIndexStore(dir string) *VulnIndexStore {
	return &VulnIndexStore{path: filepath.Join(dir, vulnIndexFileName)}
}

// Read returns the stored index. A missing file is an empty index, not an error; an undecodable one is quarantined as
// *.corrupt (so the next import does not overwrite the evidence) and reported as an error.
func (s *VulnIndexStore) Read() (models.VulnIndex, error) {
	return readJSONFile[models.VulnIndex](s.path, vulnIndexLabel)
}

// Write replaces the stored index atomically. It is written compact: the file can be tens of megabytes.
func (s *VulnIndexStore) Write(idx models.VulnIndex) error {
	data, err := json.Marshal(idx)
	if err != nil {
		return fmt.Errorf("failed to encode the %s: %w", vulnIndexLabel, err)
	}
	if err := storage.WriteAtomic(s.path, "vulnindex-*.json.tmp", data, storage.ClassUserContent); err != nil {
		return fmt.Errorf("failed to save the %s: %w", vulnIndexLabel, err)
	}
	return nil
}

// Delete removes the stored index, and any copy set aside as corrupt. There being none is not an error.
func (s *VulnIndexStore) Delete() error {
	for _, path := range []string{s.path, s.path + corruptSuffix} {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("failed to delete the %s: %w", vulnIndexLabel, err)
		}
	}
	return nil
}
