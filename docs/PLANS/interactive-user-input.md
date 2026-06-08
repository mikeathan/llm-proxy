# Plan: Interactive User Input (ask_user Tool)

**Status:** proposed
**Date:** 2026-06-08
**Related specs:** SPEC-001, SPEC-007

## Problem

Some automation tasks (e.g., `memory-cascade-test`) are designed for multi-turn
interactive flows where the agent saves facts, then a human asks questions, and
the agent answers. Currently the entire prompt is dumped as one monolithic task,
making these prompts unusable without rewriting them for self-execution.

## Solution

Add an `ask_user` tool that lets the agent pause execution and wait for user
input during an automation run. The agent sends a question, the automation
pauses, the frontend polls for pending questions, the user types a response,
and the agent resumes with the answer as a tool result.

## Implementation

### Backend

**1. New tool: `ask_user`** (`internal/core/tools/ask_user.go`)
- Manifest entry in `internal/core/tools/manifests/`
- Accepts `question` (string) and optional `choices` ([]string) for structured input
- Returns "awaiting user response" immediately (non-blocking)
- Stores the pending question in a store accessible to the frontend

**2. New store: PendingQuestionStore** (`internal/platform/storage/`)
- In-memory store keyed by workspace/run-id
- Methods: `SetPending(workspaceID, runID, question)`, `GetPending()`, `Respond(workspaceID, runID, response)`, `Clear(workspaceID, runID)`
- The `Respond()` call unblocks the agent's waiting goroutine

**3. New API endpoints** (`internal/transport/http/`)
- `GET /admin/api/conversation/pending-questions` — returns unanswered questions
- `POST /admin/api/conversation/respond` — accepts `run_id`, `response`; unblocks the agent

**4. Agent integration** (`internal/core/assistant/`)
- When `ask_user` is called, the tool execution returns a pending status
- The agent's tool processing loop detects this and **pauses** (blocks on a channel)
- The channel is resolved when `Respond()` is called via the API
- The agent receives the response as the tool result and continues

**5. Guardrails**
- `ask_user` is always allowed (no whitelist check needed — it's a communication tool)
- Guardrail can be set to block it per-workspace if needed

### Frontend

**6. Notification component** (`frontend/src/components/`)
- Polls `GET /admin/api/conversation/pending-questions` every 5 seconds
- When a pending question is found, shows a modal/banner with the question text and an input field
- User types response, clicks "Submit", sends to `POST /admin/api/conversation/respond`
- Agent unblocks and continues execution

## File Change Checklist

1. `internal/core/tools/manifests/ask_user.json` — manifest
2. `internal/core/tools/ask_user.go` — tool implementation
3. `internal/core/assistant/registry.go` — register ask_user tool
4. `internal/platform/storage/pending_questions.go` — question store
5. `internal/transport/http/question_handlers.go` — API handlers
6. `internal/transport/http/router.go` — register routes
7. `frontend/src/composables/usePendingQuestions.ts` — polling logic
8. `frontend/src/components/AskUserModal.vue` — notification UI
9. `docs/PLANS/interactive-user-input.md` — this plan

## Edge Cases

- **Multiple pending questions**: Queue them, answer one at a time
- **Timeout**: If user never responds, agent resumes after timeout (default 10 min)
- **Agent finalizes before user responds**: Pending question is cleared
- **User closes browser**: Questions persist in the store until answered or expired
