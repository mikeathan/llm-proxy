# Plan: Per-Run Output Directories

**Status:** Draft  
**Date:** 2026-06-05  
**Problem:** After each automation run, collecting diagnostic information requires manual work: copy agent trace from the UI, copy server logs from the terminal, and find the recording path. The data is split across `state.json` (bloated with pruned event data), the recording JSONL (in a flat directory), and server stdout. No single file contains the complete run picture.

**Root cause:** Three design gaps:

1. `state.json` stores all 7,746+ events per run in `AutomationRun.Events`, pruned to 1KB each, accumulating 30 runs — massive bloat for data that's truncated anyway.
2. The recording and the agent events are stored independently with no cross-reference (RecordingRef is never populated for live runs).
3. Agent events are only available in memory during the run and in truncated form in state.json — no persistent, un-truncated event log survives.

---

## Design

Replace flat recording files with per-run directories containing all run artifacts. Strip full events from `state.json` — they belong in the run folder, not in the workspace metadata store.

### Directory Structure

```
{record-dir}/
  gemma-4-4b-it-Q4_K_M.gguf/
    smoke-test/
      20260605T130314Z_bf0e9f8a/          ← single run folder (timestamp_sessionID)
        recording.jsonl                    ← LLM request/response stream (already exists, moved here)
        events.jsonl                       ← AgentEvents, written live, never pruned
        run-meta.json                      ← Lightweight summary (duration, model, steps, error, recording path)
        final-report.md                    ← The agent's submit_final_answer output
```

### What changes

| File | Current behavior | New behavior |
|------|-----------------|--------------|
| `recording.jsonl` | Flat file at `{record-dir}/{model}/{task}/{timestamp}_{sessionID}.jsonl` | Inside run folder: `{run-dir}/recording.jsonl` |
| Agent events | Held in `capturedEvents []any`, written to `state.json` pruned to 1KB | Written live to `{run-dir}/events.jsonl` (append-only, no truncation) + kept in memory for backward compat |
| `state.json` | Stores full `Events []any` per run (pruned), capped at 30 runs | Stores no events — only lightweight metadata. `AutomationRun.Events` set to nil for live runs. |
| `final-report.md` | Only in `state.json last_runs[name].Output` | Written to `{run-dir}/final-report.md` + still in state.json |
| Process logs | Written to `{metadata}/{workspace}/process.log` | Could also be run-specific in the future (phase 2) |

---

## Files to Create

### 1. `internal/core/automation/rundir.go` — Run directory manager

New type `RunDir` that owns the lifecycle of a per-run directory. Responsibilities:

- Create `{record-dir}/{model}/{task}/{timestamp}_{sessionID}/` at construction time
- Provide paths for sub-files: `RecordingPath()`, `EventsPath()`, `MetaPath()`, `ReportPath()`
- Write `final-report.md` when the agent completes
- Write `run-meta.json` at the end of the run

```go
package automation

import (
    "crypto/rand"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "time"
)

// RunDir owns a single automation run's output directory.
// Created before agent execution, written to during and after the run.
type RunDir struct {
    Root    string // Absolute path to the run folder, e.g. ".../recordings/gemma/20260605T130314Z_bf0e9f8a/"
    model   string
    task    string
}

// RunMeta is written to run-meta.json at the end of the run.
type RunMeta struct {
    Model          string `json:"model"`
    Task           string `json:"task"`
    DurationMs     int64  `json:"duration_ms"`
    StepCount      int    `json:"step_count,omitempty"`
    LLMCalls       int    `json:"llm_calls,omitempty"`
    ToolCalls      int    `json:"tool_calls,omitempty"`
    Error          string `json:"error,omitempty"`
    FinalReportLen int    `json:"final_report_len,omitempty"`
    RecordingPath  string `json:"recording_path"`  // Relative to record-dir root
}

func NewRunDir(recordDir, model, task string) (*RunDir, error) {
    sessionID := generateSessionID()
    timestamp := time.Now().UTC().Format("20060102T150405Z")
    dirName := fmt.Sprintf("%s_%s", timestamp, sessionID)
    root := filepath.Join(recordDir, model, task, dirName)
    if err := os.MkdirAll(root, 0755); err != nil {
        return nil, fmt.Errorf("create run dir %s: %w", root, err)
    }
    return &RunDir{Root: root, model: model, task: task}, nil
}

func (r *RunDir) RecordingPath() string  { return filepath.Join(r.Root, "recording.jsonl") }
func (r *RunDir) EventsPath() string     { return filepath.Join(r.Root, "events.jsonl") }
func (r *RunDir) MetaPath() string       { return filepath.Join(r.Root, "run-meta.json") }
func (r *RunDir) ReportPath() string     { return filepath.Join(r.Root, "final-report.md") }
func (r *RunDir) RecordingRelPath(recordDir string) string {
    rel, _ := filepath.Rel(recordDir, r.RecordingPath())
    return rel
}

func (r *RunDir) WriteFinalReport(content string) error {
    return os.WriteFile(r.ReportPath(), []byte(content), 0644)
}

func (r *RunDir) WriteMeta(meta RunMeta) error {
    data, err := json.MarshalIndent(meta, "", "  ")
    if err != nil {
        return fmt.Errorf("marshal run-meta: %w", err)
    }
    return os.WriteFile(r.MetaPath(), data, 0644)
}

func generateSessionID() string {
    b := make([]byte, 8)
    if _, err := rand.Read(b); err != nil {
        return fmt.Sprintf("%x", time.Now().UnixNano())
    }
    return hex.EncodeToString(b)
}
```

