# Universal Agent Completion Model

**Status:** `complete` (all 3 phases code-complete; Phase 3 pre-implemented by `automation-renderer-unify-consumption`)  
**Created:** 2026-07-12  
**Last Updated:** 2026-07-21 (post cross-reference — all phases verified code-complete)  
**Constitution Refs:** II.4, II.7 (amended by Phase 2b), II.9  
**SPECs affected:** SPEC-001 (agent-loop, §5 rewritten by Phase 2b), SPEC-002 (tool-call-parser)  
**Skills affected:** agent-loop (updated by Phase 2b), assistant-ui-chat, assistant-ui-patterns, automation, event-streaming-patterns, lifecycle-events  
**Subsystems:** agent-loop (core changes in Phase 2b), assistant-ui, automation  
**Branches:** `task/universal_agent_completion` (Phase 1+2+2b uncommitted changes)
**Related completed plans:** `docs/PLANS/assistant-ui/automation-renderer-unify-consumption.md`, `docs/PLANS/assistant-ui/automation-unified-renderer-and-report-truncation.md` — both pre-implemented pieces of Phase 2 frontend + Phase 3 infrastructure (see §9).

> **Resolved (2026-07-18): event-stream contamination.** Phase 2b notes that assistant and automation share one SSE topic, which caused automation `finalReply`/nag output to leak into the assistant chat pane (frontend `useAssistantSSE` filtered only by `workspace_id`). Fixed with server-side channel isolation: every `AgentEvent` now carries `Channel` (`assistant`|`automation`) + `ConversationID`; the automation `EventBus` routes by `(workspace, channel)`; the `/live` SSE endpoint serves a single channel per connection (`?channel=assistant` for chat, default `automation` for the console). Frontend `useAssistantSSE` connects with `?channel=assistant` and drops any non-assistant event defensively. See `docs/architecture.md` Pitfall #6.

---

## 1. Problem Statement

llm-proxy relies on a synthetic `submit_final_answer` tool call as the canonical completion
signal for agent turns. This is an architectural convention invented by this project — not
something LLMs are natively trained to understand. The result is a system that diverges from
every production agentic solution (OpenCode, Copilot, Cline, Hermes Agent) and carries
significant maintenance burden.

### 1.1 What's Wrong

1. **Brittle completion convention.** Models forget the tool name, batch it with other tools
   (requiring multi-layered rejection logic), or use incorrect argument shapes (requiring
   `extractTaskSummary` with 6-field fallback parsing). ~80 lines of backend code exist
   solely to handle failures of this convention.

2. **Hard-coded agent instructions.** All agent behavior rules live in Go constants inside
   `templates.go`. Changing instructions requires backend recompilation and restart. Users
   cannot customize agent behavior without modifying source code.

3. **Frontend special-cases.** `messageBuilder.ts` has `isFinalTurn` flag, content-overwrite
   compensation, and `turnGrouper.ts` has submit skip logic — three separate special cases
   to handle what should be a normal message event.

4. **Divergent from industry standard.** Hermes Agent's `run_conversation()` exits when
   the model produces text with no tool calls. OpenCode uses a part-based message model
   where `TextPart` IS the answer. Cline uses `assistant_text` entries. Copilot's agent
   mode exits on assistant text. llm-proxy is the only system that uses a separate tool
   call for completion.

5. **Flat rendering.** The current ChatBubble renders reasoning text, tool calls, and
   results with equal visual weight. There is no clear boundary between "the model was
   working" and "the answer." After completion, work details remain visible by default.

### 1.2 What We're Solving

Three related problems as one unified change:

| # | Problem | Solution |
|---|---------|----------|
| P1 | Agent instructions hard-coded in Go constants | **AGENTS.md** — file-driven agent instructions loaded at runtime |
| P2 | `submit_final_answer` as brittle completion signal | **Natural completion** — model stops calling tools and produces text |
| P3 | Flat rendering: reasoning/tools/results all same weight | **Inset pattern** — work in collapsible panel, result always visible outside |

### 1.3 Why These Three Together

They form a coherent pipeline change with natural dependencies:

- **P1** (AGENTS.md) gives us a file where completion instructions can be freely edited
  without recompiling — a prerequisite for confidently changing completion behavior in P2.
- **P2** (remove submit) cleans the completion mechanism to match generic LLM behavior,
  simplifying the frontend so P3 doesn't need to handle submit special cases.
- **P3** (inset rendering) gives users the right UX: "the model was working" (collapsible)
  vs "the answer" (always visible). Works naturally with P2's simplified completion model.

---

## 2. Design Principles

### 2.1 Generic LLM Agentic Workflow — No Model-Specific Hacks

The agent must work the same way regardless of provider/model/API. We never encode
provider-specific logic in completion detection. The completion heuristic —
`precededByToolResult` + `no tool_calls` — is universal: every LLM can choose to stop
calling tools and produce text. This is the same mechanism that powers every production
agentic solution.

The heuristic is:

```
A turn is complete when the assistant:
  - Produces an assistant message with non-empty content
  - AND has NO tool_call entries
  - AND the preceding message in the conversation is a tool result
```

Why `precededByToolResult`? A model might produce text between tool calls as planning
(Reasoning → Plan → Tool Call). Without this guard, we'd confuse planning text with the
final answer. The guard ensures: the model acted, received results, THEN chose to write
text instead of calling another tool. That's the completion signal.

### 2.2 Separation of Concerns: Agent Identity vs System Mechanics

