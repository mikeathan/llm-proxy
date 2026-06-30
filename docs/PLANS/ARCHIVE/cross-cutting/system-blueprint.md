---
status: reference
---
# System Blueprint: LLM-Proxy

**Status: REFERENCE** — This is the system architecture document, not an implementation plan. All major components described here are implemented (3-tier config, agent loop, MCP integration, guardrails, admin UI).

## I. Architectural Overview
LLM-proxy is a high-performance LLM proxy and agentic automation platform. It is built on a "Hardened Hull" architecture, where all external interactions and resource usage are governed by a central security engine.

## II. Multi-Tier Storage (3-Tier)
1.  **Tier 1: Infrastructure (SystemConfig)**
    *   `config.json`: Server binding, metrics configuration, and system-wide workspaces path.
2.  **Tier 2: Policy (UserSettings)**
    *   `settings.yml`: Default guardrail policies, local model binary paths, and user preferences.
3.  **Tier 3: Runtime (Registry & Secrets)**
    *   `registry.json`: Live model catalog, MCP server connections, and tool registrations.
    *   `secrets.json`: Encrypted (AES-256-GCM) API keys and credentials.

## III. Security Subsystems

### 1. Guarded Network (NetworkGuardrails)
*   All socket communication (HTTP/TCP) is intercepted by a custom `DialContext`.
*   **DNS Rebinding Protection**: Prevents SSRF attacks by validating resolved IPs before connection.
*   **Boundary Enforcement**: Disables LAN access for Internet-facing tools unless explicitly overridden.

### 2. Execution Guardrails (GuardrailEngine)
*   **Global Filters**: Prevents leakage of credentials (API keys) in model responses.
*   **Terminal Jail**: Whitelists commands and blocks dangerous patterns (e.g., `rm -rf /`).
*   **Persistent Shell Sessions**: All terminal execution occurs within stateful, long-lived sessions (bash). These sessions maintain `cwd` and environment variables across multiple tool calls to support complex, multi-step automation.
*   **Automated Lifecycle Management**: Idle terminal sessions are automatically terminated by a background reaper based on a configurable `session_idle_timeout_seconds` to prevent resource leaks.
*   **FS Jail**: Restricts file I/O to designated workspace paths using absolute path resolution.

### 3. Resource Management
*   **Bounded Concurrency**: Tools are executed through a semaphore pool (10 slots) to prevent CPU/RAM exhaustion.
*   **Lifecycle Tethering**: All background tasks (MCP, Dispatcher) share the application's root context.

## IV. Core Subsystems

### 1. Agent Engine
*   Implements the "Observe-Orient-Decide-Act" loop.
*   Asynchronous tool execution with real-time status updates via EventBus.

### 2. MCP Bridge (Model Context Protocol)
*   Dynamic discovery of remote tools and resources.
*   Transparent proxying of tool calls to distributed services.

### 3. Automation Dispatcher
*   Manages long-running tasks across multiple workspaces.
*   Persists execution state and logs for historical auditing.

## V. Future Roadmap
*   **Discovery Panel Implementation**: Visualizing the 3-tier environment.
*   **Advanced Sandboxing**: Migrating more tools to the Wazero-based WASM sandbox.
*   **Federated Agents**: Allowing multiple agents to collaborate across different MCP nodes.
