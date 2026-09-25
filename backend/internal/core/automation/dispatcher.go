package automation

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"llm-proxy/internal/core/eventbus"
	"llm-proxy/internal/core/runlane"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"

	"github.com/robfig/cron/v3"
)

// defaultAutomationTimeout is the wall-clock cap for a single automation run
// when the automation pins no model with a longer timeout_minutes. The
// effective per-run bound is max(defaultAutomationTimeout, model timeout), so
// a slow model configured with timeout_minutes > 10m is not cut off mid-run.
const defaultAutomationTimeout = 10 * time.Minute

// Dispatcher manages automation execution via a cron scheduler.
type Dispatcher struct {
	registry    *AutomationRegistry
	persistence *persistence.WorkspaceManager
	executor    TaskExecutor
	cron        *cron.Cron
	logger      logging.Logger
	lane        *runlane.Scheduler
	laneKeyFor  func(model string) runlane.LaneKey

	mu      sync.RWMutex
	jobs    map[string]cron.EntryID // automationID -> cron.EntryID
	metrics *DispatcherMetrics

	historyMu     sync.RWMutex
	globalHistory []models.AutomationRun
	events        *eventbus.Bus

	runMu      sync.RWMutex
	activeRuns map[string]*activeRun // workspaceID -> run metadata
	// diagnosticDelay is how long StopAutomation waits before force-killing an
	// unresponsive shell. Zero means the default (defaultDiagnosticDelay).
	diagnosticDelay time.Duration
	// automationTimeout is the base wall-clock cap for a single automation run
	// (overridable via WithAutomationTimeout); the effective bound is max(this,
	// the pinned model's timeout_minutes).
	automationTimeout time.Duration

	stopOnce sync.Once
}

// activeRun tracks a running automation session including its cancellation
// function and optional shell process group ID for force-termination.
type activeRun struct {
	cancel     context.CancelFunc
	pgid       int                // negated PGID for syscall.Kill; 0 when no active shell
	diagCancel context.CancelFunc // cancels the StopAutomation diagnostic goroutine
}

// DispatcherDeps groups the dispatcher's required collaborators. Lane and
// LaneKeyFor are mandatory: a missing injection must fail loudly instead of
// silently losing the run-scheduling guarantee.
type DispatcherDeps struct {
	Persistence *persistence.WorkspaceManager
	Executor    TaskExecutor
	Logger      logging.Logger
	Lane        *runlane.Scheduler
	LaneKeyFor  func(model string) runlane.LaneKey
}

func NewDispatcher(
	deps DispatcherDeps,
	opts ...Option,
) (*Dispatcher, error) {
	if deps.Lane == nil {
		return nil, errors.New("dispatcher: run lane is required")
	}
	if deps.LaneKeyFor == nil {
		return nil, errors.New("dispatcher: lane key resolver is required")
	}
	d := &Dispatcher{
		registry:    NewAutomationRegistry(),
		persistence: deps.Persistence,
		executor:    deps.Executor,
		// cron.Recover wraps every scheduled job so a panic inside a run can
		// never crash the whole service (robfig/cron runs jobs in bare
		// goroutines with no recovery of its own).
		cron: cron.New(
			cron.WithParser(cron.NewParser(
				cron.SecondOptional|cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow|cron.Descriptor,
			)),
			cron.WithChain(cron.Recover(cronLogger{deps.Logger})),
		),
		logger:            deps.Logger,
		lane:              deps.Lane,
		laneKeyFor:        deps.LaneKeyFor,
		jobs:              make(map[string]cron.EntryID),
		metrics:           &DispatcherMetrics{},
		events:            eventbus.NewBus(),
		activeRuns:        make(map[string]*activeRun),
		diagnosticDelay:   defaultDiagnosticDelay,
		automationTimeout: defaultAutomationTimeout,
	}

	for _, opt := range opts {
		opt(d)
	}

	return d, nil
}

// cronLogger adapts the app logger to robfig/cron's Logger interface so
// cron.Recover can report panicked jobs through the normal logging pipeline.
type cronLogger struct{ l logging.Logger }

