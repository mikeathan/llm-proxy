---
status: superseded
date: 2026-04-01
superseded_by: SPEC-007
---

# Unified Automation Dispatcher & Agent IDE - Architectural Blueprint

**Date:** 1 April 2026  
**Target:** Go 1.24+ / Vue 3.5+  
**Status:** Superseded by SPEC-007

---

## I. Backend Architecture

### 1.1 Package Decomposition

```
backend/internal/
├── dispatcher/           # Core automation orchestration
│   ├── registry.go       # AutomationRegistry, Dispatcher
│   ├── executor.go       # TaskExecutor interface + default impl
│   └── strategies.go     # Isolated | Persistent execution contexts
│
├── trigger/              # Trigger implementations
│   ├── trigger.go        # Trigger interface (ShouldRun, NextRun)
│   ├── cron.go           # CronTrigger (robfig/cron)
│   ├── interval.go       # IntervalTrigger (@every X)
│   └── manual.go         # ManualTrigger (UI-initiated)
│
├── persistence/          # File I/O with safety guarantees
│   ├── workspace.go      # Workspace CRUD with flock locking
│   └── atomic.go         # Write-Rename-Sync pattern
│
├── telemetry/            # NEW: Efficiency metrics and KPI tracking
│   ├── metrics.go        # Efficiency Ratio (Value-Add vs. Admin time)
│   ├── latency.go        # Execution latency tracking
│   └── recorder.go      # Automated internal metadata (no manual audit logs)
│
├── api/
│   ├── dispatcher_handlers.go  # REST API
│   └── ws_dispatcher.go        # WebSocket events
│
└── app/
    ├── dispatcher_executor.go  # Implements TaskExecutor using AppServices
    └── bootstrap.go            # Wiring: Dispatcher → Scheduler integration
```

**Boundaries:**
- `trigger/` is **pure** — no dependencies on dispatcher, persistence, or app services
- `persistence/` encapsulates all file I/O; dispatcher calls `persistence` only
- `dispatcher/` orchestrates; it knows about triggers and persistence, but not HTTP or app internals
- `api/` depends on dispatcher for handler logic
- `telemetry/` is self-contained; records metrics from dispatcher but has no reverse dependencies

---

### 1.2 Trigger Interface

```go
// trigger/trigger.go
type Trigger interface {
    ShouldRun(lastRun time.Time) bool  // Evaluates if trigger should fire
    NextRun() time.Time                 // Returns next scheduled execution
    Type() string                       // "cron" | "interval" | "manual"
}

// Factory for deserialization
func NewTrigger(t models.TriggerConfig) (Trigger, error)
```

**Implementations:**

| Type | `ShouldRun` Logic | `NextRun` Logic |
|------|-------------------|-----------------|
| `cron` | `cron.NextRun() <= now` | `cron.NextRun()` |
| `interval` | `lastRun.IsZero() \|\| now.Sub(lastRun) >= interval` | `lastRun.Add(interval)` |
| `manual` | Always `false` (only fires via explicit trigger) | `time.Time{}` (unset) |

---

### 1.3 Automation Data Model

```go
// models/automation.go
type TriggerConfig struct {
    Type  string `yaml:"type"`  // "cron" | "interval" | "manual"
    Value string `yaml:"value"` // "*/5 * * * *" | "15m" | ""
}

type Automation struct {
    Name      string         `yaml:"name"`
    Trigger   TriggerConfig  `yaml:"trigger"`
    TaskFile  string         `yaml:"task_file"`
    Strategy  string         `yaml:"strategy"`  // "isolated" | "persistent"
}

type WorkspaceConfig struct {
    Automations []*Automation `yaml:"automations"`
    // ... existing fields
}
```

**config.yaml Example:**
```yaml
automations:
  - name: "Pulse"
    trigger: { type: "interval", value: "15m" }
    task_file: "heartbeat.md"
    strategy: "persistent"
  - name: "Morning Brief"
    trigger: { type: "cron", value: "0 8 * * *" }
    task_file: "daily.md"
    strategy: "isolated"
  - name: "Manual Review"
    trigger: { type: "manual" }
    task_file: "review.md"
    strategy: "isolated"
```

