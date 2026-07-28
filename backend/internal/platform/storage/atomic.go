package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteAtomic writes data atomically: it creates a temp file in the destination
// directory, writes, fsyncs, and renames it into place so a crash mid-write never
// leaves a partial file. dir is created with 0755 if missing.
func WriteAtomic(dest, pattern string, data []byte) (err error) {
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync temp: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err = os.Rename(tmp, dest); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
