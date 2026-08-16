package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RemoveScopedDir deletes dir only when it resolves beneath base and is a real
// directory (not a symlink), preventing path-traversal deletion of unrelated
// data. A missing dir is not an error. It is the single shared implementation
// for deleting scoped directories such as automation run trees and run dirs.
func RemoveScopedDir(base, dir string) error {
	rel, err := filepath.Rel(base, dir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return fmt.Errorf("refusing to remove %s: outside %s", dir, base)
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to remove %s: it is a symlink", dir)
	}
	if !fi.IsDir() {
		return fmt.Errorf("refusing to remove %s: not a directory", dir)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove %s: %w", dir, err)
	}
	return nil
}
