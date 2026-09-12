---
status: complete (pending Linux-CI runtime confirmation of the Landlock probe + optional macOS seatbelt on framework-capable hardware)
current_phase: "PLAN IMPLEMENTED for the shipped scope (Phases 0–4). Residual (environmental/optional): Landlock + mapping runtime tests auto-run on Linux CI; per-child rlimits for non-service Linux runs; macOS Seatbelt requires a host with the Sandbox framework (uid-first launchd covers macOS production)."
date: 2026-09-06
last_implementation_update: 2026-09-06
revision: 3
revision_summary: "Rewritten after design review (2026-09-06): network-default-off + egress proxy promoted to core, FS-jail re-scoped to per-workspace isolation (Linux prod already runs as a dedicated unprivileged user — setup.sh/systemd), macOS moves off the deprecated sandbox-exec CLI to a uid-first + Sandbox-API strategy, pure-Go phases re-ranked ahead of OS layers; single agent-action pipeline with the OS jail and egress proxy as execution backends behind one dispatch stage (D7); measured platform facts folded in. Implementation began 2026-09-06 — see §10 progress log."
related_specs: [SPEC-006]
constitution_references: [II.3]
related_plans: [cross-cutting/sandbox-runtime-invisibility.md, unattended-run-safety-hardening.md, cross-cutting/xdg-config-data-relocation.md]
supersedes_notes: "Revision 1 (2026-08-31). Kept unchanged: threat-model framing, why-not-Docker/WASM/profiles rationale, Landlock/Seatbelt mechanics, wrapper/runner mechanism notes. Changed: decisions D1–D8, deployment reality, phase order, macOS mechanism, network semantics, egress proxy, grant model, unified pipeline, UI design."
---

# Agent OS Sandboxing (kernel-enforced containment — rev 2: network-first, uid-first)

## 0. What changed in this revision (decisions log)

This plan was reviewed line-by-line against the codebase (2026-09-06) and rewritten. The
review validated most of rev 1's technical claims (see §3) but changed its center of
gravity. **Decisions recorded here override rev 1 wherever they conflict:**

| # | Decision | Overrides |
|---|---|---|
| D1 | **Agent-initiated network is OFF by default**, granted per workspace / per automation run, instead of rev 1's `network: true` (matches-today) default. Network — not file access — is the residual #1 threat after deployment containment. | rev 1 §4.1 `network: true` |
| D2 | **Egress proxy promoted from "optional Phase 5" to a core phase (P1).** It is the only mechanism that gives *domain-level* network policy and it closes the Linux-UDP-escape + old-kernel gaps uniformly at the app layer. | rev 1 §4.1/Phase 5 |
| D3 | **The per-process OS FS jail is re-scoped.** Its primary job is **per-workspace isolation** (agent in workspace A must not read workspace B) and **containment on non-production runs** (dev/desktop where the backend runs as the operator). Production Linux already runs the whole service as a dedicated unprivileged user (setup.sh + systemd, `User=llm-proxy`) — host-secret reads are already blocked *by Unix permissions* there. The OS layer is therefore **thin**, and the UI/`Effective` state must say so. | rev 1 §2 framing ("FS jail is the main event") |
| D4 | **macOS does not depend on the deprecated `sandbox-exec` CLI.** Preferred macOS containment is the same dedicated-user model Linux uses (launchd daemon running as an unprivileged user). The remaining per-workspace seatbelt layer, where unavoidable (dev-mode runs as the operator), uses the **Sandbox framework API** via a small helper, not the deprecated CLI — and is isolated behind the Provider so it can degrade visibly, never silently. | rev 1 §4.2/Phase 2 (seatbelt via `sandbox-exec -p`) |
| D5 | **Resources are enforced where the OS actually enforces them** (measured): Linux systemd service gets real cgroup v2 limits (`MemoryMax`, `TasksMax`); rlimits only where settable; **macOS cannot set `RLIMIT_AS`/`RLIMIT_DATA` at all** (measured) — memory stays honest "best-effort", fork-bombs/files capped via `RLIMIT_NPROC`/`RLIMIT_FSIZE` (measured working). | rev 1 §4.1 Phase 1 rlimit claims |
| D6 | **Additive-config hazard fixed with per-key backfill** (+`yaml` tags), not plain bools. Verified: yaml.v3 ignores `json:` tags and lowercases field names, so today's on-disk keys are `maxstoragegb`/`maxmemorymb`; the whole-section merge at `storage/manager.go:322` would silently boot every existing install with new bools at `false`. | rev 1 §4.1 fix option |
| D7 | **No new "layers" — one agent-action pipeline + two execution backends.** All *decision-time* checks are stages (S0–S3) of the single stream every tool call already traverses; the OS jail and egress proxy are *execution-time backends* behind one dispatch stage (S4), selected by a per-run policy snapshot (S1). Nothing new inspects independently; nothing new re-derives policy from hot config. | rev 1/rev 2 "layering" framing (§4.2) |
| D8 | **Shell pooling keyed by (workspace, network-on); recycle only on epoch.** Grants are per-run, shells are long-lived, and OS network state for a process is binary — so the pool key becomes `(workspaceID, networkOn)` (≤2 shells/workspace). LAN-vs-internet is an *app-layer* concept (in-process tool guardrails + egress policy + the guarded `scan_local_network` tool), never an OS shell state. Scope transitions between runs need no recycle (different key; stale-scope shell idle-reaped); `Recycle` is reserved for epoch changes (filesystem toggle, capability change, hot settings). Documented residual: a shell under `networkOn=true` reaches both LAN and internet — the OS cannot split them. | rev 1/rev 2 "one shell per workspace" pool assumption |

Unchanged decisions from rev 1, still endorsed: no Docker default (macOS VM bind-mount I/O),
no WASM (cannot constrain host binaries), no per-use-case profiles (one bit + per-run grants),
downgrade-never-bypass + honest `Effective` reporting, single `Provider` wiring point.

---

## 1. Problem

Today's "sandbox" (`backend/internal/shell/terminal.go`, `workspaceEnvTemplates`) is an
**environment-level jail, not a security boundary**. What is and isn't enforced today:

- **Genuinely enforced (kept, unchanged):** `IsSecurePath` path checks jail the *filesystem
  tools* to the workspace (symlink-resolving, `tools/security.go:29`); terminal cwd is
  workspace-confined; `ValidateTerminalCommand` (`core/tools/terminal.go:96`) is a real text
  validator (blocks absolute paths outside the jail, `..` traversal, blocked filenames,
  recurses into `sh -c`/`bash -c`). The `.sandbox` runtime-dir invisibility invariant and the
  env allowlist in `prepareShellEnv` stay.
- **NOT a boundary (the gap):** that terminal validator is **string inspection of the model's
  output**. Once text passes, the command executes with the backend's full user privileges
  and the OS has no idea any jail exists. Demonstrated on this machine (replicating the exact
  env jail, 2026-09-06):
  - **Symlink escape:** the agent may write files in the workspace (it must). `ln -s ~/.ssh workspace/k; cat workspace/k` references a jail-internal path — no absolute path, no `..`; nothing for the validator to catch. The read lands outside.
  - **Indirect execution:** the validator sees `bash run.sh` / `npm install` / `git clone` — it cannot see the *contents* of the script, the package's postinstall, or the hook. The agent's dev-work hot path is exactly "execute code whose content the filter never saw", and the agent controls that content.
  - **Writes outside the workspace** (absolute `/tmp`, …) succeed from the env-jailed shell.
  - **Raw sockets from the shell** succeed with zero enforcement (`curl https://…`).
- **Configured but never enforced:** `HostSandboxingConfig.MaxMemoryMB` / `MaxStorageGB`
  (`models/host_settings.go`) are decorative — no `setrlimit`, no cgroup anywhere.
- **No `sandboxing.network`:** guardrail network controls cover the *in-process* tools
  (`NetworkTools`, `tools/network.go:93/299/316`) but shell sockets have no control at all.

**Two deployment realities that reshape the threat model:**

1. **Linux production already runs the entire service as a dedicated unprivileged user.**
   `setup.sh` installs a locked system account (`llm-proxy`, `--shell /usr/sbin/nologin`),
   and `docs/services/llm-proxy.service` runs the backend *and every agent-spawned child* as
   that user with `NoNewPrivileges=true`, `ProtectSystem=strict`, `ProtectHome=read-only`,
   `PrivateTmp=true`, one writable root (`ReadWritePaths=/var/lib/llm-proxy`),
   `CapabilityBoundingSet=` empty, `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`.
   External model/binary paths are granted to that user via **read-only ACLs**
   (`./setup.sh access`, traverse-only parents). Consequence: in that deployment a rogue agent
   **cannot read `~/.ssh`** — the process lacks the permission *by construction*. What it
   still can do: read every other workspace (all owned by `llm-proxy`), exfiltrate over the
   network, exhaust resources, and dial anywhere from the shell.
2. **macOS / dev runs execute as the operator.** `launch.sh` and any desktop run have no
   dedicated user — the operator's SSH keys, cookies, and dev tree are all readable by agent
   processes. This is where per-process confinement still matters for host secrets.

The original rev-1 problem statement ("a prompt-injected model can read your keys today")
is therefore **already solved on Linux production** and **still open on macOS/dev**, while
**network abuse and cross-workspace reads are open everywhere**. This revision targets that
actual risk distribution.

## 2. Threat model (what "secure" means here — decided explicitly)

**In scope (this plan defends against):**

| Threat | Example | Primary defense (deployment → OS → Go, innermost wins) |
|---|---|---|
| Exfiltration of *workspace* data / beaconing | agent POSTs workspace contents, phones home | **Network default-off (D1) + egress domain policy (D2)** |
| Exfiltration of *host* secrets | `cat ~/.ssh/id_rsa`, browser cookies | **Dedicated-user deployment (Linux: exists; macOS: new, D4)** + FS jail on non-production runs |
| Cross-workspace reads | automation in workspace A reads workspace B | **Per-workspace FS jail** (Landlock / seatbelt / uid boundaries) |
| Persistence on host | launchd agents, crontabs, rc edits | **Dedicated-user deployment** (writable root only); FS write confinement otherwise |
| Damage outside the workspace | `rm -rf` anything not owned | **Dedicated-user + read-only root**; FS jail otherwise |
| Resource exhaustion | fork bombs, memory bombs, disk fill | Linux systemd **`TasksMax`/`MemoryMax`** (real cgroups); rlimits where settable (measured: macOS `NPROC`/`FSIZE` yes, `AS`/`DATA` no); Phase-3 accounting |
| Unbounded network abuse by shell | raw sockets when network off | OS network deny where available (seatbelt full; Landlock TCP ≥6.7) **+ egress proxy for everything else (D2)** — egress proxy is the *uniform* answer |
| `.sandbox` runtime leaking into model context | already solved | `sandbox-runtime-invisibility` plan — unchanged, preserved |

**Out of scope (documented, not defended here):**

1. **Backend process compromise.** The OS layers confine *children* (shell, spawned tools).
   If the model finds a bug in the Go backend itself, kernel sandboxes do not help. Mitigation
   is the dedicated-user deployment (Linux: done; macOS: this plan adds it). This is why
   deployment containment is not a footnote here.
2. **Kernel 0-days.** Theoretical; not defended.
3. **MCP tool execution (SPEC-008).** `nodeherder` dials servers via the guarded
   `DialContext`, but the server process (possibly remote) is outside the jail. MCP servers
   are operator-configured, trusted integrations; documented, not defended.
4. **Domain-level filtering by the OS alone.** No Seatbelt/Landlock/bwrap mode does "only
   api.telegram.org". Solved at the **app layer by the egress proxy (D2)** — the only local
   way to get domain-level egress, and this repo is an LLM proxy by nature.

