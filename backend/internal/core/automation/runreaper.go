package automation

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"llm-proxy/internal/platform/logging"
)

// runDirTimeLayout matches the {timestamp}_{session} run directory prefix
// created by NewRunDir.
const runDirTimeLayout = "20060102T150405Z"

// DefaultRunReaperInterval is the prune cadence and DefaultRunRetention the
// retention window used when wiring the reaper at startup.
const (
	DefaultRunReaperInterval = time.Hour
	DefaultRunRetention      = 30 * 24 * time.Hour
)

// RunReaper prunes completed run directories older than the retention window
// so a long-lived scheduled service does not fill the disk with per-run
// events.jsonl / recording.jsonl files. It mirrors the ledger Cleaner pattern:
// create with NewRunReaper, call Start(ctx) to run in a background goroutine.
type RunReaper struct {
	runsRoot  string
	interval  time.Duration
	retention time.Duration
}

// NewRunReaper creates a reaper over runsRoot.
func NewRunReaper(runsRoot string, interval, retention time.Duration) *RunReaper {
	return &RunReaper{runsRoot: runsRoot, interval: interval, retention: retention}
}

// Start runs the prune loop — once immediately, then on the interval — until
// ctx is cancelled.
func (r *RunReaper) Start(ctx context.Context) {
	logging.Info("Run reaper started", "root", r.runsRoot, "interval", r.interval, "retention", r.retention)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.prune(ctx)
	for {
		select {
		case <-ticker.C:
			r.prune(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// prune removes run directories older than the retention window, then removes
// the now-empty workspace/model/task parent directories (deepest first).
func (r *RunReaper) prune(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	cutoff := time.Now().UTC().Add(-r.retention)

	var oldRuns, dirs []string
	_ = filepath.WalkDir(r.runsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		dirs = append(dirs, path)
		if r.isRunDir(d.Name()) && r.dirTime(d.Name()).Before(cutoff) {
			oldRuns = append(oldRuns, path)
		}
		return nil
	})

	for _, p := range oldRuns {
		if err := os.RemoveAll(p); err != nil {
			logging.Warn("Run reaper: failed to remove run dir", "path", p, "error", err)
		}
	}

	// Clean up empty parents (each prune leaves ws/model/task dirs behind).
	sort.Slice(dirs, func(i, j int) bool { return depth(dirs[i]) > depth(dirs[j]) })
	for _, p := range dirs {
		if p != r.runsRoot && os.Remove(p) == nil {
			logging.Debug("Run reaper: removed empty dir", "path", p)
		}
	}
}

// isRunDir reports whether a directory name matches the {timestamp}_{session}
// run-directory pattern.
func (r *RunReaper) isRunDir(name string) bool {
	if len(name) < len(runDirTimeLayout)+2 || name[len(runDirTimeLayout)] != '_' {
		return false
	}
	_, err := time.Parse(runDirTimeLayout, name[:len(runDirTimeLayout)])
	return err == nil
}

// dirTime extracts the run start time from the {timestamp}_{session} prefix.
// A zero time is returned on parse failure (treated as older than any cutoff).
func (r *RunReaper) dirTime(name string) time.Time {
	t, err := time.Parse(runDirTimeLayout, name[:len(runDirTimeLayout)])
	if err != nil {
		return time.Time{}
	}
	return t
}

// depth counts path separators, used to sort directories deepest-first so
// empty parents are removed bottom-up.
func depth(p string) int {
	return strings.Count(p, string(filepath.Separator))
}
