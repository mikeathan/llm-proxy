---
status: complete
---
# Process Lifecycle Management — Orphan Cleanup & Admin Process View

**Status: COMPLETE ✅**
**Last updated: 2026-05-29**

---

## Executive Summary

When the LLM proxy is killed with SIGKILL (or crashes), child llama-server processes become orphans and continue holding GPU VRAM. The existing shutdown chain (`signalStopLocked` → SIGTERM → wait → SIGKILL to process group) handles graceful shutdowns but never runs on SIGKILL.

This plan:
1. Reverts the ad-hoc PID-file + port-cleanup code added in this session
2. Implements proper kernel-level orphan protection via `Pdeathsig` on Linux (already done)
3. Adds an admin process view to list and manually kill orphan processes
4. Places the process view in the admin UI under an "Infrastructure" section, not in the main model/agent workflow

---

## Critical Rules (Read Before Any Step)

1. **Follow existing code patterns** — Handlers go in `internal/transport/http/`, types in `models/`, frontend services in `frontend/src/services/`, composables in `frontend/src/composables/`.
2. **No raw `os/exec` in agent tool code** — The admin process endpoint is infrastructure, not agent tools. Using `os/exec` there is acceptable.
3. **No `context.Background()` for long-lived processes** — Per Constitution II.2.
4. **After each step:** `go build ./... && go test ./...` — must pass.
5. **UI placement:** The process view goes under a new "Infrastructure" tab section, not in Dashboard/Settings/Logs/Agent IDE.
6. **No new dependencies** — Use only stdlib and existing project utilities.

---

## Progress Tracker

| Step | Description | Status |
|------|-------------|--------|
| 0 | Revert ad-hoc cleanup code from `local_provider.go` | [x] |
| 1 | Linux `Pdeathsig` — cross-platform helper (`sysprocattr_*`) | [x] |
| 2 | Backend: process listing utility `internal/platform/process/` | [x] |
| 3 | Backend: process kill handler `process_handlers.go` | [x] |
| 4 | Backend: register routes in `services.go` / router | [x] |
| 5 | Frontend: add `process` types in `types/` | [x] |
| 6 | Frontend: add API methods in `adminService.ts` | [x] |
| 7 | Frontend: create `InfrastructurePanel.vue` component | [x] |
| 8 | Frontend: add "Infrastructure" tab in `App.vue` + `AdminHeader.vue` | [x] |
| 9 | Frontend: build + verify | [x] |
| 10 | Full test suite | [x] |

---

## Step 0: Refactor Ad-Hoc Startup Cleanup Code

Simplify the port-cleanup and startup logic in `internal/core/llm/providers/local_provider.go`:

- **Revert PID-file tracking**: Remove the ad-hoc PID file creation and removal in `StartModel()` and `Shutdown()`.
- **Retain Port-Level Safety Check**: Retain a simplified check at `StartModel()` startup to see if the target port is in use. If it is occupied by an orphaned `llama-server`, attempt to kill that specific process before starting the new instance. This prevents `"address already in use"` launch failures when swapping models.

---

## Step 1: Linux `Pdeathsig` & macOS SIGKILL Findings

Keep the platform-specific sysprocattr helper files:
- `sysprocattr_linux.go` (`//go:build linux`) — sets `Pdeathsig: syscall.SIGTERM`
- `sysprocattr_other.go` (`//go:build !linux`) — no-op on macOS

### findings on Catching SIGKILL / Crashes:
1. **SIGKILL (Signal 9)** is handled directly by the OS kernel and cannot be caught, blocked, or ignored by any user-space process (including Go or C binaries). No code executes in the process when SIGKILL is delivered.
2. **Crashes** (such as Go panics or SIGSEGV) can be caught using `recover()` and Go's `os/signal` package (`signal.Notify`), but these hooks do not guarantee execution during sudden power failures or SIGKILLs.
3. **macOS Parent-Death Signaling**: macOS/XNU does not support `Pdeathsig` or any direct kernel parent-death trigger. The only fully automated ways are modifying the child to watch the parent via `kqueue`/stdin pipe EOF (impractical without wrappers).
4. **Conclusion**: The most robust, dependency-free cross-platform approach is:
   - **Linux**: Native `Pdeathsig` handles parent SIGKILL instantly.
   - **macOS**: Startup-time port safety checks in `StartModel()` to self-heal and release ports occupied by orphans, combined with the manual Admin UI process panel.