**Why not Docker/containers (default):** unchanged from rev 1 and re-confirmed. On macOS a
container is a Linux VM whose workspace bind-mount I/O is 5–20× slower — precisely the
agent's hot path (`npm install`, `go build`, `git status`) — plus daemon/licensing/image
dependencies. On Linux it is stronger, so it remains a *future optional executor*, never the
default. **Why not a uniform Linux VM on macOS too (rev-1 "Direction C"):** considered; it is
the only "one mechanism everywhere" that is strong on both, but it forces the slow VM I/O
path onto macOS for zero per-workspace benefit over the native mechanisms. Rejected; the
native mechanisms + uid containment are both faster and sufficient.

**Why not WASM (Wazero) — prior attempt history:** unchanged from rev 1. A WASM sandbox only
isolates WASM-compiled tool bodies, not the host binaries (`node`, `go`, `npm`, `git`) that
are the agent's hot path. Prior Wazero attempt shipped 2026-04 (`5211572`, `e3254d5`) and was
removed 2026-05-14 (`febdc99`). Not revisited.

**Why not per-use-case profiles:** unchanged from rev 1. Operator mental model = one bit
("may the agent use the network") plus, per this revision, **an explicit per-run grant for
automations** (see D1/§4.4). A policy matrix rots and missing profiles cause the stuck-loop
behavior this project already fought.

## 3. Verified findings from the review (2026-09-06)

Every claim below was checked against the code or measured on this host (macOS 26.6.2,
Darwin 25.6, go 1.26.1, node 26). They are **facts the implementation must not re-derive**.

| Finding | Evidence | Where it bites |
|---|---|---|
| yaml.v3 ignores `json:` tags and lowercases field names | Probe reproduced: snake_case keys unmarshal to 0/`false`; `maxstoragegb`/`maxmemorymb` keys parse | On-disk `settings.yml` sandboxing keys are lowercased field names; the whole-section merge (`storage/manager.go:322`) fires only when the struct is entirely zero |
| `RLIMIT_AS` and `RLIMIT_DATA` **cannot be lowered at all** on modern macOS (non-root) | `ulimit -v`/`-d` → "Invalid argument"; Python `resource.setrlimit` → ValueError | rev-1 "memory via rlimits on Darwin" is impossible; must report `Effective: memory not enforced (Darwin)` |
| `RLIMIT_NPROC` **is** settable + enforced on macOS, **per-UID** | Fork test under cap 30 died with `fork: Resource temporarily unavailable` | Fork-bomb defense works but its blast radius is every process of that uid — self-DoS if the backend shares the operator's uid; fixed by dedicated user |
| `RLIMIT_FSIZE` **is** settable + enforced on macOS | 10 KB write under 2 KB cap → `File too large` (errno 27) | Per-file kernel backstop available on Darwin |
| Node reserves >256 MB virtual space even idle; more on Linux | Idle `node` VSZ ≈ 416 MB (arm64, pointer-compression **off**); Linux x64 enables PC → ~4 GB cage | rev-1's 256 MB `RLIMIT_AS` default kills node/go at startup on Linux; recalibrate + acceptance-test `node -e`, `go build`, `npm install` under the default |
| `executeLocal` children get **no env floor** | `core/tools/terminal.go:827` `newCommand` sets no `Env` → inherits backend env (real `HOME`) | Must receive the same env floor + Wrap as the pooled shell, or it leaks more than the pooled shell when `filesystem:false` |
| Landlock is **whitelist-only** (no deny rules); Seatbelt supports deny-over-allow | Mechanism (kernel LSM vs MAC profile) | "ro `$HOME` except `~/.ssh`" is expressible in Seatbelt, **not** Landlock — Linux must enumerate positive grants (toolchain paths incl. nvm/asdf/Homebrew under `$HOME`) and grant traverse on ancestors |
| Linux prod already runs as dedicated unprivileged user | `setup.sh` (`SVC_USER`, `choose_service_user`), `docs/services/llm-proxy.service` | The FS jail's job shrinks to per-workspace isolation + non-production containment (D3) |
| `sandbox-exec(1)` is DEPRECATED (man page); binary still present and functional | `man sandbox-exec` → "DEPRECATED … adopt App Sandbox"; `/usr/bin/sandbox-exec` runs | Drive macOS seatbelt via the Sandbox framework API, not the CLI; keep it behind the Provider (D4) |
| No macOS launchd artifact exists in the repo | `find` → only `setup.sh` (systemd/Linux), `launch.sh`, `docs/services/llm-proxy.service` | New macOS deployment artifact needed (Phase 4) |
| Wazero sandbox removed; SPEC-006 frontmatter cites nonexistent Constitution `I.5` | git log `febdc99`; `docs/SPECS/guardrails.md` frontmatter vs Constitution §I (4 items; sandbox law is II.3) | Fix SPEC-006 frontmatter while touching it (rev 1 said the same) |
| Guardrail `DisabledToolNames` (guardrails.go:345) is the schema narrow waist; network tools gated by `Enabled`/`AllowLanAccess` | `tools/network.go:93/299/316`, SPEC-006 §6 | Feed the new network default into it so tools vanish from the schema in **both** native and XML-text tool modes |
| Agent-triggered outbound all rides the guarded `NetworkTools` client (search client injected `registry.go:242`; connector sends via injected factory `notifiers/telegram.go:193`; MCP dials via guarded `DialContext` wired `bootstrap.go:343`) | traced all client/exec construction | The single-stream claim is verifiable; keep it that way with an audit-invariant test (Phase 1) |
| Raw-client hygiene breaches (none agent-triggered, all must die): `orchestrator/slot_manager.go:62` uses `http.DefaultClient` for llama `/slots` (Constitution I.1 breach — I.2 requires the injected doer); `search.go:35` falls back to `http.DefaultClient`; `admin_handlers.go:507` builds a raw client for Telegram webhook registration | grep | Fix so "zero raw clients in the binary" is grep-enforceable; the egress-proxy phase requires every egress path be observable |

## 4. Design

### 4.1 Configuration (replaces `HostSandboxingConfig` fields)

```yaml
sandboxing:
  enabled: true      # master. Semantics CHANGED vs today (see below)
  filesystem: true   # per-workspace OS FS jail (Landlock/seatbelt). On Linux-prod the
                     #   dedicated user already blocks host writes — this adds
                     #   workspace↔workspace isolation + dev-mode host-secret blocking
  network: false     # HOST MASTER / hard ceiling (D1, R4): OFF by default. When OFF, no
                     #   workspace grant or approval can open network for agent activity
                     #   (enforced OUTSIDE the guardrail MergeWith chain). When ON, the
                     #   existing per-workspace guardrails decide scope. See §4.4 grant model
  egress_proxy: 0    # 0 = off. When on (D2): all agent egress (tools AND shell, via
                     #   HTTP(S)_PROXY) goes through the local proxy, which applies
                     #   domain/CIDR policy. Only domain-level filter that exists.
  max_memory_mb: 2048# Linux: cgroup MemoryMax (service) or RLIMIT_DATA/AS (best-effort
                     #   dev). macOS: NOT ENFORCEABLE (measured) — Effective reports it
  max_storage_gb: 2  # Phase-3 workspace accounting — best-effort boundary, NOT a
                     #   kernel-enforced quota
```

- **`enabled` semantics decision (was rev-1 H4):** today `enabled:false` is a bootstrap
  fatal tied to terminal availability (`bootstrap.go:138`, UI toggle "Enable Persistent
  Terminals"). After this plan it becomes the OS-jail master with an explicit env-floor mode.
  If `enabled:false` becomes bootable, require a loud opt-in/effective-state warning — a
  fail-closed path today must not fail open silently. Decision: keep it bootable only with the
  warning; default stays `true`.
- **`network` default-off migration (D1):** the additive-config hazard now works *for* us —
  an absent `network` key is "undecided". Backfill rule: **new installs default `false`;
  upgrades from a file without the key default to today's effective behavior (allowed) and
  the UI shows a one-time "restrict agent network?" prompt** writing the explicit value. Never
  let an absent key silently mean *either* direction twice: after backfill the key is always
  explicit. Connectors' outbound sends and automation LAN scans are covered by per-run grants
  (§4.4) so fail-closed does not silently break core product function.
- **Additive-config fix (D6):** add `yaml:` tags to the new fields **and** replace the
  whole-section merge at `storage/manager.go:322` with a per-key backfill mirroring the
  Metrics merge (`manager.go:300-321`). Plain bools + whole-struct equality would boot every
  existing install with the new fields at `false` (jail off, network off). Regression test in
  the style of `TestRunLoggingDefaultAndBackfill` (`app_config_store_test.go:189`). The
  `HostSandboxingConfig ==` zero-check compares pointer fields if `*bool` is used — another
  reason to prefer per-key backfill over whole-struct equality.

### 4.2 One action pipeline (decision-time stages) + two execution backends

**Framing (D7):** the sandbox and egress proxy are NOT additional layers that each
inspect independently. Every agent action flows through ONE ordered pipeline. The
first four stages already exist and are unchanged in position — the new work plugs
into them (S0/S1 inputs) and adds two *backends* behind S4:

| Stage | What happens | Mechanism (exists / new) |
|---|---|---|
| **S0 Schema presence** — does the tool exist in this run? | `resolveToolProvider` (`tool_availability.go:16`) filters `ListTools` by `DisabledToolNames` — which now folds in category `Enabled` + `sandboxing.network` + per-run grant. Both native `tools[]` and XML-text tool manuals derive from here. | exists; **new inputs** (D1) |
| **S1 Policy snapshot** — one immutable `EffectivePolicy` per run/turn | {network on/off/grant, filesystem enforced/off + mechanism, memory, resource, epoch}. Resolved once from config + capability detection + workspace override + run grant. Every later stage reads the snapshot; nothing re-reads hot config → no layer drift, no policy skew (R6). | **new** |
| **S2 Per-call validation** | `ValidateToolCall` (`guardrails.go`), `ValidateTerminalCommand` (`terminal.go:96`), `IsSecurePath`, domain/CIDR checks — existing, unchanged position | exists |
| **S3 Approval gate** | `require_review` → SSE banner → `OnGuardrail` decision (`tool_exec.go:402-464`). Security-boundary denials never reach this stage (synchronous, non-approvable — §4.4) | exists |
| **S4 Dispatch → execution** | Two side-effect kinds, each with exactly ONE backend hook: (a) in-process tools consume the snapshot's resolved transport (egress proxy) + config; (b) child processes spawn through `Provider.Wrap` at `exec.Cmd` creation — the single spawn choke point (pooled shell + `executeLocal` — refactor R1). Kernel FS/network enforcement and rlimits/cgroups attach here | exists dispatch; **two new backends** |
| **S5 Result conditioning** | `scrubOutput`/redaction/truncation (`.sandbox` never reaches model context) | exists |

**Why this is not duplication:** decision-time (S0–S3) reasons over *intent* — text,
policy, and the operator's approval — which only Go can do. Execution-time (S4
backends) reasons over *behavior* — syscalls and sockets inside a child that no
longer consults Go policy — which only the OS can do. They are different moments
and cannot be collapsed into one function; they are unified by the S1 snapshot and
by the pipeline's single audit trace (every stage stamps the same run/turn id, so
"why was this blocked?" has one answer: hidden at S0 / denied at S2 / approval at
S3 / kernel `EPERM` at S4).

The per-surface enforcement facts from rev 2 remain, now as S4-backend detail:

| Surface | Dedicated-user deployment (Linux prod; macOS after Phase 4) | Dev / desktop run (no dedicated user) | In-process tools |
|---|---|---|---|
| Host-secret / host-write containment | Unix permissions + read-only root (exists) | OS FS jail — Landlock / seatbelt | `tools/filesystem.go` jails — unchanged |
| Workspace↔workspace isolation | OS FS jail on every shell | OS FS jail (same code path) | path checks — unchanged |
| Network on/off | Egress proxy (uniform) + OS deny where available | same | Go choke point + proxy |
| Resources | systemd `MemoryMax`/`TasksMax` (real) | rlimits where settable; honest `Effective` | per-tool timeouts (exists) |

