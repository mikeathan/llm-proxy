# Plan: Recording Playback for Automations

## Summary

Allow users to select a previously recorded automation run (JSONL) and replay it instead of calling a live LLM. The recording/playback lives in its own standalone Go package with zero dependency on proxy/LLM code.

## Bug Fix First

**Problem:** Running the same automation twice with `--record-dir` appends the second run's data to the first run's file instead of creating a new file.

**Root cause:**
1. `RuntimeClientProvider.ensureClient()` caches a single `proxy.Client` per model (provider.go:92)
2. The bootstrap factory wraps it once with `recorder.New()` — same `RecordingClient` reused across runs
3. `RecordingClient.ensureFile()` checks `if rc.file != nil { return nil }` (recorder.go:40)
4. Second run skips file creation and appends to the first run's file

**Fix — 2 files:**

`internal/core/automation/executor.go` — inject execution identity into context:
```go
execCtx := models.WithTaskName(ctx, req.AutomationName)
execCtx = models.WithRunID(execCtx, generateRunID())
```

`internal/core/proxy/recorder/recorder.go` — detect new session, close old file:
- Add `currentRunID string` field to `RecordingClient`
- Add `closeFile()` method (flush + close + nil-out writer/encoder/file)
- In `ensureFile()`: if `models.GetRunID(ctx)` differs from `currentRunID`, call `closeFile()` first

Result: every `Execute()` produces a separate file: `{recordDir}/{model}/{automation}/{timestamp}_run_12345.jsonl`

---

## Package Architecture

```
backend/
  internal/recordings/               ← NEW: standalone, stdlib only
    types.go                         ← RecordingMeta, RecordingTurn
    store.go                         ← RecordingStore: scan, list, get, delete, refresh
    playback.go                      ← PlaybackClient: parse JSONL, replay turns
    store_test.go
    playback_test.go

  internal/core/proxy/recorder/       ← EXISTING: writes JSONL (bug fix only)
    recorder.go                       ← Fix: per-execution file via run ID

  internal/app/                       ← EXISTING: wiring
    playback_bridge.go               ← NEW: wraps PlaybackClient → proxy.Client (sole bridge)
```

### Dependency Graph

```
internal/recordings/        → stdlib only (no project imports)

internal/app/playback_bridge.go → imports:
  - internal/recordings/
  - internal/core/proxy/      ← ONLY file that bridges the two

internal/core/proxy/recorder/  → imports: internal/core/proxy, models (unchanged)
```

---

## Changes (sorted by file)

### Backend

| # | File | Change |
|---|------|--------|
| 1 | `models/workspace.go` | Add `RecordingRef string` to `Automation` struct |
| 2 | `internal/core/proxy/recorder/recorder.go` | Add `closeFile()`, `currentRunID` field; check run ID delta in `ensureFile()` |
| 3 | `internal/recordings/types.go` | NEW — `RecordingMeta`, `RecordingTurn` |
| 4 | `internal/recordings/store.go` | NEW — `RecordingStore` |
| 5 | `internal/recordings/playback.go` | NEW — `PlaybackClient` |
| 6 | `internal/recordings/store_test.go` | NEW |
| 7 | `internal/recordings/playback_test.go` | NEW |
| 8 | `internal/app/playback_bridge.go` | NEW — adapts `*PlaybackClient` to `proxy.Client` |
| 9 | `internal/core/automation/registry.go` | Add `RecordingRef` to `AutomationEntry` |
| 10 | `internal/core/automation/executor.go` | Add `RecordingRef` to `ExecuteRequest`; inject `WithRunID`; use playback when ref set |
| 11 | `internal/core/automation/dispatcher.go` | Pass `RecordingRef` through to executor |
| 12 | `internal/transport/http/dispatcher_handlers.go` | Add `recording_ref` to `AutomationInfo` |
| 13 | `internal/transport/http/recordings_handlers.go` | NEW — REST endpoints for recordings |
| 14 | `internal/app/bootstrap.go` | Init `RecordingStore` when `--record-dir` set; wire into services; mount routes |

### Frontend

| # | File | Change |
|---|------|--------|
| 15 | `frontend/src/types/dispatcher.ts` | Add `recording_ref` to `Automation` type |
| 16 | `frontend/src/services/dispatcherService.ts` | Add recording API methods |
| 17 | `frontend/src/composables/useRecordings.ts` | NEW — recording data composable |
| 18 | `frontend/src/components/AgentIde/recordings/RecordingsPanel.vue` | NEW — browse/select recordings |
| 19 | `frontend/src/components/AgentIde/automation/AutomationDetails.vue` | Show "Using recording:" badge |
| 20 | `frontend/src/components/AgentIde/AgentIde.vue` | Add "Recordings" tab to left sidebar |

