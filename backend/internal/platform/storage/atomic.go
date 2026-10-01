package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
)

const (
	// tempSuffixBytes is the random part of a root-scoped temp file name.
	tempSuffixBytes = 8
	// tempFileMode matches os.CreateTemp; the class mode is applied after rename.
	tempFileMode = 0o600
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

// WriteAtomicInRoot is WriteAtomic for a file addressed relative to root: every
// operation goes through the *os.Root, so neither ".." nor a symlink can place
// the file outside it (no check-then-use gap). name is slash-separated and may
// be nested; the temp file is created beside the target from the target's base
// name, so nested names never put a separator in the temp pattern.
func WriteAtomicInRoot(root *os.Root, name string, data []byte, class FileClass) (err error) {
	dir := path.Dir(name)
	if err := prepareRootDir(root, dir, class); err != nil {
		return err
	}
	suffix := make([]byte, tempSuffixBytes)
	if _, err := rand.Read(suffix); err != nil {
		return fmt.Errorf("temp name: %w", err)
	}
	tmp := path.Join(dir, path.Base(name)+"-"+hex.EncodeToString(suffix)+".tmp")
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, tempFileMode)
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	defer func() {
		if err != nil {
			_ = root.Remove(tmp)
		}
	}()
	if err = writeSyncClose(f, data); err != nil {
		return err
	}
	if err = root.Rename(tmp, name); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	if err = root.Chmod(name, class.fileMode()); err != nil {
		return fmt.Errorf("chmod %s: %w", name, err)
	}
	return nil
}

// prepareRootDir creates dir inside root and tightens it to the class's
// directory mode, as WriteAtomic does for its destination directory.
func prepareRootDir(root *os.Root, dir string, class FileClass) error {
	if err := root.MkdirAll(dir, class.dirMode()); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	if fi, err := root.Lstat(dir); err == nil && fi.Mode().Perm()&0o077 != 0 {
		if err := root.Chmod(dir, class.dirMode()); err != nil {
			return fmt.Errorf("tighten dir perms on %s: %w", dir, err)
		}
	}
	return nil
}

// writeSyncClose writes data, fsyncs and closes f, closing it on every path.
func writeSyncClose(f *os.File, data []byte) error {
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	return nil
}
