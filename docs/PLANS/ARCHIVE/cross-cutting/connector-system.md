# PLAN: Generic Outbound Communication Connector System — Phase 1

**Status:** Complete  
**SPEC:** `docs/SPECS/communication.md`  
**Depends on:** None

---

## Goal

Replace the hardcoded `Notifier` interface + `TelegramNotifier` struct with a generic `Connector` interface and map-based config, so any communication platform (Slack, Discord, email, etc.) can be added later via a connector name + type + settings map. No backward-compat shim. Telegram refactored to comply with CONSTITUTION §5 (network I/O via `NetworkTools`, never `http.DefaultClient`).

---

## Architecture

### Config shape in `registry.json`

Current (removed):
```json
{
  "communication": {
    "telegram": { "enabled": true, "chat_id": "12345" }
  }
}
```

New:
```json
{
  "communication": {
    "connectors": {
      "my-telegram": {
        "type": "telegram",
        "enabled": true,
        "settings": { "chat_id": "12345" },
        "secret_ref": "my-telegram"
      }
    }
  }
}
```

Secrets store path: `SecretsStore.GetSecret("connector", "my-telegram")` → bot token.  
The `secret_ref` field is the key name inside the secrets store for that connector's token/credential.

### Connector Interface

```go
// internal/core/tools/communication.go
type Connector interface {
    Name() string
    Send(ctx context.Context, message string) error
}

type CommunicationTools struct {
    connectors map[string]Connector
}
```

No inbound methods yet (Phase 2). The map key is the connector name from the config (`"my-telegram"`), so `NotifyAll` provides meaningful error attribution.

### Tool

The existing `notify_user` tool stays unchanged. Its handler calls `r.Communication.NotifyAll(ctx, args.Message)` which iterates the map.

---

## Backend — Files to Modify

### 1. `backend/models/config.go`

**Remove:**
```go
type CommunicationConfig struct {
    Telegram struct {
        Enabled bool   `json:"enabled"`
        ChatID  string `json:"chat_id"`
    } `json:"telegram"`
}
```

**Add:**
```go
type CommunicationConfig struct {
    Connectors map[string]ConnectorConfig `json:"connectors"`
}

type ConnectorConfig struct {
    Type      string            `json:"type"`
    Enabled   bool              `json:"enabled"`
    Settings  map[string]string `json:"settings"`
    SecretRef string            `json:"secret_ref,omitempty"`
}
```

### 2. `backend/internal/core/tools/communication.go`

**Changes:**
- Remove `Notifier` interface
- Add `Connector` interface (`Name()`, `Send()`)
- `CommunicationTools.notifiers []Notifier` → `connectors map[string]Connector`
- `AddNotifier(n Notifier)` → `AddConnector(name string, c Connector)` — adds to map
- `NotifyAll(ctx, msg string)` — iterates map, calls `Send` on each enabled connector, collects errors
- `TelegramNotifier` — change constructor to accept `client *http.Client` instead of using `http.DefaultClient`
- `TelegramNotifier.Send()` — uses the injected client
- Remove the manual `net/url` query encoding; use `http.PostForm` or construct request body properly

**Full new file structure (sketch):**
```go
package tools

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"
)

type Connector interface {
    Name() string
    Send(ctx context.Context, message string) error
}

type CommunicationTools struct {
    connectors map[string]Connector
}

func NewCommunicationTools() *CommunicationTools {
    return &CommunicationTools{connectors: make(map[string]Connector)}
}

func (c *CommunicationTools) AddConnector(name string, conn Connector) {
    c.connectors[name] = conn
}

func (c *CommunicationTools) NotifyAll(ctx context.Context, message string) error {
    var errs []error
    for name, conn := range c.connectors {
        if err := conn.Send(ctx, message); err != nil {
            errs = append(errs, fmt.Errorf("%s (%s): %w", name, conn.Name(), err))
        }
    }
    if len(errs) > 0 {
        return fmt.Errorf("some notifications failed: %v", errs)
    }
    return nil
}

type TelegramNotifier struct {
    Token   string
    ChatID  string
    client  *http.Client
}

func NewTelegramNotifier(token, chatID string, client *http.Client) *TelegramNotifier {
    return &TelegramNotifier{Token: token, ChatID: chatID, client: client}
}

func (t *TelegramNotifier) Name() string { return "Telegram" }

func (t *TelegramNotifier) Send(ctx context.Context, message string) error {
    if t.Token == "" || t.ChatID == "" || t.client == nil {
        return fmt.Errorf("telegram connector not fully configured")
    }
    apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.Token)
    formData := url.Values{}
    formData.Set("chat_id", t.ChatID)
    formData.Set("text", message)
    formData.Set("parse_mode", "Markdown")
    req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(formData.Encode()))
    if err != nil { return err }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    resp, err := t.client.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
        return fmt.Errorf("telegram API error: status %d, body: %s", resp.StatusCode, string(body))
    }
    return nil
}
```

