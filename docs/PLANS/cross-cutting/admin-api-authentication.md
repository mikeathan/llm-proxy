---
status: proposed
date: 2026-09-30
last_reviewed: 2026-10-29
related_specs: [SPEC-003, SPEC-006, SPEC-007, SPEC-009]
constitution_references: [I.4, III.6, V.1]
evidence: docs/audits/2026-09-30-platform-scan.md (S1–S5, S9); admin-api-authentication-design.md § Evidence ledger (2026-10-29)
---

# Admin / API Authentication and Control-Plane Hardening

**Status:** proposed, reviewed 2026-10-29. The design, evidence ledger, threat model and UI are in
[admin-api-authentication-design.md](admin-api-authentication-design.md); this file keeps the phases and decisions. The
owner's four decisions of 2026-10-29 are in [§ Review 2026-10-29](#review-2026-10-29). Phases 0 and 1 need no
decision and change no contract; they can start immediately.

## Why this plan exists

The agent sandbox is well built (see `agent-os-sandboxing.md`). The door in front of it is not: **no
request to this server is authenticated.** Verified 2026-09-30 (`docs/audits/2026-09-30-platform-scan.md`
S1–S4):

- `internal/transport/http/router.go` adds only panic recovery; ~100 routes in `internal/app/routes.go`
  are registered with no auth.
- The default bind is `0.0.0.0` (`internal/app/app.go:92-98`; `docs/api-reference.md:5`).
- So anyone who can reach the port can: kill any PID the service user may signal
  (`process_handlers.go:291-314` → `process.Kill`), change `llama_server_binary`/args and start the model
  server — **arbitrary code execution as the service user** (`app_context_system.go:171`,
  `providers/local_provider.go:176`), switch off the host sandbox (`PUT /admin/api/host`), write workspace
  files, start agents that use the terminal tool, and wipe or restart the service.
- A web page the operator happens to visit can also reach it: no `Host`/`Origin` check (DNS rebinding), and several
  destructive handlers read no request body, so a plain cross-site `<form method=POST>` (a CORS "simple request")
  fires them — **`/admin/api/system/wipeout` (deletes the data root, workspaces and secrets), `/system/factory-reset`,
  `/system/clear-runtime-data`, `/system/restart`, `/stop`, `/runtime/processes/{pid}/stop`**
  (`handlers/system_handlers.go:132-222`, `process_handlers.go:48-100`); `start` also works via `?name=`. The
  workspace SSE answers `Access-Control-Allow-Origin: *` (`dispatcher_handlers.go:354`). **This makes Phase 1 item 2
  (Origin check) urgent and independent of the login decisions — it is a day of work that removes one-click data loss.**

Two docs describe auth that does not exist: SPEC-007 §V.1 ("behind admin auth") and the webhook doc
("No admin auth" is accurate only for webhooks). SPEC-003 §I says the UI has "no account model" — that
was a deliberate single-operator-on-a-LAN decision, and this plan is the point where it needs revisiting
because the same server now runs agents with a shell and is being reached over the LAN from another
machine (the Mac → remote llama host setup in `2026-08-28-ops-performance-review.md`).

## What was decided before (and why it matters here)

| Earlier decision | Consequence for this plan |
|---|---|
| SPEC-003 §I: "runs on a LAN, has no account model" | Single operator. Do **not** build users/roles. One operator credential + machine credentials is enough. |
| Constitution §I.4: inbound webhooks are exempt from the network guardrails and use plain `net/http` | Webhooks stay a separate public surface; they get their own mandatory secret (Phase 1), not the operator login. |
| SPEC-007 §V.1.1: queue-cancel key is `crypto/rand` because "cancel is unauthenticated on the public surface" | A stopgap that assumed auth would come later. Keep the key; add auth on top. |
| Sandbox plan D3: Linux prod runs the service as a dedicated unprivileged user; macOS/dev runs as the operator | Bounds the blast radius of S2 but does not remove it (workspaces, secrets, model binary). |
| Constitution §III.6: `master.key` sits next to `secrets.json` | Acceptable only while the control plane is not reachable by strangers. |
| Remote clients already send `Authorization: Bearer <configured key>` to their upstream (`provider_openai_compatible.go:341-361`) | A Bearer check on `/v1/*` needs **no client change**: set the same key on the remote proxy and on the Mac's provider entry. |

