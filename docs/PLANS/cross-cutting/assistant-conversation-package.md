# Assistant conversation package (deferred extraction)

**Status:** proposed
**Date:** 2026-09-12
**Related Specs:** SPEC-001 (Agent Loop)

## Context

Split out of the now-retired `assistant-package-restructure` plan. The three safe extractions landed — `assistant/usage`, `assistant/repetition`, `assistant/toolpolicy` — plus the two stray-test fixes (root `assistant/` went **51 → 42** files). `conversation` was deferred because it is **not** a safe mechanical move, and forcing it would have introduced duplication.

## Why it was deferred

1. **Child → parent import.** `conversation_service.go` + `conversation_helpers.go` reference root concrete types/consts ~130 times (`Agent`, `AgentEvent`, `NewAgentBuilder`, `Engine`, `ToolProvider`, `Observer`, `GuardrailDecisionStore`, `ChannelAssistant`, `PhaseSession*`, `Event*`, `MaxHistoryChars`, plus helper funcs). As `assistant/conversation` they must import root and qualify every reference.
2. **Kept-in-root ports.** `EventPublisher` / `EventRecorder` cannot move — root's `agent_builder.go:20` uses `EventPublisher` — so they stay in root while the service imports them. `conversation.go` must be split accordingly.
3. **Test doubles (the real blocker).** `conversation_service_test.go` (755 lines) drives the loop with `MockClient` / `MockProvider` / `MockEngine`, which live in root `_test.go` and are **not importable from another package**. Keeping the tests in root is also impossible: an in-package `assistant` test that imports `assistant/conversation` — which imports `assistant` — is an import **cycle**. So the tests must move **and** carry their own doubles (~60 lines of duplication) unless the doubles are first made shareable.

## Step 0 (prerequisite) — consolidate the LLM/tool test doubles

- Add an importable support package, e.g. `backend/internal/testing/assistanttest/`, holding `MockClient`, `MockProvider`, `MockEngine`.
- It must **not** import `assistant`: the mocks satisfy the interfaces structurally (`ListTools` / `GetSystemPrompt` / `UseNativeTools`; `ExecuteTool`), and only need `proxy`. That keeps it importable from in-package `assistant` tests without a cycle. Note `internal/testing/mocks` is **not** an option — it imports `assistant` (that cycle is exactly why the registry tests already use a local `stubSearchSecrets`).
- Update the root assistant tests to use the shared doubles; behaviour stays identical.

## Then — extract `assistant/conversation/`

1. Move `conversation.go`, `conversation_helpers.go`, `conversation_service.go` and their mirroring tests (`conversation_helpers_test.go`, `conversation_service_test.go`).
2. Keep `EventPublisher` / `EventRecorder` in root (append to `agent_events.go`); split them out of the moved `conversation.go`.
3. Qualify root references (`assistant.X`) in the moved sources and import `assistant` from the child package.
4. Update `internal/transport/http/handlers/assistant_handlers.go` to import `assistant/conversation` (`ConversationService`, `NewConversationService`).
5. Tests use the `assistanttest` doubles from Step 0.

## Expected result

Root drops ~5 more files (→ ~37). No behaviour change — pure move.

## Verification

- `cd backend && go build ./... && go vet ./... && go test ./... -race -coverprofile=coverage.out -covermode=atomic && go run ./tools/check-complexity/`.
- Assert: no import cycles (the build proves it); handler tests still green; every moved test mirrors its source.

## Risks

- **Cycle risk is the whole game:** root must never import `assistant/conversation`.
- Large mechanical qualification; do it in one change and rely on the compiler, not piecemeal.

## Out of scope

- The loop core, strategies and guards (they take `*runSession`; needs a separate session-facade design).
