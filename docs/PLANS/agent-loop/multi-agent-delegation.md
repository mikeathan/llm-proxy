# Multi-Agent Delegation — one run pipeline, then cloud sub-agents

**Status:** active (2026-10-22). R0, Part A and Part B (B0–B6, B8) built; B7, the live eval, is open.
**Related Specs:** SPEC-001 (Agent Loop), SPEC-007 (Automation Dispatcher, §V run scheduler), SPEC-012 (Prompt
Profiles), SPEC-006 (Guardrails), SPEC-004 (Memory).
**Constitution refs:** II.2 / II.14 (lifecycle tethering), II.10 (guardrail decision flow; automations never wait),
II.13 (prompts only in `templates.go`), IV.1 / IV.2 (ctx, `%w`), IV.4 (no dead code), V.3 (no half-finished work).
**Origin:** research [`docs/audits/2026-10-22-multi-agent-research.md`](../../audits/2026-10-22-multi-agent-research.md)
(decisions §9) and [`docs/audits/2026-10-22-architecture-review.md`](../../audits/2026-10-22-architecture-review.md).
**User decisions:** R0 (fix the category drift now) and R1 = (b): Part A steps A0–A2, then Part B, then Part C.

## Goal

1. **Part A.** Chat and automation build a run in one way.
2. **Part B.** A delegate tool starts a cloud sub-agent through that same path. It adds one small package, not edits
   across five.

Behaviour does not change in Part A, except for the deliberate items listed per step.

## R0 — tool category drift (done 2026-10-22)

- Categories now come from one table: `models.CategoryOf` (reverse of `toolsByCategory`).
- The hand-written `toolCategory` switch in `tool_exec.go` is removed.
- The registry derives each manifest key from the tool name.
- The guardrail `planRemember` switch uses the category constants.
- **Deliberate change:** `edit_file_block` is `filesystem`. It gets the filesystem timeout, its approval shows
  `filesystem`, and "Allow and remember" saves its path. Before, nothing was saved for it.
- `security_guardrails`, the memory, reminders and system tools now report their real category instead of
  `general`. `general` remains for tools outside the table (MCP).
- Tests:
  - `models.TestCategoryOf`
  - `TestToolCtxWithTimeout_EditFileBlockUsesFilesystemTimeout`
  - `TestPreviewRemember_EveryFileToolRemembersItsPath`
  - The old `TestToolCategory_AllCases` encoded the drift and was removed.

## Part A — one way to build a run

### A0. Characterisation tests

Before anything moves, record the `AgentOptions` each path resolves today. Pointers and functions are excluded; only
the presence of the memory store, observer and decision handler is recorded.

- **Chat:** the options its builder produces for a matrix of model config, profile (with and without tools / memory /
  steps), workspace memory override, global defaults and excluded tools.
- **Automation:** `buildAgentOptions` for the same matrix plus allowed tools, loop override, memory mode, journal and
  network grant.
- Known, deliberate differences stay visible in the tests:
  - Chat always attaches the memory store, which `memory_search` needs; an automation attaches it only when memory is
    on.
  - Chat has a deny list; automation has an allow list.
  - Only automation resolves a network scope.
  - Only automation sets an explicit `MaxSteps = DefaultMaxSteps`.

**Accept:** tests pass on the current code, and `go test ./internal/core/assistant/ ./internal/core/automation/`
passes.

### A1. One resolver

- `assistant.RunOverrides`: the explicit choices for one run (model, profile, loop, allowed / excluded tools, memory
  mode, network scope).
- `assistant.ResolveRunOptions(deps, kind, overrides) → AgentOptions`: one precedence chain, explicit → profile →
  model config → global default.
- It replaces:
  - chat: `AgentBuilder` and `hotMemoryEnabled`;
  - automation: `buildAgentOptions` and `memoryActive`. The prompt's memory block reads the same resolved value.
- It lives in `assistant`, not `run/`, because chat's `conversationService` is still in `assistant` and `run` would
  import it. It moves to `run/` with A5.

**Accept:** the A0 tests are unchanged and green; `AgentBuilder` is deleted; complexity has no new violations.

### A2. Shared run helpers

- **Run ID.** One generator, `models.NewRunID`, replacing `assistant.GenerateRunID`, `automation.generateRunID` and
  the inline `run_%d` in `execution.go`. *Deliberate:* automation run IDs change format; they are opaque.
- **Recording.** One `OpenRunRecording` (run folder + event sink + attach the recording client) used by the chat
  handler and the executor, in a small `internal/core/run` package.
- **Cold-model wait.** Memory review uses `proxy.WaitForClient` like chat. *Deliberate:* a cold model now waits
  instead of failing.