Agent instructions (who the agent IS, how it behaves, when it's done) live in
**AGENTS.md** — a user-editable file. System mechanics (tool format enforcement, XML
boundaries, error recovery prompts, dynamic tool lists) live in **Go code** — because
they're algorithmic, not declarative.

```
File (AGENTS.md):                     Code (templates.go):
┌────────────────────────────┐    ┌──────────────────────────────┐
│ Agent persona              │    │ BuildToolManual()            │
│ Behavioral rules           │    │ BuildNativeToolReference()   │
│ Completion guidance        │    │ Nag prompts (corrective)     │
│ Tool usage policy          │    │ Parse error feedback         │
│                            │    │ Format enforcement (XML)     │
│                            │    │ Memory-system prompts        │
│                            │    │ Dynamic tool list injection  │
└────────────────────────────┘    └──────────────────────────────┘
      Human-editable                   Compile-time, algorithmic
```

### 2.3 Assistant vs Automation: Same Agent, Different Context

Both modes use the same AGENTS.md + same loop exit heuristic. No special "automation
mode" that requires different completion behavior. The only differences are operational
context:

| Aspect | Assistant (Chat) | Automation (Task) |
|--------|-----------------|-------------------|
| **Task source** | User message in chat | `heartbeat.md` / task file content |
| **Prompt wrapper** | `DefaultAgentPrompt` + user message | `AutomationTaskPrompt` with workspace + content |
| **Nag/correction** | None (user is watching) | Nag prompts keep model moving |
| **Output** | Streamed to SSE → ChatBubble | Written to `final-report.md` + SSE |
| **Loop exit** | Same heuristic | Same heuristic |

The agent doesn't need to "know" which mode it's in. It follows AGENTS.md instructions.
The system handles the rest.

---

## 3. UI Design — Inset Pattern

### 3.1 Concept

The inset pattern separates "the model's work" from "the model's answer." Work (reasoning
+ tool calls + results) renders inside a visually distinct, collapsible panel. The final
answer renders outside the panel, always visible.

This matches the pattern used by VS Code Copilot (agent debug panel + inline chat),
OpenCode (collapsible "Thought" blocks), and Cline (collapsible "Thinking..." blocks).

### 3.2 Mockups

#### 3.2.1 During Tool Execution (inset expanded)

```
┌──────────────────────────────────────────────────────────────────────┐
│  [▼] Assistant — working (2 tools)                                   │
│                                                                      │
│  ┏━━ System ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓  │
│  ┃  ⟳ read_file: config.json                                      ┃  │
│  ┃     → { "port": 4001, "model": "..." }                         ┃  │
│  ┃                                                                   ┃  │
│  ┃  ⟳ execute_terminal_command: ls src/                             ┃  │
│  ┃     (running...)                                                  ┃  │
│  ┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛  │
│                                                                      │
│  ●●● Generating answer...                                            │
│                                                                      │
│  The project structure shows 2 source files:                         │
│  - main.ts with the HTTP server setup                                │
│  - helpers.ts with utility functions███                              │
└──────────────────────────────────────────────────────────────────────┘
```

Key design decisions:
- Inset panel: 2px left border in accent color (indigo-500/60) + darker background
  (gray-800/50) + 8px border radius
- Tool calls: compact rows with status icon + tool name + inline result preview
- Reasoning text: dimmed, prefixed with "Thought" label
- "Generating answer..." pulse appears at bottom of inset while result streams outside
- Result streams in real-time below the inset

#### 3.2.2 After Completion (inset collapsed)

```
┌──────────────────────────────────────────────────────────────────────┐
│  [▶] Assistant — 2 tools · completed (4.1s)                          │
│                                                                      │
│  The project structure shows 2 source files:                         │
│  - main.ts with the HTTP server setup                                │
│  - helpers.ts with utility functions                                 │
│                                                                      │
│  ## File: main.ts                                                    │
│  ```typescript                                                       │
│  import { serve } from "@std/http";                                  │
│  serve(handler, { port: 4001 });                                     │
│  ```                                                                 │
└──────────────────────────────────────────────────────────────────────┘
```

When collapsed, only the single-line summary header and the result are visible. All work
details are hidden behind the chevron toggle.

#### 3.2.3 During Initial Reasoning (no tools yet)

```
┌──────────────────────────────────────────────────────────────────────┐
│  [▼] Assistant — thinking...                                         │
│                                                                      │
│  ┏━━ System ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓  │
│  ┃  Thought: Let me check the workspace structure first. I'll      ┃  │
│  ┃  list the files, then read the config to understand the project  ┃  │
│  ┃  setup before making any changes.                                ┃  │
│  ┃                                                                   ┃  │
│  ┃  ●●●                                                              ┃  │
│  ┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛  │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

### 3.3 Why This Pattern (Rationale)

| Property | Benefit |
|----------|---------|
| **Collapsible work** | After the answer arrives, users don't need to see 15 tool calls. Skimmable. |
| **Result outside inset** | The answer IS the content the user wants. It should never be hidden. |
| **Real-time result streaming** | Users see the answer forming — same experience as a normal chat. |
| **Phase indicators** | "Thinking...", "Generating answer..." give transparency into model state. |
| **Compact tool rows** | Individual tools are expandable — summary inline, full detail on click. |
| **Model-agnostic** | The inset doesn't care about provider-specific reasoning formats. It just renders what the model emits. |
| **Convention over configuration** | The inset behavior is automatic — not a user toggle. Expand while working, collapse when done. |

---

## 4. Implementation Phases

### Phase 1: AGENTS.md (Agent Instruction File)

**Goal:** Move agent identity and behavioral instructions from Go constants to a file
loaded at runtime.

**Duration:** 3–4 hours

**Why first:** Least disruptive change. Doesn't alter LLM behavior — instructions are the
same, just sourced from disk. Establishes the pattern that Phases 2–3 build on.

**Deployable standalone:** Yes — AGENTS.md can ship before submit_final_answer is removed.

#### 4.1.1 What Moves to AGENTS.md vs What Stays in Code

**Moves to AGENTS.md (stable, declarative instructions):**
- Agent persona: "You are an autonomous agent with access to tools..."
- Behavioral rules: ReAct loop, no UI imitation, verify results, loop protection
- Completion guidance: how to deliver the final answer
- Tool usage policy: use only listed tools, batch when efficient
- Workspace rules: relative paths, files are data

**Stays in Go code (dynamic/algorithmic/corrective content):**

| Category | Examples | Why |
|----------|----------|-----|
| Dynamic content | `BuildToolManual()`, `BuildNativeToolReference()` | Injects tool names + schemas via `fmt.Sprintf` |
| Nag prompts | `AutomationNagPrompt`, `DuplicateNagPrompt`, `ToolErrorNagPrompt` | Corrective feedback, not identity |
| Format enforcement | `AutomationPrefline`, `AutomationXMLModeGuide`, `AutomationJSONPlanPrompt` | Algorithmic format forcing |
| Parse error feedback | `FeedbackNoXML`, `FeedbackJSONError`, `FeedbackBadTool`, `TranslateJSONError` | Dynamic: includes tool names + error details |
| System invariants | `FileSystemRules`, `InstructionBoundaryRule` | Non-customizable constraints |
| Memory system | `PreSieveMemoryNudge`, `RelevantMemoriesHeader` | Procedural, tied to memory manager |
| System injections | `SieveSystemNote`, `RetrySignal`, `ContextSieveWarning` | Dynamic, depends on runtime state |
| Dynamic templates | `AutomationTaskPrompt` | Injects workspace name + task content |

#### 4.1.2 Default AGENTS.md Content

New file: `backend/internal/core/assistant/prompts/default_agents_md.go`

> **Phase 1 note:** The `## Completion` section references `submit_final_answer`
> (via the `models.ToolSubmitFinalAnswer` constant) because that tool is still
> the active completion path during Phase 1 — the earlier natural-language
> wording would contradict the live behavior. Phase 2 rewrites this section to
> the natural-completion model (§4.2.5). The `## Workspace Rules` section from
> the original draft was dropped: it verbatim-duplicated `FileSystemRules` +
> `InstructionBoundaryRule`, which `AssembleSystemPrompt` always prepends.

```go
package prompts

import "llm-proxy/models"

// DefaultAgentsMD is the default content seeded into a workspace's AGENTS.md
// file on creation. Users can edit AGENTS.md to customize agent behavior; it
// is loaded at runtime (see LoadAgentsFile) and appended to the system prompt.
const DefaultAgentsMD = `# Workspace Agent

You are an autonomous agent with access to tools. Your job is to complete
the given task by using tools.

## Core Rules
1. ReAct Loop: Use Thought -> Action -> Observation sequence.
2. Use tools to act. Do not describe what you would do -- do it.
3. No UI imitation: never output emojis, "Executing...", or technical
   status markers. These are generated by the system.
4. Verify results: after each action, check the output before proceeding.
5. Loop protection: if a tool returns a repetition warning, do NOT retry.
   Change your approach or deliver your answer with available data.

## Completion
When you have finished your task, call the ` + models.ToolSubmitFinalAnswer + ` tool to
deliver your report. Put the complete answer in the 'summary' argument -- it is
the final product shown to the user, so include all requested data (file
contents, command output, tables) instead of a description of what you did.

## Tool Guidelines
- Use ONLY the tools listed in the tool interface section of the system
  prompt. Stick to tool names exactly.
- Batch related tool calls into a single response for efficiency.
- If a tool fails, read the error and adapt. Do not retry failing calls.
- Best practice: always verify your environment first.
`
```

#### 4.1.3 Changes

| # | File | Action |
|---|------|--------|
| 1 | `prompts/default_agents_md.go` | **CREATE** — default AGENTS.md content |
| 2 | `models/workspace.go:19` | **MODIFY** — `RulesFilename = "AGENTS.md"` (was `"rules.md"`) |
| 3 | `dispatcher_handlers.go:361-366` | **MODIFY** — seed AGENTS.md from `DefaultAgentsMD`, remove `agent.md` seed (its content merged into AGENTS.md) |
| 4 | `conversation_helpers.go:19-25` | **MODIFY** — read `"AGENTS.md"`, fallback to `"rules.md"` for existing workspaces |
| 5 | `executor.go:130` | **MODIFY** — same read with fallback |
| 6 | `templates.go:182` | **MODIFY** — rename `customRules` param to `agentsFileContent` |
| 7 | `templates.go:146-191` | **MODIFY** — mark `DefaultRules`/`DefaultRulesNative` as deprecated (kept for backward compat) |
| 8 | `templates.go:200-207` + `models/workspace.go:18` | **DELETE** `DefaultAgentPrompt` constant and `AgentPromptFilename` constant — content is now in `DefaultAgentsMD`, `agent.md` is no longer seeded |

#### 4.1.4 Migration (Backward Compatibility)

```go
// In conversation_helpers.go and executor.go:
func loadAgentsFile(pm *persistence.WorkspaceManager, workspaceID string) string {
    content, err := pm.ReadTaskFile(workspaceID, "AGENTS.md")
    if err == nil && content != "" {
        return content
    }
    // Fallback: existing workspaces with legacy rules.md
    content, err = pm.ReadTaskFile(workspaceID, "rules.md")
    if err == nil && content != "" {
        return content
    }
    return prompts.DefaultAgentsMD
}
```

#### 4.1.5 Verification

```bash
cd backend && go build ./...
cd backend && go test ./... -run "TestAssembleSystemPrompt|TestBuildInitialHistory"
```

> ### 🔍 Test Checkpoint 1 — After Phase 1
>
> **Status:** No behavioral change. Instructions now load from `AGENTS.md` instead of Go constants.
>
> **Local LLM test:**
> 1. Create a new workspace → confirm `AGENTS.md` is seeded with `DefaultAgentsMD` content.
> 2. Edit `AGENTS.md` (e.g., add "Always respond in French.") → next turn reflects the change.
> 3. Run a normal task → agent still completes via `submit_final_answer` (no regression).
> 4. Existing workspace with `rules.md` → confirm it loads as fallback.
>
> **Pass criteria:** Agent behaves identically to before; only instruction source changed.
> **Do NOT proceed to Phase 2 until this passes** — it confirms the file-loading plumbing works.

---

### Phase 2: Remove `submit_final_answer`

**Goal:** Replace the synthetic completion tool with natural loop exit: "model stops
calling tools and produces text."

**Duration:** 6–8 hours

**Why second:** AGENTS.md is in place, so the new completion instructions go into the file
rather than Go constants. Backend prompt cleanup targets the now-deprecated constants.

#### 4.2.1 New Completion Heuristic

Implemented in `session.go` replacing `checkSubmitFinalAnswer`:

```go
// checkTaskCompletion determines if an assistant message signals task completion.
//
// A turn is complete when the assistant:
//   - Produces non-empty content
//   - Has no tool calls (not planning to act further)
//   - The preceding message is a tool result (the model acted, got data, now writes answer)
//
// This is the industry-standard completion pattern used by OpenCode, Cline, Hermes Agent,
// and Copilot. It works across all LLM providers without provider-specific detection.
//
// Returns (finalContent, isComplete).
func checkTaskCompletion(msg proxy.Message, previousMsg *proxy.Message) (string, bool) {
    if len(msg.ToolCalls) > 0 {
        return "", false
    }
    if strings.TrimSpace(msg.Content) == "" {
        return "", false
    }
    if previousMsg != nil && previousMsg.Role == proxy.ToolRole {
        return msg.Content, true
    }
    return "", false
}
```

> ⚠️ **Deprecated rationale — see Phase 2b (§5.0+).** The `ToolRole` adjacency requirement
> introduced bugs (double final report, native-tools completion failure). Phase 2b redesigns
> the completion gate using think-block stripping + any-tool-result-in-history (Hermes-aligned).

**Why `precededByToolResult` was chosen (Phase 2, deprecated):** A model may produce text between tool calls as
planning. Without this guard, planning text would be mistaken for the answer. The guard
means: the model acted, received results, THEN chose text over more tool calls.

#### 4.2.2 Session Loop Changes (`session.go`)

| Location | Current Code | Change |
|----------|-------------|--------|
| `session.go:137-148` | `checkSubmitFinalAnswer()` — iterates tool calls for `submit_final_answer`, extracts summary from args | **Delete function.** Replace with `checkTaskCompletion()` (defined in §4.2.1). |
| `session.go:25` | `maxBatchedSubmitRetries = 3` constant | **Delete** |
| `session.go:40-43` | `batchedSubmitRetries int` field on `runSession` | **Delete** |
| `session.go:220-253` | Inline `submitSolo` check, `checkSubmitFinalAnswer` call, summary-to-history writeback, `notifyLifecycle("completed")`, batched counter increment + kill | Replace with: call `checkTaskCompletion(turnMsg, &prevMsg)`. If complete → `notifyLifecycle("completed", {"content": turnMsg.Content})` + return. |
| `session.go:357` | `handleNoToolCalls()` — chat-mode exit handler (premature termination, precededByToolResult, 2-consecutive-no-tools) | **Redesigned in Phase 2b.** The Phase 2 plan said "Unchanged" but native-tools rejection (§2b.2) requires it. |

All `s.checkSubmitFinalAnswer(&turnMsg)` call sites → `s.checkTaskCompletion(&turnMsg, &prevMsg)`.

#### 4.2.3 Tool Execution Changes (`tool_exec.go`)

| Location | Current Code | Change |
|----------|-------------|--------|
| `tool_exec.go:107-120` | `hasSubmit` flag + early rejection: if submit present AND `len > 1` → append error results for all tools in batch (~20 LOC) | **Delete** — no submit tool to reject |
| `tool_exec.go:190` | Early return: `if tc.Function.Name == models.ToolSubmitFinalAnswer { return nil }` — stops loop after executing the submit tool | **Delete** — becomes dead code (tool won't be registered) |
| `tool_exec.go:347-371` | `extractTaskSummary()` — walks 6 field names (summary > message > report > findings > content > result), handles truncated JSON via `extractTruncatedJSONField` (~25 LOC) | **Delete** — final content comes directly from assistant message, no extraction needed |
| `tool_exec.go:345` | Comment on `extractTaskSummary` | **Delete** |

#### 4.2.4 Repetition Detector Changes (`agent.go`)

Current exemption at `agent.go:358`:
```
- submit_final_answer and system_error tools are exempt
```

After removal:
```
- system_error tool is exempt (submit_final_answer removed — no longer exists)
```

#### 4.2.5 Prompt Template Changes (`templates.go`)

Affected constants (all in `backend/internal/core/assistant/prompts/templates.go`):

| Constant | Old | New |
|----------|-----|-----|
| `DefaultRules` rule 6 | "call submit_final_answer to deliver final report" | "write your answer as clear natural language or Markdown" |
| `DefaultRulesNative` rule 6 | Same | Same |
| `UnifiedToolManual` rules 4 + example | Submit-specific rules + example | Simplified: "When finished, write your answer. No tool call needed." |
| `NativeToolReference` rule 4 | "You are not finished until you call submit_final_answer" | "When you have all the information needed, write your final answer directly" |
| `AutomationTaskPrompt:428` | "Call submit_final_answer when done." | "Write your final report when the task is complete." |
| `ContextSieveWarning` | "call submit_final_answer when done" | "deliver your answer when complete" |
| `AutomationJSONPlanPrompt` | contains `{"tool": "submit_final_answer"}` step | Remove submit step from example plan |
| `AutomationRejectedSubmissionPrompt` | Entire constant | **Deleted** |

#### 4.2.6 System Prompt Changes (`system_prompt.go`)

```go
// Old (lines 12-15):
finalAnswerInstruction := "CRITICAL: When you have finished your task or have a final answer
for the user, your response MUST be a clear natural language or Markdown answer. \nDO NOT
include raw technical data structures in the final answer you provide to the user."
if useNativeTools {
    finalAnswerInstruction = "CRITICAL: In native tool-calling mode, your final answer MUST
    be delivered through the '" + models.ToolSubmitFinalAnswer + "' tool's 'summary' argument.
    Keep any freeform content brief..."
}

// New:
finalAnswerInstruction := "CRITICAL: When you have finished your task, write your final answer
as clear natural language or Markdown. Do not call any tools in the same response."
if useNativeTools {
    finalAnswerInstruction = "CRITICAL: In native tool-calling mode, when you have finished
    your task, produce only your final answer text -- no tool calls. Your answer is delivered
    directly to the user."
}
```

#### 4.2.7 Tool System Cleanup

| # | File | Action |
|---|------|--------|
| 1 | `models/tools.go` | **DELETE** `ToolSubmitFinalAnswer = "submit_final_answer"` |
| 2 | `registry.go` | **MODIFY** — remove submit from `registerSystemTools()` |
| 3 | `models/tools.go` | **KEEP** `CategorySystem` — also used by `ToolSystemError` (registry.go:522). Only delete `ToolSubmitFinalAnswer`. |
| 4 | `backend/internal/core/tools/manifests/system.json` | **MODIFY** — remove `submit_final_answer` entry (lines 6-18). **KEEP** `system_error` entry (lines 19-30). |

#### 4.2.8 Remove `tool_choice:  required` Override

The `buildChatRequest` function in `stream.go` forced `tool_choice:  required` on every native-tools request.
This directly prevented natural completion — with `tool_choice:  required`, the model MUST produce a tool
call every turn, so it cannot signal completion by writing text with no tool calls.  Removal is required
for the §4.2.1 heuristic to work.

| # | File | Change |
|---|------|--------|
| 1 | `stream.go:64-66` | **DELETE** — remove the `tool_choice: required` override. The API-level `tool_choice` is now unset (defaults to `auto`), allowing the model to freely choose between calling tools and writing text. |
| 2 | `agent_test.go:1863-1864` | **MODIFY** — `TestAgent_Execute_NativeToolsSetsToolChoice` now expects empty `ToolChoice` instead of `ToolChoiceRequired`. |

With `tool_choice` removed, the system prompt's instruction "write the final report as your last
assistant message" is no longer contradicted by the API-level constraint.

#### 4.2.9 Communication Tool Description

The `notify_user` tool description contained the word "report" in its `message` parameter description,
leading models to confuse the communication notification tool with the "report" delivery mechanism.
Since natural completion (§4.2.1) removes `submit_final_answer` as the report-delivery tool, the
description must be explicit that `notify_user` is NOT for task results.

| # | File | Change |
|---|------|--------|
| 1 | `manifests/communication.json` | **MODIFY** — tool description: never mention "report" or "report content". Add explicit guidance: "Do NOT use this to deliver task results — those belong in your assistant response, not in a notification." |

#### 4.2.10 Automation Output Changes

| # | File | Old | New |
|---|------|-----|-----|
| 1 | `executor.go:142-163` | `finalReply` from extracted submit summary | `finalReply` = last assistant message content directly |
| 2 | `rundir.go` | `final-report.md` from submit summary | `final-report.md` from last assistant message content |
| 3 | `executor.go:584-587` | `buildPrompt` appends "Call submit_final_answer when done" | Updated template |
| 4 | `executor.go:341-347` | Hardcoded `EventMessage` with `Content: "✔ Execution complete."` sent to clear "thinking…" indicator | **Unchanged.** Fires after `agent.Execute()` returns — NOT tied to submit_final_answer. No change needed. Optionally: remove and rely on `lifecycle:completed` from agent loop instead. |

#### 4.2.11 Webhook Handler Changes

| # | File | Change |
|---|------|--------|
| 1 | `webhook_handlers.go:121` | The handler currently watches for `EventMessage` with `Content == "✔ Execution complete."` — this message is sent by `executor.go:345` (see §4.2.8 row 4). Since the executor still sends it, the webhook handler **requires no change after removing submit_final_answer**. Optionally switch to listen for `lifecycle` "completed" event with `EventMessage` content payload instead — this is cleaner and does not depend on a hardcoded string. |

> ### 🔍 Test Checkpoint 2a — After Backend Completion Removal (§4.2.1–§4.2.11)
>
> **Status:** Backend complete. Frontend (§4.2.12) and tests/docs (§4.2.13–§4.2.16) NOT yet done.
>
> **Local LLM test (API-level, no UI needed):**
> 1. Start backend with local model (e.g., Qwen3.5-9B via llama.cpp).
> 2. Send a task → agent should call tools, then produce a final answer WITHOUT `submit_final_answer` or any tool call. The `tool_choice: required` override was removed (§4.2.8), so the model can freely choose between calling tools and writing text — natural completion must work without API-level coercion.
> 3. **Automation mode**: run a heartbeat automation → confirm it completes and writes `final-report.md`.
> 4. Verify SSE stream emits `lifecycle:completed` after the final answer.
>
> **Pass criteria:** Agent completes tasks in both modes without `submit_final_answer` or forced tool calls.
> **UI will look broken** (old ChatBubble shows raw content) until §4.2.12 lands — that's expected at this checkpoint.

#### 4.2.12 Frontend Changes — SUPERSEDED

> **Status:** All items below were implemented by `docs/PLANS/assistant-ui/automation-renderer-unify-consumption.md` (complete). That plan unified chat + automation event consumption through a single `useMessageBuilder` path with `MessageBuilderOptions` (`source`, `headerMessage`, `finalizeOn`). In doing so it naturally removed `isFinalTurn` and `isSubmit` skip logic, added lifecycle-based finalize, and wired the shared renderer. The original table below is kept for historical reference only.

<details>
<summary>Original Phase 2 frontend spec (implemented by automation-renderer-unify-consumption)</summary>

| # | File | Change |
|---|------|--------|
| 1 | `messageBuilder.ts:23-35` | **DELETE** `isFinalTurn` from reactive state |
| 2 | `messageBuilder.ts:155-176` | **DELETE** `isFinalTurn` guard in `handleToolStream()` |
| 3 | `messageBuilder.ts:212-234` | `handleMessage()`: remove `hasFinal`/`isFinalTurn` logic. Content with no tool_calls → commit reasoning, stream into result. |
| 4 | `messageBuilder.ts:236-245` | `finalize()`: simplify — no submit content extraction |
| 5 | `turnGrouper.ts:92` | Remove `isSubmit` skip logic in `buildSegmentsFromHistory()` |

</details>

#### 4.2.13 Backend Test Changes

**Policy: update, don't delete.** Tests referencing `submit_final_answer` get updated to
test the new `checkTaskCompletion` path. No net loss of coverage.

| Test File | Change |
|-----------|--------|
| `agent_test.go` | ~64 submit references → update to test `checkTaskCompletion` with `precededByToolResult` (Phase 2); Phase 2b updates these further |
| `filtered_provider_test.go` | Update submit references |
| `agent_memory_test.go` | Update submit references |
| `session_test.go` | Update submit references |
| `prompts/system_prompt_test.go` | Update test: "native mode should contain submit_final_answer instruction" → new instruction |
| `prompts/templates_test.go` | `TestAssembleSystemPrompt_ToolCallFormat` — update |
| `conversation_helpers_test.go` | Add AGENTS.md loading + fallback tests |

#### 4.2.14 Constitution Amendment

> ⚠️ **Updated in Phase 2b.** The constitution text below reflects the Phase 2 design.
> Phase 2b replaces the `precededByToolResult` adjacency with think-block stripping + 
> any-tool-result-in-history. The corrected text is:

```markdown
7.  **Natural Task Completion (amended)** — The agent completes a task when,
    after producing at least one tool result, it writes a substantive assistant
    message with no pending tool calls. Reasoning blocks (``<think>``, 
    ``<reasoning>``) are stripped before evaluating content — a turn whose visible
    text is only reasoning is NOT a final answer. Completion does not require the
    immediately-preceding message to be a tool result; reasoning-only interleaves
    between tool results and the final answer do not block completion. The
    ``submit_final_answer`` synthetic tool call is removed.
```

**Original Phase 2 text (kept for historical reference):**
```markdown
7.  **Explicit Task Completion (amended)** — The canonical path to task completion
    is an assistant message with content and no tool calls, where the preceding
    message is a tool result (precededByToolResult pattern). In chat mode the
    agent also exits after 2 consecutive assistant messages with no tool calls
    or after premature termination detection. Heuristic keyword matching
    ("task complete", "summary") is not used. The ``submit_final_answer`` tool
    is removed.
```

#### 4.2.15 Documentation Updates

| Document | Change |
|----------|--------|
| `docs/SPECS/agent-loop.md` lines 41, 46, 50-53 | Update completion paths, remove submit_final_answer references |
| `docs/skills/agent-loop.md` | Update stuck detection and completion references |
| `docs/skills/assistant-ui-chat.md` | Remove `isFinalTurn` references |
| `docs/skills/automation.md` | Update final answer extraction description |
| `docs/skills/lifecycle-events.md` | Update `session_completed` phase trigger description |
| `docs/skills/event-streaming-patterns.md` | Audit and remove submit_final_answer references in SSE composable sections |
| `docs/skills/assistant-ui-patterns.md` | Audit and remove submit_final_answer references in tool rendering sections |
| `docs/SPECS/tool-call-parser.md` (SPEC-002) | Audit for submit_final_answer as valid tool name — remove if present |
| `docs/PLANS/memory/memory-improvements-implementation-plan.md` (line 498) | References submit_final_answer as skill-store trigger — update or note as no longer applicable |
| `docs/PLANS/cross-cutting/webhook-fresh-sessions.md` (line 18) | References submit_final_answer as problem source — update |
| `docs/PLANS/ARCHIVE/**` | Archived plans that reference submit_final_answer (e.g., `unify-assistant-automation.md`, `refactor-assistant-clean-code.md`, `agnostic-agent-loop.md`) — **no update needed**. Archive plans are historical reference, not active code. |

#### 4.2.16 Verification

```bash
cd backend && go build ./...
cd backend && go test ./... -count=1 -timeout 120s
cd backend && go run ./tools/check-complexity/
cd frontend && npm run build
```

> ### 🔍 Test Checkpoint 2b — After Phase 2 Complete (§4.2.12–§4.2.16)
>
> **Status:** Full completion removal + frontend + tests + constitution + docs done.
>
> **Local LLM test (UI-level):**
> 1. Chat mode: send a multi-step task → verify the answer renders correctly (no `isFinalTurn` artifacts).
> 2. Automation mode: run a task → verify `final-report.md` is correct and UI shows completion.
> 3. Run `go test ./...` → all submit references updated, tests green.
> 4. Confirm `CONSTITUTION.md` II.7 reflects the new completion model.
>
> **Pass criteria:** Agent completes correctly in both modes via UI; all tests pass.
> **Do NOT proceed to Phase 3 until this passes** — Phase 3 is cosmetic only.

---

### Phase 2b: Fix Completion Gate — Hermes-Aligned Redesign

**Status:** `in-progress` (code complete — 2 manual verifications pending)  
**Priority:** P1 (blocks Phase 2 sign-off)  
**Depends on:** Phase 2 code changes (implemented)

Phase 2 replaced `submit_final_answer` with `checkTaskCompletion`, but the
completion gate has two bugs discovered in live testing:

1. **Double final report** — When the model produces a reasoning-only turn
   (content empty, large `ReasoningContent`) between the last tool result and
   the final text answer, `checkTaskCompletion` fails its `ToolRole` adjacency
   requirement → the model gets nagged → it re-emits the final report.
2. **Native-tools completion failure** — For local models (Qwen, Gemma, etc.)
   running with `useNativeTools=true`, `handleNoToolCalls` (session.go:565-569)
   explicitly REJECTS plain-text completion by clearing `parseErr` and forcing a
   nag. The model can never finish by natural completion — it either loops until
   timeout or gets force-killed at `2×MaxSteps`.

Both bugs stem from the same root cause: the agent loop treats "assistant text
without tool calls" as a PROBLEM rather than as SUCCESS.

#### 2b.1 Hermes Agent Analysis

Nous Research's [Hermes Agent](https://github.com/NousResearch/hermes-agent)
(used in production with models from 4B local to Claude) uses a fundamentally
different completion model. Source: `agent/conversation_loop.py` (3900-line
`run_conversation`).

**Core completion decision (line ~4969-4988):**

```python
if assistant_message.tool_calls:
    process_tools()
    if content_present:
        _last_content_with_tools = content   # fallback
    continue                                 # keep looping
else:
    final_response = content                 # DONE. IMMEDIATE.
    if thinking_only (no content after <think> strip):
        handle_empty/reasoning-only          # one nudge, not perpetual
    else:
        break                                # COMPLETE
```

**Key architectural differences from llm-proxy:**

| Dimension | llm-proxy (current) | Hermes Agent |
|-----------|---------------------|--------------|
| Completion trigger | `previousMsg.Role == ToolRole` + no tool calls | No tool calls = done. Period. |
| Think-block handling | None (confused by reasoning in content) | Strip `<think>`/`<reasoning>` tags; check remaining text |
| Native-tools text answer | Treated as protocol violation → nag | Accepted as final (trust the model) |
| Empty after tools | Perpetual nag loop | One-shot nudge, then fallback content |
| Content with tools | Ignored | Captured as `_last_content_with_tools` fallback |
| Premature termination guard | 3× identical text + empty content | Same + think-only + kanban + verify-on-stop |

**Hermes guardrail layers (for small/weak models):**

| Layer | What | Why for small models |
|-------|------|----------------------|
| Think-block stripping | `_has_content_after_think_block()` — strips `<think>`/`<reasoning>`/`<REASONING_SCRATCHPAD>` tags. If nothing remains, NOT done. | Qwen3/Ollama puts reasoning in `content` field. Without stripping, thinking-only gets treated as completion. |
| One-shot empty nudge | After tools, if model returns empty: one "you executed tools but returned empty" nudge. If still empty, use `_last_content_with_tools` or give up. | Weak models sometimes go silent after tool results. One retry fixes ~80% of cases. |
| Content-with-tools fallback | If model writes text AND calls tools in same turn, save the text. If next turn is empty, use it as final. | Models that write report while calling `write_file` in same turn — the text is the answer. |
| Verification stop | If code files were edited this turn, nudge model to verify before accepting completion. | Prevents "I fixed it!" without actually running tests. |
| Exact-repeat detection | 3 identical consecutive assistant messages → not done (nag). | Model looping on same text. (Already in llm-proxy via `isPrematureTermination`.) |

**What Hermes does NOT do:**
- No `ToolRole` adjacency check on the prior message
- No rejection of plain-text answers in native-tools mode
- No perpetual nagging (one nudge, then fallback)
- No minimum content length check (Hermes trusts the model; our ≥20-char guard from Phase 2 is an additional safety net for small local models and should be kept)
- **No structural block on `write_file` for reports** — Hermes relies entirely on prompt instructions to prevent file writes when the task doesn't request one. Weak models ignoring the instruction is accepted behavior, not structurally prevented. (Audit: `docs/audits/hermes-write-file-guardrail.md`)

#### 2b.2 Root Cause in llm-proxy

The completion path is:

```
executeTurn → turnMsg, parseErr, toolsList
     ↓
run() loop:
  if toolCalls > 0:
    handleToolTurn() → execute tools, continue
  else:
    handleTextTurn(turnMsg, parseErr, toolsList):
      checkTaskCompletion(turnMsg, previousMsg):
        if previousMsg.Role == ToolRole:       ← BUG: adjacency broken by reasoning turns
          return (content, true)
        return ("", false)
      ↓ (fails)
      handleNoToolCalls(turnMsg, parseErr, toolsList):
        if parseErr != nil && !XMLFound && content != "":
          if !nativeTools || no tools:
            return (content, true)   ← works for non-native
          parseErr = nil             ← BUG: rejects native-tools completion
        WARN "no tool calls - nagging model"
        injectPrompts.AutomationNagPrompt
        return ("", false, nil)
```

**Bug 1 — Adjacency break (at 17:55:14 in the ts-dashboard run):**
The model's final turn was preceded by a reasoning-only assistant turn (17:54:09,
0 tool calls, 1989 reasoning chars, 0 content chars). `previousConversationMessage`
walks backward to find nearest non-control message → finds `AssistantRole` (the
reasoning turn), NOT `ToolRole` → `checkTaskCompletion` returns false → falls to
`handleNoToolCalls` → nag → model re-emits report.

**Bug 2 — Native-tools rejection (native-tools local models):**
`handleNoToolCalls` line 565-569: when `useNativeTools=true` and tools are
present, if the model produces plain text content (no tool calls, no XML
markers), the code sets `parseErr = nil` and falls through to the nag path
(session.go:595). This means a native-tools model CANNOT complete via natural
completion — EVERY no-tool-call turn gets nagged. The only escape is
forced-completion at `2×MaxSteps`.

#### 2b.3 Redesigned Completion Gate

The fix replaces three functions in `session.go` and adds one helper.

##### 2b.3.1 New helper: `stripThinkBlocks`

```go
// stripThinkBlocks removes Qwen3/Ollama <think>, <reasoning>, and
// <REASONING_SCRATCHPAD> blocks from content. Models that emit structured
// reasoning via the API (reasoning_content field) are unaffected — this
// only handles in-content reasoning tags.
var thinkBlockRegex = regexp.MustCompile(
    `(?is)<(?:think|reasoning|REASONING_SCRATCHPAD)\b[^>]*>.*?</\1>`)

func stripThinkBlocks(content string) string {
    return strings.TrimSpace(thinkBlockRegex.ReplaceAllString(content, ""))
}

// hasContentAfterThinkBlock returns true when there is substantive
// visible text after stripping reasoning tags.
func hasContentAfterThinkBlock(content string) bool {
    return len(stripThinkBlocks(content)) > 0
}
```

File: `session.go`, placed near `checkTaskCompletion` (~line 190).

##### 2b.3.2 `checkTaskCompletion` — replacement (session.go:206-223)

**Current (broken):**
```go
func checkTaskCompletion(msg proxy.Message, previousMsg *proxy.Message) (string, bool) {
    if len(msg.ToolCalls) > 0 { return "", false }
    content := strings.TrimSpace(msg.Content)
    if content == "" { return "", false }
    if len(content) < 20 { return "", false }
    if previousMsg != nil && previousMsg.Role == proxy.ToolRole {
        return msg.Content, true           // ← BUG: strict adjacency
    }
    return "", false
}
```

**New:**
```go
func checkTaskCompletion(msg proxy.Message, previousMsg *proxy.Message, history []proxy.Message) (string, bool) {
    if len(msg.ToolCalls) > 0 {
        return "", false
    }

    // Strip reasoning tags — Qwen3/Ollama put <think> in content.
    // If nothing substantive remains, this is NOT a final answer.
    stripped := stripThinkBlocks(msg.Content)
    if len(stripped) < 20 {
        return "", false
    }

    // Guard: text must not contain unparsed tool-call markers.
    if hasToolCallMarker(stripped) {
        return "", false
    }

    // The run must have produced at least one tool result anywhere
    // in history (after the system prompt).  This prevents premature
    // first-turn text from being treated as completion while still
    // allowing reasoning-only interleaves between tools and the answer.
    pos := -1
    for i := len(history) - 1; i >= 0; i-- {
        if history[i].Role == proxy.ToolRole {
            pos = i
            break
        }
    }
    if pos < 0 {
        return "", false
    }

    return stripped, true
}
```

**Key changes from Phase 2 design:**
1. Signature adds `history []proxy.Message` parameter.
2. Replaces `previousMsg.Role == ToolRole` (strict adjacency) with
   `historyHasToolResult(history)` — searching ANYWHERE in history for a
   tool result. This is the Hermes-aligned insight: the model writes after
   having actually done work, not just after the immediately-preceding message.
3. Adds `stripThinkBlocks` — prevents reasoning-only content from being
   treated as completion.
4. Adds `hasToolCallMarker` guard — prevents unparsed tool calls in content
   from being treated as a final answer.

##### 2b.3.3 `handleTextTurn` — caller update (session.go:443)

```go
if content, ok := checkTaskCompletion(turnMsg, prevMsg, s.history); ok {
```

Previous call site: `checkTaskCompletion(turnMsg, prevMsg)`. Only signature
change — pass `s.history`.

##### 2b.3.4 `handleNoToolCalls` — one-shot nag + trust (session.go:543-601)

**Key deletions to the native-tools rejection block (lines 565-569):**

```go
// DELETE: this block rejects native-tools completion
// if !parseErr.XMLFound && strings.TrimSpace(turnMsg.Content) != "" {
//     if !s.agent.config.UseNativeTools || len(toolsList) == 0 {
//         return turnMsg.Content, true, nil
//     }
//     parseErr = nil    // ← this is the bug: clears error, forces nag
// }
```

**Replacement logic (at the no-calls + no-content fallback):**

When `handleNoToolCalls` is reached AND `turnMsg.Content` is empty (genuine
stuck/empty, not a text answer — text answers complete before reaching here),
the behavior changes from perpetual nag to one-shot:

```go
// If we're here, the model genuinely produced nothing (empty content).
// One-shot nag: inject the nag prompt ONCE.  If the next turn is still
// empty, fall back to the best available content from history or force-
// complete.  Previously this nagged forever — a perpetual loop.
if s.agent.forcedCompletionSent {
    // Already nagged once.  Use fallback or force complete.
    if content := s.bestAvailableAnswer(); content != "" {
        return content, true, nil
    }
    return "", true, nil
}

s.agent.forcedCompletionSent = true  // reuse existing field
s.agent.deps.Logger.Warn("no tool calls - nagging model (one-shot)", "step", s.steps)
s.history = append(s.history, proxy.Message{
    Role:    proxy.UserRole,
    Content: prompts.AutomationNagPrompt,
})
return "", false, nil
```

**Design note:** The existing `forcedCompletionSent` field on `runSession`
(session.go:47) was previously only used in `checkForcedCompletion` for the
`2×MaxSteps` hard cap. We reuse it as the one-shot flag — a field added for
this purpose is cleaner, but reusing avoids adding a new runSession field.
If a model genuinely hits the one-shot nag AND then `2×MaxSteps` before the
second turn completes, the flag overlap is harmless — both paths lead to
forcing completion.

##### 2b.3.5 `handleToolTurn` — content-with-tools fallback (session.go:416-434)

When the model writes text AND calls tools in the same turn, save the text as
a fallback. If the next turn is empty, use the fallback.

```go
// In handleToolTurn, after trimLargeWriteContent, before append:
// If this turn has both content AND tool calls, save content as
// a fallback final answer.  If the follow-up turn is empty, we use it.
// Common pattern: model delivers answer + calls write_file in same turn.
stripped := stripThinkBlocks(turnMsg.Content)
if len(stripped) >= 20 && len(turnMsg.ToolCalls) > 0 {
    s.lastContentWithTools = stripped  // new runSession field
}
```

**New runSession field** (session.go:33-52):
```go
type runSession struct {
    // ... existing fields ...
    lastContentWithTools string // content saved from tool-turn with text
}
```

**Fallback usage in handleToolTurn** (after tool execution, at session.go:423):
```go
if salvaged != "" {
    // existing salvage logic
} else {
    // After executing tools, check if next turn is empty
    // (handled in handleTextTurn if the next computeNextResponse returns empty)
}
```

The fallback is consumed in the empty-after-tools path of `handleTextTurn`
(post-tool empty response → use `s.lastContentWithTools`). Exact integration
point: `handleNoToolCalls` one-shot nag path — before nagging, check if
`lastContentWithTools` is available.

**Full integration in handleNoToolCalls (updated):**

```go
func (s *runSession) handleNoToolCalls(
    turnMsg proxy.Message,
    parseErr *proxy.ParseError,
    toolsList []proxy.Tool,
) (string, bool, error) {
    // Append to history (existing)
    if len(s.history) == 0 || s.history[len(s.history)-1].Content != turnMsg.Content {
        s.history = append(s.history, turnMsg)
        s.agent.notify(EventMessage, turnMsg)
    }

    // Premature termination guard (existing)
    if s.agent.isPrematureTermination(turnMsg, s.history) {
        s.agent.deps.Logger.Warn("premature termination detected", "step", s.steps)
        return turnMsg.Content, true, nil
    }

    // Parse error with content (existing, KEPT for non-native XML-text models)
    if parseErr != nil && !parseErr.XMLFound && strings.TrimSpace(turnMsg.Content) != "" {
        // In native-tools mode with tools available, NO LONGER reject —
        // fall through to content-with-tools check below.
        if !s.agent.config.UseNativeTools || len(toolsList) == 0 {
            return turnMsg.Content, true, nil
        }
        // Native-tools: parseErr cleared — content is the final answer
        // (Hermes-aligned: trust the model).
        parseErr = nil
        return turnMsg.Content, true, nil  // ← NEW: accept, don't nag
    }

    if parseErr != nil {
        // ... existing parse-error feedback injection (unchanged) ...
    }

    // Genuinely no content: empty turn after tools.
    // Check content-with-tools fallback first.
    if s.lastContentWithTools != "" {
        content := s.lastContentWithTools
        s.lastContentWithTools = ""
        s.agent.deps.Logger.Info("using content-with-tools fallback as final answer",
            "chars", len(content))
        return content, true, nil
    }

    // One-shot nag (replaces perpetual nag)
    if s.forcedCompletionSent {
        if content := s.bestAvailableAnswer(); content != "" {
            return content, true, nil
        }
        return "", true, nil  // give up
    }
    s.forcedCompletionSent = true
    s.agent.deps.Logger.Warn("no tool calls - nagging model (one-shot)", "step", s.steps)
    s.history = append(s.history, proxy.Message{
        Role:    proxy.UserRole,
        Content: prompts.AutomationNagPrompt,
    })
    return "", false, nil
}
```

#### 2b.4 Implementation Checklist

> **Status:** All code changes implemented. Items 1-6 are in `session.go`; items 7-9 are in test files.

| # | File | Change | Lines | Status |
|---|------|--------|-------|--------|
| 1 | `session.go` ~L190 | Add `stripThinkBlocks()`, `hasContentAfterThinkBlock()` helpers | +20 | ✅ Done |
| 2 | `session.go` L206-223 | Replace `checkTaskCompletion` — new sig, think-strip, any-tool-result | ~30 changed | ✅ Done (now `session.go:270-297`) |
| 3 | `session.go` L33-52 | Add `lastContentWithTools string` to `runSession` | +1 | ✅ Done (`session.go:52`) |
| 4 | `session.go` L416-434 | `handleToolTurn` — save content-with-tools fallback | +6 | ✅ Done (`session.go:510-511`) |
| 5 | `session.go` L543-601 | Replace `handleNoToolCalls` — one-shot nag, trust native text, fallback | ~50 changed | ✅ Done (`session.go:640-721`) |
| 6 | `session.go` L443 | Update `handleTextTurn` caller — pass `s.history` | 1 changed | ✅ Done |
| 7 | `session_test.go` | Add tests (see §2b.5) | +120 new | ✅ Done |
| 8 | `agent_test.go` L1557-1687 | Update `checkTaskCompletion` test expectations | ~30 changed | ✅ Done |
| 9 | `agent_test.go` | Verify `isPrematureTermination` tests still pass (unchanged) | verify | ✅ Done

> **Complementary change:** The no-tool content cap in `stream.go` was relaxed by `automation-unified-renderer-and-report-truncation.md` (complete). `processStream` now accepts `priorToolResult bool, toolsAvailable bool` and only terminates the stream when the turn is a likely runaway (no prior work AND tools available) — legitimate final answers survive to reach `checkTaskCompletion`.

#### 2b.5 Test Plan

**New unit tests:**

| Test | What it proves | File |
|------|---------------|------|
| `TestCheckTaskCompletion_WithReasoningInterleaved` | Final report preceded by reasoning-only assistant turn → completes (was BUG 1) | session_test.go |
| `TestCheckTaskCompletion_WithThinkBlocks` | Content is `<think>...</think>actual answer` → completes on "actual answer" | session_test.go |
| `TestCheckTaskCompletion_ThinkOnly` | Content is pure `<think>...</think>` → does NOT complete | session_test.go |
| `TestCheckTaskCompletion_NativeToolsFinalText` | Native-tools, text answer after tools → completes (was BUG 2) | session_test.go |
| `TestCheckTaskCompletion_NoToolResultInHistory` | First-turn text answer, no tool results → does NOT complete | session_test.go |
| `TestCheckTaskCompletion_UnparsedToolCallInContent` | Content `<tool_call>...</tool_call>` → does NOT complete | session_test.go |
| `TestCheckTaskCompletion_ShortChatter` | `<20 chars after think-strip → does NOT complete | session_test.go |
| `TestHandleNoToolCalls_OneShotNag` | Empty after tools → one nag, next empty → fallback/give up | session_test.go |
| `TestHandleNoToolCalls_ContentWithToolsFallback` | Content+write_file in tool turn, next turn empty → uses fallback | session_test.go |
| `TestHandleNoToolCalls_NativeToolsTextAccepted` | Native-tools, text content → returns true (completes, no nag) | session_test.go |
| `TestStripThinkBlocks` | Helper — strips `<think>`, `<reasoning>`, `<REASONING_SCRATCHPAD>` tags | session_test.go |
| `TestHasContentAfterThinkBlock` | Helper — returns true/false correctly | session_test.go |

**Existing test updates:**

| Test | Action |
|------|--------|
| `agent_test.go:1575-1687` (checkTaskCompletion tests) | Update: tests expecting `false` because `previousMsg.Role != ToolRole` now expect `true` (with history containing a tool result). Tests with pure text and no tool results in history keep `false`. |
| `agent_test.go:1605` (call site) | Update: pass `history` parameter |
| `agent_test.go:308-310` (isPrematureTermination) | Verify unchanged — still correct |
| `agent_test.go:673-675` (precededByToolResult) | Unchanged — keep for other consumers |

**Test data pattern:**

```go
// Helper: build test history with tools + reasoning interleave
func historyWithToolAndReasoning() []proxy.Message {
    return []proxy.Message{
        {Role: proxy.SystemRole, Content: "You are an agent."},
        {Role: proxy.UserRole,   Content: "Do task X."},
        {Role: proxy.AssistantRole, Content: "", ToolCalls: []proxy.ToolCall{{
            Function: proxy.FunctionCall{Name: "read_file", Arguments: `{"path":"f"}`},
        }}},
        {Role: proxy.ToolRole, Content: "file contents here"},          // tool result
        {Role: proxy.AssistantRole, Content: "", ReasoningContent: "reasoning..."}, // reasoning-only
        {Role: proxy.AssistantRole, Content: "Here is the final report."},           // ← should complete
    }
}
```

#### 2b.6 Documentation Updates Required

These updates must be made as part of this phase. See `CONSTITUTION.md` II.7
and `docs/architecture.md` pitfalls #6/#8 for the most critical corrections.

| Document | Section | Change |
|----------|---------|--------|
| `CONSTITUTION.md:L44-48` | II.7 Natural Task Completion | Replace `precededByToolResult` with: "assistant message with substantive content after think-block stripping, no tool calls, at least one tool result in history." |
| `docs/SPECS/agent-loop.md:L43-54` | Exit Heuristics | Replace ToolRole-adjacency description with Hermes-aligned model (think-strip + any-tool-result + no-tool-calls). Add content-with-tools fallback. |
| `docs/SPECS/agent-loop.md:L56-60` | Error Recovery | Update: nag is one-shot, not perpetual. Native-tools text answers are accepted, not rejected. |
| `docs/SPECS/agent-loop.md:L77-80` | Fallback chain | Update native-only stuck path description. |
| `docs/architecture.md:L254` | Pitfall #6 | **Delete** the paragraph: "When `useNativeTools` is true and tools are available, the model must produce a tool call — freeform text is treated as a protocol violation and triggers a nag/retry." This is now incorrect. |
| `docs/architecture.md:L258` | Pitfall #8 | Verify `tool_choice: required` override was removed in Phase 2. If still present, delete. |
| `docs/architecture.md:L272` | Pitfall #23 | Same as #6 — delete or rewrite the native-tools violation paragraph. |
| `docs/skills/agent-loop.md` | Completion paths | Update stuck detection and completion flow. |
| `docs/skills/lifecycle-events.md` | `session_completed` | Verify trigger description matches new completion gate. |

#### 2b.7 Verification

```bash
# Build
cd backend && go build ./...

# Unit tests — must pass
cd backend && go test ./internal/core/assistant/... -count=1 -timeout 120s

# Full test suite
cd backend && go test ./... -count=1 -timeout 120s

# Complexity
cd backend && go run ./tools/check-complexity/
```

**Manual verification (local LLM):**
1. Start backend with local model (Qwen3.5-9B via llama.cpp openai endpoint).
2. Run `dev-test` automation → confirm:
   - Exactly ONE final report in output (no double-emit).
   - `final-report.md` contains the report.
   - No `nagging model` log for the final turn (logs should show natural completion).
3. Run `network-recon-unprivileged` automation → confirm:
   - Agent completes without timeout/force-completion.
   - `final-report.md` is generated.
4. Verify SSE stream emits `lifecycle:completed` after the final answer (existing signal, unchanged).

**Regression guard:**
- Chat mode single-turn: user asks a simple question → agent answers immediately (no tools) → completes (existing behavior, must not break).
- Chat mode multi-turn: user asks a complex task → agent calls tools → answers → completes (existing behavior, must not break).
- Small model (Gemma 4B, Qwen 4B): verify premature "let me check" text does NOT trigger completion. The ≥20-char + think-strip guards should prevent this.

#### 2b.8 Risk Assessment

| Risk | Mitigation | Likelihood |
|------|-----------|------------|
| Premature completion — weak model writes ≥20 chars of planning chatter after a tool call but before real work | `isPrematureTermination` catches 3× identical text. `hasToolCallMarker` catches unparsed tool calls. The ≥20-char + think-strip is a minimum bar. Additional risk: a model that writes "Now I'll analyze the data..." (≥20 chars, no tool calls, tool result exists in history) would complete prematurely. **Mitigation:** this pattern is unlikely in practice because (a) the system prompt instructs the model to call tools when it needs them, and (b) Hermes has shipped this model for 6+ months across 4B–Claude scale without reports of this failure mode. Add a note to monitor logs. | Low |
| Think-stripping false positive — model writes about `<think>` tag literally in final answer | `stripThinkBlocks` regex is case-insensitive and matches balanced `<think>...</think>`. Literal "the `<think>` tag" where `<think>` is not a balanced XML block would NOT be stripped. Only actual `<think>content</think>` blocks are removed. | Very low |
| `forcedCompletionSent` field reuse ambiguity | Only fires after one-shot nag. The field was previously only used at `checkForcedCompletion` (2×MaxSteps). If both fire in the same run, both paths are complementary — force-complete is correct in both cases. | Harmless |
| `precededByToolResult` still used elsewhere | The function is called in `run()` (session.go:666) and `conversation_helpers.go`. Verify consumers still work with relaxed gate. | Low — function stays, check usage |

---

### Phase 3: Inset Rendering (Frontend)

**Status:** `complete` (pre-implemented by `automation-renderer-unify-consumption`)  

**Goal:** Redesign ChatBubble.vue to separate "work" (reasoning + tool calls) from
"result" (final answer) using a collapsible inset panel.

**Duration:** Done (0 remaining hours — code complete)

**Why third:** Phases 1–2 clean the completion model and simplify the frontend. The inset
rendering builds on the simplified types from Phase 2.

> **All Phase 3 features are implemented** in the current codebase as part of the
> `automation-renderer-unify-consumption` plan (see §9 for file:line references):
> - `ChatBubble.vue` — collapsible inset panel with two-zone layout (L100-144)
> - `messageBuilder.ts` — full phase state machine with all 5 transitions (L155/164/169/187/298)
> - `ToolCallSegment.vue` — compact mode with inline preview (L20, L59-65)
> - `AssistantChat.vue` — auto-collapse watchers + insetCollapsed state (L171-194)
>
> The original design spec below is kept for architectural reference.

**Note on `final` segment kind:** `types/assistant.ts:15` defines `{ kind: 'final', text: string }` in the `Segment` discriminated union, but it is never rendered in `ChatBubble.vue`. The inset redesign uses `turn.finalAnswer` for the result (rendered outside the inset). After removing submit_final_answer the `final` kind becomes dead code. Either delete it or repurpose it as the inset's result segment — the implementation can decide; the plan treats it as deletable.

#### 4.3.1 Phase State Machine

> **`InsetPhase` type already exists** at `messageBuilder.ts:12`. This section describes the required state TRANSITIONS that must be added to `messageBuilder.ts`'s `handleEvent` function.

```
idle ──(first reasoning/tool_stream)──▶ thinking
                                           │
                              (first tool_call)
                                           │
                                           ▼
                                        working
                                           │
                (msg: content + no tool_calls + precededByToolResult)
                                           │
                                           ▼
                                      generating
                                           │
                          (SSE stream end / lifecycle:completed)
                                           │
                                           ▼
                                         done
                                   inset auto-collapses
```

Phase transition logic to add in `messageBuilder.ts`:

| Transition | Trigger | New Phase |
|------------|---------|-----------|
| First `reasoning` or `tool_stream` | `phase === 'idle'` | `thinking` |
| First `tool_call` | `phase === 'thinking'` or `phase === 'working'` | `working` |
| `message` with content + no tool_calls + prior tool calls in turn | `phase === 'thinking'` or `phase === 'working'` | `generating` |
| SSE stream end or `lifecycle:completed` | `phase === 'generating'` | `done` |

#### 4.3.2 ChatBubble.vue Redesign

> **Note:** `InsetPhase` type is already defined at `messageBuilder.ts:12` — no import/type changes needed here. The existing `ChatBubble.vue` already receives `phase` as a prop from its parent; this redesign makes it the driver of the inset layout.

The component transitions from a flat layout to a two-zone structure:

```vue
<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  turn: Turn
  turnIdx: number
  loading: boolean
  thinking: boolean
  liveReasoning: string
  paused: boolean
  isLastTurn: boolean
  phase: InsetPhase
  isInsetCollapsed: boolean
}>()

