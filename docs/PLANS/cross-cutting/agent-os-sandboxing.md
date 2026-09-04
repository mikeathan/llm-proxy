---
status: proposed
date: 2026-08-31
related_specs: [SPEC-006]
constitution_references: [II.3]
related_plans: [cross-cutting/sandbox-runtime-invisibility.md, unattended-run-safety-hardening.md]
---

# Agent OS Sandboxing (kernel-enforced containment for everything the agent does)

## 1. Problem

Today's "sandbox" (`backend/internal/shell/terminal.go`, `workspaceEnvTemplates`) is an
**environment-level jail, not a security boundary**:

- `HOME` → `{workspace}/.sandbox` plus `GOPATH`, `GOMODCACHE`, `TMPDIR`, `XDG_CACHE_HOME`
  redirects *where files land*, but nothing stops a shell command from reading
  `~/.ssh/id_rsa`, writing `~/Library/LaunchAgents/...` (persistence), or touching `/etc`.
- `HostSandboxingConfig.MaxMemoryMB` / `MaxStorageGB` (`backend/models/host_settings.go`)
  are **configured but never enforced** — no `setrlimit`, no cgroups. A fork bomb or a
  `malloc` storm in an automation run consumes host resources unbounded.
- Guardrails (`tools/security.go`, command whitelist, blocked paths, output redaction) are
  **string inspection of LLM output** — a correctness layer, not isolation. Any whitelist
  miss = full host access at the backend process user's privileges.
- `sandboxing.network` does not exist: when the agent is allowed to use the network, that
  applies uniformly (fetch/scan/connectors), but **shell commands make raw sockets with no
  enforcement at all**.

The agent runs whatever the model emits — dev work, network scanning, Telegram checks,
unattended automation. A prompt-injected or simply confused model must not be able to
damage or read anything outside the workspace. Today it can.

## 2. Threat model (what "secure" means here — decided explicitly)

**In scope (this plan defends against):**

| Threat | Example | Blocked by |
|---|---|---|
| Exfiltration of host secrets | `cat ~/.ssh/id_rsa`, `~/.zsh_history`, browser cookies | FS jail (kernel-enforced read confinement) |
| Persistence on host | launchd agents, crontabs, shell rc edits, SSH keys | FS write confinement |
| Damage outside workspace | `rm -rf ~/`, wiping `/usr/local` | FS write confinement |
| Resource exhaustion | fork bombs, memory bombs, disk-fill via `/tmp` | rlimits (RLIMIT_NPROC / RLIMIT_AS / RLIMIT_FSIZE) — FSIZE is per-file, AS caps *virtual* space not RSS; aggregate disk is Phase-4 accounting (best-effort) |
| Unbounded network abuse by shell | beaconing, port-scanning the LAN when network is off | OS network denial — macOS `(deny network*)` is total; Linux Landlock is TCP-only (UDP escapes), strictness needs bwrap — plus Go-layer toggle |
| `.sandbox` runtime leaking into model context | already solved — `sandbox-runtime-invisibility` plan | unchanged, preserved |

**Out of scope (documented, not defended here):**

1. **Backend process compromise** — the OS sandbox applies to *child* processes (the
   shell, spawned tools). If the model finds a bug *in the Go backend itself*, the kernel
   sandbox does not help. Primary mitigation: run the backend as a dedicated unprivileged
   user (Phase 6 documentation + systemd hardening; no code).
2. **Exfiltration via allowed channels.** If `sandboxing.network: true`, the agent can
   POST workspace contents anywhere. Kernel sandboxes can do "network off" but **not**
   "only api.telegram.org" — no domain-level egress filtering exists in Seatbelt,
   Landlock, or bubblewrap. Optional mitigation: Phase 5 egress-proxy mode (route all
   agent egress through the local proxy and deny direct connect). This fits the repo's
   purpose (an LLM proxy) and is the only local, non-cloud way to get domain-level egress.
3. **Kernel 0-days.** Theoretical for this project; not defended.
4. **MCP tool execution (SPEC-008).** The jail constrains the backend's *own* children
   (shell, spawned tools). Tool calls executed by an **MCP server** are not OS-sandboxed:
   `nodeherder` dials servers via the guarded `DialContext`, but the server runs in its
   own process (possibly remote) outside the shell jail. MCP servers are
   operator-configured, trusted integrations; documented here, not defended.