**Accept:**
- `go build ./... && go test ./...` passes.
- One live chat and one manual automation run on the dev instance (an isolated `--data` folder), each showing a run
  record and a recording.

## Part B — cloud sub-agents (v1: web research)

Decisions from the research report §9:
- Children never run on the parent's local model.
- A separate pool (global 3, 3 per turn).
- Depth 1; at most 15 steps per child; a returned summary of at most ~1.5k tokens.
- Web search and page fetch only; no file access; no approvals.
- The profile lists what it may delegate to, and the parent model decides when; `general` does not delegate.
- Stuck detection by lack of progress, not a fixed time limit.

**Design gaps found after Part A, to close before B3 / B4:**

- **(a) Child events must not reach the parent's session.** The chat observer (`buildObserver`) rebuilds and saves
  the session on every tool result or message, and counts a "Step N" on every tool call. A child's events therefore
  travel as one nested wrapper event, which `buildPartialHistory`, the step counter and the frontend all treat as
  nested.
- **(b) Children get no approval handler.** A nil handler fails at once, per II.10, which is D6. `RunSpec` gains a
  sub-agent kind. `TestResolveRunOptions`'s "every run gets a decision handler" assertion changes on purpose for that
  kind.
- **(c) Each child has its own run ID, usage tracker and `CloseRun`.** Its tokens go into `TurnRun.Subagents`, not the
  parent's totals, so local and cloud tokens are never mixed.
- **Depth 1 in code:** a child gets no `SubRunner`.
- **No web tools, no delegate:** delegate is not offered when a child would have no web tools, for example when no
  search provider is set.
- **Order:** B-first is one end-to-end test through `conversationService` with `MockClient` and a fake `SubRunner`. It
  proves:
  1. the parent gets the summary;
  2. the saved session holds only the parent's messages and steps;
  3. the child had no approval handler and no delegate tool.

  Then B0, B1 and B5.

**Inputs needed from the user:**

- **The sub-agent model.** One setting, which must be a cloud model; when it is unset, delegate is not offered. An
  embedded built-in profile cannot name a model from the user's catalogue.
- **B7 credentials and spend.** Either keys are added to a scratch instance, or the eval runs against the dev backend.

| Step | What | Accept |
|---|---|---|
| B0 | **Capability table:** `models` gains per-tool access (`read` / `write` / `external` / `state`), next to the category table | Table test: every built-in tool has one access value |
| B1 | **Profile fields:** `model`, `delegate_summary`, `delegates: [ids]`; validated by the parser; the schema is published by `GET /admin/api/profiles` | Parser table tests; unknown delegate IDs warn |
| B2 | **`SubRunner` port** in `assistant` + the `delegate` tool (manifest, prompt text in `templates.go`); offered only when the run's profile lists delegates and a port is set | Registry and tool tests with a fake port |
| B3 | **`internal/core/delegation`:** the pool (bounded, ctx-aware, busy result), the child plan from the helper profile (cloud model required, read-only web tools, no memory writes, connectors or reminders), and a child run inside the parent's context through `ResolveRunOptions` | Unit tests: pool limits, refuses a local model, tool scope, cancellation follows the parent |
| B4 | **Events + record:** child events carry `parent_run_id` and the helper ID on the parent's channel; `TurnRun.Subagents` records model, duration, tokens and status | Event test; record JSON test |
| B5 | **Settings + wiring:** `scheduler.cloud_subagent_concurrency` (default 3); bootstrap injects the port; the `research` profile lists `web-researcher` (a new built-in profile) | `go test ./...`; settings round-trip |
| B6 | **UI:** sub-agent steps nested and collapsed in the chat | `npm test`, `npm run build`, visual check |
| B7 | **Eval:** 10–15 research prompts, local Qwen 16k parent, with and without delegation, N repeats; LLM-judge quality, peak parent context, overflow and loop errors, wall time, tokens | Results in `docs/audits/`; ship only if quality ≥ baseline and overflows go down |
| B8 | **Docs:** SPEC-001 (delegation section), SPEC-012 (profile fields), SPEC-007 §V (sub-agent pool), architecture pitfalls | Harness check |

## Part C — later

- A3: chat lifecycle (lane claim, cold-model wait, cancel / replace) out of the HTTP handler; narrow the service
  interfaces.
- A5: the test-doubles package (Step 0 of `cross-cutting/assistant-conversation-package.md`), `conversation/` out of
  `assistant`, `ResolveRunOptions` into `run/`, heartbeat out of `executor.go`, plus cleanup Bucket A.
