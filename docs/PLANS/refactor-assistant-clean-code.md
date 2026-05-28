# Refactoring Assistant Package — Clean Code & Architecture

**Status: IN PROGRESS**
**Last updated: 2026-05-28**

---

## Executive Summary

Refactor `backend/internal/core/assistant/` from a 1799-line `agent.go` monolith into focused, testable files **within the same package.** No package rename, no new sub-packages, no breaking external contracts — 6 external packages depend on this API surface.

**Result:** 11 well-organized source files (down from 11 original, but now logically grouped), all functions under 80 lines, cyclomatic complexity under 10, zero pointer-to-primitive args.

---

## Critical Rules (Read Before Any Step)

1. **NEVER change test logic.** If a test fails after a refactor step, the refactor code is wrong — fix the refactor, not the test. Tests are the ground truth.
2. **After EACH step:** Run `go build ./... && go test ./internal/core/assistant/... -count=1`. Green → proceed. Red → revert and fix.
3. **After EACH phase completes:** Mark the phase as `[x]` in this plan document.
4. **Do NOT modify** `guardrails/`, `prompts/` sub-packages, or any file starting with `_test`.
5. **Before any refactoring Phase**, run a coverage report so you know the baseline. After the phase, re-run to confirm coverage didn't drop.

---

## Progress Tracker

| Step | Description | Status |
|------|-------------|--------|
| 0.0 | Fix 3 failing pre-existing tests | [ ] |
| 0.1 | Add test coverage for low-coverage functions | [ ] |
| 0.2 | Merge small files (engine.go → tool_provider.go, guardrail_decision.go → agent.go, tiers.go → agent.go, content_filter.go → stream.go) | [ ] |
| 1.1 | Create `errors.go` | [ ] |
| 1.2 | Create `sieve.go` | [ ] |
| 1.3 | Create `stream.go` | [ ] |
| 1.4 | Create `tool_exec.go` | [ ] |
| 1.5 | Create `session.go` | [ ] |
| 2.1 | Create `runSession` struct | [ ] |
| 2.2 | Refactor `Execute` to use `runSession` | [ ] |
| 2.3 | Extract helpers from `Execute` body | [ ] |
| 2.4 | Refactor `handleNoToolCalls` to method on `runSession` | [ ] |
| 3.1 | Define `HistorySieve` interface | [ ] |
| 3.2 | Create concrete sieve implementations | [ ] |
| 3.3 | Keep Agent wrapper methods + add sieve unit tests | [ ] |
| 4.1 | Extract `buildChatRequest` | [ ] |
| 4.2 | Extract `prepareMessagesForTurn` | [ ] |
| 4.3 | Extract `doPreflightCheck` | [ ] |
| 4.4 | Extract `handlePrefillRejection` + `handleEmptyStream` | [ ] |
| 4.5 | Final refactored `computeNextResponse` | [ ] |
| 5.1 | Clean up `processStream` | [ ] |
| 5.2 | Clean up `processToolCalls` | [ ] |
| 5.3 | Clean up `computeNextResponseNonStreaming` | [ ] |
| 5.4 | Clean up `computeNextResponseStreamXML` | [ ] |
| 5.5 | Refactor `InitializeAgentStack` | [ ] |

---

## Table of Contents

