package agentwatch

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"llm-proxy-stacks/sdk"

	"llm-proxy/internal/core/tools"
)

const (
	// maxScanFile is the largest file the pass reads; agent output above it is a log or a download, not a note.
	maxScanFile = 1 << 20
	// maxScanFiles bounds the files one pass reads over all workspaces.
	maxScanFiles = 5000
)

// plainPath is a workspace-relative path the record may keep and show: letters, digits and a few separators.
var plainPath = regexp.MustCompile(`^[A-Za-z0-9 _./()-]{1,200}$`)

// WorkspaceDir is one workspace and its folder.
type WorkspaceDir struct {
	ID  string
	Dir string
}

// Scanner is the stack's SecretScan port. Workspaces lists the workspace folders; nil turns the pass off.
type Scanner struct {
	Workspaces func() ([]WorkspaceDir, error)
}

// errScanBudget ends a pass that read its share of files.
var errScanBudget = errors.New("scan budget used")

// Scan reads the files agents wrote in every workspace (not the program's own dot-folders, not dependency folders, not links, not files over
// maxScanFile) and reports each place that holds a secret-shaped string, by workspace, place and kind only. The value never leaves the file.
func (s Scanner) Scan(ctx context.Context) ([]stacksdk.SecretHit, error) {
	if s.Workspaces == nil {
		return nil, nil
	}
	dirs, err := s.Workspaces()
	if err != nil {
		return nil, stacksdk.Errorf(stacksdk.CodeInternal, "the workspaces could not be listed")
	}
	budget := maxScanFiles
	var out []stacksdk.SecretHit
	for _, d := range dirs {
		if ctx.Err() != nil || budget <= 0 {
			break
		}
		for _, f := range secretsIn(ctx, d.Dir, &budget) {
			out = append(out, stacksdk.SecretHit{Workspace: d.ID, Where: f.where, Kind: f.kind})
		}
	}
	return out, nil
}

// secretFound is one place and the kind of secret in it.
type secretFound struct{ where, kind string }

// secretsIn walks one workspace folder.
func secretsIn(ctx context.Context, root string, budget *int) []secretFound {
	var out []secretFound
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || ctx.Err() != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if *budget <= 0 {
			return errScanBudget
		}
		*budget--
		if kind := secretKindOfFile(path, d); kind != "" {
			rel, relErr := filepath.Rel(root, path)
			if relErr == nil && plainPath.MatchString(filepath.ToSlash(rel)) {
				out = append(out, secretFound{where: filepath.ToSlash(rel), kind: kind})
			}
		}
		return nil
	})
	return out
}

// secretKindOfFile reads a file of at most maxScanFile bytes and names the first secret shape in it.
func secretKindOfFile(path string, d fs.DirEntry) string {
	info, err := d.Info()
	if err != nil || info.Size() > maxScanFile {
		return ""
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return tools.SecretKindIn(string(body))
}
