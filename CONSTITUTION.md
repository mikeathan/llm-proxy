# Antigravity Constitution (The Law)

This document defines the immutable architectural and security laws of the Antigravity project. All code must adhere to these rules without exception.

## I. Network Security (The Guarded Hull)

1.  **No Raw Sockets**: No code is permitted to instantiate a raw `http.Client`, `http.DefaultClient`, `net.Dial`, or `net.DialContext` for external or LAN communication.
2.  **Mandatory Guardrails**: All network interactions must pass through the `NetworkTools` abstraction. 
    *   **Agent Tools**: Must use `network.FetchURL` or `network.HTTPClient()`.
    *   **Infrastructure**: Must use `network.DialContext()` to ensure DNS rebinding protection and boundary checks (LAN vs Internet) are applied at the socket level.
3.  **Boundary Enforcement**: The system must strictly distinguish between LAN access and Internet access. Tools must verify destination safety before the first byte is sent.

## II. Resource Management (The Bounded Deck)

1.  **Sequential Execution**: To prevent race conditions and ensure state integrity (e.g., ensuring a directory exists before writing to it), all agentic tool calls within a single turn MUST be executed sequentially by default.
    *   Concurrent execution is prohibited for state-modifying tools (Terminal, FileSystem). Independent operations may be parallelized only if they adhere to a strict **Semaphore** (currently capped at 10) to prevent host resource exhaustion.
2.  **Lifecycle Tethering**: No background process (Dispatcher, MCP Client, Watcher) shall be started with `context.Background()`. All long-running processes must be tethered to a cancellable context derived from the application's root lifecycle.
3.  **Sandbox Isolation**: All execution of untrusted code (scripts, binaries, WASM) must occur within the verified `Sandboxing` subsystem. No raw `os/exec` calls are allowed for agent-triggered work. Terminal execution must utilize a persistent shell session to maintain state (CWD, environment) throughout the agent's task lifecycle. **Environment variables passed from the host to the sandbox must be strictly filtered using an explicit Allowlist (defined via `terminal.json` overrides) to prevent secret leakage.**

## III. Data & Metadata (The High-Fidelity Rule)

1.  **No Regex for Binary**: Parsing of high-stakes binary metadata (e.g., GGUF files) must be done using specialized, performance-optimized libraries. String manipulation or regex-based extraction for metadata is forbidden.
2.  **Atomic Persistence**: All system configuration writes must be atomic. Use `storage.Update` patterns to prevent partial state corruption.
3.  **Workspace Jail**: File system access for agents must be strictly jailed to the assigned workspace directory using `IsSecurePath`. The system root is never a valid target for agentic I/O. Terminal commands must be validated using segment-aware parsing to ensure chained commands (e.g., &&, ||, ;, |) do not bypass security guardrails.

## IV. Code Standards (The Clean Signal)

1.  **Context Propagation**: Every function performing I/O or network calls MUST accept `context.Context` as its first argument.
2.  **Error Integrity**: Never return raw strings as errors. Always use `fmt.Errorf` with the `%w` verb to maintain the error chain for diagnostic trace-back.
3.  **Failover Clarity**: Distinguish between "Transitional States" (e.g., loading weights) and "Terminal Errors". Fallback logic should only trigger on terminal failures.

## V. Governance (The Living Law)

1.  **Constitution over Convenience**: If a feature requires violating these laws, the law must be formally amended in the Constitution after a security review, rather than bypassed in the implementation.
