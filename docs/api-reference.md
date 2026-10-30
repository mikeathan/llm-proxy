# API Reference

## Base URL

All API endpoints are served from the server root (default `http://127.0.0.1:4001` for a new install, `http://0.0.0.0:4001` for one that predates 0.20.0).

### Sign-in (since 0.18.0; compulsory since 0.22.0)

Sign-in is compulsory. Until the operator sets a passphrase, every `/admin/api/` route except the four marked *open*
answers `401 {"error":"setup_required"}` (the UI then shows only the setup page); **an install that had none before
0.22.0 is locked until `llm-proxy auth setup-code` and the setup page have been used**. Once one exists,
every `/admin/api/` route except the four marked *open* needs either the session cookie `llmproxy_session`
(`HttpOnly; SameSite=Strict; Path=/admin`, `Secure` over HTTPS) or `Authorization: Bearer lp_…` with an **admin** script
token; otherwise `401`. `/v1/*` is open until the first **inference** token is made, then needs `Authorization: Bearer lp_…`
(an admin token also works). The UI shell and the webhooks are not affected. Set up on the server, as the server's OS user:
`llm-proxy auth setup-code` (a one-time code, 15 minutes; the command acts on the data folder the server would use with the same flags and environment (`--data DIR`, `LLM_PROXY_HOME`, `.env`, else `~/.config/llm-proxy`), so `go run . auth setup-code` and the installed service resolve alike; run as root it first becomes the account that owns that folder, so the file is readable by the server, and with a systemd unit installed and no folder named root uses the service's folder, so on a service install `sudo llm-proxy auth setup-code` works from anywhere; every command prints the sign-in file it used and says whether a live server reads that folder, using `server.json` which a running server keeps in its data folder, then enter it with a passphrase on `/admin/setup`;
`llm-proxy auth status` and `llm-proxy auth reset` (removes the passphrase, sessions and tokens and locks the admin API again; no web route does this). Scripts and evals against a fresh server need a token: set up, make an admin token under *Script access*, and export it as `LLM_PROXY_TOKEN`.

| Method | Path | |
|---|---|---|
| GET | `/admin/api/auth/status` | *open.* `{set_up, setup_open, authenticated, elevated, encrypted}` (`set_up`: a passphrase exists) and nothing else |
| POST | `/admin/api/auth/setup` | *open.* `{code, passphrase}`: sets the first passphrase and signs in. `403` wrong code (five wrong codes cancel it), `409` no code or already set up, `400` weak passphrase (12+ characters, not a common one) |
| POST | `/admin/api/auth/login` | *open.* `{passphrase}` sets the cookie. `401` wrong; `429` with `Retry-After` after five wrong tries from one address (or 30 an hour overall) |
| POST | `/admin/api/auth/logout` | *open.* Clears the cookie and ends the session (`204`) |
| POST | `/admin/api/auth/reauth` | `{passphrase}`: opens a 5-minute window for the destructive actions and **replaces the cookie** |
| GET / DELETE | `/admin/api/auth/sessions[?id]` | List where you are signed in (never the cookie value); end one |
| POST | `/admin/api/auth/sessions/revoke-all` | Sign out everywhere, this browser too |
| POST | `/admin/api/auth/passphrase` | `{old, new}`; ends every other session |
| GET / POST / DELETE | `/admin/api/auth/tokens[?id]` | List script tokens (no secret); make one `{name, scope: "admin"\|"inference"}` (the secret is in this answer only, and this needs the passphrase again); revoke |
| GET, PUT | `/admin/api/auth/session-length` | How long a sign-in lasts: `{idle_hours, absolute_days, max_idle_hours, max_absolute_days, default_idle_hours, default_absolute_days}`. `PUT {idle_hours, absolute_days}` saves `server.auth` (whole numbers, 1 to the ceiling, idle no longer than absolute); it needs the passphrase again (step-up) and applies to the next request. |
| GET | `/admin/api/auth/exposure` | Where the server listens and what protects it: `{listening{address,network}, sign_in_required, encrypted, inference_key_required, webhooks{inbound,with_secret}, sandbox{enabled,filesystem}, attention[{id,text,fix}]}`. No secret. |
| GET | `/admin/api/auth/events` | The last 20 sign-in events (never a secret) |

**Step-up.** These writes answer `403 {"error":"reauth_required"}` until the session has re-entered the passphrase in the
last 5 minutes (the UI asks and repeats the request): `POST /system/wipeout`, `/system/factory-reset`,
`/system/clear-runtime-data`, `PUT /host`, every write under `/secrets/` and `/mcp`, and `POST /auth/tokens`; and `PUT /config` or
`PUT /system` **only when the update changes a program the server runs**: the model server's arguments, environment or path
(`default_args`, `environment`, `providers.local`) or the GPU program (`gpu_binary`). A value equal to the current one does not
count, so an ordinary settings save never asks. A script token cannot confirm a passphrase, so it gets `403` on all of these. Lifetimes: a session ends after 24 hours unused or 14 days in all (`server.auth.idle_hours` and `server.auth.absolute_days` in `settings.yml`; 0 or unset is the default, at most 720 hours and 90 days, and idle never exceeds absolute; a change applies without a restart).
Everything the sign-in keeps (passphrase as argon2id, tokens and session ids as SHA-256) is in `auth.json` in the data root,
mode 0600, and survives a factory reset.

### Browser protection on `/admin/` and `/admin/api/` (since 0.17.0)

There is still no sign-in (plan `admin-api-authentication`); these rules stop pages that are not the UI. `/v1/*` and the
webhook path are not affected.

- **Host:** an IP address, `localhost`, this machine's name (and `<name>.local`) and any name in `server.allowed_hosts`
  (`settings.yml`) are served; any other name gets `421`. Set `allowed_hosts` when the UI is reached through a reverse
  proxy or a custom DNS name.
- **Encrypted (since 0.20.0):** the session cookie is `Secure`, and `/auth/status` reports `"encrypted":true`, when the
  request arrived over TLS or came from an address in `server.trusted_proxies` (`settings.yml`; addresses or ranges,
  empty by default) with `X-Forwarded-Proto: https`. The header is ignored from any other address. From a listed proxy
  `X-Forwarded-For` also names the client for the sign-in lockout and log (read from the right, skipping listed proxies).
- **`GET /admin/api/system`** returns the model server's `environment` with the value of each secret-looking variable
  (name contains TOKEN, SECRET, KEY, PASSWORD, PASSWD or CREDENTIAL) as `********`; a save that sends the mask keeps the
  stored value. See
  [service_setup.md](service_setup.md#encrypted-access-a-tls-front-end).
- **Cross-site:** a request to `/admin/api/` that the browser marks `Sec-Fetch-Site: cross-site` or `same-site` gets `403`,
  whatever its method; writes with a foreign `Origin` and no Fetch-Metadata also get `403`. Scripts and `curl` send neither
  header and pass.
- **Headers:** `X-Frame-Options: DENY`, `Content-Security-Policy: frame-ancestors 'none'`, `X-Content-Type-Options: nosniff`,
  `Referrer-Policy: no-referrer` on the UI and the API; `Cache-Control: no-store` on API answers.
- The workspace live stream no longer sends `Access-Control-Allow-Origin: *`; `POST /admin/api/runtime/processes/{pid}/stop`
  only stops a model server the Models page lists (`403` otherwise).

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
  "profile": "",
  "agent": "",
  "use_helpers": false
}
```

`profile` is optional: a prompt profile id (`GET /admin/api/profiles`) for this message, which the conversation then keeps; empty keeps the conversation's own, else the workspace's, else General. An unknown id is refused with `400`.

`agent` is optional: a profile for **this message only** (a turn command's, `/research` or `/agent`); it outranks `profile` and the conversation's for that turn and is never stored. An unknown id is refused with `400`.

`use_helpers` is optional: ask the assistant to hand the independent parts of the message to helpers; ignored when the turn's profile names none or helpers are not set up (`GET /admin/api/helpers`).

`model` is optional: a registry model for this message. Empty runs the primary model (with the fallback if the primary cannot be reached). A named model is used as is, with no fallback, and an unknown name is refused with `400`.

**Response:** `202 Accepted` — the turn runs detached from the request (a refresh or a closed tab does not stop it; `POST /admin/api/conversation/cancel` does) and is observed over the SSE event bus (`/admin/api/dispatcher/workspaces/{workspace}/live?channel=assistant`). The answer is in the session afterwards.

```json
{ "status": "running", "conversation_id": "conv_20260101120000" }
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
| GET | `/admin/api/profiles` | Every prompt profile (an "agent" in the UI; built-in and the user's files) with its flags and what it is the default for; never its instructions (SPEC-012) |
| GET | `/admin/api/profiles/{id}` | One profile with its instructions, the profiles that list it as a helper, and whether a user file replaces a built-in |
| PUT | `/admin/api/profiles/{id}` | Create or replace one of the user's own profiles; `400 {error, field}` when invalid, `409` for a built-in |
| DELETE | `/admin/api/profiles/{id}` | Delete the user's own profile file (a replaced built-in comes back); `409` for a built-in |
| GET | `/admin/api/profile-schema` | The settings of a profile with their help text, for the Settings → Agents form |

A workspace's `assistant_profile` (workspace config `PUT`) and an automation's `profile` name a profile; an unknown id is `400`.

### Network scan

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/netaudit/scope` | The allowed scan ranges (CIDR, device count), `allow_unconfined`, and the networks this machine is on as suggestions |
| PUT | `/admin/api/netaudit/scope` | Replace the ranges (`entries`), set `allow_unconfined` and/or `device_timeout_minutes` (0 standard, 5 or 7: how long a scan may spend on one device); a field left out stays; a refused entry or a time that is not offered is a `400` and nothing is saved |
| GET | `/admin/api/netaudit/readiness` | `{ready, requirements[], notices[]}`: what is missing before a scan can run, each with the one fix; `notices` are improvements that never stop an audit (`vuln_data_missing`: the vulnerability data is not, or only partly, downloaded; `vuln_data_old`: it is complete but more than 60 days old; the fix of both opens the `network-scan` settings section) and do not change `ready` |
| GET | `/admin/api/netaudit/devices` | The devices seen on the network by MAC address, those needing a look first: `devices[]` (`mac`, `ip`, `name`, `first_seen`, `last_seen`, `approved`, `randomized`, `new`), `gateway` (`ip`, `mac`, `changed`), `baselined`, `updated_at` |
| PUT | `/admin/api/netaudit/devices/{mac}` | Body `{name}` names the device (and approves it; at most 40 characters, no control characters) or `{approved: true}` approves it. `400` for a bad MAC or name, `404` for an unknown device. Answers with the list |
| DELETE | `/admin/api/netaudit/devices/{mac}` | Remove a device from the list (it returns as new if it is still there). Answers with the list |
| POST | `/admin/api/netaudit/devices/gateway/confirm` | **Step-up.** Accept the router's current hardware address as the right one (it silences the alarm for a spoofed router). Answers with the list |
| GET | `/admin/api/netaudit/decoys` | The decoy services: `enabled`, `listening` ports, a plain `problem` when a port could not be opened, `ports` (`number`, `name`) and `touches` (`source`, `port`, `first`, `last`, `count`; no bytes are kept) |
| PUT | `/admin/api/netaudit/decoys` | **Step-up.** Body `{enabled}`. Switching on opens listening sockets on this computer (fixed unprivileged ports, off by default); switching off closes them. Answers with the state |
| GET | `/admin/api/netaudit/presence` | The new-device check: `enabled`, `interval_minutes`, `allowed_intervals` (5, 10, 15, 30), `last_check`, and a plain `problem` when it cannot look (for example off Linux) |
| PUT | `/admin/api/netaudit/presence` | Body `{enabled, interval_minutes}` (0 = 10). `400` for an interval that is not offered |
| GET | `/admin/api/netaudit/vulndata` | The vulnerability data card: `state` (`idle`, `running`, `stopped`, `done`), the `file` being fetched, `files_done` / `files_total`, `bytes_done` / `bytes_total` of that file, a plain `reason` when stopped, and what the index holds (`nvd` with `years`, `kev`). Poll it about once a second while `running` |
| POST | `/admin/api/netaudit/fixes` | "I've done this": body `{workspace_id, automation, key, note?}` records that the operator applied the fix for one finding of the automation's latest scan (only a check finding with a fix guide; `404` if the scan does not show it, `400` for an open port or a note over 200 characters). It changes nothing about the finding, the comparison between scans or any notification; the next scan decides whether the fix worked. Operator action, behind the admin sign-in (low impact: a record never hides a finding) |
| DELETE | `/admin/api/netaudit/fixes?workspace_id&key` | Take a fix record back (`204`, or `404` when there is none) |
| DELETE | `/admin/api/netaudit/vulndata` | **Step-up.** Delete all the downloaded vulnerability data (a running download is stopped first and nothing of it is kept); answers `200` with the empty status. Audits then skip the known-vulnerability check until it is downloaded again. The Settings card asks first |
| POST | `/admin/api/netaudit/vulndata/download` | Start the one-off download (the exploited-vulnerabilities list and the last five NVD years, from two fixed hosts, through the guarded network client): `202` with the status; fetches only the files not yet held, so it also resumes a stopped download, and refreshes everything only when all are held and the oldest source is more than 60 days old (when they are held and recent nothing starts and the answer is `200` with the status); `?restart=1` fetches every file again whatever is held (what is held stays until its file is replaced); `409` while one runs. Operator action, behind the admin sign-in |
| POST | `/admin/api/netaudit/vulndata/cancel` | Stop a running download; finished files stay |
| POST | `/admin/api/netaudit/vulndata/import?source=nvd&year=YYYY` or `?source=kev` | Load one data file the operator fetched elsewhere, for a host that cannot download (**step-up**: a freshly confirmed passphrase, a token is refused). The raw file is the body (NVD's yearly file as gzip or plain JSON; CISA's list as JSON), capped at 256 MiB; `year` is required for `nvd` and must be one of the five years the data covers. Answers `200` with the status. `400` when the source, year or file is not usable (nothing of the file is repeated), `409` while a download or import runs |
| GET | `/admin/api/netaudit/findings?workspace_id&automation` | The automation's latest scan: findings (each with `key`, `new`, `changed` + `previous_version`, `accepted`; a `known_vulns` finding also carries `cves`, `exploited` and `cve_scores`, the id sets and each flaw's base score; a check finding also carries `guide` (`steps`, `commands[]`, `risk`, `undo`: fixed text) and, when the operator marked it fixed, `fix_state` (`marked`: no scan since; `still_present`: a later scan still shows it), `fix_marked_at` and `fix_note`; `resolved` entries carry `fixed_at` when the operator had marked them), what was resolved, `unscanned` devices (the scan ran out of time on them; their earlier findings are kept) and `devices_up`. A check finding (mode `checks`) has `check` (for example `smb_v1`), `severity` (low, medium, high, critical) and `evidence` (the code's own sentence) besides the port; open ports have `check: open_port`, `severity: info` |
| GET / POST / DELETE | `/admin/api/netaudit/acks` | List, accept (`key`, `note`, `days`) or take back (`?key`) an accepted finding |
| GET / PUT / DELETE | `/admin/api/netaudit/expectations` | What each device should offer: `GET ?workspace_id` lists `{device, ports[], note, set_at}`; `PUT {workspace_id, device, ports[], note?}` sets or replaces one (`ports: []` means nothing should answer; `400` for a device that is not an IPv4 address inside the allowed scan networks, a port outside 1-65535, more than 64 ports or 256 devices, or a note over 200 characters); `DELETE ?workspace_id&device` takes it back (`204`, or `404`). The next scan reports `unexpected_port` (medium) for an open port not on the list and `expected_port_closed` (low) for a listed port that did not answer, skipping a device the scan could not finish. Nothing is probed for it. Operator action, behind the admin sign-in (it can only hide or reveal findings, never widen a scan) |
| GET / POST | `/admin/api/netaudit/deep` | The **deep check** of one device (Constitution II.3, "Intrusive runs"). `GET ?workspace_id` lists the last check of each device, newest first: `{device, at, ports[{port, tls_answered, ssh_answered}], findings[]}` (each finding has `check`, `check_title`, `severity`, `evidence`, `fix`, `guide`). `POST {workspace_id, automation, device}` runs one and answers when it is done (up to six minutes): it takes **nothing else** from the caller (the ports are the device's open TCP ports in that audit's latest scan, at most 16; the scripts are `ssh-auth-methods` and `ssl-enum-ciphers`, nothing more), refuses an address outside the scan scope, and uses nmap only, at `-T2` and 50 packets a second. It is a **step-up route**: a session whose passphrase was confirmed a moment ago, never a script token (`403`). A device is left alone for ten minutes after a check (`429` with `Retry-After`); `409` when another scan is running or the last scan found no open port on the device; `400` for anything that is not one IPv4 address in scope; `503` without nmap or with scanning off; `504` when it ran out of time (nothing is stored). Findings: `tls_weak_protocol`, `tls_weak_cipher`, `ssh_password_login`. The audit's next report carries them, dated, for 90 days while the device is up and the port still open |

### Providers

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/providers/models?provider={p}&api_key_name={k}` | List remote models |
| GET | `/admin/api/providers/manifests` | List provider manifests |
| POST | `/admin/api/providers/test` | Test connection: body `{provider, api_key?, api_key_name?, base_url?}` (a POST with a body, not a query string, since it can send a stored key to a URL; plan admin-api-authentication E5) |

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

Optional per-automation `scan` (a network audit's settings, `models.ScanSettings`): `targets` (addresses, ranges or names; empty = every network allowed in Settings), `depth` (`quick` or `thorough`, default thorough), `ports` (empty = the common ports) and `checks` (bool, default on). The server applies them when the audit runs and the scan tool ignores what the model passes for them; an automation on the Security auditor with no `scan` gets the defaults. A bad value is a `400`. `GET …/automations` echoes the effective settings (depth and checks filled in) and omits `scan` for an automation that is not an audit.

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

`POST /api/v1/webhooks/{connector_name}` — External platforms post here. Not covered by admin sign-in: each connector has its own webhook secret (`webhook_token`, sent as `X-Webhook-Token` or, from Telegram, `X-Telegram-Bot-Api-Secret-Token`), and a request without it, or any request to a connector with no secret, gets `401`.

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
| GET | `/admin/api/reminders/events` | `?after=<unix ms>` → `{cursor, items}`: the reminder outcomes saved after `after` that the web UI should be told about, newest first, at most 20; each `{id, message, event (due\|late\|partial\|failed\|missed), at (unix ms), channel}`. `cursor` is always the server's newest event time (0 when there is none), to send back as `after`. With no `after`: `items` is empty and only the cursor is returned. A delivery to one named connector is not listed; a failure or a miss always is (SPEC-011 1.6). 400 if `after` is not a non-negative integer |

| POST | `/admin/api/reminders` | Create: `{message, fire_at? \| cron? (+ until?), connector, timezone?}` (same shape and rules as the edit below; `timezone` defaults to the owner's setting, else UTC). 201 with the item; 400 refused (message names why) or bad JSON/time/zone; 415 unless JSON |
| PUT | `/admin/api/reminders/{id}` | Edit an open or paused reminder, or **reschedule** a finished one (sent / missed / failed become scheduled again; a paused one stays paused; a cancelled one is a 404): `{message, fire_at?, cron?, until?, connector}` — `fire_at` (RFC 3339) for a one-off, or `cron` (+ optional `until`) for a recurring one; `connector` is `""` (automatic), `in-app` or a configured connector type. Same rules as creation; its timezone is kept. Returns the item (as in `GET`). 400 refused (message names why, nothing changes) or bad id/JSON; 404 unknown or no longer open; 415 unless JSON |
| POST | `/admin/api/reminders/{id}/pause` | Silence a repeating, scheduled reminder (`status: paused`). Returns the item. 400 if it is a one-off or not scheduled; 404 unknown |
| POST | `/admin/api/reminders/{id}/resume` | Un-silence a paused reminder at its next occurrence after now. 400 if not paused or its end date has passed |
| POST | `/admin/api/reminders/{id}/skip` | Skip a repeating reminder's next occurrence. 400 if it is a one-off, not scheduled, or this is its last occurrence |
| DELETE | `/admin/api/reminders/{id}` | Delete a reminder in any status (an open one is never sent). 204; 404 unknown; 400 bad id |

Each item also carries `connector` (the channel as chosen: `""` automatic, `in-app`, or a type) and `until`. Reminders are created by the assistant's tools, by `/remind` (Commands API) and by `POST` above (SPEC-011).

## Commands API

Slash commands (`/remind`, SPEC-011 §2.11; `/research`, `/agent`, SPEC-012) answered in code, without the assistant. The zone is the owner's `reminders.timezone`, else the request's `timezone` (the browser's), else UTC. Always registered; the list is empty when reminders are unavailable.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/admin/api/commands` | `{commands: [{name, usage, summary, examples[]}]}` for the chat's command menu |
| POST | `/admin/api/commands/run` | `{message, timezone?, workspace_id?}` → `{handled, reply?, failed?, turn?, automation?}`. `workspace_id` is the chat's workspace, where `/run <automation>` starts the automation (`automation` names it; its result is on the Automations page). `handled: false` for text that is not a registered command (send it to the assistant). `failed: true` when the command was refused; `reply` then carries the reason and usage. A turn command (`/research`, `/agent`, SPEC-012) answers with `turn: {message, agent?, use_helpers?}`: send `message` to the assistant with those choices for that message only (`agent` in the chat request), after showing `reply` when there is one |
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
