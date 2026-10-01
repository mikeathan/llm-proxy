package persistence

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"llm-proxy/models"
)

// MaxTreeEntries caps one workspace tree listing. The cap is applied
// breadth-first, so a huge tree loses its deepest entries first and
// WorkspaceTree.Truncated tells the UI it happened.
const MaxTreeEntries = 5000

const (
	treeRoot         = "."
	hiddenNamePrefix = "."
)

// heavyDirNames are listed (collapsed) but never descended into: they are
// dependency or build output, and would otherwise exhaust the entry cap.
var heavyDirNames = []string{"node_modules", ".venv", "venv", "__pycache__", "dist", "build", "target"}

// ListTree returns the workspace's files and directories, recursively, as
// sorted workspace-relative slash paths (plan Phase 3). A missing workspace
// lists as empty.
func (m *WorkspaceManager) ListTree(workspaceID string) (models.WorkspaceTree, error) {
	var tree models.WorkspaceTree
	err := m.withWorkspaceRoot(workspaceID, false, func(root *os.Root) error {
		var listErr error
		tree, listErr = listTree(root, MaxTreeEntries)
		return listErr
	})
	if errors.Is(err, fs.ErrNotExist) {
		return models.WorkspaceTree{Entries: []models.TreeEntry{}}, nil
	}
	return tree, err
}

// listTree walks root breadth-first, keeping at most limit entries.
// Exclusions: dot-names (including .sandbox) at every depth; the reserved
// metadata names and the legacy sessions directory at the root only. Symlinks
// are listed as files only when they resolve, inside the root, to a regular
// file; escaping, broken and directory links are omitted (never followed).
func listTree(root *os.Root, limit int) (models.WorkspaceTree, error) {
	fsys := root.FS()
	tree := models.WorkspaceTree{Entries: []models.TreeEntry{}}
	queue := []string{treeRoot}
	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]
		children, err := fs.ReadDir(fsys, dir)
		if err != nil {
			return tree, fmt.Errorf("read %s: %w", dir, err)
		}
		for _, child := range children {
			entry, descend, ok := classifyTreeEntry(root, dir, child)
			if !ok {
				continue
			}
			if len(tree.Entries) == limit {
				tree.Truncated = true
				sortTree(&tree)
				return tree, nil
			}
			tree.Entries = append(tree.Entries, entry)
			if descend {
				queue = append(queue, entry.Path)
			}
		}
	}
	sortTree(&tree)
	return tree, nil
}

// classifyTreeEntry turns a directory entry into a tree entry, reporting
// whether to descend into it and whether to list it at all.
func classifyTreeEntry(root *os.Root, dir string, child fs.DirEntry) (entry models.TreeEntry, descend, ok bool) {
	name := child.Name()
	if strings.HasPrefix(name, hiddenNamePrefix) || isReservedAtRoot(dir, name) {
		return entry, false, false
	}
	p := path.Join(dir, name)
	switch {
	case child.Type()&fs.ModeSymlink != 0:
		info, err := root.Stat(p) // fails for links that escape the root
		if err != nil || !info.Mode().IsRegular() {
			return entry, false, false
		}
		return models.TreeEntry{Path: p, Type: models.TreeEntryFile}, false, true
	case child.IsDir():
		heavy := slices.Contains(heavyDirNames, name)
		return models.TreeEntry{Path: p, Type: models.TreeEntryDir, Collapsed: heavy}, !heavy, true
	case child.Type().IsRegular():
		return models.TreeEntry{Path: p, Type: models.TreeEntryFile}, false, true
	default:
		return entry, false, false // sockets, devices, pipes
	}
}

func isReservedAtRoot(dir, name string) bool {
	return dir == treeRoot && (slices.Contains(workspaceFiles, name) || name == legacySessionsDirName)
}

func sortTree(tree *models.WorkspaceTree) {
	slices.SortFunc(tree.Entries, func(a, b models.TreeEntry) int { return strings.Compare(a.Path, b.Path) })
}
