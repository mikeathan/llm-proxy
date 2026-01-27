# MCP Client Implementation for LLM-Proxy

## Goal

Implement an MCP client in the llm-proxy that connects to the NodeHerder MCP server and injects device context into every LLM request, ensuring the LLM always has the correct metric information before making tool calls.

## Problem Being Solved

Claude Desktop doesn't read MCP resources before calling tools, causing the LLM to guess metric names instead of using the actual device context. By implementing our own MCP client, we can:

1. Pre-fetch the `nodeherder://devices` resource
2. Inject it into the system prompt for every LLM call
3. Subscribe to resource updates and keep a cached copy

## Architecture

```
┌─────────────┐     ┌─────────────────────────────────────────┐     ┌──────────────────┐
│   Client    │────▶│             LLM-Proxy                   │────▶│  LLM API (GPT/   │
│  (Browser)  │     │                                         │     │  Claude/etc)     │
└─────────────┘     │  ┌─────────────────────────────────┐    │     └──────────────────┘
                    │  │  MCP Client Module               │    │
                    │  │  - Connects to NodeHerder        │    │
                    │  │  - Caches device context         │    │
                    │  │  - Intercepts tool calls         │    │
                    │  └─────────────┬───────────────────┘    │
                    └────────────────│────────────────────────┘
                                     │
                                     ▼ (stdio or SSE)
                    ┌─────────────────────────────────────────┐
                    │        NodeHerder MCP Server            │
                    │  - nodeherder://devices resource        │
                    │  - query_device tool                    │
                    └─────────────────────────────────────────┘
```

## Implementation Steps

### 1. Add MCP Client Transport

Connect to NodeHerder MCP server via stdio (spawn process) or SSE (HTTP).

```go
type MCPClient struct {
    deviceContext *DeviceContextResponse  // Cached devices
    mu            sync.RWMutex
}

func NewMCPClient(nodeherderPath string) (*MCPClient, error) {
    // Spawn nodeherder with --mcp-only flag
    // Initialize JSON-RPC connection over stdio
}
```

### 2. Fetch and Cache Device Context

On startup and on `notifications/resources/updated`:

```go
func (c *MCPClient) RefreshDeviceContext() error {
    // Call resources/read for "nodeherder://devices"
    // Parse response into DeviceContextResponse
    // Cache it for injection
}
```

### 3. Build System Prompt with Device Context

Inject cached device context into every LLM request:

```go
func (c *MCPClient) BuildSystemPrompt() string {
    c.mu.RLock()
    defer c.mu.RUnlock()

    return fmt.Sprintf(`You have access to smart home devices via the query_device function.

## Available Devices and Metrics

%s

## Rules
1. ONLY use metric names that appear in the device's "exposes" list above
2. Do NOT guess metric names - use exact names from the list
3. Use "last" aggregation for current state queries
`, c.formatDeviceContext())
}
```

### 4. Intercept Tool Calls (Optional Validation)

Before forwarding tool calls to NodeHerder, validate metrics:

```go
func (c *MCPClient) ValidateToolCall(call ToolCall) error {
    if call.Name == "query_device" {
        // Parse metrics from call arguments
        // Check they exist in cached device context
        // Return error if invalid
    }
    return nil
}
```

### 5. Forward Tool Calls to MCP Server

For valid tool calls, forward to NodeHerder and return results:

```go
func (c *MCPClient) ExecuteTool(call ToolCall) (string, error) {
    // Send tools/call to MCP server
    // Return formatted result
}
```

## Device Context Format for LLM

```
### Living room presence sensor (0xa4c13894070052fc)
Available metrics:
- presence (binary) - last, count
- illuminance (numeric) - last, min, max, avg

### Attic temperature sensor (0x00124b0029207763)
Available metrics:
- temperature (numeric) - last, min, max, avg
- humidity (numeric) - last, min, max, avg
- battery (numeric) - last
```

## Tool Definition for LLM

```json
{
  "name": "query_device",
  "description": "Query smart home device metrics. Use metric names from the device list above.",
  "parameters": {
    "target_name": "Device name (e.g., 'living room presence sensor')",
    "metrics": "Array of metric names from device's expose list",
    "aggregation": "One of: last, min, max, avg, count",
    "time_scope": "One of: last_hour, today, last_24_hours, last_7_days"
  }
}
```

## Key Benefits

1. **Guaranteed Context** - Device list is in the system prompt, LLM can't miss it
2. **Validation** - Catch wrong metric names before they reach NodeHerder
3. **Caching** - No repeated database reads, just cached HubState
4. **Updates** - Subscribe to resource changes for real-time device updates
5. **Model-Agnostic** - Works with any LLM API (OpenAI, Anthropic, local models)

## MCP Protocol Messages Needed

| Method                | Purpose                   |
| --------------------- | ------------------------- |
| `initialize`          | Handshake with MCP server |
| `resources/list`      | Get available resources   |
| `resources/read`      | Fetch device context      |
| `resources/subscribe` | Subscribe to updates      |
| `tools/list`          | Get available tools       |
| `tools/call`          | Execute query_device      |

## Testing

1. Start NodeHerder in MCP mode: `./nodeherder --mcp-only`
2. Connect llm-proxy MCP client
3. Verify device context is cached
4. Make LLM request asking about a device
5. Verify LLM uses correct metric names from injected context
