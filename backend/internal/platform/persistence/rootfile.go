package persistence

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// rootFile is one JSON file in the application's data folder, outside every workspace: read with the missing-file and
// corrupt-file rules of readJSONFile, replaced atomically, deletable. The device inventory and the stored identities use it.
type rootFile[T any] struct {
	path  string
	tmp   string
	label string
}

func newRootFile[T any](dir, name, label string) rootFile[T] {
	return rootFile[T]{path: filepath.Join(dir, name), tmp: name[:len(name)-len(filepath.Ext(name))] + "-*.json.tmp", label: label}
}

func (f rootFile[T]) read() (T, error) { return readJSONFile[T](f.path, f.label) }

func (f rootFile[T]) write(v T) error {
	if err := writeJSONFile(f.path, f.tmp, f.label, v); err != nil {
		return fmt.Errorf("failed to save the %s: %w", f.label, err)
	}
	return nil
}

// remove deletes the file and any copy set aside as corrupt. There being none is not an error.
func (f rootFile[T]) remove() error {
	for _, path := range []string{f.path, f.path + corruptSuffix} {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("failed to delete the %s: %w", f.label, err)
		}
	}
	return nil
}