**Single-stream audit (verified 2026-09-06):** every agent-triggered exec site is one of
the two Wrap points (`terminal.go:828`, `shell/terminal.go:40`); every agent-triggered
egress path rides the guarded `NetworkTools` client (fetch/scan `network.go`, search
client injected `registry.go:242`, connector sends via the injected factory
`notifiers/telegram.go:193`, MCP via guarded `DialContext` wired at `bootstrap.go:343`).
Remaining exec/`http.Client` sites are infrastructure (llama-server lifecycle, metrics,
procwatch, provider doers) and are exempt by Constitution I.2 — but three raw-client
breaches were found (§3) that must be fixed for the pipeline claim to be
*grep-enforceable*: `slot_manager.go:62` `http.DefaultClient` (I.1 breach), `search.go:35`
`DefaultClient` fallback, `admin_handlers.go:507` raw client. Phase 1 adds an
**audit-invariant test** that fails on any new `http.DefaultClient` / bare `exec` outside
the allowlist, so "single stream" stays a tested fact, not a prose claim.

### 4.3 Package layout (single wiring point)

**Package placement (canonical — overrides rev-1's `backend/internal/sandbox/` proposal):**
mirror the existing layout. OS mechanisms live in `platform/*` next to `platform/procwatch`;
app services live in `core/*` next to `core/proxy`. One rule keeps it discoverable: nothing
in `platform/sandbox` imports `core/*`; `core/policy` is the only package that merges host
sandboxing + guardrails + run grants.

```
internal/platform/process/   // EXISTS — add rlimit_unix.go (measured settable matrix)
internal/platform/sandbox/   // NEW — Provider, EffectiveState, New() registry, adapters:
  sandbox.go                 //   Config, Provider interface, Effective, New(ctx,cfg) —
                             //   capability detection ONCE at bootstrap; no-op Provider
                             //   when filesystem:false
  profilegen.go              //   pure string generation (no branching)
  seatbelt_darwin.go         //   SBPL profile render + tiny helper (Sandbox API, NOT the
                             //   deprecated sandbox-exec CLI — D4); build-tagged tests
  runner_linux.go            //   sandbox-runner: Landlock (+rlimits) in a child, then
                             //   syscall.Exec of the real argv
  landlock_linux.go          //   GoLandlock FS rules (+ TCP rules, ABI v4 / kernel ≥6.7)
  bwrap_linux.go             //   optional strict network-off arg builder
  noop.go                    //   identity adapter
internal/core/egress/        // NEW (D2) — local egress-proxy listener + domain/CIDR policy
internal/core/policy/        // NEW (D1/D7) — S1 resolver: EffectivePolicy per run/turn
                             //   (or fold into guardrails — decide in Phase 0)
```

```go
type Provider interface {
    // Wrap mutates the prepared exec.Cmd (path, args, env, SysProcAttr) so the child
    // runs under the OS sandbox for the given workspace. One call site (shell factory
    // + executeLocal — see Risk 4).
    Wrap(cmd *exec.Cmd, ws WorkspaceView) error
    // Effective returns what is actually enforced on this host, stamped with the
    // policy epoch it was created under (see Risk 6 — hot-updated settings must
    // recycle long-lived shells, or Effective must reflect the stale epoch).
    Effective() EffectiveState
}
```

Wiring (unchanged from rev 1): `internal/shell.newPersistentShell` receives the Provider and
calls `Wrap` before `cmd.Start()` — bootstrap → app context → shell factory, one chain. **This
revision additionally requires the same `Wrap` + env floor on `executeLocal`**
(`core/tools/terminal.go:827/836`), which rev 1 left as a risk.

**macOS mechanism (D4):** Seatbelt enforcement is kernel-enforced regardless of entry point.
Rev 1 proposed the deprecated `sandbox-exec -p` CLI. This revision: generate the SBPL
profile as before but apply it via the **Sandbox framework API** (`sandbox_init`) from a tiny
helper that wraps the real argv (the CLI's only job is profile-apply-then-exec; a 50-line
helper does the same without the deprecated binary). Implementation note for Phase 2: verify
the API's deprecation annotation against the then-current SDK (the `sandbox-exec` man page
deprecates the *tool*; the framework is the supported mechanism for App Sandbox and remains
the route Chromium-class sandboxes use). The Provider interface means if Apple ever removes
seatbelt for non-App-Sandbox processes, `Effective` degrades visibly (rev 1 risk R1 stands).

### 4.4 Go-layer network choke point + per-run grants (D1)

- **Default posture:** `sandboxing.network: false` means agent-initiated network is denied.
  Enforcement, in order:
  1. **Schema hiding (primary):** feed `network:false` into `DisabledToolNames`
     (`guardrails/guardrails.go:345` → the SPEC-006 §6 narrow waist) so fetch/scan/search/
     connector-send tools vanish from the schema — in **both** native `tools[]` and XML-text
     tool-manual modes (a model without native tool-call support simply never sees the tools).
  2. **Hard synchronous denial (defense-in-depth):** any residual call path returns a
     non-approvable security-boundary rejection (SPEC-006 II.3) *before* dialing — never the
     approval flow (approvable network denial = click-allow bypass; unattended runs would
     stall the full `GuardrailApprovalTimeout`, cf. the 2026-08-31 `wc`-block incident,
     `agent_test.go:3907`).
- **Network grant model — three levels, most-specific wins within the host ceiling
  (resolved; answers "how do I enable network for a workspace?"):**
  - **L0 host master** `sandboxing.network` (default **off**): the hard ceiling. OFF =
    effective network is `none` for EVERY workspace/automation/assistant, regardless of
    any guardrail config or persisted approval (R4 — enforced as an intersection OUTSIDE
    `MergeWith`, so the OR-merge can never re-enable it). ON = network is permitted on this
    host; actual scope still needs L1.
  - **L1 workspace scope** = the EXISTING `guardrails.network` block (`Enabled` +
    `AllowLanAccess`/`AllowInternetAccess`, settings.yml default + per-workspace
    `config.yaml` overrides + persisted approvals) — no parallel new knob. LAN and Internet
    are **independent** (Internet does not imply LAN), so L1 resolves to `lan` (LAN only),
    `internet_only` (internet, LAN blocked), or `internet` (both). When L0 is ON,
    L1 is what grants network to **assistant chats AND to every automation that does not
    set its own grant**. When L0 is OFF, L1 is inert (never effective).
  - **L2 per-automation grant** `Automation.NetworkGrant ∈ {inherit(default), none, lan,
    internet_only, internet}` (models/workspace.go, next to `AllowedTools`): an optional override for
    that automation's runs only. Default `inherit` = the workspace's L1 scope governs.
    Explicit grants may tighten (`none` in an internet workspace) or loosen (`lan` in a
    none workspace) — a visible, auditable operator decision. L0 remains the only ceiling.
  - **Effective per run (S1 resolver):** `effectiveScope = L0 ? (L2 ?? L1) : none`;
    `networkOn(OS) = effectiveScope != none` (D8). Concretely:
    - *"One workspace does dev work with the internet; the rest are offline"* → L0 ON +
      that workspace's L1 `AllowInternetAccess: true`.
    - *"A LAN-scan automation in an otherwise offline workspace"* → L0 ON + that
      automation's L2 `network: lan` (workspace stays `none`; the grant is visible on the
      automation).
    - *"Kill everything during an incident"* → L0 OFF (workspaces keep their config; the
      ceiling does the work).
  - **Answering the review question directly:** once L1 grants a workspace network, ANY
  assistant conversation in that workspace and ANY automation WITHOUT its own L2 override
  have access at the allowed scope. An automation WITH an L2 grant uses that grant instead.
  A workspace grant alone never beats L0-off.
- **Scope boundary (do not gate infrastructure):** the gate lives at agent-tool entry points
  only. Provider calls ride the pooled `proxy.SharedTransport` (`core/proxy/client.go:133`,
  Constitution I.2 carve-out) — never gated, or the app stops serving model requests.
  Connector inbound webhook *receipt* is server-side and unaffected; connector *sends*
  (auto-reply, triggered messages) are agent egress and ARE gated — state that in the UI and
  reconcile with the connector-auto-reply plan.
- **Telegram answer (review Q):** under `network:false` the agent's send TOOL
  (`notify_user`, incl. auto-reply/triggered sends issued through the tool loop) is hidden and
  blocked; inbound receipt keeps working. Replies the webhook handler delivers to the chat that
  initiated the message (auto-reply to the inbound message, `/run` result replies) ride the
  guarded connector client as part of the inbound conversation flow — the destination is that
  same initiator, so they are exempt like receipt (Constitution I.4); SPEC-009 v1.1 documents
  this boundary. Under a per-run/workspace grant, tool sends work as today through the guarded
  dial path. `Effective` + UI make the current state visible.

### 4.5 Egress proxy (core, D2)

`sandboxing.egress_proxy: 0|port`. When on:

- **Shell children** get `HTTP_PROXY`/`HTTPS_PROXY`/`NO_PROXY` env pointing at the local
  listener (Seatbelt/Landlock deny direct connect where they can — the proxy is the uniform
  answer for hosts where they can't).
- **In-process tools** (`NetworkTools`, search) are pointed at the proxy transport — otherwise
  the shell is proxied while `fetch_url` still dials direct, defeating the mode.
- **The proxy applies domain/CIDR policy** (`allow: telegram.org; deny: everything else`),
  enforced by the *proxy process*, which is the one local component the agent does not
  control. This is the only local way to get domain-level egress (§2 out-of-scope #4, now
  in scope).
- **DNS caveat (Phase 2 detail):** to prevent UDP/53 exfil around an HTTP proxy, resolve
  inside the proxy or set `NO_PROXY` to empty and deny raw DNS from jailed children where the
  OS layer can (seatbelt full; Landlock can't — Linux ≤6.7/bwrap gap reported honestly in
  `Effective`).
- Speed: a localhost hop is negligible vs. the workloads involved; it rides the existing
  guarded HTTP stack.

### 4.6 Deployment containment (D3/D4)

- **Linux (exists — do not regress):** `setup.sh` → `User=llm-proxy`, nologin, single root,
  read-only system/home, ACL grants for model paths via `./setup.sh access`. Phase 4 extends
  the unit with cgroup resource limits (`MemoryMax`, `TasksMax`) — real enforcement, better
  than any rlimit (R7).
- **macOS (new):** a launchd daemon artifact (plist template + setup step) running the backend
  as a dedicated unprivileged user, mirroring the Linux unit's shape (single writable root via
  `LLM_PROXY_HOME`, keychain note: cloud-provider secrets are encrypted in `secrets.json`
  under that root — the service user holds the master key, which is the correct model).
  Dev/desktop runs keep running as the operator and are covered by the OS FS jail instead.
- What deployment containment does **not** give: workspace↔workspace isolation under a shared
  service user (→ OS jail) and network policy (→ proxy). The three layers compose; each is
  justified by exactly what the others cannot do.

### 4.7 Grant/deny spelling (rev-1 H3 kept + Landlock semantics)

Rev 1 required grant/deny sets to be spelled out, not "allow rw / ro / deny else" — kept.
**Review finding added (R5):** the vocabulary differs per mechanism and the plan must not
pretend otherwise:

- **Seatbelt (macOS):** deny-over-allow exists → express "ro `$HOME` except `~/.ssh`,
  `~/.aws`, `~/.config/gcloud`, `$SSH_AUTH_SOCK`, docker/podman/containerd sockets" as allow-ro
  + explicit denies. Socket-hatch deny list must include `/var/run/docker.sock` AND
  `/run/containerd/containerd.sock`, podman sockets (`/run/podman/*`, `/run/user/*/podman/*`).
  Named invariant (unchanged): **docker.sock reachable + network = trivial escape to host root
  via the Docker API.**
