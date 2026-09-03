# LLM Proxy

[![Go](https://img.shields.io/badge/Go-1.26.2-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue)](LICENSE)

A local-first LLM orchestration platform. One Go binary manages llama.cpp
runtimes, proxies OpenAI-compatible chat, runs an autonomous agent loop, and
serves a web admin UI — no external servers required.

## What It Does

LLM Proxy is a self-hosted control plane for running and talking to LLMs. It
sits between your applications and one or more model backends — local
`llama.cpp` servers or cloud providers — and gives you a single OpenAI-compatible
endpoint, an autonomous agent, and a web console to manage it all. Everything
runs from one Go binary with an embedded frontend; no separate servers or
databases to wire up.

- **Model Runtime** — Start/stop llama-server on demand with idle-timeout
  reaping. Local GGUF models plus cloud providers (OpenAI, Gemini, OpenRouter, NVIDIA).
- **Chat Proxy** — `POST /v1/chat/completions` in OpenAI wire format, so any
  OpenAI-compatible client works unchanged.
- **Autonomous Agent** — The model emits tool calls, tools execute (terminal,
  filesystem, network, search), and results feed back into the loop until a
  final answer or limit is reached.
- **Automations** — Schedule or trigger multi-step workflows that chain model
  calls and tools together.
- **Memory** — A durable, tag-based memory system the agent can read and write
  across sessions.
- **Connectors & MCP** — Outbound/inbound communication connectors and Model
  Context Protocol integration to extend tooling.
- **Guardrails** — Pattern whitelists, command jail, and path boundaries.
  Blocked calls pause for human approval.
- **Admin UI** — A Vue 3 SPA for model management, chat, automations, and
  settings, embedded directly in the binary.

## Quick Start

```bash
./scripts/setup-gitleaks.sh  # one-time: installs gitleaks + registers the secret-scanning git hook
cd backend
go build ./...
go run main.go
```

Then open the Admin UI in your browser:

- Admin UI: `http://localhost:4001/admin/`

## Try the API

The chat endpoint speaks OpenAI wire format, so any OpenAI-compatible client
works. From the shell:

```bash
curl http://localhost:4001/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"<model>","messages":[{"role":"user","content":"hello"}]}'
```

## Commands

```bash
go build ./...                     # compile everything
go test ./...                      # run all tests
go run ./tools/check-complexity/   # complexity ≤12
npm install && npm run build       # frontend production build
go run main.go                      # start (default :4001; all files under ~/.config/llm-proxy or LLM_PROXY_HOME)
go run main.go --data <dir>         # explicit single root for everything (overrides LLM_PROXY_HOME)
go run main.go --record             # record LLM responses to {root}/runs/ for replay tests
```

Full dev workflow and AI-agent rules: see [`AGENTS.md`](AGENTS.md).

## Documentation

| | | |
|---|---|---|
| 📚 API reference | [docs/api-reference.md](docs/api-reference.md) | All endpoints |
| 🏗️ Architecture | [docs/architecture.md](docs/architecture.md) | Directory map, contracts, checklists |
| 🛡️ Constitution | [CONSTITUTION.md](CONSTITUTION.md) | Immutable laws (6 sections) |
| 🧪 Testing | [docs/skills/testing-guide.md](docs/skills/testing-guide.md) | Patterns, record-replay |
| 🤖 Agent loop | [docs/skills/agent-loop.md](docs/skills/agent-loop.md) | Spec + implementation guide |
| 📇 Full catalog | [docs/INDEX.md](docs/INDEX.md) | All docs indexed |
| 🗺️ Skill map | [docs/skills/README.md](docs/skills/README.md) | When to load which skill |

## Contributing

PRs welcome via Conventional Commits. Read [`CONTRIBUTING.md`](CONTRIBUTING.md)
for standards and [`AGENTS.md`](AGENTS.md) for AI assistant rules.

## License

Released under the [MIT License](LICENSE).