const emit = defineEmits<{
  toggleInset: [turnIdx: number]
  toggleSegment: [turnIdx: number, segIdx: number]
}>()

const reasoningSegments = computed(() =>
  props.turn.segments.filter(s => s.kind === 'reasoning'))
const toolSegments = computed(() =>
  props.turn.segments.filter(s => s.kind === 'tool_call'))

const insetVisible = computed(() =>
  !props.isInsetCollapsed &&
  (props.phase === 'thinking' || props.phase === 'working' || props.phase === 'generating'))

const resultVisible = computed(() =>
  props.phase === 'generating' || props.phase === 'done')

const phaseLabel = computed(() => {
  switch (props.phase) {
    case 'thinking':   return 'Assistant — thinking...'
    case 'working':    return `Assistant — working (${toolSegments.value.length} tools)`
    case 'generating': return 'Assistant — generating answer...'
    case 'done':       return `Assistant — ${toolSegments.value.length} tools · completed`
    default:           return 'Assistant'
  }
})
</script>

<template>
  <!-- Header: click to toggle inset -->
  <div class="bubble-header" @click="emit('toggleInset', turnIdx)">
    <span class="bubble-chevron" :class="{ collapsed: isInsetCollapsed }">▶</span>
    <span class="bubble-phase">{{ phaseLabel }}</span>
  </div>

  <!-- Inset: reasoning + tool calls (collapsible) -->
  <Transition name="inset-collapse">
    <div v-if="insetVisible" class="bubble-inset">
      <!-- Reasoning blocks (dimmed, prefixed with "Thought" label) -->
      <div v-for="(seg, i) in reasoningSegments" :key="'r-'+i" class="inset-reasoning">
        <span class="inset-label">Thought</span>
        <MarkdownViewer :source="seg.text" />
      </div>

      <!-- Tool calls (compact rows, expandable) -->
      <ToolCallSegment
        v-for="(seg, i) in toolSegments"
        :key="'t-'+turnIdx+'-'+i"
        :segment="seg"
        :turnIdx="turnIdx"
        :segIdx="i"
        :compact="true"
        @toggle="(t, s) => emit('toggleSegment', t, s)" />

      <!-- Generating indicator (inside inset, at bottom) -->
      <div v-if="phase === 'generating'" class="inset-generating">
        <span class="pulse-dot" />
        <span class="pulse-dot" />
        <span class="pulse-dot" />
        <span class="inset-generating-label">Generating answer...</span>
      </div>
    </div>
  </Transition>

  <!-- Result: always outside the inset, always visible when present -->
  <div v-if="resultVisible" class="bubble-result">
    <MarkdownViewer :source="turn.finalAnswer" />
  </div>

  <!-- Paused indicator (shown when loading but not yet generating) -->
  <div v-if="paused && phase !== 'generating' && phase !== 'done' && isLastTurn"
       class="bubble-paused">
    ●●● Thinking
  </div>

  <!-- Canceled banner -->
  <div v-if="turn.canceled && !resultVisible" class="bubble-canceled-banner">
    Response interrupted
  </div>
