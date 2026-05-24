# Agent Improvements Plan

Merged findings from [OpenClaw](https://github.com/openclaw/openclaw) and [agent-sdk-go](https://github.com/Ingenimax/agent-sdk-go) into 7 independent, testable, non-breaking phases.

---

## Phase 1: Custom Execute/Stream Functions

**Source:** agent-sdk-go — `CustomRunFunction`, `CustomRunStreamFunction`

**Goal:** Allow the agent loop to be replaced entirely by setting a function on `AgentOptions`. Enables recording playback, testing, and custom execution without forking.

### Changes

**`internal/core/assistant/agent.go`** — `AgentOptions` struct:
```go
type AgentOptions struct {
    // ... existing fields

    // CustomExecuteFunc, when set, replaces the default agent loop entirely.
    // The function receives the execution context and message history, and
    // returns the final reply text, the complete history, and any error.
    CustomExecuteFunc func(ctx context.Context, history []proxy.Message, agent *Agent) (finalReply string, fullHistory []proxy.Message, err error)

    // CustomStreamFunc, when set, replaces the default streaming path.
    CustomStreamFunc func(ctx context.Context, history []proxy.Message, agent *Agent) (<-chan AgentEvent, error)
}
```

**`internal/core/assistant/agent.go`** — `Execute()` method:
```go
func (a *Agent) Execute(ctx context.Context, history []proxy.Message) (string, []proxy.Message, error) {
    if a.opts.CustomExecuteFunc != nil {
        return a.opts.CustomExecuteFunc(ctx, history, a)
    }
    // ... existing loop
}
```

**`internal/core/assistant/agent.go`** — `processStream()` or wherever streaming is initiated:
```go
if a.opts.CustomStreamFunc != nil {
    return a.opts.CustomStreamFunc(ctx, history, a)
}
// ... existing stream handling
```

### Tests
- Custom func is called when set → verify it receives the right inputs
- Custom func is skipped when nil → verify existing loop runs
- CustomStreamFunc follows the same contract

### Why
Recording playback hooks in here without touching the agent loop. The `PlaybackBridge` wraps a `recordings.PlaybackClient` as `proxy.Client`, and the custom execute function just calls `agent.Execute()` with that client — no agent loop changes needed.

---

## Phase 2: Guardrail Middleware Wrapping

**Source:** agent-sdk-go — `LLMMiddleware` wrapping `interfaces.LLM`

**Goal:** Wrap `proxy.Client` so every LLM call is automatically guarded at the transport layer. Eliminates the risk of unguarded code paths.

### Changes

**`internal/core/assistant/guardrails/llm_middleware.go`** — new file:
```go
package guardrails

type LLMGuardrailMiddleware struct {
    client   proxy.Client
    pipeline *GuardrailPipeline  // request + response pipeline
}

func NewLLMGuardrailMiddleware(client proxy.Client, pipeline *GuardrailPipeline) *LLMGuardrailMiddleware {
    return &LLMGuardrailMiddleware{client: client, pipeline: pipeline}
}

func (m *LLMGuardrailMiddleware) Chat(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
    guardedReq, err := m.pipeline.ProcessRequest(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("guardrail blocked request: %w", err)
    }
    resp, err := m.client.Chat(ctx, guardedReq)
    if err != nil {
        return nil, err
    }
    guardedResp, err := m.pipeline.ProcessResponse(ctx, resp)
    if err != nil {
        return nil, fmt.Errorf("guardrail blocked response: %w", err)
    }
    return guardedResp, nil
}

func (m *LLMGuardrailMiddleware) Stream(ctx context.Context, req proxy.ChatRequest) (<-chan *proxy.ChatResponse, error) {
    guardedReq, err := m.pipeline.ProcessRequest(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("guardrail blocked request: %w", err)
    }
    // Pass through to underlying stream — response guardrails apply per-chunk
    return m.client.Stream(ctx, guardedReq)
}
```

**`internal/app/bootstrap.go`** — wrap client factory when guardrails are active:
```go
factory := func(baseURL, model string, headers http.Header) proxy.Client {
    client := proxy.NewLLMClient(baseURL, model, nil, headers)
    if c.RecordDir != "" {
        client = recorder.New(client, c.RecordDir, model)
    }
    if s.guardrailEngine != nil {
        client = guardrails.NewLLMGuardrailMiddleware(client, s.guardrailEngine.Pipeline())
    }
    return client
}
```

**`internal/core/assistant/agent.go`** — remove inline guardrail calls after verifying they're covered by middleware. This is a cleanup pass — do it only after the middleware is proven in production.

### Tests
- Mock `proxy.Client` + mock guardrail pipeline that tracks calls
- `Chat()`: guards called before LLM, guards called after LLM
- Block action returns error, Warn passes through
- `Stream()`: request guard called, response not modified

---

## Phase 3: Fallback Retry with Context Injection

**Source:** OpenClaw — `resolveFallbackRetryPrompt()`

**Goal:** When the agent retries after an empty-stream failure, inject a `[Retry]` marker so the model knows it's a retry and doesn't re-enter the same reasoning loop.

**Note on thinking content format compatibility:** Different providers format thinking/reasoning content blocks differently across their SSE responses (Anthropic uses `thinking` vs `reasoning_content` signatures, OpenAI uses `reasoning` fields, Gemini uses `thinking` blocks). The retry path in `processStream()` is the natural place to normalize these variations — if a model sends unexpected thinking content that triggers a false positive on reasoning-stuck detection, the retry injects context and retries non-streaming (which bypasses the problematic SSE parsing path entirely). This is already handled by the progressive sieve recovery mechanism, so no additional code change is needed beyond what Phase 3 describes.

### Changes

**`internal/core/assistant/agent.go`** — in the empty-response fallback path:
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

**`internal/core/assistant/agent.go`** — `injectRetryContext()` helper:
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

**`internal/core/assistant/agent.go`** — initialize + instrument:
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

## Phase 6: Execution Plan Strategy

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

**`internal/core/assistant/agent.go`** — integration:
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

## Phase 7: ResponseFormat + Sub-Agent Auto-Wrap

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

**`internal/core/proxy/client.go`** — pipe through to HTTP body:
```go
if req.ResponseFormat != nil {
    body["response_format"] = req.ResponseFormat
}
```

Already works for OpenAI-compatible APIs. For others, the field is omitted by `omitempty`.

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
- Sub-agent tool has correct name/description from agent metadata

---

## Phase 8: Anthropic Prompt Caching

**Source:** agent-sdk-go — `CacheConfig`

**Goal:** Reduce latency and token costs for Anthropic provider by marking system prompts and tool definitions with `cache_control` breakpoints. Repeated calls reuse cached prefixes.

### Changes

**`models/config.go`** — add `CacheConfig` to model settings:
```go
type CacheConfig struct {
    SystemMessage bool   `yaml:"cache_system_message,omitempty" json:"cache_system_message,omitempty"`
    Tools         bool   `yaml:"cache_tools,omitempty"           json:"cache_tools,omitempty"`
    Conversation  bool   `yaml:"cache_conversation,omitempty"    json:"cache_conversation,omitempty"`
    TTL           string `yaml:"cache_ttl,omitempty"             json:"cache_ttl,omitempty"` // e.g. "5m"
}
```

**`internal/core/proxy/client.go`** — in the Anthropic provider branch, inject cache control markers:
```go
if req.AnthropicCache != nil {
    if req.AnthropicCache.SystemMessage {
        body["system"] = append([]map[string]interface{}{{
            "type": "text",
            "text": req.SystemPrompt,
            "cache_control": {"type": "ephemeral"},
        }}, existingSystem...)
    }
    if req.AnthropicCache.Tools && len(tools) > 0 {
        tools[len(tools)-1]["cache_control"] = map[string]string{"type": "ephemeral"}
    }
}
```

Note: Anthropic's API uses `cache_control: { type: "ephemeral" }` on the last system message block and/or last tool definition. The proxy just needs to inject these markers when the model config has caching enabled.

### Tests
- No `CacheConfig` set → no markers injected (backwards compatible)
- Caching enabled → `cache_control` appears in last system block and/or last tool
- Anthropic provider only; other providers skip this logic

---

## What We Skip

| Dropped Item | Reason |
|-------------|--------|
| OpenClaw char-budget context trimming | `budget_squeezer.go` already handles this with token budgets |
| OpenClaw provider-specific streaming | Our `client.go` dispatches by provider; marginal improvement |
| OpenClaw result-classified fallback | Requires new error classification types + routing. Save for v2. |
| agent-sdk-go YAML config hierarchy | Our config system works for our use case |
| agent-sdk-go memory interface | We have history management; pluggable memory adds complexity without clear benefit |

---

## Implementation Order

```
Phase 1 (Custom funcs) → Phase 3 (Retry injection) → Phase 4 (Tool dedup)
    → Phase 5 (UsageTracker) → Phase 2 (Guardrail middleware)
    → Phase 7 (ResponseFormat + auto-wrap) → Phase 6 (Execution plan) → Phase 8 (Caching)
```

Phases 1, 3, and 4 can be done in parallel.
Phase 5 touches both agent and executor — needs coordination.
Phase 2 requires confirming no code path bypasses guardrails.
Phase 6 is the most involved — needs careful design of the plan prompt.
Phase 7 and Phase 4 both touch `tool_provider.go` — do Phase 4 first then Phase 7, or merge carefully.
Phase 8 is independent and can go anywhere after proxy client changes settle.

---

## Files Changed (complete list)

```
internal/core/assistant/agent.go
internal/core/assistant/tool_provider.go
internal/core/assistant/usagetracker.go              (new — Phase 5)
internal/core/assistant/prompts/templates.go
internal/core/assistant/guardrails/llm_middleware.go   (new — Phase 2)
internal/core/assistant/strategy_plan.go               (new — Phase 6)
internal/core/automation/executor.go
internal/core/proxy/client.go                          (Phase 7 + Phase 8)
internal/app/bootstrap.go
models/config.go                                       (Phase 6 + Phase 8)
models/llm_messages.go                                 (Phase 7)
```

Frontend (dashboard model form):
```
frontend/src/types/models.ts        (if enable_execution_plan shown)
frontend/src/components/...         (model settings toggle)
```