- **Landlock (Linux):** whitelist-only, deny-by-default. `~/.ssh` needs no rule (it is denied
  by default) — but every positive grant needs **traverse (execute) rules on path ancestors**,
  and the toolchain must be enumerated as ro-exec grants: PATH dirs resolved at shell creation
  (`prepareShellEnv` already curates exact dirs), their `realpath`, ld.so + lib dirs, CA
  bundles, `/dev/{null,zero,urandom,random}`, ro `/proc`. Toolchains living under `$HOME`
  (nvm/asdf/Homebrew-on-Linux) must be enumerated per resolved PATH entry — grant rules derive
  from the same list `prepareShellEnv` uses, so versions changing path still works (rules are
  per-inode at spawn time). Landlock cannot express "ro home except…" — say so in the docs and
  in `Effective` messaging rather than implying parity.

### 4.8 Engineering patterns (chosen + why, with repo precedent)

The pipeline (D7) is kept **extensible without a framework**. Every pattern below already
has a precedent in this codebase; the plan extends those, it does not invent new
infrastructure:

1. **Pipeline = documented ordering of plain sequential calls, not middleware.
   S0–S5 live in the existing dispatch control flow** (`tool_exec.go`), read top to bottom.
   Extensibility comes from where each kind of change plugs in — never from reordering
   magic: a new gate adds an S0 input (one entry in `DisabledToolNames`); a new validation
   appends to S2; a new mechanism adds an S4 backend behind the same interfaces. No
   pluggable-stages framework: the repo's complexity gate (≤12) and no-dead-code rules
   would fight generic indirection.
2. **Policy snapshot = immutable value object built by one resolver (builder).**
   `ResolvePolicy(cfg, capabilities, wsOverride, runGrant) EffectivePolicy` runs once per
   run/turn; every stage takes the snapshot as a parameter. Nothing re-derives policy from
   hot config. Precedent: `network.DialContext()` is already injected once and reused;
   `AppContext.HostSettings()` already projects runtime state (`app_context_system.go:47`).
3. **Capability detection = small `detect*` probes + a registry (strategy).** One function
   per mechanism (`detectLandlockABI`, `detectSeatbelt`, `detectBwrap`); `sandbox.New`
   runs them once and selects adapters. A new mechanism = new adapter + probe + `Effective`
   label + UI badge; nothing else changes. Precedent: local provider tool-call probing
   (Constitution II.5), provider-manifest defaults.
4. **Backends = ports behind interfaces (provider/adapter).** `Provider.Wrap` (spawn
   enforcer) and the egress transport/policy are interfaces with adapters incl. an
   identity/no-op adapter, so `filesystem:false` / `network:on` keep ONE code path.
   Precedent: `RegisterConnectorFactory` + injected `NetworkTools` (`telegram.go:193`);
   `mcp.Client.DialContext` is pluggable (`mcp/client.go:180`).
5. **Wiring = one composition root (bootstrap).** Bootstrap constructs the resolver + the
   two backends and injects them down (shell factory, tool registry, connector factories).
   No globals, no service locator. Precedent: bootstrap already injects `DialContext`
   (`bootstrap.go:343`), `SetHTTPDoer` (`bootstrap.go:363`), `NetworkTools` into tools.
6. **Change propagation = epoch + observer.** Host-settings updates bump an epoch on the
   snapshot resolver; long-lived shells are recycled (existing kill-group/procwatch); the
   next run re-resolves with the new epoch. Precedent: `SecretsStore.OnChange` →
   `AppContext.Sync()` (Constitution III.9), fsnotify reload watchers.
7. **Audit trace = one correlation id through the stages.** S0–S5 stamp the run/turn id;
   the existing `guardrail_violation` lifecycle events and `GuardrailBlockedPayload`
   `decision_id` pattern extend to a single "policy decision" record, so why-blocked has
   one answer per id.
8. **Audit-invariant tests keep the pipeline honest.** A CI test greps the allowlist of
   exec/raw-client sites and fails on new out-of-band paths (Phase 1). Precedent: the
   `internal/testing/utils` `ExecCommand` seam used by tests.
9. **Test seams.** Reuse the existing overridable-`exec` seam for `Wrap` tests; OS probe
   tests are build-tagged and skip with an explicit `Effective` assertion (R5).

### 4.9 Implementation map: files touched, refactor, SSOT, packages

**Refactor-before-build (R1–R4).** The audit found the same logic already duplicated in
several places. Patching sandboxing on top of these would compound the mess — each is
collapsed to a single instance first, then the new machinery plugs into that instance:

| Refactor | Current duplication | Target | Removes |
|---|---|---|---|
| R1 **Spawn helper** | `shell/terminal.go:39` + `core/tools/terminal.go:827/838` build `exec.Cmd`/Setpgid/kill separately; executeLocal has no env floor | `platform/process.AgentCommand(ctx, wsPolicy, argv)` — env floor + Setpgid + `Provider.Wrap` + rlimits in ONE place (mirrors `platform/procwatch`) | dup spawn logic, dup kill-goroutine, measured env divergence |
| R2 **Validator service** | `ValidateToolCall→validateTerminal` (`guardrails.go:144`), `TerminalTools.Validate` (`terminal.go:87`, passes `nil` blocked list), `ValidateTerminalCommand` (`terminal.go:96`) | one `TerminalValidator` (rules + blocked-list) called by engine and tools | 3 divergent entry points |
| R3 **Guarded client factory** | `network.go:35-73`; `search.go:35` `http.DefaultClient` fallback; `slot_manager.go:62` `DefaultClient` (I.1 breach); `admin_handlers.go:507`; `mcp/client.go:174` | one `network.GuardedHTTPClient(policy)`; infra uses injected doers; `DefaultClient` fails CI | 4 client builders, 3 raw-client breaches |
| R4 **Policy resolver** | `HostSandboxingConfig` + `GetGuardrails()` + ws overrides via `MergeWith` (**ORs `Enabled`** — `models/config.go:186`) + per-call `getEffectiveConfig` (`registry.go:60-90`) | one `PolicyResolver` → per-run `EffectivePolicy` = f(host sandboxing, guardrails+ws, run grant) with **intersection** on security bits, host switch OUTSIDE the MergeWith chain (else a ws override/persisted approval re-enables what the host turned off) | scattered re-derivation; OR-re-enable hole; policy skew (R6) |

**New packages** (mirroring existing layout — `platform/*` for OS mechanisms like
`platform/procwatch`, `core/*` for app services like `core/proxy`):
`platform/process` (+rlimit), `platform/sandbox` (Provider/adapters/Effective/noop),
`core/egress` (proxy listener + domain/CIDR policy), `core/policy` (S1 resolver — or fold
into guardrails, decide in Phase 0).

**Files touched** (complete): models (`host_settings.go`, `config.go` backfill,
`workspace.go` +`Automation.NetworkGrant` next to existing `AllowedTools`); storage
(`manager.go` per-key backfill + tests); guardrails (`guardrails.go`, `tool_availability.go`);
shell+tools (`shell/terminal.go`, `core/tools/terminal.go`); network (`network.go`,
`search.go`, `notifiers/*`, `platform/network/*`, **`slot_manager.go`, `admin_handlers.go`
hygiene**); agent loop (`tool_exec.go` S1 stamp, audit id) + automation
(`dispatcher/executor` grant → run); app (`bootstrap.go` composition root,
`app_context_system.go` Effective projection); transport+UI (host-settings handler,
`SecuritySettings.vue`); SPEC/docs (`SPEC-006`, `architecture.md`, service unit, new macOS
launchd).

**Bottlenecks / perf** (design-around now): persistent-shell mutex already serializes
commands per workspace — never hold it during Wrap setup; `executeLocal` pays full spawn
cost per call (runner exec + env build — pool/reuse, acceptance-measure in Phase 2); egress
proxy reuses the pooled guarded transport and resolves DNS at the proxy, never routing
`SharedTransport` infra traffic through it; config reads ride the existing cached reader;
OS-sandbox cost is spawn-time only (persistent shell amortizes — the reason interposition
designs are rejected).

**Open design point (resolved in Phase 0 — D8):** per-run network grants vs. long-lived
pooled shells. **Decision:** pool key = `(workspaceID, networkOn)` — at most two shells per
workspace (network off / on). A run resolves `networkOn = grant != none && host switch on`
from its grant (stamped into `ctx` at loop start, like `models.GetWorkspaceID(ctx)`), picks
the matching shell, and keeps state within the run (grant is constant per run). Scope
changes between runs need NO recycle (different key; stale scope idle-reaped). `Recycle`
is epoch-only: filesystem toggle, capability change, hot settings update. Because
automations are single-active per workspace (`AgentState.IsRunning`), and chat-vs-automation
with different scopes simply use different keys, no thrash and no state loss. `Wrap` receives
`wsView{path, networkOn, epoch}` and applies OS network deny only when `networkOn=false`.
LAN-vs-internet stays an app-layer concept (in-process tools + egress policy); a shell under
`networkOn=true` reaches both — documented residual in `Effective` and the threat model.

### 4.10 UI design — network grant model & sandboxing surface

**Principle: one surface owns each level** (L0 host / L1 workspace / L2 automation), and
every surface shows an **effective-state readout** so the operator never has to infer what is
actually enforced. Levels are never duplicated across screens — the hierarchy is expressed by
where each control sits, and by the readout, not by copies of the same toggle.

**L0 — Host (Settings → rename "Local Host Terminal" → "Security & Sandboxing").**
Restructure `SecuritySettings.vue` (which today is only the persistent-terminal toggle +
reset controls + monitor) top-to-bottom as cards, reusing existing `.advanced-card` /
`.toggle-header` / `.setting-label` / alert styles:

```
┌ Security & Sandboxing ────────────────────────────────[ Effective: seatbelt · net off ]─┐
│ header: subtitle = sandboxing for everything the agent runs (host-wide)                 │
│                                                                                          │
│ CARD 1 · Sandboxing (3 master toggles, each row: toggle + label + description + inline  │
│         "Effective" pill)                                                               │
│   ▣ Sandboxing master (was "Enable Persistent Terminals")                                │
│   ▣ Confine agent files to workspaces        [filesystem]  Effective: landlock           │
│   ▣ Allow agent network  (HOST MASTER)       [network]     ▸ OFF — hard ceiling; every   │
│         sub-line when off: no workspace/automation/assistant reaches the network;        │
│         connector sends blocked (inbound receipt unaffected). [amber styling]            │
│   ▣ Egress proxy :port  (appears when network on) — domain-level filtering for all agent │
│         egress; only knob that can say "telegram.org yes, rest no"                        │
│ [⚠ migration banner when key absent: network currently ALLOWED (legacy); Restrict?       │
│   [Restrict (recommended)] [Keep allowed]]                                               │
│                                                                                          │
│ CARD 2 · Resources          memory (MB) / storage (GB) + per-OS enforcement note         │
│ CARD 3 · Effective enforcement on THIS host (read-only table, requested vs actual):      │
│   Filesystem: seatbelt/landlock/off · Network: off|scope · Memory: cgroup|rlimit|        │
│   best-effort (Darwin) · Sessions: n.  Amber/red row when requested ≠ enforced           │
│   (e.g. "tcp-only — UDP open; enable egress proxy or bwrap") — downgrade is NEVER silent │
│ CARD 4 · Reset controls (existing) ·  CARD 5 · Session monitor (TerminalMonitor, below)   │
│ action bar: Unsaved-changes state · Restart runtime · Apply & Save                        │
└──────────────────────────────────────────────────────────────────────────────────────────┘
```