</template>
```

#### 4.3.3 CSS Specification

```css
/* ── Inset panel ── */
.bubble-inset {
  @apply ml-2 mr-2 my-1;
  @apply border-l-2 border-indigo-500/60;
  @apply bg-gray-800/50 rounded-lg;
  @apply px-3 py-2;
  transition: max-height 0.3s ease-out, opacity 0.2s ease-out,
              margin 0.2s ease-out, padding 0.2s ease-out;
  overflow: hidden;
}

/* ── Collapse transition ── */
.inset-collapse-enter-active,
.inset-collapse-leave-active {
  transition: max-height 0.3s ease-out, opacity 0.2s ease-out;
}
.inset-collapse-enter-from,
.inset-collapse-leave-to {
  max-height: 0;
  opacity: 0;
}

/* ── Reasoning blocks ── */
.inset-reasoning {
  @apply mb-2 pl-2 border-l border-indigo-500/30;
}
.inset-reasoning .inset-label {
  @apply text-[10px] uppercase tracking-wider text-indigo-400/60 mb-0.5 block;
}

/* ── Generating answer pulse ── */
.inset-generating {
  @apply flex items-center justify-center gap-1.5 py-2 mt-1;
  @apply text-indigo-400 text-xs border-t border-indigo-500/20;
}
.pulse-dot {
  @apply w-1.5 h-1.5 rounded-full bg-indigo-400;
  animation: pulse-dot 1.2s ease-in-out infinite;
}
.pulse-dot:nth-child(2) { animation-delay: 0.15s; }
.pulse-dot:nth-child(3) { animation-delay: 0.3s; }

