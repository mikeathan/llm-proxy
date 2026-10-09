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
| GET | `/admin/api/conversation/sessions/{workspace}/{session}` | Get session. A user message in `history` may carry `run`: `{model, started_at, duration_ms, prompt_tokens?, completion_tokens?}` — how the turn it started ran; tokens only when the provider reported them |
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
| GET | `/admin/api/config` | Full config. `memory` is always present: `{assistant_hot, automation_hot}`, the resolved global hot-memory defaults (shipped: assistant on, automations off) |
| PUT | `/admin/api/config` | Update config. `memory: {assistant_hot, automation_hot}` writes both defaults and leaves the other memory settings alone; omit `memory` to leave them unchanged |
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
| GET | `/admin/api/active-runs` | Global run state: `lane_holders`, `queued`, and `lanes` (per lane `lane`, `limit`, `running`, `waiting`, `holder_keys`). A chat (`kind: interactive`) holder also carries `conversation_id` (omitted while unknown, and for automations and inbound callers) so the UI can link straight to the running conversation |
| GET | `/admin/api/workspaces/{ws}/active-runs` | A workspace's assistant / automation running flags |
| POST | `/admin/api/queue/{queue_key}/promote` | Serve a queued inbound caller now (cancels the run holding its model) |
| POST | `/admin/api/queue/{queue_key}/cancel` | Drop a queued inbound caller |

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
| GET | `/admin/api/dispatcher/workspaces/{workspace}/heartbeat` | The workspace heartbeat: `{config, status?, lane, wakes_local_model, has_checks}` (SPEC-007 §II.6) |
| PUT | `/admin/api/dispatcher/workspaces/{workspace}/heartbeat` | Save `{enabled, every, model, notify, active_hours}` (`every` 1m–24h; `active_hours` `HH:MM-HH:MM` in server time, optional, wraps past midnight, an empty window like `08:00-08:00` → 400; `notify` as on an automation; invalid → 400) into the workspace config and schedule it at once; returns the same state as GET. The name `heartbeat` is reserved for this and cannot be used for an automation |
| PUT | `/admin/api/dispatcher/workspaces/{workspace}/config` | Update workspace config (replaces the document). `assistant_memory` (`""` inherit, `"on"`, `"off"`) overrides the global assistant hot-memory default for this workspace's chats; any other value → 400 |
| GET | `/admin/api/dispatcher/workspaces/{workspace}/live` | SSE event stream |
| POST | `/admin/api/dispatcher/trigger/{workspace}/{automation}` | Trigger automation |
| POST | `/admin/api/dispatcher/stop/{workspace}` | Stop automation |