### 3. `backend/internal/core/assistant/registry.go`

**`initCommunicationTools`** signature changes from:
```go
func initCommunicationTools(appCtx interface {
    GetRegistry() models.RegistryData
    Secrets() models.SecretsStore
}) *tools.CommunicationTools
```
To:
```go
func initCommunicationTools(appCtx interface {
    GetRegistry() models.RegistryData
    Secrets() models.SecretsStore
}, network *tools.NetworkTools) *tools.CommunicationTools
```

**Body changes** — replace:
```go
comm := tools.NewCommunicationTools()
telegramToken := appCtx.Secrets().GetSecret("communication", "telegram")
if reg.Communication.Telegram.Enabled && telegramToken != "" {
    comm.AddNotifier(&tools.TelegramNotifier{
        Token: telegramToken,
        ChatID: reg.Communication.Telegram.ChatID,
    })
}
```
With:
```go
comm := tools.NewCommunicationTools()
for name, cfg := range reg.Communication.Connectors {
    if !cfg.Enabled { continue }
    switch cfg.Type {
    case "telegram":
        token := appCtx.Secrets().GetSecret("connector", name)
        chatID := cfg.Settings["chat_id"]
        if token == "" || chatID == "" {
            continue
        }
        comm.AddConnector(name, tools.NewTelegramNotifier(token, chatID, network.HTTPClient()))
    default:
        // unknown connector type — log warning, skip
    }
}
```

Also add the necessary string import for the map key iteration (already imported in registry.go).

**`InitializeAgentStack`** — pass `network` into `initCommunicationTools`:
```go
comm := initCommunicationTools(appCtx, network)
```

The `network` variable is already created at line 206 before `comm` is set.

### 4. `backend/internal/transport/http/system_handlers.go`

Line 39 — `Communication: reg.Communication` serializes the new map-based config automatically via JSON marshalling. No code change needed, but verify the shape in the response.

### 5. `backend/internal/transport/http/admin_handlers.go`

Line 167 — `Communication models.CommunicationConfig` field stays. Line 300 — `Communication: reg.Communication` stays. No code changes.

### 6. `backend/internal/app/app_context.go`

Lines 427-448 — `req.Communication` is a `*CommunicationConfig` and is copied into `reg.Communication = *req.Communication`. No code changes — the new map type serializes/deserializes transparently.

---

## Frontend — Files to Modify/Create

### 1. `frontend/src/types/admin.ts`

**Add:**
```typescript
export interface ConnectorConfig {
  type: string
  enabled: boolean
  settings: Record<string, string>
  secret_ref?: string
}
```

**Update `CommunicationConfig`:**
```typescript
export interface CommunicationConfig {
  connectors: Record<string, ConnectorConfig>
}
```

### 2. `frontend/src/composables/models/useConfig.ts`

Update `DEFAULT_CONFIG.communication`:
```typescript
communication: {
  connectors: {},
},
```

### 3. `frontend/src/components/settings/CommunicationSettings.vue` (NEW)

Full component sketch:

```vue
<script setup lang="ts">
import { ref, computed } from "vue"
import type { GlobalConfig, ConnectorConfig } from "../../types/admin"
import BaseButton from "../common/buttons/BaseButton.vue"

const props = defineProps<{
  editConfig: GlobalConfig
}>()

const emit = defineEmits<{
  (e: "update:editConfig", config: GlobalConfig): void
  (e: "updateConfig"): void
}>()

const connectors = computed({
  get: () => props.editConfig.communication?.connectors ?? {},
  set: (val: Record<string, ConnectorConfig>) => {
    const clone = JSON.parse(JSON.stringify(props.editConfig))
    if (!clone.communication) clone.communication = { connectors: {} }
    clone.communication.connectors = val
    emit("update:editConfig", clone)
  }
})

interface ConnectorForm { name: string; type: string; chat_id: string; token: string }
const showForm = ref(false)
const form = ref<ConnectorForm>({ name: "", type: "telegram", chat_id: "", token: "" })

function addConnector() {
  const name = form.value.name.trim()
  if (!name) return
  const settings: Record<string, string> = {}
  if (form.value.chat_id) settings.chat_id = form.value.chat_id

  const updated = {
    ...connectors.value,
    [name]: {
      type: form.value.type,
      enabled: true,
      settings,
      secret_ref: form.value.token ? name : undefined,
    },
  }
  connectors.value = updated
  showForm.value = false
  form.value = { name: "", type: "telegram", chat_id: "", token: "" }
}

function removeConnector(name: string) {
  const updated = { ...connectors.value }
  delete updated[name]
  connectors.value = updated
}

function toggleConnector(name: string) {
  const updated = { ...connectors.value }
  if (updated[name]) updated[name] = { ...updated[name], enabled: !updated[name].enabled }
  connectors.value = updated
}

function save() {
  emit("updateConfig")
}
</script>

<template>
  <div class="settings-container">
    <h2 class="settings-title">Communication Connectors</h2>
    <div class="form-helper mb-4">
      Configure external platforms the agent can use to send notifications and reports.
    </div>

    <div v-if="Object.keys(connectors).length === 0" class="empty-state">
      No connectors configured. Add a Telegram, Slack, or other connector below.
    </div>

    <div v-for="(cfg, name) in connectors" :key="name" class="connector-row">
      <div class="connector-info">
        <span class="connector-name">{{ name }}</span>
        <span class="connector-type">{{ cfg.type }}</span>
      </div>
      <label class="toggle-row">
        <input type="checkbox" :checked="cfg.enabled" @change="toggleConnector(name)" />
      </label>
      <button @click="removeConnector(name)" class="btn-remove" title="Remove connector">
        <Icon name="trash" size="xs" />
      </button>
    </div>

    <button v-if="!showForm" @click="showForm = true" class="btn-add">
      + Add Connector
    </button>

    <div v-if="showForm" class="add-form">
      <input v-model="form.name" placeholder="Connector name (e.g. my-telegram)" />
      <select v-model="form.type">
        <option value="telegram">Telegram</option>
      </select>
      <input v-model="form.chat_id" placeholder="Chat ID (telegram)" />
      <input v-model="form.token" type="password" placeholder="Bot token" />
      <div class="form-actions">
        <BaseButton variant="primary" @click="addConnector">Add</BaseButton>
        <BaseButton variant="secondary" @click="showForm = false">Cancel</BaseButton>
      </div>
    </div>

    <div class="save-bar">
      <BaseButton variant="primary" icon="play" @click="save">Save Connector Settings</BaseButton>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.settings-container {
  @apply bg-gray-800 rounded-lg shadow-xl border border-gray-700 p-6 space-y-4;
}
.settings-title {
  @apply text-xl font-bold text-white mb-2;
}
.form-helper {
  @apply text-xs text-gray-500;
}
.empty-state {
  @apply text-sm text-gray-500 italic py-4 text-center;
}
.connector-row {
  @apply flex items-center justify-between py-2 px-3 bg-gray-900 rounded border border-gray-700;
}
.connector-info {
  @apply flex items-center gap-2;
}
.connector-name {
  @apply text-sm font-medium text-gray-200;
}
.connector-type {
  @apply text-[10px] uppercase tracking-wider text-blue-400 bg-blue-500/10 px-2 py-0.5 rounded;
}
.toggle-row {
  @apply flex items-center;
}
.toggle-row input {
  @apply w-4 h-4;
}
.btn-remove {
  @apply p-1 hover:bg-red-500/15 text-gray-500 hover:text-red-400 rounded transition-colors;
}
.btn-add {
  @apply text-sm text-blue-400 hover:text-blue-300 py-2;
}
.add-form {
  @apply space-y-3 p-4 bg-gray-900 rounded border border-gray-700;
}
.add-form input,
.add-form select {
  @apply w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-sm;
}
.form-actions {
  @apply flex gap-2;
}
.save-bar {
  @apply pt-4 border-t border-gray-700 flex justify-end;
}
</style>
```

