---
id: SPEC-006
title: Guardrail Engine
version: "1.1"
status: stable
last_updated: 2026-09-06
constitution_references: [II.3]
related_specs: [SPEC-001]
supersedes:
---

# SPEC: Guardrail Engine

## Changelog

- **1.1 (2026-09-06)** — OS Enforcement Layer + network default-off (new §II.7). Host
  sandboxing switches (`sandboxing.filesystem`, `sandboxing.network`) are hard gates OUTSIDE
  the override stack (§II.2): `MergeWith` may tighten, never re-enable a switch the host
  turned off. Network-off hides network-category tools at the schema narrow waist (§II.6) in
  both native and XML-text modes and denies residual calls synchronously (never the approval
  flow). Downgrade-never-bypass + `Effective` read-time reporting added (host-settings GET,
  never persisted; see §II.7.4). Frontmatter
  `constitution_references` corrected `[I.5]`→`[II.3]` (Constitution §I has four items; the
  sandbox law is Constitution II.3). Mechanism and phases: plan
  `docs/PLANS/cross-cutting/agent-os-sandboxing.md` (rev 2).

## I. Intent

The guardrail engine validates every tool call against configurable rules before execution.
It prevents the agent from executing dangerous commands, accessing restricted paths, or
leaking secrets. Blocked calls pause for user approval via a decision flow.

## II. Functional Requirements

### 1. Validation Hierarchy

Tool calls are validated in order:

1. **Global** — Secret pattern detection (API keys), user-defined blocked patterns (regex).
2. **Terminal** — Command whitelist, blocked patterns, blocked filenames (inherited from the
   filesystem `blocked_filenames` list **plus** the internal invariant paths, merged via
   `effectiveBlockedFilenames`), path jail prevention, timeout enforcement, external path
   access (workspace-level only). The whitelist is enforced **per command segment** — the
   command is decomposed with shell-syntax awareness (quotes, heredocs in every marker form,
   here-strings, escaped delimiters, newlines as separators) so a disallowed command cannot
   ride in on the tail of an allowlisted one. Commands with **unbalanced syntax** (an
   unterminated quote or heredoc, which would make the tail opaque to the whitelist) are
   rejected fail-closed; `executeShell` re-checks before touching the persistent shell so a
   malformed command can never wedge the shared session. In addition to the input-side
   denial, terminal **output** is scrubbed of blocked-path references
   (`redactBlockedPaths`): recursive commands (`find .`, `du -sh .`, `ls -la`, `tree`)
   emit blocked paths even when no explicit operand was written, so the same invariant is
   enforced on output before the result reaches the agent.
3. **Filesystem** — Path validation, extension whitelist, filename blocking (user
   `blocked_filenames` merged with internal invariant paths), read-only enforcement, path
   jail. Directory listings hide the same merged blocked set — an internal path (`.sandbox`)
   must not even appear as an entry.

Internal invariant paths (currently `.sandbox`, the sandbox runtime directory) are defined
once in `tools/security.go` (`internalBlockedPaths`) and enforced uniformly across every
surface — filesystem validation, directory listings, terminal input, terminal output — via
the shared `blockedFilename` / `blockedPathEntry` helpers. Adding a new internal path is a
one-line list entry; no per-tool code.
4. **Network** — LAN/Internet boundary, domain blocking, IP blocking.
5. **Search** — Query length limits, site blocking.
6. **Communication** — Review requirement, message limits.

### 2. Override Stack (highest priority last)

1. Provider Manifests (embedded defaults from `manifests/*.json`).
2. `settings.yml` → `guardrails:` (user-level overrides).
3. `{workspace}/config.yaml` → `guardrails:` (workspace-level overrides).

Merging is via `AgentGuardrailsConfig.MergeWith()`: most categories OR booleans, override
non-zero ints, and merge slices with dedup. The **network block** is presence-aware instead —
if a layer's decoded document contains a `network:` block, its `enabled` / `allowlanaccess` /
`allowinternetaccess` values replace the inherited ones (so a workspace can **restrict** with an
explicit `false`, not only loosen), while a layer that omits the block inherits the baseline.
Presence is recorded at YAML decode time (`NetworkGuardrailsConfig.UnmarshalYAML`) and is never
persisted; L0 (`sandboxing.network`) remains the hard ceiling regardless.

### 3. Guardrail Decision (Approval) Flow

When a tool call is blocked:

