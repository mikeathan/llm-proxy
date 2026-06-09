---
status: partial
related_specs: [SPEC-001]
---
# Agent Improvements Plan

Merged findings from [OpenClaw](https://github.com/openclaw/openclaw) and [agent-sdk-go](https://github.com/Ingenimax/agent-sdk-go) into 7 independent, testable, non-breaking phases.

---

## ~~Phase 1: Custom Execute/Stream Functions~~ (REMOVED)

**Why removed:** `CustomExecuteFunc` and `CustomStreamFunc` were injection points on `AgentOptions` that bypassed the entire agent loop. They were never wired by any production caller — only tests used them. Recording and playback were instead implemented at the transport layer via `RecordingClient` and `PlaybackBridge` (both wrapping `proxy.Client`), which is a cleaner separation. The fields and guard checks have been removed from `agent.go`.

---

## ~~Phase 2: Guardrail Middleware Wrapping~~ (REMOVED)

**Why removed:** The `LLMGuardrailMiddleware` was implemented as a decorator wrapper on `proxy.Client` but its `ProcessRequest`/`ProcessResponse` pipeline methods were no-ops — they never called `GuardrailEngine.ValidateToolCall()`. The real guardrail enforcement remained inline in the agent loop (`agent.go:1243`). Additionally, the middleware operated at the `ChatRequest`/`ChatResponse` level (transport layer), while guardrails validate individual `ToolCall` objects after parsing — a fundamental abstraction mismatch that couldn't be bridged without redundant dual enforcement. The interactive guardrail-decision flow (user approve/deny/persist) is also tightly coupled to the agent loop and can't live in a transport wrapper. The files have been deleted; the inline enforcement in `agent.go` is the canonical path.

---

## Phase 3: Fallback Retry with Context Injection

**Source:** OpenClaw — `resolveFallbackRetryPrompt()`

**Goal:** When the agent retries after an empty-stream failure, inject a `[Retry]` marker so the model knows it's a retry and doesn't re-enter the same reasoning loop.

**Note on thinking content format compatibility:** Different providers format thinking/reasoning content blocks differently across their SSE responses (Anthropic uses `thinking` vs `reasoning_content` signatures, OpenAI uses `reasoning` fields, Gemini uses `thinking` blocks). The retry path in `processStream()` is the natural place to normalize these variations — if a model sends unexpected thinking content that triggers a false positive on reasoning-stuck detection, the retry injects context and retries non-streaming (which bypasses the problematic SSE parsing path entirely). This is already handled by the progressive sieve recovery mechanism, so no additional code change is needed beyond what Phase 3 describes.

### Changes

**`internal/core/assistant/agent.go                          (Phase 1 custom funcs removed)`** — in the empty-response fallback path:
```go
// During processStream(), when stream returns empty:
// current code: retries silently via Chat()
// new code:
if isRetry && len(history) > 0 {
    history[len(history)-1].Content = injectRetryContext(history[len(history)-1].Content,
        "Retry after previous attempt returned empty response.")
}
```

**`internal/core/assistant/prompts/templates.go`** — add retry signal template:
```go
const RetrySignal = "[Retry after the previous model attempt failed or timed out]"
```

**`internal/core/assistant/agent.go                          (Phase 1 custom funcs removed)`** — `injectRetryContext()` helper:
```go
func injectRetryContext(userContent string, signal string) string {
    return fmt.Sprintf("%s\n\n%s", signal, userContent)
}
```

### Tests
- Mock LLM returns empty stream, then succeeds on retry
- Verify retry context appears in history on second call
- Verify it does NOT appear on the first call
- Verify it works with the progressive sieve recovery (stacks correctly)

---

## Phase 4: Tool Deduplication

**Source:** agent-sdk-go — `deduplicateTools()`

**Goal:** Prevent duplicate tool definitions when MCP servers re-list tools already registered locally.

### Changes

**`internal/core/assistant/tool_provider.go`** — add dedup in `ListTools()`:
```go
func (r *LocalToolRegistry) ListTools(ctx context.Context) ([]proxy.Tool, error) {
    tools, err := r.providers.ListTools(ctx)
    if err != nil {
        return nil, err
    }
    return deduplicateTools(tools), nil
}

func deduplicateTools(tools []proxy.Tool) []proxy.Tool {
    seen := make(map[string]bool, len(tools))
    result := make([]proxy.Tool, 0, len(tools))
    for _, t := range tools {
        if !seen[t.Function.Name] {
            seen[t.Function.Name] = true
            result = append(result, t)
        }
    }
    return result
}
```

Apply to `MultiToolProvider.ListTools()` as well if it exists.

### Tests
- No duplicates → unchanged ordering
- All duplicates → single entry
- Partial overlap (MCP + local share some tools) → unique set
- First occurrence wins (later dups dropped)

---

## Phase 5: UsageTracker

**Source:** agent-sdk-go — context-scoped `UsageTracker`

**Goal:** Replace ad-hoc metrics with a composable tracker. Track tokens, LLM calls, tool calls, execution time.

### Changes

**`internal/core/assistant/usagetracker.go`** — new file:
```go
package assistant

type UsageTracker struct {
    InputTokens      int
    OutputTokens     int
    ReasoningTokens  int
    LLMCalls         int
    ToolCalls        int
    UsedTools        []string
    ExecutionTime    time.Duration
    mu               sync.Mutex
}

type usageKey struct{}

func withUsageTracker(ctx context.Context) context.Context {
    return context.WithValue(ctx, usageKey{}, &UsageTracker{})
}

func GetUsageTracker(ctx context.Context) *UsageTracker {
    if t, ok := ctx.Value(usageKey{}).(*UsageTracker); ok {
        return t
    }
    return nil
}

func (t *UsageTracker) AddLLMCall(inputTokens, outputTokens, reasoningTokens int) {
    t.mu.Lock()
    defer t.mu.Unlock()
    t.LLMCalls++
    t.InputTokens += inputTokens
    t.OutputTokens += outputTokens
    t.ReasoningTokens += reasoningTokens
}

func (t *UsageTracker) AddToolCall(name string) {
    t.mu.Lock()
    defer t.mu.Unlock()
    t.ToolCalls++
    t.UsedTools = append(t.UsedTools, name)
}
```

**`internal/core/assistant/agent.go                          (Phase 1 custom funcs removed)`** — initialize + instrument:
```go
// At start of Execute():
ctx = withUsageTracker(ctx)

// After each LLM call:
if t := GetUsageTracker(ctx); t != nil {
    t.AddLLMCall(resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.ReasoningTokens)
}

// After each tool call:
if t := GetUsageTracker(ctx); t != nil {
    t.AddToolCall(toolName)
}
```

**`internal/core/automation/executor.go`** — read after `agent.Execute()`:
```go
finalReply, fullHistory, agErr := agent.Execute(execCtx, history)
if t := assistant.GetUsageTracker(execCtx); t != nil {
    procLog.Info("Usage", "llm_calls", t.LLMCalls, "tool_calls", t.ToolCalls,
        "input_tokens", t.InputTokens, "output_tokens", t.OutputTokens)
}
```

### Tests
- `AddLLMCall()` increments counters correctly with concurrency
- `GetUsageTracker()` returns nil when no tracker in context
- Integration: agent run produces expected counts

---

## Phase 6: Execution Plan Strategy (WIRED — active for automations)

**Source:** agent-sdk-go — `executionplan` package

**Goal:** For deterministic multi-step workflows, generate a plan upfront (DAG of tool steps) and execute sequentially. Reduces LLM iterations from N to 1. Falls back to normal loop on failure.

### Changes

**`internal/core/assistant/strategy_plan.go`** — new file:
```go
package assistant

type ExecutionPlan struct {
    Description string
    Steps       []ExecutionStep
}

type ExecutionStep struct {
    ToolName    string
    Description string
    Input       string
    Parameters  map[string]interface{}
}

type ExecutionPlanStrategy struct {
    llm    proxy.Client
    tools  []proxy.Tool
    prompt string
}

// Generate asks the LLM to produce a plan as JSON.
func (s *ExecutionPlanStrategy) Generate(ctx context.Context, task string) (*ExecutionPlan, error) {
    // build prompt listing available tools with their schemas
    // call LLM, parse JSON response into ExecutionPlan
    // validate tool names + required params exist
}
```

**`internal/core/automation/executor.go`** — wire when `EnableExecutionPlan` is true on the model config:
```go
if cfg.EnableExecutionPlan {
    tools, listErr := e.svc.ToolProvider().ListTools(ctx)
    if listErr == nil && len(tools) > 0 {
        agentOpts.PlanStrategy = assistant.NewExecutionPlanStrategy(client, tools)
    }
}
```

**`internal/core/assistant/agent.go                          (Phase 1 custom funcs removed)`** — integration:
```go
type AgentOptions struct {
    // ... existing
    PlanStrategy *ExecutionPlanStrategy  // nil = normal iterative loop
}

func (a *Agent) Execute(ctx context.Context, history []proxy.Message) (string, []proxy.Message, error) {
    if a.opts.PlanStrategy != nil && len(tools) > 0 {
        plan, err := a.opts.PlanStrategy.Generate(ctx, lastUserMessage)
        if err == nil {
            return a.executePlan(ctx, history, plan)
        }
        // fall through to normal loop on generation failure
    }
    // ... existing loop
}
```

**`models/config.go`** — add to `ModelConfig`:
```go
EnableExecutionPlan bool `yaml:"enable_execution_plan,omitempty" json:"enable_execution_plan,omitempty"`
```

### Tests
- Plan generation succeeds → plan executed, tool order matches
- Plan generation fails → falls back to normal loop
- Unknown tool name → graceful error + fallback
- No tools → plan strategy skipped

---

## Phase 7: ResponseFormat + Sub-Agent Auto-Wrap (PARTIAL — ResponseFormat in struct, pipes via `json.Marshal`; sub-agent wrap not implemented)

**Source:** agent-sdk-go — `ResponseFormat`, sub-agent tool wrapping

### Changes

**`models/llm_messages.go`** — add to `ChatRequest`:
```go
type ResponseFormat struct {
    Type   string      `json:"type,omitempty"`
    Name   string      `json:"name,omitempty"`
    Schema interface{} `json:"schema,omitempty"`
}

type ChatRequest struct {
    // ... existing fields
    ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}
```

No explicit piping needed — `client.go` does `json.Marshal(req)` where `ChatRequest.ResponseFormat` has `json:"response_format,omitempty"` — it serializes automatically when set.

**`internal/core/assistant/tool_provider.go`** — sub-agent auto-wrap:
```go
func NewLocalToolRegistry(providers ToolProvider, subAgents []*Agent) *LocalToolRegistry {
    tools := providers.ListTools()
    for _, sa := range subAgents {
        wrapped := wrapAgentAsTool(sa)
        tools = append(tools, wrapped)
    }
    return &LocalToolRegistry{tools: deduplicateTools(tools), providers: providers}
}
```

### Tests
- `ResponseFormat` appears in HTTP body JSON when set

---

## What We Skip

| Dropped Item | Reason |
|-------------|--------|
| OpenClaw char-budget context trimming | `budget_squeezer.go` already handles this with token budgets |
| OpenClaw provider-specific streaming | Our `client.go` dispatches by provider; marginal improvement |
| OpenClaw result-classified fallback | Requires new error classification types + routing. Save for v2. |
| agent-sdk-go YAML config hierarchy | Our config system works for our use case |
| agent-sdk-go memory interface | We have history management; pluggable memory adds complexity without clear benefit |
| Anthropic prompt caching (Phase 8) | Agent is provider-agnostic; Anthropic-specific cache_control markers don't belong |
| Sub-agent auto-wrap (Phase 7) | Not needed until sub-agents exist; trivially addable later (~50-80 lines) |

---

## Implementation Order

```
Phase 3 (Retry injection) → Phase 4 (Tool dedup)
    → Phase 5 (UsageTracker) → Phase 6 (Execution plan)
    → Phase 7 (ResponseFormat — partial)
```

Phases 3, 4, 5 are implemented and live.
Phase 6 is wired (automation executor) — enable via model config `enable_execution_plan: true`.
Phase 7: ResponseFormat in struct, auto-piped via `json.Marshal` — done.

---

## Files Changed (complete list)

```
internal/core/assistant/agent.go                          (Phase 1 custom funcs removed)
internal/core/assistant/tool_provider.go
internal/core/assistant/usagetracker.go              (new — Phase 5)
internal/core/assistant/prompts/templates.go
internal/core/assistant/guardrails/llm_middleware.go   (~~new — Phase 2 — removed~~)
internal/core/assistant/strategy_plan.go               (new — Phase 6)
internal/core/automation/executor.go
internal/core/proxy/client.go                          (Phase 7)
internal/app/bootstrap.go
models/config.go                                       (Phase 6)
models/llm_messages.go                                 (Phase 7)
```

Frontend (dashboard model form):
```
frontend/src/types/models.ts        (if enable_execution_plan shown)
frontend/src/components/...         (model settings toggle)
```
