---
status: proposed
date: 2026-09-30
last_reviewed: 2026-09-30
related_specs: [SPEC-006, SPEC-008, SPEC-009]
constitution_references: [I.1, I.2, II.3]
evidence: docs/audits/2026-09-30-platform-scan.md (S6, S6b, S7, S10, S11)
---

# Sandbox and Egress — Residual Hardening

**Status:** proposed. Small, independent items; none needs a SPEC change except where noted.

## Scope and what this plan is NOT

The agent sandbox is substantially complete and this plan does **not** redo it. Read
`agent-os-sandboxing.md` (status `complete`, Phases 0–4 + two hardening passes) first: network default-off,
per-run grants, loopback-only egress proxy with a per-process token, Landlock jail (Linux), env allowlist,
`.sandbox` invisibility, dedicated-user deployment. This plan collects what that plan *listed as residual*
plus what the 2026-09-30 scan found in the guard code. The larger gap — the **unauthenticated control
plane** in front of the sandbox — is [`admin-api-authentication.md`](admin-api-authentication.md); several
items here only matter because of it.

Rejected before and **not reopened here** (reasons in `agent-os-sandboxing.md` §2): Docker/containers as the
default, WASM (tried and removed 2026-05), per-use-case permission profiles, the deprecated `sandbox-exec` CLI.

## Findings this plan acts on

| ID | Item | Evidence | Label |
|---|---|---|---|
| S6 | `validateIP` lets `0.0.0.0` / `::` (and multicast) through; with internet access on, `fetch_url http://0.0.0.0:<port>/admin/api/...` reaches the local admin API | `backend/internal/core/tools/network.go:250-272` (guard), `:49-82` (dial-time use) | VERIFIED by reading, not executed |
| S6b | A network-on shell can call loopback (Landlock TCP-deny only covers network-off children) | SPEC-006 §II.7.5 | FROM-DOC |
| S7 | `freePort` kills any process holding the model port | `backend/internal/core/llm/providers/local_provider.go:140-152` | VERIFIED |
| S10a | macOS has no per-workspace FS jail (Seatbelt not built; uid-first launchd is the answer) | sandboxing plan §10 | FROM-DOC |
| S10b | Egress proxy has a per-*process* token, not per-*run* scope; a shell under `networkOn` reaches LAN and internet (OS cannot split them) | sandboxing plan §10 "Residuals" | FROM-DOC |
| S10c | macOS cannot cap per-process memory (measured); Linux relies on systemd cgroup | sandboxing plan D5 | FROM-DOC |
| S10d | MCP servers are operator-trusted and outside the jail | sandboxing plan §2 out-of-scope 3; SPEC-008 | FROM-DOC |
| S10e | Complexity-debt baseline of 69 functions over the limit | sandboxing plan "complexity gate note" | FROM-DOC |
| S12 | The egress audit AST test the sandbox plan cites is not at its stated path (`backend/internal/platform/network/` has no `egress_audit_test.go`) | directory listing 2026-09-30 | VERIFIED — Phase 1 step 6 |
| — | Webhook secret optional | see authentication plan Phase 1 | VERIFIED (other plan) |

## Phase 1 — Fix the guard (S6, S7). Write the failing test first. — **done 2026-10-08**

**Implemented:** one classifier `platform/network.CheckAlwaysBlocked` (unspecified/loopback/link-local/multicast, typed `ErrAlwaysBlockedAddress`) + `IsLAN` (private + CGNAT `100.64.0.0/10`, so a tailnet counts as LAN — tightens the old "CGNAT = internet" behaviour). `tools.validateIP` uses it; the egress proxy enforces it in `net.Dialer.Control` on the resolved IP (403, rebinding-safe; `Server.dialGuard` is the test seam). `validateAddress` now fails closed on DNS error. `freePort` is replaced by `portReclaimer`: it terminates only processes of the configured server binary launched with `--port <port>` (reusing `process.ListByBinary`/`process.Kill`, no new persisted state) and otherwise fails the start with `ErrPortInUse`. Step 6: the AST audit exists at `internal/platform/network/transport_test.go` (S12 was a wrong path in the plan, not a missing test); the stale `local_provider.go` allow-list entry was removed.

1. **Reproduce S6 with a test.** In `internal/core/tools/network_test.go`, table-drive `validateIP` with
   `0.0.0.0`, `::`, `224.0.0.1`, `ff02::1`, `127.1.2.3`, `::ffff:127.0.0.1`, `169.254.169.254`, `100.64.0.1`,
   and an end-to-end `FetchURL` against an `httptest` server on `127.0.0.1` addressed as `http://0.0.0.0:<port>/`.
   Expect blocked; today the unspecified cases pass.
   - Verify (red → green): `cd backend && go test ./internal/core/tools/ -run 'ValidateIP|FetchURL.*Unspecified' -count=1`.
2. **Fix.** In `validateIP` reject `ip.IsUnspecified()`, `ip.IsMulticast()`, `ip.IsInterfaceLocalMulticast()`,
   and normalise v4-in-v6 (`ip.To4()`) before classification. Keep the loopback/link-local "always blocked"
   comment accurate.
3. **Same classifier for the proxied path.** The egress proxy resolves inside the proxy; confirm its
   `HostListPolicy` (`internal/core/egress`) rejects loopback/unspecified/link-local *targets* regardless of
   the allow-list, and add a test (`CONNECT 0.0.0.0:<port>` → 403). If it does not, reuse the one classifier —
   do not write a second list.
