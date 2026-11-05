package agentwatch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"llm-proxy/internal/core/tools"
	"llm-proxy/models"
)

const (
	// maxScanFile is the largest file the pass reads; agent output above it is a log or a download, not a note.
	maxScanFile = 1 << 20
	// maxScanFiles bounds the files one pass reads over all workspaces.
	maxScanFiles = 5000
	// scanEvery is how often the pass runs; firstScanAfter is the wait after start, so it does not run during startup.
	scanEvery      = 24 * time.Hour
	firstScanAfter = 10 * time.Minute
	// maxSeenSecrets bounds the remembered findings, so a secret is reported once.
	maxSeenSecrets = 1000
)

// plainPath is a workspace-relative path the record may keep and show: letters, digits and a few separators.
var plainPath = regexp.MustCompile(`^[A-Za-z0-9 _./()-]{1,200}$`)

// WorkspaceDir is one workspace and its folder.
type WorkspaceDir struct {
	ID  string
	Dir string
}

// errScanBudget ends a pass that read its share of files.
var errScanBudget = errors.New("scan budget used")

// scanSecrets reads the files agents wrote in every workspace (not the program's own dot-folders, not dependency folders, not
// links, not files over maxScanFile) and records each new place that holds a secret-shaped string, by place and kind only. One
// alert per workspace names how many places were new.
func (w *Watch) scanSecrets(ctx context.Context) {
	if w.deps.Workspaces == nil {
		return
	}
	dirs, err := w.deps.Workspaces()
	if err != nil {
		w.report(fmt.Errorf("failed to list the workspaces for the secrets pass: %w", err))
		return
	}
	budget := maxScanFiles
	for _, d := range dirs {
		if ctx.Err() != nil || budget <= 0 {
			return
		}
		found := secretsIn(ctx, d.Dir, &budget)
		if len(found) > 0 {
			w.recordSecrets(ctx, d.ID, found)
		}
	}
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

// recordSecrets keeps the places not reported before and alerts once for them.
func (w *Watch) recordSecrets(ctx context.Context, workspace string, found []secretFound) {
	w.update(ctx, func(r *models.AgentWatch, now time.Time) string {
		if r.SecretsSeen == nil {
			r.SecretsSeen = map[string]time.Time{}
		}
		fresh := 0
		for _, f := range found {
			key := workspace + "/" + f.where + "|" + f.kind
			if _, seen := r.SecretsSeen[key]; seen {
				continue
			}
			if len(r.SecretsSeen) >= maxSeenSecrets {
				break
			}
			r.SecretsSeen[key] = now
			addEvent(r, models.AgentWatchEvent{At: now, Kind: models.AgentSecretFound, Workspace: workspace, Where: f.where, Detail: f.kind})
			fresh++
		}
		if fresh == 0 {
			return ""
		}
		places := "1 file written by agents holds"
		if fresh != 1 {
			places = fmt.Sprintf("%d files written by agents hold", fresh)
		}
		return fmt.Sprintf("Agent watch: %s what looks like a key or token, in workspace %q. Open Settings, Safety to see where, then delete it and replace the key.",
			places, plainOr(workspace))
	})
}
