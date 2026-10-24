# Run pipeline cleanup and agent commands — Part C of multi-agent delegation

**Status:** complete (2026-10-24). D1–D3 decided. Every phase is built, merged and checked live on vertex: the
refactor C0–C6 (#129, #130, #131), the commands C3 (#132), the history fix it exposed (#133) and the wrap-up (#134).
One live check is left, postponed by the user: the connector path on a real Telegram chat (Remaining Work).
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
  - `/run <automation>` is a second command path, a `switch` in the webhook handler outside the registry. It was to
    join the registry in the commands PR; deferred there, done in the wrap-up (C3 results).
  - Long replies are already split: Telegram `Send` cuts a message over 4000 UTF-16 units on line boundaries
    (`tools/notifiers/telegram.go:51,60`, SPEC-009 1.2), so a research answer longer than Telegram's 4096 cap arrives
    as several messages. Checked 2026-10-23 (C6).

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
| C2 | **Chat service — done.** `core/chat` (`chat.Service`, `chat.Request`) holds the turn lifecycle; the handler and the webhook call it; `Execute` takes one `ExecuteRequest`; the chat handlers depend on a 11-method `AssistantHandlerDeps`, not the 26-method `AssistantService` (the automation side still uses that one; C4). The lifecycle tests moved with the code (22 tests) | Characterisation tests unchanged; handlers contain no lifecycle; complexity has no new violations |
| C3 | **Turn commands (PR 2) — built, see C3 results.** `TurnSpec`, `/research`, `/agent`, the web helper preview, the connector path, long-reply handling (already done by the connector); docs in SPEC-009, SPEC-012 and SPEC-003. `/run` into the registry deferred | Unit tests per command; a live Telegram-style message on vertex runs Research with helpers |
| C4 | **Merge duplicates — done, see C4 results.** Memory review on the shared admission; the others were looked at and left | Tests green; complexity has no new violations |
| C5 | **Package moves — done, see C5 results.** The conversation service is in `chat` (#129); the resolver is `run.ResolveOptions`; the heartbeat's quiet signal is in `automation/heartbeat.go` | `go build` proves no cycle; root `assistant` no longer imports `conversation` |
| C6 | **Close out — done, see C6 results.** SPECs, `architecture.md`, skills, plan statuses, tracker | `./scripts/check-agent-harness.sh` |

Order: C0 → C2 → C4 → C5 → C6 (PR 1), then C3 (PR 2). C4 and C5 can swap.

### C2 results (2026-10-23)

- The package is `core/chat`, not `conversation/` as the review sketched: the conversation service itself moves in
  there in C5.
- **Found while moving it:** the connector's agent answer was read with `result.(map[string]any)`, but the turn's result
  is the typed `assistant.ExecuteResult`, so by reading the code a Telegram agent turn's reply was never sent back
  (nothing tested that path). The webhook now takes `result.Reply` through a narrow `AgentTurns` interface, with tests
  that a reply goes back through the connector and that an empty or failed turn sends nothing. Not checked on a real
  Telegram chat.
- Turn errors are plain errors now: the handler's `Status` was never used (the run is detached after the 202).
- Verified: `go test ./...`, the complexity check (no new violations), the ported lifecycle tests.

### C4 results (2026-10-23)

- **Merged:** the memory review now takes the same admission as a turn through `chat.Service.Lease` (an interactive
  lane claim, then the cold-model wait), instead of carrying its own copy in the handler; the chat handlers' interface
  shrank to 8 methods (no lane, no client provider).
- **Looked at and left alone, on purpose:**
  - `TurnRun` and `AutomationRun` share three fields (model, duration, warnings) but their JSON differs (`omitempty`
    on one, none on the other, and `AutomationRun` is stored in `state.json`); an embedded `RunRecord` would change one
    of the two shapes for three fields.
  - The chat observer and the automation observer share only "publish and write to the recording" (six lines); the chat
    one also checkpoints the session, so a common observer would be a parameterised wrapper around two different jobs.
  - The handler helper split from the codebase audit (`writeJSONError`, `requirePathParams`) already exists.
- The automation side's wide interfaces (`automation.LLMServiceProvider`, `handlers.AssistantService`) are unchanged.

### C5 results (2026-10-23)

- **Moved to `chat`:** `conversation.go` (`ExecuteRequest`, `ExecuteResult`, `ConversationService`),
  `conversation_service.go`, `conversation_helpers.go` (history truncation, cancel filtering, partial history, local
  time) and `memory_capture.go`, with their tests (40 test functions; the set of test names across `assistant` and
  `chat` is unchanged). `chat` imports `assistant`; `assistant` does not import `chat`.
- **Stayed in `assistant`, split out of the moved files:** `event_ports.go` (`EventPublisher`, `EventRecorder`),
  `system_prompt.go` (`LoadAgentsFile`, `BuildSystemPrompt`, the AGENTS.md cache; automation uses them too) and
  `tool_history.go` (`NeutralizeStaleToolFailures`; it reads the guardrail engine), with their tests.
- **Exported for `chat`:** `assistant.HistoryChars` and `assistant.IsAgentControlMessage`, one definition each.
- Far smaller than the ~50 symbols estimated: only those two helpers had to be exported; everything else
  was already exported and is qualified.
- Test support: the temporary memory store helper is `assistanttest.NewMemoryStoreAndDB` (both packages use it).
  `TestConversationSystemMessage` uses its own workspace id instead of clearing `assistant`'s AGENTS.md cache.
- The complexity baseline keys for `TruncateHistory` and `buildPartialHistory` follow the file to `chat/` (same
  functions, unchanged).
- Verified: `go build ./...`, `go vet ./...`, `go test ./...`, the complexity check.

### C5 second part (2026-10-23, after #129)

- **The resolver is `run.ResolveOptions`** (`run/options.go`) with `run.Spec`, `run.Overrides`, `run.OptionDeps` and
  `run.HotMemoryActive`; the `assistant` copies are deleted, not aliased. `run` imports `assistant`; neither
  `assistant` nor `delegation` imports `run` (`go list -deps`). Callers: `chat`, `automation/executor.go`,
  `delegation/runner.go`. In `chat.captureMemories` the `run` parameter (a `*models.TurnRun`) is now `turnRun`, which
  shadowed the package.
- Tests follow: `run/options_test.go` (`TestResolveOptions`, `TestResolveOptions_SubagentRules`, renamed from
  `TestResolveRunOptions*`). The adaptive-result-cap test splits in two: the setting reaching the options
  (`run.TestResolveOptions_AdaptiveResultCap`) and the options reaching the agent (`assistant.
  TestNewAgent_AdaptiveResultCapReachesTheAgent`, which reads the agent's unexported config), so one test name is new.
- **Heartbeat:** the Pulse Logic block (`HEARTBEAT_OK`, `isQuietHeartbeat`, `isQuietResponse`, `heartbeatResultOf`,
  `ApplyPulseLogic`) and `TestApplyPulseLogic` move unchanged from `executor.go` / `executor_test.go` to `heartbeat.go`
  / `heartbeat_test.go`. `executor.go` is 609 lines (735 at the review).

### Live smoke on vertex (2026-10-23, build `6e9c7f0`, throwaway workspaces, deleted afterwards)

- **Passed:** a turn and a follow-up in the same conversation (the history is replayed); replace (a new message
  cancels the running one, which is kept with `cancelled_indices`, and the new one answers); cancel (`canceled: true`,
  idle at once, the user's message kept and marked); a new turn in the cancelled conversation; the memory review
  (C4's admission) answered in 4 s. Model `deepseek-v4.1-flash`; turns 1–2 also on `nemotron-3.5-super-vl:free`.
- **Not checked:** the connector path, including C2's fix for the Telegram reply that was never sent. vertex is local
  HTTP only, and the inbound webhook cannot reach it; checked once a reachable webhook exists.
- **Seen, not caused by this work:** the free model returned 502s, then held a stream open for four minutes with no
  visible output until a new message replaced the turn. *Corrected 2026-10-23:* a stream is already bounded. The
  response headers had arrived (the cancel was logged by the body-reading loop), and after that a cloud stream is cut 3
  minutes after its request when no token came (`streamFirstTokenTimeout`, checked every 30 s), 5 minutes after its
  first token with no tool call (`streamMaxDuration`; reasoning deltas count as tokens) and after 5 minutes without a
  chunk (`proxy.StreamChunkTimeout`). Four minutes is inside those limits, so the replacement came first; no fix was
  made. A gap nobody has seen: before the headers arrive a cloud request may wait 10 minutes (`ResponseHeaderTimeout`,
  sized for long-thinking models).

### Live smoke of #130 on vertex (2026-10-23, `v0.11.9`, throwaway workspaces, deleted afterwards)

- **Passed:** a manual automation run (the executor through `run.ResolveOptions`) wrote its report in 3.4 s; a
  Research turn with `use_helpers` (the delegation runner through `run.ResolveOptions`) made one `delegate_task` call,
  three `web-researcher` children finished on `deepseek-v4.1-flash`, and the answer cited its sources (95 s); replace,
  cancel and a new turn after a cancel passed again.

### C6 results (2026-10-23)

- The docs were swept for the old locations: SPEC-001, SPEC-004 (memory) and SPEC-007 (dispatcher) name
  `run.ResolveOptions` and `heartbeat.go`; `architecture.md` maps `core/chat` and `core/run`; the
  `lifecycle-events`, `event-streaming-patterns`, `agent-loop` and `assistant-ui-chat` skills point at `core/chat`
  (`last_reviewed` bumped). SPEC-009 needed no change: C2's connector fix makes the code do what it already says.
- **Corrected while sweeping:** `event-streaming-patterns` said a chat turn's events come back in the HTTP body; the
  message endpoint answers 202 and the turn runs detached, so the events reach the client over SSE only. The
  handler's own comment still named `RunWithCancel`; it names `chat.Service.Run`.
- `multi-agent-delegation.md` Part C: A3 and A5 are done here.
- Found: `ExecuteResult.Events` was filled on every turn but no caller read it (the web handler answers 202, the
  webhook reads `Reply`). Removed in the wrap-up, with `FormatCollectedEvents` and the frontend's never-sent `events`
  on the chat response type.

### C3 results (2026-10-23)

- **Commands** (`core/commands/agent.go`): `/research <question>` and `/agent <agent> <question>` are **turn commands**:
  `Command.Turn` returns a `TurnStart` (the `TurnSpec`, a note, the preview line), and `Registry.Execute` answers with
  `Reply.Turn`. They read the profile catalog and a "why helpers cannot run" function (`app/commands.go`, the same words
  as the chat's settings), so `commands` imports neither `chat` nor `assistant` for them. `/agent` accepts every
  catalog id, the rule the chat's picker and `profile` use; `/agent <agent>` alone is refused (D3).
- **One-message agent** (`chat.Request.Agent` → `ExecuteRequest.Agent` → `resolveProfile`): it outranks `profile` and
  the session's profile for that turn and is never stored, which is what "one message only" needs — `profile` is
  stored on the session. Checked at the boundary like `profile` (`prepareRequest`, 400).
- **Channels:** `POST /admin/api/commands/run` answers with `turn`, and the composer sends `turn.message` with the
  agent and helpers (a queued message takes the same path). The webhook routes an inbound message through an `inbound`
  struct, so a turn command's turn runs like any message (`startAgentTurn`), after its note.
- **`/run` into the registry — deferred in C3, done in the wrap-up** (user, 2026-10-23: "do everything else"). It is
  `commands.Run` (`run.go`): it triggers the automation in the message's workspace through an `AutomationStarter`
  port (`app/commands.go` adapts `Dispatcher.Trigger`) and names it in `Reply.Automation`; the webhook sends the start
  line, then follows the automation's event stream and sends its result (`followAutomation`, the old monitor without
  its trigger, split under the complexity limit; `handleAutomation` and its baseline key are gone). The web chat sends
  its workspace with every command, so `/run` works there too. Behaviour changes: an unknown automation is refused
  with the reason (it was logged and the chat still got "Running…"), a bare `/run` gets its usage (it went to the
  agent), and the reply is sent as plain text. The connector path is unit-tested only (no connector token yet).
  **Checked live** (vertex `v0.11.13`, 2026-10-24, web path): the command list includes `run`; the preview reads "Runs
  'smoke' now" and starts nothing; `/run` alone, an unknown automation and a request with no workspace are refused
  with the reason and usage; `/run smoke` answered "Running 'smoke'..." with `automation: smoke`, and the run finished
  in 4 s with its report; a plain chat turn afterwards answered as usual. The
  run record does not say which agent answered (`TurnRun` has no profile field); not added. So **Retry** on a turn
  command's answer reruns the stored question under the conversation's own agent, without helpers.
- **Corrected while documenting:** `api-reference.md` still showed the message endpoint answering `200` with the
  reply and events; it answers `202 {status, conversation_id}` and the turn runs detached.
- Verified: `go test ./...`, the complexity check, `npm test`, `npm run build`.
- **Live on vertex (2026-10-23, `v0.11.11`):** `GET /admin/api/commands` lists `research` and `agent`; the preview reads
  "Runs under Research with helpers" (and "ask a question after the agent (Research)" for `/agent research`);
  `/commands/run` returns `turn: {message, agent: research, use_helpers: true}`, refuses `/research` alone and an unknown
  agent with the known ids; a message with `agent: ghost` is a 400. In a three-turn chat (Builder, then the command's
  turn, then plain) the Research turn delegated to helpers (`delegate_task`, two children in the process log), the
  stored profile stayed `builder` and the third turn ran under Builder's prompt. The connector path is not checked (no
  connector token given yet).
- **Found by that check, not caused by C3:** the chat service cut the stored history to 12 KB at the start of every
  turn (`initSession`, since #31), so a chat past 12 KB kept only its first question and newest exchange; the check's
  second question and first answer were dropped by the third turn. Fixed separately (SPEC-001 1.17, pitfall 63): the
  stored-history ceiling is 256 KB on every path, and `shrinkHistory` may cut the head's messages as a last resort
  (decided with the user, 2026-10-23), merged as #133. **Checked live** (`v0.11.12`): a four-turn chat (two ~800-word
  essays, a `/research` turn with helpers, then "list the first word of each previous answer") on `deepseek-v4.1-flash`
  and on the local 16k `Qwen3.6 35B A3B`. On both, every turn answered its own question, all four questions stayed
  stored past 12 KB (19.8 KB and 17.8 KB), the last turn recalled "ALPHA, BRAVO, CHARLIE", the delegated turn's two
  `subagents` records survived the next turn, and there were no overflows or errors.

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

- **The only item left (a live check, postponed by the user):** the connector path on a real Telegram chat (C2's reply fix, `/research` and `/run` from a
  connector). Telegram cannot call
  vertex (no public HTTPS), so the test posts a Telegram-shaped update itself, on vertex or from the LAN over plain HTTP
  (`POST http://localhost:4001/api/v1/webhooks/<connector>`, header `X-Telegram-Bot-Api-Secret-Token` when the
  connector has a webhook secret, body `{"message":{"text":"…","chat":{"id":1}}}`); the answer goes out to Telegram
  over HTTPS as reminders do. Send "Reply with exactly TELEGRAM-OK", then `/research <question>`, then `/run <automation>`; each answers
  `{"status":"ok"}` and the replies should arrive in the connector's chat. The connector needs a workspace set.
  Postponed by the user (2026-10-23).