1. [Current State & Problems](#1-current-state--problems)
2. [External API Contract (Do Not Break)](#2-external-api-contract-do-not-break)
3. [Dependency Map](#3-dependency-map)
4. [Target File Structure](#4-target-file-structure)
5. [Step 0: Pre-Flight — Fix Tests & Merge Small Files](#5-step-0-pre-flight--fix-tests--merge-small-files)
6. [Phase 1: Safe File Split (Behavior-Preserving)](#6-phase-1-safe-file-split-behavior-preserving)
7. [Phase 2: Extract `runSession` (State Encapsulation)](#7-phase-2-extract-runsession-state-encapsulation)
8. [Phase 3: Strategy Pattern for Sieves](#8-phase-3-strategy-pattern-for-sieves)
9. [Phase 4: Decompose `computeNextResponse`](#9-phase-4-decompose-computenextresponse)
10. [Phase 5: Clean Up Remaining Functions](#10-phase-5-clean-up-remaining-functions)
11. [Phase 6: Guardrails Subpackage (Deferred)](#11-phase-6-guardrails-subpackage-deferred)
12. [Verification Gates](#12-verification-gates)
13. [Risk Register](#13-risk-register)
14. [Appendix A: Function → Target File Map](#appendix-a-function--target-file-map)
15. [Appendix B: Test File Map](#appendix-b-test-file-map)

---

## 1. Current State & Problems

### 1.1 The Big File

| File | Lines | Key Contents |
|---|---|---|
| `agent.go` | **1799** | Agent struct, Execute loop, all LLM calls, stream processing, sieves, tool execution, message prep, error detection, prefill logic, repetition detector |

### 1.2 Functions Exceeding 80-Line Limit (Must Fix)

| Function | Lines | Location | Target after refactor |
|---|---|---|---|
| `(a *Agent).Execute` | 200 | `agent.go:218` | ~30 (wrapper) + runSession methods |
| `(a *Agent).computeNextResponse` | 178 | `agent.go:787` | ~50 |
| `(a *Agent).computeNextResponseNonStreaming` | 138 | `agent.go:1109` | ~60 |
| `(a *Agent).processStream` | 126 | `agent.go:982` | ~60 |
| `(a *Agent).processToolCalls` | 115 | `agent.go:1370` | ~60 |
| `(a *Agent).computeNextResponseStreamXML` | 83 | `agent.go:1248` | ~50 |
| `(a *Agent).handleNoToolCalls` | 81 | `agent.go:593` | ~40 |
| `initializeAgentStack` | 173 | `registry.go:83` | ~40 + helpers |

### 1.3 Pointer-to-Primitive Anti-Pattern

`Agent.Execute` declares these counters and passes pointers through function calls. All eliminated by `runSession`:

```go
parseErrorStreak        int       // passed to handleNoToolCalls →
lastParseErrorKind      string    // passed to handleNoToolCalls → runSession fields
totalErrorStreak        int       // passed to handleNoToolCalls →
modelCompatNotified     bool      // passed to handleNoToolCalls →
```

### 1.4 Existing Coverage Gaps (Must Improve Before Refactoring)

| Function | Coverage | Action |
|---|---|---|
| `executePlan` | **0.0%** | Add test for execution plan path |
| `notifyPrematureTerminationNag` | **0.0%** | Add test for premature termination in automation |
| `toolCategory` | **28.6%** | Add test cases for all tool categories |
| `injectToolInstructions` | **66.7%** | Add test for missing system message path |
| `computeNextResponseStreamXML` | **62.8%** | Add test for XML fallback prefill paths |
| `guardrails/guardrails.go` | **36.3%** | Deferred to Phase 6 (separate project) |

### 1.5 Key Complexity Hotspots

1. **`computeNextResponse` (178 lines, ~12 decision points):** Budget pre-flight, prefill retry, streaming dispatch, stream failure fallback, empty-stream XML retry, non-streaming fallback — all in one function.
2. **`Execute` (200 lines, ~18 decision points):** Error classification, sieve streak tracking, starvation counting, duplicate detection, tool result assembly, submit_final_answer detection.
3. **`processToolCalls` (115 lines):** Batched submission rejection, guardrail validation, async guardrail decision with channel-based blocking, engine execution, error handling, result appending — interleaved with lock management.

---

## 2. External API Contract (Do Not Break)

These types, functions, and values are consumed by `app/bootstrap.go`, `transport/http/assistant_handlers.go`, `automation/executor.go`, `automation/dispatcher.go`, `automation/broadcast.go`, and `testing/mocks/assistant_service.go`. **Their names, signatures, and behavior must remain identical.**

### Complete Public API Surface (Immutable)

```
// Types
Agent, AgentOptions, AgentEvent, AgentEventType, Observer
GuardrailDecisionCallback, GuardrailBlockedPayload, GuardrailDecision
GuardrailInvalidatedPayload, GuardrailDecisionStore
ExecutionPlan, ExecutionStep, ExecutionPlanStrategy
UsageTracker, ToolHandler
LocalToolRegistry, MultiToolProvider, CompositeEngine
ProviderTuningDefaults

// Interfaces
Engine { ExecuteTool(ctx, ToolCall) (any, error) }
ToolProvider { ListTools(ctx) ([]Tool, error); GetSystemPrompt() (string, error); UseNativeTools() bool }

// Constructors
NewAgent(client, ToolProvider, Engine, AgentOptions) *Agent
NewEngine(mcp, logger) Engine
NewLocalToolRegistry(...) *LocalToolRegistry
NewMultiToolProvider(useNativeTools bool, ...ToolProvider) *MultiToolProvider
NewCompositeEngine(primary, secondary Engine) *CompositeEngine
InitializeAgentStack(...) (ToolProvider, Engine, *GuardrailEngine)
NewGuardrailDecisionStore() *GuardrailDecisionStore
NewGuardrailDecisionCallback(store, observer) GuardrailDecisionCallback
NewExecutionPlanStrategy(llm, tools, logger) *ExecutionPlanStrategy
GetUsageTracker(ctx) *UsageTracker
FilterStreamingMarkup(content) (string, bool)
ProviderTiers() map[string]ProviderTuningDefaults

// Constants
DefaultMaxSteps, DefaultContextBudget, DefaultMaxTokens
MinReasoningStuckThreshold, DefaultStarvationLimit
AgentGlobalTimeout, AgentTurnTimeout, AgentRetryTimeout

// Event type constants
EventStepStart, EventMessage, EventToolCall, EventToolResult
EventGuardrailViolation, EventGuardrailBlocked, EventGuardrailInvalidated
EventError, EventToolStream, EventLifecycle

// Error sentinel
var ErrToolNotInternal
```

---

## 3. Dependency Map

### 3.1 What stays in `assistant/` vs `orchestrator/`

| Concern | Location | Notes |
|---|---|---|
| ICU budgeting | `orchestrator/` | Pre-flight check, refund, squeeze |
| Stream token counting | `orchestrator/` | StreamInterceptor, ReasoningNormalizer |
| LLM slot management | `orchestrator/` | llama.cpp KV cache |
| Context sieves | `assistant/` | Physical/Reactive/Aggressive |
| Stream processing | `assistant/` | processStream calls orchestrator for interception |
| Tool execution | `assistant/` | Validation, guardrails, engine dispatch |
| Message preparation | `assistant/` | Normalization, tool instruction injection |
| Error classification | `assistant/` | parseErrorKind, is*Error helpers |
| Repetition detection | `assistant/` | Duplicate tool call detection |
| Prefill logic | `assistant/` | Automation prefill, thinking-mode rejection |

### 3.2 Internal imports of `agent.go`

```
"llm-proxy/internal/core/assistant/guardrails"
"llm-proxy/internal/core/assistant/prompts"
"llm-proxy/internal/core/orchestrator"
"llm-proxy/internal/core/proxy"
"llm-proxy/internal/platform/logging"
"llm-proxy/models"
// plus stdlib: context, encoding/json, fmt, regexp, strings, sync, time
// NOTE: "llm-proxy/internal/platform/storage" appears in imports but is UNUSED — remove during cleanup
```

---

## 4. Target File Structure

All files remain in `package assistant` in `internal/core/assistant/`. No new sub-packages.

**Before (current):**
```
internal/core/assistant/
├── agent.go              (1799L) ← THE MONOLITH
├── agent_events.go       (143L)
├── engine.go             (42L)  ← merges into tool_provider.go
├── guardrail_decision.go (105L) ← merges into agent.go
├── registry.go           (363L)
├── strategy_plan.go      (91L)  ← merges into tool_exec.go
├── tiers.go              (22L)  ← merges into agent.go
├── tool_provider.go      (107L)
├── usagetracker.go       (49L)
├── content_filter.go     (55L)  ← merges into stream.go
├── guardrails/           (sub-pkg, 534L)
└── prompts/              (sub-pkg, 403L)
```

**After (target):**
```
internal/core/assistant/
├── agent.go              # Agent struct, AgentOptions, NewAgent, repetitionDetector,
│                         #   GuardrailDecisionStore, ProviderTiers,
│                         #   prepareMessages, injectToolInstructions,
│                         #   injectNativeToolReference (~400L)
├── session.go       [NEW]# runSession, executeTurn, handleNoToolCalls,
│                         #   handleContentToolCalls, termination heuristics (~500L)
├── sieve.go         [NEW]# HistorySieve, 3 sieve implementations,
│                         #   truncateLongContent (~250L)
├── stream.go        [NEW]# computeNextResponse, processStream,
│                         #   computeNextResponseNonStreaming, computeNextResponseStreamXML,
│                         #   stuck detection, prefill logic, FilterStreamingMarkup (~600L)
├── tool_exec.go     [NEW]# processToolCalls, executePlan, validateToolArgs,
│                         #   appendToolResult, ExecutionPlan, ExecutionPlanStrategy (~350L)
├── events.go             # AgentEvent, event types, Observer,
│                         #   GuardrailDecisionCallback, notify* methods (143L UNCHANGED)
├── tool_provider.go      # ToolProvider, Engine, MultiToolProvider,
│                         #   CompositeEngine, assistantEngine, NewEngine (~150L)
├── registry.go           # LocalToolRegistry, InitializeAgentStack (363L UNCHANGED)
├── errors.go        [NEW]# Error classification helpers (~80L)
├── usagetracker.go       # UsageTracker (49L UNCHANGED)
├── guardrails/
│   └── guardrails.go     # GuardrailEngine (UNCHANGED)
└── prompts/
    ├── templates.go      # All prompt constants/builders (UNCHANGED)
    └── system_prompt.go  # BuildSystemMessage (UNCHANGED)
```

**Key rule:** Functions ONLY move between files within `package assistant`. The package statement in every file is `package assistant`. No import changes for consumers of the package.

---

## 5. Step 0: Pre-Flight — Fix Tests & Merge Small Files

### Step 0.0: Fix 3 Failing Pre-Existing Tests [ ]

**MANDATORY — must pass before any refactoring begins.**

Current failures (2026-05-28):

```
--- FAIL: TestRepetitionDetector_StreakReset (0.00s)
    agent_test.go:1805: expected duplicateStreak to be 1, got 0
    agent_test.go:1814: expected consecutive duplicate, got isDup=true streak=1
    agent_test.go:1820: expected error on third consecutive duplicate, got nil

--- FAIL: TestRepetitionDetector_SlidingWindow (0.00s)
    --- FAIL: TestRepetitionDetector_SlidingWindow/alternating_loop (0.00s)
        agent_test.go:1867: expected streak=1 after second A, got 0
        agent_test.go:1879: expected streak=2 after second B, got 0
        agent_test.go:1887: expected fatal error on alternating loop, got nil
    --- FAIL: TestRepetitionDetector_SlidingWindow/cwd_normalization (0.00s)
        agent_test.go:1931: expected duplicate after cwd normalization
        agent_test.go:1934: expected non-empty nag prompt

--- FAIL: TestAgent_Execute_ToolExecutionErrorFeedback (0.00s)
    agent_test.go:2056: Execute failed: llm completion failed: expected ToolErrorNagPrompt
```

**To fix:**

1. Read `repetitionDetector.check()` at `agent.go:194-216`. Read assertions at `agent_test.go:1780-1820`. Identify whether the implementation drifted or test expectations are stale. **Do NOT change the test's intent — if test expectations are stale, fix the implementation; if implementation is correct, adjust test to match.**

2. For `TestAgent_Execute_ToolExecutionErrorFeedback`, check `processToolCalls` to verify it injects `ToolErrorNagPrompt` into history when a tool execution fails. Read the test setup at `agent_test.go:2020-2060`.

**Verification:** `go test ./internal/core/assistant/... -count=1` — all tests green.

**After completing Step 0.0, mark it `[x]` above.**

---

### Step 0.1: Add Test Coverage for Low-Coverage Functions [ ]

**Before any refactoring, add tests for uncovered/barely-covered functions.** This ensures refactoring won't silently break untested paths.

**Run baseline coverage first:**
```bash
cd backend
go test -coverprofile=/tmp/assistant_pre_cover.out ./internal/core/assistant/... 2>&1
go tool cover -func=/tmp/assistant_pre_cover.out | grep -E "(executePlan|notifyPrematureTerminationNag|toolCategory|injectToolInstructions|computeNextResponseStreamXML)" 2>/dev/null
```

**Add tests (in `agent_test.go` unless noted):**

1. **`executePlan` (0.0%):** Create a test `TestAgent_ExecutePlan_Success` that:
   - Creates an Agent with mock client and a plan strategy
   - Sets up a simple 2-step ExecutionPlan (e.g., terminal_execute + file_read)
   - Mock Engine returns results for each step
   - Verifies Execute returns `"[Plan execution complete]"` without error

2. **`notifyPrematureTerminationNag` (0.0%):** Add to existing `TestAgent_PrematureTerminationInAutomation` or create a new test that verifies the nag message is appended to history when premature termination is detected in automation mode.

3. **`toolCategory` (28.6%):** Add test `TestToolCategory_AllCases` that calls `toolCategory()` with every tool name defined in `models/tools.go` and asserts the correct category string.

4. **`injectToolInstructions` (66.7%):** Add test for the edge case where no system message exists in history (should prepend a new one). Add test for when tools list is empty (should return unchanged history).

5. **`computeNextResponseStreamXML` (62.8%):** Add test for the case where prefill thinking error is detected and retried successfully.

**CRITICAL: When writing new tests, follow these rules:**
- Use `MockClient` and `MockEngine` patterns already established in `agent_test.go`
- Do NOT refactor production code — you're adding tests for existing behavior
- Each new test must pass before you move to the next

**Verification:** `go test ./internal/core/assistant/... -count=1 -coverprofile=/tmp/assistant_post0_1.out` — green. Coverage should increase from 71.9%.

**After completing Step 0.1, mark it `[x]` above.**

---

### Step 0.2: Merge Small Files [ ]

**Goal:** Reduce file count before the main split. Four existing files each under 110 lines that lack standalone justification. Merge them into logically related files.

**IMPORTANT:** This is a pure file-move operation. Copy-paste the content. Do NOT change any code. Do NOT add/remove imports. Do NOT reformat.

#### 0.2a: Merge `engine.go` (42L) → `tool_provider.go`

**Why:** `Engine` interface is always paired with `ToolProvider`. `assistantEngine` implementation. `NewEngine` constructor. All 42 lines belong next to the `CompositeEngine` which also implements `Engine`.

Open `engine.go`, copy its entire content, append it to the END of `tool_provider.go` (after the `CompositeEngine` code). Remove `engine.go`.

`tool_provider.go` goes from 107 lines → ~149 lines.

**Verification:** `go build ./... && go test ./internal/core/assistant/... -count=1` — green.

#### 0.2b: Merge `guardrail_decision.go` (105L) → `agent.go`

**Why:** `GuardrailDecisionStore` + `NewGuardrailDecisionCallback` are tightly coupled to `Agent.guardrails` and `Agent.onGuardrail`. They're conceptually part of the Agent's runtime.

Open `guardrail_decision.go`, copy its entire content, insert it into `agent.go` after the `AgentOptions` type definition (before `NewAgent`). Remove `guardrail_decision.go`.

`agent.go` goes from 1799 lines → ~1904 lines (temporarily, before the main split).

**Verification:** `go build ./... && go test ./internal/core/assistant/... -count=1` — green.

#### 0.2c: Merge `tiers.go` (22L) → `agent.go`

**Why:** `ProviderTuningDefaults` and `ProviderTiers()` are reference defaults used during `NewAgent` creation. They're tiny and belong near the `AgentOptions.DefaultMaxSteps` etc. constants.

Copy `tiers.go` content into `agent.go` right after the const block that defines `DefaultMaxSteps`, `DefaultContextBudget` etc. Remove `tiers.go`.

**Verification:** `go build ./... && go test ./internal/core/assistant/... -count=1` — green.

#### 0.2d: Merge `content_filter.go` (55L) → into `stream.go` (created in Phase 1)

**Why:** `FilterStreamingMarkup` is ONLY called from `processStream` and `normalizeContent` is an internal helper. They belong in the stream processing file.

Since `stream.go` doesn't exist yet (it's created in Phase 1 Step 1.3), defer this merge until that step. **Add a note to Step 1.3** to include `FilterStreamingMarkup` and `normalizeContent` in the new `stream.go` when it's created. Remove `content_filter.go` at that time.

#### 0.2e: Move `ExecutionPlanStrategy` and `ExecutionPlan` from `strategy_plan.go` → `tool_exec.go` (created in Phase 1)

**Why:** `ExecutionPlanStrategy` generates tool execution plans, and `executePlan` executes them. They belong together.

Defer until Phase 1 Step 1.4. When creating `tool_exec.go`, include `ExecutionPlan`, `ExecutionStep`, `ExecutionPlanStrategy`, `NewExecutionPlanStrategy`, and `Generate` from `strategy_plan.go`. Remove `strategy_plan.go` at that time.

**Note:** If you accidentally remove `strategy_plan.go` before creating `tool_exec.go`, tests that import `ExecutionPlanStrategy` will break. Ensure the content is in `tool_exec.go` first.

---

### Files After Step 0.2 (pre-Phase 1):

```
internal/core/assistant/
├── agent.go              (~1950L) ← includes tiers.go + guardrail_decision.go content
├── agent_events.go       (143L)
├── tool_provider.go      (~149L)  ← includes engine.go content
├── registry.go           (363L)
├── strategy_plan.go      (91L)    ← will merge into tool_exec.go in Phase 1
├── content_filter.go     (55L)    ← will merge into stream.go in Phase 1
├── usagetracker.go       (49L)
├── guardrails/           (UNCHANGED)
└── prompts/              (UNCHANGED)
```

**7 source files** (down from 11). Ready for Phase 1 file split.

**After completing Step 0.2, mark it `[x]` above.**

---

## 6. Phase 1: Safe File Split (Behavior-Preserving)

**Goal:** Split the ~1950-line `agent.go` into focused files without changing ANY code. Pure cut-and-paste. Zero logic changes.

**Approach:** Cut functions from `agent.go` and paste them into new files. The compiler treats all files in `package assistant` as one unit — Go doesn't care which file a function is in.

### Step 1.1: Create `errors.go` [ ]

**Move from `agent.go` → new file `errors.go`:**

```
parseErrorKind(e *proxy.ParseError) string         // line numbers WILL shift after merges
isTruncationError(errStr string) bool               // Find by function name, not line number
isToolCallParseError(err error) bool
isContextSizeError(err error) bool
isToolSupportError(err error) bool
isPrefillThinkingError(err error) bool
```

**Procedure:**
1. `touch internal/core/assistant/errors.go`
2. Write `package assistant` as first line
3. Copy the import block — include only: `"strings"`, `"llm-proxy/internal/core/proxy"`
4. Copy each function body verbatim (including comments if any)
5. **Delete** those functions from `agent.go`
6. Make sure no other function in `agent.go` was cut accidentally

**Verification:** `go build ./... && go test ./internal/core/assistant/... -count=1` — green.

**Mark `[x]` when done.**

### Step 1.2: Create `sieve.go` [ ]

**Move from `agent.go` → new file `sieve.go`:**

```
const sieveLockedHead      = 2
const sievePhysicalTail    = 10
const sieveReactiveTail    = 6
const sieveAggressiveTail  = 3
const compressContentMax   = 4000
const compressReasoningMax = 2000

truncateLongContent(s string, limit int) string
(a *Agent).applyPhysicalSieve(history []proxy.Message) []proxy.Message
(a *Agent).applyReactiveSieve(history []proxy.Message) []proxy.Message
(a *Agent).applyAggressiveSieve(history []proxy.Message) []proxy.Message
```

**Procedure:**
1. `touch internal/core/assistant/sieve.go`
2. Write `package assistant`
3. Import: `"llm-proxy/internal/core/assistant/prompts"`, `"llm-proxy/internal/core/proxy"`
4. Copy constants first, then functions
5. Delete from `agent.go`

**Verification:** `go build ./... && go test ./internal/core/assistant/... -count=1` — green. **Mark `[x]`.**

### Step 1.3: Create `stream.go` [ ]

**Move from `agent.go` → new file `stream.go`:**

```
var toolCallInContent = regexp.MustCompile(`(?si)<tool_call>.*?</tool_call>`)

(a *Agent).computeNextResponse(ctx, history, tools) (proxy.Message, error)
injectRetryContext(history []proxy.Message) []proxy.Message
(a *Agent).processStream(ctx, ch, fullMsg) error
(a *Agent).computeNextResponseNonStreaming(ctx, history, tools) (proxy.Message, error)
(a *Agent).computeNextResponseStreamXML(ctx, history, tools) (proxy.Message, error)
(a *Agent).findAutomationCtx(history []proxy.Message) bool
(a *Agent).shouldPrefill(isAutomationCtx bool) bool
(a *Agent).stuckThreshold() int
cleanReasoningContent(s string) string
```

**ALSO merge `content_filter.go` into stream.go at this step:**
Copy `FilterStreamingMarkup` and `normalizeContent` from `content_filter.go` into `stream.go`. Append them after the functions moved from `agent.go`. Delete `content_filter.go`.

**Imports needed in stream.go:**
```
"context", "encoding/json", "fmt", "regexp", "strings", "time"
"llm-proxy/internal/core/assistant/prompts"
"llm-proxy/internal/core/orchestrator"
"llm-proxy/internal/core/proxy"
"llm-proxy/models"
```

**IMPORT CLEANUP RULE:** After EVERY move from `agent.go` to a new file:
1. Delete moved functions from `agent.go`
2. Delete imports from `agent.go` that are ONLY used by the moved functions (keep shared imports)
3. Add any new imports needed by the receiving file only if they weren't already imported in `agent.go`
4. Run `go vet ./internal/core/assistant/` — it will catch any unused imports
5. Then run the test suite.  If vet passes and tests pass, the move is clean.

The `storage` import in `agent.go` is known to be stale — remove it during the file split.

**Verification:** `go build ./... && go test ./internal/core/assistant/... -count=1` — green. **Mark `[x]`.**

### Step 1.4: Create `tool_exec.go` [ ]

**Move from `agent.go` → new file `tool_exec.go`:**

```
(a *Agent).processToolCalls(ctx, msg, history) error
formatGuardrailError(err error) map[string]string
(a *Agent).executePlan(ctx, history, plan) (string, []Message, error)
toolCategory(toolName string) string
extractTaskSummary(rawArgs string) string
validateToolArgs(tc proxy.ToolCall, tools []proxy.Tool) error
(a *Agent).appendToolResult(history, tc, result)
```

**ALSO merge `strategy_plan.go` into `tool_exec.go` at this step:**
Copy `ExecutionPlan`, `ExecutionStep`, `ExecutionPlanStrategy`, `NewExecutionPlanStrategy`, and `Generate` from `strategy_plan.go` into `tool_exec.go`. Append them after the functions moved from `agent.go`. Delete `strategy_plan.go`.

**Imports needed:**
```
"context", "encoding/json", "fmt", "strings", "sync", "time"
"llm-proxy/internal/core/assistant/prompts"
"llm-proxy/internal/core/proxy"
"llm-proxy/internal/platform/logging"
"llm-proxy/models"
```

**Verification:** `go build ./... && go test ./internal/core/assistant/... -count=1` — green. **Mark `[x]`.**

### Step 1.5: Create `session.go` [ ]

**Move from `agent.go` → new file `session.go`:**

```
(a *Agent).executeTurn(ctx, history) (Message, *ParseError, []Tool, error)
(a *Agent).handleNoToolCalls(turnMsg, history, isAutomation, parseErr, toolsList,
    steps, parseErrorStreak, lastParseErrorKind, totalErrorStreak, modelCompatNotified) (string, bool, error)
(a *Agent).handleContentToolCalls(msg *proxy.Message) *proxy.ParseError
(a *Agent).precededByToolResult(history []proxy.Message) bool
(a *Agent).countConsecutiveChat(history []proxy.Message) int
(a *Agent).isPrematureTermination(msg proxy.Message, history []proxy.Message) bool
```

**Imports needed:**
```
"context", "fmt", "strings", "time"
"llm-proxy/internal/core/assistant/prompts"
"llm-proxy/internal/core/proxy"
"llm-proxy/models"
```

**Verification:** `go build ./... && go test ./internal/core/assistant/... -count=1` — green. **Mark `[x]`.**

### After Phase 1: What Remains in `agent.go`

```
package assistant

import (...relevant remaining imports...)

// Constants (those NOT moved to sieve.go)
const DefaultMaxSteps = 25
const DefaultContextBudget = 8000
...

// Types
type Agent struct { ... }
type AgentOptions struct { ... }
type toolKey struct { ... }
type repetitionDetector struct { ... }
type ProviderTuningDefaults struct { ... }
type GuardrailDecisionStore struct { ... }

// Constructor
func NewAgent(...) *Agent { ... }
func (o *AgentOptions) applyDefaults() { ... }
func ProviderTiers() map[string]ProviderTuningDefaults { ... }

// Guardrail decision
func NewGuardrailDecisionStore() *GuardrailDecisionStore { ... }
func (s *GuardrailDecisionStore) Register(...) { ... }
func (s *GuardrailDecisionStore) Resolve(...) bool { ... }
func (s *GuardrailDecisionStore) Remove(...) { ... }
func NewGuardrailDecisionCallback(...) GuardrailDecisionCallback { ... }

// Repetition detection
func (rd *repetitionDetector) check(...) (bool, string, error) { ... }

// Message preparation
func (a *Agent) prepareMessages(history []proxy.Message) []proxy.Message { ... }
func (a *Agent) injectToolInstructions(history []proxy.Message, tools []proxy.Tool) []proxy.Message { ... }
func (a *Agent) injectNativeToolReference(history []proxy.Message, tools []proxy.Tool) []proxy.Message { ... }

// Main execute loop
func (a *Agent) Execute(ctx context.Context, history []proxy.Message) (string, []proxy.Message, error) { ... }
```

**Approximate size:** ~450 lines (was 1950 after merges).

**Final post-Phase-1 file list (11 files):**
```
agent.go (~450L), session.go (~290L), sieve.go (~150L), stream.go (~600L),
tool_exec.go (~350L), events.go (143L), tool_provider.go (~149L),
registry.go (363L), errors.go (~80L), usagetracker.go (49L)
+ guardrails/ + prompts/ (unchanged)
```

**FULL verification after Phase 1 completes:**
```bash
cd backend
go build ./...
go vet ./...
go test ./internal/core/assistant/... -count=1 -v
go test ./internal/core/orchestrator/... -count=1
go build ./...  # builds consumers too
```

**After Phase 1 is fully green, mark all Step 1.x items `[x]` in the Progress Tracker.**

---

## 7. Phase 2: Extract `runSession` (State Encapsulation)

**Goal:** Replace pointer-to-primitive arguments in `Execute` → `handleNoToolCalls` with an encapsulating struct. Reduce `Execute` from 200 lines to ~30.

### Step 2.1: Create the `runSession` struct [ ]

Add to the TOP of `session.go` (before all existing functions):

```go
type runSession struct {
    agent   *Agent
    ctx     context.Context

    history        []proxy.Message
    steps          int
    sieveStreak    int
    starvationCount int
    warnedAdvisory bool

    parseErrorStreak    int
    lastParseErrorKind  string
    totalErrorStreak    int
    modelCompatNotified bool

    isAutomation bool
    rd           repetitionDetector
}

func newRunSession(agent *Agent, ctx context.Context, history []proxy.Message) *runSession {
    s := &runSession{
        agent:   agent,
        ctx:     ctx,
        history: append([]proxy.Message{}, history...),
    }
    for _, m := range s.history {
        if prompts.IsAutomationTask(m.Content) {
            s.isAutomation = true
            break
        }
    }
    return s
}
```

**Verification:** `go build ./...` — green (compiles but not yet used). **Mark `[x]`.**

### Step 2.2: Refactor `Agent.Execute` to use `runSession` [ ]

Replace the body of `Execute` in `agent.go` with a thin wrapper that creates a `runSession` and delegates:

```go
func (a *Agent) Execute(ctx context.Context, history []proxy.Message) (string, []proxy.Message, error) {
    execCtx, cancel := context.WithTimeout(ctx, a.globalTimeout)
    defer cancel()

    execCtx = withUsageTracker(execCtx)

    if a.planStrategy != nil {
        lastUserMsg := ""
        for i := len(history) - 1; i >= 0; i-- {
            if history[i].Role == proxy.UserRole {
                lastUserMsg = history[i].Content
                break
            }
        }
        if lastUserMsg != "" {
            tools, err := a.provider.ListTools(execCtx)
            if err == nil && len(tools) > 0 {
                plan, planErr := a.planStrategy.Generate(execCtx, lastUserMsg)
                if planErr == nil {
                    return a.executePlan(execCtx, history, plan)
                }
                a.logger.Warn("plan generation failed, falling back to normal loop",
                    "error", planErr)
            }
        }
    }

    s := newRunSession(a, execCtx, history)
    return s.run()
}
```

Now add `s.run()` to `session.go`. This is the EXACT loop body that was in `Execute`, but:
- Replace `a.` with `s.agent.` for Agent methods (notify, logger, executeTurn, processToolCalls)
- Replace local vars (`steps`, `sieveStreak`, `starvationCount`, etc.) with `s.` field access
- Replace `execCtx` with `s.ctx`

**CRITICAL:** Do not change ANY logic. This is a mechanical conversion:
- `steps` → `s.steps`
- `starvationCount` → `s.starvationCount`
- `sieveStreak` → `s.sieveStreak`
- `warnedAdvisory` → `s.warnedAdvisory`
- `parseErrorStreak` → `s.parseErrorStreak`
- `lastParseErrorKind` → `s.lastParseErrorKind`
- `totalErrorStreak` → `s.totalErrorStreak`
- `modelCompatNotified` → `s.modelCompatNotified`
- `currentHistory` → `s.history`
- `isAutomation` → `s.isAutomation`
- `rd` → `s.rd`
- `execCtx` → `s.ctx`
- `a.` → `s.agent.` (for logger, notify, executeTurn, processToolCalls, etc.)

**Verification:** `go build ./... && go test ./internal/core/assistant/... -count=1` — green. **Mark `[x]`.**

### Step 2.3: Extract helpers from the run loop [ ]

Move these logic blocks from `s.run()` into separate `runSession` methods in `session.go`:

```go
func (s *runSession) handleContextSizeError() (string, []proxy.Message, error) {
    // The context-size error handling block from the old Execute loop
    // Returns the triple to signal "continue" (empty string, history, nil = continue loop)
    // or an actual error to abort
}

func (s *runSession) handleToolCallParseError(err error) {
    // Parse error feedback injection block
}

func (s *runSession) resetParseErrorState() {
    s.parseErrorStreak = 0
    s.lastParseErrorKind = ""
    s.totalErrorStreak = 0
    s.modelCompatNotified = false
}

func (s *runSession) trimLargeWriteContent(turnMsg *proxy.Message) {
    // Content trimming for large write_file payloads
}

func (s *runSession) checkSubmitFinalAnswer(turnMsg proxy.Message) (string, bool) {
    // submit_final_answer detection and summary extraction
}
```

Each helper should be under 25 lines.

**Verification:** `go build ./... && go test ./internal/core/assistant/... -count=1` — green. **Mark `[x]`.**

### Step 2.4: Refactor `handleNoToolCalls` to method on `runSession` [ ]

**This is the most sensitive step — be careful.**

Current signature (on `*Agent`):
```go
func (a *Agent) handleNoToolCalls(
    turnMsg proxy.Message,
    history *[]proxy.Message,
    isAutomation bool,
    parseErr *proxy.ParseError,
    toolsList []proxy.Tool,
    steps int,
    parseErrorStreak *int,
    lastParseErrorKind *string,
    totalErrorStreak *int,
    modelCompatNotified *bool,
) (string, bool, error)
```

New signature (on `*runSession`):
```go
func (s *runSession) handleNoToolCalls(
    turnMsg proxy.Message,
    parseErr *proxy.ParseError,
    toolsList []proxy.Tool,
) (string, bool, error)
```

**Conversion steps:**
1. Copy the function body EXACTLY from the old `Agent.handleNoToolCalls`.
2. Replace `a.` with `s.agent.` for Agent-specific fields (logger, observer, useNativeTools).
3. Replace pointer args with `s.` fields:
   - `*history` → `s.history` (use `&s.history` where the function appends)
   - `isAutomation` → `s.isAutomation`
   - `steps` → `s.steps`
   - `*parseErrorStreak` → `s.parseErrorStreak`
   - `*lastParseErrorKind` → `s.lastParseErrorKind`
   - `*totalErrorStreak` → `s.totalErrorStreak`
   - `*modelCompatNotified` → `s.modelCompatNotified`
4. Delete the old `(a *Agent) handleNoToolCalls` function.
5. Update the call site in `s.run()`:
   ```go
   // Old:
   reply, shouldExit, err := s.agent.handleNoToolCalls(
       turnMsg, &s.history, s.isAutomation, parseErr, toolsList,
       s.steps, &s.parseErrorStreak, &s.lastParseErrorKind,
       &s.totalErrorStreak, &s.modelCompatNotified,
   )
   // New:
   reply, shouldExit, err := s.handleNoToolCalls(turnMsg, parseErr, toolsList)
   ```
6. **Search `agent_test.go`** for any test that calls `handleNoToolCalls` directly on `*Agent`. If found, do NOT change the test — instead keep a thin backward-compat wrapper on `*Agent`:
   ```go
   func (a *Agent) handleNoToolCalls(args...) (string, bool, error) {
       // tests may call this — create a runSession and delegate
   }
   ```
   If NO tests call it directly, the old signature can be safely removed.

**Verification:** `go build ./... && go test ./internal/core/assistant/... -count=1` — green. **Mark `[x]`.**

**After Phase 2 is fully green, mark all Step 2.x items `[x]` in the Progress Tracker.**

---

## 8. Phase 3: Strategy Pattern for Sieves

**Goal:** Unify the three sieve methods behind a `HistorySieve` interface. Add unit tests for each strategy.

### Step 3.1: Define the `HistorySieve` interface [ ]

Add to `sieve.go`:

```go
type HistorySieve interface {
    Sieve(history []proxy.Message) []proxy.Message
    Name() string
}
```

**Verification:** `go build ./...` — compiles. **Mark `[x]`.**

### Step 3.2: Create concrete implementations [ ]

Add three struct types to `sieve.go` implementing `HistorySieve`:

```go
type physicalSieve struct {
    logger        logging.Logger
    contextBudget int
}

func (p *physicalSieve) Name() string { return "physical" }

func (p *physicalSieve) Sieve(history []proxy.Message) []proxy.Message {
    // EXACT logic from the current applyPhysicalSieve body
    // Replace a.logger with p.logger, a.contextBudget with p.contextBudget
}

type reactiveSieve struct {
    logger logging.Logger
}

func (r *reactiveSieve) Name() string { return "reactive" }

func (r *reactiveSieve) Sieve(history []proxy.Message) []proxy.Message {
    // EXACT logic from the current applyReactiveSieve body
}

type aggressiveSieve struct {
    logger logging.Logger
}

func (a *aggressiveSieve) Name() string { return "aggressive" }

func (a *aggressiveSieve) Sieve(history []proxy.Message) []proxy.Message {
    // EXACT logic from the current applyAggressiveSieve body
}
```

**Verification:** `go build ./... && go test ./internal/core/assistant/... -count=1` — green.

### Step 3.3: Keep Agent wrapper methods + add sieve unit tests [ ]

Replace the existing `apply*Sieve` methods with thin wrappers:

```go
func (a *Agent) applyPhysicalSieve(history []proxy.Message) []proxy.Message {
    s := &physicalSieve{logger: a.logger, contextBudget: a.contextBudget}
    return s.Sieve(history)
}

func (a *Agent) applyReactiveSieve(history []proxy.Message) []proxy.Message {
    s := &reactiveSieve{logger: a.logger}
    return s.Sieve(history)
}

func (a *Agent) applyAggressiveSieve(history []proxy.Message) []proxy.Message {
    s := &aggressiveSieve{logger: a.logger}
    return s.Sieve(history)
}
```

Add unit tests (in `agent_test.go` or new `sieve_test.go`):

```go
func TestPhysicalSieve_UnderBudget(t *testing.T) { ... }
func TestPhysicalSieve_OverBudget_Compresses(t *testing.T) { ... }
func TestPhysicalSieve_OverBudget_DropsMessages(t *testing.T) { ... }
func TestReactiveSieve_NormalCase(t *testing.T) { ... }
func TestReactiveSieve_TooShort(t *testing.T) { ... }
func TestAggressiveSieve_NormalCase(t *testing.T) { ... }
func TestAggressiveSieve_TooShort(t *testing.T) { ... }
```

**Verification:** `go build ./... && go test ./internal/core/assistant/... -count=1` — green. **Mark all `[x]`.**

---

## 9. Phase 4: Decompose `computeNextResponse`

**Goal:** Break the 178-line function into small helper functions. Each helper should be under 40 lines.

### Step 4.1: Extract `buildChatRequest` [ ]

```go
func (a *Agent) buildChatRequest(
    prepared []proxy.Message,
    llmTools []proxy.Tool,
    isAutomationCtx bool,
) proxy.ChatRequest {
    req := proxy.ChatRequest{
        Messages:  prepared,
        Tools:     llmTools,
        MaxTokens: a.maxTokens,
    }
    if a.useNativeTools && isAutomationCtx {
        req.ToolChoice = proxy.ToolChoiceRequired
    }
    if isAutomationCtx {
        req.Temperature = 0.1
        if a.reasoningBudget > 0 {
            req.ReasoningBudget = a.reasoningBudget
        } else {
            req.ReasoningBudget = a.maxTokens / 4
        }
    }
    return req
}
```

**Verification:** `go build ./... && go test ./internal/core/assistant/... -count=1` — green. **Mark `[x]`.**

### Step 4.2: Extract `prepareMessagesForTurn` [ ]

```go
func (a *Agent) prepareMessagesForTurn(
    history []proxy.Message,
    tools []proxy.Tool,
    llmTools []proxy.Tool,
) ([]proxy.Message, string) {
    prepared := a.prepareMessages(history)
    if llmTools == nil && len(tools) > 0 {
        prepared = a.injectToolInstructions(prepared, tools)
    } else if llmTools != nil && len(tools) > 0 {
        prepared = a.injectNativeToolReference(prepared, tools)
    }

    var prefill string
    isAutoCtx := a.findAutomationCtx(history)
    if a.shouldPrefill(isAutoCtx) {
        prefill = prompts.AutomationPrefline
        prepared = append(prepared, proxy.Message{
            Role:    proxy.AssistantRole,
            Content: prefill,
        })
    }
    return prepared, prefill
}
```

**Verification:** green. **Mark `[x]`.**

### Step 4.3: Extract `doPreflightCheck` [ ]

```go
func (a *Agent) doPreflightCheck(
    ctx context.Context,
    history []proxy.Message,
    req *proxy.ChatRequest,
) (txnID string, err error) {
    if a.orch == nil || a.orch.Budget == nil {
        return "", nil
    }
    totalChars := 0
    for _, m := range history {
        totalChars += len(m.Content)
    }
    preflight, err := a.orch.Budget.PreFlightCheck(ctx, a.workspaceID,
        orchestrator.PreFlightRequest{
            ModelName:       a.modelName,
            ProviderType:    a.providerType,
            ContextChars:    totalChars,
            MaxTokens:       a.maxTokens,
            ReasoningBudget: a.reasoningBudget,
            ICUWeight:       a.icuWeight,
        })
    if err != nil {
        return "", fmt.Errorf("budget error: %w", err)
    }
    if !preflight.Allowed {
        return "", fmt.Errorf("budget exceeded: %s", preflight.Reason)
    }
    if preflight.SqueezeFactor < 1.0 {
        a.maxTokens = preflight.AdjustedMaxTokens
        a.reasoningBudget = preflight.AdjustedReasoning
        req.MaxTokens = a.maxTokens
    }
    return preflight.TransactionID, nil
}
```

**Verification:** green. **Mark `[x]`.**

### Step 4.4: Extract `handlePrefillRejection` + `handleEmptyStream` [ ]

```go
func (a *Agent) handlePrefillRejection(
    ctx context.Context, history []proxy.Message, tools []proxy.Tool,
) (<-chan *proxy.ChatResponse, error) {
    a.prefillDisabled = true
    a.notifyPrefillDisabled()
    prepared := a.prepareMessages(history)
    if len(tools) > 0 {
        prepared = a.injectToolInstructions(prepared, tools)
    }
    req := proxy.ChatRequest{
        Messages:        prepared,
        MaxTokens:       a.maxTokens,
        Temperature:     0.1,
        ReasoningBudget: a.maxTokens / 4,
    }
    return a.client.Stream(ctx, req)
}

func (a *Agent) handleEmptyStream(
    ctx context.Context, history []proxy.Message,
    tools []proxy.Tool, llmTools []proxy.Tool,
) (proxy.Message, error) {
    history = injectRetryContext(history)
    if llmTools != nil {
        a.logger.Info("empty response with native tools, retrying in XML mode")
        a.notifyLifecycle("fallback_started", map[string]any{
            "reason": "empty stream with native tools", "mode": "xml",
        })
        savedNative := a.useNativeTools
        savedSuppress := a.suppressReasoningBudget
        a.useNativeTools = false
        a.suppressReasoningBudget = true
        msg, err := a.computeNextResponseStreamXML(ctx, history, tools)
        a.useNativeTools = savedNative
        a.suppressReasoningBudget = savedSuppress
        return msg, err
    }
    return a.computeNextResponseNonStreaming(ctx, history, tools)
}
```

**Verification:** green. **Mark `[x]`.**

### Step 4.5: Final Refactored `computeNextResponse` [ ]

**⚠️ CRITICAL: Variable Shadowing Risk in Defer Block**

The budget refund defer block evaluates `streamErr` at function exit to decide whether to refund. If you use short variable declaration (`:=`) in any retry branch, e.g.:

```go
ch, streamErr := a.handlePrefillRejection(ctx, history, tools) // BAD — shadows outer streamErr
```

the outer `streamErr` stays `nil` and the defer sees the wrong value — the refund either fires when it shouldn't or misses when it should.

**Rule:** Always use direct assignment (`=`) for retry assignments to `streamErr` and `ch`. Never `:=`.

```go
// ✅ CORRECT
var ch <-chan *proxy.ChatResponse
var streamErr error
ch, streamErr = a.client.Stream(ctx, req)
// ...
if streamErr != nil {
    ch, streamErr = a.handlePrefillRejection(ctx, history, tools) // = not :=
}
```

Also preserve exact log messages (`.Info`, `.Warn` strings) inside the defer block:

```go
defer func() {
    if streamErr != nil && a.orch != nil && a.orch.Budget != nil && txnID != "" {
        // Log messages must match original code exactly
        a.logger.Warn("ICU refund failed", "error", err)
    }
}()
```

After extraction and with the shadowing rule applied, replace the 178-line body with the ~50-line orchestrated version:

```go
func (a *Agent) computeNextResponse(
    ctx context.Context, history []proxy.Message, tools []proxy.Tool,
) (proxy.Message, error) {
    llmTools := tools
    if !a.useNativeTools {
        llmTools = nil
    }

    prepared, prefill := a.prepareMessagesForTurn(history, tools, llmTools)
    isAutoCtx := a.findAutomationCtx(history)
    req := a.buildChatRequest(prepared, llmTools, isAutoCtx)

    txnID, pfErr := a.doPreflightCheck(ctx, history, &req)
    if pfErr != nil {
        return proxy.Message{}, pfErr
    }

    ch, streamErr := a.client.Stream(ctx, req)
    a.logger.Info("stream request sent", "model", a.modelName,
        "max_tokens", a.maxTokens, "tool_choice", req.ToolChoice)

    if a.orch != nil && a.orch.Budget != nil && txnID != "" {
        defer func() {
            if streamErr != nil {
                a.orch.Budget.Refund(ctx, txnID)
            }
        }()
    }

    if streamErr != nil {
        if prefill != "" && isPrefillThinkingError(streamErr) {
            a.logger.Info("prefill rejected by server, retrying without prefill")
            ch, streamErr = a.handlePrefillRejection(ctx, history, tools)
            prefill = ""
        }
        if streamErr != nil {
            a.logger.Warn("streaming not supported, falling back to non-streaming")
            return a.computeNextResponseNonStreaming(ctx, history, tools)
        }
    }

    var fullMsg proxy.Message
    fullMsg.Role = proxy.AssistantRole
    if streamErr = a.processStream(ctx, ch, &fullMsg); streamErr != nil {
        return proxy.Message{}, streamErr
    }

    if prefill != "" {
        fullMsg.Content = prefill + fullMsg.Content
    }

    a.logger.Info("stream completed",
        "content_len", len(fullMsg.Content),
        "reasoning_len", len(fullMsg.ReasoningContent),
        "tool_calls", len(fullMsg.ToolCalls))

    if t := GetUsageTracker(ctx); t != nil {
        t.AddLLMCall(len(prepared), len(fullMsg.Content), len(fullMsg.ReasoningContent))
    }

    if fullMsg.Content == "" && len(fullMsg.ToolCalls) == 0 {
        return a.handleEmptyStream(ctx, history, tools, llmTools)
    }

    return fullMsg, nil
}
```

**CRITICAL:** Verify EVERY line matches the original behavior. The budget refund defer must fire on stream errors, the prefill retry must work, the empty-stream fallback must chain correctly. Compare the extracted helpers side-by-side with the original code.

**Verification:** `go build ./... && go test ./internal/core/assistant/... -count=1` — green. **Mark all Step 4.x `[x]`.**

---

## 10. Phase 5: Clean Up Remaining Functions

### Step 5.1: Clean Up `processStream` (126 → ~60) [ ]

Extract two helpers:

```go
func (a *Agent) checkStreamStuck(fullMsg *proxy.Message) bool {
    if a.skipStuckCheck || len(fullMsg.Content) > 0 || len(fullMsg.ToolCalls) > 0 {
        return false
    }
    return len(fullMsg.ReasoningContent) > a.stuckThreshold()
}

func (a *Agent) tryExtractToolCallFromReasoning(fullMsg *proxy.Message) bool {
    if len(fullMsg.ReasoningContent) == 0 {
        return false
    }
    if !toolCallInContent.MatchString(fullMsg.ReasoningContent) {
        return false
    }
    cleaned := cleanReasoningContent(fullMsg.ReasoningContent)
    if cleaned == "" {
        return false
    }
    fullMsg.Content = cleaned
    return true
}
```

Replace the inline stuck-detection and tool-call-extraction blocks with calls to these helpers inside `processStream`.

**Verification:** green. **Mark `[x]`.**

### Step 5.2: Clean Up `processToolCalls` (115 → ~60) [ ]

Extract guardrail resolution logic:

```go
func (a *Agent) resolveGuardrail(ctx context.Context, tc proxy.ToolCall) (approved, stopBatch bool) {
    if err := a.guardrails.ValidateToolCall(ctx, tc, a.workspaceID); err != nil {
        a.logger.Warn("guardrail check rejected", "name", tc.Function.Name, "error", err)
        a.notifyGuardrailViolation(tc.Function.Name, err)

        if a.onGuardrail == nil {
            return false, true
        }

        decision, decErr := a.onGuardrail(ctx, GuardrailBlockedPayload{
            DecisionID: fmt.Sprintf("gr_%d", time.Now().UnixNano()),
            Tool:       tc.Function.Name,
            Args:       tc.Function.Arguments,
            Reason:     err.Error(),
            Category:   toolCategory(tc.Function.Name),
        })
        if decErr == nil && decision.Allow {
            if decision.Persist {
                a.guardrails.PersistOverride(a.workspaceID,
                    toolCategory(tc.Function.Name), tc.Function.Name, tc.Function.Arguments)
            }
            return true, false
        }
        return false, true
    }
    return false, false
}
```

The `processToolCalls` for-loop body becomes:
```go
for _, tc := range msg.ToolCalls {
    // ... context check ...
    // ... validation ...
    approved, stopBatch := a.resolveGuardrail(ctx, tc)
    if stopBatch {
        a.appendToolResult(history, tc, formatGuardrailError(...))
        return nil
    }
    // ... execution ...
}
```

**Verification:** green. **Mark `[x]`.**

### Step 5.3: Clean Up `computeNextResponseNonStreaming` (138 → ~60) [ ]

Reuse `prepareMessagesForTurn` and `buildChatRequest` from Phase 4. Extract tool-support error fallback:

```go
func (a *Agent) retryWithoutTools(ctx context.Context, history []proxy.Message) (proxy.ChatResponse, error) {
    chatCtx, cancel := context.WithTimeout(ctx, AgentRetryTimeout)
    defer cancel()
    return a.client.Chat(chatCtx, proxy.ChatRequest{
        Messages:  history,
        MaxTokens: a.maxTokens,
    })
}
```

Replace duplicated preparation/prefill/request-building blocks with calls to the Phase 4 helpers.

**Verification:** green. **Mark `[x]`.**

### Step 5.4: Clean Up `computeNextResponseStreamXML` (83 → ~50) [ ]

Reuse `prepareMessagesForTurn` from Phase 4. Extract the prefill rejection retry pattern (identical to `computeNextResponse`'s retry). This function shrinks naturally after reusing Phase 4 helpers.

**Verification:** green. **Mark `[x]`.**

### Step 5.5: Refactor `InitializeAgentStack` (173 → ~40) [ ]

Extract tool initialization sub-functions in `registry.go`:

```go
func initTerminalTools(appCtx, persistence, resolver, shellManager, observer, defaultGuardrails) *tools.TerminalTools { ... }
func initCommunicationTools(appCtx) *tools.CommunicationTools { ... }
func initSearchTools(appCtx, network) *tools.InternetTools { ... }
func initNetworkTools(appCtx, persistence, defaultGuardrails, logger) *tools.NetworkTools { ... }
func initFileSystemTools(appCtx, persistence, resolver, defaultGuardrails) *tools.FileSystemTools { ... }
```

Each init function is ~20 lines. `InitializeAgentStack` becomes:
```go
func InitializeAgentStack(appCtx, persistence, mcp, logger, shellManager, observer) (...) {
    resolver := appCtx.Resolver()
    defaultGuardrails := appCtx.GetGuardrails()

    terminal := initTerminalTools(...)
    grEngine := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
        return defaultGuardrails
    }, resolver, persistence)
    comm := initCommunicationTools(...)
    network := initNetworkTools(...)
    search := initSearchTools(...)
    fsTools := initFileSystemTools(...)

    localRegistry := NewLocalToolRegistry(terminal, comm, search, fsTools, network)
    provider := NewMultiToolProvider(false, localRegistry, mcp)
    mcpEngine := NewEngine(mcp, logger)
    engine := NewCompositeEngine(localRegistry, mcpEngine)

    return provider, engine, grEngine
}
```

**Verification:** `go build ./... && go test ./internal/core/assistant/... -count=1` — green. **Mark `[x]`.**

**After Phase 5 is fully green, mark all Step 5.x items `[x]` in the Progress Tracker.**

---

## 11. Phase 6: Guardrails Subpackage (Deferred)

**Current coverage: 36.3%.** The guardrail engine is a separate `guardrails/` subpackage. This should be treated as its own project:

1. Improve test coverage to >70% before refactoring
2. `ValidateToolCall` (63 lines) could be broken down by tool category
3. `validateGlobal`, `validateTerminal`, etc. are already well-separated

**Do not begin this phase until Phases 0-5 are complete and stable.**

---

## 12. Verification Gates

### After EVERY individual file change:

```bash
cd backend
go build ./... && echo "BUILD: OK" || echo "BUILD: FAIL — REVERT"
go test ./internal/core/assistant/... -count=1 && echo "TESTS: OK" || echo "TESTS: FAIL — REVERT"
```

### After each Phase completes:

```bash
cd backend

# 1. Full build
go build ./...

# 2. All assistant tests (must be green, 0 failures)
go test ./internal/core/assistant/... -count=1 -v

# 3. No stale imports or vet issues
go vet ./internal/core/assistant/...

# 4. Orchestrator tests (should be unchanged)
go test ./internal/core/orchestrator/... -count=1

# 5. World build (catches consumer compilation errors)
go build ./...

# 6. Full vet
go vet ./...

# 7. Record-replay tests
go test -tags recordreplay ./internal/core/assistant/... -v -count=1

# 8. Coverage report (should not decrease from baseline)
go test -coverprofile=/tmp/assistant_cover.out ./internal/core/assistant/... 2>&1
go tool cover -func=/tmp/assistant_cover.out | tail -1
```

### Coverage baseline (pre-refactor, after Step 0.1 fixes):

Save the total coverage % at the end of Step 0.1. After each Phase, coverage must stay **at or above** this baseline. If it drops, a code path was broken.

---

## 13. Risk Register

| Risk | Likelihood | Mitigation |
|---|---|---|
| `handleNoToolCalls` method move breaks test | High | Search `agent_test.go` for direct calls BEFORE changing signature |
| Stale import in `agent.go` (`storage` package) | Medium | Remove during file split |
| Coverage drops after extraction | Medium | Run coverage report after each step |
| `content_filter.go` removal before `stream.go` created | Low | Defer merge to Step 1.3 when `stream.go` is created |
| `strategy_plan.go` removal before `tool_exec.go` created | Low | Defer merge to Step 1.4 when `tool_exec.go` is created |
| Concurrent test flaky | Low | `TestUsageTracker_Concurrency` — not touched |
| Budget refund defer logic breaks | Medium | Phase 4 handles this — verify the `streamErr` capture correctly |
| Variable shadowing of `streamErr` in Phase 4 retry blocks | High | Always use `=` not `:=` for retry assignments — documented in Phase 4 step 4.5 |

---

## Appendix A: Function → Target File Map

Shows where each function ends up after ALL phases complete:

| Function | Phase | Final File |
|---|---|---|
| `NewAgent`, `applyDefaults`, `repetitionDetector.check` | 0.2c | `agent.go` |
| `prepareMessages`, `injectToolInstructions`, `injectNativeToolReference` | stays | `agent.go` |
| `ProviderTiers`, `ProviderTuningDefaults` | 0.2c | `agent.go` |
| `GuardrailDecisionStore`, `NewGuardrailDecisionCallback` | 0.2b | `agent.go` |
| `Execute` (thin wrapper) | 2.2 | `agent.go` |
| `runSession.run()`, `handleContextSizeError`, `handleToolCallParseError`, `resetParseErrorState`, `trimLargeWriteContent`, `checkSubmitFinalAnswer` | 2.2-2.3 | `session.go` |
| `executeTurn`, `handleContentToolCalls`, `precededByToolResult`, `countConsecutiveChat`, `isPrematureTermination` | 1.5 | `session.go` |
| `handleNoToolCalls` | 2.4 | `session.go` (on `*runSession`) |
| `HistorySieve`, `physicalSieve`, `reactiveSieve`, `aggressiveSieve` | 3.1-3.2 | `sieve.go` |
| `applyPhysicalSieve`, `applyReactiveSieve`, `applyAggressiveSieve` (wrappers) | 3.3 | `sieve.go` |
| `truncateLongContent` | 1.2 | `sieve.go` |
| `computeNextResponse`, `buildChatRequest`, `prepareMessagesForTurn`, `doPreflightCheck`, `handlePrefillRejection`, `handleEmptyStream` | 4.1-4.5 | `stream.go` |
| `processStream`, `checkStreamStuck`, `tryExtractToolCallFromReasoning` | 5.1 | `stream.go` |
| `computeNextResponseNonStreaming`, `retryWithoutTools` | 5.3 | `stream.go` |
| `computeNextResponseStreamXML` | 5.4 | `stream.go` |
| `injectRetryContext`, `findAutomationCtx`, `shouldPrefill`, `stuckThreshold`, `cleanReasoningContent` | 1.3 | `stream.go` |
| `FilterStreamingMarkup`, `normalizeContent` | 0.2d | `stream.go` |
| `processToolCalls`, `resolveGuardrail` | 5.2 | `tool_exec.go` |
| `executePlan`, `ExecutionPlan`, `ExecutionPlanStrategy`, `NewExecutionPlanStrategy`, `Generate` | 1.4+0.2e | `tool_exec.go` |
| `validateToolArgs`, `appendToolResult`, `toolCategory`, `formatGuardrailError`, `extractTaskSummary` | 1.4 | `tool_exec.go` |
| `parseErrorKind`, `is*Error` (6 functions) | 1.1 | `errors.go` |
| `Engine`, `assistantEngine`, `NewEngine`, `ToolProvider`, `MultiToolProvider`, `CompositeEngine` | 0.2a | `tool_provider.go` |
| `initTerminalTools`, `initCommunicationTools`, etc. | 5.5 | `registry.go` |
| `InitializeAgentStack` (reduced) | 5.5 | `registry.go` |
| `LocalToolRegistry`, `NewLocalToolRegistry`, tool registration | stays | `registry.go` |

## Appendix B: Test File Map

| Test File | Tests What | Phases Affected |
|---|---|---|
| `agent_test.go` (2670L) | Agent Execute, LLM calls, sieves, tool calls, errors | 0-5 |
| `event tests` (in agent_events.go) | Event notification | Unchanged |
| `engine_test.go` | Engine interface (tests stay — engine.go merged into tool_provider.go) | 0.2a |
| `tool_provider_test.go` | MultiToolProvider, CompositeEngine | Unchanged |
| `registry_test.go` | LocalToolRegistry, InitializeAgentStack | 5.5 |
| `guardrail_decision_test.go` | Guardrail decisions (tests stay — file merged into agent.go) | 0.2b |
| `strategy_plan_test.go` | ExecutionPlanStrategy (tests stay — file merged into tool_exec.go) | 0.2e |
| `content_filter_test.go` | FilterStreamingMarkup (tests stay — file merged into stream.go) | 0.2d |
| `usagetracker_test.go` | UsageTracker | Unchanged |
| `guardrails/guardrails_test.go` | Guardrail validation | Phase 6 |
| `agent_recording_test.go` | Record-replay integration | Unchanged |
