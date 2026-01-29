# llm-proxy

LLM proxy that starts and manages local model runtimes, exposes an OpenAI-style
`/v1/chat/completions` API, and provides a higher-level assistant endpoint that
injects device context into the prompt.

## How it works

- The proxy starts a model process (llama-server) on demand and forwards
  `/v1/chat/completions` requests to it.
- `/api/conversation/message` fetches device context, builds a system prompt,
  and sends a chat request to the model.
- Admin routes under `/admin` expose status and model management.

Additional endpoints:

- `POST /v1/chat/completions`: OpenAI-style chat completions.
- `GET /admin`: Admin UI.
- `GET /admin/api/state`: Server/model status snapshot.
- `POST /admin/api/start`: Start a model by name.
- `POST /admin/api/stop`: Stop the active model.
- `POST /admin/api/models`: Add a model.
- `PUT /admin/api/models`: Update a model.
- `DELETE /admin/api/models`: Remove a model.
- `GET /admin/api/config`: Read current config.
- `PUT /admin/api/config`: Persist config updates.
- `GET /admin/api/logs`: Read active model logs.
- `GET /admin/api/metrics`: Read GPU/system metrics snapshot.

## Configuration

The proxy reads `config/config.json` at startup. See `models/config.go` for the
full schema.

Example snippet:

```json
{
  "server": {
    "bind": "0.0.0.0:4001",
    "model_host": "127.0.0.1",
    "idle_timeout_seconds": 1800,
    "llama_server_binary": "/home/mikeathan/dev/llama.cpp/build/bin/llama-server",
    "default_args": [
      "--ctx-size",
      "4096",
      "--threads",
      "6",
      "--gpu-layers",
      "999"
    ]
  },
  "models": [
    {
      "name": "qwen2.5-7b",
      "filename": "qwen2.5-3b-instruct-q4_k_m.gguf",
      "args": [],
      "port": 5001
    }
  ],
  "model_dir": "/home/mikeathan/dev/models"
}
```

Key fields:

- `server.bind`: host:port to listen on.
- `server.model_host`: host used when constructing the model URL.
- `server.idle_timeout_seconds`: stop the model after this idle period.
- `server.llama_server_binary`: path to the llama-server binary.
- `server.default_args`: args passed to the model server on startup.
- `models[]`:
  - `name`: model name.
  - `filename`: filename in `model_dir`.
  - `args`: per-model args (appended to defaults).
  - `port`: port to run the model on.
- `model_dir`: base directory for model files.
- `metrics.gpu`: GPU metrics settings.
  - `provider`: `auto`, `nvidia-smi`, `rocm-smi`, `amdgpu_top`, `sysfs`, `none`
  - `binary`, `index`, `sysfs_path`: optional overrides.

## Environment variables

- `MCP_SERVER_SSE_URL`: Optional. URL for the NodeHerder MCP Server SSE endpoint. Defaults to `http://localhost:4110/api/mcp/events`.

Note: Legacy variables `SERVICE_CLIENT_ID` and `SERVICE_CLIENT_SECRET` are no longer used as device context is now fetched via MCP.

Send a test request to the assistant endpoint (replace the port if your config
uses a different `server.bind`):

```bash
curl -sS -X POST "http://localhost:4001/api/conversation/message" \
  -H "Content-Type: application/json" \
  -d '{
    "conversation_id": "local-dev-1",
    "context_version": "v1",
    "message": "Get the average temperature for device dev1 from 2025-01-01 to 2025-01-02."
  }'
```