---

## Step 2: Backend — Process Listing Utility

### New file: `internal/platform/process/process.go`

Cross-platform process scanner. Lists running processes whose binary path matches a given name fragment.

```go
package process

import "time"

type Info struct {
	PID     int       `json:"pid"`
	Binary  string    `json:"binary"`
	Args    []string  `json:"args"`
	Model   string    `json:"model"`    // extracted from args (clean basename of the GGUF)
	Port    int       `json:"port"`     // extracted from args (the --port argument)
	Started time.Time `json:"started"`
	Uptime  string    `json:"uptime"`
	Active  bool      `json:"active"`   // true if this process is actively managed by LLMProxy
}

// ListByBinary returns all running processes matching the given binary name.
func ListByBinary(binaryName string, activePID int) ([]Info, error) { ... }

// Kill sends SIGTERM, waits 5s, then SIGKILL if still alive.
func Kill(pid int) error { ... }
```

**Linux path** (`//go:build linux`):
- Read `/proc/*/cmdline` for PIDs.
- Check if `strings.Contains(filepath.Base(args[0]), binaryName)` matches.
- Query `/proc/<pid>` directory creation/mod time as the start time.

**macOS path** (`//go:build !linux`):
- Run `ps -eo pid,lstart,command` to ensure fields are in a predictable order.
- Skip header. For subsequent lines, split by whitespace.
- Extract `PID` (`fields[0]`).
- Extract the 5 started time fields (`fields[1:6]`), join them with space, and parse using `time.ParseInLocation(time.ANSIC, dateStr, time.Local)`.
- Reconstruct the command string from `fields[6:]` and verify if it contains the target binary name.
- Parse `-m` and `--port` arguments. Use `filepath.Base` on the GGUF path to format `Model`.

---

## Step 3: Backend — Process Handlers

Update `internal/transport/http/process_handlers.go`:

1. Expose the actively running model's PID:
   Add `PID int` to [llm.ActiveModelInfo](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/llm/manager.go#L51-L59) and populate it during runtime.
2. In `AdminProcessesHandler`:
   Get the active model via `h.runtime.ActiveInfo()`. Extract `activePID`. Call `process.ListByBinary("llama-server", activePID)`.
3. In `AdminProcessKillHandler`:
   Parse the target PID. If `activePID == targetPID`, call `h.runtime.StopActive()` to cleanly shut down and update proxy state. Otherwise, call `process.Kill(pid)`.

**Routes in buildRouter**:
```go
router.Get("/admin/api/runtime/processes", admin.AdminProcessesHandler, jsonMethodNotAllowed)
router.Post("/admin/api/runtime/processes/{pid}/stop", admin.AdminProcessKillHandler, jsonMethodNotAllowed)
```

---

## Step 4: Frontend — Add Types

### In `frontend/src/types/admin.ts`:

```typescript
export interface ProcessInfo {
  pid: number
  binary: string
  model?: string
  port?: number
  started: string
  uptime: string
  active: boolean
}

export interface ProcessListResponse {
  processes: ProcessInfo[]
}

export interface ProcessKillResponse {
  status: string
  pid: number
}
```

---

## Step 5: Frontend — Add API Methods

### In `frontend/src/constants/api.ts`:
```typescript
export const API_ENDPOINTS = {
  // ... existing endpoints ...
  processes: `${API_BASE}/runtime/processes`,
}
```

### In `frontend/src/services/adminService.ts`:
```typescript
export const AdminApiService = {
  // ... existing methods ...

  fetchProcesses: (): Promise<ProcessListResponse> =>
    get<ProcessListResponse>(API_ENDPOINTS.processes),

  stopProcess: (pid: number): Promise<ProcessKillResponse> =>
    post<ProcessKillResponse>(`${API_ENDPOINTS.processes}/${pid}/stop`),
}
```

---

## Step 6: Frontend — Create useProcesses Composable

Create `frontend/src/composables/useProcesses.ts` using the `mountCount` polling pattern:

```typescript
import { ref, onMounted, onUnmounted } from 'vue'
import { AdminApiService } from '../services/adminService'
import type { ProcessInfo } from '../types/admin'

const processes = ref<ProcessInfo[]>([])
let pollInterval: ReturnType<typeof setInterval> | null = null
let mountCount = 0

const refresh = async () => {
  try {
    const res = await AdminApiService.fetchProcesses()
    processes.value = res.processes
  } catch (e: any) {
    console.error('Failed to fetch processes:', e.message)
  }
}

export function useProcesses() {
  onMounted(() => {
    mountCount++
    if (mountCount === 1) {
      refresh()
      pollInterval = setInterval(refresh, 10000)
    }
  })

  onUnmounted(() => {
    mountCount--
    if (mountCount === 0 && pollInterval) {
      clearInterval(pollInterval)
      pollInterval = null
    }
  })

  return { processes, refresh }
}
```

---

## Step 7: Frontend — Infrastructure Panel

### New component: `frontend/src/components/infrastructure/InfrastructurePanel.vue`
- Renders table with Columns: Status, PID, Model, Port, Uptime, Actions.
- Uses `useProcesses` composable.
- Highlight the active process row with a distinct badge (e.g., `"Active (Managed)"`).
- Map the "Stop" button click to call `AdminApiService.stopProcess(pid)`.
- Wrap the stop action in the standard `useConfirm` confirmation modal.
- Display a toast notification on successful kill/error.

### Admin State Sync After Kill
When a process is killed from the Infrastructure panel, the Dashboard's admin state must update immediately (not wait for the 5s poll). Add a `triggerAdminStateRefresh` export to `useAdminState.ts`:

```typescript
// In useAdminState.ts — expose refresh for cross-tab syncing
export function triggerAdminStateRefresh() {
  return fetchState()  // calls the internal fetchState
}
```

In `InfrastructurePanel.vue`, after a successful kill:

```typescript
const handleKill = async (pid: number) => {
  try {
    await AdminApiService.stopProcess(pid)
    toast.success(`Process ${pid} stopped`)
    await refresh()                        // refresh process list
    await triggerAdminStateRefresh()       // sync Dashboard
  } catch (e) {
    toast.error(e.message)
  }
}
```

This ensures:
- Dashboard "Active Model" status updates immediately
- Infrastructure process list reflects the kill
- All UI components stay in sync without waiting for the next poll cycle

---

## Step 8: Frontend — Add Tab Routing & Header Button

Modify `App.vue` and `AdminHeader.vue`:
- Add `'infrastructure'` to the `AppTab` union type.
- Add the `<InfrastructurePanel v-else-if="activeTab === 'infrastructure'" />` to the main templates.
- Add a new "Infrastructure" tab button in the `AdminHeader.vue` nav bar.

---

## Step 9: Verify and Run Tests

```bash
cd frontend && npm run build
cd ../backend && go build ./... && go test ./...
```

---

## Risk Register

| Risk | Mitigation |
|------|------------|
| `ps` command output parsing is fragile | Use `ps -eo pid,lstart,command` to ensure fixed column positions for the PID and start time. |
| User stops the active process directly | `AdminProcessKillHandler` intercepts active PID stopping and invokes the clean `h.runtime.StopActive()` routine instead. |
| Port conflicts with unseen non-proxy orphans | The `StartModel` safety check frees the target port dynamically before launch. |
| Permission errors during kill | Catch exit status codes and return clean human-readable error messages in JSON if permission is denied. |