---

### 1.4 Execution Strategies

```go
// dispatcher/strategies.go
type ExecutionStrategy interface {
    Prepare(ctx context.Context, workspaceID string, state *models.AgentState) (context.Context, error)
    Name() string
}

// IsolatedStrategy: Fresh context, no memory from previous runs
type IsolatedStrategy struct{}

func (s *IsolatedStrategy) Prepare(ctx context.Context, workspaceID string, state *models.AgentState) (context.Context, error) {
    // Reset memory in prompt: "You are an agent. Previous context: [empty]"
    return ctx, nil
}

// PersistentStrategy: Includes previous state.json memory and persistent shell session
type PersistentStrategy struct{}

func (s *PersistentStrategy) Prepare(ctx context.Context, workspaceID string, state *models.AgentState) (context.Context, error) {
    // 1. Inject previous output/error into context:
    // "You are an agent. Previous result: {state.LastOutput}, Error: {state.LastError}"
    
    // 2. The Persistent Shell in the sandbox ensures 'cd' and environment state 
    // are preserved between calls within this workspace session.
    return ctx, nil
}
```

---

### 1.5 Dispatcher & Registry

```go
// dispatcher/registry.go
type AutomationEntry struct {
    ID        string
    Workspace string
    Name      string
    Trigger   trigger.Trigger
    TaskFile  string
    Strategy  ExecutionStrategy
    State     models.AgentState  // Current execution state
}

type AutomationRegistry struct {
    manager   *persistence.WorkspaceManager
    mu        sync.RWMutex
    automations map[string]*AutomationEntry  // key: "workspaceID/automationName"
}

type Dispatcher struct {
    registry  *AutomationRegistry
    executor  TaskExecutor
    scheduler *cron.Cron
    logger    *slog.Logger
}
```

**Key Operations:**
- `Register(workspaceID string, automation *models.Automation)` — Parse trigger, store entry
- `Unregister(workspaceID, automationName)` — Remove from registry and scheduler
- `Trigger(workspaceID, automationName)` — Manual trigger (bypasses ShouldRun)
- `List(workspaceID) []*AutomationEntry` — Enumerate automations for workspace

---

### 1.6 TaskExecutor Interface

```go
// dispatcher/executor.go
type TaskExecutor interface {
    Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error)
}

type ExecuteRequest struct {
    WorkspaceID  string
    AutomationName string
    TaskFile    string
    Strategy    ExecutionStrategy
    State       *models.AgentState  // Current persisted state
}

type ExecuteResponse struct {
    Output   string
    Error    error
    State    *models.AgentState  // Updated state to persist
}
```

**Default Implementation** (`dispatcher/default_executor.go`):
- Reads `TaskFile` content via persistence
- Calls LLM via `AppServices.LLM`
- Returns response with updated state

---

### 1.7 The "Smart Skip" Mechanism (Pulse Logic)

**Problem:** Frequent heartbeat triggers (e.g., every 5m) generate noise when system is healthy.

**Solution:** HEARTBEAT_OK suppression.

```go
// In TaskExecutor.Execute, after LLM call:
if strings.Contains(output, "HEARTBEAT_OK") {
    // System is healthy. Suppress logging, don't update LastOutput.
    // Instead: update LastPulse timestamp only
    resp.State.LastPulse = time.Now()
    resp.Output = ""  // Suppress noisy output
    return resp, nil
}
```

**State JSON (updated):**
```json
{
  "last_output": "",
  "last_pulse": "2026-04-01T10:30:00Z",
  "last_error": "",
  "next_run_at": "2026-04-01T10:45:00Z",
  "is_running": false
}
```

**Trigger `ShouldRun` enhancement for Pulse:**
```go
func (t *IntervalTrigger) ShouldRun(lastRun time.Time) bool {
    if lastRun.IsZero() {
        return true
    }
    // Check if last run was a HEARTBEAT_OK (no output, just pulse)
    // If so, optionally extend interval
    if time.Since(lastRun) < t.interval {
        return false
    }
    return true
}
```

