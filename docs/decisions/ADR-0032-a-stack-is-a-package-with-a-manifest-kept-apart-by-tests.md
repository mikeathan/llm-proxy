---
id: ADR-0032
title: A stack is a package with a manifest, kept apart by tests, and Network is the first one moved out of the main source
status: proposed
date: 2026-11-06
subsystems: [network-audit, platform]
supersedes:
superseded_by:
---

# ADR-0032: A stack is a package with a manifest, kept apart by tests, and Network is the first one moved out of the main source

Proposed: it is in force when the owner accepts it. It amends [ADR-0028](ADR-0028-stack-module-registry.md) in one point only (the
registry was "descriptive"); the rest of ADR-0028 stands, as ADR-0028 did with ADR-0022.

## Context
- **Today a stack is a menu and a registry, not a boundary.** ADR-0028: "The registry is descriptive: features keep their own packages".
  All code is one Go module (`backend/go.mod`) under `internal/`. `core/netaudit` is about 21,500 lines (tests excluded) and is imported by
  26 files of other packages; 63 of its own files import `llm-proxy/models`, and it also imports `core/tools`, `core/toolregistry`,
  `assistant/prompts`, `platform/progexec` and `platform/sandbox` (counted 2026-11-06).
- Network is the largest and fastest-growing feature area; every addition touches the shared settings types, the tool registry and the
  prompts, so a change in one place can break another.
- **Owner direction (2026-11-06):** a stack is an isolated package outside the main source, activated in the service, movable to its own
  repository later; Network moves first and everything else stays future work. The owner also asked for no added latency and no added
  complexity, and for the rules that keep stacks apart to be law (Constitution IV.6).

## Decision
We will define a **stack** as its own Go module under the repository's `stacks/<name>/` folder, outside `backend/`, with a **manifest** (its modules, the
schema of its settings, its tools, and the ports it provides and requires), requiring only the SDK module `stacks/sdk` (module `llm-proxy-stacks/sdk`, package name `stacksdk`). The rules that
keep it apart are Constitution IV.6. The compiler enforces most of them (a module outside `backend/` cannot see `internal/`, and cannot import what its `go.mod` does not list); tests enforce the rest: an import test over `go list`, a `go.mod` test, and a JSON round trip of every port call.
`backend/go.mod` requires the SDK with a `replace` to `../stacks/sdk`, and each stack's `go.mod` does the same, so every module builds alone; no `go.work` is committed (the repo ignores it; a developer may keep one locally, which lets a stack see the service's exported packages, so CI builds each stack with `GOWORK=off`). Stacks are compiled into the one binary and run under the one unit. Network moves first; other stacks stay in the main source until a plan
moves them. Moving a stack to its own repository is moving its folder.

**Parked, not built: running a stack as its own process.** Nothing in this ADR creates one. IV.6 keeps the door open at no runtime cost
(plain-data ports), and it is revisited only when a feature needs a privilege the main unit refuses (see the follow-on plan).

## Pattern
A **modular monolith** built with **ports and adapters** (hexagonal architecture):
- *Ports* are small interfaces in `stacksdk`, one per capability, carrying plain data. A stack implements them (the adapters); the core
  depends only on the interface. Core and stacks both depend on the SDK, never on each other's internals (dependency inversion).
- The **registry** (`core/stack`, extended to read manifests) is the microkernel half: a small core that stacks register into.
- **One composition root**, `internal/app/stacks.go`, is the only file that imports a stack and wires it in. Plain constructors; no
  dependency-injection framework.
- The rules are **fitness functions**: tests that fail the build (the `go list` import test, the `go.mod` test, the JSON round trip), not review conventions.
- Not used: microservices or a process boundary (parked), dynamic plugins, CQRS or event sourcing.
- The risk to guard against is an SDK that grows into a copy of `internal/`: keep each port to one capability, contracts only (IV.6).

## Consequences
- Easier: a stack changes without touching the rest; its tests run alone; moving it later is moving a directory that already builds against the
  SDK.
- No new runtime cost: the core calls a stack through an interface, an in-process call.
- Harder: an SDK to keep small and stable; the settings types leave `models` for the stack, with persisted keys unchanged; Network's tools and prompt
  fragments are declared in its manifest.
- **Constitution I.1 to I.3 hold unchanged:** the network guard is reached through the SDK, and a test pins that a stack cannot dial without it.
- Features that need a privilege the unit refuses (a raw socket, a capability) are blocked by this decision, not enabled by it.

## Alternatives considered
- **Stacks inside `backend/` in the one Go module** (the first draft of this ADR) — cheapest, but the compiler allows a stack to import `internal/`
  there (they share a parent), so only a test stands between them. The owner chose separate modules outside `backend/` on 2026-11-07: the
  cost is a `replace` line and a per-module CI loop; the gain is a boundary the compiler enforces and a folder that moves to its own repository unchanged.
- **One module for the SDK and all stacks under `stacks/`** — simpler, but stacks could import each other and none could be extracted alone.
- **Run each stack as its own process or unit now** — the only way to give a stack a privilege the agent lacks, but it adds a hop, a second
  binary, a supervisor and an approval flow. The owner chose to leave it out and keep the door open.
- **In-process plugins (Go `plugin`, WASM)** — Go plugins need identical toolchains and cannot unload; WASM cannot do raw sockets.
- **Keep one flat tree** — the entanglement above stays and grows.

## Links
- Plan: [`stacks-as-packages-and-network-stack.md`](../PLANS/cross-cutting/stacks-as-packages-and-network-stack.md); blocked follow-on:
  [`network-stack-privileged-additions.md`](../PLANS/cross-cutting/network-stack-privileged-additions.md).
- Constitution IV.6 (proposed). Related: [ADR-0028](ADR-0028-stack-module-registry.md), [ADR-0033](ADR-0033-the-sandbox-is-core-and-stacks-gain-no-way-around-it.md).