**Why not Docker/containers:** evaluated and rejected as the default (see §4 Decisions).
Summary: on macOS Docker is a Linux VM whose workspace bind-mount I/O is 5–20× slower —
exactly the agent's hot path (`npm install`, `go build`, `git status`) — plus a hard
external dependency (Docker Desktop licensing, daemon lifecycle) and image maintenance for
tool availability. On Linux it is stronger, so it remains available as a *future optional
executor*, never the default.

**Why not WASM (Wazero) again — prior attempt history:** this repo already shipped a
Wazero/WASI-based sandbox (`backend/internal/sandbox/`, `sandbox.go` + `wazero.go`, plus
a `SandboxMonitor.vue`), added 2026-04-22/25 (`5211572`, `e3254d5`) and **removed
2026-05-14** (`febdc99`) with no recorded rationale — the removal also renamed
`SandboxMonitor.vue` → `TerminalMonitor.vue`. The archived
`system-blueprint.md:54` still lists "Advanced Sandboxing: Migrating more tools to the
Wazero-based WASM sandbox" as future work (stale). A WASM sandbox only isolates
WASM-compiled tool bodies — it cannot constrain arbitrary host binaries (`node`, `go`,
`npm`, `git`), which are exactly the agent's hot path, so it does not close the threat
model in §2. Not revisited here.

**Why not per-use-case profiles:** a policy matrix ("dev-work", "network-recon", "comms"
profiles with per-domain allowlists) was considered and rejected. The operator's mental
model is one bit — "may the agent use the network" — not a matrix. Profiles rot (every new
use case needs a new profile, and a missing profile silently blocks the agent → stuck-loop
behavior this project fought in the safety-hardening plan). Domain-level allowlists are
not kernel-enforceable anyway (see threat model item 2). Design principle inherited from
the guardrail engine: **hard invariants and blocklists, not hand-crafted per-scenario
policies.**

## 3. Current state inventory (what we build on)

| Existing mechanism | Location | Kept? |
|---|---|---|
| `sandboxing.enabled` toggle (bootstrap fatals when false) | `models/host_settings.go`, `app/bootstrap.go:138` | Replaced by 3-switch model below; the fatal-on-disabled behavior becomes "env floor" (see Phase 1) |
| HOME/`GOPATH`/`TMPDIR` redirection into `.sandbox` | `internal/shell/terminal.go:377` | Kept as-is (env floor) |
| Env allowlist + curated PATH | `prepareShellEnv()` | Kept as-is |
| Persistent shell, Setpgid, kill-group, procwatch | `internal/shell/shell.go`, `terminal.go` | Kept; sandbox hooks attach at shell creation (single call site) |
| Guardrail engine, command whitelist, blocked paths, output redaction | `tools/security.go`, SPEC-006 | Kept; OS layer is defense-in-depth *underneath* it, never a replacement |
| `NetworkGuardrailsConfig` (CIDR/port lists, SSRF checks) | `tools/network.go` | Kept as-is; the new `network` switch gates *whether* these tools run, not *how* they filter |
| `MaxMemoryMB` / `MaxStorageGB` config fields | `models/host_settings.go` | Finally enforced — memory via rlimits (best-effort on Darwin), storage via Phase-4 accounting, not kernel quota |
| `Functional` effective-state reporting pattern | `app/app_context_system.go:47` | Reused for `Effective` sandbox state |

Key fact driving the design: **in-process tools cannot be OS-sandboxed.** `network_fetch`,
`scan_local_network`, `search`, and communication connectors run inside the backend
process; Landlock/Seatbelt only constrain the child processes we spawn. So the network
switch must have *two* enforcement points (Go choke point for in-process tools, OS layer
for shell) — this is not duplication, it is the only correct layering.

## 4. Design

### 4.1 Configuration (replaces `HostSandboxingConfig` fields)

```yaml
sandboxing:
  enabled: true        # master switch — semantics CHANGED vs today: today it gates terminal
                       #   availability (bootstrap fatal when false, UI toggle "Enable
                       #   Persistent Terminals"); after Phase 1 it becomes the OS-jail
                       #   master with an explicit env-floor mode (decision in Phase 1)
  filesystem: true     # OS-level FS jail: agent processes may write only their
                       #   workspace (+ .sandbox runtime dirs); host is read-only
  network: true        # agent may use the network AT ALL — terminal raw sockets,
                       #   fetch_url, scan_local_network, search, connectors
  max_memory_mb: 256   # RLIMIT_AS caps VIRTUAL address space, not RSS — 256 MB default
                       #   breaks node/V8/Go (multi-GB VA reservations): recalibrate or use
                       #   RLIMIT_DATA/RSS where the OS enforces them; Darwin best-effort
  max_storage_gb: 2    # Phase 4 workspace-size accounting — best-effort boundary check
                       #   (TOCTOU/bypassable window), NOT a kernel-enforced quota
```