**L1 — Workspace (existing per-workspace guardrails; NO new knob).** The network card the
workspace editor already shows (`GuardrailForm` in `WorkspaceSettings`: enabled +
`allow_lan_access`/`allow_internet_access` + blocked domains/IPs) stays the workspace scope
control. Additions only:
- **Effective-scope readout** at the top of that card: pill "Effective: Internet (host on)" /
  "None — host master is OFF" (amber).
- When L0 is off, the card body is dimmed with a single line: *"Host master is OFF — this
  workspace policy is inert until Settings → Security & Sandboxing enables agent network."*
  plus a shortcut link to the Security tab (no duplicated toggle).
- Host-level guardrails page (`GuardrailSettings`, system-wide) gets the same treatment.

**L2 — Automation (`AutomationForm.vue`).** New "Network access" field in the execution
group (beside Model / Loop Strategy), rendered as a segmented control:

```
┌ Execution ────────────────────────────────────────────────┐
│ Connection source · Specific model · Loop strategy        │
│ Network access:   (inherit)  (none)  (LAN only) (Internet) │
│                   ▾ default = inherit workspace scope     │
│   helper: will inherit the workspace scope (Internet).    │
│   Choose a scope only if this unattended run must differ. │
│   [disabled + note when L0 off: host network is off]      │
└───────────────────────────────────────────────────────────┘
```

**Cross-surface consistency:**
- Shared read-only component `EffectiveNetworkBadge` (scope + host-off) reused on the host
  page, workspace card, automation form, and — for audit — each historical run row shows the
  **resolved scope that run actually used** (feeds the S1 audit trace; an unattended run that
  lost network is explainable at a glance).
- `TerminalMonitor`: after D8 a workspace can have two shells (net on/off) — list rows gain a
  small scope label when both exist ("net off"/"net on"), else unchanged.
- Frontend contract note: `SecuritySettings.vue` projects `sandboxing.*` — the JSON shape
grows (`filesystem`, `network`, `egress_proxy`, `effective`) but **runtime `Effective` stays a
read-time projection from the host-settings GET**, never persisted into `settings.yml`
(reuses the `Functional` pattern at `app_context_system.go:47`).
- Colors: green = enforced as requested; amber = partial/downgrade (`tcp-only`, Darwin memory
  best-effort); red = requested but off/unavailable. Same palette as existing alert boxes.

## 5. Phases (re-ranked: pure-Go security first, OS layers second)

TDD throughout (red→green→refactor; `docs/skills/tdd-guide.md`). Backend phases require
`go build ./... && go test ./...` + `go run ./tools/check-complexity/` (complexity ≤12 —
profile generators, Wrap, and the proxy policy engine are the watch-list). Capability
detection decomposed into one small `detect*` per mechanism; `profilegen.go` stays pure;
`Wrap` is a switch over the resolved policy. **Every phase ships its acceptance test names**
(per the sibling-plan convention), and the CI note must state which kernel/ABI the runner
actually exercised (rev-1 risk R5, kept).

### Phase 0 — network default-off + per-run grants (pure Go, no OS code)
- `HostSandboxingConfig`: add `Network *bool` (or per-key backfill) + `yaml` tags; replace
  whole-section merge with per-key backfill (`manager.go:300-321` style); regression test
  (D6).
- `network:false` → `DisabledToolNames` schema-hiding + hard synchronous denial (D1 §4.4).
- Per-run `network` grant in automation templates; override stack may only tighten.
- Connector-send gating + inbound-webhook exemption + UI copy (rev-1 Phase 3 frontend work,
  promoted: effective-state badges start here with `network: off|granted`).
- **Shell pooling re-keyed (D8):** `ShellProvider.GetOrCreate` takes a `WorkspacePolicy
  {EnvAllow, PathExtensions, NetworkOn, Epoch}`; session map key → `(wsID, networkOn)`;
  reaper over composite keys; `Recycle(wsID)` epoch-only; `ListSessions` groups by
  workspace with scope labels. Grants stamped into `ctx` at loop start; `TerminalTools`
  resolves `networkOn` per run.
- **Acceptance (additions):** `TestPoolKeyedByWorkspaceNetworkState`,
  `TestScopeTransitionUsesDifferentShellNoRecycle`, `TestEpochBumpRecyclesWorkspaceShells`,
  `TestChatAndAutomationDifferentScopesIsolated`, `TestShellStatePersistsWithinRun`,
  `TestReaperCollectsStaleScopeShell`, `TestSessionListGroupsByWorkspace`.
- **Acceptance:** `TestNetworkDefaultOff`, `TestDisabledToolsAbsentFromSchema`
  (native + XML mode), `TestResidualCallSynchronousNonApprovable`,
  `TestAutomationNetworkGrantOnlyPath`, `TestConfigAbsentKeyBackfill` —
  plus the rev-1 regression tests for yaml backfill. With `network:false`, LAN-scan/fetch/
  send tools never appear in any schema and any residual call fails fast, never entering the
  approval flow (the unattended-stall regression, `agent_test.go:3907` style).

### Phase 1 — egress proxy (pure Go, D2)
- `egress/` package: local listener + domain/CIDR policy engine; point `NetworkTools`/search
  transport at it; shell env `HTTP_PROXY/HTTPS_PROXY`.
- **Raw-client hygiene (from the audit, §3):** replace `slot_manager.go:62`
  `http.DefaultClient` with the injected provider doer (Constitution I.2); remove the
  `search.go:35` `DefaultClient` fallback (client becomes required); route the
  `admin_handlers.go:507` Telegram webhook-registration client through an injected
  transport. Add the **audit-invariant test** (any new `http.DefaultClient` / bare `exec`
  outside the allowlist fails CI).
- Config field + UI; disabled by default.
- **Acceptance:** `TestDirectDialDenied`, `TestProxiedEgressAppliesDomainPolicy`,
  `TestShellEnvProxyInjection`, `TestInProcessToolsShareProxyPath`,
  `TestProxyNeverGatesProviderCarveOut` (Constitution I.2 must keep working),
  `TestAuditInvariantNoOutOfBandEgress` (the grep test above), `TestSearchClientRequired`,
  `TestSlotManagerUsesInjectedDoer`.

### Phase 2 — OS filesystem jail (per-workspace isolation + dev-mode containment)
- macOS: SBPL profile generator (`profilegen.go` + seatbelt helper via the Sandbox framework
  API — D4); Linux: GoLandlock FS rules via the sandbox-runner child (mechanism note rev 1
  unchanged). Grant/deny spelled per mechanism (§4.7, R5).
- `sandbox` package skeleton + `Provider` + `Effective` + capability detection at `New()`.
- Apply `Wrap` + env floor to **both** the pooled shell and `executeLocal` (refactor R1;
  Risk 4).
- Preserve `.sandbox` invisibility invariants; never re-expose `.sandbox` under a jailed child.
- **Acceptance:** probe tests per OS — read outside workspace fails (EPERM-equivalent), write
  outside fails, workspace + `.sandbox` writes succeed, socket hatches (docker/containerd/
  podman/`$SSH_AUTH_SOCK`) read-fail, **workspace B unreadable from workspace A's shell**.
  Build-tagged per OS; skip only with `Effective` reflecting the skip (R5).

### Phase 3 — resources + storage accounting (measured reality, D5)
- Linux service: systemd `MemoryMax`/`TasksMax` (cgroup v2 — real). Non-service Linux:
  `RLIMIT_DATA`/`AS` recalibrated (default 2048 MB; acceptance: `node -e`, `go build`,
  `npm install` succeed under it — 256 MB is known-broken, §3). macOS: apply `RLIMIT_FSIZE`
  + `RLIMIT_NPROC` (measured enforced); **skip `RLIMIT_AS`/`RLIMIT_DATA` (measured
  unsettable)** and report `Effective: memory not kernel-enforced (Darwin)`. `RLIMIT_NPROC`
  is per-UID — under a shared uid it self-DoSes; document dedicated-user requirement (R7).
- `max_storage_gb`: accounting loop + `RLIMIT_FSIZE` per-file backstop; honest labeling, no
  "quota" claim.
- **Acceptance:** `TestDarwinRlimitsSettable` (guards the measured matrix),
  `TestNodeGoNpmUnderDefaultLimit`, `TestForkBombCapped`, `TestStorageBoundaryAccounting`,
  `TestEffectiveReportsDarwinMemoryUnenforced`.

### Phase 4 — deployment (macOS launchd + Linux cgroups + docs)
- macOS launchd plist template + setup step (dedicated unprivileged user; D4/D3).
- Linux unit: add `MemoryMax`/`TasksMax`; re-verify `systemd-analyze security` (the unit
  already scores via `NoNewPrivileges`/`ProtectSystem` — keep it that way).
- Env-secret audit of `prepareShellEnv` allowlist (connector API keys must not leak into shell
  env).
- **Acceptance:** service runs as dedicated user on both OSes; secret paths unreadable by the
  service user by construction; dev-mode runs covered by Phase-2 jail.

## 6. Risks / known limitations

1. **Seatbelt deprecation (macOS).** Re-framed by D4: the deprecated `sandbox-exec` CLI is
   not used; the Sandbox framework mechanism remains. Primary macOS containment becomes the
   dedicated-user deployment, so seatbelt is only load-bearing for dev-mode + per-workspace
   isolation. If Apple removes the mechanism for non-App-Sandbox processes, `Effective`
   degrades visibly and Linux-style alternatives are re-evaluated then.
2. **Landlock network is TCP-only (UDP escapes) and needs kernel ≥6.7; 5.13–6.6 has no
   kernel network at all.** With the egress proxy as the uniform network control (D2) this
   becomes an `Effective`-reported nuance rather than a security hole: even where the kernel
   cannot deny a shell socket, the proxy is the only egress path that carries agent traffic
   policy. Old kernels without proxy → honest `go-only`.
3. **CLI breakage under the FS jail.** Some tools probe odd paths. Mitigation (rev 1, kept):
   conservative allowlist, failures surface as tool errors (record-and-continue), small
   extension-point list in the profile generator.
4. **`executeLocal` parity.** Both child paths must receive the same `Wrap` **and** the same
   env floor (measured gap, §3). Included in Phase 2 scope, not left as a risk.
5. **Landlock whitelist-only semantics.** Enumerating toolchain grants incl. toolchains under
   `$HOME`, plus ancestor-traverse rules (§4.7), is the top Phase-2 implementation risk; the
   derivation-from-`prepareShellEnv`-PATH approach bounds it.
6. **Policy skew / no enforcement lifecycle.** Resolved by the S1 snapshot: policy is
   resolved once per run/turn and stamped with the epoch it reflects; hot settings updates
   bump the epoch, recycle affected workspace shells (kill-group/procwatch exist), and the
   next run's S1 re-resolves. No stage ever re-derives policy from hot config mid-run, so
   there is no window where the UI claims one policy and a long-lived shell enforces another.
7. **`RLIMIT_NPROC` per-UID blast radius** (measured). Only safe at scale under the dedicated
   user; under a shared uid a fork bomb can starve the operator. Documented; not "solved" by
   the rlimit itself.
8. **Wrapper identity.** Spawning via seatbelt-helper/runner makes `cmd.Process.Pid`/PGID/
   procwatch observe the wrapper. Both mechanisms `exec` in-process (PID preserved) — re-verify
   kill-group/`StopAutomation` under the wrapper (rev 1, kept).
9. **Egress-proxy DNS/UDP.** An HTTP proxy does not cover raw UDP (DNS, QUIC). Phase 1/2 must
   handle DNS resolution inside the proxy and report the residual honestly (see §4.5).

## 7. Rollback

