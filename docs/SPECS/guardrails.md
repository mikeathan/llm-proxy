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
2. **Terminal** — Command whitelist, blocked patterns, path jail prevention, timeout enforcement,
   external path access (workspace-level only).
3. **Filesystem** — Path validation, extension whitelist, filename blocking, read-only enforcement,
   path jail.
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
2. `onGuardrail` callback registers a channel in `GuardrailDecisionStore` + publishes SSE event.
3. Agent blocks on channel for up to 60s waiting for user decision.
4. User approves/denies via `POST /admin/api/conversation/guardrail-decision`.
5. If approved with `persist: true` → `PersistOverride()` writes to workspace `config.yaml`.
6. Agent continues or fails based on decision.

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
