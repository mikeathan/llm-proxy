# Architecture Map — What Lives Where

The package and directory map: which code owns what. Start here to find where a change belongs.

Part of the [architecture reference](../architecture.md).

### Type Definitions

`models/` contains ALL shared types. No logic, only structs, constants, and interfaces.

- `models/config.go` — ModelConfig, ProviderConfig, GPUConfig, AgentGuardrailsConfig
- `models/infrastructure.go` — SystemConfig, UserSettings, ModelOverride
- `models/registry.go` — RegistryData, ModelRegistryEntry
- `models/llm.go` — Provider interface, error sentinels
- `models/llm_messages.go` — Message, ToolCall, ChatRequest (includes `MaxTokens`)
- `models/tools.go` — Tool name constants
- `models/workspace.go` — Workspace, AutomationRun, AgentState

### Core Systems

- `internal/core/assistant/` — Agent loop, tool providers, guardrails, prompts, provider tiers
- `internal/core/netaudit/` — network-audit domain: scan scope, modes, scan service, readiness; engines in `nmap/` and `portprobe/`, the agent tool in `scantool/`
- `internal/platform/progexec/`, `internal/platform/scanscope/` — the guarded-program runner and the scan-target policy
- `internal/core/toolregistry/` — the local tool registry, MCP engine and the composite provider/engine (wired by `InitializeAgentStack`)
- `internal/core/proxy/` — LLM HTTP client, XML tool call parser, history normalization
- `internal/core/proxy/recorder/` — `RecordingClient` decorator (captures LLM responses to JSONL)
- `internal/core/llm/` — Model lifecycle (start/stop/reap), GGUF scanning, provider registry
- `internal/core/automation/` — Scheduled task dispatch and execution, split by concern: `dispatcher.go` (type/constructor/metrics), `scheduling.go` (cron registration + fsnotify hot-reload), `admission.go` (lane admission, flock wait, cancel-queued), `execution.go` (run + stop), `history.go` (global activity ledger).
- `internal/core/eventbus/` — `Bus.Subscribe` / `Publish` / `Unsubscribe`, keyed by `(workspace, channel)`, fans out agent events to SSE-connected clients (`channel` is `EventChannel` — `assistant` | `automation`, see Pitfall #6); `Sink` writes the same events to a per-run JSONL file.
- `internal/core/chat/` — The chat turn lifecycle: `chat.Service.Run` registers the workspace's running turn (a new message replaces it, `Cancel` stops it), claims the run lane, waits for a cold model, opens the recording and calls the conversation service (`chat.ConversationService.Execute(ExecuteRequest)`, `conversation_service.go`: session, history, agent run, checkpoints, memory capture). The HTTP handlers and the connectors (webhook) call it; they carry none of this. `chat.Request` is the one message type for every channel. `chat` imports `assistant`, never the reverse.
- `internal/core/run/` — What every kind of run sets up the same way around the agent: `ResolveOptions` (`options.go`, the one precedence chain for a chat turn, an automation and a helper's task: the run's own choice → profile → model config → global default) and `OpenRecording` (the run folder, event file and request recording, for chat and automation). It imports `assistant`; `assistant` never imports it.
- `internal/core/delegation/` — Runs delegated tasks as cloud sub-agents (SPEC-001 §15): `Runner` implements `assistant.SubRunner` (injected at bootstrap with a copy of the services taken before the runner is set, so a child cannot delegate); `pool.go` is the sub-agent pool beside the run lanes.
- `internal/core/runlane/` — Global run scheduler: per-workload-class lanes (`local`/`cloud`), FIFO queueing, interactive preemption (SPEC-007 §V), plus the **model-residency gate**: external `/v1` callers are admitted to the local model (wait/full-refuse) and can never evict a model a run is using (SPEC-007 §V.1).
- `internal/platform/rundir/` — Run output directory layout (`RunDir`) and retention reaper (`RunReaper`).
- `internal/core/egress/` — loopback-only agent egress proxy (host domain allow/deny policy + dial-time `network.CheckAlwaysBlocked` IP guard; absolute-form + CONNECT). Sandboxing plan D2/Phase 1.
- Sandboxing enforcement lives in `platform/sandbox` (Provider/Effective) + `platform/process` (spawn) + `platform/sizewatch` (disk accounting). To add a NEW enforcement mechanism (e.g. a future executor): see `docs/PLANS/ARCHIVE/cross-cutting/agent-os-sandboxing.md` §4.3/§9 — add a build-tagged adapter + capability detection at `sandbox.New` + an `Effective` label + UI badge; the spawn seam and downgrade contract are already wired.
- `internal/core/tools/` — Tool implementations (terminal, filesystem, network, search, memory, communication). Multi-implementation families live in subpackages: `tools/searchproviders/` (search backends) and `tools/notifiers/` (connectors).
- `internal/core/commands/` — Slash commands (SPEC-011 §2.11): a channel-independent `Registry` (`Match`/`Execute`/`Preview`/`List`), the `/remind` command and the turn commands `/research` and `/agent` (`agent.go`, SPEC-012 1.6). The web chat (`handlers/command_handlers.go`) and the inbound webhook (`WebhookHandler.handleCommand`) both call it, so a new command appears in every channel. A command returns text and never touches a transport; a turn command returns a `TurnSpec` (message, agent, helpers) that each channel starts as a chat turn (`chat.Request.Agent`, for that message only). `/run <automation>` is a registry command too (`run.go`): it triggers the automation in the message's workspace (`Env.WorkspaceID`) through an `AutomationStarter` port and names it in `Reply.Automation`, and the webhook then follows that automation's event stream and sends its result (`followAutomation`); the web chat shows the start line.
- `internal/core/reminders/` — Reminders (SPEC-011): `when.go` (`ParseCommand`: the one place that turns "sunday 8pm …" into an instant or cron), SQLite `Store`, `Service` (create/cancel plus the one timer loop `Run`, late/missed policy, retries), `schedule.go` (timezone resolution, cron next-fire, validation), `transition.go` (the whole delivery lifecycle as one pure `Policy.Advance`: retry, move on, close, late/missed) and `channel.go` (`ChannelPolicy.Resolve`: the connectors a reminder goes to — every enabled one not in `reminders.skip_connectors`; delivery to several succeeds when at least one takes it and never retries the ones that failed beside a success). New reminder states or rules go in those two, not in `Service` (pause, resume and skip are pure functions in `transition.go`). A user change bumps `Reminder.Rev` and the loop's write-back is guarded on it: never write a delivery result without the `Rev` it read. No import of `tools` or HTTP: delivery and the in-app notice arrive through its `Deliverer` / `Notice` ports; the app layer adapts them (`internal/app/reminders.go`) and the agent tools (`tools/reminders.go`) reach it through `tools.ReminderService`.
- `internal/core/memorycapture/` — Domain logic for saving memory from chats (SPEC-004 §II.8): the explicit-request keyword table (`phrases.go`) and extractor, shared guards, `Capture`, and the per-chat `Reviewer`. No imports of `assistant`, `tools` or HTTP: storage and the model arrive through its `Library` / `Completer` ports (adapters live in `chat/memory_capture.go` and `handlers/assistant_memory_review_handlers.go`).
- `internal/core/mcp/` — MCP client (SSE transport, tool mirroring)
- `internal/core/orchestrator/` — Token budget management, context length resolution, slot scheduling, stream interleaving, reasoning budget normalization. See SPEC-005.
  - `slot_manager.go` — Manages concurrent inference slots (per-model capacity, queueing, timeout).
  - `budget_manager.go` — Token budget tracking per time window (`Spend`/`Refund`), throttling.
  - `budget_squeezer.go` — Compression fallback when budgets are tight (reduces context).
  - `stream_interceptor.go` — SSE stream parsing, interleaving parallel tool call outputs.
  - `reasoning_normalizer.go` — Normalizes reasoning token formats across providers.
- `internal/core/nodeherder/` — MCP tool provider adapter. Wraps the MCP orchestrator into a `ToolProvider` interface that the agent calls via `ListTools`/`ExecuteTool`. Manages MCP tool registration, mirroring, and credential injection.
  - `provider.go` — `ListTools` (polls MCP server), `ExecuteTool` (forwards to MCP server via `CallTool`), subscription to system prompt updates.
  - `token_manager.go` — Capability token resolution for MCP tool authentication.
- `internal/testing/llmprofiles/` — `FixtureClient` + `RunAgainstFixtures` (replay test framework)

### Infrastructure

- `internal/platform/storage/` — Generic atomic JSON/YAML stores with change callbacks
- `internal/platform/network/` — Shared/guarded HTTP transports + Constitution I.2 infrastructure doer (plus the single-stream audit-invariant test)
- `internal/platform/process/` — Central agent child spawn (R1: Setpgid, kill-group) + rlimit application (Darwin/Linux, measured matrix)
- `internal/platform/sandbox/` — OS confinement Provider seam + `Effective` downgrade reporting; Linux Landlock mechanism + self-exec runner; grant profile model
- `internal/platform/sizewatch/` — TTL-cached workspace disk accounting (`max_storage_gb`, best-effort)
- `internal/platform/units/` — binary size conversions (KiB/MiB/GiB → bytes) shared by storage accounting, sandbox rlimits and fetch limits (single home; no inline `1024`/`<<30` arithmetic)
- `internal/platform/logging/` — Structured logging (global + per-workspace process logs)
- `internal/app/` — Bootstrap, AppContext (central state manager), service wiring
- `internal/transport/http/` — Router, middleware, frontend embed (`handlers/admin_ui.go`: SPA fallback — client routes get `index.html`, `/admin/api/*`, `assets/` and root files 404; SPEC-003 §III.3)
- `internal/transport/http/handlers/` — HTTP handler types (Admin, System, Process, MCP, Model, Secrets, Dispatcher, Assistant, Proxy, Recordings, Memory, Webhook)

### Frontend (`frontend/src/`)

The admin UI contract — destinations, route table, theme tokens, tree, notifications — is SPEC-003 (`docs/SPECS/discovery-panel.md`); its §III.1 is the directory map. Entry points:

- `main.ts` — the one bootstrap: router, `startRunNotifications(router)`, mount
- `router/index.ts` (route table) · `router/routes.ts` (typed builders — the only way to build a location) · `types/routes.ts` (route names)
- `theme/tokenRegistry.ts` (every token name) · `styles/tokens.css` (preset values as `R G B` channels) · `public/theme-boot.js` (pre-paint, contract-tested against `theme/apply.ts`)
- `components/common/<domain>/` shared primitives · `components/layout/` shell · `views/` one lazy chunk per destination
- `/design` (dev builds only) — living gallery of tokens and primitives, with Playwright visual baselines (`e2e/`, `npm run test:visual`)