Three switches, each with exactly one enforcement layer per surface:

| Switch | Terminal (child proc) | In-process tools |
|---|---|---|
| `filesystem` | Landlock (Linux) / Seatbelt (macOS) | `tools/filesystem.go` already jails paths — unchanged |
| `network` | Seatbelt network deny (macOS) / Landlock TCP (Linux ≥6.7) / bwrap `--unshare-net` when available | choke point in `NetworkTools` + connectors refuse to dial |
| `enabled` (resource) | rlimits on every spawned shell | existing per-tool timeouts |

No per-workspace modes, no profiles. The workspace-level `guardrails:` override stack
(SPEC-006 §II.2 — Override Stack) stays the only place where *tightening* beyond these
switches happens.

**Additive-config hazard (must-fix before Phase 1 ships):** `Filesystem`/`Network` are
new bool fields on `HostSandboxingConfig`, which today carries JSON tags only — the
on-disk `settings.yml` keys are the lowercased field names (`maxstoragegb`,
`maxmemorymb`, `functional`), not the snake_case keys shown above. Existing files that
predate the new keys unmarshal to `false`, and the whole-section default-merge
(`storage/manager.go:322`) fires only when the section is entirely zero — so an
`enabled: true` file would silently boot with the OS jail off **and** network off. Fix:
`*bool` with read-time defaulting, or an explicit per-key backfill in
`mergeAppConfigDefaults` (mirroring the Metrics merge, `manager.go:304-321`), plus a
regression test in the style of `TestRunLoggingDefaultAndBackfill`
(`app_config_store_test.go:189`).

### 4.2 Enforcement matrix (what the kernel can actually do)

| OS | FS jail | Network deny | Notes |
|---|---|---|---|
| macOS (all supported) | `sandbox-exec` Seatbelt profile | Seatbelt `(deny network*)` (all socket families) | Deprecated by Apple but kernel-enforced, zero deps; Seatbelt profiles are battle-tested (Codex CLI uses `sandbox-exec`; Chromium uses Seatbelt via its own launcher, not the CLI). No public successor exists for arbitrary child processes. |
| Linux ≥ 6.7 (network rules are Landlock ABI v4) | Landlock | Landlock TCP bind/connect | Pure syscalls, unprivileged, zero deps. **TCP-only: UDP (DNS/53, QUIC) is unconstrained by every released kernel** (UDP ACLs are an unmerged RFC), so this is on/off for TCP only; `Effective` reports `tcp-only`; bwrap `--unshare-net` is the only strict network-off on Linux. |
| Linux 5.13–6.6 | Landlock | ❌ kernel can't | FS jail only; network falls back to bwrap if present, else Go-layer only (weakest link — reported honestly in `Effective`) |
| Linux (optional strict) | + bubblewrap | `--unshare-net` | Requires `bwrap` binary + unprivileged userns enabled (Ubuntu 24.04+/Debian 12 AppArmor restrictions). Capability-detected, never assumed. **Precedence: Landlock FS is the Linux default; bwrap only when Landlock FS is absent (kernel <5.13) and binary+userns exist. bwrap is all-or-nothing — `--unshare-net` forces `network:false`; it cannot express "network on, LAN off".** |

**Fail semantics: downgrade, never bypass.** If the requested enforcement is unavailable
on the host OS, the sandbox starts with the strongest available subset and reports the
*effective* state — mirroring the existing `Functional` flag pattern
(`app/app_context_system.go:47`). Silent absence of enforcement is a security bug;
effective-vs-requested must be visible in settings UI and logs.

### 4.3 Package layout (single wiring point)

New package `backend/internal/sandbox/`:

```
sandbox/
  sandbox.go        // Config, Provider interface, Effective state, New(ctx, cfg) —
                    //   capability detection happens ONCE here at bootstrap
  rlimit.go         // shared rlimit application (RLIMIT_AS/FSIZE/NPROC) — both OS
  rlimit_unix.go    // syscall impl (build-tagged; darwin+linux share the unix impl)
  seatbelt_darwin.go// Seatbelt profile generation + `sandbox-exec -p` command wrapping
  runner_linux.go    // sandbox-runner helper: installs Landlock (+rlimits) in a child,
                     //   then syscall.Exec's the real argv (see mechanism note below)
  landlock_linux.go  // GoLandlock: FS rules (+ TCP rules on ABI v4 / kernel ≥6.7)
  bwrap_linux.go     // Phase 5: optional bwrap argument builder
  profilegen.go      // per-workspace profile text generation (workspace path, .sandbox dirs)
```

Interface (one method, called from ONE place):

```go
type Provider interface {
    // Wrap mutates the prepared exec.Cmd (path, args, env, SysProcAttr) so the
    // child runs under the OS sandbox for the given workspace.
    Wrap(cmd *exec.Cmd, ws WorkspaceView) error
    // Effective returns what is actually enforced on this host, stamped with the
    // policy epoch it was created under (see Risks item 6 — hot-updated settings
    // must recycle long-lived shells, or Effective must reflect the stale epoch).
    Effective() EffectiveState // what is actually enforced on this host
}
```

Wiring: `internal/shell.newPersistentShell` already builds the `exec.Cmd`; it receives the
`Provider` (or a no-op when `filesystem:false`) and calls `Wrap` before `cmd.Start()`.
This satisfies the centralization rule — sandbox wiring lives in bootstrap → app context →
shell factory, one discoverable chain, no ad-hoc enforcement sprinkled in tools.

**Mechanism note (Linux):** Go's `os/exec` has **no pre-exec hook** and Landlock is
self-confining (`landlock_restrict_self` applies only to the calling process), so the jail
cannot be installed on a child from the parent. Linux therefore needs a small
`sandbox-runner` binary built with the backend: `Wrap` replaces `argv[0]` with the runner,
the runner installs Landlock FS rules (+ rlimits) and then `syscall.Exec`s the real
command (`bash`). Seatbelt is unaffected — `sandbox-exec -p` is already an external
wrapper. Same mechanism carries rlimits (RLIMIT_AS/FSIZE/NPROC) on both OSes; rlimits can
also be injected as a `ulimit -Hv/-Hu/-Hf` preamble on the persistent-shell stdin (hard
limits set by a non-root child can only be lowered, which is all we need), but the runner
is the single consistent path.

**Dependency note:** first-class rlimit/Landlock code promotes `golang.org/x/sys`
v0.42.0 (currently `// indirect`, `backend/go.mod:39`) to a direct dependency, or adds the
go-landlock module — per AGENTS/CONSTITUTION heavy-dependency rules, this must be
explicitly approved before Phase 1 (hand-rolling via `syscall.Syscall` avoids the
promotion).

Generated Seatbelt profiles are **Go string templates rendered per workspace** (workspace
abs path, `.sandbox/tmp` etc. inserted at render time) — never static files, keeping the
single-root data layout (XDG relocation plan) intact.

### 4.4 Go-layer network choke point

`sandboxing.network: false` enforcement for in-process tools:

- **Scope boundary (do not gate infrastructure):** the gate lives at agent-tool entry
  points only — `NetworkTools`, search, and communication connectors. The proxy's own
  outbound traffic is exempt by construction: provider calls ride the pooled
  `proxy.SharedTransport` (`internal/core/proxy/client.go:133`, Constitution I.2
  carve-out), which must NOT be gated or the app stops serving model requests. Same for
  connector webhook re-registration dialing. The gate is per-tool-call, never at the
  transport layer.
- `NetworkTools` (`tools/network.go`): `FetchURL`, `ScanLocalNetwork`, `GetNetworkInfo`
  return a **hard, non-approvable denial** before any dial. Implemented once in
  `Config()`/a gate helper, not per-method. **This denial must be a synchronous
  security-boundary rejection** (SPEC-006 II.3 path), NOT a guardrail violation routed to
  the approval flow: an approvable network denial is a bypass (click allow → egress
  re-enabled) and an unattended run would stall the full 5-minute
  `GuardrailApprovalTimeout` (the 2026-08-31 `wc`-block incident,
  `agent_test.go:3907`). Schema consistency: feed `network:false` into the existing
  `DisabledToolNames` narrow waist (`guardrails/guardrails.go:345`) so the tools vanish
  from the agent's schema entirely (SPEC-006 II.6) — denial is then defense-in-depth, not
  the primary gate.
