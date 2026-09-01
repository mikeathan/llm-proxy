package providers

import (
	"context"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/platform/procwatch"
	"llm-proxy/models"
	"os/exec"
	"time"
)

// RunningModel represents a managed local model process.
type RunningModel struct {
	Cfg        models.ModelConfig
	Cmd        *exec.Cmd
	Cancel     context.CancelFunc
	Started    time.Time
	LastUsed   time.Time
	Logs       *logging.BufferLogger
	Throughput *metrics.TokenTracker

	// watch owns Cmd.Wait and detects an unexpected exit (crash on launch — bad
	// args, missing model). It is the single owner of Wait, so shutdown paths
	// wait on Done() rather than calling Wait a second time. Shared with
	// internal/shell (persistentShell) via procwatch.
	watch *procwatch.Watch
}

// StartWatch launches the process-exit watch (shared procwatch pattern). It
// must be called exactly once per RunningModel, after the command has started.
func (r *RunningModel) StartWatch() {
	r.watch = procwatch.New(r.Cmd)
}

// Done returns the process-exit channel. Returns nil if StartWatch was never
// called (a RunningModel not created via a real launch).
func (r *RunningModel) Done() <-chan struct{} {
	if r.watch == nil {
		return nil
	}
	return r.watch.Done()
}

// Exited reports whether the underlying process has terminated. A local model
// is a server that must stay running while active, so any unexpected exit
// (clean or not) means it is no longer serving.
func (r *RunningModel) Exited() bool {
	if r.watch == nil {
		return false
	}
	return r.watch.Exited()
}

// Err returns the exit error if the process has terminated, else nil.
func (r *RunningModel) Err() error {
	if r.watch == nil {
		return nil
	}
	return r.watch.Err()
}

func (r *RunningModel) LastUsedTime() time.Time {
	return r.LastUsed
}
