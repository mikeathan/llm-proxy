package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteAtomic writes data atomically: it creates a temp file in the destination
// directory, writes, fsyncs, and renames it into place so a crash mid-write
// never leaves a partial file. The parent dir is created/tightened per the
// class's directory mode, and the final file is chmod'd to the class's file mode
// (CreateTemp yields 0600, which is correct for secrets but too tight for user
// content). The class carries the canonical policy — callers pass a FileClass,
// never a raw octal, so the permission intent is always readable.
func WriteAtomic(dest, pattern string, data []byte, class FileClass) (err error) {
	dir := filepath.Dir(dest)
	dirPerm := class.dirMode()
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	if fi, err := os.Lstat(dir); err == nil && fi.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(dir, dirPerm); err != nil {
			return fmt.Errorf("tighten dir perms on %s: %w", dir, err)
		}
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
	// Apply the class's file mode to the renamed destination. The temp file is
	// 0600; for non-secret classes this tightens/closes the gap explicitly.
	if err = os.Chmod(dest, class.fileMode()); err != nil {
		return fmt.Errorf("chmod %s: %w", dest, err)
	}
	return nil
}
