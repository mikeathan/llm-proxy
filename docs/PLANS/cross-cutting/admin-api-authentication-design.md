---
status: proposed
date: 2026-10-29
last_reviewed: 2026-10-29
related_specs: [SPEC-003, SPEC-006, SPEC-007, SPEC-009]
constitution_references: [I.4, III.6, V.1]
evidence: this file § Evidence ledger (code read on main 40ea6965, 2026-10-29); docs/audits/2026-09-30-platform-scan.md
---

# Admin / API authentication — reviewed design (2026-10-29)

The design half of [admin-api-authentication.md](admin-api-authentication.md), which keeps the phases, the decisions and
the status. This file holds the evidence, the threat model, what the 2026-09-30 plan missed, the credential, session and
CSRF design, and the UI. **Nothing here is built.** Evidence is labelled **verified** (read in code or run), **inferred**
(follows from code but not run) or **to prove** (Phase 0 settles it). Code references are to main at 40ea6965.

## 1. Evidence ledger

| # | Finding | Where | State |
|---|---|---|---|
| E1 | No request is authenticated. The router adds only panic recovery; 148 routes are registered bare; no inbound code reads an `Authorization` header (the only uses are outbound, in the providers and the egress proxy). | `transport/http/router.go` (`recoverHandler`), `app/routes.go`, grep of `Authorization` | verified |
| E2 | Default bind is every interface; the server has a 30-minute write timeout (for streams) and no TLS. | `app/app.go` `ResolveBindAddr`, `http.Server{WriteTimeout: 30*time.Minute}` | verified |
| E3 | The workspace live stream answers `Access-Control-Allow-Origin: *`. | `handlers/dispatcher_handlers.go:527` | verified |
| E4 | `POST /admin/api/runtime/processes/{pid}/stop` signals **any** PID unless it is the active model. | `handlers/process_handlers.go:299-312` | verified |
| E5 | **A stored provider key can be sent to a URL the caller chooses, by a GET.** `GET /admin/api/providers/test?provider&api_key&api_key_name&base_url` builds a provider from those four query values. When `api_key_name` names a stored key, the registrar fills in the real key; the caller's `base_url` is replaced only if the stored key has its own `base_url`. `TestConnection` then lists models at that URL with the key as `Authorization: Bearer`. | `model_handlers.go:115-133` → `llm/manager.go:246-260` → `providers/registrar.go:130-150` → `provider_openai_compatible.go:374` | verified by reading; **to prove** by running (Phase 0). The earlier audit called it S5 and an assumption. |
| E6 | E5 is a GET, so Origin and CSRF checks that skip safe methods do not cover it: a cross-site `<img src="http://<lan-ip>:4001/admin/api/providers/test?...">` fires it, and an IP-literal `Host` passes the Host rule in the old Phase 1. Key names are listable (`GET /admin/api/secrets/keys` returns the masked list). The same handler also puts secrets in the **query string** (logs, history, `Referer`). | `secrets_handlers.go:28`, `model_handlers.go:122-124` | inferred from E5 |
| E7 | `GET /admin/api/providers/models?provider&api_key_name` also uses a stored key; it takes no `base_url`, so it only reaches the provider's own URL. Lower risk. | `model_handlers.go:97-111` | verified |
| E8 | A webhook token is checked only when one is set; with none, the endpoint is open apart from a per-connector rate limit. | `handlers/webhook_handlers.go:82-100` | verified |
| E9 | The browser uses `fetch` in 33 places in 5 services (`assistant`, `memory`, `template`, `commands`, `reminders`) that bypass `httpClient.ts`, so a 401 handler there sees only part of the traffic. | `grep fetch( frontend/src` | verified |
| E10 | `EventSource` cannot read a status code. `useSSEConnection` counts errors and retries every 30 s for ever, and `useAssistantSSE` and `useLiveConsole` use it too. A 401 would look like a flaky connection. | `composables/network/useSSEConnection.ts` | verified |
| E11 | Five scripts call the API without any credential: `evallib.py` (used by four eval scripts) and `assistant.sh`. `assistant.sh` posts to `/api/conversation/message`, a path no route serves (the route is `/admin/api/conversation/message`), so it is already stale. `telegram-bot.sh` talks to Telegram and the webhook. | `scripts/*` | verified |
| E12 | Provider keys are masked in GET responses and recordings run a redactor, so the cheap leaks are closed; E5 is the way around the masking. | `secrets_handlers.go:28`, `proxy/recorder/recorder.go:35-46` | verified |
| E13 | The agent jail (Landlock, Linux) grants only listed paths; there is **no file jail on macOS**, so an agent shell there reads the data root as the operator. An auth store must hold only hashes and nothing forgeable. `InternalDir` is the central metadata directory, outside every workspace. | `sandbox/landlock_provider_linux.go:78,177`, `storage/resolver.go:67`, audit S10 | verified |
| E14 | A network-on agent shell reaches loopback (audit S6b), so "localhost is trusted" is false; a `curl` from it sends no `Origin`, so no browser-origin check can stop it. | audit S6b, SPEC-006 §II.7.5 | verified (doc) |
| E15 | Go is 1.26.2, which has `net/http.CrossOriginProtection` (Fetch-Metadata, then Origin vs Host; safe methods always pass; no header = allowed). `golang.org/x/crypto v0.29.0` is already in the module graph as an indirect dependency (argon2id lives there). | `go doc net/http.CrossOriginProtection`, `go.mod` | verified |
| E16 | The Vite dev proxy uses `changeOrigin: true`, which rewrites `Host`; a hand-written Origin-equals-Host check would break `npm run dev`. | `frontend/vite.config.ts` | verified |
| E17 | Everything added since the 2026-09-30 plan is unauthenticated too: the network-audit scope, vulnerability-data download (starts outbound fetches), findings, acknowledgements, fix records and expectations; the host terminal and sandbox switches (`PUT /admin/api/host`); `PUT /admin/api/config` (the llama binary and its arguments, i.e. command execution as the service user). | `app/routes.go` | verified |
| E18 | No request logger writes full URLs (no match in `transport` or `app`); app logs are served by `GET /admin/api/app-logs`, so anything printed at startup could be read through the API. | grep | verified; **to prove** that the startup output reaches that log |

