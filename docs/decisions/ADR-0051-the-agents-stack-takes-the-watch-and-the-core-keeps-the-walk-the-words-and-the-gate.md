---
id: ADR-0051
title: The Agents stack takes the watch; the core keeps the folder walk, the words and the gate
status: proposed
date: 2026-11-12
subsystems: [platform, guardrails]
supersedes:
superseded_by:
---

# ADR-0051: The Agents stack takes the watch; the core keeps the folder walk, the words and the gate

Proposed: it is in force when the owner accepts it. Carries out [ADR-0032](ADR-0032-a-stack-is-a-package-with-a-manifest-kept-apart-by-tests.md)
for a second stack, following [ADR-0035](ADR-0035-a-stack-reaches-the-service-only-through-named-ports-and-keeps-only-the-files-it-lists.md),
[ADR-0036](ADR-0036-a-stack-offers-findings-by-id-and-the-core-words-them.md) and [ADR-0039](ADR-0039-how-the-listening-and-discovering-sensors-reach-the-network-stack.md).

## Context
- The Agents stack has been a registry entry since ADR-0028: two modules (`agent_activity`, `secrets_pass`) and the Lock down action. Its code was
  `core/agentwatch` (about 470 lines) plus a handler, a store and a model type in the core.
- **Owner direction (2026-11-12):** move the agent logic into stacks in stages, so the main service is eventually pluggable; start with the watch only.
- The watch needs three things only the core has: the agent events on the event bus (core types that carry arguments and error text), the workspace
  folders and the secret shapes (`tools.SecretKindIn`), and the operator's connectors. Constitution IV.6 forbids a stack from importing any of them
  and ADR-0035 keeps host reads in the core.

## Decision
We will move the record, its alert rules and its route into `stacks/agents`, and keep three things in the core:
- **The gate and the feed.** The core reads the bus (`Bus.SetTap`), checks the activity switch with a cheap typed read on the publisher's goroutine
  (no `Config.Load` on that path), vets every name (a tool name, an automation name, the host of a `fetch_url`; the workspace id is the service's own and is passed as it always was) and pushes a plain
  `stacksdk.Observation` through the optional `Observer` interface (`Manifest.Observes` names the kinds). The stack queues it and returns; a full queue
  drops and counts, as before. This is the observation push ADR-0039 Q3 described. Its fields are agent-shaped; a stack that listens to something else declares its own kinds in `Observes` and adds the fields it needs. The queue exists from the stack's construction, so an event published before the stack starts is kept for the loop.
- **The folder walk.** A new `SecretScan` port (`PortSecretScan`): `Scan(ctx)` returns `(workspace, relative path, kind)` and nothing else. The core
  walks the folders, keeps the bounds (1 MiB a file, 5,000 files a pass) and detects the secret; the stack keeps the schedule, the once-only record and
  the alert. No file's bytes cross the port, and the stack reads no workspace folder.
- **The words.** The stack publishes a typed `stacksdk.Alert` (an id and vetted tokens) through the existing `Events` port; `agentwatch.Words` is the only
  place a sentence is built, from a fixed table, and drops an id it does not know or a token that is not plain. The sentences are the ones sent before the
  move, pinned by a test.

Lock down stays in the core. The run path calls it directly (`assistant/tool_exec.go`, `guardrails`), so a stack that could be absent must not hold it
(ADR-0033, ADR-0034). The stack's manifest does not list it; the registry keeps it as the Agents stack's action, as before.

The record file, its keys and kind strings, the `agents` settings document, the route (`GET /admin/api/agent-watch`) and every alert sentence are
unchanged, so a record written by an earlier version reads back as it was and the screen needs no change. The stack requires the ports `config`,
`events`, `log`, `datadir` and `secretscan`: no dial, no fetch, no program.

## Consequences
- Easier: the watch changes without touching the engine; the stack's tests run alone (`GOWORK=off`); the next stack can reuse `Observer` and `Alert`.
- Harder: the SDK grows by one port, one optional interface and two data types; the core keeps a small package (`core/agentwatch`) as the stack's
  adapter, and the vetting code stays in the core (it sees untrusted model text, so that is the right side of the boundary).
- Cost: one more goroutine (the stack's loop, as the watch's was) and a JSON encode per alert; no added latency on the agent loop.
- The stack does not make agent capability removable: the engine, guardrails, memory, delegation and Lock down stay core. Removing the stack removes
  the oversight, not the agents.

## Alternatives considered
- **A stack that walks the workspace folders itself** — a second host reader in a stack (ADR-0035 rejected it) and the secret bytes would cross.
- **Pass the raw event (`AgentEvent`) to the stack** — it carries tool arguments, URLs and error text, and is a core type; it would break IV.6's
  plain-data rule.
- **The stack words its own alerts** — a sentence built in a stack from kept text is what ADR-0036 keeps out of the operator's phone.
- **Move Lock down too** — the run path depends on it; see above.
- **Move the whole agent engine now** — the owner's direction, in stages; this is stage one and the plan lists what a later stage needs.

## Links
- [`../PLANS/cross-cutting/agents-stack-watch.md`](../PLANS/cross-cutting/agents-stack-watch.md).
- `stacks/agents/`, `stacks/sdk/ports.go`, `backend/internal/core/agentwatch/`, `backend/internal/app/stacks.go`.