- Helpers with `on_finish`; the reviewer type; `files` / `cloud_files`.
- The Settings → Agents editor with schema-driven hints (built-ins read-only, "Duplicate to edit").
- Pipelines with Event-Driven Triggers.

## Part A results (2026-10-22)

- **A0.** `TestChatAgentOptions_Characterisation` (`conversation_service_test.go`) and
  `TestAutomationAgentOptions_Characterisation` (`executor_test.go`) record every resolved field. Chat got the seam
  `agentOptions(chatAgentRun)`, replacing the 11-parameter `buildAgent`. Both tests passed unchanged through A1 and A2.
- **A1.** `assistant/run_options.go`: `RunOptionDeps`, `RunOverrides`, `RunSpec`, `ResolveRunOptions` and
  `HotMemoryActive`.
  - Removed: `AgentBuilder` (`agent_builder.go`), chat's `hotMemoryEnabled` and six forwarding methods, automation's
    `memoryActive` and its hand-built options.
  - `ConversationDeps` and `LLMServiceProvider` embed `RunOptionDeps`.
  - `EnableHotMemory` now means "effective and a store exists" for both kinds. The agent already read it that way
    (`hot_memory.go`, `session.go`).
- **A2.**
  - `models.NewRunID` replaces three generators.
  - `run.OpenRecording` (`internal/core/run/recording.go`) replaces chat's `setupRecording` and automation's
    `setupRunDir` / `createRunDir` / `attachRecording`. *Deliberate:* the client is attached even when the event file
    cannot be created.
  - Memory review waits for a cold model (`TestReviewMemories_WaitsForAColdModel`).
- **Verified.** `go build`, `go vet` and `go test ./...` pass; complexity has no new violations. On the isolated
  instance (port 4002, own `--data`, vertex `Qwen3.6 35B A3B`), a chat turn listed files and recorded its run, and a
  manual automation produced a hex run ID, recording, events, final report and saved report.

## Part B results (2026-10-22, branch `fix-recordings-list-labels`, with the recordings-list fix)

- **B0.** No new table: the child's tools are an explicit allow list (`internet_search`, `fetch_url`).
  `toolpolicy.EffectFor` calls `read_file` and the LAN scanner read-only too, so "read-only" is not a safe scope.
- **B1.**
  - `delegates` on profiles (validated; the `delegation` category cannot be named in `tools`).
  - `web-researcher` built-in; Research delegates to it.
  - The full schema endpoint for the editor stays in Part C. `GET /admin/api/profiles` already returns `delegates`.
- **B2.**
  - `assistant/subrun.go`: the port.
  - `assistant/delegate.go`: the agent owns the tool (`delegatingProvider` adds the schema; `executeTool` intercepts).
    The tool has 1–3 parallel tasks per call and 6 per run; results are text, never parent errors; summaries are cut to
    the parent's cap.
  - The tool has no per-tool timeout.
- **B3.** `internal/core/delegation`:
  - `Runner` refuses unless the lane classifier says cloud.
  - The child has a fresh run ID and tracker, is unattended, and stays inside the parent's network scope.
  - `pool.go` is live-sized, with a 2 min wait, then busy.
  - The resolver's `profile.KindSubagent` removes the approval handler, memory and runner.
- **B4.** `EventSubagent` carries coarse `started` / `tool_call` / `tool_result` / `done` events, serialised across
  children. `TurnRun.Subagents` holds the records. Child tokens never mix with the parent's.
- **B5.** The settings are `agent.subagent_model` (empty = off) and `agent.subagent_concurrency` (default 3, 1–10,
  validated, 400). They sit in the existing `agent:` block, not under `scheduler`, so no new section plumbing was
  needed. Bootstrap sets `subRunner` after copying the services, so the runner's own copy has none (depth 1).
- **B6.**
  - Settings → Runs: Helper model (cloud models only, Off by default) and Helpers at once.
  - Chat: helper lines under the delegate step, a "N helpers" footer with per-helper tokens in the tooltip, and the
    "Asked helpers" step wording.
- **B8.** SPEC-001 1.12 §15, SPEC-012 1.2, SPEC-007 1.19 §V, architecture map + pitfall 60.
- **Verified.**
  - `go build`, `go vet` and `go test ./...` pass; `-race` passes on assistant and delegation; complexity has no new
    violations.
  - `npm test` (1823), `npm run lint`, `npm run build` and `npm run test:visual` (24) pass.
- **Not yet verified live.** No real cloud model has run a delegate call; that is B7.

## Remaining Work

- **B7:** a live smoke on the dev backend (the user named it), then the eval: research prompts with and without
  delegation, LLM-judge quality, peak parent context, wall time, tokens. It needs `agent.subagent_model` set to a cloud
  model and a search key there.
- **Part C.**
