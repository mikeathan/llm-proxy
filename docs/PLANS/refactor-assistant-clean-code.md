# Refactoring Assistant Package to Agent Package & Clean Code

This document outlines the implementation plan for:
1. Renaming the `assistant` package to `agent` to align with the repository architecture.
2. Splitting the package into two modular packages (`agent` and `agent/registry`) to separate the cognitive agent loop from system tool execution and discovery.
3. Refactoring the implementation to resolve cyclomatic complexity, eliminate nested conditionals, and enforce clean coding standards.

---

## 1. Package Renaming & API Simplification

To avoid stuttering (Go anti-pattern `agent.Agent`) and present an idiomatic API, types will be renamed at the import boundary:

| Old API (`assistant` package) | New API (`agent` package) | Description |
| :--- | :--- | :--- |
| `assistant.Agent` | `agent.Runner` | The core agent executor struct. |
| `assistant.AgentOptions` | `agent.Options` | Options to configure execution constraints. |
| `assistant.AgentEvent` | `agent.Event` | Status and result notification payloads. |
| `assistant.AgentEventType` | `agent.EventType` | String identifiers for different event types. |
| `assistant.NewAgent(...)` | `agent.NewRunner(...)` | Constructor for the execution runner. |

---

## 2. Package Split & Boundaries

We will split the current `assistant` folder into two packages:

### A. Core Cognitive Loop (`llm-proxy/internal/core/agent`)
Responsible for driving the LLM iteration loop, parsing incoming streams, performing sifting strategies, and publishing execution notifications.
- **Dependencies**: `proxy`, `orchestrator`, `models`, `logging`. No dependencies on local operating-system tools or MCP managers.

### B. Tool Discovery & Registry (`llm-proxy/internal/core/agent/registry`)
Responsible for discovering and configuring system tools (filesystem, network, terminal, MCP nodeherder), binding them to workspace-specific policies, and providing them via the `ToolProvider` and `Engine` interfaces.
- **Dependencies**: `agent` (for interface implementations), `tools`, `nodeherder`, `shell`, `persistence`.

---

## 3. Proposed Architectural Patterns

We will introduce three design patterns to address cyclomatic complexity:

### A. Transient Session Runner (State Encapsulation)
Instead of executing the loop on the long-lived `Runner` struct directly, we will encapsulate execution state inside a short-lived, transient struct:
```go
type runSession struct {
    runner       *Runner
    ctx          context.Context
    history      []proxy.Message
    steps        int
    starvation   int
    sieveStreak  int
    errorStreak  int
    // other loop-specific counters...
}
```
All step execution and no-tool-call helper functions will be defined as methods on `runSession`, eliminating complex pointer-to-counter arguments (like `*parseErrorStreak`).

### B. Strategy Pattern for History Sieving (Context Reduction)
Context pruning logic will be abstracted behind a `HistorySieve` interface:
```go
type HistorySieve interface {
    Sieve(history []proxy.Message, budget int) []proxy.Message
}
```
Concrete strategies will be defined in a dedicated file:
- `PhysicalSieve`: Pre-turn budget pruning.
- `ReactiveSieve`: Triggered by LLM context overflow errors.
- `AggressiveSieve`: Maximum pruning after consecutive failures.

### C. Pipeline Pattern for Tool Execution
Processing a batch of tool calls involves validation, guardrails, and engine execution. We will wrap this in a tool execution pipeline:
```go
type ToolPipeline struct {
    validators []ToolValidator
    guardrails []GuardrailInterceptor
    engine     Engine
}
```

---

## 4. File Structuring

To avoid single-file bloat (Go Staff Engineer Guideline 3), code will be structured into modular files:

### `internal/core/agent/` (Core Loop)
- `agent.go`: Public runner definitions and options.
- `session.go` [NEW]: Step state execution runner (`runSession`).
- `sieve.go` [NEW]: `HistorySieve` interface and pruning strategies.
- `stream.go` [NEW]: Stream-handling logic (`processStream`, `FilterStreamingMarkup`, and delta accumulation).
- `prompts/`: Nested package for prompt templates (unchanged).
- `guardrails/`: Nested package for safety guardrails (unchanged).

### `internal/core/agent/registry/` (Tool Registry)
- `registry.go`: Workspace configuration, local tools binding, and guardrail wrapping.
- `provider.go`: MultiToolProvider and CompositeEngine.

---

## 5. Clean Code Standards

All newly created files must strictly adhere to the updated `AGENTS.md` Go guidelines:
- **Max Function Length**: 80 lines. Extract logical blocks into small, well-named helper functions.
- **Max Cyclomatic Complexity**: 10 per function. Early returns and guard clauses must be used ("happy path to the left") to limit nested conditionals to a maximum depth of 3 levels.
- **No pointer-to-primitive arguments** for state tracking; use encapsulating structs (`runSession`).

---

## 6. Verification Plan

### Automated Tests
- Rename the package header in test files to `package agent` and verify all tests pass:
  ```bash
  cd backend
  go test ./internal/core/agent/... -v
  ```
- Ensure the overall build and compilation remains valid:
  ```bash
  go build ./...
  ```
