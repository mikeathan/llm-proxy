---
status: reference
last_reviewed: 2026-07-03
---

# Backend Audit Report
_Scope: bugs, memory leaks, bottlenecks, and optimisations_
_Files reviewed: agent.go · session.go · stream.go · tool_exec.go · registry.go · agent_events.go · sieve.go · client.go · history.go · assistant_handlers.go · dispatcher_handlers.go · terminal.go_

---

## 1. Bugs

### 1.1 — Goroutine leak in `Chat()` body-close goroutine  
**File:** [`client.go:95-98`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/proxy/client.go#L95-L98)  
**Severity: High**

```go
go func() {
    <-ctx.Done()
    resp.Body.Close()
}()
```

When the request completes _before_ the context is cancelled (the normal case), this goroutine leaks permanently — it blocks forever on `<-ctx.Done()` and is never collected. At high request rates this accumulates into hundreds of orphaned goroutines.

**Root cause:** There is no `streamDone` channel or `sync.Once`-gated cleanup to release the goroutine when the body has already been read successfully. `defer resp.Body.Close()` already closes it on normal exit; the goroutine is a belt-and-suspenders attempt that was never guarded.

---

### 1.2 — Race condition on `a.maxTokens` / `a.reasoningBudget` mutation during live execution  
**File:** [`stream.go:203-207`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/assistant/stream.go#L203-L207)  
**Severity: Medium**

```go
if preflight.SqueezeFactor < 1.0 {
    a.maxTokens = preflight.AdjustedMaxTokens   // ← mutates Agent field
    a.reasoningBudget = preflight.AdjustedReasoning
    req.MaxTokens = a.maxTokens
}
```

`doPreflightCheck` mutates shared `Agent` struct fields (`maxTokens`, `reasoningBudget`) inside `computeNextResponse`, which is called from the agent loop. If two concurrent SSE observers or the heartbeat goroutine (inside `processStream`) read those fields simultaneously, this is an unguarded data race. The `Agent` struct has no mutex protecting these fields.

---

### 1.3 — `truncateHistory` uses quadratic-time `append` splicing  
**File:** [`assistant_handlers.go:463-467`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/transport/http/assistant_handlers.go#L463-L467)  
**Severity: Medium (correctness / performance)**

```go
for totalChars > maxHistoryChars && startIdx < len(history)-1 {
    totalChars -= len(history[startIdx].Content)
    history = append(history[:startIdx], history[startIdx+1:]...)  // O(n) per iteration
}
```

Each inner `append` copies the entire suffix of `history`. With a long session this is O(n²) in the number of messages. Also, the original slice passed in is mutated: `append` can write beyond `startIdx` in the original backing array even though the caller retains the old slice header. This is a subtle aliasing bug — the session's `History` field may now share backing memory with an in-flight slice.

**Fix pattern:** compute `dropUntil` index in a single O(n) scan, then return `history[dropUntil:]`.

---

### 1.4 — `readSSELine` reads one byte at a time  
**File:** [`client.go:197-212`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/proxy/client.go#L197-L212)  
**Severity: Medium (performance / bottleneck)**

```go
b := make([]byte, 1)
for {
    n, err := r.Read(b)
    ...
    buf = append(buf, b[0])
```

This issues one syscall per byte of every SSE line. A typical tool-call response line is ~200–500 bytes, meaning 200–500 syscalls per line. `bufio.Scanner` or `bufio.Reader.ReadString('\n')` would reduce this to one syscall per line. For high-throughput streaming this is the single biggest CPU bottleneck in the client path.

---

### 1.5 — `processToolCalls` holds `mu` during `appendToolResult` but never uses it as a true mutex  
**File:** [`tool_exec.go:106,143-145,160-179`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/assistant/tool_exec.go#L106)  
**Severity: Low (correctness smell)**

`processToolCalls` creates a `sync.Mutex` at the top of the function and locks it in two places, but the calls it guards (`appendToolResult`) are entirely serial — tool calls are executed one at a time in a plain `for` loop, never concurrently. The mutex provides no protection (there is no other goroutine competing) and adds misleading noise. If execution is ever parallelised in the future, the guards are in the wrong places (they don't cover `engine.ExecuteTool`).

---

### 1.6 — `ListAutomations` calls `ReadState` in a tight loop — N+1 I/O  
**File:** [`dispatcher_handlers.go:118`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/transport/http/dispatcher_handlers.go#L118)  
**Severity: Medium (bottleneck)**

```go
for _, entry := range entries {
    if state, err := h.dispatcher.Persistence().ReadState(entry.Workspace); err == nil {
        ...
    }
}
```

For each automation entry a full workspace state file is read from disk. With N automations across N workspaces this is N disk reads per `GET /automations`. This is called on every frontend poll.

---

### 1.7 — `injectRetryContext` mutates the caller's `history` slice in place  
**File:** [`stream.go:345-353`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/assistant/stream.go#L345-L353)  
**Severity: Low (correctness)**

```go
func injectRetryContext(history []proxy.Message) []proxy.Message {
    for i := len(history) - 1; i >= 0; i-- {
        if history[i].Role == proxy.UserRole {
            history[i].Content = ...   // ← mutates caller's slice element
```

The caller passes `history` by value (slice header), but the slice elements are modified directly. The agent loop's `s.history` backing array is modified without the loop being aware. On the next turn the "Retry:" prefix persists in history, potentially confusing the model.

---

### 1.8 — `findAutomationCtx` scans the full history on every call  
**File:** [`stream.go:672-679`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/assistant/stream.go#L672-L679)  
**Severity: Low (performance)**

`findAutomationCtx` is called at least twice per turn (in `computeNextResponse` and `prepareMessagesForTurn`). It performs a full linear scan of the history to find the automation tag. For a 35-step run with 100+ messages this is invoked ~70 times. The `runSession` already has `isAutomation` cached — `findAutomationCtx` should be eliminated in favour of that cached field or stored once on `Agent`.

---

### 1.9 — `sanitizeCommand` calls `os.Getwd()` on every tool invocation  
**File:** [`terminal.go:439`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/tools/terminal.go#L439)  
**Severity: Low (performance)**

```go
cwd, _ := os.Getwd()
```

`os.Getwd()` is a syscall. It is called inside `sanitizeCommand` on every `execute_terminal_command` tool execution. Since the process working directory never changes at runtime, this value can be cached once at startup.

---

### 1.10 — `stream.go` builds `displayText` by concatenating two large strings every chunk  
**File:** [`stream.go:486-492`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/assistant/stream.go#L486-L492)  
**Severity: Low (performance)**

```go
displayText := fullMsg.ReasoningContent + fullMsg.Content
displayContent, hasToolCall := FilterStreamingMarkup(displayText)
```

Each incoming SSE chunk triggers a full re-concatenation of the accumulated content (potentially megabytes for long reasoning chains). The result is passed to `FilterStreamingMarkup` which then scans the entire string again. The effective work per chunk is O(total_chars_so_far).

---

## 2. Memory Leaks

### 2.1 — `GuardrailDecisionStore.pending` map never bounded  
**File:** [`agent.go:130-170`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/assistant/agent.go#L130-L170)  
**Severity: Medium**

When a guardrail decision is registered (`Register`) but the context is cancelled before `Resolve` is called, `Remove` is called and the channel is cleaned up. However if a decision is registered and then abandoned _without_ a context cancellation (e.g. server reboot of the client, network drop, or a bug in the decision callback path), the channel remains in the map indefinitely. The map grows until a restart. With high automation throughput (many guardrail events) this accumulates.

---

### 2.2 — `collectedEvents` slice in `handleAssistant` grows unbounded  
**File:** [`assistant_handlers.go:201-208`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/transport/http/assistant_handlers.go#L201-L208)  
**Severity: Low**

```go
var collectedEvents []assistant.AgentEvent
publishObs := func(ev assistant.AgentEvent) {
    collectedEvents = append(collectedEvents, ev)
    ...
}
```

For long assistant sessions (35 steps × 5 events/step) this accumulates 175+ `AgentEvent` structs in memory, each with a payload. These are all serialised at the end into `eventsJSON`. For a short assistant conversation this is negligible, but for multi-hour runs that accumulate thousands of events it is wasteful. If the frontend only needs the _last_ events or a count, the full collection is unnecessary.

---

### 2.3 — `StreamChunkTimeout` timer created and stopped per line  
**File:** [`client.go:157-162`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/proxy/client.go#L157-L162)  
**Severity: Low**

```go
timer := time.AfterFunc(StreamChunkTimeout, func() { resp.Body.Close() })
line, err := readSSELine(reader)
timer.Stop()
```

`time.AfterFunc` allocates a new timer object and an OS timer on every SSE line. For a response with 1000 chunks this allocates and releases 1000 timers. Using a single `time.Timer` reset with `timer.Reset(StreamChunkTimeout)` would allocate once.

---

## 3. Bottlenecks

### 3.1 — `NormalizeHistory` + `SanitizeHistory` always makes two full copies  
**File:** [`history.go:22-113`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/proxy/history.go#L22-L113)  
**Severity: Medium**

`NormalizeHistory` allocates `prepared` (a full copy), then `merged` (another full copy), then `SanitizeHistory` allocates `sanitized` (a third full copy). This is called once per turn. For a 50-message history at turn 30 this is 150 full message copies per turn, meaning ~4500 message copies over a 30-turn session.

---

### 3.2 — `physicalSieve` re-counts total chars after compression  
**File:** [`sieve.go:43-49`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/assistant/sieve.go#L43-L49)  

```go
totalChars = 0
for _, m := range history {
    totalChars += len(m.Content)
}
```

The sieve counts total chars before truncation, performs truncation, then recounts from scratch. A running delta could be tracked as content is truncated, avoiding the second O(n) scan.

---

### 3.3 — `filterStreamingMarkup` uses `strings.Index` with a linear scan per cutoff pattern  
**File:** [`stream.go:700-719`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/assistant/stream.go#L700-L719)  

The function checks 13 cutoff strings sequentially. Each check scans the full `displayContent`. For a large reasoning block (e.g. 10 KB) and many chunks, this is 13 × 10 KB = 130 KB of string scanning per chunk. An `Aho-Corasick` automaton or a single compiled regex would reduce this to a single O(n) pass.

---

## 4. Design / Architecture Observations

### 4.1 — `Agent` fields mutated during execution without synchronisation  
`prefillDisabled`, `useNativeTools`, `suppressReasoningBudget`, `skipStuckCheck`, `memoryInjected`, `maxTokens`, `reasoningBudget` are all modified inside the agent loop (sometimes across fallback paths in `handleEmptyStream` and `computeNextResponseStreamXML`). There is no mutex. The SSE heartbeat goroutine concurrently reads `fullMsg.Content` and `fullMsg.ReasoningContent`. This pattern relies on the goroutine scheduler not interleaving, which is not guaranteed.

### 4.2 — Conversation ID uses time format, not UUID  
**File:** [`assistant_handlers.go:124`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/transport/http/assistant_handlers.go#L124)  

```go
payload.ConversationID = "conv_" + time.Now().Format("20060102150405")
```

Two requests landing in the same second produce identical conversation IDs. This silently clobbers sessions if two requests arrive in the same second for the same workspace with no `conversation_id`. `generateRunID()` already exists and produces a crypto-random ID — the same pattern should be used here.

### 4.3 — `processToolCalls` calls `provider.ListTools` again on every tool invocation  
**File:** [`tool_exec.go:136`](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/assistant/tool_exec.go#L136)  

```go
toolsList, _ := a.provider.ListTools(ctx)
if err := validateToolArgs(tc, toolsList); err != nil {
```

`ListTools` was already called at the start of `executeTurn` and the result was passed in as `toolsList`. `processToolCalls` then calls it again for each tool call, ignoring the already-fetched list. This is redundant and potentially inconsistent (the tool list could change between calls, though in practice it does not).

---

## Summary Table

| # | Severity | Category | Short Title |
|---|----------|----------|-------------|
| 1.1 | 🔴 High | Memory Leak / Bug | Goroutine leak in `Chat()` body-close path |
| 1.2 | 🟠 Medium | Race Condition | `maxTokens`/`reasoningBudget` mutated without mutex |
| 1.3 | 🟠 Medium | Bug + Perf | `truncateHistory` O(n²) + aliasing bug |
| 1.4 | 🟠 Medium | Bottleneck | `readSSELine` single-byte reads |
| 1.5 | 🟡 Low | Correctness | Unused mutex in `processToolCalls` |
| 1.6 | 🟠 Medium | Bottleneck | N+1 disk reads in `ListAutomations` |
| 1.7 | 🟡 Low | Bug | `injectRetryContext` mutates caller's slice |
| 1.8 | 🟡 Low | Perf | `findAutomationCtx` full scan every turn |
| 1.9 | 🟡 Low | Perf | `os.Getwd()` syscall per terminal invocation |
| 1.10 | 🟡 Low | Perf | Full re-concat of streaming buffer per chunk |
| 2.1 | 🟠 Medium | Memory Leak | Unbounded `GuardrailDecisionStore.pending` map |
| 2.2 | 🟡 Low | Memory | `collectedEvents` grows for entire session |
| 2.3 | 🟡 Low | Memory | Timer alloc per SSE line in stream client |
| 3.1 | 🟠 Medium | Bottleneck | Three full history copies per `NormalizeHistory` call |
| 3.2 | 🟡 Low | Perf | Redundant char recount in `physicalSieve` |
| 3.3 | 🟡 Low | Perf | 13 linear string scans per SSE chunk in markup filter |
| 4.1 | 🟠 Medium | Design | Agent fields mutated without mutex during execution |
| 4.2 | 🟠 Medium | Bug | Conversation ID collision within same second |
| 4.3 | 🟡 Low | Perf | `ListTools` called twice per tool invocation |
