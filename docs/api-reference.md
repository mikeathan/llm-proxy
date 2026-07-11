# API Reference

## Base URL

All API endpoints are served from the server root (default `http://0.0.0.0:4001`).

---

## Conversation API

### Send Message

`POST /admin/api/conversation/message`

```json
{
  "workspace_id": "default",
  "conversation_id": "conv_20260101120000",
  "message": "Hello",
  "context_version": "",
  "timezone": "",
  "exclude_tools": []
}
```

**Response:** `200 OK`

```json
{
  "reply": "Hello! How can I help you?",
  "conversation_id": "conv_20260101120000",
  "workspace_id": "default",
  "events": [
    { "type": "tool_call", "payload": { "name": "notify_user", ... } },
    { "type": "tool_result", "payload": { "name": "notify_user", ... } }
  ]
}
```

### Cancel

`POST /admin/api/conversation/cancel`

```json
{ "workspace_id": "default", "conversation_id": "conv_..." }
```

### Sessions

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/conversation/sessions/{workspace}` | List sessions |
| GET | `/admin/api/conversation/sessions/{workspace}/{session}` | Get session |
| DELETE | `/admin/api/conversation/sessions/{workspace}/{session}` | Delete session |
| DELETE | `/admin/api/conversation/sessions/{workspace}` | Delete all sessions |
| PATCH | `/admin/api/conversation/sessions/{workspace}/{session}` | Rename session (body: `{"title":"..."}`) |

---

## Admin API

### State

`GET /admin/api/state?available=1`

Returns full admin state: active model, available models, guardrails config, provider status.

### Models

| Method | Path | Description |
|--------|------|-------------|
| POST | `/admin/api/models` | Add model |
| PUT | `/admin/api/models` | Update model |
| DELETE | `/admin/api/models?name={name}` | Delete model |
| DELETE | `/admin/api/models/all?provider={provider}` | Delete all models for provider |
| GET | `/admin/api/registry` | Full registry (models, providers, MCP servers) |
| PUT | `/admin/api/registry` | Update registry |

### Providers

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/providers/models?provider={p}&api_key_name={k}` | List remote models |
| GET | `/admin/api/providers/manifests` | List provider manifests |
| GET | `/admin/api/providers/test?provider={p}` | Test connection |

### Secrets

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/secrets/keys?provider={p}` | Get masked API keys |
| PUT | `/admin/api/secrets/keys?provider={p}` | Replace API keys |
| DELETE | `/admin/api/secrets/keys?provider={p}&key_id={id}` | Delete key |

### System

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/version` | Build info (version, commit, date) |
| GET | `/admin/api/config` | Full config |
| PUT | `/admin/api/config` | Update config |
| GET | `/admin/api/system` | System settings |
| PUT | `/admin/api/system` | Update system |
| POST | `/admin/api/system/restart` | Restart server |

### Host

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/host` | Host settings |
| PUT | `/admin/api/host` | Update host settings |
| POST | `/admin/api/host/terminal/reset?workspaceID={id}` | Reset terminal |
| GET | `/admin/api/host/terminal/sessions` | List terminal sessions |

### Runtime

| Method | Path | Description |
|--------|------|-------------|
| POST | `/admin/api/start` | Start model |
| POST | `/admin/api/stop` | Stop active model |
| GET | `/admin/api/logs` | Process logs |
| DELETE | `/admin/api/logs` | Clear logs |
| GET | `/admin/api/metrics` | System metrics |
| GET | `/admin/api/runtime/processes` | List processes |
| POST | `/admin/api/runtime/processes/{pid}/stop` | Kill process |

### Log Level

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/log-level` | Get log level |
| PUT | `/admin/api/log-level` | Set log level (`{"level":"DEBUG"}`) |

### App Logs

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/app-logs` | Download app log |
| GET | `/admin/api/app-logs/tail` | Tail app log |
| DELETE | `/admin/api/app-logs` | Clear app log |

### MCP

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/mcp` | List MCP servers |
| POST | `/admin/api/mcp` | Add MCP server |
| PUT | `/admin/api/mcp` | Update MCP server |
| DELETE | `/admin/api/mcp?name={name}` | Remove MCP server |

---

## Dispatcher / Automation API

### Automations

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/dispatcher/automations` | List automations |
| GET | `/admin/api/dispatcher/metrics` | Dispatcher metrics |
| GET | `/admin/api/dispatcher/activity` | Global activity log |

### Workspaces

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/dispatcher/workspaces` | List workspaces |
| POST | `/admin/api/dispatcher/workspaces` | Create workspace |
| DELETE | `/admin/api/dispatcher/workspaces/{workspace}` | Delete workspace |
| GET | `/admin/api/dispatcher/workspaces/{workspace}/state` | Get workspace state |
| GET | `/admin/api/dispatcher/workspaces/{workspace}/config` | Get workspace config |
| PUT | `/admin/api/dispatcher/workspaces/{workspace}/config` | Update workspace config |
| GET | `/admin/api/dispatcher/workspaces/{workspace}/live` | SSE event stream |
| POST | `/admin/api/dispatcher/trigger/{workspace}/{automation}` | Trigger automation |
| POST | `/admin/api/dispatcher/stop/{workspace}` | Stop automation |

### Workspace Files

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/dispatcher/workspaces/{workspace}/files` | List files |
| GET | `/admin/api/dispatcher/workspaces/{workspace}/files/{file}` | Read file |
| PUT | `/admin/api/dispatcher/workspaces/{workspace}/files/{file}` | Write file |
| DELETE | `/admin/api/dispatcher/workspaces/{workspace}/files/{file}` | Delete file |

### Workspace Automations

| Method | Path | Description |
|--------|------|-------------|
| POST | `/admin/api/dispatcher/workspaces/{workspace}/automations` | Create automation |
| PUT | `/admin/api/dispatcher/workspaces/{workspace}/automations/{automation}` | Update automation |
| DELETE | `/admin/api/dispatcher/workspaces/{workspace}/automations/{automation}` | Delete automation |

### Recordings

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/recordings` | List recordings |
| GET | `/admin/api/recordings/status` | Recording status |
| GET | `/admin/api/recordings/{id}` | Get recording |
| DELETE | `/admin/api/recordings/{id}` | Delete recording |

---

## Proxy API

`/v1/chat/completions` — OpenAI-compatible chat completions endpoint.

Supports streaming (SSE) with `stream: true`.

---

## Public Webhooks

`POST /api/v1/webhooks/{connector_name}` — External platforms post here. No admin auth.

---

## Templates

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/templates` | List templates |

---

## Memory API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/memory/{workspace}` | List memories |
| POST | `/admin/api/memory/{workspace}/search` | Search memories |
| GET | `/admin/api/memory/{workspace}/{id}` | Get memory |
| PUT | `/admin/api/memory/{workspace}/{id}` | Update memory |
| DELETE | `/admin/api/memory/{workspace}/{id}` | Delete memory |
| DELETE | `/admin/api/memory/{workspace}` | Clear workspace memories |

---

## Error Format

All error responses use:

```json
{ "error": "description of what went wrong" }
```

HTTP status codes: 400 (bad request), 404 (not found), 409 (conflict), 500 (internal error), 503 (service unavailable).
