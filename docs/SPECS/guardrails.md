---
id: SPEC-006
title: Guardrail Engine
version: "1.0"
status: stable
last_updated: 2026-05-28
constitution_references: [I.5]
related_specs: [SPEC-001]
supersedes:
---

# SPEC: Guardrail Engine

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

Merging is via `AgentGuardrailsConfig.MergeWith()` which ORs booleans, overrides non-zero ints,
and merges slices with dedup.

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

### 6. Schema/Policy Consistency (Static Tool Availability)

`DisabledToolNames(workspaceID)` returns the tool names whose category has a hard "disabled by
policy" gate (`Enabled == false`): `notify_user` (Communication), `internet_search` (Search),
`fetch_url` / `scan_local_network` / `get_network_info` (Network). Workspace overrides are merged
exactly as `ValidateToolCall` resolves them, and tools with an active in-memory override are
skipped. Terminal/filesystem categories are allowlist-based (no `Enabled` hard gate) and are not
covered.

The agent tool schema is derived from this set at one narrow waist (`resolveToolProvider` in
`NewAgent`), so no strategy or channel can ever observe a tool the policy statically disables.
`RequireReview`, allowlists, and blocked-domain checks remain execution-time gates — they never
hide a tool from the schema.

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