### Workspace Files

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/dispatcher/workspaces/{workspace}/tree` | Recursive file tree: `{"entries":[{"path","type":"file"\|"dir","collapsed"?}],"truncated"}` — slash paths, sorted, dirs included, capped at 5000 entries breadth-first; dotfiles and escaping symlinks omitted; `node_modules`-style dirs listed `collapsed` and not descended |
| GET | `/admin/api/dispatcher/workspaces/{workspace}/files/{file...}` | Read file (nested path; each segment percent-encoded) |
| PUT | `/admin/api/dispatcher/workspaces/{workspace}/files/{file...}` | Write file (parent directories created) |
| DELETE | `/admin/api/dispatcher/workspaces/{workspace}/files/{file...}` | Delete a file or an empty folder; `?recursive=true` deletes a folder with everything under it (the workspace root itself is refused) |

### Workspace Automations

Optional per-automation `memory_mode` overrides the global automation hot-memory default (`memory.automation_hot`): `""` (inherit), `"on"` or `"off"`. Any other value, including the old `"hot"`, → 400. Echoed as `memory_mode` in `GET …/dispatcher/automations`.

Optional per-automation `notify` `{connector, dedup, dedup_days, send_empty}` delivers each run's report through a communication connector (SPEC-007 §II.6). `connector` is required when `notify` is present and `dedup_days` must not be negative; otherwise → 400. Echoed as `notify` in `GET …/dispatcher/automations`. Sending `"notify": null` on update clears delivery.

Optional per-automation `skip_if_busy` (bool, default false): a scheduled fire that cannot start immediately is skipped instead of queued, and a run preempted by a chat is dropped instead of restarted (SPEC-007 §V). Manual triggers ignore it. Echoed as `skip_if_busy`.

Optional per-automation `journal` (bool, default false): the automation keeps notes between runs — its runs see them in the task and rewrite them with the `automation_journal` tool (SPEC-007 §II.7). Echoed as `journal`; read or clear the text with the journal endpoints below.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/admin/api/dispatcher/workspaces/{workspace}/automations` | Create automation |
| PUT | `/admin/api/dispatcher/workspaces/{workspace}/automations/{automation}` | Update automation |
| DELETE | `/admin/api/dispatcher/workspaces/{workspace}/automations/{automation}` | Delete automation |
| GET | `/admin/api/dispatcher/workspaces/{workspace}/automations/{automation}/journal` | Read the automation's learning journal (`{"journal": "..."}`; empty when none) |
| DELETE | `/admin/api/dispatcher/workspaces/{workspace}/automations/{automation}/journal` | Clear the learning journal |

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
| GET | `/admin/api/memory/{workspace}` | List memories. `?type=` filters by type; `?hot=true` returns exactly the facts every run carries (the injection query: workspace + `global`), uncapped; `?unused=true` returns facts never sent to a model nor returned by search. Entries include `priority`, `injected_count`, `searched_count`, `last_used_at` |
| POST | `/admin/api/memory/{workspace}` | Add a fact: `{content, title?, scope?, mode?, keep?, priority?}` (defaults workspace / on_demand / permanent; routed by the agent tool's own table). 201 with the entry; 400 invalid (empty / over 2000 chars / bad combination / workspace `global`); 409 duplicate; 415 unless `Content-Type: application/json`. Source is `operator` |
| GET | `/admin/api/memory/{workspace}/injection-preview` | What a run receives from memory: `{block, chars, tokens_estimate, budget_chars, context_budget_chars, operator_chars, over_budget, model, budget_resolved, included[], cut[]}`. `?model=` sizes it from that model's resolved context budget (404 if unknown); without it the fallback budget is used and `budget_resolved` is false. `?message=` adds `recall: {block, ids[]}` — the facts that chat message would recall (SPEC-004 §4.1). Built by the same code the agent runs |
| GET | `/admin/api/memory/{workspace}/export` | Download the workspace's facts plus the user-wide facts as `memory-{workspace}.md` (`text/markdown`, attachment). Does not count as use |
| POST | `/admin/api/memory/{workspace}/import` | Add the facts of a markdown file: `{markdown}` → `{created, skipped, issues:[{line, message}]}`. Same validation/routing as `POST …/memory/{workspace}`; source `import`; duplicates skipped; max 200 facts; 400 when no facts / bad workspace; 415 unless JSON |
| GET | `/admin/api/memory/{workspace}/notes` | The operator's MEMORY.md: `{global, workspace, max_chars}` |
| PUT | `/admin/api/memory/{workspace}/notes` | Replace one notes file: `{scope: "workspace"\|"global", content}`. Blank content removes the file. 400 over `max_chars` (6000) / bad scope / bad workspace; 415 unless JSON; 503 if notes are not configured |
| POST | `/admin/api/memory/{workspace}/search` | Search memories |
| GET | `/admin/api/memory/{workspace}/{id}` | Get memory |
| PUT | `/admin/api/memory/{workspace}/{id}` | Update memory `{title, content, hot?, priority?}` (priority 0 low / 1 normal / 2 high, else 400 and nothing is written). Tags are preserved; `hot: true/false` promotes/demotes the fact in the every-run set. 404 if missing |
| DELETE | `/admin/api/memory/{workspace}/{id}` | Delete memory |
| DELETE | `/admin/api/memory/{workspace}` | Clear workspace memories |

`POST /admin/api/conversation/sessions/{workspace}/{session}/memory-review` — ask the chat's model, once, which facts in a saved conversation are worth remembering (SPEC-004 §II.8). No body. 200 `{suggestions:[{content, scope, mode, duplicate}]}` (at most 5; `duplicate` marks a fact memory already holds; an unusable model reply is an empty list); 404 unknown session; 503 `The model is busy…` when the interactive run lane is not free within the 120 s limit; 502 when the model call fails. It reads only the user's and assistant's text, **saves nothing**, and never changes the conversation: the UI saves the facts the operator approves through `POST /admin/api/memory/{workspace}`. An explicit "remember …" in a chat message is saved by the backend itself before the run and reported on the turn's run record as `memory_saved`.

User-wide facts (scope `user`) are stored under the reserved workspace `global`; every endpoint above accepts `global` as `{workspace}` to list, edit, promote or delete them (creating always targets a real workspace).

---

## Error Format

All error responses use:

```json
{ "error": "description of what went wrong" }
```

HTTP status codes: 400 (bad request), 404 (not found), 409 (conflict), 500 (internal error), 503 (service unavailable).