### 4. `frontend/src/components/settings/Settings.vue`

Add import:
```typescript
import CommunicationSettings from "./CommunicationSettings.vue"
```

Add tab routing alongside the existing tabs (after the existing tab divs):
```vue
<!-- Communication -->
<div v-show="activeTab === 'communication'">
  <CommunicationSettings
    :editConfig="config"
    @update:editConfig="(val: any) => config = val"
    @updateConfig="handleSaveConfig"
  />
</div>
```

Note: `config = val` in the template triggers `config.value = val` since `config` is a ref with auto-unwrapping — correct.

### 5. `frontend/src/domain/settings.ts`

Register the `"communication"` tab. Add to the settings groups returned by `getSettingsGroups`. For example, in the services/integrations group:
```typescript
{ name: "Services", tabs: ["communication"] }
```

Add icon/label mapping:
```typescript
case "communication": return "📡"
case "communication": return "Communication"
```

---

## Backend Tests That Need Updating

Run `go test ./...` and fix any that reference:
- `models.CommunicationConfig` fields directly (e.g. `.Telegram`)
- `tools.Notifier` interface
- `tools.NewCommunicationTools()` followed by `AddNotifier()`

Typical fix pattern:
```go
// Old: expect reg.Communication.Telegram.Enabled
// New: expect reg.Communication.Connectors["telegram"].Enabled
```

## Frontend Build Verification

`npm run build` must pass with zero errors.

---

## CONSTITUTION Compliance

### §5 — Network & Terminal
- **Before (violation):** `http.DefaultClient.Do(req)` in `TelegramNotifier.Send`
- **After (compliant):** `TelegramNotifier` receives `*http.Client` from `NetworkTools.HTTPClient()` injected at initialization

### §4 — Model Architecture
No changes. Agent loop, tool execution, and prompt system untouched.

---

## No Changes Required (verified)

| Area | File(s) | Reason |
|------|---------|--------|
| Tool constants | `models/tools.go` | `ToolNotifyUser` stays |
| Tool manifests | `manifests/communication.json` | Same `notify_user` schema |
| Agent loop | `agent.go`, `stream.go`, `session.go` | Tool execution flow unchanged |
| Guardrails | `guardrails.go`, `guardrails/*` | `CategoryCommunication` unchanged |
| System prompt | `prompts/templates.go` | Tool listing auto-generated from `registerAll()` |
| Secrets store | `models/secrets.go` | Interface unchanged; path becomes `("connector", name)` |
| SSE events | `agent_events.go` | Event types unchanged |
| MCP | `nodeherder/*` | Not related |

---

## Manual Testing Steps

1. Start server: `cd backend && go run main.go --data ./data`
2. Navigate to Settings → Communication tab
3. Add a connector:
   - Name: `my-telegram`
   - Type: `telegram`
   - Chat ID: your chat ID
   - Bot token: your Telegram bot token
4. Click Save
5. Open the assistant (select a workspace, open chat)
6. Send: "send me a test notification via Telegram"
7. Wait for agent to call `notify_user` tool
8. Verify message arrives in Telegram
9. Check server logs for any connector errors

---

## Completed Phases

- **Phase 1:** Generic outbound connector system with Telegram support. See `connector-inbound-webhook.md` for Phase 2.