---

### 1.8 Atomic Persistence with flock

```go
// persistence/workspace.go
func (m *WorkspaceManager) ExecuteWithLock(workspaceID string, fn func(*os.File) error) error {
    // Acquire exclusive lock
    f, err := m.AcquireLock(workspaceID)
    if err != nil {
        return err
    }
    defer m.ReleaseLock(f)

    return fn(f)
}

// Atomic WriteState using temp file + rename + sync
func (m *WorkspaceManager) WriteState(workspaceID string, state *models.AgentState) error {
    return m.ExecuteWithLock(workspaceID, func(f *os.File) error {
        // ... Write-Rename-Sync pattern
    })
}
```

**Race Prevention:**
- Concurrent triggers (Cron + Manual UI) both call `TryAcquireLock`
- Only one succeeds; the other receives `ErrLockHeld` and is skipped for this cycle
- `ExecuteHeartbeat` uses `TryAcquireLock` (non-blocking) to detect concurrent runs

---

### 1.9 Telemetry Package (Efficiency KPI Tracking)

The `telemetry/` package provides automated internal metadata tracking and efficiency metrics without manual audit logs.

```go
// telemetry/metrics.go
package telemetry

type EfficiencyMetrics struct {
    WorkspaceID    string
    AutomationName string

    // Time measurements
    ValueAddTime   time.Duration  // Actual LLM execution time
    AdminTime      time.Duration // Lock contention, file I/O, scheduling overhead

    // Computed KPI
    EfficiencyRatio float64      // ValueAddTime / (ValueAddTime + AdminTime)
}

type LatencyRecorder struct {
    mu      sync.Mutex
    samples []LatencySample
}

type LatencySample struct {
    WorkspaceID    string
    AutomationName string
    Latency        time.Duration
    Timestamp      time.Time
    Success        bool
}

func (r *LatencyRecorder) Record(exec *dispatcher.ExecuteRequest, elapsed time.Duration, success bool) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.samples = append(r.samples, LatencySample{
        WorkspaceID:    exec.WorkspaceID,
        AutomationName: exec.AutomationName,
        Latency:        elapsed,
        Timestamp:      time.Now(),
        Success:        success,
    })
}

func (r *EfficiencyRecorder) ComputeRatio(workspaceID, automationName string) float64 {
    // Aggregate: sum of value-add vs admin time over sliding window
    var valueAdd, admin time.Duration
    cutoff := time.Now().Add(-15 * time.Minute)

    r.mu.Lock()
    defer r.mu.Unlock()

    for _, s := range r.samples {
        if s.WorkspaceID != workspaceID || s.AutomationName != automationName {
            continue
        }
        if s.Timestamp.Before(cutoff) {
            continue
        }
        if s.Success {
            valueAdd += s.Latency
        } else {
            admin += s.Latency // Errors count as admin overhead
        }
    }

    total := valueAdd + admin
    if total == 0 {
        return 1.0
    }
    return float64(valueAdd) / float64(total)
}
```

**REST Endpoint for KPI Query:**
```
GET /api/telemetry/efficiency?workspace=ws-1&automation=Pulse
Response: { "efficiency_ratio": 0.94, "value_add_ms": 4500, "admin_ms": 280, "window": "15m" }
```

**WebSocket Events for Live Telemetry:**
| Event | Payload | Description |
|-------|---------|-------------|
| `telemetry_sample` | `{workspace, automation, latency_ms, success}` | Per-execution sample |
| `efficiency_update` | `{workspace, automation, ratio}` | Ratio recalculated |

---

### 1.10 Scalability Verification (2x Volume Support)

**Horizontal Worker Scaling:**
```go
type Dispatcher struct {
    registry  *AutomationRegistry
    executor  TaskExecutor
    scheduler *cron.Cron
    logger    *slog.Logger

    // Horizontal scaling: multiple workers process triggers
    workerPool errgroup.Group
    workerCount int  // Configurable; default: runtime.NumCPU()
}
```