**Key design decisions:**
- `RunDir` is a simple struct, not an interface — no need for polymorphism here
- It needs `recordDir` to compute `RecordingRelPath()` for the runtime reference
- Timestamp+sessionID format matches existing RecordingClient pattern
- `WriteFinalReport` and `WriteMeta` are separate because they happen at different times (report during agent execution, meta at the very end)

### 2. `internal/core/automation/eventsink.go` — Event sink for the run

New type `EventSink` that lives alongside `RunDir`. Its single job is writing AgentEvents as JSONL.

```go
package automation

import (
    "bufio"
    "encoding/json"
    "os"
    "sync"
    
    "llm-proxy/internal/core/assistant"
)

// EventSink writes AgentEvents to a JSONL file as they fire.
// Thread-safe, no buffering delay (flush on every write).
type EventSink struct {
    mu     sync.Mutex
    writer *bufio.Writer
    file   *os.File
    encoder *json.Encoder
}

func NewEventSink(path string) (*EventSink, error) {
    f, err := os.Create(path)
    if err != nil {
        return nil, fmt.Errorf("create events file %s: %w", path, err)
    }
    w := bufio.NewWriterSize(f, 65536)
    return &EventSink{
        file:    f,
        writer:  w,
        encoder: json.NewEncoder(w),
    }, nil
}

func (s *EventSink) Write(ev assistant.AgentEvent) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    if err := s.encoder.Encode(ev); err != nil {
        return fmt.Errorf("encode event: %w", err)
    }
    return s.writer.Flush() // flush immediately so file is always up-to-date
}

func (s *EventSink) Close() {
    s.mu.Lock()
    defer s.mu.Unlock()
    if s.writer != nil {
        s.writer.Flush()
        s.writer = nil
    }
    if s.file != nil {
        s.file.Close()
        s.file = nil
    }
}
```

**Key design decisions:**
- Flushes on every write so the file is always current if the process crashes or the user inspects mid-run
- 64KB buffer prevents tiny syscalls for each JSON line
- `Write()` is the only public method — simple interface for the Observer callback

---

## Files to Modify

### 3. `internal/core/proxy/recorder/recorder.go` — RecordingClient uses RunDir

**Change**: Add an optional `recordDirPath` field. When set, the recorder writes `recording.jsonl` inside that directory instead of creating its own flat file path.

```go
type RecordingClient struct {
    underlying    proxy.Client
    recordDir     string
    modelName     string
    mu            sync.Mutex
    file          *os.File
    writer        *bufio.Writer
    encoder       *json.Encoder
    currentRunID  string
    // NEW: optional directory override
    currentDir    string // if set, write recording.jsonl inside this dir
}

// NEW: SetDir overrides the recording file location to write inside a run directory.
// Must be called before the first Chat/Stream call. Not thread-safe with ensureFile.
func (rc *RecordingClient) SetDir(dir string) {
    rc.currentDir = dir
}

// MODIFIED ensureFile: use currentDir if set
func (rc *RecordingClient) ensureFile(ctx context.Context) error {
    rc.mu.Lock()
    defer rc.mu.Unlock()
    // ... existing runID switching logic ...

    if rc.file != nil {
        return nil
    }

    if rc.currentDir != "" {
        // Write inside the managed run directory
        filePath := filepath.Join(rc.currentDir, "recording.jsonl")
        f, err := os.Create(filePath)
        // ... rest same as before ...
        return nil
    }

    // Original flat-file path for backward compat (chat, non-automation)
    sessionID := generateSessionID()
    timestamp := time.Now().UTC().Format("20060102T150405Z")
    taskName := models.GetTaskName(ctx)
    dir := filepath.Join(rc.recordDir, rc.modelName, taskName)
    // ... rest existing code ...
}
```

