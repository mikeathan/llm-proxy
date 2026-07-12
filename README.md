# LLM Proxy

[![Go](https://img.shields.io/badge/Go-1.26.2-00ADD8?logo=go)](https://go.dev/)
[![License: Proprietary](https://img.shields.io/badge/License-Proprietary-red)](LICENSE)

Local-first LLM orchestration platform. Manages llama.cpp runtimes, proxies
OpenAI-compatible chat, runs an autonomous agent loop, and exposes a web admin UI —
all from a single Go binary.

## What It Does

- **Model Runtime** — Start/stop llama-server on demand, idle-timeout reaping.
  Local GGUF models + cloud providers (OpenAI, Gemini, Vertex AI).
- **Chat Proxy** — `POST /v1/chat/completions` in OpenAI wire format.
- **Autonomous Agent** — Model writes tool calls, tools execute (terminal,
  filesystem, network, search), results fed back. Runs until final answer or limit.
- **Guardrails** — Pattern whitelist, command jail, path boundaries. Blocked calls
  pause for human approval.
- **Admin UI** — Vue-3 SPA: model management, chat, automations, settings.
  Embedded in the Go binary — zero external server.

## Quick Start

```bash
./setup.sh          # one-time: installs gitleaks + registers the secret-scanning git hook
cd backend
go build ./...
go run main.go
```

Then open:
- Admin UI: `http://localhost:4001/admin/`
- Chat API: `http://localhost:4001/v1/chat/completions`

## Commands

```bash
go build ./...                     # compile everything
go test ./...                      # run all tests
go run ./tools/check-complexity/   # complexity ≤12
npm install && npm run build       # frontend production build
go run main.go                      # start (default :4001, data dir = ./data)
go run main.go --data <dir>         # override data directory
go run main.go --record             # record LLM responses to {data}/runs/ for replay tests
```

Full dev workflow and AI-agent rules: see [`AGENTS.md`](AGENTS.md).

## Documentation

| | | |
|---|---|---|
| 📚 API reference | `docs/api-reference.md` | All endpoints |
| 🏗️ Architecture | `docs/architecture.md` | Directory map, contracts, checklists |
| 🛡️ Constitution | `CONSTITUTION.md` | Immutable laws (6 sections) |
| 🧪 Testing | `docs/skills/testing-guide.md` | Patterns, record-replay |
| 🤖 Agent loop | `docs/skills/agent-loop.md` | Spec + implementation guide |
| 📇 Full catalog | `docs/INDEX.md` | All docs indexed |
| 🗺️ Skill map | `docs/skills/README.md` | When to load which skill |

## Contributing

PRs welcome via Conventional Commits. Read [`CONTRIBUTING.md`](CONTRIBUTING.md)
for standards and [`AGENTS.md`](AGENTS.md) for AI assistant rules.

## License

Proprietary.
