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
  "exclude_tools": [],
  "model": "",
  "profile": ""
}
```

`profile` is optional: a prompt profile id (`GET /admin/api/profiles`) for this message, which the conversation then keeps; empty keeps the conversation's own, else the workspace's, else General. An unknown id is refused with `400`.

`model` is optional: a registry model for this message. Empty runs the primary model (with the fallback if the primary cannot be reached). A named model is used as is, with no fallback, and an unknown name is refused with `400`.

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

### Profiles

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/profiles` | Every prompt profile (built-in and the user's files) with its flags and what it is the default for; never its instructions (SPEC-012) |

A workspace's `assistant_profile` (workspace config `PUT`) and an automation's `profile` name a profile; an unknown id is `400`.

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
| GET/PUT | `/admin/api/config` → `reminders` | `reminders: {skip_connectors, timezone}` — `skip_connectors` are the connector types ticked off for reminders (every other enabled connector gets them, SPEC-011 §2.4); `timezone` an IANA name. Both optional; PUT saves them as given (SPEC-011); an unknown IANA `timezone` is a 400 and nothing is saved; omit `reminders` to leave it unchanged |
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
| GET | `/admin/api/dispatcher/automations` | List automations. Each carries its latest run's `last_output`, `last_error` and `last_warnings` (non-fatal problems such as a report that could not be delivered). A scheduled automation also carries `next_run_at` (RFC 3339, the server's clock, read from the scheduler's own entry); a manual one omits it |
| GET | `/admin/api/dispatcher/metrics` | Dispatcher metrics |
| POST | `/admin/api/dispatcher/schedule/preview` | Check a trigger and list its next runs. Body `{"trigger":{"type","value"},"count"?}` → `{valid, error?, timezone, next_runs[]}`; cron is five fields or a descriptor, no seconds; an invalid schedule is a 200 with `valid:false` |
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

## Reminders API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/reminders` | `{open, recent}` — scheduled reminders soonest first, and the 20 most recent finished ones (`done`, `missed`, `failed`). Each item: `{id, message, fire_at (RFC 3339 UTC), cron?, timezone, channel, status, last_error?}`; `channel` is the connector type it goes to or `in-app`. Registered only when reminders are available (database + connector registry) |

| POST | `/admin/api/reminders` | Create: `{message, fire_at? \| cron? (+ until?), connector, timezone?}` (same shape and rules as the edit below; `timezone` defaults to the owner's setting, else UTC). 201 with the item; 400 refused (message names why) or bad JSON/time/zone; 415 unless JSON |
| PUT | `/admin/api/reminders/{id}` | Edit an open or paused reminder, or **reschedule** a finished one (sent / missed / failed become scheduled again; a paused one stays paused; a cancelled one is a 404): `{message, fire_at?, cron?, until?, connector}` — `fire_at` (RFC 3339) for a one-off, or `cron` (+ optional `until`) for a recurring one; `connector` is `""` (automatic), `in-app` or a configured connector type. Same rules as creation; its timezone is kept. Returns the item (as in `GET`). 400 refused (message names why, nothing changes) or bad id/JSON; 404 unknown or no longer open; 415 unless JSON |
| POST | `/admin/api/reminders/{id}/pause` | Silence a repeating, scheduled reminder (`status: paused`). Returns the item. 400 if it is a one-off or not scheduled; 404 unknown |
| POST | `/admin/api/reminders/{id}/resume` | Un-silence a paused reminder at its next occurrence after now. 400 if not paused or its end date has passed |
| POST | `/admin/api/reminders/{id}/skip` | Skip a repeating reminder's next occurrence. 400 if it is a one-off, not scheduled, or this is its last occurrence |
| DELETE | `/admin/api/reminders/{id}` | Delete a reminder in any status (an open one is never sent). 204; 404 unknown; 400 bad id |

Each item also carries `connector` (the channel as chosen: `""` automatic, `in-app`, or a type) and `until`. Reminders are created by the assistant's tools, by `/remind` (Commands API) and by `POST` above (SPEC-011).

## Commands API

Slash commands (`/remind`, SPEC-011 §2.11) answered in code, without the assistant. The zone is the owner's `reminders.timezone`, else the request's `timezone` (the browser's), else UTC. Always registered; the list is empty when reminders are unavailable.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/commands` | `{commands: [{name, usage, summary, examples[]}]}` for the chat's command menu |
| POST | `/admin/api/commands/run` | `{message, timezone?}` → `{handled, reply?, failed?}`. `handled: false` for text that is not a registered command (send it to the assistant). `failed: true` when the command was refused; `reply` then carries the reason and usage |
| POST | `/admin/api/commands/preview` | `{message, timezone?}` → `{matched, command?, text?, error?}`: what the command would do with the text typed so far (nothing is stored), or why it would be refused |

## Memory API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/memory/{workspace}` | List memories. `?type=` filters by type; `?hot=true` returns exactly the facts every run carries (the injection query: workspace + `global`), uncapped; `?unused=true` returns facts never sent to a model nor returned by search. Entries include `kind`, `why`, `expires_at`, `priority`, `injected_count`, `searched_count`, `last_used_at` |
| POST | `/admin/api/memory/{workspace}` | Add a fact: `{content, title?, scope?, mode?, keep?, priority?, kind?, why?, expires?}` (`kind` user/feedback/project/reference, `why` one line ≤ 200 chars, `expires` the last useful day `YYYY-MM-DD` UTC and not in the past; a session fact gets `memory.retention_days` when none is given) (defaults workspace / on_demand / permanent; routed by the agent tool's own table). 201 with the entry; 400 invalid (empty / over 2000 chars / bad combination / workspace `global`); 409 duplicate; 415 unless `Content-Type: application/json`. Source is `operator` |
| GET | `/admin/api/memory/{workspace}/injection-preview` | What a run receives from memory: `{block, chars, tokens_estimate, budget_chars, context_budget_chars, operator_chars, notes_cut_chars, model, budget_resolved, included[], cut[]}`. `?model=` sizes it from that model's resolved context budget (404 if unknown); without it the fallback budget is used and `budget_resolved` is false. `?message=` adds `recall: {block, ids[]}` — the facts that chat message would recall (SPEC-004 §4.1). Built by the same code the agent runs |
| GET | `/admin/api/memory/{workspace}/export` | Download the workspace's facts plus the user-wide facts as `memory-{workspace}.md` (`text/markdown`, attachment). Does not count as use |
| POST | `/admin/api/memory/{workspace}/import` | Add the facts of a markdown file: `{markdown}` → `{created, skipped, issues:[{line, message}]}`. Same validation/routing as `POST …/memory/{workspace}`; source `import`; duplicates skipped; max 200 facts; 400 when no facts / bad workspace; 415 unless JSON |
| GET | `/admin/api/memory/{workspace}/notes` | The operator's MEMORY.md: `{global, workspace, max_chars}` |
| PUT | `/admin/api/memory/{workspace}/notes` | Replace one notes file: `{scope: "workspace"\|"global", content}`. Blank content removes the file. 400 over `max_chars` (6000) / bad scope / bad workspace; 415 unless JSON; 503 if notes are not configured |
| POST | `/admin/api/memory/{workspace}/search` | Search memories |
| GET | `/admin/api/memory/{workspace}/{id}` | Get memory |
| PUT | `/admin/api/memory/{workspace}/{id}` | Update memory `{title, content, hot?, priority?, kind?, why?, expires?}` (a typing field left out is kept; `""` clears `why`/`expires`; bad values are 400; priority 0 low / 1 normal / 2 high, else 400 and nothing is written). Tags are preserved; `hot: true/false` promotes/demotes the fact in the every-run set. 404 if missing |
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
