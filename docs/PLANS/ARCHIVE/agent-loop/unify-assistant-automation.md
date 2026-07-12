---
status: superseded
last_reviewed: 2026-07-11
---

# Unify Assistant and Automation Agent Paths

## Status

Implemented 2026-06-15

## Summary

The assistant and automation code paths were sharing the same `Agent` struct but
diverging at runtime through `isAutomationCtx` / `isAutomation` branching in
`stream.go`, `session.go`, and `agent.go`.  The `isAutomationCtx` flag was a
heuristic that scanned history for `"TASK: You are an autonomous agent"` — a
transport-layer concern leaking into the core agent loop.  This caused the
assistant path to miss `tool_choice: "required"`, `reasoning_budget`, and
`temperature` settings, leading to reasoning-stuck loops.

## Changes

### Core (`internal/core/assistant/`)

- **`agent.go`**: Added `EnableHotMemory` field to `AgentOptions`. Removed
  `suppressReasoningBudget` field.

- **`stream.go`**: Removed `isAutomationCtx` parameter from `buildChatRequest`,
  `prepareMessagesForTurn`, `shouldPrefill`. Deleted `findAutomationCtx()`.
  `buildChatRequest` now always applies `tool_choice: "required"` (when native
  tools are available), `temperature` (when configured), and `reasoning_budget`
  (when configured).

- **`session.go`**: Removed `isAutomation` field from `runSession`. Removed
  marker-scan loop from `newRunSession`. `handleNoToolCalls` unified into a
  single handler (exit on readable text despite parse error, nag on empty).
  `maybeFlushMemoryBeforeTurn` uses `enableHotMemory` instead of
  `findAutomationCtx`.

- **`handleNoToolCalls` unified**: Single handler for both contexts. Parse error
  with readable text → exit. Empty response with tools → nag. No tools + content
  → exit. Identical behavior regardless of context type.

### Handler Layer (`internal/transport/http/`)

- **`agentbuilder.go`** (new): Shared `AgentBuilder` with chainable methods
  (`WithModelConfig`, `WithHotMemory`, etc.).

- **`assistant_handlers.go`**: Replaced inline `AgentOptions` construction with
  `AgentBuilder`. Sets `EnableHotMemory: true` for assistant sessions.

### Automation (`internal/core/automation/`)

- **`executor.go`**: Removes unused `NoToolCallHandler` option — the unified
  handler in the agent core handles both contexts identically.

## Behavioral Changes

| Setting | Before (assistant) | Before (automation) | After (both) |
|---|---|---|---|
| `tool_choice` | Unset | `"required"` (native tools) | `"required"` when native tools + tools available |
| `temperature` | Unset | `0.1` default | From model config |
| `reasoning_budget` | Not sent | Sent to API | Sent when configured |
| Memory injection | Yes | No | When `EnableHotMemory: true` |
| Pre-sieve memory nudge | Yes | No | When `EnableHotMemory: true` |
| No-tool-call handling | Exit on content | Nag + retry | Unified (exit on readable text when non-native, nag on empty or native-tools protocol violation) |