1. `ValidateToolCall()` fails → creates `GuardrailBlockedPayload` with `decision_id`.
2. `onGuardrail` callback registers a channel in `GuardrailDecisionStore` + publishes SSE event. The event is stamped with the **producer's channel** (`ChannelAssistant` for chat, `ChannelAutomation` for runs): the event bus partitions by channel and defaults empty to `automation`, so an assistant approval without a channel never reaches the assistant SSE and the chat shows no banner while the backend waits the full timeout.
3. Agent blocks on channel for up to `GuardrailApprovalTimeout` (default 5 min, per-model configurable via `guardrail_approval_timeout_seconds`) waiting for user decision.
4. User approves/denies via `POST /admin/api/conversation/guardrail-decision` (the `GuardrailBanner` component posts directly; the assistant chat wires `@allow/@deny` to `submitDecision`, mirroring the automation console).
5. If approved with `persist: true` → `PersistOverride()` writes to workspace `config.yaml`.
6. Agent continues or fails based on decision.

**Automation runs never wait for approval** (Constitution II.10): the
`automation` channel has no interactive user, so non-security guardrail
violations are denied immediately — the tool result is fed back to the model
with hard policy guidance ("Action blocked by security policy. Do NOT retry,
rephrase, or attempt the same outcome via a different path") and the run
continues. Waiting for an approval prompt in an unattended run previously
burned the full `GuardrailApprovalTimeout` and aborted the run with a
misleading `context deadline exceeded` when the run's own deadline expired.
Security-boundary violations are always synchronous rejections regardless of
channel.

Synchronous rejections (no approval flow — e.g. path/workspace boundary checks that
never prompt) publish a `guardrail_violation` lifecycle event with payload `{tool, error}`.
The frontend surfaces it as its own chat segment so the block is visible without a preceding
`tool_call`/`tool_result` pair.

**Tool-terminal errors are a separate axis.** A tool that fails for an operator-actionable
reason (missing/rejected credential) is classified by the **agent loop**, not this engine:
the tool is disabled for the run and, in automation, the run fails — this is about the tool
*not working*, not about *permission*. Guardrail denials remain about permission and route
through the flow above. See SPEC-001 §II.6 and
`docs/PLANS/cross-cutting/tool-error-classification.md`.

### 4. Tool-Level Guardrail Configuration

Each tool manifest (`manifests/*.json`) defines default guardrails:

```json
{
  "guardrails": {
    "enabled": true,
    "require_review": false,
    "max_messages_per_task": 5
  }
}
```

- `enabled`: Whether guardrail validation runs for this tool.
- `require_review`: Whether all calls to this tool require human approval.
- Communication tools default to `require_review: true` with a per-task message cap.

### 5. External Path Access (Terminal)

`TerminalGuardrailsConfig.AllowedExternalPaths` lets a workspace-level override grant the agent
access to absolute paths outside the workspace jail. Constrained to workspace-level config only.

A small **implicit always-safe set** is exempt from the absolute-path jail without any
configuration: currently `/dev/null` (the universal output sink — writes are discarded, reads
return EOF). It is matched by exact literal only, so permitting it does not open up `/dev/*`
(`/dev/random`, `/dev/urandom`, block devices, etc. remain blocked). This keeps the standard
`... > /dev/null` / `2>/dev/null` idiom working without requiring an `allowedexternalpaths` entry.

### 6. Schema/Policy Consistency (Static Tool Availability)

`DisabledToolNames(workspaceID)` returns the tool names whose category has a hard "disabled by
policy" gate (`Enabled == false`), or whose category is closed by the host-level network gate
(§II.7): `notify_user` (Communication), `internet_search` (Search),
`fetch_url` / `scan_local_network` / `get_network_info` (Network). Workspace overrides are merged
exactly as `ValidateToolCall` resolves them, and tools with an active in-memory override are
skipped. Terminal/filesystem categories are allowlist-based (no `Enabled` hard gate) and are not
covered.

The agent tool schema is derived from this set at one narrow waist (`resolveToolProvider` in
`NewAgent`), so no strategy or channel can ever observe a tool the policy statically disables.
`RequireReview`, allowlists, and blocked-domain checks remain execution-time gates — they never
hide a tool from the schema.

`internet_search` is additionally hidden while no search provider is usable. The engine holds a
live availability predicate (`SetSearchAvailable`, mirroring `SetHostNetworkAllowed`) that reports
whether the selected provider is registered and its `search:<provider>` secret is non-empty. It is
read with no I/O from the live registry config + secrets store, so an operator change applies to
the next run without a restart. A nil predicate leaves the previous behaviour (availability
governed only by the `Search.Enabled` tier). Schema-hide is the only gate: a residual call reaches
the tool and returns its own `ErrSearchNotConfigured`, recorded as a non-approvable tool result —
never an approval prompt.

### 7. OS Enforcement Layer (host sandboxing — under guardrails, never a replacement)

Host-level OS sandboxing (plan: `docs/PLANS/cross-cutting/agent-os-sandboxing.md`) sits
*underneath* this engine as a behavioral backstop for agent-spawned child processes. Contract:

1. **One pipeline, two enforcement kinds.** Guardrail stages reason over tool-call *intent*
   (schema presence, per-call validation, approval). The OS layer reasons over child-process
   *behavior* (filesystem/network syscalls) at spawn time. Neither replaces the other:
   in-process tools cannot be OS-sandboxed; shell children cannot be Go-gated. Guardrail
   denial of intent and kernel denial of behavior compose as defense in depth.
2. **Host switches are hard gates outside the override stack.** `sandboxing.filesystem` and
   `sandboxing.network` are host-level ceilings. The override stack (§II.2) may loosen within
   the guardrail tier — workspace overrides and persisted approvals must NEVER re-enable a
   switch the host turned off. Effective scope is resolved per run as a function of host
   state + merged guardrails + the run's network grant (assistant: workspace scope;
   automation: declared grant or workspace scope).
