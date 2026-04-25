# SPEC: Discovery Panel

## I. Intent
The Discovery Panel serves as the central nervous system for the Antigravity project. It provides a high-fidelity interface for the user to explore, configure, and monitor the three core pillars of the agentic environment: **Models**, **Tools**, and **Workspaces**.

## II. Functional Requirements

### 1. Model Catalog (Tier 1)
*   **Inventory**: List all models registered in the system (Local LlamaCpp, Remote OpenAI/Anthropic/Gemini).
*   **Metadata Display**: Show parameter count, quantization levels, and context length.
*   **Lifecycle Control**: Start/Stop buttons for local model runtimes.
*   **Provider Management**: Configure API keys and base URLs for remote providers.

### 2. Tool Registry (Tier 2)
*   **Local Tools**: Browse internal capabilities (Terminal, FS, Search, Communication).
*   **Remote MCP**: Automatically discover and list tools provided by connected MCP servers (e.g., NodeHerder).
*   **Documentation**: View auto-generated tool signatures and descriptions directly in the UI.

### 3. Workspace Explorer (Tier 3)
*   **Isolation Mapping**: List active workspaces and their associated security policies.
*   **Activity Feed**: Real-time stream of events occurring within each workspace.
*   **Storage Metrics**: Monitor disk usage and file counts for agent-managed directories.

## III. Technical Architecture

### Component Hierarchy
*   `DiscoveryPanel.vue` (Main View)
    *   `SidebarNavigation.vue` (Context Switcher)
    *   `ModelGrid.vue` (Tier 1 Explorer)
    *   `ToolManifest.vue` (Tier 2 Explorer)
    *   `WorkspaceList.vue` (Tier 3 Explorer)

### Data Orchestration
*   The panel polls the `/admin/api/state` and `/admin/api/mcp` endpoints to maintain synchronization with the backend.
*   Real-time updates are pushed via the `EventBus` to reflect tool execution and model state changes.

## IV. Design Aesthetics
*   **Visual Style**: Dark mode by default, utilizing a glassmorphic "Glass Deck" aesthetic.
*   **Interactive Elements**: Hover-activated tooltips for complex tool parameters.
*   **State Indicators**: Pulse animations for active model runtimes and running automations.
*   **Responsiveness**: Grid-based layout that adapts from widescreen desktops to tablet views.

## V. Security Guardrails
*   The Discovery Panel respects all `CONSTITUTION.md` rules.
*   Credential display is masked by default; editing requires explicit "Unlock" action.
*   Tool execution from the panel is subject to the same `GuardrailEngine` validation as agentic calls.
