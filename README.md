# LLM Proxy

Local-first LLM orchestration platform. Manages model runtimes (llama.cpp), provides an OpenAI-compatible API, an agent loop for autonomous task execution, and a web admin UI.

## How It Works

1. **Model Runtime Management** — Starts/stops llama-server processes on demand with idle-timeout reaping. Supports local GGUF models and cloud providers (OpenAI, Gemini, Vertex AI).
2. **OpenAI-Compatible Proxy** — `POST /v1/chat/completions` forwards to the active model using OpenAI wire format.
3. **Agent Loop** — The assistant endpoint runs an autonomous agent: model generates tool calls (XML or native function calling), tools execute (terminal, filesystem, network, search), results feed back to the model. Repeats until `submit_final_answer` or step limit.
4. **Guardrail Engine** — Validates every tool call against configurable rules (blocked patterns, command whitelist, path jail, network boundaries). Blocked calls pause for user approval.
5. **Admin UI** — Vue 3 SPA for model management, chat, automation dashboards, settings, and guardrail configuration. Embedded in the Go binary.

## Quick Start

```bash
# Build
cd backend
go build ./...

# Run (creates data/ directory on first start)
go run main.go --data ./data

# Run in record mode (captures LLM responses to JSONL files for replay testing)
go run main.go --data ./data --record-dir=testdata/recordings

# Admin UI at http://localhost:4001/admin/
# Chat API at http://localhost:4001/v1/chat/completions
```

## Endpoints

### Proxy

| Method | Path                   | Description                        |
| ------ | ---------------------- | ---------------------------------- |
| ANY    | `/v1/chat/completions` | OpenAI-compatible chat completions |

### Assistant / Chat

| Method     | Path                                         | Description             |
| ---------- | -------------------------------------------- | ----------------------- |
| POST       | `/api/conversation/message`                  | Send message to agent   |
| POST       | `/admin/api/conversation/guardrail-decision` | Resolve guardrail block |
| GET        | `/api/conversation/sessions/:ws`             | List chat sessions      |
| GET/DELETE | `/api/conversation/sessions/:ws/:id`         | Get/delete session      |

### Admin API (`/admin/api/`)

| Method              | Path                   | Description                     |
| ------------------- | ---------------------- | ------------------------------- |
| GET                 | `/state`               | Full admin state snapshot       |
| POST                | `/start`               | Start a model                   |
| POST                | `/stop`                | Stop active model               |
| POST/PUT/DELETE     | `/models`              | Model CRUD                      |
| GET                 | `/registry`            | Registry catalogue + providers  |
| PUT                 | `/registry`            | Update full registry            |
| GET/PUT             | `/config`              | System config                   |
| POST                | `/system/restart`      | Restart application             |
| GET/PUT             | `/host`                | Host machine settings           |
| GET                 | `/logs`                | Active model logs               |
| GET                 | `/app-logs/tail`       | Application log tail            |
| GET/PUT             | `/log-level`           | Log level control               |
| GET                 | `/metrics`             | GPU/CPU/token metrics           |
| GET/POST/PUT/DELETE | `/mcp`                 | MCP server CRUD                 |
| GET                 | `/providers/models`    | List provider models            |
| GET                 | `/providers/manifests` | Provider manifest list          |
| GET                 | `/providers/test`      | Test provider connection        |
| GET/PUT/DELETE      | `/secrets/keys`        | API key management              |
| GET/PUT             | `/secrets/tools`       | Tool secrets (Tavily, Telegram) |
| GET                 | `/templates`           | Playbook templates              |
| GET                 | `/templates/:id`       | Template content                |

### Dispatcher (`/admin/api/dispatcher/`)

| Method          | Path                          | Description             |
| --------------- | ----------------------------- | ----------------------- |
| GET             | `/automations`                | List all automations    |
| POST            | `/trigger/:ws/:name`          | Trigger automation      |
| POST            | `/stop/:ws`                   | Stop running automation |
| GET/POST/DELETE | `/workspaces`                 | Workspace CRUD          |
| POST/PUT/DELETE | `/workspaces/:ws/automations` | Automation CRUD         |
| GET/PUT/DELETE  | `/workspaces/:ws/files/:file` | File CRUD               |
| GET/PUT         | `/workspaces/:ws/state`       | Workspace state         |
| GET/PUT         | `/workspaces/:ws/config`      | Workspace config        |
| GET             | `/workspaces/:ws/live`        | SSE event stream        |

## Record & Replay Testing

The server can record live LLM responses to JSONL files and replay them in offline integration tests — no running models required.

### Recording

Start the server with `--record-dir`:

```bash
cd backend
go run main.go --data ./data --record-dir=testdata/recordings
```

Every LLM call (Chat or Stream) is written to `{record-dir}/{model-name}/{timestamp}_{session-id}.jsonl` with one JSON object per line:

- `request` — model, messages, tool definitions
- `chunk` — stream delta (content, tool_calls, reasoning)
- `response` — non-streaming response
- `error` — HTTP/connection errors
- `done` — stream completion marker

Hit different LLMs with different prompts through the proxy or agent API — each model gets its own subdirectory.

### Replay

Replay tests are opt-in via the `recordreplay` build tag:

```bash
go test -tags recordreplay ./internal/core/assistant/ -run TestAgent_Execute_AgainstRecordings -v
```