Nothing was ever tried for auth, so there is no failed approach to avoid. The closest near-miss is the
network default-off migration in the sandbox plan (absent key = legacy behaviour + a one-time prompt);
this plan reuses that pattern so an upgrade never silently locks the operator out or silently leaves the
door open.

## Decisions for the user

Recommendation first in each row.

| # | Decision | Options | Recommendation and reason |
|---|---|---|---|
| A1 | Who is authenticated? | (a) one operator token + optional inference keys; (b) per-user accounts; (c) mTLS/OIDC | **(a).** Single operator is the stated product model; (b)/(c) add a user store and lifecycle nobody asked for. |
| A2 | How does the browser UI authenticate? | (a) login page → `HttpOnly; SameSite=Strict` session cookie; (b) token in `localStorage` sent as `Authorization`; (c) HTTP basic | **(a).** `EventSource` (the SSE the UI depends on) cannot set headers, and a cookie also defeats the body-less-POST CSRF (S3). (b) breaks SSE and exposes the token to XSS. |
| A3 | Should `/v1/*` (inference) require a key? | (a) always; (b) only when the operator turns it on; (c) never | **(b), default on for new installs.** Existing remote setups keep working until the operator sets a key; see A5. |
| A4 | Default bind for **new** installs | `127.0.0.1` vs `0.0.0.0` | **`127.0.0.1` for fresh installs, never changed on upgrade.** A remote llama host must opt in to LAN exposure, and only after auth exists (Phase 3). |
| A5 | Upgrade behaviour when auth is not yet configured | (a) stay open + persistent warning banner; (b) refuse to start; (c) auto-generate a token and print it | **(a) for loopback-bound, (c)+banner for non-loopback-bound.** Never (b): a service that will not boot after `git pull` is worse than a warning. Mirrors the `sandboxing.network` undecided-key migration. |
| A6 | Constitution change | add a new law (e.g. §I.5 "The control plane is authenticated; unauthenticated surfaces are an enumerated allowlist") | **Yes.** Per Constitution §V.1 an invariant that would be violated by the status quo must be formalised, not just coded. |
| A7 | Is TLS in scope? | terminate in-process vs. document reverse proxy | **Document a reverse proxy; out of scope for code.** The cookie must be `Secure` when the request is HTTPS (or `X-Forwarded-Proto`), nothing more. |

## Review 2026-10-29

A second pass over the code (evidence ledger E1–E18 in the design file) changed this plan in four ways.

**Owner decisions, 2026-10-29** (these settle A1, A2 and A5 in part, and add three):

| Decision | Choice |
|---|---|
| Credentials (A1) | A passphrase for the UI (argon2id) and **named, revocable script tokens** (SHA-256, shown once, scope `admin` or `inference`). Not one token for everything. |
| Step-up | The most destructive actions ask for the passphrase again within 5 minutes (list in the design, §4.6). |
| First run (A5) | A **one-time setup code** made on demand by `llm-proxy auth setup-code` and entered in the UI. Nothing is printed to a log. Upgrades stay open with a persistent banner until set up. |
| Default bind (A4) | Fresh installs listen on loopback only; existing installs are never changed. |

**Findings the first plan missed** (full list in design §3): a stored provider key can be sent to a caller-chosen URL by
a **GET** (`providers/test`, E5/E6), which Origin checks that skip safe methods do not stop; Phase 1 protects only
against browsers, so the agent case (S6b) stays open until Phase 2; the "loopback listener always accepts set-token"
recovery contradicts the design rule below and is replaced by an OS-user CLI; printing a generated token leaks it to
the journal and possibly `app-logs`; in-memory sessions end at every self-restart; the frontend has 33 raw `fetch`
calls and three `EventSource` users outside any single 401 handler; the eval and shell scripts have no credential;
clickjacking, the plain-HTTP LAN and the macOS no-file-jail case are not covered.

## Design rule: loopback is not trusted

Do **not** exempt `127.0.0.1` from authentication. The agents this server runs can reach loopback:
a `networkOn` shell is not jailed from it (SPEC-006 §II.7.5), and the agent network guard currently lets
`0.0.0.0`/`::` through (audit S6, fixed in `sandbox-and-egress-residuals.md`). A prompt-injected run that can
call the admin API can turn the sandbox off. Consequences for this plan:

