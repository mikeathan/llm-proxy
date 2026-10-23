# Run pipeline cleanup and agent commands — Part C of multi-agent delegation

**Status:** approved (2026-10-23). D1–D3 decided. C0 and C1 done; C2 next.
**Related Specs:** SPEC-001 (Agent Loop), SPEC-007 (Automation Dispatcher), SPEC-009 (Communication), SPEC-012
(Prompt Profiles), SPEC-003 (UI).
**Constitution refs:** II.2 / II.14 (lifecycle tethering), II.10 (guardrail decision flow), IV.1 / IV.2 (ctx, `%w`),
IV.4 (no dead code), V.3 (no half-finished work).
**Follows:** [`multi-agent-delegation.md`](multi-agent-delegation.md) (Part C), [`agents-editor.md`](agents-editor.md).
**Origin:** the architecture review [`docs/audits/2026-10-22-architecture-review.md`](../../audits/2026-10-22-architecture-review.md)
(steps 4 and 5, §1 duplicates, §3 target shape) and the user's request (2026-10-23): fold the clean-up, refactoring,
merging of duplicates, engineering patterns and single source of truth into the same work as the agent commands.

## 1. Why these belong together

The agent commands need a way to start a turn that is the same for every channel. Today the web chat's turn lifecycle
(lane claim, cold-model wait, "model starting", the running-agent registry, cancel and replace) lives in the HTTP
handler, so a Telegram message cannot share it. Moving it out is the refactor the review already named (step 4), and
it is what makes a command that shapes a turn a small change instead of a second copy.

## 2. Where things stand (measured 2026-10-23, `main` at `1d27b130`)

- **Done:** R0 (tool categories from one table), Part A (`ResolveRunOptions`, `AgentBuilder` deleted, `models.NewRunID`,
  `run.OpenRecording`), Part B (delegation), the Agents editor.
- **Still the review's findings:**
  - `conversationService.Execute` takes **13 positional parameters** (`conversation_service.go:46`); the repo rule is ≤4.
  - The chat lifecycle is in `handlers/assistant_handlers.go` (672 lines); `assistant` has 31 non-test files.
  - `handlers.AssistantService` and `automation.LLMServiceProvider` are wide interfaces over one app context.
  - Memory review still calls `GetClientForModel` directly (no cold-model wait) and is its own copy of the run setup.
  - `models.TurnRun` and `models.AutomationRun` carry the same model, duration, tokens and warnings fields.
  - The test doubles (`MockClient`, `MockProvider`, `MockEngine`) are in root `_test.go` files, so no other package's
    tests can use them (blocks moving `conversation/` out of `assistant`).
  - Bucket A of `cross-cutting/post-implementation-cleanup.md` (13 dead symbols and files, dated 2026-08-01) is still
    open; each item is re-checked with a grep before deletion.
- **Slash commands:** `internal/core/commands` holds model-free commands only (`/remind`). A command runs, returns
  text, and never starts a turn. The web chat's menu lists whatever the server registers.
- **The connector path (C1, read 2026-10-23):**
  - A Telegram or webhook message already goes through the same `AssistantMessageHandler.RunWithCancel` as the web
    chat, with the same `AssistantMessage` payload (`webhook_handlers.go:handleAgentMessage`). That payload has
    `profile` and `use_helpers`, so a turn command only has to set two fields; no second path is needed.
  - **Every connector message gets a fresh conversation** (`webhookSessionKey` adds a timestamp), so a Telegram chat
    has no multi-turn history and nothing for an agent choice to stick to. Only the workspace's `assistant_profile`
    persists.
  - The web chat runs commands through `POST /admin/api/commands/run`: `handled: false` sends the text on as a normal
    message. A turn command extends that answer with the `TurnSpec`, and the composer sends the message with those
    run choices once.
  - `/run <automation>` is a second command path, a `switch` in the webhook handler outside the registry. It joins the
    registry in the commands PR, so there is one place that knows the commands.
  - Open for the commands PR: Telegram caps a message at 4096 characters and a research answer can be longer; whether
    the connector splits long replies is not checked yet.

## 3. Goal

1. A chat turn starts the same way from every channel, through one service with a request struct, not 13 parameters.
2. A command can start a turn under a chosen agent with helpers on, from the web chat, Telegram and the other
   connectors, with one implementation.
3. The remaining duplicates of the review are merged to one definition each, and dead code is deleted, with no
   unintended change in behaviour.

## 4. Design

### 4.1 Commands that shape a turn

`commands.Command` today returns text. Add one more kind, a **turn command**, whose result is a `TurnSpec`:

```go
type TurnSpec struct {
    Message    string // what the assistant is asked, without the "/research" prefix
    Agent      string // a profile id ("" keeps the conversation's own)
    UseHelpers bool
}
```