The test runner (`llmprofiles.RunAgainstFixtures`) loads all `.jsonl` files from `testdata/recordings/`, wraps each in a `FixtureClient`, and runs the agent against it. The `FixtureClient` implements `proxy.Client` so the agent loop operates identically to a live run.

### JSONL Fixture Format

```json
{"type":"request","model":"gemma4","messages":[...],"tools":[...]}
{"type":"response","choices":[{"message":{"role":"assistant","content":"answer"}}]}
{"type":"done","total_chunks":1}
```

For streaming sessions, lines alternate `chunk`/`response`/`done` following the same request line.

## Configuration

### Three-Tier Config Model

```
Tier 1: config.json       — System infrastructure (host, ports, timeouts)
Tier 2: settings.yml      — User preferences (local paths, guardrails, per-model tuning)
Tier 3: registry.json     — Dynamic state (models, providers, MCP servers)

+ Workspace overrides: {metadata}/{workspace}/config.yaml
```

### Data Directory Layout

```
data/
├── config.json          — System config
├── registry.json        — Model catalogue + providers
├── secrets.json         — Encrypted API keys (AES-256-GCM)
├── registry/definitions/ — Provider manifests (*.json)
├── templates/           — Playbook templates
├── workspaces/          — Run files, heartbeat, process logs
└── metadata/            — Workspace configs (config.yaml per workspace)
```

```
~/.config/llm-proxy/
├── settings.yml         — User preferences
└── host.json            — Host machine settings
```

### settings.yml

```yaml
local:
  llama_server_binary: /usr/local/bin/llama-server
  model_dir: /home/user/models
  default_args:
    - "--ctx-size"
    - "4096"
    - "--threads"
    - "6"
    - "--gpu-layers"
    - "999"

guardrails:
  terminal:
    blocked_commands:
      - "rm -rf /"
    blocked_patterns:
      - "curl.*|.*sh"

model_overrides:
  my-model:
    max_steps: 30
    context_budget: 20000
    tool_call_format: native
    prefill: true
```

### registry.json

```json
{
  "primary_model": "qwen2.5-7b",
  "fallback_model": "",
  "catalogue": [
    {
      "id": "qwen2.5-7b",
      "name": "qwen2.5-7b",
      "provider_id": "local",
      "model_id": "qwen2.5-3b-instruct-q4_k_m.gguf",
      "port": 8081,
      "args": ["--flash-attn"],
      "prefill": false
    }
  ],
  "providers": {
    "openai": {
      "type": "openai",
      "base_url": "https://api.openai.com/v1"
    }
  },
  "mcp_servers": []
}
```

## Agent Loop

The agent loop runs in `internal/core/assistant/agent.go`. Each iteration:

1. **Physical Sieve** — If history exceeds `ContextBudget`, prune middle turns (keep system prompt + first user message + last 3 turns)
2. **List tools** — Aggregate local tools + MCP tools
3. **Compute response** — Stream LLM completion, accumulate text + native tool_calls
4. **Parse tool calls** — Extract `<tool_call>` XML from text (fallback path). Native tool_calls from deltas (primary path).
5. **Validate** — Guardrail engine checks each tool call against rules
6. **Execute** — Run tool, append result to history
7. **Repeat** — Until `submit_final_answer` or maxSteps

Tool call format (XML):

```xml
<tool_call>{"tool":"tool_name","args":{"key":"value"}}</tool_call>
```

## Frontend

```bash
cd frontend
npm install
npm run dev         # dev server (proxies API to :4001)
npm run build       # production build → embedded in Go binary
```

Vue 3 + Vite + TypeScript. The production build is embedded via `//go:embed`. After frontend changes, `npm run build` then rebuild the backend.

## Environment Variables

- `APP_ENV` — Application environment (default: `development`)
- `SERVICE_CLIENT_ID` / `SERVICE_CLIENT_SECRET` — Service credentials (.env file)

## Architecture

```
backend/
├── main.go                    — Entry point
├── models/                    — Shared types (no logic)
├── internal/
│   ├── app/                   — Bootstrap, AppContext state manager
│   ├── core/
│   │   ├── assistant/         — Agent loop, tool providers, guardrails, prompts
│   │   ├── automation/        — Task scheduler, executor, event bus
│   │   ├── llm/               — Model lifecycle, GGUF scanner, provider registry
│   │   ├── mcp/               — MCP client (SSE transport)
│   │   ├── proxy/
│   │   │   ├── ...            — LLM HTTP client, XML parser, history normalization
│   │   │   └── recorder/      — RecordingClient decorator (JSONL capture)
│   │   └── tools/             — Tool implementations (terminal, fs, network, search)
│   ├── platform/              — Logging, storage, persistence, metrics
│   ├── shell/                 — Persistent shell sessions
│   ├── testing/
│   │   ├── mocks/             — Shared test mocks
│   │   └── llmprofiles/       — FixtureClient + RunAgainstFixtures (replay testing)
│   └── transport/http/        — HTTP handlers + embedded frontend
```

## Documentation

- `CONSTITUTION.md` — Immutable architectural laws (read first)
- `CLAUDE.md` — Project guide with invariants and API reference
- `AGENTS.md` — Instructions for AI coding assistants
- `docs/SPECS/agent-loop.md` — Agent loop specification
- `docs/SPECS/tool-call-parser.md` — Tool call parser specification
- `docs/PLANS/agnostic-agent-loop.md` — Implementation plan
- `docs/audits/agent-stability-report.md` — Stability audit results

## License

Proprietary.