**flock Contention Handling:**
- **Problem:** At high volume, concurrent `TryAcquireLock` calls on the same workspace create contention.
- **Solution:** Implement exponential backoff with jitter on lock acquisition:
```go
func (m *WorkspaceManager) TryAcquireLockWithBackoff(workspaceID string, maxRetries int) (*os.File, error) {
    for attempt := 0; attempt < maxRetries; attempt++ {
        f, err := m.TryAcquireLock(workspaceID)
        if err == nil {
            return f, nil
        }
        backoff := time.Duration(attempt*attempt) * 5 * time.Millisecond
        backoff += time.Duration(rand.Int63n(int64(backoff / 2))) // jitter
        time.Sleep(backoff)
    }
    return nil, fmt.Errorf("lock unavailable after %d attempts", maxRetries)
}
```

**Load Stress Test Requirements:**
```
- Target: 2x projected peak volume
- Metric: P99 latency under load < 500ms
- Metric: Lock contention retries < 3 per execution
- Metric: Zero missed triggers due to lock unavailability
```

---

### 1.11 Scheduler Integration

The Dispatcher **replaces** the simple Scheduler. Instead of one cron job per workspace, we have:

```
┌─────────────────────────────────────────────────────────┐
│                    Dispatcher                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────┐  │
│  │  Registry   │  │  Scheduler  │  │  TaskExecutor   │  │
│  │  (map of    │──│  (cron +    │──│  (LLM calls)    │  │
│  │  automations)│  │  interval)  │  │                 │  │
│  └─────────────┘  └─────────────┘  └─────────────────┘  │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
              ┌─────────────────────────┐
              │   persistence/          │
              │   WorkspaceManager      │
              │   (flock + atomic I/O)   │
              └─────────────────────────┘
```

**Startup Flow:**
1. `Dispatcher.Start(ctx)` loads all workspaces via `ListWorkspaces()`
2. For each workspace, reads `config.yaml` and registers all automations
3. Each automation's trigger is added to the cron scheduler
4. On `handleFSEvent` (config change), re-reads `config.yaml` and reconciles registry

---

## II. Frontend Architecture

### 2.1 Three-Pane Agent IDE Layout

```
┌──────────────────────────────────────────────────────────────────┐
│  Header: Workspace Selector │ Connection Status │ Settings      │
├────────────┬───────────────────────────┬─────────────────────────┤
│            │                           │  [tab: heartbeat.md]    │
│  Workspace │      File Browser         │  [tab: config.yaml]     │
│  List      │      (tree view)          │  [tab: state.json]      │
│            │                           ├─────────────────────────┤
│  ● ws-1    │  ▼ heartbeat.md           │                         │
│  ○ ws-2    │    instructions.md        │   Editor (Monaco/Code)  │
│  ● ws-3    │    state.json             │   or                     │
│            │    config.yaml            │   State Inspector        │
│            │                           │   (JSON tree view)       │
├────────────┴───────────────────────────┴─────────────────────────┤
│  Automation Dashboard                                             │
│  ┌─────────────┬──────────────┬──────────────┬─────────────────┐ │
│  │ Pulse       │ Interval 15m │ Next: 10:45  │ [▶ Trigger]     │ │
│  │ Morning Brf │ Cron 8:00    │ Next: tomorrow│ [▶ Trigger]    │ │
│  │ Manual Rev  │ Manual       │ —            │ [▶ Trigger]     │ │
│  └─────────────┴──────────────┴──────────────┴─────────────────┘ │
├──────────────────────────────────────────────────────────────────┤
│  Output Console                                                  │
│  10:30:01 [Pulse] Execution started...                            │
│  10:30:05 [Pulse] HEARTBEAT_OK — suppressed                      │
└──────────────────────────────────────────────────────────────────┘
```

---

### 2.2 Frontend Composables