- `/research <question>` → `TurnSpec{Message: question, Agent: "research", UseHelpers: true}`. One message only: the
  conversation's own agent is unchanged afterwards.
- `/agent <id> <question>` → the same for any of the user's agents. An unknown id is answered with the known ones.
- `/agent <id>` alone is not offered (D3).
- The registry stays channel-neutral; each channel feeds a `TurnSpec` into the same chat service entry that a typed
  message uses. `Preview` (the live preview in the web chat's helper) says "Runs under Research with helpers" without
  starting anything.
- `/research` is offered only when the Research profile exists and helpers can run (`GET /admin/api/helpers`),
  otherwise it runs under Research without helpers and says so.

### 4.2 One chat service

- `ChatService.Run(ctx, ChatRequest)`: `ChatRequest` carries workspace, conversation, message, the run choices
  (`model`, `profile`, `use_helpers`, excluded tools), the channel and the sinks. `Execute`'s 13 parameters become this.
- The lane claim, cold-model wait with its "model starting" event, the running-agent registry and cancel / replace move
  from the handler into it. The handler decodes, calls, encodes.
- The service interfaces narrow: each consumer declares the methods it uses.

### 4.3 Single source of truth

- Memory review runs through the same run setup (gains the cold-model wait; the one deliberate behaviour change).
- One `RunRecord` (model, duration, tokens, cached, warnings, children) embedded in `TurnRun` and `AutomationRun`.
- One shared `assistanttest` package for the loop's test doubles (must not import `assistant`).
- Handler helpers from the codebase audit: one `writeError`, one path-variable parser, one service-error mapper.

## 5. Phases

**Two pull requests (D1):** PR 1 is the refactoring, C0 + C2 + C4 + C5 + C6, built in that order as separate commits
that each pass the tests; PR 2 is the commands, C3, after PR 1 is merged (D2: refactor first). Test first, `go test
./...`, the complexity check and a live smoke on vertex. No phase changes behaviour except where it says so.

| Phase | What | Accept |
|---|---|---|
| C0 | **Safety net and dead code — done.** `internal/testing/assistanttest` holds the loop's doubles (the root tests alias them). The chat turn lifecycle already has characterisation tests in `handlers/assistant_handlers_test.go` (replace, cancel, running and queued state, cold-model wait, use_helpers, errors); they move with the code in C2. Bucket A was already gone (a search finds none of the 13); two stale comments corrected | `go test ./...` green; no Bucket A symbol exists |
| C1 | **Read the connector path** (done, §2) | Findings written into this plan, no code |
| C2 | **Chat service.** `ChatRequest`; lifecycle out of the handler; `Execute` ≤4 parameters; `AssistantService` narrowed | Characterisation tests unchanged; handlers contain no lifecycle; complexity has no new violations |
| C3 | **Turn commands (PR 2).** `TurnSpec`, `/research`, `/agent`, the web helper preview, the connector path, `/run` into the registry, long-reply handling; docs in SPEC-009 and SPEC-003 | Unit tests per command; a live Telegram-style message on vertex runs Research with helpers |
| C4 | **Merge duplicates.** Memory review on the shared setup; `RunRecord`; handler helpers; remaining D6 / D7 copies | Tests; the one deliberate change (memory review waits for a cold model) has its own test |
| C5 | **Package moves.** `conversation/` out of `assistant`, `ResolveRunOptions` into `run/`, heartbeat out of `executor.go` | `go build` proves no cycle; root `assistant` no longer imports `conversation` |
| C6 | **Close out.** SPECs, `architecture.md`, plan statuses, tracker | `./scripts/check-agent-harness.sh` |

Order: C0 → C2 → C4 → C5 → C6 (PR 1), then C3 (PR 2). C4 and C5 can swap.

## 6. Risks

- C2 and C5 touch the busiest path in the product. The characterisation tests in C0 are what make them safe; if they
  cannot be written first, the phase does not start.
- A command that starts a turn from a connector can be abused as a way around a profile's limits only if profiles are
  not checked at the boundary: the agent id is validated against the catalog the same way `profile` is today.
- Merge conflicts with in-flight branches: check open branches before C2 and C5.
- The loop files (`session.go`, `tool_exec.go`, `stream.go`) are not in scope; they need their own review.

## 7. Decisions

- **D1 (decided):** the refactoring is one PR (C0, C2, C4, C5, C6); the commands are their own PR (C3).
- **D2 (decided):** refactor first, then the commands.
- **D3 (decided, 2026-10-23): `/agent <id>` on its own is not offered.** A Telegram message always starts a fresh
  conversation, so there is nothing for the choice to stick to. Only `/research <q>` and `/agent <id> <q>`, one message
  each. To run Research for every Telegram message, set the workspace's default agent.

## Remaining Work

All phases.
