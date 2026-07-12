---
status: superseded
last_reviewed: 2026-07-11
---

> **Archived:** Proposed, no activity — archived.

# Plan: Terminal UI (TUI) for LLM-Proxy

**Status:** proposed
**Date:** 2026-06-27
**Related specs:** SPEC-003 (Discovery Panel), SPEC-006 (Guardrails), SPEC-004 (Memory), SPEC-001 (Agent Loop), SPEC-007 (Automation Dispatcher), SPEC-008 (MCP)

## Problem

The admin interface is a Vue 3 SPA served over HTTP. Users who work exclusively in the terminal (SSH into headless servers, tmux users, automated environments) cannot access dashboard, logs, settings, or agent chat without a browser. The backend already has all REST endpoints — there is no technical barrier to a terminal-native client.

## Solution

Build a standalone Go TUI binary (`cmd/tui/`) using `charmbracelet/bubbletea` that connects to the same REST API + SSE stream the Vue SPA uses. It runs alongside the backend (or on a remote machine pointing at the backend's address). It is **not** a replacement — the web UI and TUI coexist. Users run whichever fits their context.

```
                  ┌───────────────────┐
                  │  Backend (:4001)  │
                  │  REST API + SSE   │
                  └────────┬──────────┘
                           │ HTTP
              ┌────────────┼────────────┐
              │            │            │
     ┌────────▼───┐ ┌─────▼─────┐ ┌────▼─────┐
     │ Vue SPA    │ │ curl/CLI  │ │ TUI      │
     │ (browser)  │ │ (adhoc)   │ │ (bubble) │
     └────────────┘ └───────────┘ └──────────┘
```

## Implementation

### Project layout

```
cmd/tui/
  main.go                    # Entry: flags, start tea.Program
  tea/
    root.go                  # Root model — tab switching, keybind dispatch
    dashboard.go             # Dashboard view (metrics, system health)
    settings.go              # Settings model + sub-models (wizard panels)
    logs.go                  # Scrollable log viewer
    chat.go                  # Chat view (message list + input)
    chat_sse.go              # SSE goroutine for live agent events
    workspaces.go            # Workspace list + file tree
    automations.go           # Automation list + detail + trigger
    memory.go                # Memory search + detail
    recordings.go            # Recording list + playback
    styles.go                # Lipgloss styles (color scheme, layout)
    components/
      spinner.go             # Loading indicator
      table.go               # Tabular data display
      form.go                # Text input / select / toggle fields
      confirm.go             # Confirmation dialog
      markdown.go            # Glamour markdown renderer
      viewport.go            # Scrollable viewport wrapper
      help.go                # Keybinding help overlay
  api/
    client.go                # Base HTTP client (fetch, poll)
    models.go                # Model CRUD endpoints
    config.go                # Config / settings endpoints
    metrics.go               # Metrics polling
    logs.go                  # Log fetching
    chat.go                  # Message send / session list
    workspaces.go            # Workspace + file CRUD
    automations.go           # Automation trigger / list
    memory.go                # Memory CRUD + search
    recordings.go            # Recording list / status
    sse.go                   # SSE stream reader (EventSource)
    types.go                 # Response types (mirrors frontend types/)
```

### Root model architecture

The root Bubble Tea model switches between view models on tab press. Each view model is a `tea.Model` that receives the root's dimensions.

```go
type rootModel struct {
    activeTab   Tab        // dashboard | settings | logs | chat | workspaces | automations | memory | recordings
    tabs        []Tab      // tab bar ordering
    width       int
    height      int

    // sub-models (each a tea.Model)
    dashboard   tea.Model
    settings    tea.Model
    logs        tea.Model
    chat        tea.Model
    workspaces  tea.Model
    automations tea.Model
    memory      tea.Model
    recordings  tea.Model
}
```

Key dispatch: `Ctrl+1..8` switches tabs. `q` or `Ctrl+C` quits. `?` shows help overlay.

### View tree (mapping to web UI)

| Web tab | TUI view | UI approach |
|---|---|---|
| Dashboard | `dashboard.go` | Single pane: GPU metrics gauge, active model count, system uptime. 3s poll via `tea.Tick`. |
| Settings | `settings.go` | Sequential wizard. Each sub-tab (local, gemini, openai, mcp, guardrails, security) is a separate form screen with Back/Next. |
| Logs | `logs.go` | Scrollable viewport (bubbles/viewport). Tabs for app/process logs. `r` to toggle follow mode. |
| Agent IDE → Chat | `chat.go` | Split pane: top=scrollable messages, bottom=text input. SSE-driven live updates. |
| Agent IDE → Workspaces | `workspaces.go` | File tree (bubbles/filetree) → select file → open in `$EDITOR`. |
| Agent IDE → Automations | `automations.go` | List → detail. `Enter` to trigger, `t` to stop. |
| Agent IDE → Memory | `memory.go` | Search bar + results list. `Enter` to view detail. |
| Agent IDE → Recordings | `recordings.go` | List + status indicator. `Enter` to view recording. |

### API client

A single `Client` struct wraps `net/http`:

```go
type Client struct {
    baseURL    string
    httpClient *http.Client
}

func New(baseURL string) *Client
func (c *Client) Get[T any](path string) (T, error)
func (c *Client) Post[T, B any](path string, body B) (T, error)
func (c *Client) Put[T, B any](path string, body B) (T, error)
func (c *Client) Delete(path string) error
```

Each domain file (`api/models.go`, `api/metrics.go`, etc.) wraps `Client` with typed methods using the generic helpers above.

Response types live in `api/types.go` — manually mirrored from `frontend/src/types/`. Auto-generation is not justified until the type count drifts significantly.

### SSE for live chat

`chat.go` spawns a goroutine on `ChatInit()` that connects to `GET /admin/api/dispatcher/workspaces/{id}/live`. The goroutine reads SSE frames via `bufio.Scanner`, decodes `event:` + `data:` lines, and sends typed events back to the Bubble Tea program via a `chan tea.Msg`. The chat model handles reconnect (exponential backoff, max 30s).

```go
type SSEMessage struct {
    Event string
    Data  []byte
}

type SSEError struct {
    Err error
}
```

### Markdown rendering

Chat messages contain markdown (code blocks, lists, inline formatting). Use `charmbracelet/glamour` with a dark theme:

```go
var glamourRenderer = glamour.NewTermRenderer(
    glamour.WithAutoStyle(),
    glamour.WithWordWrap(80),
)
```

Render on message receipt, cache the rendered string on the message struct so scrolling doesn't re-render.

### File editing in workspaces

When a user selects a file in the workspace tree:
1. Fetch file content via `GET /admin/api/dispatcher/workspaces/{w}/files/{f}`
2. Write to a temp file in `/tmp/llmproxy-*`
3. Launch `$EDITOR` (fallback: `vi`) via `tea.ExecProcess`
4. On editor exit, read temp file, POST via `PUT /admin/api/dispatcher/workspaces/{w}/files/{f}`
5. Update file tree

This avoids implementing a terminal text editor inside the TUI.

### Keybindings

| Key | Action |
|---|---|
| `Ctrl+1`..`8` | Switch tab |
| `q` / `Ctrl+C` | Quit / Confirm quit |
| `?` | Toggle help overlay |
| `↑`/`↓` / `j`/`k` | Navigate lists / scroll |
| `Enter` | Select / Submit / Open |
| `Esc` | Back / Close detail |
| `r` | Refresh current view |
| `Tab` / `Shift+Tab` | Form field focus |
| `PgUp`/`PgDn` | Scroll viewport |
| `/` | Search/filter in current view |

### Colorscheme

Match the backend's terminal aesthetic — dark background, cyan highlights, green for success, red for errors. Defined in `styles.go` as lipgloss `Style` values.

## Phasing

### Phase 1 — Scaffold + Dashboard + Logs (3 days)

| Day | Deliverable |
|---|---|
| 1 | `cmd/tui/main.go`, `tea/root.go`, `api/client.go`, `api/types.go`. Tab switching works, keybindings, help overlay. |
| 2 | `api/metrics.go`, `tea/dashboard.go` — metrics polling, CPU/GPU gauge, model count, uptime. |
| 3 | `api/logs.go`, `tea/logs.go` — scrollable log viewer, app/process toggle, follow mode. |

### Phase 2 — Settings wizard (3 days)

| Day | Deliverable |
|---|---|
| 1 | `tea/components/form.go` — text input, toggle, select. `tea/settings.go` — tab navigation between sub-screens. |
| 2 | `api/models.go`, `api/config.go` — local model config, global settings forms. |
| 3 | `api/secrets.go` — API key management. MCP server list. Guardrail config. Security settings. |

### Phase 3 — Chat + SSE (4 days)

| Day | Deliverable |
|---|---|
| 1 | `api/chat.go` — session list, message history, send message. `tea/chat.go` — message list view. |
| 2 | `api/sse.go`, `tea/chat_sse.go` — SSE goroutine, reconnect logic, event → `tea.Msg` bridge. |
| 3 | Markdown rendering via glamour. Message bubbles with role colors. |
| 4 | Chat input (multiline text area), send on `Ctrl+Enter`, session switching via list sidebar. |

### Phase 4 — Workspaces + Automations + Memory (4 days)

| Day | Deliverable |
|---|---|
| 1 | `api/workspaces.go`, `tea/workspaces.go` — file tree, `$EDITOR` integration. |
| 2 | `api/automations.go`, `tea/automations.go` — list, detail view, trigger/stop. |
| 3 | `api/memory.go`, `tea/memory.go` — search, list, detail view. |
| 4 | `tea/recordings.go` — list, status, playback detail. Polish: error handling, edge cases, README. |

## File Change Checklist

1. `cmd/tui/main.go` — entry point, flags, program init
2. `cmd/tui/tea/root.go` — root model, tab dispatch, keybinds, layout
3. `cmd/tui/tea/dashboard.go` — dashboard view
4. `cmd/tui/tea/settings.go` — settings wizard
5. `cmd/tui/tea/logs.go` — log viewer
6. `cmd/tui/tea/chat.go` — chat view (message list + input)
7. `cmd/tui/tea/chat_sse.go` — SSE event loop
8. `cmd/tui/tea/workspaces.go` — workspace file tree
9. `cmd/tui/tea/automations.go` — automation list + detail
10. `cmd/tui/tea/memory.go` — memory search + detail
11. `cmd/tui/tea/recordings.go` — recording list
12. `cmd/tui/tea/styles.go` — lipgloss styles
13. `cmd/tui/tea/components/spinner.go` — loading indicator
14. `cmd/tui/tea/components/table.go` — table display
15. `cmd/tui/tea/components/form.go` — form widgets
16. `cmd/tui/tea/components/confirm.go` — confirmation dialog
17. `cmd/tui/tea/components/markdown.go` — glamour renderer
18. `cmd/tui/tea/components/viewport.go` — scrollable viewport
19. `cmd/tui/tea/components/help.go` — keybind help overlay
20. `cmd/tui/api/client.go` — base HTTP client
21. `cmd/tui/api/types.go` — response type definitions
22. `cmd/tui/api/models.go` — model endpoints
23. `cmd/tui/api/config.go` — config endpoints
24. `cmd/tui/api/metrics.go` — metrics endpoints
25. `cmd/tui/api/logs.go` — log endpoints
26. `cmd/tui/api/chat.go` — chat endpoints
27. `cmd/tui/api/workspaces.go` — workspace endpoints
28. `cmd/tui/api/automations.go` — automation endpoints
29. `cmd/tui/api/memory.go` — memory endpoints
30. `cmd/tui/api/recordings.go` — recording endpoints
31. `cmd/tui/api/sse.go` — SSE reader
32. `docs/PLANS/cross-cutting/terminal-ui.md` — this plan
33. `docs/INDEX.md` — add entry

## Edge Cases

- **Backend unreachable**: Show connection error with retry prompt. `Client` methods return wrapped errors; root model displays a persistent error bar. All views degrade gracefully (show "offline" state).
- **SSE reconnect storm**: Cap backoff at 30s. Reset backoff on successful connection. Show "reconnecting..." indicator in chat status bar.
- **Resize**: Bubble Tea handles terminal resize via `tea.WindowSizeMsg`. Root model re-layouts all sub-models on resize. Views stack vertically when width < 100 columns.
- **$EDITOR not set**: Fall back to `vi` (universally available). If `vi` also missing, show error and prompt user to `export EDITOR`.
- **Slow API**: All API calls wrapped in spinner/loading state via the shared spinner component. 10s timeout on HTTP requests.
- **Long message lists**: Viewport only renders visible lines. On scroll-up, fetch older messages from session history.
- **No SSE endpoint** (dispatcher disabled): Chat view shows read-only message history. No live streaming — user sends message, pool-waits for response.
- **Secrets in logs**: Follow same guardrail as web UI — never display provider keys. Key names shown, values masked.
- **Multiple workspaces**: Workspace selector at top of workspaces/chat/automations views. Switch with `Ctrl+P`.
- **UTF-8 / emoji**: Bubble Tea handles Unicode. Glamour renders emoji in markdown. No special handling needed.

## Build & Distribution

```bash
# Build standalone binary
cd backend
go build -o llmproxy-tui ./cmd/tui/

# Cross-compile
GOOS=linux GOARCH=amd64 go build -o llmproxy-tui-linux ./cmd/tui/
GOOS=darwin GOARCH=arm64 go build -o llmproxy-tui-darwin ./cmd/tui/
```

Flags:

```
--api string        Backend API base URL (default "http://127.0.0.1:4001")
--workspace string  Default workspace ID (default "default")
```

No other configuration needed — all state lives in the backend. The TUI is stateless.

## Server-side changes required

**None.** The TUI consumes existing REST API and SSE endpoints. No new backend routes or CORS configuration needed (localhost-to-localhost).

Optional enhancement: add a `Content-Type: text/event-stream` CORS header for SSE if the TUI connects from a non-localhost address. Not needed for the default localhost use case.