- Connectors (`internal/.../connectors`, communication): refuse agent-initiated outbound
  dialing with the same denial. Inbound webhook *receipt* is unaffected (server-side, not
  agent egress) — but connector *sends* (auto-reply, triggered messages) are outbound and
  ARE blocked by `network:false`: state that as intended behavior in the UI, and note the
  interaction with the active connector-auto-reply plan.
- Search (`tools/search.go`): gated identically at the tool's own entry (it holds its own
  client, `search.go:25-35`).
- Terminal: OS layer handles raw sockets (see matrix); when the OS layer cannot do network
  (Linux 5.13–6.6 without bwrap → `go-only`; Linux ≥6.7 Landlock-only → `tcp-only`, UDP
  escapes), the effective state reports it so the operator knows shell sockets are not
  fully kernel-blocked.

Hardening invariant: the network switch **tightens**; user `guardrails:` overrides may
tighten further (blocked CIDRs etc.) but nothing in the override stack may re-enable a
switch that is off.

## 5. Phases

TDD throughout (red→green→refactor; `docs/skills/tdd-guide.md`). Backend phases require
`go build ./... && go test ./...` + `go run ./tools/check-complexity/` (complexity ≤12 —
the profile generators and Wrap functions are the watch-list). Acceptance criteria must
name concrete tests (per the sibling `sandbox-runtime-invisibility.md` convention, not
prose-only): each phase lists its test functions and the OS × kernel-ABI × mode matrix it
exercises. Capability detection is decomposed into one small `detect*` per mechanism;
`profilegen.go` stays pure string generation with no branching; `Wrap` is a switch over
the resolved policy — so no single function drifts past the complexity gate.

### Phase 1 — rlimits + config plumbing (both OS, smallest, ships value immediately)
- `HostSandboxingConfig`: add `Filesystem`, `Network` as **`*bool` with read-time
  defaulting** (or explicit per-key backfill in `mergeAppConfigDefaults`) plus `yaml`
  tags; keep `Enabled` as master. See additive-config hazard in §4.1 — plain bools would
  silently boot existing installs with jail off and network off.
- **Decide `Enabled` semantics explicitly** (H4): today `enabled:false` is a bootstrap
  fatal tied to terminal availability ("Enable Persistent Terminals" UI toggle); after
  this plan it becomes the OS-jail master with an env-floor mode. If `enabled:false`
  becomes bootable, require a loud opt-in/effective-state warning when the new switches
  are off — a security-relevant behavior change on a path that is fail-closed today
  needs its own acceptance criterion.
- `sandbox` package skeleton: `Config`, `Provider`, `EffectiveState`, `New()`.
- rlimit application: `RLIMIT_AS = MaxMemoryMB`, `RLIMIT_FSIZE`, `RLIMIT_NPROC` (deny fork
  bombs) on every `persistentShell` start and every `executeLocal` process, via the
  sandbox-runner (Linux) or a `ulimit -Hv/-Hu/-Hf` preamble on the shell stdin
  (macOS) — see §4.3 mechanism note. **`RLIMIT_AS` caps virtual address space, not RSS**:
  256 MB default breaks node/V8/Go toolchains (multi-GB VA reservations) — recalibrate
  the default (or enforce via `RLIMIT_DATA`/cgroup where available) and mark memory caps
  **best-effort on Darwin** with a measured acceptance test, not a hard promise.
- Bootstrap validation: invalid combos logged; `Effective` surfaced through
  `app_context_system.go` next to `Functional` — as a read-time projection, never
  persisted back into settings.yml (runtime state must not leak into the merged doc).
- **Acceptance:** a shell command that tries to allocate > MaxMemoryMB is killed **and**
  `node -e`, `go build`, `npm install` still succeed under the default limit; config
  toggles persist across reload with absent keys defaulting to true; `Enabled:false`
  boots only with a visible effective-state warning; unit tests for rlimit math and
  config resolution.

### Phase 2 — OS filesystem jail
- macOS: Seatbelt profile generator (`profilegen.go` + `seatbelt_darwin.go`): allow rw on
  workspace + `.sandbox` subdirs, ro on system paths, deny everything else; spawn via
  `sandbox-exec -p '<profile>' bash --norc --noprofile -s`.