func (c cronLogger) Info(msg string, kv ...any) { c.l.Info(msg, kv...) }
func (c cronLogger) Error(err error, msg string, kv ...any) {
	c.l.Error(msg, append([]any{"error", err}, kv...)...)
}

type Option func(*Dispatcher)

// WithAutomationTimeout sets the base wall-clock cap for a single automation
// run. The effective bound is max(this, the pinned model's timeout_minutes),
// so a slow model configured with a longer timeout is not cut off mid-run.
func WithAutomationTimeout(t time.Duration) Option {
	return func(d *Dispatcher) {
		if t > 0 {
			d.automationTimeout = t
		}
	}
}

type DispatcherMetrics struct {
	TotalExecutions      int64
	SuccessfulExecutions int64
	FailedExecutions     int64
	SkippedExecutions    int64
	QueuedExecutions     int64
	PreemptedExecutions  int64
	TotalLatency         time.Duration
	mu                   sync.Mutex
}

func (m *DispatcherMetrics) RecordExecution(success, skipped bool, latency time.Duration) {
	atomic.AddInt64(&m.TotalExecutions, 1)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalLatency += latency
	if skipped {
		atomic.AddInt64(&m.SkippedExecutions, 1)
	} else if success {
		atomic.AddInt64(&m.SuccessfulExecutions, 1)
	} else {
		atomic.AddInt64(&m.FailedExecutions, 1)
	}
}

// RecordQueued and RecordPreempted count scheduler admissions and lane
// preemptions. Both counters are in-memory only — skipped/preempted runs
// produce no persisted history entry, so LoadHistory cannot reconstruct them;
// they reset on restart together with the lanes themselves.
func (m *DispatcherMetrics) RecordQueued()    { atomic.AddInt64(&m.QueuedExecutions, 1) }
func (m *DispatcherMetrics) RecordPreempted() { atomic.AddInt64(&m.PreemptedExecutions, 1) }

// DispatcherMetricsSnapshot is a consistent copy of the counters for reporting.
// Counters are read atomically; TotalLatency is read under the mutex that
// RecordExecution writes it with.
type DispatcherMetricsSnapshot struct {
	TotalExecutions      int64
	SuccessfulExecutions int64
	FailedExecutions     int64
	SkippedExecutions    int64
	QueuedExecutions     int64
	PreemptedExecutions  int64
	TotalLatency         time.Duration
}

// Snapshot copies the counters for a reader (e.g. the metrics endpoint) without
// racing RecordExecution: the atomic counters are loaded individually and the
// mutex-guarded latency is read under the lock.
func (m *DispatcherMetrics) Snapshot() DispatcherMetricsSnapshot {
	m.mu.Lock()
	latency := m.TotalLatency
	m.mu.Unlock()
	return DispatcherMetricsSnapshot{
		TotalExecutions:      atomic.LoadInt64(&m.TotalExecutions),
		SuccessfulExecutions: atomic.LoadInt64(&m.SuccessfulExecutions),
		FailedExecutions:     atomic.LoadInt64(&m.FailedExecutions),
		SkippedExecutions:    atomic.LoadInt64(&m.SkippedExecutions),
		QueuedExecutions:     atomic.LoadInt64(&m.QueuedExecutions),
		PreemptedExecutions:  atomic.LoadInt64(&m.PreemptedExecutions),
		TotalLatency:         latency,
	}
}

func (d *Dispatcher) Events() *eventbus.Bus {
	return d.events
}

func (d *Dispatcher) ListAll() []*AutomationEntry {
	return d.registry.ListAll()
}

func (d *Dispatcher) Metrics() *DispatcherMetrics {
	return d.metrics
}

// IsAutomationRunning reports whether an automation is currently executing
// in the given workspace. It is a read-only check safe for use in HTTP
// handlers and observability — it never cancels or mutates the run.
func (d *Dispatcher) IsAutomationRunning(workspaceID string) bool {
	d.runMu.Lock()
	defer d.runMu.Unlock()
	_, ok := d.activeRuns[workspaceID]
	return ok
}

// Persistence returns the underlying WorkspaceManager.
func (d *Dispatcher) Persistence() *persistence.WorkspaceManager {
	return d.persistence
}
