package automation

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"llm-proxy/models"
)

// cronAutomation builds a cron-triggered automation for scheduling tests.
func cronAutomation(name, schedule string) *models.Automation {
	return &models.Automation{
		Name:     name,
		Trigger:  models.TriggerConfig{Type: models.TriggerCron, Value: schedule},
		TaskFile: "task.md",
	}
}

// manualAutomation builds a manual-triggered automation for scheduling tests.
func manualAutomation(name string) *models.Automation {
	return &models.Automation{
		Name:     name,
		Trigger:  models.TriggerConfig{Type: models.TriggerManual},
		TaskFile: "task.md",
	}
}

// TestDispatcher_Register_ReplacesExistingSchedule guards the invariant that a
// re-register (trigger change or schedule edit) replaces the automation's cron
// entry instead of orphaning it. An orphaned entry kept firing alongside the
// new one — e.g. a cron automation switched to manual still ran at its old
// time (#stale-cron-entry).
func TestDispatcher_Register_ReplacesExistingSchedule(t *testing.T) {
	tests := []struct {
		name        string
		initial     *models.Automation
		updated     *models.Automation
		wantJobs    int
		wantCronLen int
	}{
		{
			name:        "cron switched to manual removes cron job",
			initial:     cronAutomation("a", "0 0 * * *"),
			updated:     manualAutomation("a"),
			wantJobs:    0,
			wantCronLen: 0,
		},
		{
			name:        "cron schedule change replaces cron job",
			initial:     cronAutomation("a", "0 0 * * *"),
			updated:     cronAutomation("a", "0 1 * * *"),
			wantJobs:    1,
			wantCronLen: 1,
		},
		{
			name:        "manual re-registered stays unscheduled",
			initial:     manualAutomation("a"),
			updated:     manualAutomation("a"),
			wantJobs:    0,
			wantCronLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newLaneDispatcher(t, &mockExecutor{})

			if err := d.Register("ws", tt.initial); err != nil {
				t.Fatalf("initial register: %v", err)
			}
			if err := d.Register("ws", tt.updated); err != nil {
				t.Fatalf("updated register: %v", err)
			}

			if got := len(d.jobs); got != tt.wantJobs {
				t.Errorf("jobs = %d, want %d", got, tt.wantJobs)
			}
			if got := len(d.cron.Entries()); got != tt.wantCronLen {
				t.Errorf("cron entries = %d, want %d (orphaned entry would double-fire)",
					got, tt.wantCronLen)
			}
		})
	}
}

// TestDispatcher_Register_ConcurrentReRegisterLeavesOneEntry stresses the
// replace-and-add path: an HTTP edit and the config watcher can re-register the
// same automation at once. The end state must be exactly one cron entry, never
// an orphaned second one.
func TestDispatcher_Register_ConcurrentReRegisterLeavesOneEntry(t *testing.T) {
	d := newLaneDispatcher(t, &mockExecutor{})

	const workers = 50
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		schedule := "0 0 * * *"
		if i%2 == 1 {
			schedule = "0 1 * * *"
		}
		wg.Add(1)
		go func(schedule string) {
			defer wg.Done()
			if err := d.Register("ws", cronAutomation("a", schedule)); err != nil {
				t.Errorf("concurrent register: %v", err)
			}
		}(schedule)
	}
	wg.Wait()

	if got := len(d.jobs); got != 1 {
		t.Errorf("jobs = %d, want 1", got)
	}
	if got := len(d.cron.Entries()); got != 1 {
		t.Errorf("cron entries = %d, want 1 (concurrent re-register orphaned an entry)", got)
	}
}

// TestDispatcher_StartWatcher_WatchesWorkspaceMetadataDirs guards that the
// hot-reload watcher watches each per-workspace metadata directory, where
// config.yaml actually lives. Watching only the workspaces content root (the
// old BaseDir) meant config.yaml edits were never observed, so the reconciler
// never ran.
func TestDispatcher_StartWatcher_WatchesWorkspaceMetadataDirs(t *testing.T) {
	d := newLaneDispatcher(t, &mockExecutor{})

	metadataRoot := d.persistence.MetadataRoot()
	workspaceDir := filepath.Join(metadataRoot, "ws-1")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("create workspace metadata dir: %v", err)
	}

	watcher, err := d.startWatcher(context.Background())
	if err != nil {
		t.Fatalf("startWatcher: %v", err)
	}
	defer watcher.Close()

	watched := watcher.WatchList()
	for _, want := range []string{metadataRoot, workspaceDir} {
		if !slices.Contains(watched, want) {
			t.Errorf("watcher does not watch %q; watched=%v", want, watched)
		}
	}
}
