package providers

import (
	"context"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/metrics"
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
}

func (r *RunningModel) LastUsedTime() time.Time {
	return r.LastUsed
}