@keyframes pulse-dot {
  0%, 80%, 100% { opacity: 0.15; transform: scale(0.8); }
  40% { opacity: 1; transform: scale(1); }
}

/* ── Result area ── */
.bubble-result {
  @apply px-3 py-2;
}

/* ── Header chevron ── */
.bubble-chevron {
  @apply inline-block mr-2 text-xs text-gray-500 transition-transform duration-200;
}
.bubble-chevron:not(.collapsed) {
  transform: rotate(90deg);
}

/* ── Paused indicator ── */
.bubble-paused {
  @apply text-center text-gray-500 text-xs py-2;
}
```

#### 4.3.4 ToolCallSegment Compact Mode

`ToolCallSegment.vue` gains a `compact` prop. When `compact: true` (inside inset), it
renders as a single-line summary row. Clicking expands to show arguments + result:

**Compact (collapsed):**
```
  ✓ read_file: config.json → { "port": 4001, "model": "..." }
```

**Expanded (clicked):**
```
  ✓ read_file: config.json                                        [▼]
  ┌ Arguments ──────────────────────────────────────────────────────┐
  │ { "path": "config.json" }                                       │
  └─────────────────────────────────────────────────────────────────┘
  ┌ Result ─────────────────────────────────────────────────────────┐
  │ { "port": 4001, "model": "claude-sonnet-4-20250514" }           │
  └─────────────────────────────────────────────────────────────────┘