- Linux: GoLandlock FS rules (same grant set) installed by the sandbox-runner child before
  `syscall.Exec` (no pre-exec hook exists in Go — §4.3 mechanism note).
- **Grant/deny sets are spelled out, not "allow rw / ro / deny else"** (H3) — this is
  where jail breaks hide. Grants: workspace + `.sandbox` rw; `path_extensions` dirs from
  `prepareShellEnv` (e.g. `.sandbox/node_modules/.bin`) stay writable + executable;
  `/dev/null|zero|urandom|random`; ro `/proc` on Linux (Go runtime reads it); ro
  `/etc/resolv.conf`, `/etc/hosts`, `/etc/ssl` + CA bundles (git/curl https); macOS
  Security framework paths. Explicit **deny list for socket escape hatches**:
  `$SSH_AUTH_SOCK`, `~/.ssh`, `~/.aws`, `~/.config/gcloud`, `/var/run/docker.sock`.
  Named invariant: **`docker.sock` reachable + `network:true` = trivial escape to host
  root via the Docker API** — docker.sock must be denied whenever present on the host.
- Capability detection at `New()`; effective state reports `filesystem: seatbelt|landlock|off`.
- Preserve `.sandbox` invisibility invariants — the sandbox package *grants* the runtime
  dirs; the redaction layer (tools/security.go) is untouched; the jail never re-exposes
  `.sandbox` to listings/output (regression-test the merged `internalBlockedPaths` under
  a jailed child).
- **Acceptance:** probe tests — child tries `cat ~/.ssh/id_rsa` (or `$HOME/../id_rsa`) →
  EPERM/EPERM-equivalent; write outside workspace fails; workspace + `.sandbox` writes
  succeed; socket-hatch paths (`docker.sock`, `$SSH_AUTH_SOCK`) read-fail. Tests are
  build-tagged per OS and skip when kernel/lib absent (with `Effective` reflecting the
  skip).

### Phase 3 — network switch (Go choke point)
- `network: false` feeds `DisabledToolNames` (guardrails/guardrails.go:345 → narrow waist
  in tool_availability.go, SPEC-006 II.6) so fetch/scan/search/connector-send tools are
  **schema-hidden from the agent**, and the execution-time hard denial in §4.4 remains as
  defense-in-depth for any residual path.
- Frontend: sandboxing switches in host settings (effective-state badges:
  `seatbelt`, `landlock`, `tcp-only`, `go-only`, `unavailable`); follow the
  `docs/architecture.md` settings-tab checklist; note the JSON shape change is a
  frontend-visible contract (SecuritySettings.vue projects `sandboxing.*`) and keep
  runtime effective state out of the persisted shape.
- **Acceptance:** with `network:false`, the disabled tools never appear in `ListTools`;
  any residual call fails fast with a **synchronous, non-approvable** denial that reaches
  the agent as a tool result (record-and-continue — architecture.md pitfall 31) and never
  enters the approval flow; terminal raw-socket denial verified where the OS layer
  supports it.

### Phase 4 — OS network enforcement + storage accounting
- macOS: Seatbelt `(deny network*)` when `network:false` (profile variant) — covers all
  socket families.
- Linux ≥6.7: Landlock TCP bind/connect deny-all when `network:false` — **TCP only; UDP
  (DNS/53, QUIC) stays open**, so `Effective` reports `tcp-only`; bwrap `--unshare-net`
  is the only strict Linux network-off (see §4.2).
- Linux with bwrap present: `--unshare-net` (strictest; detected, optional; all-or-
  nothing, see §4.2 precedence).
- `max_storage_gb`: **honest labeling — this is accounting, not kernel-enforced quota.**
  Per-file size is already capped by the existing filesystem-tool `MaxFileSizeKB`
  (`tools/filesystem.go:233-307`). Add `RLIMIT_FSIZE` (per-file kernel backstop) + an
  async workspace-size accounting loop surfaced in `Effective`, and hard-block
  shell-spawn/filesystem writes only past a cached threshold. Drop the word "quota" for
  the async control; a du-on-every-spawn check is expensive on large repos and
  TOCTOU-weak against a single `dd`/`npm install` run.
- **Acceptance:** strict matrix tests per OS row in §4.2 (assert `tcp-only` vs `go-only`
  vs `seatbelt` per host); storage-accounting test: boundary enforced, cached window
  documented, no false claim of hard enforcement.

