---
status: complete
last_reviewed: 2026-07-11
---

# Fresh Webhook Sessions + Source Grouping

**Status:** ✅ Completed
**Note:** The backend fresh-session change (§1–§2) is implemented. The frontend
grouping described in §3–§4 was **superseded** by
`docs/PLANS/cross-cutting/session-source-backend-driven.md`, which derives `source`
from the backend and groups webhook sessions under a single "Webhook" header (manual
stays flat, with a group-level delete). Do not implement §3–§4 as written — they
reference the removed `sessionSource(id)` parser and `groupOrder`/`groupLabels`.

## Problem

Webhook conversations reuse a stable session key (`wb_{connectorName}_{chatID}`). When a second message arrives in the same Telegram chat, `findOrCreateSourceSession` returns the existing session with its full history — including the prior `submit_final_answer` containing a ~2000-char report. Qwen gets confused by its own output in context and produces malformed responses (echoed content text alongside the tool call).

## Solution

Each webhook message gets a **fresh conversation** by appending a timestamp suffix to the session key. Previous runs persist in storage and remain visible in the assistant history panel. Sessions are grouped by source in the UI.

### Session key format

```
Before: wb_vertex-telegram_8699725510         (reused, history grows)
After:  wb_telegram_8699725510_20260709T144343Z (unique per run, always fresh)
```

- `wb` — webhook source (frontend `source.ts` already detects this prefix)
- `telegram` — platform type (groups by platform)
- `8699725510` — Telegram chat ID (stable identifier)
- `20260709T144343Z` — ISO UTC timestamp (guarantees uniqueness, lexicographically sortable)

## Changes

### 1. Backend: `backend/internal/transport/http/handlers/webhook_handlers.go`

**Remove** `findOrCreateSourceSession` (currently lines 212–226). No longer needed — each run gets a fresh key, never reuses an existing session.

**Add** helper:

```go
func webhookSessionKey(platformType, chatID string) string {
    return fmt.Sprintf("wb_%s_%s_%s", platformType, chatID, time.Now().UTC().Format("20060102T150405Z"))
}
```

**Simplify** `handleAgentMessage` — remove the session read and error handling, construct the payload directly:

```go
func (h *WebhookHandler) handleAgentMessage(workspaceID, connectorName, connectorType, message string, chatID string) {
    h.Logger.Info("webhook agent request", "workspace", workspaceID, "connector", connectorName)

    payload := &AssistantMessage{
        WorkspaceID:    workspaceID,
        ConversationID: webhookSessionKey(connectorType, chatID),
        Message:        message,
        Timezone:       "UTC",
        ExcludeTools:   []string{models.ToolNotifyUser},
    }

    go h.runAgentReply(workspaceID, connectorName, connectorType, chatID, payload)
}
```

Drops from 18 lines to 12 lines. No `session` variable, no `err` check, no `findOrCreateSourceSession`.

### 2. Tests: `backend/internal/transport/http/handlers/webhook_handlers_test.go`

New file. One test — format verification (not flaky):

```go
func TestWebhookSessionKey_Format(t *testing.T) {
    key := webhookSessionKey("telegram", "8699725510")
    parts := strings.Split(key, "_")
    if len(parts) != 4 {
        t.Fatalf("expected 4 parts, got %d: %s", len(parts), key)
    }
    if parts[0] != "wb" || parts[1] != "telegram" || parts[2] != "8699725510" {
        t.Errorf("unexpected format: %s", key)
    }
    if len(parts[3]) != 16 { // ISO timestamp = 16 chars: 20060102T150405Z
        t.Errorf("expected 17-char timestamp suffix, got %d: %q", len(parts[3]), parts[3])
    }
}
```

### 3. Frontend: `frontend/src/components/AgentIde/assistant/ChatSessionList.vue`

> ⚠️ **Superseded** by `session-source-backend-driven.md` — `source` is now
> backend-derived and grouping lives in `utils/assistant/source.ts`. The
> `sessionSource(id)` / `groupOrder` / `groupLabels` approach below was removed.

**Add** group computation — pure view logic for the session list, no extraction needed:

```ts
import { sessionSource, type SessionSource } from '../../../utils/assistant/source'

const groupOrder: SessionSource[] = ['webhook-telegram', 'webhook', 'manual']
const groupLabels: Record<SessionSource, string> = {
  'webhook-telegram': 'Webhook — Telegram',
  'webhook': 'Webhook',
  'manual': '',
}
const groupedSessions = computed(() => {
  const result: { source: SessionSource; sessions: SessionBrief[] }[] = []
  for (const src of groupOrder) {
    const matches = props.sessions.filter(s => sessionSource(s.id) === src)
    if (matches.length) result.push({ source: src, sessions: matches })
  }
  return result
})
```

**Update** template — wrap existing session list with outer group loop:

```
Before:
  <div class="session-list">
    <div v-for="session in sessions" :key="session.id" ...>

After:
  <div class="session-list">
    <template v-for="group in groupedSessions" :key="group.source">
      <div v-if="groupLabels[group.source]" class="session-group-header">
        {{ groupLabels[group.source] }}
      </div>
      <div v-for="session in group.sessions" :key="session.id" ...>
```

**CSS** — add `.session-group-header` style (subtle, non-sticky, matches panel font size/color).

### 4. Frontend: `frontend/src/utils/assistant/source.ts`

> ⚠️ **Superseded** — `source.ts` was rewritten to map a backend-supplied
> `source` string (no ID parsing). The `id.startsWith('wb_telegram')` logic below
> no longer exists.

No changes. Existing detection already works:

```ts
if (id.startsWith('wb_telegram')) return 'webhook-telegram'
// New key "wb_telegram_8699725510_20260709T144343Z" matches wb_telegram → correct
```

### 5. Docs

| File | What changes |
|---|---|
| `docs/PLANS/cross-cutting/connector-auto-reply.md:37` | `"wb_{connectorName}_{sourceChatID}"` → `"wb_{platformType}_{chatID}_{timestamp}"` |
| `docs/PLANS/cross-cutting/connector-auto-reply.md:123` | `fmt.Sprintf("wb_%s_%d", connectorName, sourceChatID)` → `webhookSessionKey(platformType, chatID)` |
| `docs/PLANS/assistant-ui/running-indicator-webhook.md:12` | Add note: timestamp suffix does not affect `wb_` / `wb_telegram_` prefix detection |

## Verification

```bash
cd backend && go test ./internal/transport/http/handlers/ -run TestWebhookSessionKey -v -count=1
cd backend && go build ./...
cd frontend && npm run build
```

Manual: send two Telegram messages in the same chat → verify each produces a clean response with no echoed prior answer. Check history panel shows both runs grouped under a single "Webhook" header (manual sessions stay flat).

## Impact

- **Files touched:** 5 (3 backend, 2 frontend, 2 docs updated)
- **New test file:** 1
- **Lines added:** ~35
- **Lines removed:** ~20
- **Zero behavior change to agent execution** — only session key generation changes