```typescript
// composables/useWorkspaceEditor.ts
export function useWorkspaceEditor(workspaceId: Ref<string>) {
  const activeTab = ref<string | null>(null)
  const openFiles = shallowRef<Map<string, string>>(new Map())  // path → content
  const modified = shallowRef<Set<string>>(new Set())

  const fileTree = computed(() => {
    // Return file tree structure for workspace
  })

  async function openFile(path: string) {
    const content = await api.workspace.readFile(workspaceId.value, path)
    openFiles.value = new Map(openFiles.value).set(path, content)
    activeTab.value = path
  }

  async function saveFile(path: string, content: string) {
    await api.workspace.writeFile(workspaceId.value, path, content)
    modified.value.delete(path)
  }

  return { fileTree, activeTab, openFiles, modified, openFile, saveFile }
}

// composables/useAutomation.ts
export function useAutomation(workspaceId: Ref<string>) {
  const automations = ref<Automation[]>([])
  const executionStatus = ref<Map<string, ExecutionStatus>>(new Map())
  const ws = useWebSocket()  // For live updates

  async function loadAutomations() {
    const config = await api.workspace.getConfig(workspaceId.value)
    automations.value = config.automations || []
  }

  async function trigger(automationName: string) {
    await api.dispatcher.trigger(workspaceId.value, automationName)
  }

  // WebSocket: listen for execution events
  ws.on('execution_start', ({ automation, timestamp }) => {
    executionStatus.value.set(automation, { status: 'running', timestamp })
  })

  ws.on('execution_complete', ({ automation, output, timestamp }) => {
    executionStatus.value.set(automation, { status: 'complete', output, timestamp })
  })

  ws.on('execution_error', ({ automation, error, timestamp }) => {
    executionStatus.value.set(automation, { status: 'error', error, timestamp })
  })

  return { automations, executionStatus, loadAutomations, trigger }
}

// composables/useWebSocket.ts
export function useWebSocket() {
  const connected = ref(false)
  let ws: WebSocket | null = null

  function connect() {
    ws = new WebSocket('ws://localhost:8080/ws/dispatcher')
    ws.onopen = () => connected.value = true
    ws.onclose = () => { connected.value = false; reconnect() }
  }

  function on(event: string, handler: (data: any) => void) {
    ws?.addEventListener('message', (e) => {
      const msg = JSON.parse(e.data)
      if (msg.type === event) handler(msg.data)
    })
  }

  return { connected, connect, on }
}
```

---

### 2.3 State Inspector (Real-time state.json viewer)

```vue
<!-- components/StateInspector.vue -->
<template>
  <div class="state-inspector">
    <div class="header">
      <span>State Inspector</span>
      <span class="timestamp">Last updated: {{ lastUpdated }}</span>
    </div>
    <div class="json-tree">
      <TreeView :data="parsedState" />
    </div>
    <div class="execution-status" v-if="state.isRunning">
      <span class="pulse" /> Running since {{ state.startedAt }}
    </div>
  </div>
</template>

<script setup lang="ts">
const props = defineProps<{ workspaceId: string }>()
const state = ref<AgentState>({})
const lastUpdated = ref('')
const ws = useWebSocket()

// Live updates via WebSocket
ws.on('state_updated', ({ workspace, newState }) => {
  if (workspace === props.workspaceId) {
    state.value = newState
    lastUpdated.value = new Date().toLocaleTimeString()
  }
})
</script>
```

### 2.4 Post-Mortem Dashboard (Rapid Feedback View)

A dedicated panel for summarizing the last 15 minutes of execution history, enabling immediate iteration on failures and bottlenecks.