## 2. Threat model

| Who | Can do today | Stopped by |
|---|---|---|
| **A. A stranger on the LAN** | Everything in E1, E4, E17: run agents with a shell, switch off the sandbox, change the model binary, wipe the data root. | Phase 2 (login). Phase 1 does not stop this. |
| **B. A web page the operator visits** | Body-less cross-site POSTs (wipeout, factory reset, restart); DNS rebinding; **cross-site GETs with side effects (E5)**. | Phase 1: Host rule, Fetch-Metadata check on **all** methods of `/admin/api`, E5 fixed at the source, framing headers. |
| **C. A prompt-injected agent with a network-on shell** | Calls the admin API on loopback with no browser headers. | Phase 2 only, with the rule that no credential ever enters an agent environment. |
| **D. Another OS user, or an unjailed agent (macOS) reading files** | Reads `settings.yml`, `master.key`, the auth files. | Store only hashes (argon2id passphrase, SHA-256 of 256-bit random tokens and session ids). A hash cannot be replayed. The existing co-location of `master.key` (Constitution III.6) is unchanged. |
| **E. A listener on the LAN (plain HTTP)** | Reads the passphrase and the session cookie. | Not stopped in code: document a TLS front end, show "not encrypted" in the exposure summary, mark the cookie `Secure` behind HTTPS. |
| **F. The operator's own mistakes** | Locks themselves out; leaks a secret printed in a log; loses sessions at every restart. | Recovery by the server's OS user only; no secret is ever printed to a log; sessions persist. |

## 3. What the 2026-09-30 plan missed or got wrong

1. **E5/E6 is probably the most serious finding and Phase 1 did not close it** (a GET). New Phase 1 item: the endpoint becomes a POST with a JSON body, secrets leave the URL, and a stored key may only be used with that key's own base URL (or none).
2. **Phase 1 does nothing about the agent (C).** The old text implied the control plane was hardened by Phase 1; it only stops browsers. This must be said in the plan and in the network-audit "convention" notes: they stay conventions until Phase 2.
3. **The lockout recovery in the risk table was a loopback listener that "always accepts `set-token`".** That contradicts the plan's own "loopback is not trusted" rule (C can call it). Recovery is a command-line tool run as the server's OS user that edits the auth file; never an HTTP route.
4. **Printing a generated token at startup (A5c) leaks it** to the systemd journal and possibly to `app-logs` (E18). Replaced by a one-time setup code made on demand by a CLI (§4.8).
5. **In-memory sessions** sign the operator out at every restart, and this app restarts itself (exit 75, settings, deploys). Sessions are persisted, hashed.
6. **The frontend 401 handler** has two holes: 33 raw `fetch` calls (E9) and `EventSource` (E10). Both are phase work, not footnotes.
7. **Scripts** (E11) need a credential; one operator token for everything would put full admin in every eval script. Named, revocable tokens instead.
8. **Clickjacking.** After auth, a page that frames `/admin/` could still trick a click on "wipe". Framing and sniffing headers are needed.
9. **One token for human and machine** means rotating a script's key signs the human out. Split credentials.
10. **No re-authentication for the destructive actions**, so a left-open session wipes the data root with one click. Step-up (§4.6).
11. **Plain HTTP on the LAN** (E): not mentioned.
12. **The Mac dev case (E13)**: auth files must be safe to read.