**Constitution compliance**: No violations. This is backward-compatible — when `currentDir` is empty, behavior is identical to today.

### 4. `internal/core/automation/executor.go` — Wire RunDir + EventSink into execution

**Changes in `Execute()`:**

a) **Before agent creation** (after getting client, before building agentOpts): Create `RunDir` and `EventSink` when recording is active.

b) **In the Observer callback**: Write each event to the EventSink as it fires.

c) **After execution**: Write `run-meta.json` and `final-report.md`. Set the recording reference so state.json links to it.

d) **Cleanup**: Close EventSink on both success and error paths.

```go
func (e *LLMTaskExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
    startTime := time.Now()
    // ... existing setup ...

    // --- NEW: Create run directory and event sink ---
    var runDir *RunDir
    var eventSink *EventSink
    recordDir := e.svc.RecordDir() // NEW method on LLMServiceProvider
    if recordDir != "" && req.Model != "" {
        var rErr error
        runDir, rErr = NewRunDir(recordDir, req.Model, req.AutomationName)
        if rErr != nil {
            procLog.Warn("failed to create run dir, continuing without per-run output", "error", rErr)
        } else {
            es, esErr := NewEventSink(runDir.EventsPath())
            if esErr != nil {
                procLog.Warn("failed to create event sink, continuing without", "error", esErr)
            } else {
                eventSink = es
                // Update the RecordingClient to write inside our run directory
                if rcl, ok := client.(interface{ SetDir(string) }); ok {
                    rcl.SetDir(runDir.Root)
                }
            }
        }
    }

    var capturedEvents []any
    agentOpts := assistant.AgentOptions{
        // ... existing fields ...
        Observer: func(ev assistant.AgentEvent) {
            capturedEvents = append(capturedEvents, ev)        // keep in memory
            e.svc.Events().Publish(req.WorkspaceID, ev)        // live SSE
            if eventSink != nil {
                eventSink.Write(ev)                            // persist un-truncated
            }
        },
        // ...
    }
    // ... rest of agent creation ...

    // --- MODIFIED execution + cleanup ---
    // Defer event sink close
    if eventSink != nil {
        defer eventSink.Close()
    }

    finalReply, fullHistory, agErr := agent.Execute(execCtx, history)
    // ... usage tracking ...
    
    if agErr != nil {
        errStr := fmt.Sprintf("agent execution failed: %v", agErr)
        if runDir != nil {
            meta := RunMeta{
                Model:      req.Model,
                Task:       req.AutomationName,
                DurationMs: time.Since(startTime).Milliseconds(),
                Error:      errStr,
            }
            if t := assistant.GetUsageTracker(execCtx); t != nil {
                meta.LLMCalls = t.LLMCalls
                meta.ToolCalls = t.ToolCalls
            }
            meta.RecordingPath = runDir.RecordingRelPath(recordDir)
            runDir.WriteMeta(meta)
        }
        resp.State.LastError = errStr
        resp.State.SetRunning("")
        e.recordRun(req, resp.State, "", errStr, time.Since(startTime), capturedEvents)
        return resp, fmt.Errorf("agent execution failed: %w", agErr)
    }

    // ... existing output formatting ...

    if runDir != nil {
        runDir.WriteFinalReport(output)
        meta := RunMeta{
            Model:          req.Model,
            Task:           req.AutomationName,
            DurationMs:     time.Since(startTime).Milliseconds(),
            FinalReportLen: len(output),
            RecordingPath:  runDir.RecordingRelPath(recordDir),
        }
        if t := assistant.GetUsageTracker(execCtx); t != nil {
            meta.LLMCalls = t.LLMCalls
            meta.ToolCalls = t.ToolCalls
        }
        runDir.WriteMeta(meta)
    }

    e.recordRun(req, resp.State, runResult, runError, time.Since(startTime), capturedEvents)
    return resp, nil
}
```

