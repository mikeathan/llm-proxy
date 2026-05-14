package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// IsSecurePath checks if a path is within the allowed roots, resolving symlinks to prevent jail escape.
func IsSecurePath(path string, allowedRoots []string) (string, error) {
	if len(allowedRoots) == 0 {
		return "", fmt.Errorf("security violation: no allowed roots defined")
	}

	absLocal := resolveCanonical(path)
	
	for _, root := range allowedRoots {
		absRoot := resolveCanonical(root)

		// Robust prefix check: handle same path or subpath correctly
		if absLocal == absRoot || strings.HasPrefix(absLocal, absRoot+string(filepath.Separator)) {
			return absLocal, nil
		}
	}

	return "", fmt.Errorf("path access denied: outside of authorized workspaces")
}

// resolveCanonical resolves as much of the path as possible using EvalSymlinks.
// For non-existent paths, it recursively resolves parents until an existing directory is found.
func resolveCanonical(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}

	current := abs
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			if current == abs {
				return resolved
			}
			rel, _ := filepath.Rel(current, abs)
			return filepath.Join(resolved, rel)
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return abs
}
