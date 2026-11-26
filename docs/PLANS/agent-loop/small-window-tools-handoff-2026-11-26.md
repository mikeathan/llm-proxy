---
status: partial
date: 2026-11-26
last_reviewed: 2026-11-26
related_specs: [SPEC-005, SPEC-001, SPEC-003]
constitution_references: [VI]
related_plans: [../ARCHIVE/agent-loop/request-size-estimator.md]
evidence: vertex app log 2026-11-21T16:19:45Z ("the system prompt, task and tools leave no room in the 8192-token window (the request was 8724 tokens)"); vertex /admin/api/registry read 2026-11-26 (Inkling 2 Mini and Ornith-1.5-35B at --ctx-size 8192, no tool_call_format); TestFirstChatRequest_FitsAnEightKWindow.
---

# Small local windows: the tools a chat is shown (handoff)

## Why

A chat on Inkling 2 Mini at `--ctx-size 8192` failed before the question was read: the first request was 8724 tokens.
The fixed part had grown with the tools (12 tools / 6,316 chars of schema on 2026-10-18; 26 tools / ~19,900 chars on
2026-11-26), and every tool's description was sent twice: in the tools field and again in the system prompt's
"AVAILABLE TOOLS" list (~7,500 chars).

## Built (branch `prompt-fit-small-windows`, from origin/main 7b0c0bdc, uncommitted, not pushed)

1. **The native tool reference names the tools only** (`prompts.BuildNativeToolReference`): header and rules 1–5 kept,
   descriptions dropped. Every native run, local and cloud, is ~7,500 chars smaller. The XML manual is unchanged.
2. **A chat on a small local window is shown the core tools** (`toolpolicy.SmallWindowTools`, `Agent.fitToolsToWindow`):
   small = local workload with a context budget below what a 16384-token window derives. Core = filesystem, search,
   network (fetch_url, get_network_info), memory, plus the always-on system/security/journal tools. Left out: terminal,
   notify_user, reminders, newsroom, network scan/findings, radio digest, delegate, stack and MCP tools. Not trimmed: a
   run with an allow list (automations that chose tools, helpers), any automation (unattended: it fails with the
   too-large error rather than report without a needed tool), any cloud run. Decided once when the run is built.
3. **`--ctx-size` in a model's args counts as its window** (`orchestrator.ResolveLocalContext`, SPEC-005 §2): server
   report → args → training context → 8192. One parser, `orchestrator.ServingContextFromArgs`, also used by the
   OpenAI-style model list. Before, a model whose server had not answered was sized by its GGUF training context
   (Inkling: 1,048,576).
4. **Models page notice** (`small_window` on the model view, `ModelCard.vue`): one line saying chats get the core
   tools and how to lift it. Same rule as the agent (`toolpolicy.IsSmallWindow`).

Skipped on purpose: shortening tool descriptions (item 3 of the analysis). After 1 and 2 it saves little on a small
window, and description text is tuned behaviour on every window (pitfall 57 shows how sensitive it is).

5. **Built (owner asked 2026-11-26, both parts in this PR):** a stored `n_ctx` no longer outranks `--ctx-size` for a
   launched model (`orchestrator.DropStoredServingContext`, called in `providers.NormalizeModelConfig` and on the
   saved copy in `persistModelAndOverrides`), and the add/edit enrichment records `n_ctx` for a local model only when a
   server reported it (`servingWindowToRecord`). The live report still wins in memory; a Sync goes back to the args
   until the server is asked again. Checked against vertex's eight local models: every one resolves to its `--ctx-size`.

## Checks run (2026-11-26)

`go build ./...`, `go vet ./...`, `go test ./...` (120 packages ok), `go run ./tools/check-complexity/` (no new
violations), `npm test` (3678 passed), `npm run build`, `npm run test:visual` (430 passed, 6 skipped),
`./scripts/check-agent-harness.sh` (pass). **Not run:** a live chat on vertex.

Measured by `TestFirstChatRequest_FitsAnEightKWindow`: a General chat's first request on 8k is 15,676 chars of the
16,386 allowed at the default 3.0 chars/token (96%); at Inkling's observed 3.9 that is ~74%, leaving the conversation
about a quarter of the window. 8k stays tight: one large search result gets cut by the sieve.

## Left, in order

1. Owner: review, commit (include `backend/internal/transport/http/frontend_dist/index.html`), push, PR, merge.
2. Owner + agent: deploy to vertex, chat with Inkling at 8192 ("what can you do?", then a web look-up and a memory
   save); the app log should show "small context window: this run is offered the core tools only" once per run.
   Ornith-1.5-35B is also on 8192 on vertex and gets the same treatment.
3. Owner decision if wanted: whether reminders belong in the small-window core (≈2,560 chars) — General's prompt still
   mentions them.
4. After deploy, check the Models page and `/v1/models`: Ornith-1.5-35B should show the small-window notice and an
   8192 window, NVIDIA Nemotron 3.5 Lightning 16384, before either has run. Their stale `n_ctx` stays in
   `registry.json` until each is next saved, but is ignored (see 5 below).

## Traps found

- The default guardrail config switches search and fetch off, so a test built on a bare engine under-counts tools.
- `automation_journal` is in every mock tool list but only offered to journal automations.
- The Models card's "context" figure is the GGUF training context, not the served window (unchanged here).
- The small-window rule judges the context budget, not the raw window: a 16k model with a raised `max_tokens` would be
  judged small. Checked on vertex's eight local models through `ApplyMetadataDefaults`: only Inkling is small.

## First thing to open

`backend/internal/core/assistant/toolpolicy/small_window.go`, then `TestNewAgent_ASmallLocalWindowOffersTheCoreTools`
in `backend/internal/core/assistant/request_size_test.go`.