---

## API Endpoints

```
GET  /admin/api/recordings              → [RecordingMeta]
GET  /admin/api/recordings/status       → {enabled: bool, dir: string}
GET  /admin/api/recordings/{id}         → RecordingMeta
DELETE /admin/api/recordings/{id}       → {status: "deleted"}
```

Recording handlers live in their own file (`recordings_handlers.go`), not mixed with dispatcher or admin handlers.

---

## types.go Schema

```go
type RecordingMeta struct {
    ID             string    // relative path, e.g. "gemma4/daily-sync/20260524T120000Z_run_12345.jsonl"
    Model          string
    AutomationName string
    Timestamp      time.Time
    FilePath       string    // absolute path on disk
    FileSize       int64
    SessionID      string
}

type RecordingTurn struct {
    Request struct {
        Messages json.RawMessage
        Tools    json.RawMessage
    }
    Response struct {
        Choices json.RawMessage
    }
    Chunks []json.RawMessage
    Error  string
}
```

---

## store.go (RecordingStore)

```go
func NewRecordingStore(recordDir string) (*RecordingStore, error)
func (s *RecordingStore) List() []RecordingMeta
func (s *RecordingStore) ListByAutomation(name string) []RecordingMeta
func (s *RecordingStore) Get(id string) (*RecordingMeta, bool)
func (s *RecordingStore) Delete(id string) error
func (s *RecordingStore) Refresh() error
```

Scans `{recordDir}/**/*.jsonl` recursively. Groups by `{model}/{automationName}/` directory structure. Parses timestamp and session ID from filenames.

---

## playback.go (PlaybackClient)

```go
func NewPlaybackClient(path string) (*PlaybackClient, error)
func (p *PlaybackClient) NextTurn() *RecordingTurn
func (p *PlaybackClient) Reset()
```

Parses a JSONL file into ordered `RecordingTurn` entries. Each `request`/`response`/`chunk` sequence = one turn. Exhausted returns nil.

---

## playback_bridge.go

Wraps `*PlaybackClient` to satisfy `proxy.Client`:

```go
type PlaybackBridge struct {
    client *PlaybackClient
}

func (b *PlaybackBridge) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error)
func (b *PlaybackBridge) Stream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error)
```

- `Chat()` calls `NextTurn()`, synthesizes `*proxy.ChatResponse` from recorded response/chunks
- `Stream()` same, returns channel of recorded chunks
- When turns exhausted, returns error

---

## Executor Integration

In `LLMTaskExecutor.Execute()`:

```go
// After getting LLM client (line ~110):
if req.RecordingRef != "" && e.svc.RecordingStore() != nil {
    if meta, ok := e.svc.RecordingStore().Get(req.RecordingRef); ok {
        if pc, err := recordings.NewPlaybackClient(meta.FilePath); err == nil {
            client = app.NewPlaybackBridge(pc)
            procLog.Info("Running from recording", "ref", req.RecordingRef)
        }
    }
}

// Inject run ID for recording file isolation:
execCtx := models.WithTaskName(ctx, req.AutomationName)
execCtx = models.WithRunID(execCtx, generateRunID())
```

`LLMServiceProvider` interface gets a new method:
```go
RecordingStore() *recordings.RecordingStore
```

Returns nil when `--record-dir` is not set.

---

## Bootstrap Wiring

```go
// In Container struct / BuildAppServices():
var recordingStore *recordings.RecordingStore
if c.RecordDir != "" {
    rs, err := recordings.NewRecordingStore(c.RecordDir)
    if err == nil {
        recordingStore = rs
    }
}

// Pass to AppServices:
s.recordingStore = recordingStore

// Mount routes:
recordingsHandlers := api.NewRecordingHandlers(recordingStore)
router.Get("/admin/api/recordings", recordingsHandlers.List)
router.Get("/admin/api/recordings/status", recordingsHandlers.Status)
router.Get("/admin/api/recordings/{id}", recordingsHandlers.Get)
router.Delete("/admin/api/recordings/{id}", recordingsHandlers.Delete)
```

---

## What Does NOT Change

| File | Reason |
|------|--------|
| `internal/core/assistant/agent.go` | Still consumes `proxy.Client`; no recording awareness |
| `internal/core/proxy/client.go` | `LLMClient` unchanged |
| `internal/core/proxy/provider.go` | `RuntimeClientProvider` unchanged |
| `internal/testing/llmprofiles/profiles.go` | Test-only FixtureClient stays as-is |
| `frontend/src/App.vue` | Tab structure unchanged |
| SSE console, history, pulse | All consumer-side; no recording awareness needed |