```vue
<!-- components/PostMortemDashboard.vue -->
<template>
  <div class="post-mortem">
    <div class="summary-bar">
      <div class="metric">
        <span class="value">{{ metrics.executions }}</span>
        <span class="label">Executions</span>
      </div>
      <div class="metric success">
        <span class="value">{{ metrics.successRate }}%</span>
        <span class="label">Success Rate</span>
      </div>
      <div class="metric">
        <span class="value">{{ metrics.avgLatency }}ms</span>
        <span class="label">Avg Latency</span>
      </div>
      <div class="metric">
        <span class="value">{{ efficiencyRatio }}%</span>
        <span class="label">Efficiency Ratio</span>
      </div>
    </div>

    <div class="timeline">
      <div
        v-for="event in recentEvents"
        :key="event.id"
        :class="['event', event.type]"
      >
        <span class="time">{{ event.timestamp }}</span>
        <span class="automation">{{ event.automation }}</span>
        <span class="status">{{ event.status }}</span>
        <span class="duration" v-if="event.duration">{{ event.duration }}ms</span>
      </div>
    </div>

    <div class="bottlenecks" v-if="bottlenecks.length">
      <h4>Detected Bottlenecks</h4>
      <ul>
        <li v-for="b in bottlenecks" :key="b.id">
          {{ b.description }} — {{ b.impact }}% impact
        </li>
      </ul>
    </div>
  </div>
</template>

<script setup lang="ts">
interface PostMortemEvent {
  id: string
  timestamp: string
  automation: string
  type: 'success' | 'error' | 'skipped'
  status: string
  duration?: number
}

interface Bottleneck {
  id: string
  description: string
  impact: number
}

const props = defineProps<{ workspaceId: string }>()
const recentEvents = ref<PostMortemEvent[]>([])
const bottlenecks = ref<Bottleneck[]>([])

const metrics = computed(() => {
  const total = recentEvents.value.length
  const errors = recentEvents.value.filter(e => e.type === 'error').length
  const durations = recentEvents.value.filter(e => e.duration)
  const avgLatency = durations.length
    ? durations.reduce((sum, e) => sum + (e.duration || 0), 0) / durations.length
    : 0

  return {
    executions: total,
    successRate: total > 0 ? Math.round(((total - errors) / total) * 100) : 100,
    avgLatency: Math.round(avgLatency)
  }
})

const efficiencyRatio = computed(() => {
  // Value-Add time vs Admin (overhead) time
  const valueAdd = metrics.value.executions * metrics.value.avgLatency
  const admin = recentEvents.value.filter(e => e.type === 'skipped').length * 10 // 10ms overhead per skip
  const total = valueAdd + admin
  return total > 0 ? Math.round((valueAdd / total) * 100) : 100
})

// WebSocket subscription for live post-mortem data
const ws = useWebSocket()
ws.on('postmortem_event', (event: PostMortemEvent) => {
  recentEvents.value = [event, ...recentEvents.value].slice(0, 100) // keep last 100
  if (event.type === 'error') {
    analyzeBottleneck(event)
  }
})

function analyzeBottleneck(event: PostMortemEvent) {
  // Simple heuristic: errors within 1 minute of each other = correlated
  const recent = recentEvents.value.filter(e =>
    e.type === 'error' &&
    Math.abs(new Date(e.timestamp).getTime() - new Date(event.timestamp).getTime()) < 60000
  )
  if (recent.length >= 3) {
    bottlenecks.value.push({
      id: crypto.randomUUID(),
      description: `Recurring error in ${event.automation}`,
      impact: Math.min(recent.length * 10, 50)
    })
  }
}
</script>
```

---

## III. API Strategy

### 3.1 REST Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/workspaces` | List all workspaces |
| `GET` | `/api/workspaces/:id` | Get workspace (includes automations) |
| `GET` | `/api/workspaces/:id/config.yaml` | Raw config file |
| `PUT` | `/api/workspaces/:id/config.yaml` | Update automations |
| `POST` | `/api/workspaces/:id/automations/:name/trigger` | Manual trigger |
| `GET` | `/api/workspaces/:id/state.json` | Current state |
| `GET` | `/api/workspaces/:id/files/*` | Read workspace file |
| `PUT` | `/api/workspaces/:id/files/*` | Write workspace file |
| `GET` | `/api/telemetry/efficiency` | Efficiency KPI (value-add vs admin time ratio) |
| `GET` | `/api/telemetry/latency` | Latency histogram for automations |

### 3.2 WebSocket Events