- `sandboxing.filesystem: false` → restores rev-1-era behavior (env jail + text guardrails).
- `sandboxing.network: false→true` or a per-run grant → restores network for that scope.
  Because the default flips to `false` (D1), the upgrade path must be explicit (§4.1 backfill
  rule) — an unattended automation silently losing network is a feature (fail-closed), but it
  must be explainable: schema-hiding + `Effective` show *why* a tool vanished.
- `sandboxing.enabled: false` → today's behavior minus the bootstrap fatal, with the loud
  opt-in/effective-state warning (Phase 0 acceptance).
- No data migration beyond the config backfill; egress proxy and OS layers are pure additions.

## 8. SPEC / docs impact

- **SPEC-006 (stable):** version bump + change record adding an "OS Enforcement Layer +
  network default-off" subsection: the sandbox sits *under* guardrails; effective-state
  reporting contract; downgrade-never-bypass; the hard-denial/approval-flow boundary; and
  the per-run network grant semantics. **Fix the frontmatter `constitution_references: [I.5]`
  → `[II.3]`** (no `I.5` exists) while touching it.
- **`docs/architecture.md`:** sandbox + egress packages in the directory map; "adding a
  sandbox enforcement" checklist pointer.
- **`docs/service_setup.md` + `docs/services/`:** Linux cgroup additions; **new macOS launchd
  artifact + doc** (dedicated-user deployment, keychain note).
- **`docs/PLANS/ARCHIVE/cross-cutting/system-blueprint.md:54`:** mark the stale Wazero
  future-work line superseded.
- **`setup.sh`:** unchanged flow stays authoritative for Linux; macOS install story references
  the new launchd artifact (no new systemd code paths).

## 9. Pre-implementation gate — remaining tasks and decisions (final checklist)

Everything below must be resolved/queued before Phase-0 code starts. Items are labeled
[decision], [code], [ops/docs], or [process]. R-phase mapping for the refactors: **R4
(resolver) → Phase 0 · R1 (spawn helper) → Phase 0 shape / Phase 2 Wrap · R2 (validator
service) → Phase 0 · R3 (guarded client) → Phase 1.**

1. **[decision] Dependency approval (AGENTS/CONSTITUTION heavy-deps).** Landlock/rlimit code
   needs `golang.org/x/sys` promoted from `// indirect` (`backend/go.mod:39`) to direct, or the
   go-landlock module. Approve explicitly BEFORE Phase 2 (hand-rolling via `syscall.Syscall`
   avoids it). Carry-over from rev 1 that this revision dropped.
2. **[ops] CI matrix + Effective-assert harness.** Jobs: ubuntu-24.04 (kernel 6.8 → Landlock
   ABI v4 FS+TCP), ubuntu-22.04 (image now ships a 6.8 HWE kernel → also ABI v4 FS+TCP; the
   ABI < v4 FS-only network downgrade is unit-covered, not runnable on hosted runners),
   macos-latest (seatbelt). Each job asserts its `Effective` state in the probe tests and
   FAILS if capability detection silently degrades (prevents the whole matrix skipping). Note:
   if the CI runner is itself a container without Landlock, detect that case and run the
   FS-matrix job on a VM runner.
3. **[decision/code] Default env allowlist (security fix, pull early — today an empty
   `AllowedEnvVars` passes ALL host env into the shell).** Verified: `prepareShellEnv`
   (`shell/terminal.go`) copies `os.Environ()` wholesale when the allowlist is empty. Decide the
   default allowlist set (PATH, HOME/TMPDIR floor, LANG/LC, locale, proxy vars) and ship it in
   Phase 0 — the Phase-4 env-secret audit becomes a review, not the fix.
4. **[design note] TasksMax vs RLIMIT_NPROC.** Under systemd the unit's cgroup `TasksMax` is a
   per-SERVICE process cap with NO per-uid blast radius — do NOT also set `RLIMIT_NPROC`
   in-process for the service path (redundant + harmful). In-process NPROC rlimit applies only
   to non-systemd runs (dev, macOS).
5. **[spec detail] Egress proxy hardening (expand §4.5 before Phase 1):** bind `127.0.0.1`
   only; policy enforced on the CONNECT/absolute-URI authority (filter by hostname pre-TLS —
   no MITM); DNS resolved at the proxy; `NO_PROXY` limited to loopback (dev servers); proxy
   listener itself excluded from the agent jail and refuses callers without a per-run token
   (loopback + token prevents other local processes abusing it).
6. **[design note] Observability/audit contract.** Startup log line with `Effective`;
   lifecycle event on downgrade (requested ≠ enforced); per-run resolved network scope stamped
   into run audit rows (feeds the UI readout in §4.10); reuse `guardrail_violation` events for
   hard denials — no new event vocabulary without SPEC-006 change record.
7. **[code] Performance acceptance (benchmarks).** Capture baselines BEFORE any change (record
   `go build`, `npm install`, and `executeLocal` spawn latency in a fixture repo). Budgets to
   assert in CI: persistent-shell commands ≤2% overhead vs unsandboxed; `executeLocal` spawn
   +runner ≤ small fixed ms; egress-proxy path ≤5% on fetch; `node -e`/`go build`/`npm install`
   succeed under the default memory limit. Numbers recorded, not asserted blind.
8. **[decision] Seatbelt helper form (Phase 2).** Prefer a tiny helper binary (mirrors
   `runner_linux.go`, no cgo across the toolchain) over in-process cgo `sandbox_init`;
   re-verify the Sandbox API's deprecation annotation against the SDK in force at
   implementation time (§4.3 note).
9. **[process] SPEC change records BEFORE code (V.2 spec-first).** Queue SPEC-006 version bump
   + "OS Enforcement Layer & network default-off" subsection (+ frontmatter `I.5`→`II.3` fix),
   and a SPEC-009 note for connector-send gating + its interaction with the connector-auto-
   reply plan. Do these as the first commit of Phase 0.
10. **[ops] systemd unit calibration.** `MemoryMax`/`TasksMax` values sized so parallel
    toolchain builds (go build -p max, npm install) inside one workspace still succeed;
    validate with `systemd-analyze security` + a stress run; keep `NoNewPrivileges`/
    `ProtectSystem` intact.
11. **[ops] macOS deployment detail (Phase 4).** Dedicated user + launchd plist; single root
    ownership; note that cloud credentials are already at rest as encrypted `secrets.json`
    under that root (master-key co-location — Constitution III.6) so no Keychain dependency;
    document dev (operator-user) vs daemon (dedicated-user) modes.
12. **[process] Rollout sequencing.** Phases ship independently behind their switches
    (revertible one at a time); order P0→P4; the network default-off migration banner (§4.1)
    is the only cross-version UX change and needs its own release note. Regression guard: the
    connector INBOUND webhook path must keep working with `network:false` (exemption test).
13. **[process] DoD for the plan itself.** All T-items resolved; SPEC-006/009 bumped;
    architecture map + service_setup + launchd + system-blueprint stale line updated;
    performance baselines captured; then Phase-0 implementation begins.
14. **[decision] Phase-0 open calls to confirm first:** (a) `core/policy` package vs folding
    the resolver into guardrails; (b) the exact default env allowlist set (T3); (c) Linux
    non-service memory lever `RLIMIT_DATA` vs `RLIMIT_AS`; (d) validate 2048 MB default under
    a real `go build`; (e) egress proxy default port + UI copy tone.

## 10. Implementation progress & resume point (work log)

**Status: implemented for the shipped scope (Phases 0–4); see the post-review hardening pass
at the tail of this section (2026-09-06/07).** All slices below shipped on 2026-09-06;
backend gates were green at each checkpoint (macOS `go build ./...` + `go test ./...`) and
the tree is NOT committed (repo git rules). Pre-implementation-gate items T1–T14: T1
resolved (x/sys promoted to direct), T3 (default env allowlist) shipped, T4/T10 deployment
choices are in the service files, T9 (SPEC-006/009) done, T11 (launchd) shipped, T14
decisions adopted (T14a resolver folded into `models`). T2 (Linux CI matrix asserting
`Effective`), T5 (per-run proxy token), T6 (full observability), and T7 (measured perf
baselines) remain open — recorded below.

### Shipped (2026-09-06)

| Slice | What landed | Key files |
|---|---|---|
| **T9 / SPEC** | SPEC-006 → v1.1 (OS Enforcement Layer §II.7, network default-off, changelog, frontmatter `[I.5]→[II.3]`); INDEX.md rows refreshed | `docs/SPECS/guardrails.md`, `docs/INDEX.md` |
| **0.1 Config plumbing (D6)** | `HostSandboxingConfig` + `Filesystem *bool`(nil=ON, no explicit default) / `Network *bool`(nil=legacy-allowed; fresh default OFF) / `EgressProxy int`; yaml tags (legacy keys byte-identical) + `omitempty`; read-time accessors (`FilesystemEnabled`/`NetworkAllowed`/`NetworkDecided`); `MaxMemoryMB` default 256→2048; regression tests (legacy file boots jail-on + network undecided, never rewritten with a key; fresh persists `network: false`; explicit never clobbered) | `backend/models/host_settings.go`, `backend/models/host_settings_test.go`, `backend/internal/platform/storage/app_config_store_test.go` |
| **0.2 Schema hard gate (D1/R4)** | `GuardrailEngine.SetHostNetworkAllowed` (outside MergeWith + override cache); `ErrNetworkDisabled` sentinel; `DisabledToolNames` hides 5 egress tools when host OFF; `ValidateToolCall` denies before overrides; agent loop classifies via `errors.Is` → non-approvable; wired from `AppContext.HostSettings` in `InitializeAgentStack` | `internal/core/assistant/guardrails/guardrails.go`, `tool_exec.go`, `registry.go` + tests (merged into existing test files) |
| **0.3 Scope model (L2)** | `models.NetworkScope` typed enum (""=inherit·none·lan·internet, fail-safe `NetworkOn`); `Automation.NetworkGrant`; `NetworkGuardrailsConfig.EffectiveScope(host, grant)` resolver; ctx helpers `WithRunNetworkScope`/`RunNetworkScopeFrom` | `backend/models/workspace.go`, `config.go`, `host_settings_test.go` |
| **0.4 Run-aware grants + shell re-key (D8)** | End-to-end `L0?(L2??L1):none`: executor resolves `ResolveRunScope(ws,grant)` → `AgentOptions.RunNetworkScope` → scope-aware schema (`resolveToolProviderForScope` + `DisabledToolNamesForScope`) + `Execute` ctx stamp → engine scope validation (grant none hard-deny; lan≠internet virtual cfg) + network-tool runtime scope + shell pool key; **shell pool keyed (workspace, NetworkOn)** via `shell.WorkspacePolicy`, epoch recycle, `Recycle(ws)` drops all scopes, `TerminalSessionView.NetworkOn` | `internal/core/automation/{executor,dispatcher,registry}.go`, `internal/core/assistant/{agent,agent_builder,tool_availability}.go`, `guardrails/guardrails.go`, `internal/shell/{shell,terminal}.go`, `internal/core/tools/terminal.go`, `internal/app/app_context_test.go` |
| **Cleanup** | Merged `loop_resolver.go`→`loop_strategy.go`, `filtered_provider.go`→`tool_availability.go`; consolidated standalone guardrail tests into existing files; generalized `.agents/rules/go-staff-engineer.md` Go-idioms (toolchain-aware, `new(T)` for zero-value pointers) | above + `.agents/rules/go-staff-engineer.md` |
| **0.5a Frontend — L0 Security page** | Typed contract: `SandboxingConfig`/`HostSettings` in `src/types/admin.ts`, typed `fetchHostSettings`/`updateHostSettings` in `adminService.ts`. `SecuritySettings.vue` restructured to "Security & Sandboxing" per §4.10: master/filesystem/network toggles with inline Effective state, amber migration banner keyed on undecided `network` (Restrict/Keep actions), informational Resources card (Phase-3 honesty), Effective-enforcement readout card, kept Reset Controls + Restart + TerminalMonitor. Undecided keys stay undefined on PUT so the backend reads them unchanged. Gates: `npm run build` (eslint+vue-tsc) ✅, `npm test` 72 ✅, `npm run lint` ✅ | `frontend/src/types/admin.ts`, `services/admin/adminService.ts`, `components/settings/SecuritySettings.vue` (+ tracked `frontend_dist/index.html` artifact) |
| **0.5b Frontend/backend — L2 automation grant** | Backend fail-fast validation of `network_grant` in `validateAutomation` (empty=inherit; none/lan/internet; else 400) + `AutomationInfo` read model carries `network_grant` for edit round-trip. Frontend: `AutomationFormData.networkGrant`, `Automation.network_grant` (dispatcher read model), composable prefill/reset, and the "Network access" select (inherit/none/LAN/Internet) in the Execution group; payload sends `network_grant`. Gates: backend 38/38, frontend build+lint+tests 72 ✅ | `backend/internal/transport/http/handlers/dispatcher_handlers.go` (+test), `frontend/src/types/{automation,dispatcher}.ts`, `composables/automation/useAutomationForm.ts`, `components/AgentIde/automation/AutomationForm.vue` |
| **0.5c Effective/audit surfacing** | Backend: `RunMeta.NetworkScope` (run-meta.json audit trail, both success and error writers) + start-of-run `procLog` line with resolved scope + grant. Frontend: `TerminalMonitor` rows keyed (workspace, network_on) with net on/off scope tags (dual D8 shells). Gates: backend 38/38, frontend build ✅ | `backend/internal/core/automation/{rundir,executor}.go`, `frontend/src/components/settings/TerminalMonitor.vue` |

