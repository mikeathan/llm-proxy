---
trigger: always_on
description: Comprehensive Staff-level Go expertise (10+ years). Covers Clean Architecture, Robust Concurrency, Self-Healing systems, and High-Performance Go idioms.
---

# Target: \*_/_.go

# Staff Go Backend Engineer Skill

Apply these standards to all Go-related tasks:

## 1. Concurrency & Thread Safety

- **State Protection**: Always use `sync.RWMutex` for shared struct fields. Favor "sharing by communicating" (channels) for data flow, but use Mutexes for state protection.
- **Atomic Operations**: Use `sync/atomic` for simple counters or status flags.
- **Goroutine Discipline**: Never start a goroutine without a clear termination strategy (via `context.Context` or `close(chan)`).

## 2. Resilience & Self-Healing (Quiet-Pulse)

- **Exponential Backoff**: Implement retries with `minDelay=5s`, `maxDelay=5m`, and a doubling multiplier.
- **Log Management**: Mute failure logs after 3 attempts to prevent log-spam. Transition to "Background Heartbeat" mode once the max delay is reached.
- **Atomic Initialization**: Handshake with external dependencies (like MCP or DBs) fully _before_ setting `initialized = true`.
- **Failover-First Design**: When implementing model hierarchies (Primary/Fallback), strictly distinguish between "Transitional States" (e.g., loading weights) and "Terminal Errors" (e.g., OOM, 401, 503). Never fallback during a normal startup.


## 3. Clean Architecture & Patterns

- **Interfaces**: Define interfaces at the point of use (consumer-side), not producer-side.
- **Error Handling**: Use early returns ("Happy Path" to the left). Wrap errors with context: `fmt.Errorf("action failed: %w", err)`.
- **Dependency Injection**: Use constructor functions (`NewService`) to inject all dependencies. No global variables or `init()` magic.
- **Logical Modularization**: Avoid single-file bloat (e.g., >500 lines). Extract lifecycle management, registration, and provider-specific logic into scoped files (e.g., `lifecycle.go`, `registry.go`, `providers.go`) while maintaining package-level visibility.
- **Structural Decoupling**: Avoid polluting core managers with complex `if type == "x"` conditionals or infrastructure discovery logic (e.g., binary paths, secret resolution). Extract these into dedicated `Registrars`, `Registries`, or `Factories`. The core manager should orchestrate lifecycles, while the registrar handles the "how" of configuration and instantiation.


## 4. Performance & Resource Safety

- **Memory**: Pre-allocate slices/maps (`make([]T, 0, cap)`) when size is known. Use pointers for large structs (>64 bytes) but values for small, immutable data.
- **Context Propagation**: Always pass `ctx` as the first argument to I/O or long-running functions. Use `context.WithTimeout` for all external network calls.

## 5. Testing

- **Table-Driven Tests**: Use the sub-test pattern (`t.Run`) and anonymous structs for test cases.
- **Mocks**: Generate or manually implement interfaces to isolate unit tests from external I/O.
- **Validation**: Include tests for failover scenarios, ensuring the system recovers gracefully from provider outages.

## 6. Build & Verification
- **Continuous Compilation**: Always run `go build ./...` across the backend after attempting refactoring, interface changes, or file structure updates to proactively identify broken package imports or undefined type dependencies.