## 4. Design

### 4.1 Surfaces

| Surface | Rule |
|---|---|
| `/admin/` shell and its assets | **Public.** The shell holds no data. Keeping it public lets the cookie stay `SameSite=Strict` when the operator follows a link in from Telegram or a bookmark (a protected shell would be a cross-site navigation with no cookie). Write this reason next to the route table so nobody "fixes" it. |
| `/admin/api/auth/status`, `/login`, `/setup` | Public, rate limited. `status` returns `{required, authenticated, setup_open}` and nothing else. |
| `/admin/api/*` (everything else, including `/live` streams) | Operator session cookie **or** `Authorization: Bearer <admin token>`. Default deny from the classification table. |
| `/v1/*` | Phase 3: inference key when any exists; open until the first is created. |
| `/api/v1/webhooks/{connector}` | Its own mandatory secret (E8), not the login. |

A route-classification test walks every registered route and fails on one that is not in the table: the next route added cannot be open by accident.

### 4.2 Credentials (decided 2026-10-29: passphrase for people, named tokens for machines)

- **Passphrase** (one operator). argon2id (`x/crypto/argon2`, promoted from indirect to direct), parameters at or above the OWASP minimum and tuned to about 150 ms on the target; the parameters are stored with the hash so they can be raised. Minimum 12 characters, a small embedded list of the most common passwords refused, no composition rules. At most two verifications run at once, so a flood of logins cannot exhaust memory.
- **Tokens** (machines). Format `lp_` + 32 random bytes (base32). Stored as SHA-256 only, shown once, with a name, scope (`admin` or `inference`, so A3's inference keys are the same mechanism), created, last used, revocable. Sent as `Authorization: Bearer`. The `lp_` prefix is added to the redaction patterns and to `.gitleaks.toml` so a pasted token is caught.
- Compare with `subtle.ConstantTimeCompare` after hashing; one error text for every failure.

### 4.3 Sessions

- 256-bit random id; the server stores **SHA-256(id)** in `auth/sessions.json` under the config root (atomic write, 0600, outside every workspace, E13), with created, last seen, client address and browser name, and the step-up expiry.
- Cookie `llmproxy_session`: `HttpOnly; SameSite=Strict; Path=/admin`, plus `Secure` when the request is HTTPS. The `Host-` cookie prefix needs `Secure`, which plain-HTTP LAN installs cannot set, so it is not used.
- Idle expiry 8 hours, absolute 7 days (to be confirmed as decision D3). A new id on login and on step-up (no fixation). Sign-out deletes it; "sign out everywhere" deletes all.
- Live streams are checked when they open and again at each heartbeat, so a revoked session cuts its stream.
- Behind a reverse proxy, `X-Forwarded-Proto` is believed only from addresses in `server.trusted_proxies`.

### 4.4 Cross-site protection (Phase 1, no login needed)

- `net/http.CrossOriginProtection` on every non-safe method of `/admin/api`, **plus** a Fetch-Metadata gate on **all** methods of `/admin/api`: a request carrying `Sec-Fetch-Site: cross-site` or `same-site` is refused, because GETs have side effects (E6). A request with neither `Sec-Fetch-Site` nor `Origin` is a non-browser client and passes (it meets authentication in Phase 2). This also works through the Vite proxy (E16), which a hand-written Origin-equals-Host test would not.
- Host rule as in the old plan: any IP-literal host, loopback names, the machine's name and `.local`, and `server.allowed_hosts`; other names get 421. (It stops DNS rebinding; it does not stop E6 by itself, which is why the Fetch-Metadata gate and the E5 fix are there.)
- Remove the `ACAO: *` (E3). Add `Content-Security-Policy: frame-ancestors 'none'`, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, and `Cache-Control: no-store` on `/admin/api` responses. A fuller CSP for the shell is a follow-up after the build is checked for inline scripts.

### 4.5 Browser transport

- All 33 raw `fetch` calls move onto `httpClient.ts` first (no behaviour change, its own PR). Then there is one place for the 401 handler, and `credentials: 'same-origin'` is explicit.
- On an `EventSource` error the composable calls `GET /admin/api/auth/status`; "not authenticated" routes to login instead of retrying for ever.
- A 401 keeps the current route as the return target.

### 4.6 Step-up (decided 2026-10-29: yes)

`POST /admin/api/auth/reauth {passphrase}` marks the session elevated for 5 minutes. These return `403 {"error":"reauth_required"}` until then: wipeout, factory reset, clear runtime data, `PUT /host` (sandbox and terminal switches), `PUT /config` and `PUT /system` when the model binary, its arguments or environment change, secrets and tool secrets (PUT/DELETE), MCP add and update, creating a token, changing the passphrase, `allow_unconfined` in the scan scope, and the future deep check (network-audit §13.J). The list lives beside the classification table and a test checks that each listed route is gated. Bearer tokens of scope `admin` do not get step-up; the destructive routes above refuse a token unless the token was created with step-up in the last 5 minutes (tokens cannot wipe by default).

### 4.7 Brute force and logging

Per-address and global limits (reusing `platform/ratelimiter`) with growing delay; a locked address is told how long to wait. CLI recovery is never blocked. Auth events (`login_ok`, `login_failed`, `logout`, `reauth`, `token_created`, `token_revoked`, `setup`, `reset`) are logged with the client address and **never** the secret, and the last 20 are kept for the Security page.

### 4.8 First run, upgrade and recovery (decided 2026-10-29: one-time setup code)

- State `unset` (today's behaviour, the upgrade state): the API stays open and the UI shows a persistent banner "Sign-in is not set up", louder when the server listens beyond loopback. Never refuse to boot.
- Setting up: on the server run `llm-proxy auth setup-code` (as the server's OS user). It writes a 15-minute, single-use code hash to the auth file and prints the code **to that terminal only**, never to the log. The UI first-run screen takes the code and a new passphrase. `setup.sh` runs the same command at the end of an install.
- Once a passphrase exists, sign-in is required; there is no UI switch to turn it off. `llm-proxy auth reset` (OS user only) clears the passphrase and all sessions and tokens so setup can be repeated.
- Anyone with the OS user already owns the box (Constitution III.6), so this adds no new trust.

### 4.9 Agents

The passphrase, tokens, cookies and the auth files never enter an agent environment (a test on `prepareShellEnv`), and recordings and logs redact `lp_` tokens and cookies. A prompt-injected run has nothing to present.

### 4.10 TLS

Out of code. Document a reverse proxy or Tailscale in front, and show "Not encrypted" in the exposure summary when the request is plain HTTP and the bind is not loopback.

## 5. UI

Existing primitives and tokens only; no new design language. Plain words, no jargon; every failure says what happened and what to do.

**First run** (route `setup`, shown when `status.setup_open`):
```
Set up sign-in
Run  llm-proxy auth setup-code  on the server and enter the code.
[ Setup code      ]
[ New passphrase  ]   12 characters or more. A few words is fine.
[ Repeat it       ]
                  [ Set up sign-in ]
```
**Login** (route `login`): one field, one button, a "Show" toggle, `autocomplete="current-password"` with a hidden username field so password managers fill it; the lock-out message names the wait; the page works on a phone (44 px targets, no horizontal scroll). After sign-in the user returns to the page they were on, and a composer draft they were typing is kept (to verify against the existing draft handling in Phase 2b).
**Banner** while unset: "Sign-in is not set up. Anyone on your network can change settings and run the assistant. [Set up sign-in]", stronger wording beyond loopback.
**Settings → Security** (new tab, checklist `docs/architecture/checklists.md` → settings tab):
- *Sign-in*: change passphrase (asks for the old one).
- *Where you are signed in*: sessions with browser, address, last seen, this one marked; Sign out / Sign out everywhere.
- *Script access*: tokens with name, scope, created, last used; "New token" shows it once with Copy; Revoke.
- *Exposure*: listening on (loopback or network), sign-in required, encrypted or not, inference key required, webhook secrets present (n of m), sandbox mode, recent sign-ins. Items that need attention say what to do.
**Confirm it's you**: a small dialog, shown when an action returns `reauth_required`, then repeats the action.
**Mobile and accessibility**: errors in `role="alert"`, focus moves to the first field, visible focus rings, no colour-only state; the visual tests (desktop and mobile) get baselines for setup, login, the banner and the Security tab.

## 6. Security tests (the acceptance list)

Every registered route is classified; each `admin` route returns 401 with no credential and 200 with one; a cross-site GET to the provider test is refused, and the endpoint no longer accepts a key in the URL; Host and Fetch-Metadata matrices (including the Vite proxy shape); cookie flags; session id and token stored only as hashes; session fixation (new id on login); step-up gates each listed route; streams close when a session is revoked; setup code is single-use, expires, and is never in any log; `auth reset` works with no network; login floods are limited and do not exhaust memory; timing-safe compare; no log line contains a secret or `lp_`; the agent environment holds no auth material; `/v1` keeps working with no key until one exists; an upgrade with no configuration still boots and shows the banner.

## 7. Open questions

- **D3**: session lifetimes (idle 8 h / absolute 7 d proposed).
- **D4**: tokens that touch destructive routes: allowed only right after step-up (proposed) or never.
- Whether `server.trusted_proxies` is wanted at all (proposed: yes, empty by default).
- E18, E5 and the unmasked-token question for connector GETs are settled in Phase 0 by running them.