4. **Remove the misleading `return nil`** in `validateAddress` (`network.go:235-239`): make the pre-check
   fail closed on DNS error (the dial guard already does) so the two layers agree and the next reader does
   not have to re-derive that it was safe.
5. **`freePort` ownership (S7).** Before killing, verify the listener's PID belongs to a previous instance of
   our model server (recorded PID file or matching `Cmd` path + start time); otherwise fail the start with a
   clear error naming the foreign process. Same reasoning as the repo note *verify process ownership before
   killing* — a wrong kill of a user's process is worse than a failed start.
   - Verify: `cd backend && go test ./internal/core/llm/... -run FreePort -count=1`.

6. **Locate or re-create the raw-client audit test (S12).** The claim "zero raw `http.Client`/`exec` outside the
   allow-list, AST-enforced" (Constitution §I.1) is only as good as its test. Find it
   (`grep -rln "DefaultClient" backend --include='*_test.go'`); if it does not exist, re-create it as an AST scan over
   `backend/internal` covering `http.DefaultClient`, `http.Get/Post/Head/PostForm`, `&http.Client{}` without a
   `Transport`, `exec.Command*` and `os.StartProcess` outside the documented spawn sites
   (`platform/process`, `shell/terminal.go`, `providers/local_provider.go`, `platform/process/process_darwin.go`,
   `platform/metrics/gpu_providers.go`). It will need `local_provider.go`'s `fuser`/`lsof` calls allow-listed or,
   better, replaced once 1.5 lands.
   - Verify: `cd backend && go test ./internal/platform/... -run 'Audit|RawClient' -count=1`.

Acceptance: S6 test red before, green after; `go test ./...` and
`go run ./tools/check-complexity/` (no new >12) pass; the egress-audit AST test
described in `agent-os-sandboxing.md` §10 (slice 1a; it was not found at the path that plan names, so locate it
first with `grep -rln "DefaultClient" backend --include='*_test.go'`) still passes.

## Phase 2 — Close the "shell can reach the admin API" path (S6b)

Depends on authentication Phase 2 for the real fix; until then reduce exposure.

1. **Never put auth material in the shell env** — test on `prepareShellEnv`
   (`backend/internal/shell/terminal.go`) that nothing named like a token/session/secret survives the allowlist.
2. **Deny the control-plane port to network-on shells where the kernel can.** Landlock ABI ≥ 4 supports
   `ConnectTCP` *allow* rules by port, not deny; evaluate "network-on = allow-list of egress-proxy port only"
   (i.e. force the proxy path) instead of "network-on = everything". This changes the documented D8 residual
   (*a networkOn shell reaches LAN and internet*) and is therefore **a design decision for the user** — record the
   options here, do not build until chosen:
   - (a) keep D8 as is; rely on authentication (recommended baseline, zero new risk of breaking CLIs);
   - (b) on Linux ABI ≥ 4, restrict `networkOn` shells to the egress proxy port (strong, but breaks tools that
     ignore `HTTP_PROXY`, e.g. `git` over ssh);
   - (c) per-run proxy scope with run tokens (the deferred 1e item) plus (b).
3. Until (b)/(c): show in the Security page's `Effective` card that shell network is "all-or-nothing", which
   the copy already says — verify the copy still matches after the authentication work.

Verify: `cd backend && go test ./internal/shell/... ./internal/platform/sandbox/... -count=1`;
Linux-only runtime checks run in the `sandbox-conformance` CI job.

Acceptance: the decision (a/b/c) is written into `agent-os-sandboxing.md` §6; if (b) or (c) is chosen, a
conformance test proves a network-on child cannot connect to the admin port but can reach the proxy.

## Phase 3 — Platform residuals (optional, environment-dependent)

| Item | Action | Gate |
|---|---|---|
| S10a macOS Seatbelt | Only if a dev-mode operator needs per-workspace isolation on macOS. The preferred answer is the dedicated-user launchd deployment already shipped (`docs/services/llm-proxy.launchd.plist`). Do not build Seatbelt until a host with the Sandbox framework is available and the user asks. | user request |
| S10c memory | Document `MemoryMax` guidance per model size in `docs/service_setup.md`; no code. | docs |
| S10d MCP | Add an `mcp_servers[].trusted: true` acknowledgement in the UI when adding a server, plus SSRF validation on the URL (reject link-local/unspecified; LAN requires the same network guard). Adding a server becomes an authenticated action after the authentication plan. | SPEC-008 amendment |
| S10e complexity debt | Burn down the baseline opportunistically (ratchet: a function touched must drop below 12). Do not schedule a dedicated refactor. | `go run ./tools/check-complexity/` |

## Risks

| Risk | Mitigation |
|---|---|
| Blocking unspecified/multicast breaks a legitimate LAN scan | `scan_local_network` iterates explicit host IPs; add a test that a `/24` scan still works. |
| `freePort` refuses to start after a crash left an orphan | The ownership check accepts our recorded PID/start time, so a crashed prior instance is still recognised. |
| Webhook/connector behaviour change | Owned by the authentication plan Phase 1. |

## Remaining Work

Phases 2–3 (Phase 1 done 2026-10-08).