- The operator token and session secrets must never enter the agent's environment (the shell env allowlist
  already filters; add a test that `prepareShellEnv` output contains no auth material).
- The middleware authenticates every request regardless of source address; "localhost means trusted" shortcuts
  are not allowed.
- Agent child processes run with no cookie jar; the token is only ever in the operator's browser and in
  `settings.yml` as a hash.

## Phases

Each phase ends with its verify command green. Order: de-risk and prove first; decisions gate Phase 2.

### Phase 0 — Characterise before changing (no behaviour change)

**Why first:** the audit labels S5 an assumption, and "which routes are public" must become a machine-checked
fact before anything is wrapped.

1. **Route classification table test.** In `internal/app`, a test that walks every registered route and
   asserts each is in an explicit table: `public` (static UI shell, `/api/v1/webhooks/*`, health/version if
   kept), `inference` (`/v1/*`), or `admin` (everything else). An unclassified route fails the test. This
   is the "default deny" guard that stops the next route being added open by accident.
   - Verify: `cd backend && go test ./internal/app/ -run TestRouteClassification -count=1` → FAIL first (no
     table), PASS after.
2. **Exploit-shaped tests** that pass today and will invert later: unauthenticated `POST /admin/api/runtime/processes/{pid}/stop`
   for a PID that is not ours; `Host: evil.example` request; body-less cross-origin `POST`.
3. **Prove or kill S5** (key exfiltration through a credential `base_url`): a test with a recording
   `httptest` upstream; configure a credential whose `base_url` points at it; call the provider test/model-list
   path; assert whether the `Authorization` header carries the real key. If yes, S5 becomes a Phase 1 item.
   - Verify: `cd backend && go test ./internal/core/llm/... ./internal/transport/... -run 'S5|BaseURLKey' -count=1`.

Acceptance: tests merged, route table exists, S5 is resolved to *confirmed* or *not reproducible* in this doc.

### Phase 1 — Zero-config hardening (no login, no contract change)

All of this is safe for every existing deployment.

