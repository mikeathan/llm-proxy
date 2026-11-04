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

// ubuntuIndexFileName is the downloaded Ubuntu security notices, one file beside the settings, outside every workspace and
// replaced whole by an update.
const ubuntuIndexFileName = "ubuntu-security.json"

const ubuntuIndexLabel = "Ubuntu security data"

// UbuntuIndexStore keeps the Ubuntu security index in one file.
type UbuntuIndexStore struct {
	path string
}

// NewUbuntuIndexStore keeps the index in dir. Nothing is read or created until Read or Write.
func NewUbuntuIndexStore(dir string) *UbuntuIndexStore {
	return &UbuntuIndexStore{path: filepath.Join(dir, ubuntuIndexFileName)}
}

// Read returns the stored index. A missing file is an empty index, not an error; an undecodable one is set aside as *.corrupt
// and reported as an error.
func (s *UbuntuIndexStore) Read() (models.UbuntuIndex, error) {
	return readJSONFile[models.UbuntuIndex](s.path, ubuntuIndexLabel)
}

// Write replaces the stored index atomically. It is written compact.
func (s *UbuntuIndexStore) Write(idx models.UbuntuIndex) error {
	data, err := json.Marshal(idx)
	if err != nil {
		return fmt.Errorf("failed to encode the %s: %w", ubuntuIndexLabel, err)
	}
	if err := storage.WriteAtomic(s.path, "ubuntu-security-*.json.tmp", data, storage.ClassUserContent); err != nil {
		return fmt.Errorf("failed to save the %s: %w", ubuntuIndexLabel, err)
	}
	return nil
}

// Delete removes the stored index, and any copy set aside as corrupt. There being none is not an error.
func (s *UbuntuIndexStore) Delete() error {
	for _, path := range []string{s.path, s.path + corruptSuffix} {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("failed to delete the %s: %w", ubuntuIndexLabel, err)
		}
	}
	return nil
}