**Connection:** `ws://localhost:8080/ws/dispatcher`

**Server → Client:**

| Event | Payload | Description |
|-------|---------|-------------|
| `execution_start` | `{workspace, automation, timestamp}` | Automation began |
| `execution_complete` | `{workspace, automation, output, timestamp}` | Completed successfully |
| `execution_skipped` | `{workspace, automation, reason, timestamp}` | Lock held, skipped |
| `execution_error` | `{workspace, automation, error, timestamp}` | Failed |
| `state_updated` | `{workspace, state}` | State.json changed |
| `automation_added` | `{workspace, automation}` | New automation registered |
| `automation_removed` | `{workspace, automation}` | Automation unregistered |
| `telemetry_sample` | `{workspace, automation, latency_ms, success}` | Per-execution sample |
| `efficiency_update` | `{workspace, automation, ratio}` | Efficiency ratio recalculated |
| `postmortem_event` | `{id, timestamp, automation, type, status, duration}` | Post-mortem timeline event |

**Client → Server:**

| Event | Payload | Description |
|-------|---------|-------------|
| `subscribe` | `{workspaceIds: string[]}` | Subscribe to workspace updates |
| `trigger` | `{workspace, automation}` | Manual trigger |

---

## IV. Testing Strategy

### 4.1 Unit Tests (Trigger Package)

```go
func TestCronTrigger_ShouldRun(t *testing.T) {
    trig := NewCronTrigger("*/5 * * * *")

    // Last run was 6 minutes ago → should run
    lastRun := time.Now().Add(-6 * time.Minute)
    if !trig.ShouldRun(lastRun) {
        t.Error("expected ShouldRun=true for past schedule")
    }

    // Last run was 2 minutes ago → should not run
    lastRun = time.Now().Add(-2 * time.Minute)
    if trig.ShouldRun(lastRun) {
        t.Error("expected ShouldRun=false for recent run")
    }
}

func TestIntervalTrigger_NextRun(t *testing.T) {
    trig := NewIntervalTrigger(15 * time.Minute)
    lastRun := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

    next := trig.NextRun()
    expected := time.Date(2026, 4, 1, 10, 15, 0, 0, time.UTC)

    if !next.Equal(expected) {
        t.Errorf("NextRun mismatch: got %v, want %v", next, expected)
    }
}
```

### 4.2 Integration Tests with `testing/synctest` (Go 1.24+)

**Note:** Go 1.26.1 is available in this environment. `testing/synctest` requires Go 1.24+ with `GOEXPERIMENT=synctest`. If unavailable, use a mock clock package.

```go
func TestDispatcher_AutomationLifecycle(t *testing.T) {
    // Use synthetic time via t.Context() or mock clock
    clock := synctest.NewT(t)
    dispatcher := NewDispatcher(manager, executor, clock)

    // Register automation
    dispatcher.Register("ws-1", &models.Automation{
        Name:     "Test",
        Trigger:  models.TriggerConfig{Type: "interval", Value: "5m"},
        TaskFile: "test.md",
        Strategy: "isolated",
    })

    // Advance time by 5 minutes
    clock.Advance(5 * time.Minute)

    // Verify execution was triggered
    if executor.Called() != 1 {
        t.Errorf("expected 1 execution after 5m advance, got %d", executor.Called())
    }
}
```

### 4.3 Concurrency Tests

```go
func TestDispatcher_ConcurrentTriggerPrevention(t *testing.T) {
    dispatcher := NewDispatcher(manager, executor, logger)
    dispatcher.Register("ws-1", automationWithInterval("1s"))

    // Launch 10 concurrent triggers
    var wg sync.WaitGroup
    results := make([]error, 10)

    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            _, err := dispatcher.Trigger("ws-1", "Test")
            results = append(results, err)
        }()
    }

    wg.Wait()

    // Count successful vs skipped
    success := 0
    skipped := 0
    for _, err := range results {
        if err == nil {
            success++
        } else if errors.Is(err, ErrLockHeld) {
            skipped++
        }
    }

    if success != 1 {
        t.Errorf("expected exactly 1 success, got %d", success)
    }
    if skipped != 9 {
        t.Errorf("expected 9 skipped, got %d", skipped)
    }
}
```

