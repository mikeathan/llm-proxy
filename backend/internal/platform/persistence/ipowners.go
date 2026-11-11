package persistence

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"llm-proxy/internal/platform/storage"
)

// ipOwnersFileName holds the downloaded table of address owners, one file in the application's data folder. It is public data, so it is a plain
// file; the time it was downloaded is the file's own modification time.
const ipOwnersFileName = "destination-owners.bin"

// IPOwnersStore keeps the downloaded table of address owners in one file.
type IPOwnersStore struct{ path string }

func NewIPOwnersStore(dir string) *IPOwnersStore {
	return &IPOwnersStore{path: filepath.Join(dir, ipOwnersFileName)}
}

// Read returns the stored table and when it was written. A missing file is an empty table.
func (s *IPOwnersStore) Read() ([]byte, time.Time, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, time.Time{}, nil
	}
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to read the destination owners: %w", err)
	}
	info, err := os.Stat(s.path)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to read the destination owners: %w", err)
	}
	return data, info.ModTime(), nil
}

// Write replaces the stored table atomically and stamps it with the time it was downloaded.
func (s *IPOwnersStore) Write(data []byte, at time.Time) error {
	if err := storage.WriteAtomic(s.path, "destination-owners-*.bin.tmp", data, storage.ClassUserContent); err != nil {
		return fmt.Errorf("failed to save the destination owners: %w", err)
	}
	if err := os.Chtimes(s.path, at, at); err != nil {
		return fmt.Errorf("failed to stamp the destination owners: %w", err)
	}
	return nil
}

// Delete removes the file. There being none is not an error.
func (s *IPOwnersStore) Delete() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to delete the destination owners: %w", err)
	}
	return nil
}