1. **Host allowlist middleware.** The bind is usually `0.0.0.0`, so "derive from the bind address" is empty and
   would lock out the LAN UI and the Mac's `/v1` calls. Rule instead: accept (a) any **IP-literal** `Host`
   (an attacker's DNS-rebinding page must use a DNS name — this is the actual vector), (b) loopback names,
   `os.Hostname()` and `<hostname>.local`, (c) anything in `server.allowed_hosts` (new, additive; for reverse-proxy
   or custom DNS names). Reject other DNS names with 421. Test matrix: `127.0.0.1`, `192.168.x.y:4001`, `localhost`,
   `myhost.local`, `evil.example` (rejected), `evil.example` listed in `allowed_hosts` (accepted).
2. **Cross-site protection** (design §4.4): `net/http.CrossOriginProtection` on every non-safe method of `/admin/api`,
   plus a Fetch-Metadata gate on **all** methods (refuse `Sec-Fetch-Site: cross-site|same-site`), because some GETs have
   side effects. Closes the body-less-POST CSRF (S3) and works through the Vite dev proxy, which a hand-written Origin
   check would not. A request with neither header is a non-browser client and passes until Phase 2.
3. **Drop `Access-Control-Allow-Origin: *`** from `StreamWorkspaceEvents` (`dispatcher_handlers.go:354`); the UI
   is same-origin. Test: SSE response has no ACAO header.
4. **Process-stop ownership.** `POST /admin/api/runtime/processes/{pid}/stop` only signals a PID the
   runtime manager or shell pool spawned (look-up set), else 403. `freePort` (`local_provider.go:140-152`)
   must verify the listener is a previous llama-server of ours before `kill -9`. Closes S2's "kill anything"
   and S7; aligns with the repo memory *verify process ownership before killing*.
5. **Webhook secret mandatory.** A connector with `enabled: true` and no `webhook_token` no longer accepts
   inbound calls (401 with a log line naming the connector). Migration: connector settings UI generates a
   token on enable; existing connectors without one show a banner (this is a behaviour change — call it out
   in the changelog). **Dependency (verified):** Telegram only sends the secret header if `setWebhook` was called
   with `secret_token` (`notifiers/telegram.go:81`; `admin_handlers.go:500-518` already passes the connector's
   `webhook_token`). So generating a token for an existing connector is not enough — the UI must re-run the
   "create webhook" action afterwards, or inbound messages stop arriving. Connector types other than Telegram must
   document which header carries the token (`X-Webhook-Token`). Constitution §I.4 exemption is unchanged.
6. **Fix E5 at the source.** `providers/test` becomes a POST with a JSON body; no secret in a URL; a stored key may only
   be used with its own base URL (or none). Same review for `providers/models`.
7. **Security headers**: `frame-ancestors 'none'`, `X-Frame-Options`, `nosniff`, `Referrer-Policy: no-referrer`,
   `Cache-Control: no-store` on API responses.
8. **Frontend prep (own PR, no behaviour change):** move the 33 raw `fetch` calls onto `httpClient.ts`; give scripts a
   `LLM_PROXY_TOKEN` environment variable (sent as Bearer, ignored until Phase 2); fix or delete the stale
   `scripts/assistant.sh` path.
9. **Complexity gate** stays green (`go run ./tools/check-complexity/` ≤ baseline, no new >12).

**Phase 1 stops browser-origin attacks only.** A prompt-injected agent or a stranger on the LAN is not stopped until
Phase 2; the network-audit "conventions" stay conventions until then.

Verify: `cd backend && go build ./... && go test ./... && go run ./tools/check-complexity/`; frontend
unaffected but run `cd frontend && npm test`.

Acceptance: the Phase 0 exploit-shaped tests are inverted and green; UI still loads from `http://<lan-ip>:4001/admin/`
and from `localhost`; a Mac → remote `/v1` call still works unchanged.

### Phase 2 — Operator authentication (blocked on A1, A2, A5)

1. **Config** `settings.yml → server.auth`: `mode` (`unset|off|token`), `operator_token_hash` (argon2id or
   bcrypt), `session_ttl`, `allowed_hosts`. Add `yaml` tags and per-key backfill — the sandbox plan's D6 lesson:
   plain bools with whole-struct merge boot existing installs with the wrong value. `unset` is the
   upgrade state and means "legacy open + banner".
2. **Credentials and recovery** (design §4.2, §4.8). Passphrase (argon2id) set through a one-time setup code from
   `llm-proxy auth setup-code`; named script tokens (`lp_…`, SHA-256, scopes `admin`/`inference`); `llm-proxy auth reset`
   as the OS-user recovery. Nothing is printed to a log, and recovery is never an HTTP route. Replaces the
   auto-generated startup token (A5c).
3. **Middleware** wrapping `buildRouter`: `admin` routes require a valid session cookie **or**
   `Authorization: Bearer <operator token>`; constant-time compare; per-IP failed-attempt backoff (reuse
   `platform/ratelimiter`). The Phase 0 table drives which routes are wrapped.
4. **Login / logout endpoints** (`POST /admin/api/auth/login`, `/logout`, `GET /admin/api/auth/status`) are the only
   admin routes reachable unauthenticated; `status` returns `{required, authenticated}` and nothing else.
5. **Session store** persisted as SHA-256 hashes in the config root (`auth/sessions.json`), new id on login and step-up,
   idle and absolute expiry, listed and revocable; cookie `HttpOnly; SameSite=Strict; Path=/admin`, `Secure` over HTTPS.
   Live streams re-check the session at each heartbeat. Step-up (`/auth/reauth`) gates the destructive routes (design §4.6).
6. **Frontend** (design §5): `setup` and `login` views, a global 401 handler in `httpClient.ts` (after the Phase 1 fetch
   migration) that routes to login and keeps the target, an `EventSource` error that asks `/auth/status` instead of
   retrying for ever, the unset banner, and a Settings → Security tab (passphrase, sessions, script tokens, exposure
   summary). Existing primitives only; visual baselines for each screen on desktop and mobile.
7. **SPEC-003** v2.3: new "Sign-in" section; §I "no account model" reworded to "one operator credential".
   **SPEC-007 §V.1** wording fixed to match. **Constitution** new law (A6).

Verify: `cd backend && go test ./internal/transport/... ./internal/app/... -count=1`;
`cd frontend && npm test && npm run build && npm run test:visual` (login page baseline added);
manual: `curl -i http://127.0.0.1:4001/admin/api/state` → 401 once `mode: token`.

Acceptance: with `mode: token`, every route in the Phase 0 table marked `admin` returns 401 without a credential
and 200 with one; the workspace live SSE (`/admin/api/dispatcher/workspaces/{workspace}/live`) works in the browser after login; a wrong token is rate limited;
upgrading an existing install without touching config still boots and shows the banner.

### Phase 3 — Inference keys for `/v1/*` (blocked on A3)

1. `server.auth.inference_keys[]` (name, hash, created, last_used) managed in Settings → Security. Keys are
   shown once.
2. `/v1/*` accepts `Authorization: Bearer <key>` (what upstream clients already send). Unknown/missing →
   401 **before** the residency gate, so unauthenticated callers cannot occupy the inbound queue (SPEC-007
   §V.1 `inbound_max_queued`).
3. `DELETE /v1/queue/{key}` keeps its random key and additionally requires the same inference key.
4. Document the recommended remote setup: set an inference key on the llama host, put the same key in the
   Mac's provider credential, optionally restrict `allowed_hosts`.

Verify: `cd backend && go test ./internal/transport/http/handlers/ -run 'Inbound|Proxy' -count=1` plus a new
matrix test (no key / wrong key / right key × `/v1/models`, `/v1/chat/completions`, `DELETE /v1/queue/{k}`).

Acceptance: with a key required, an unauthenticated caller never appears in `active-runs` queued list;
the existing remote-proxy flow works after adding the key to the provider credential.

### Phase 4 — Safer defaults and deployment

1. Fresh installs bind `127.0.0.1` (A4); `setup.sh`, `docs/services/llm-proxy.service`,
   `docs/services/llm-proxy.launchd.plist` and `docs/service_setup.md` get an explicit "to serve the LAN, set
   `server.bind` and an operator token" step.
2. Startup log line and the Security page show the effective exposure (`bind`, auth mode, inference key
   required, webhook secrets present) next to the existing `Effective` sandbox readout.
3. `docs/api-reference.md` gets an Authentication section; the route table annotates each group.

Verify: `./scripts/check-agent-harness.sh` (docs), `cd backend && go test ./...`.

Acceptance: a clean install is unreachable from another machine until the operator opts in; the exposure
summary is correct in each of the four combinations (loopback/LAN × auth off/on).

## Risks and non-goals

| Risk | Mitigation |
|---|---|
| Locking the operator out | `llm-proxy auth reset` run as the server's OS user (never an HTTP route: the agent can reach loopback); `unset` is never fatal. |
| SSE + cookies behind a reverse proxy | `SameSite=Strict` needs same site; document proxy path mapping; test with `X-Forwarded-*`. |
| Breaking the Mac → remote setup | Phase 3 uses the header those clients already send; Phase 1 has an explicit acceptance test for it. |
| Scope creep into users/roles/OIDC | Non-goal (A1). Revisit only if a second operator appears. |
| Webhook behaviour change (Phase 1.5) | Called out in changelog; banner + auto-generated token; Constitution §I.4 unchanged. |

Non-goals: TLS termination in-process, per-route scopes beyond operator / inference / webhook, audit log UI
(a structured `auth` log line is enough).

## Features that depend on this plan

[`network-audit.md`](network-audit.md) (2026-10-24) was designed with this plan **parked by the owner**, so these are
conventions there, not security controls, until Phases 1–2 here land:

- operator-only acknowledgement of scan findings (an unauthenticated caller on the LAN can edit them);
- the scan-scope list, the enabled scan modes and the "allow unconfined scans" switch;
- approval of intrusive scan modes;
- scripts and intrusive modes against the host's own addresses (the admin API is reachable on those);
- the import of vulnerability data for known-vulnerability matching (network-audit §13.G, proposed 2026-10-27): an unauthenticated caller on the LAN could replace the index and silence the matching findings;
- the fix records of network-audit §13.H Level 1 (`/netaudit/fixes`; low impact, since a record never hides a finding) and everything in its Level 2: managed-host records, stored SSH keys, and the approval of a command on a host. Level 2 must not be built before this plan lands.

When this plan lands, revisit those items and make them real checks.

## Remaining Work

**Built 2026-10-29 on branch `admin-auth-planning` (v0.17.0):** Phase 0 (the route classification
test `internal/app/route_classes_test.go`, `Router.Routes()`, and the E5 proof, **run and confirmed**:
`providers/registrar_baseurl_key_test.go`) and the browser half of Phase 1: `api.Guard` (Host rule, Fetch-Metadata on every
method of `/admin/api`, `CrossOriginProtection` on writes, framing/sniffing/referrer headers, `no-store` on the API;
`server.allowed_hosts`), `providers/test` as a POST with a body (frontend caller updated), the wildcard CORS header
removed from the live stream, PID-stop ownership (only a model server the Models page lists), and an optional
`LLM_PROXY_TOKEN` in `scripts/evallib.py`.

**Phase 2 built 2026-10-29 on branch `admin-auth-phase2` (v0.18.0, backend and UI together):**
- *Backend:* `platform/auth` (argon2id passphrase, `lp_` script tokens, persisted hashed sessions with idle and absolute expiry,
  a one-time setup code that is single-use, expires and is cancelled by guessing, per-address and global lockout, step-up
  elevation that rotates the session id, the last 20 events, `auth.json` 0600 that a second process (the CLI) can change and
  the server notices); `api.Authenticate` (default deny from the path: `/admin/api/*` needs a session or an admin token once a
  passphrase exists, `/v1/*` needs a key once an inference token exists, a store that cannot be read gives `503` and never
  reads as open; step-up routes refuse tokens); `/admin/api/auth/*`; `llm-proxy auth setup-code|status|reset`; the `lp_`
  token shape in the shared secret patterns and `.gitleaks.toml`.
- *Frontend:* every request already goes through `apiFetch`; the router guard and `startAuth`, `LoginView`, `SetupView`, the
  "not set up" banner, the "confirm it's you" dialog (a request the server answers `reauth_required` is repeated after the
  passphrase), `EventSource` errors ask `/auth/status` instead of retrying for ever, and Settings → Safety → Sign-in & access
  (change passphrase, sessions, script tokens, recent sign-ins).
- *Deliberately decided here:* script tokens are refused on the step-up routes (design D4: never, the safer option, until the
  owner chooses otherwise); session lifetimes are **24 h idle / 14 d absolute (D3, owner chose this 2026-10-29)** and are settings (`server.auth.idle_hours`, `server.auth.absolute_days`, capped at 720 h and 90 d, applied without a restart).

**Still to do:** (a) the **mandatory webhook secret** (E8) — it must ship with the UI that generates a token on enable and
re-runs Telegram's `setWebhook`, or an existing connector stops receiving messages at deploy; (c) fix or delete `scripts/assistant.sh`;
(d) Phase 0 leftovers: audit the remaining GETs for side effects, check connector/config GETs for unmasked tokens, confirm
startup output does not reach `app-logs` (E18); (e) ~~conditional step-up for `PUT /config` and `PUT /system`~~ **done 2026-10-29**: `AppContext.SystemUpdateChangesWhatRuns` compares the update with the current settings (model-server environment, arguments and path, GPU program) and `api.StepUpGranted` / `api.RefuseStepUp` gate it in the handlers; a settings save that sends back what it read is not a change; (f) live streams re-check the session at each heartbeat (a revoked session's open stream
runs on until it closes); (g) `server.trusted_proxies` so `Secure` and "encrypted" follow a reverse proxy's `X-Forwarded-Proto`;
(h) the **exposure summary** (listening address, webhook secrets, sandbox mode) on the Sign-in page, `setup.sh` printing the
setup code and a loopback-only default bind for new installs (Phase 4); (i) ~~`llm-proxy auth` finds the service~~ **done 2026-10-29** (0.18.2: it chooses the folder as the server does (`.env` loaded first), as root becomes the account that owns the folder (the service's folder when a unit is installed and none is named), prints the file it used, and says whether a live server reads it from the `server.json` a running server leaves in its data folder; before this a setup code typed from a checkout landed in a file the service never read. Blind spots, all reported by that check: systemd drop-ins and `EnvironmentFile=` are not read, servers older than 0.18.2 leave no marker); (j) the **Constitution** law (A6) and SPEC-003/SPEC-007
wording, which need the owner's review. **Phase 1 stops browser-origin attacks; Phase 2 is what stops a stranger on the LAN
and a prompt-injected agent**, provided the passphrase is set: until the operator runs `llm-proxy auth setup-code` the API is
still open.

D4 is decided: **script tokens never do step-up actions** (owner accepted the recommendation 2026-10-29); revisit only if a real script needs one.