3. **Network default-off.** When the effective network scope is none, network-category tools
   (`fetch_url`, `scan_local_network`, `get_network_info`, `internet_search`, connector-send)
   are hidden at the schema narrow waist (§II.6) in BOTH native and XML-text tool modes, and
   any residual call path returns a synchronous, non-approvable security-boundary rejection —
   never the approval flow (§II.3). Inbound webhook receipt is exempt (server-side,
   Constitution I.4); outbound connector sends are gated.

   The two host/workspace network abilities (LAN, Internet) are **independent** — Internet
   does not imply LAN. The resolved run scope is the full combination: `none` (off), `lan`
   (local network only), `internet_only` (internet with the local network blocked), or
   `internet` (both). At `lan`, internet-only egress (`internet_search`, connector send) is
   hidden and hard-denied; at `internet_only`, the local-network tools (`scan_local_network`,
   `get_network_info`, and `fetch_url` to private addresses) are hidden and hard-denied. The
   forbidden sets come from one source (`scopeForbiddenTools`), shared by the schema
   (`DisabledToolNamesForScope`) and the non-approvable hard gate (`securityHardGates`).
4. **Downgrade-never-bypass + Effective state.** If requested OS enforcement is unavailable on
   the host (kernel/ABI limits, deprecated or absent mechanism), the strongest available
   subset is enforced and `Effective` — a read-time projection surfaced next to `Functional`
   in host settings, never persisted into `settings.yml` — reports requested-vs-actual per
   surface (filesystem mechanism + reason, OS-network mechanism + reason, provider). Memory
   is deployment-level (Linux systemd unit cgroup `MemoryMax`/`MemoryHigh`; macOS cannot cap
   per-process memory — measured) and is NOT part of the per-process Effective projection.
   Silent absence of enforcement is a bug.
5. **Shell network state is a process-level OS property (D8).** When the effective run scope
   is none, shells are pooled under `networkOn=false` and the OS layer denies new TCP
   binds/connects where the kernel can (Landlock ABI v4+, kernel ≥ 6.7; this denies ALL
   binds/connects for network-off children, including loopback binds, and does not cover
   unconnected UDP `sendto`); where it cannot, `Effective` reports the gap and enforcement
   relies on the Go tool layer and the optional egress proxy. A `networkOn=true` shell reaches both LAN and the internet (the OS cannot
   split them) — LAN-vs-internet, including `internet_only`'s LAN ban, is enforced only on the
   in-process tools; closing the shell path requires the egress proxy (or an OS network jail).

Security-boundary denials from either kind surface as guardrail *rejections* on the existing
synchronous path (§II.3): `guardrail_violation` event, tool not executed, `stopBatch` set —
with the model told not to retry, rephrase, or route around the block.

## III. Error Handling

- Guardrail violation returns a specific error message appended as a tool result.
- The agent receives: "Guardrail violation: [rule description]".
- The tool is NOT executed — result is synthetic.
- `stopBatch` is set to true to prevent further tool execution in the same turn.

## IV. Configuration

- `settings.yml` → `guardrails:` for user-level overrides.
- `{workspace}/config.yaml` → `guardrails:` for workspace-level overrides.
- Tool manifest `manifests/*.json` for embedded defaults.
- `GuardrailDecisionStore` in-memory for pending approval requests.
