# Architecture Review Before Multi-Agent Work (2026-10-22)

**Status:** reference. Review only; no code was changed. It looks at how the backend is structured before adding
sub-agents (see [`2026-10-22-multi-agent-research.md`](2026-10-22-multi-agent-research.md)).

**Request (user's words):** "look the solution from the top and see what can be redesigned, refactored, re-implemented
for single source of truth … how to reduce code without losing functionality, how to have better more clean package
structure."

**Scope.**
- Reviewed: how runs are started and set up (chat, automation, memory review), how a run's settings are resolved,
  tool categories, and the event shape.
- Not reviewed: the loop internals (`session.go`, `stream.go`, `tool_exec.go`), the `tools` implementations beyond
  categories, and the frontend. The editor will need its own frontend pass.

**Labels:**
- **[code]**: opened in this review.
- **[doc]**: read in a plan.
- **[assumption]**: not verified.

---

## 1. The finding: one job, built several ways

A "run" is: pick the model, profile, tools, memory and loop; get a client (waiting for a cold model); create a run ID,
usage tracker and recording; send events out; run the agent; record the outcome. Each place that starts a run builds
this itself today, and sub-agents would add another.

| Starts a run | Where | Admission | Settings resolved by | Client wait |
|---|---|---|---|---|
| Chat | `handlers/assistant_handlers.go:406` → `assistant/conversation_service.go:55` | `ClaimInteractive`, in the HTTP handler (`:432`) | `resolveModelConfig`, `resolveProfile`, `hotMemoryEnabled`, `AgentBuilder` (`conversation_service.go:180-308`) | `WaitForClient` in the handler (`:315`), with a "model starting" event |
| Automation (and heartbeat) | `automation/admission.go:51` → `automation/executor.go:176` | `Submit` | inline `Resolve`, `memoryActive`, `allowedToolsFor`, `buildAgentOptions` (`executor.go:393-445, 665`) | `WaitForClient` in the executor (`:273`), without the event |
| Memory review | `handlers/assistant_memory_review_handlers.go:64` | `ClaimInteractive`, in the HTTP handler | its own | **none**: a direct `GetClientForModel`, so a cold model fails, where chat and automation wait [code; failure mode assumed] |
| *Planned:* sub-agents, `after` pipelines, standing-goal ticks, follow-ups (`action: run`), chat → automation | — | — | — | — |

**Duplicated pieces [code]:**

| # | What | Copies | Note |
|---|---|---|---|
| D1 | Run ID | `assistant.GenerateRunID` (hex, `conversation_helpers.go:168`); `automation.generateRunID` (`run_<nanos>`, `executor.go:655`); inline `run_%d` in `recordPrepareFailure` (`automation/execution.go:171`) | Three generators, two formats. The IDs are opaque: nothing parses them (grep), only `IdChip` shortens them for display. One generator is safe. (`run_err_%d` at `assistant_handlers.go:378` is an event ID, not a run ID.) |
| D2 | Run folder + recording | `handlers.setupRecording` (`assistant_handlers.go:276`) and `automation.setupRunDir`/`attachRecording` (`executor.go:311, 344`) | The same `SetDirForRun`/`SetDir` type switch, twice. |
| D3 | Cold-model wait | handler `getLLMClient` (`:315`) and executor `getLLMClient` (`:273`) | Same timeout constants. Only chat publishes "model starting"; that becomes a parameter, not a reason to keep two copies. |
| D4 | Hot-memory resolution | `hotMemoryEnabled` (`conversation_service.go:282`) and `memoryActive` (`executor.go:665`) | The same precedence (override → profile → global default) written twice. |
| D5 | Agent construction | `AgentBuilder` (chat, 17 `With…` methods) and an `AgentOptions` literal (automation) | Two ways to build one struct of 36 fields. |
| D6 | Event fan-out | `buildObserver` (`conversation_service.go:239`) and an inline observer (`executor.go:402`) | Both publish, write a file sink, and record. |
| D7 | Outcome handling | `handleCancelResult`/`handleErrorResult`/`handleSuccessResult` (`conversation_service.go:311-404`) and `handleAgentError`/`handleAgentSuccess` (`executor.go:467, 491`) | The shared part is usage, warnings and error text; the persistence differs (session vs `AgentState`). |
| D8 | Run record | `models.TurnRun` (`llm_messages.go:59`) and `models.AutomationRun` (`workspace.go:169`) | Overlap: model, duration, tokens, warnings. |
| D9 | Tool restriction | chat `ExcludeTools` (a deny list from the UI) and automation `AllowedTools` (an allow list) | Both are needed; they should be **one** scope type that holds both, not two code paths. |
| D10 | Per-run choices | chat passes model and profile as context values (`GetModelOverride`, `GetProfileRequest`); automation passes explicit `ExecuteRequest` fields | One explicit `Overrides` struct for every kind of run. |

**Already unified, and to keep:** the system prompt. `BuildSystemPrompt` (`conversation_helpers.go:77`) is shared by
chat and automation; chat only adds its conversation facts. The run lanes (`runlane`) and the profile catalog are
also single sources.

**Repo rules this breaks [code + AGENTS.md]:**
- **AGENTS.md → Centralization** ("lifecycle wiring … in one discoverable place"). Chat's lane claim, cold-model wait,
  "model starting" events, running-agent registry and cancel/replace live in the HTTP layer
  (`assistant_handlers.go:87-450`). Automation's lifecycle lives in `automation`.
- **The ≤4-parameter rule**, which `executor.go:447-450` itself quotes. `conversationService.Execute` takes 13
  positional parameters. The three `handle*Result` functions take 8–9.
- **Wide service interfaces.** `handlers.AssistantService` has 26 methods, `automation.LLMServiceProvider` 22 and
  `assistant.ConversationDeps` 13. All three are implemented by the app context. Each consumer depends on far more
  than it uses.

### 1.1 Package and file sizes (measured, source lines without tests)

| Package / file | Lines | Shrunk by the target shape? |
|---|---|---|
| `core/assistant` (27 files) | 9,518 | Partly. The run setup leaves (conversation files, `AgentBuilder`, options plumbing). The loop (`session.go` 1,602, `tool_exec.go` 1,403, `stream.go` 1,381) is **not** touched; it needs its own review. |
| `transport/http/handlers` (25 files) | 7,354 | Partly. `assistant_handlers.go` (696) loses the lifecycle. `dispatcher_handlers.go` (991) and `model_handlers.go` (943) are not addressed beyond audit items. |
| `core/tools` (17 files) | 4,835 | Little. Categories move to the capability table; the implementations stay. |
| `models` (26 files) | 3,337 | No. It gains the capability table and `RunRecord`, and `TurnRun`/`AutomationRun` lose duplicated fields. |
| `core/automation` (17 files) | 3,120 | Yes. `executor.go` (735) loses the run setup and the heartbeat logic. |
| `core/llm/manager.go` | 955 | No. Not reviewed. |

The target fixes **duplication in how runs are set up**. It does not make the big loop files smaller; that is a
separate, later review.

## 2. A single-source-of-truth bug in tool categories

Tool categories are defined in **three** places:
1. `models.toolsByCategory` (`models/tools.go:113`), documented as "the one place".
2. The string literal passed to `registerTool(r, "filesystem", …)` (`assistant/registry.go:540-639`).
3. A `switch` in `toolCategory` (`assistant/tool_exec.go:908`).

The `switch` **lacks `edit_file_block`**, which `models` puts in `filesystem`. So for `edit_file_block`:
- The per-tool timeout uses `ToolTimeout`, not `FilesystemToolTimeout` (`tool_exec.go:931`).
- The approval prompt shows category `general` (`:739`).
- An "always allow" is saved under `general` (`:763`).

The mismatch is verified **[code]**. Whether the saved override then fails to match later calls is **[assumption]**;
confirm it with a test first.

**Why it matters for delegation:**
- D6 and the `files: read` permission need to know, per tool, **whether it reads, writes, or has outside effects**.
- Categories are too coarse for that: `filesystem` holds both read and write tools.
- So the fix is not just "use `models`". It is a **tool capability table**: one entry per tool holding category,
  access (`read` / `write` / `external`), whether it needs approval, and timeout class.
- Everything else derives from that table: categories, profile filtering, child tool scope, timeouts, approval
  category, and the editor's hints.

## 3. Target shape

### 3.1 Concepts

| Concept | One definition | Replaces |
|---|---|---|
| **Agent definition** | The profile, which gains `model`, `files`, `cloud_files`, `helpers`, `delegate_summary` (multi-agent report §9) | Profile + automation `ExecuteRequest` settings fields + chat context values |
| **Overrides** | One struct: model, profile, loop, tool scope, memory, network grant | D9, D10 |
| **Run plan** | `Resolve(kind, definition, overrides, workspace) → Plan`: the effective model and tool format, tools, memory, loop, step budget and network scope. One precedence chain: explicit → profile → model → global. | D4, D5, `resolveModelConfig`, `resolveProfile`, `allowedToolsFor`, `buildAgentOptions`, `AgentBuilder` |
| **Runner** | `Runner.Run(ctx, plan, input, sinks) → Result`: client wait (with an optional "starting" hook), run ID, usage, run folder and recording, event fan-out, executing the agent, and a run record | D1, D2, D3, D6, the shared half of D7 |
| **Run record** | One `RunRecord` (model, timing, tokens, warnings, `children []RunRecord`), embedded in `TurnRun` and `AutomationRun` | D8; sub-agent usage gets a home |
| **Admission** | Stays in `runlane`. Called by a small entry layer for top-level runs only. **Sub-agents never call it**: they take the sub-agent pool. | The deadlock rule becomes structural, not a convention |
| **Tool capability table** | One table in `models` | §2 |
| **Profile schema** | Published by the backend (fields, allowed values, tool categories, agent IDs). The parser validates against it and the editor's hints read it. | The UI and server can never disagree about which fields are valid |

Callers become thin adapters:
- **chat:** load and save the session, build the input, call the runner.
- **automation:** state, saved report and delivery, then the runner.
- **delegate tool:** build a child plan from the helper's profile, call the runner with the parent's context and a
  bounded sink, return the summary.

### 3.2 Packages

Proposed, to refine in the plan:

```
internal/core/
  assistant/            the loop engine only: Agent, strategies, sieve, stream, tool execution (stays the core)
    profile/            agent definitions + published schema (exists)
    toolpolicy/         tool scope, built on the capability table (exists)
  run/                  NEW: Overrides, Resolve → Plan, Runner, RunRecord, sinks; imports assistant
  conversation/         chat adapter (the existing Assistant Conversation Package plan, now on top of run/)
  automation/           dispatcher + automation adapter; heartbeat logic moved out of executor.go (:700)
  delegation/           NEW (small): the delegate tool's port implementation, the sub-agent pool, the child plan
  runlane/              unchanged
transport/http/handlers  HTTP only: decode, call a core service, encode
```

**The dependency rule** (this is what makes delegation fit without an import cycle):
- `run` imports `assistant`, because it builds the Agent. `assistant` must never import `run`.
- The delegate tool lives in `assistant`'s registry, so `assistant` declares a small port:
  `SubRunner interface { Run(ctx, Spec) (Result, error) }`.
- `delegation` implements it on top of `run`, and bootstrap injects it.
- The Assistant Conversation Package plan already says cycle risk "is the whole game"; the same applies here.

**Narrow ports:** each package declares only the interface it uses. The runner takes a small deps struct, not the
26-method service. The three wide interfaces shrink as their callers move.

### 3.3 Patterns, used where they remove code (not added for their own sake)

- **Resolver / one precedence chain.** Settings precedence today is written per field, per kind of run. One function
  with table tests replaces it.
- **Ports and adapters.** Narrow interfaces at package edges (`SubRunner`, the runner's deps). This is what allows the
  package moves without cycles.
- **Pipeline (template method) for the run.** Fixed steps (resolve → acquire → run → record) with kind-specific
  hooks (`OnStarting`, persistence). The hooks are function fields, not inheritance.
- **Composite.** A sub-agent run *is* a run; its record nests in the parent's. Totals and display fall out of that.
- **Table-driven metadata.** The tool capability table and the profile schema each have one table, and everything
  else is derived from it.
- **Keep strategy as is.** `LoopStrategy` is already a good example; delegation does not need a new strategy.
- **Pick one construction style.** `AgentOptions` built only by `Resolve`; delete `AgentBuilder`, which has one
  caller (production; no test uses it).

## 4. Existing plans this absorbs

| Plan | Becomes |
|---|---|
| Assistant Conversation Package (proposed) | Its **Step 0** (an importable `assistanttest` package for the test doubles) is a prerequisite for every move here. Its extraction becomes "chat adapter on top of `run/`". |
| Post-Implementation Cleanup (active) | Bucket A (dead code) first: cheap, and it shrinks what has to move. B and C are fixed where the new resolver touches them (B3/B5 are more copies of the "resolve settings" problem). |
| Codebase audit report (active) | Its handler items (one `writeError`, `parsePathVar`, `handleServiceError`; files read in handlers) fit "handlers are HTTP only". |
| Agent Improvements P4 (tool dedup), P5 (UsageTracker), P7 (sub-agent wrap) | P4 → the capability table; P5 → `RunRecord` + the `usage` tracker; P7 → superseded by `delegation/` + `SubRunner`. |
| Liveness Heartbeat §4.3/§4.4 (deferred package extractions) | Re-check against the target packages; do them only if they still make sense. |

## 5. Order: no unintended behaviour change until the feature step

Three changes are **deliberate**, and each updates its characterisation test on purpose:
1. Step 1: `edit_file_block` moves to `filesystem`, which changes its timeout and its approval/override category.
2. Step 3: memory review gains the cold-model wait.
3. Step 3: new runs use one run ID format.

Each step is TDD and separately mergeable, with `go test ./...`, the complexity check and a live chat + automation
smoke run.

0. **Safety net.**
   - The `assistanttest` doubles (Step 0 of the existing plan).
   - Delete Bucket A dead code.
   - **Characterisation tests that capture the resolved settings** (model, tool format, tools, memory, profile, loop,
     steps, network scope) for chat and automation across a matrix of inputs, before anything moves.
1. **Tool capability table.** Fix the three category sources (§2); add access and approval attributes. First, a test
   that shows the `edit_file_block` mismatch.
2. **Overrides + `Resolve` → `Plan`.** Chat and automation both use it; the step 0 tests must stay green. Delete
   `AgentBuilder`, `hotMemoryEnabled`, `memoryActive` and `buildAgentOptions`.
3. **Runner + `RunRecord`.** Merge D1/D2/D3/D6 and the shared half of D7. Memory review also gets the cold-model
   wait. The run ID becomes one format; no compatibility shim, since IDs are opaque.
4. **Move chat lifecycle out of the handler** into the chat adapter. Handlers become HTTP only, and the service
   interfaces narrow.
5. **Package moves:** `conversation/`, heartbeat out of `executor.go`, the `run/` package boundary.
6. **Feature:** `delegation/` + `SubRunner` + pool + researcher + eval (multi-agent report §9). It is then mostly new
   code in one small package, not edits across five.

**No line-count saving is claimed.** Nothing was measured. The concrete removals are the duplicates listed in §1/§2,
and "a fourth and fifth kind of run cost one adapter each" is the real win **[assumption]**.

## 6. Risks

- **Size.** Steps 2–4 touch the two busiest paths in the product. The characterisation tests in step 0 are what make
  the refactor safe; without them, don't start.
- **Hidden differences between the two paths.** Some differences may be deliberate. Each one found becomes an explicit
  hook or a field in the plan, never a silent merge. Known so far:
  - Chat's "model starting" event.
  - The tool deny list vs allow list.
  - Session vs `AgentState` persistence.
- **Areas not reviewed** (loop internals, frontend) may hold more of the same.
- **Merge conflicts with in-flight branches.** Check open branches before step 2.

## 7. Decisions for the user

- **R0: fix the `edit_file_block` category now?** It is a real bug today, and the fix is one missing switch arm plus a
  regression test. It can go in on its own, independent of R1. A later cleanup replaces the switch with the capability
  table.
- **R1: order.** The feature depends on steps **0–3**:
  - the capability table (child tool scope, `files: read`);
  - `Resolve` (the child plan from a profile);
  - the runner and `RunRecord` (child runs and nested usage);
  - plus the `SubRunner` port.

  It does **not** depend on step 4 (chat lifecycle out of the handler) or step 5 (package moves), because delegation
  runs inside the parent and never goes through the chat handler.
  - **(b) Steps 0–3, then the feature, then steps 4–5. Recommended:** the same benefit, sooner.
  - (a) Steps 0–5, then the feature: cleaner, but it delays the feature for work it does not need.
  - (c) The feature now, the refactor later: it adds the "third copy" this review is about.