### Phase 1 — egress proxy (D2): slices 1a–1d all shipped below

**1a — raw-client hygiene + audit-invariant test (shipped):** replaced the three
Constitution I.1 breaches with injected transports — `slot_manager.go` rides
`network.SharedTransport` (I.2 infra carve-out), `search.go` dropped the
`http.DefaultClient` fallback (client is required; nil is a loud programming error), and
`admin_handlers.go` Telegram webhook-registration rides `network.SharedTransport`. Added
`internal/platform/network/egress_audit_test.go`: a scan test that fails on any
`http.DefaultClient`/`http.Get(` in non-test code and any `exec.*` outside the allowlisted
infra spawn sites (shell factory + executeLocal are the only agent-triggered spawns).
Gates: backend 38/38 ✅.

**1b — egress proxy core (`internal/core/egress`, shipped):** loopback-only HTTP forward
proxy with host policy (`Policy` interface; `HostListPolicy` exact/`.suffix` allow-deny,
deny-wins, optional default-deny). Supports absolute-form HTTP proxying and CONNECT tunnels
(TLS passthrough, no MITM); DNS resolves inside the proxy; the listener refuses any
non-loopback bind. Integration-tested against local `httptest`/TCP-echo upstreams (allowed
proxied GET, denied 403 never reaches upstream, CONNECT byte-echo tunnel, denied CONNECT 403,
non-loopback bind refusal, policy matrix). **Not yet wired** — config enablement
(`sandboxing.egress_proxy`), in-process tool transport swap, and shell `HTTP(S)_PROXY` env
land in 1c.

**1c — egress wiring (shipped):** composition root enables the proxy when
`sandboxing.egress_proxy` is a valid port (`startEgressProxy` in `BuildAppServices`; context
tethered, cancelled in `AppServices.Shutdown`); loopback URL + env vars threaded through
`InitializeAgentStack` → in-process `NetworkTools`/search/connector clients (`SetProxy` keeps
the guarded DialContext) and shells (`WorkspacePolicy.ProxyEnv` merged into the env floor for
network-on shells; upper+lower case `HTTP(S)_PROXY`/`NO_PROXY`). Security page gains the
egress port field (applies on restart). Tests: shell proxy-env reachable via `$HTTP_PROXY`,
`NetworkTools.SetProxy` wires `transport.Proxy` + keeps DialContext. **Notes:** policy is
allow-all until 1d (operator domain allow/deny lists); enablement is read at startup (hot
toggle = restart). Gates: backend 39/39, frontend build ✅.

**1d — operator domain policy (shipped):** `sandboxing.egress_allow_domains` /
`egress_deny_domains` (exact or `.suffix` host patterns) → `startEgressProxy` builds the
`HostListPolicy` (non-empty allow ⇒ default-deny; deny always wins; absent = allow-all, the
pre-policy behavior). `HostSandboxingConfig.EgressPolicy()` accessor; frontend type + PUT
round-trip preserve the lists (settings.yml is the editor; UI control deferred). The D6
whole-struct zero-check was made slice-safe with `reflect.DeepEqual` (structs grew slices and
became non-comparable). Test: policy + yaml round-trip in models. **Deferred (not started,
not claimed):** per-run scope enforcement AT the proxy (lan-vs-internet for shell HTTP egress)
and the per-run proxy token — tool-entry scope + schema already cover in-process tools; the
coarse shell-under-grant residual stays documented. Gates: backend 39/39 + complexity ✅,
frontend build ✅.

### Phase 2 — OS filesystem jail: seam + R1 spawn helper + Linux Landlock runner shipped below (macOS Seatbelt not built — Sandbox framework absent on the dev host)

Phase 1 (egress) is functionally complete for the shipped scope: proxy core + lifecycle +
tool/shell routing + operator domain policy. Remaining known follow-ups before/inside Phase 2:
per-run proxy scope + token (1e, optional); then Phase 2 (Landlock/Seatbelt FS jail,
`platform/sandbox` + `platform/process` rlimits, `Provider.Wrap`, epoch wiring).

**2a — `platform/sandbox` seam + downgrade contract (shipped):** new package defines the S4
spawn backend contract — `Provider.Wrap(exec.Cmd, WorkspaceView)` + `EffectiveState`
(surface mechanism + reason, provider), enforcement vocabulary (landlock/seatbelt/bwrap/none),
`Config{Filesystem,Network}` and `New(cfg)`. This build wires no OS mechanism yet (Landlock
needs the pending x/sys/go-landlock approval — §9 T1; Seatbelt needs the macOS helper
toolchain, unavailable in this dev env), so `New` returns the no-op provider whose
`EffectiveState` reports requested-but-unavailable surfaces as `none (reason)` and disabled
ones as `disabled by config` — the SPEC-006 §II.7 downgrade contract, fully tested
(3 tests). No-op `Wrap` keeps the spawn path identical regardless of mechanism presence.
Next (2b): real mechanisms behind build tags + detection at `New`, then R1 `AgentCommand`
spawn helper + `Wrap` call sites + executeLocal env-floor parity. Gates: backend 40/40,
complexity ✅.

**R1 — `platform/process` spawn helper + `executeLocal` env parity (shipped):** new
`process.AgentCommand(ctx, name, args...)` centralizes child creation (Setpgid process-group
isolation) and `process.KillGroup(cmd, sig)` centralizes group reaping. Both agent spawn
sites now use them: the persistent-shell factory (`shell/terminal.go`, killAll) and
`executeLocal` (`core/tools/terminal.go`, cancel-kill). `executeLocal` now receives the
workspace root and applies the SAME env floor as pooled shells
(`shell.BuildSandboxEnv`, exported wrapper) — closing the measured Risk-4 gap where one-shot
children inherited the backend env (real HOME) when filesystem confinement was off; no-root
one-shots keep the historical behavior. Tests: Setpgid assertion, KillGroup kills the whole
group incl. a grandchild, env floor unchanged for pooled shells. This is the single spot
`Provider.Wrap` attaches in 2b. Gates: backend 41/41, complexity ✅.

**2a-wiring — S4 seam plumbed end-to-end with the no-op provider (shipped):** the sandbox
Provider now flows from the composition root to every agent spawn: `InitializeAgentStack`
builds `sandbox.New(Config{Filesystem: FilesystemEnabled()})` → `TerminalTools` carries it →
pooled shells (`WorkspacePolicy.Sandbox`, applied in `newPersistentShell` BEFORE `cmd.Start`)
and `executeLocal` one-shots (Wrap on the built cmd). No-op Wrap keeps behavior identical;
the seam is proven live by a fake-provider test (Wrap invoked once with the correct
root/network view). Epoch-based recycle intentionally NOT forced yet — it has no consumer
until a real mechanism changes same-key policy. Next (2b): real Landlock/Seatbelt mechanisms
behind build tags + detection at `New` — **blocked on the §9 T1 dependency approval** (x/sys
promotion or go-landlock) and the macOS helper toolchain. Gates: backend 41/41, complexity ✅.

**2a-tests — `executeLocal` env-parity behavior test (shipped):** proves Risk-4 fix by
side effect (direct `.sandbox` output is redacted by the invisibility layer): a one-shot
`$HOME` write lands under `{workspace}/.sandbox`, never the real home; no-root one-shots
keep legacy behavior. Gates: backend 41/41, complexity ✅.

**Env-secret audit fix — default env allowlist (shipped):** `prepareShellEnv` now applies a
curated safe allowlist (locale/terminal/toolchain-hint vars: LANG, LC_*, TZ, TERM,
COLORTERM, NVM/PNPM/PYENV/POETRY dirs, GOROOT/GOCACHE/JAVA_HOME/…) when NO allowlist is
configured — previously an empty allowlist passed the ENTIRE backend environment (service
credentials, SSH_AUTH_SOCK, connector tokens) into every agent shell. Explicit operator
allowlists are honored verbatim (never widened); PATH + .sandbox floor vars are added
separately. Tests: secrets absent + safe vars present under nil allowlist; explicit list
still strict. Gates: backend 41/41, complexity ✅.

**Phase 3 step 2 — workspace storage accounting (shipped):** new `platform/sizewatch`
(`SumDir` walk + TTL-cached per-workspace sizes, `Forget` on delete). `TerminalTools` gained
`SetStorageOverLimiter`; `ExecuteCommand` blocks shell spawns for a workspace past
`max_storage_gb` with a message that labels it ACCOUNTING (not a quota — TOCTOU window
acknowledged). Wired in `InitializeAgentStack` from host settings (TTL 60s). Tests: SumDir
totals, TTL staleness + refresh + Forget. Phase-3 items: per-child rlimits in the Linux runner env for non-service runs (cgroup covers
the service; Darwin memory is OS-unenforceable and reported) — optional follow-up.

**Full-change review pass (shipped after implementation):** `go vet ./...` clean ·
`go test -race` on every concurrency-touched package (shell pool re-key, process, tools,
automation, egress, guardrails) clean · fix #1: egress now uses ONE pooled transport
(previously built per request) · fix #2: the network-deny reason string de-duplicated to a
single const (SSOT — it was duplicated between sandbox.New and the Landlock provider) ·
fix #3 (missed target): SPEC-009 bumped to v1.1 with the connector-SEND gating change record
(§9 T9 required it; only SPEC-006 had been done) · fix #4: architecture.md gained the
enforcement-checklist pointer (plan §8). Deferred/documented residuals unchanged: Linux-CI
runtime probe, macOS seatbelt toolchain, per-child runner rlimits (non-service), settings-API
Effective surface.

**Phase 4 — deployment + docs (shipped):** `docs/services/llm-proxy.launchd.plist` — macOS
launchd daemon template (dedicated unprivileged user, single root via LLM_PROXY_HOME,
run-at-load, restart-on-failure, logs under the root; no Keychain dependency — secrets are
the encrypted secrets.json). `docs/service_setup.md` gained the macOS install section (dscl
user, launchctl bootstrap) + the Linux cgroup note. `docs/architecture.md` directory map now
covers `core/egress`, `platform/{network,process,sandbox,sizewatch}`. The stale Wazero
future-work line in `docs/PLANS/ARCHIVE/cross-cutting/system-blueprint.md` is marked
SUPERSEDED. Env-secret audit shipped earlier as the default-allowlist fix.