### 4.4 Load Stress Test (2x Volume)

```go
func TestDispatcher_LoadStress(t *testing.T) {
    // Projected peak: 50 workspaces, 3 automations each, avg interval 5m
    // 2x volume: 100 workspaces, 6 automations each

    dispatcher := NewDispatcher(manager, executor, logger, WithWorkerCount(8))

    // Spawn 100 workspaces with 600 total automations
    for i := 0; i < 100; i++ {
        wsID := fmt.Sprintf("stress-ws-%d", i)
        for j := 0; j < 6; j++ {
            autoName := fmt.Sprintf("auto-%d", j)
            dispatcher.Register(wsID, automationWithInterval("1s"))
        }
    }

    // Let it run for 30 seconds under load
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    dispatcher.Start(ctx)

    // Metrics to verify:
    // - P99 latency < 500ms
    // - Lock retries < 3 per execution
    // - Zero missed triggers (check scheduler entries)
    metrics := dispatcher.Metrics()

    if metrics.P99Latency > 500*time.Millisecond {
        t.Errorf("P99 latency %v exceeds 500ms threshold", metrics.P99Latency)
    }
    if metrics.LockRetries > 3 {
        t.Errorf("lock retries %d exceeds threshold", metrics.LockRetries)
    }
}
```

---

## V. Migration Path (Current → Unified)

### Phase 1: Introduce Packages (~3 weeks)
- Create `internal/dispatcher/`, `internal/trigger/`, `internal/persistence/`
- Move/reimplement trigger logic from `scheduler.go`
- **Existing `scheduler.go` becomes thin wrapper** → delegates to Dispatcher
- **+15% buffer**: Handle flock edge cases during concurrent access testing

### Phase 2: Data Model Migration (~2 weeks)
- Extend `config.yaml` with `automations[]` array
- Write migration: existing `cron_schedule` → single automation with that cron
- **Integrated 15% time-buffer** for flock edge case remediation
- Validate all existing workspaces migrate without data loss

### Phase 3: Frontend Agent IDE (~3 weeks)
- Add new `WorkspacesView.vue` with three-pane layout
- Implement `useWorkspaceEditor` and `useAutomation` composables
- Build Post-Mortem Dashboard for rapid feedback
- Integrate WebSocket for live execution and telemetry updates

### Phase 4: Deprecate and Stabilize (~2 weeks)
- Remove `Scheduler` type, replace with `Dispatcher`
- Update `bootstrap.go` wiring
- Remove old scheduler routes
- Monitor efficiency KPIs until stable (target: >90% efficiency ratio)
- Run Load Stress Test at 2x projected volume

---

## VI. Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| **Registry over 1:1 cron mapping** | Enables N:M (multiple automations per workspace, multiple triggers per automation) |
| **Trigger interface is pure** | No side effects; `ShouldRun` is a pure predicate, enabling easy unit testing |
| **flock on every operation** | Simplicity over optimization; prevents subtle race conditions |
| **Strategy pattern for execution** | Isolated vs Persistent is a first-class concept; swapping is a one-liner in config |
| **WebSocket for live updates** | Real-time feedback is critical for agent trust; polling is insufficient |
| **Monaco Editor for file editing** | Professional-grade editing experience; supports large files via virtual rendering |
| **shallowRef for editor content** | Prevents deep reactivity on potentially large file contents; only track reference changes |
| **telemetry/ self-contained** | Records metrics from dispatcher; no reverse dependencies; can be queried independently |
| **Exponential backoff + jitter on lock** | Prevents thundering herd on high-volume concurrent trigger scenarios |
| **15-minute sliding window for KPIs** | Balances responsiveness with noise reduction in efficiency ratio calculations |
| **Post-mortem auto-correlation** | Groups errors within 1-minute windows to identify systemic bottlenecks without manual triage |