### 5. `internal/core/automation/executor.go` — Strip events from recordRun

**Change `recordRun`** to set `Events: nil` for live runs (events live in the run folder).
For playback runs (RecordingRef != ""), keep events since there's no run folder.

```go
func (e *LLMTaskExecutor) recordRun(req ExecuteRequest, state *models.AgentState, output, errStr string, duration time.Duration, events []any) {
    if state == nil {
        return
    }

    run := models.AutomationRun{
        ID:             generateRunID(),
        WorkspaceID:    req.WorkspaceID,
        AutomationName: req.AutomationName,
        Timestamp:      time.Now(),
        Output:         output,
        Error:          errStr,
        DurationMs:     duration.Milliseconds(),
        Model:          req.Model,
        RecordingRef:   req.RecordingRef,
        Events:         nil, // CHANGED: events live in run dir, not in state.json
    }
    // ... rest same ...
}
```

This removes the 7,746-event bloat from state.json. The frontend "Live Console" replay can be served from events.jsonl via a new endpoint in a follow-up.

### 6. `internal/core/automation/executor.go` — Add RecordDir to LLMServiceProvider

**Change the interface** and its implementation in app_context.go or wherever it's wired:

```go
type LLMServiceProvider interface {
    // ... existing methods ...
    RecordDir() string // NEW: returns the recording directory (empty = no recording)
}
```

### 7. `internal/transport/http/dispatcher_handlers.go` — New endpoint (future / optional)

Add `GET /admin/api/dispatcher/workspaces/{ws}/automations/{name}/runs/latest/events` that streams `events.jsonl` from the latest run folder. This lets the frontend reconstruct the "Live Console" view without state.json bloat.

Alternatively, the frontend can read `state.json → last_runs[name]` for metadata and load events from the recording store's file system. This is a separate follow-up task.

### 8. `frontend/` (future / optional)

Update the "Live Console" view to:
1. Read `state.json → last_runs[name]` for run summary (output, error, duration, model)
2. If `RecordingRef` points to a local recording, fetch `events.jsonl` from the recording store
3. Render events the same way as today

---

## Constitution Compliance Check

| Rule | Assessment |
|------|-----------|
| **II.6 — Structural Sieve** | Unaffected — sieve logic unchanged |
| **II.11 — Per-Model Config Flow** | Unaffected — no config changes |
| **II.14 — Execution State Tracking** | Unaffected — PlanState unchanged |
| **III.2 — Atomic Persistence** | `WriteMeta`/`WriteFinalReport` use `os.WriteFile` (atomic on same-fs rename). EventSink writes are unsafe on crash but events are append-only and replay is best-effort. For stronger guarantees, write to temp file + rename. |
| **IV.2 — Error Integrity** | All new errors use `fmt.Errorf` with `%w` |
| **IV.4 — No Dead Code** | `pruneEvents` can be removed entirely since events no longer go to state.json |
| **V.3 — No Half-Finished** | This plan is complete end-to-end, but frontend is a follow-up |

---

## Verification

### Tests
```bash
go test ./internal/core/automation/... -v
go test ./internal/core/proxy/recorder/... -v
go test ./internal/core/assistant/... -v
go test ./... -count=1
```

### Manual check
1. Start server with `--record-dir=testdata/recordings`
2. Run an automation
3. Verify `{record-dir}/{model}/{task}/{timestamp}_{sessionID}/` contains:
   - `recording.jsonl` — has LLM request/response lines
   - `events.jsonl` — has AgentEvent lines
   - `run-meta.json` — has duration, model, recording path
   - `final-report.md` — has the agent's final output
4. Verify `state.json` → `last_runs[name]` has no `events` array (or empty)
5. Run without `--record-dir` — verify no errors, everything works as before

---

## Future Work (not in scope)

- **Phase 2 — Process log per run**: Route workspace process log entries (`procLog`) into a `run.log` file inside the run directory, so the complete picture is in one folder.
- **Phase 2 — Frontend Live Console**: Update the UI to read `events.jsonl` from the recording store instead of `state.json.events`.
- **Phase 3 — Build automated diagnostic tool**: A CLI script that takes a run directory path and produces the final `paste_1.txt`-style markdown report combining events, recording, and final answer.