**Phase 3 step 1 — resource-limit primitives + Linux service cgroups (shipped):** new
`process.ApplyChildLimits` (x/sys/unix rlimits) applies FSIZE/NPROC/CPU on Darwin AND Linux
and SKIPS RLIMIT_AS on Darwin (reported, never an error — the measured "cannot set" fact),
returning applied/skipped lists for honest Effective state; child-only (post-fork pre-exec
callers). Unit test asserts the AS skip on Darwin vs apply on Linux. `docs/services/
llm-proxy.service` gained cgroup v2 limits — `MemoryMax=8G`/`MemoryHigh=6G` (whole service
scope incl. shells/toolchains; Phase-3 acceptance re-checks node/go/npm) and `TasksMax=512`
(fork-bomb cap per service, no per-uid blast radius). Next: wire limits into the Linux
runner env (per-child caps for non-service runs) and implement the `max_storage_gb`
workspace accounting loop + Effective surfacing.

**2b-linux — self-exec runner + Landlock mechanism (shipped, Linux):** the sandbox-runner
is in-process: `main()` calls `sandbox.RunChildIfRequested()` before anything else; the
child (spawned as THIS binary with `LLMPROXY_SANDBOX_RUNNER=1` + a serialized profile) applies
confinement and `syscall.Exec`s the real argv — an unconfined child is never run (fails
closed). `landlockProvider.Wrap` rewrites the cmd to the runner with a profile derived from
the workspace root + PATH toolchain dirs + Linux support paths (`DefaultProfile` +
`NeedTraverse`, deny-by-default incl. secrets/sockets). `New()` now returns the Landlock
provider on Linux kernels ≥5.13 (capability gate) and the no-op elsewhere. Tests: runner
exec path verified cross-platform via helper-process (confinement stub, real argv exec,
fail-closed), profile encode round-trip, Linux `ApplyLandlock` mapping tests, and a real
Landlock confinement probe test (`/etc/hostname` denied, granted dir readable) that runs on
Linux CI. macOS seatbelt remains unbuildable here — `Effective` reports the downgrade.
**Remaining before Phase 3:** Linux-CI runtime validation of the probe + a pass at the
macOS seatbelt helper on a machine with the framework. Gates: mac 41/41, `GOOS=linux
build/vet/test -c` ✅, complexity ✅.

**2b-linux — Landlock backend (draft notes; superseded by the shipped slice above):** §9 T1 resolved — `golang.org/x/sys` promoted
v0.42→v0.47 and made a DIRECT dependency (it already ships the Landlock constants, struct
types, and syscall numbers; wrappers hand-rolled, no new module). New OS-neutral grant model
(`platform/sandbox/profile.go`: `FSPermission` read/write/traverse, `Profile`, `DefaultProfile`
= workspace + .sandbox rw, toolchain/support ro, deny-by-default incl. secrets/sockets, plus
`NeedTraverse` ancestor marking incl. `/`). Linux-only `landlock_linux.go`: permission
mapping, kernel capability gate (≥5.13 via uname), `ApplyLandlock` (create ruleset → add
path-beneath rules → no_new_privs → restrict_self), missing-grant-paths skipped. Verified on
this host: mac build+tests 41/41, `GOOS=linux build/vet/test -c` all pass; runtime probe
tests run on Linux CI (self-restrict must happen in a child). **Next: self-exec sandbox-runner
+ main() bootstrap so `ApplyLandlock` runs in the child before the real argv; detection →
`New` selection on Linux + Effective reporting; CI runtime probes.** Seatbelt on macOS cannot
be compiled on this host (`-framework Sandbox` absent from CLT — verified) — macOS keeps the
uid-first/launchd path with the honest `Effective` downgrade.

### Post-review hardening pass (2026-09-06/07) — second pass log

Applied to the same staged tree after the full-change review pass (findings in the first-pass
report): Landlock ABI correctness, egress-proxy wiring, Effective surfacing, honest UI copy,
single-sourced scope mapping, and doc/comment fixes.

| Fix | What changed | Files |
|---|---|---|
| **Landlock ABI correctness** | Replaced the release-string capability check with a real `landlock_create_ruleset` VERSION probe; rulesets request only the rights the detected ABI supports (TRUNCATE is ABI v3+) and are created with the exact attr size that ABI expects (8/16/24 bytes) — older kernels reject mismatched sizes/rights. Network-off children get TCP bind/connect denial on ABI v4+ (kernel ≥ 6.7); below that `Effective` reports the gap. Loader/lib dirs (`/lib*`, `/usr/lib*`, `/usr/libexec`, multiarch) are granted so dynamically linked binaries can exec under the jail. No-op-path sandbox tests skip when a mechanism is active (keeps Linux CI green); added exec-under-jail, ruleset/attr-size, and provider-selection probes (Linux-tagged). | `internal/platform/sandbox/*` |
| **Egress-proxy in-process wiring bug** | The guarded `DialContext` refused loopback, so every proxied fetch died before reaching the proxy. The configured proxy address is now the single permitted loopback dial (target policy unchanged); added an end-to-end proxied-fetch regression test. | `internal/core/tools/network.go` (+test) |
| **`Effective` surfaced** | Provider registered on `AppContext`; `HostSettings()` GET stamps a read-only `effective` projection (filesystem/network mechanism + reason, provider) that is stripped on PUT (never persisted); startup log line with effective state. | `models/host_settings.go`, `internal/app/*`, `internal/core/assistant/registry.go` |
| **LAN scope = LAN only** | `DisabledToolNamesForScope` now hides internet-only egress (search/connector send) under a `lan` grant with a hard-gate (overrides cannot re-expose); scope→config mapping single-sourced in `models.WithRunScope` (guardrail validator + runtime tool config). | `models/config.go`, guardrails + tests |
| **Honest UI copy** | Security page no longer claims a shell-level "hard ceiling" or platform-unqualified "confined (on)": the Effective card reads the backend projection; master/network/filesystem/egress copy states what is actually enforced (restart caveats, macOS no-jail, memory deployment-level). AutomationForm warns when the host switch makes a grant inert. | `frontend/.../SecuritySettings.vue`, `AutomationForm.vue`, `types/admin.ts` |
| **Docs/comments** | SPEC-006 §II.7.4 + SPEC-009 changelog corrected to what is enforced (Effective per surface: filesystem/network; connector-send boundary incl. the inbound-conversation reply exemption); stale "(no-op until Phase 2)" and wiring comments fixed. | `docs/SPECS/*`, sandbox/egress/shell/process comment fixes |
| **Regression tests** | Proxied-fetch round trip; Landlock ABI/attr-size/exec probes; dual-scope idle reap; storage-over spawn block; run-meta network-scope audit; egress CONNECT-by-hostname (DNS at the proxy); LAN-scope search/notify matrix. | listed per package |
| **complexity gate note** | `tools/check-complexity` walked `backend/internal` relative to cwd, so the documented `backend/` invocation was a vacuous pass. The tool now anchors at the Go module root, and the two guardrails functions this plan/pass pushed over the ≤12 limit were split back under it (`ValidateToolCall` → `securityHardGates`+`validateCategory`; `DisabledToolNamesForScope` → `scopeTierDisabled`). The correctly-rooted gate still exits 1 on ~70 PRE-EXISTING >12 functions elsewhere (stream.go, model_handlers.go, etc.) — the plan's earlier "complexity ≤12 ✅" checkpoint claims were vacuous and the pre-existing debt is follow-up, not this plan's code. | `tools/check-complexity/main.go`, `internal/core/assistant/guardrails/guardrails.go` |
| **setup.sh review (out of hardening-pass scope)** | Reviewed the Linux-only installer/uninstaller (`setup.sh`, 922 lines): flows are idempotent, dry-run-capable, tear down what was installed, self-heal root-owned builds, and grant external-path access via traverse-only ACLs — solid. macOS deployment is intentionally NOT added to setup.sh (systemd/dscl/launchctl bifurcation is a separate sizable change): the header + non-interactive error now point macOS operators at the dedicated-user launchd artifact (`docs/services/llm-proxy.launchd.plist`, service_setup.md). Fixes applied: uninstall/purge now read `ExecStart` from the installed unit so a custom binary path is torn down correctly; removed the nonexistent `MemoryHard` systemd directive mention from the unit comment (`MemoryMax` is the OOM ceiling). | `setup.sh`, `docs/services/llm-proxy.service` |

### Post-review resolution pass (2026-09-07)

Findings from the second review resolved (all verified locally except the Linux
runtime runs, which are now automated in CI):

| Resolution | What changed |
|---|---|
| **Linux CI conformance matrix (§9 T2)** | New `sandbox-conformance` job: ubuntu-24.04 + ubuntu-22.04 (both expect Landlock + ABI v4 TCP deny — the 22.04 image now ships a 6.8 HWE kernel, so the ABI < v4 FS-only downgrade is unit-covered rather than runnable on hosted runners), macos-14 (no-mechanism downgrade). `TestEffectiveMatchesExpectedHost` (env-driven, `LLMPROXY_EXPECT_FS`/`LLMPROXY_EXPECT_NET_DENY`) FAILS on a silent downgrade instead of skipping, so the OS matrix can no longer be green while unenforced. |
| **Complexity gate truthful** | `tools/check-complexity` now anchors at the module root and carries a 69-entry **known-debt baseline**; it exits 0 on pre-existing debt (printed) and fails only on NEW functions over 12. Previously the documented invocation was a vacuous pass. |
| **Egress proxy token (§9 T5)** | Per-process 128-bit token: the proxy requires `Proxy-Authorization` (constant-time compare) and the agent's transport/shell env carries it, so other local processes cannot ride the proxy. Per-RUN scoping remains deferred (the transport is pooled per process). |
| **Audit test hardened** | Replaced the line-grep with an AST scan (`http.DefaultClient`, `http.Get/Post/PostForm/Head`, `http.Client{}` without Transport, `exec.*` outside the allowlist, `os.StartProcess`) — no longer bypassable by line splits/aliasing/comments. |
| **rlimit primitive wired** | The runner applies `RLIMIT_FSIZE` (per-file backstop derived from `max_storage_gb`) via `process.ApplyChildLimits`; AS/DATA/NPROC deliberately not set (systemd cgroup is the service lever; per-UID NPROC self-DoSes on a shared uid — plan D5/R7). |
| **`enabled:false` bootable (§4.1 decision)** | Bootstrap warns and continues with one-shot terminal execution instead of `log.Fatal`; UI copy updated (no more boot-fatal claim). |
| **D6 per-key merge + presence** | `mergeAppConfigDefaults` backfills numerics per key and uses a decode-only `SectionPresent` marker so an explicit `enabled: false` section survives reload while an absent section still takes defaults (the marker survives the store's JSON deep-copy and is stripped from the API projection). |
| **Perf baselines (§9 T7)** | Added repeatable benchmarks: profile encode/decode (spawn serialization) and egress-proxy round trip + policy decision (measured locally: localhost hop ~0.13 ms/op, policy ~37 ns/op). Full toolchain baselines still need a Linux/bench host. |

**Residuals after the resolution pass:** Linux runtime confirmation now runs in CI (the
matrix above; first run pending on the branch); per-RUN proxy scope (per-process token
shipped); macOS Seatbelt (toolchain absent on the dev host); memory enforcement remains
deployment-level (systemd cgroup; per-child AS/NPROC intentionally not wired); complexity
debt baseline (69 functions) is tracked, not refactored. `sandboxing.enabled: false` is now
bootable with a warning.

**Resume gate:** `cd backend && go build ./... && go test ./...` (42/42 packages ok on macOS)
+ `go vet ./...`; Landlock runtime probes require the Linux CI job. Nothing has been
committed (repo git rules).
