package automation

import (
	"context"
	"sync/atomic"
	"time"
)

// seedMetricsFromHistory initialises the execution counters from the persisted
// run history (each workspace's state.json, the single source of truth for
// runs) so the metrics survive a restart. The history itself is never held in
// memory: readers such as the global activity feed query persistence directly.
func (d *Dispatcher) seedMetricsFromHistory(ctx context.Context) {
	runs, err := d.persistence.RecentRuns(ctx)
	if err != nil {
		d.logger.Error("Failed to read run history for metrics", "error", err)
		return
	}

	// Reset and recalculate metrics from history. The counters are stored
	// atomically to match RecordExecution (which adds to them atomically);
	// TotalLatency is mutex-guarded, matching RecordExecution. Mixing atomic
	// and plain access to the same field would be a data race.
	var total, successful, failed int64
	var totalLatency time.Duration
	for _, run := range runs {
		total++
		totalLatency += time.Duration(run.DurationMs) * time.Millisecond
		if run.Error == "" {
			successful++
		} else {
			failed++
		}
	}
	atomic.StoreInt64(&d.metrics.TotalExecutions, total)
	atomic.StoreInt64(&d.metrics.SuccessfulExecutions, successful)
	atomic.StoreInt64(&d.metrics.FailedExecutions, failed)
	atomic.StoreInt64(&d.metrics.SkippedExecutions, 0)
	d.metrics.mu.Lock()
	d.metrics.TotalLatency = totalLatency
	d.metrics.mu.Unlock()

	d.logger.Info("Seeded execution metrics from run history", "total_executions", total)
}