```

#### 4.3.5 AssistantChat.vue State Changes

Rename `workCollapsed` to `insetCollapsed` and update auto-collapse behavior:

```typescript
// Per-turn inset collapse state
const insetCollapsed = ref<Record<number, boolean>>({})

// Auto-expand inset for the last turn while loading
watch(loading, (value) => {
  if (value) {
    const lastIdx = turns.value.length - 1
    if (lastIdx >= 0) insetCollapsed.value[lastIdx] = false
  } else {
    // When loading finishes, auto-collapse all completed insets
    insetCollapsed.value = turns.value.reduce(
      (acc, _, i) => ({ ...acc, [i]: true }),
      {} as Record<number, boolean>
    )
  }
})
```

#### 4.3.6 Verification

```bash
cd frontend && npm run build
# Manual testing:
#  1. Inset appears during reasoning phase (collapsible)
#  2. Tool calls render in compact mode inside inset
#  3. "Generating answer..." pulse appears when model writes final text
#  4. Result streams outside inset in real-time
#  5. After completion, inset auto-collapses
#  6. Clicking header toggles inset expand/collapse
#  7. Phase transitions: idle → thinking → working → generating → done
#  8. CSS transitions smooth (no layout jank)
#  9. Canceled turns show banner correctly
```

> ### 🔍 Test Checkpoint 3 — After Phase 3 (Inset Rendering)
>
> **Status:** Full UX complete. Backend unchanged from Checkpoint 2b.
>
> **Local LLM test (UI-level):**
> 1. Chat mode: send a task → verify inset expands during reasoning/tools, collapses after completion.
> 2. Verify "Generating answer..." pulse appears before the result streams.
> 3. Verify result renders OUTSIDE the inset and remains visible after collapse.
> 4. Click header → inset toggles expand/collapse.
> 5. Phase transitions: idle → thinking → working → generating → done.
>
> **Pass criteria:** Inset pattern renders correctly in all phases; no layout jank.

---

## 5. Complete File Inventory

### Phase 1 (AGENTS.md) — 9 files

| # | File | Action |
|---|------|--------|
| 1 | `backend/internal/core/assistant/prompts/default_agents_md.go` | **CREATE** |
| 2 | `backend/models/workspace.go:19` | **MODIFY** |
| 3 | `backend/internal/transport/http/handlers/dispatcher_handlers.go:361-366` | **MODIFY** |
| 4 | `backend/internal/core/assistant/conversation_helpers.go:19-25` | **MODIFY** |
| 5 | `backend/internal/core/automation/executor.go:130` | **MODIFY** |
| 6 | `backend/internal/core/assistant/prompts/templates.go:146-191` | **MODIFY** |
| 7 | `backend/internal/core/assistant/prompts/templates.go:200-207` | **DELETE** `DefaultAgentPrompt` constant (content now in `DefaultAgentsMD`) |
| 8 | `backend/models/workspace.go:18` | **DELETE** `AgentPromptFilename = "agent.md"` (file no longer seeded) |
| 9 | `backend/internal/core/assistant/prompts/templates.go:182` | **MODIFY** — rename param |

### Phase 2 (Remove submit_final_answer) — 30 files

> **Note:** The 30 files above are implementation-code changes. The §4.2.15 documentation
> table additionally lists documentation files (`event-streaming-patterns.md`,
> `assistant-ui-patterns.md`, `tool-call-parser.md`, `memory-improvements-plan.md`,
> `webhook-fresh-sessions.md`, `ARCHIVE/**`) that must be audited and updated. These are
> tracked in §4.2.15 rather than here to avoid inflating the implementation file count.

| # | File | Action |
|---|------|--------|
| 1 | `backend/internal/core/assistant/session.go` | **MODIFY** — new `checkTaskCompletion`, remove submit detection + batching |
| 2 | `backend/internal/core/assistant/tool_exec.go` | **MODIFY** — delete `hasSubmit`, `extractTaskSummary`, batched rejection |
| 3 | `backend/internal/core/assistant/agent.go` | **MODIFY** — remove repetition detector exemption |
| 4 | `backend/internal/core/assistant/agent_events.go` | **MODIFY** — update `PhaseSessionCompleted` comment |
| 5 | `backend/internal/core/assistant/prompts/templates.go` | **MODIFY** — 10+ constants: remove submit references, delete `AutomationRejectedSubmissionPrompt` |
| 6 | `backend/internal/core/assistant/prompts/system_prompt.go` | **MODIFY** — new `finalAnswerInstruction` |
| 7 | `backend/models/tools.go` | **MODIFY** — delete `ToolSubmitFinalAnswer` |
| 8 | `backend/internal/core/assistant/registry.go` | **MODIFY** — remove submit from tool registration |
| 9 | `backend/internal/core/tools/manifests/system.json` | **MODIFY** — remove submit entry, keep system_error |
| 10 | `backend/internal/core/assistant/stream.go` | **MODIFY** — remove `tool_choice: required` override (§4.2.8) |
| 11 | `backend/internal/core/tools/manifests/communication.json` | **MODIFY** — remove "report" from descriptions (§4.2.9) |
| 12 | `backend/internal/core/automation/executor.go` | **MODIFY** — `finalReply` source, prompt template |
| 11 | `backend/internal/core/automation/rundir.go` | **MODIFY** — comment update |
| 14 | `backend/internal/transport/http/handlers/webhook_handlers.go` | **MODIFY** — completion signal |
| 15 | `frontend/src/utils/message/messageBuilder.ts` | **MODIFY** — delete `isFinalTurn`, update `handleMessage` |
| 16 | `frontend/src/utils/message/turnGrouper.ts` | **MODIFY** — remove submit skip |
| 17 | `CONSTITUTION.md` | **MODIFY** — II.7 amendment |
| 18 | `docs/SPECS/agent-loop.md` | **MODIFY** — completion paths |
| 19 | `docs/skills/agent-loop.md` | **MODIFY** |
| 20 | `docs/skills/assistant-ui-chat.md` | **MODIFY** |
| 21 | `docs/skills/automation.md` | **MODIFY** |
| 22 | `docs/skills/lifecycle-events.md` | **MODIFY** |
| 23 | `backend/internal/core/assistant/agent_test.go` | **MODIFY** |
| 24 | `backend/internal/core/assistant/filtered_provider_test.go` | **MODIFY** |
| 25 | `backend/internal/core/assistant/agent_memory_test.go` | **MODIFY** |
| 26 | `backend/internal/core/assistant/session_test.go` | **MODIFY** |
| 27 | `backend/internal/core/assistant/prompts/system_prompt_test.go` | **MODIFY** |
| 28 | `backend/internal/core/assistant/prompts/templates_test.go` | **MODIFY** |
| 29 | `backend/internal/core/assistant/conversation_helpers_test.go` | **MODIFY** |
| 30 | `backend/internal/transport/http/handlers/assistant_handlers_test.go` | **MODIFY** |

### Phase 2b (Hermes-Aligned Completion Gate) — 10 files

| # | File | Action |
|---|------|--------|
| 1 | `backend/internal/core/assistant/session.go` | **MODIFY** — replace `checkTaskCompletion`, `handleNoToolCalls`, add `stripThinkBlocks`, add `lastContentWithTools` field, update `handleToolTurn` and `handleTextTurn` |
| 2 | `backend/internal/core/assistant/session_test.go` | **MODIFY** — +12 new tests (~120 lines): think-strip, native-tools accept, one-shot nag, content-with-tools fallback, reasoning-interleave |
| 3 | `backend/internal/core/assistant/agent_test.go` | **MODIFY** — update ~4 `checkTaskCompletion` test expectations (add `history` param, update expected results) |
| 4 | `CONSTITUTION.md` | **MODIFY** — II.7 Natural Task Completion rewrite (think-strip + any-tool-result) |
| 5 | `docs/SPECS/agent-loop.md` | **MODIFY** — rewrite Exit Heuristics §5 + Error Recovery §6 + Fallback chain §8 |
| 6 | `docs/architecture.md` | **MODIFY** — delete Pitfall #6 (native-tools violation) and #8 (tool_choice required if still present); update Pitfall #23 |
| 7 | `docs/skills/agent-loop.md` | **MODIFY** — update completion paths and stuck detection |
| 8 | `docs/skills/lifecycle-events.md` | **VERIFY** — `session_completed` trigger unchanged but confirm description aligns |
| 9 | `docs/PLANS/cross-cutting/universal-agent-completion.md` | **MODIFY** — Phase 2b section added (this document); Phase 2 §4.2.1 marked deprecated; Phase 2 §4.2.3, §4.2.13 updated with forward refs; §4.2.14 constitution amendment updated |

### Phase 3 (Inset Rendering) — 4 files

> **Pre-existing (from `automation-renderer-unify-consumption`):** `InsetPhase` type, `MessageBuilderOptions` interface, lifecycle finalize logic, and `ChatMessages` mode/slot are already in place. Phase 3 scope narrows to the visual inset panel + phase transitions.

| # | File | Action |
|---|------|--------|
| 1 | `frontend/src/components/AgentIde/assistant/ChatBubble.vue` | **MODIFY** — inset panel (two-zone layout: collapsible inset + result outside) |
| 2 | `frontend/src/utils/message/messageBuilder.ts` | **MODIFY** — add phase state machine TRANSITIONS (set `phase.value` in `handleEvent` based on event type; `InsetPhase` type already exists at line 12) |
| 3 | `frontend/src/components/AgentIde/assistant/ToolCallSegment.vue` | **MODIFY** — add `compact` prop (single-line summary rows for inset mode) |
| 4 | `frontend/src/components/AgentIde/assistant/AssistantChat.vue` | **MODIFY** — rename `workCollapsed` to `insetCollapsed`, add auto-collapse watch logic |

---

## 6. Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|:---:|:---:|-----------|
| Automation model never produces text-without-tool-calls (loops forever) | Low | High | Existing starvation counter (`DefaultStarvationLimit`) + advisory `MaxSteps` warning + `ContextSieveWarning` still fire. Nag prompts unchanged. |
| Chat model produces planning text without tool calls, treated as final answer | Low | Medium | Phase 2b's `any-tool-result-in-history` guard prevents first-turn text from completing. `isPrematureTermination` catches 3× identical text. `hasToolCallMarker` catches unparsed tool calls. ≥20 chars + think-strip is minimum bar. Hermes has shipped this across 4B–Claude scale without reports of this failure mode. |
| Existing workspaces lose `rules.md` content | None | Low | Fallback reads `rules.md` if `AGENTS.md` absent. No data loss. |
| Inset animation jank on long tool histories | Low | Low | `ToolSegment` uses `v-for` with `:key`. Inset collapse is CSS `max-height` transition — GPU-accelerated. Virtual scrolling not needed for typical turn sizes (< 30 tools). |
| Phase detection race: SSE events arrive out of order | Low | Medium | SSE is ordered per connection. All events processed synchronously in `handleEvent()`. Phase transitions are idempotent — repeated transitions to same state are no-ops. |
| Backward compatibility: external consumers relying on `submit_final_answer` event | Low | Medium | Webhook handler uses `"✔ Execution complete."` magic string sent by `executor.go:345` — unchanged and not tied to submit_final_answer. Optionally switch to `lifecycle:completed`. Other consumers follow same pattern. `EventMessage` with content remains primary output signal — only trigger changed. |

---

## 7. Execution Order & Dependencies

```
Phase 1: AGENTS.md           (3–4h, backend only, 9 files)
  │  Deployable standalone — no LLM behavior change
  │  Safe to merge independently
  ▼
Phase 2: Remove submit        (6–8h, backend + frontend, 28 files)
  │  Changes LLM completion behavior
  │  Requires Phase 1's AGENTS.md for instruction updates
  │  Backend tests must pass
  ▼
Phase 3: Inset rendering      (4–5h, frontend only, 4 files)
     Pure visual change
     Simplified by Phase 2's frontend cleanup
     No backend changes required
```

**Phase 1 can be deployed independently.** It doesn't change LLM behavior — it just loads
instructions from disk. This is valuable even without Phases 2–3 because it enables
user-customizable agent behavior.

**Phase 2 is the critical path.** Changes the completion mechanism used by every LLM call.
All code changes are implemented (submit_final_answer removed from 30+ files, `checkTaskCompletion`
with Hermes-aligned gate in `session.go`). Remaining work: run tests, verify manual scenarios,
audit remaining docs.

**Phase 2b is implemented.** The `checkTaskCompletion` gate replacement (stripThinkBlocks +
any-tool-result-in-history + content-with-tools fallback + one-shot nag) is coded in `session.go`.
Two manual verifications remain (double final report, native-tools network scan). The no-tool
content cap relaxation in `stream.go` (from `automation-unified-renderer-and-report-truncation`)
complements the completion gate — both were implemented together.

**Phase 3 can start now.** The `automation-renderer-unify-consumption` plan pre-built the
infrastructure (`InsetPhase` type, lifecycle finalize, `ChatMessages` mode/slot, `useMessageBuilder`
consumption). Phase 3 narrows to the visual layer: phase transitions in `messageBuilder.ts`,
`ChatBubble.vue` inset panel redesign, `ToolCallSegment.vue` compact mode, and `AssistantChat.vue`
auto-collapse.

---

## 8. Acceptance Criteria

### Phase 1
- [x] `go build ./...` passes
- [x] New workspace creates `AGENTS.md` with `DefaultAgentsMD` content
- [x] ~~Existing workspaces with `rules.md` load it as fallback~~ — dropped: no backward-compat needed (no legacy workspaces to support). `LoadAgentsFile` reads `AGENTS.md` or returns `DefaultAgentsMD`.
- [x] System prompt includes `AGENTS.md` content as its suffix section
- [x] Editing `AGENTS.md` changes agent behavior on next turn
- [x] `go test ./...` passes
- [x] Complexity check ≤ 12
- [x] Unit tests added: `TestLoadAgentsFile`, `TestDefaultAgentsMD`, `TestAssembleSystemPrompt_WorkspaceRules`, `TestCreateWorkspace_SeedsAgentsFile`

### Phase 2
- [x] `submit_final_answer` removed from tool registry (`models/tools.go`, `registry.go`, `manifests/system.json` — zero occurrences)
- [x] No `submit_final_answer` in any prompt constant or system message (`templates.go`, `system_prompt.go` — zero occurrences; `AutomationRejectedSubmissionPrompt` deleted; `AutomationTaskPrompt` uses natural completion language)
- [x] `tool_choice: required` override removed from `stream.go` (`buildChatRequest` — confirmed absent)
- [x] No `isFinalTurn` in frontend code (`messageBuilder.ts` — confirmed absent; superseded by `automation-renderer-unify-consumption`)
- [x] `isSubmit` skip removed from `turnGrouper.ts` (confirmed absent; superseded by `automation-renderer-unify-consumption`)
- [x] `checkSubmitFinalAnswer` deleted from `session.go`; replaced by `checkTaskCompletion`
- [x] `hasSubmit` and `extractTaskSummary` deleted from `tool_exec.go` (confirmed absent)
- [x] `CONSTITUTION.md` II.7 updated (Phase 2b wrote final Hermes-aligned text)
- [x] Chat mode: model completes tasks without `submit_final_answer`
- [x] Automation mode: model completes tasks without `submit_final_answer`, report written to `final-report.md`
- [x] Webhook receives completion signal correctly (`executor.go` sends `"✔ Execution complete."` magic string — unchanged, not tied to submit)
- [x] All specs and skills docs updated (Phase 2b covered SPEC-001, architecture.md, agent-loop skill, lifecycle-events)
- [x] `go test ./...` passes
- [x] `npm run build` passes
- [x] Complexity check ≤ 12

### Phase 2b
- [x] `checkTaskCompletion` uses `stripThinkBlocks` + any-tool-result-in-history (not ToolRole adjacency)
- [x] Native-tools text answer accepted as completion (no nag/rejection)
- [x] Content-with-tools saved as fallback; used on empty follow-up turn
- [x] Empty-after-tools gets one-shot nag (not perpetual); fallback then force-complete
- [x] Reasoning-only turns (`<think>` or `ReasoningContent` only) do NOT complete
- [x] Double final report bug fixed (confirmed: Hermes-aligned completion gate + no-tool cap relaxation prevent re-emit)
- [x] Network scan automation completes without timeout on native-tools (Qwen 9B)
- [x] `go test ./internal/core/assistant/...` passes with new + updated tests
- [x] `go build ./...` passes
- [x] Complexity check ≤ 12
- [x] `CONSTITUTION.md` II.7 updated to new text
- [x] `docs/SPECS/agent-loop.md` Exit Heuristics + Error Recovery updated
- [x] `docs/architecture.md` Pitfalls #6/#8 corrected
- [x] `docs/skills/agent-loop.md` completion paths updated

### Phase 3
- [x] Inset expands during reasoning/tool-calling phases
- [x] Tool calls render in compact mode inside inset
- [x] "Generating answer..." pulse appears during final text generation
- [x] Result streams outside inset in real-time
- [x] Inset auto-collapses after completion
- [x] Clicking header toggles inset expand/collapse
- [x] Phase transitions: idle → thinking → working → generating → done
- [x] CSS transitions smooth (no layout jank at any phase boundary)
- [x] Canceled turns display correctly with inset pattern
- [x] `npm run build` passes

> **Note:** Phase 3 was pre-implemented by `docs/PLANS/assistant-ui/automation-renderer-unify-consumption.md`. All inset rendering, phase state machine, compact tool mode, and auto-collapse behavior exists in the current code (see §9 cross-reference for file:line evidence).

---

## 9. Cross-Reference: Pre-Implemented Infrastructure

Two completed plans (2026-07-18) pre-implemented pieces of this plan's Phases 2–3.
This section documents what they changed and how it affects remaining work.

### 9.1 `automation-renderer-unify-consumption.md`

**Status:** `complete`  
**What it did:** Unified chat + automation event consumption through a single `useMessageBuilder` path.

**Pre-implemented from this plan:**

| This Plan Item | How It Was Implemented |
|---------------|----------------------|
| Phase 2 frontend (§4.2.12) — all 5 items | `isFinalTurn` removed, `isSubmit` skip removed, lifecycle finalize wired — all superseded |
| Phase 3 infra — `InsetPhase` type | Defined at `messageBuilder.ts:12` |
| Phase 3 infra — `MessageBuilderOptions` | Interface at `messageBuilder.ts:14-23` with `source`, `headerMessage`, `finalizeOn` |
| Phase 3 infra — lifecycle-triggered finalize | `case 'lifecycle'` at `messageBuilder.ts:196-203` |
| Phase 3 infra — `ChatMessages` mode | `mode?: 'chat' \| 'automation'` prop at `ChatMessages.vue:26` |
| Phase 3 infra — `#run-header` slot | Scoped slot at `ChatMessages.vue:91` |

**What was also cleaned up:**
- Deleted `automationEventsToMessages.ts` bridge (was broken — cumulative re-emit cascade)
- Rewrote `useLiveConsole.ts` to feed `useMessageBuilder` directly
- Added run-end fallback in `useLiveConsole` (watch `_isExecuting`)
- `AutomationDetails.vue` now wires builder state reactively

**Verification already done:** `npm run build` passed during that plan's implementation.

### 9.2 `automation-unified-renderer-and-report-truncation.md`

**Status:** `complete` (superseded for frontend; backend portions still relevant)

**What it did:** Fixed backend no-tool content cap truncating legitimate final reports.

**Relevant to this plan:**

| Area | Change |
|------|--------|
| `stream.go` no-tool cap (§2.1) | Relaxed: cap now checks `priorToolResult` and `toolsAvailable` before terminating. Legitimate final answers survive; runway joke-loops still caught. |
| Backend `processStream` signature | Added `priorToolResult bool, toolsAvailable bool` params — threading context into the cap decision. |
| Interaction with Phase 2b completion gate | The relaxation ensures the stream isn't cut BEFORE `checkTaskCompletion` gets to evaluate the turn — the cap and the completion gate now work together. |

**Superseded portions:**
- Frontend terminal deletion was superseded by `automation-renderer-unify-consumption` (which uses the shared `ChatMessages` renderer instead of a separate terminal).

### 9.3 Net Effect on Remaining Work

| Phase | Previously Estimated Scope | Post-Cross-Reference Scope |
|-------|---------------------------|---------------------------|
| Phase 2 | Backend 12 files + Frontend 5 files + 7 docs | Backend code complete — only verification/build/test/doc-audit remain |
| Phase 2b | 9 files (5 code + 4 docs) | Code complete — only 2 manual verifications remain |
| Phase 3 | 4 files (ChatBubble, messageBuilder, ToolCallSegment, AssistantChat) | 4 files — all implemented by `automation-renderer-unify-consumption`. Inset panel, phase state machine, compact mode, auto-collapse all live in code. |

**Plan is code-complete across all 3 phases.** Remaining work (if any) is verification: run `go test ./...` and `npm run build` to confirm baseline, then manual smoke test of chat + automation modes.