### Phase 5 — egress proxy mode (optional, off by default)
- `sandboxing.egress_proxy: false|port`. When on: shell env sets `HTTPS_PROXY/HTTP_PROXY`
  to the local proxy; Seatbelt/Landlock deny *direct* connect except to the proxy
  listener. Proxy applies domain policy (this is the only local way to get
  domain-level egress filtering — see §2 item 2). **The proxy must also be the egress
  path for in-process tools** (NetworkTools/search transport pointed at it) — otherwise
  the shell is proxied but `fetch_url` still dials direct, defeating the mode.
- **Acceptance:** direct-dial probes fail; proxied egress works; disabled by default.

### Phase 6 — deployment hardening (docs + service unit, no Go code)
- `docs/service_setup.md` + `docs/services/llm-proxy.service`: dedicated unprivileged user,
  `NoNewPrivileges=true`, `ProtectSystem=strict`, `PrivateTmp=true` (systemd covers Linux
  backend-process containment — complements out-of-scope item 1 in §2).
- Env-secret audit of `prepareShellEnv` allowlist (connector API keys must not leak into
  shell env).

## 6. Risks / known limitations

1. **Seatbelt deprecation (macOS):** accepted. No replacement for arbitrary child
   processes; if Apple removes it, the `Effective` state degrades visibly (not silently)
   and Landlock-style APIs would be re-evaluated then.
2. **Landlock network requires kernel ≥6.7 and is TCP-only (UDP escapes even then):**
   hosts below that get `go-only`; hosts on 6.7+ Landlock-only get `tcp-only` — honest
   reporting + Phase 5 proxy (or bwrap `--unshare-net`) mitigate.
3. **CLI breakage under Seatbelt:** some tools probe odd paths. Mitigation: profiles
   start from a conservative allowlist; failures surface as tool errors (record-and-
   continue) and the profile generator keeps a small extension point list.
4. **`executeLocal` path** must receive the same Wrap treatment as the pooled shell —
   both spawn children (safety-hardening Step 2 established Setpgid there; the same hook
   applies).
5. **Tests running under sandboxed children on CI:** capability-detected skips with
   explicit `Effective` assertions so matrix coverage is visible, not silent — and the CI
   note must state which Landlock ABI the runner kernel actually exercises, else the
   whole matrix silently skips.
6. **No enforcement lifecycle (policy skew):** host settings are hot-updatable
   (`UpdateHostSettings`), but a `persistentShell` was spawned under the policy in force
   at creation. Flipping `filesystem`/`network` leaves long-lived shells enforcing the
   old policy while the UI claims the new one. Fix: policy epoch on `Provider`,
   recycle affected workspace shells on change (kill-group/procwatch already exist), and
   `Effective` carries the epoch it reflects.
7. **Wrapper identity:** spawning via `sandbox-exec`/runner makes `cmd.Process.Pid`/
   `PGID`/procwatch observe the wrapper, not bash — re-verify kill-group/`StopAutomation`
   semantics under the wrapper (Setpgid must apply to the real shell group).

## 7. Rollback

Each switch is independent; `sandboxing.enabled: false` restores today's behavior minus
the bootstrap fatal — **note this is itself a behavior change on a security path**
(today `enabled:false` refuses to boot) and must carry a loud opt-in/effective-state
warning when the new switches are off (Phase 1 acceptance). No data migration; config is
additive **only with `*bool` + read-time defaulting or explicit per-key backfill**
(`Filesystem`/`Network` default to true when `enabled: true`, matching current defaults'
intent — plain bools would boot every existing install with the jail off and network
off; see §4.1 additive-config hazard).

## 8. SPEC impact

- SPEC-006 (Guardrail Engine): add "OS Enforcement Layer" subsection — the sandbox is a
  layer *under* guardrails, effective-state reporting contract, downgrade-never-bypass
  invariant, and the hard-denial/approval-flow boundary (§4.4). SPEC-006 is `stable`, so
  per `docs/SPEC-change-management.md` the addition needs a version bump + change record;
  also fix SPEC-006's own frontmatter `constitution_references: [I.5]` (no such item —
  the sandbox law is Constitution II.3) while touching it.
- `docs/architecture.md`: sandbox package in the directory map + "adding a sandbox
  enforcement" checklist pointer; mark `system-blueprint.md:54`'s stale Wazero
  future-work line superseded.