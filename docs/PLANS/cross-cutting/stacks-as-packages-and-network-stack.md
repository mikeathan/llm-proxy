---
status: proposed
date: 2026-11-06
last_reviewed: 2026-11-06
related_specs: [SPEC-006]
constitution_references: [I.1, I.2, I.3, I.5, II.3, IV.2, IV.6, V.1]
evidence: Read on 2026-11-06 at origin/main 55466a3b (v0.55.1) - backend/internal/core/stack/registry.go and app/stacks.go; backend/go.mod (one module at the time); import counts of backend/internal/core/netaudit (about 21,500 lines, 26 importing files, 63 files importing llm-proxy/models); transport/http/embeds.go (go:embed); platform/sandbox, platform/progexec; core/assistant/guardrails/guardrails.go:170; ADR-0023, 0028 to 0031. Nothing built or run for this plan.
---

# Stacks as packages: the Network stack first

**Status:** proposed (2026-11-06). Design only; no code is written. The owner asked for the ADRs, the Constitution text and the plans first, then
a new session to build.
**Owner direction:** a stack is "an isolated package outside of main source code", activated in the service, movable to a separate repository
later; Network moves first and everything else stays future work (§6). Added 2026-11-06: no added latency and no added complexity; the rules
that keep stacks apart are Constitution law (IV.6); **running a stack as its own process is out of scope** (§6).
**Related:** [ADR-0032](../../decisions/ADR-0032-a-stack-is-a-package-with-a-manifest-kept-apart-by-tests.md),
[ADR-0033](../../decisions/ADR-0033-the-sandbox-is-core-and-stacks-gain-no-way-around-it.md), [ADR-0028](../../decisions/ADR-0028-stack-module-registry.md),
[ADR-0023](../../decisions/ADR-0023-network-ideas-rejected-or-already-built.md), blocked follow-on [`network-stack-privileged-additions.md`](network-stack-privileged-additions.md).

## 1. Why

Today a stack is a menu and a registry; the code is one Go module under `internal/`, and Network (about 21,500 lines) is entangled with it
(§2). Every Network change can touch the shared settings types, the tool registry and the prompts. The aim is a boundary the compiler and a
test enforce, with no runtime cost.

## 2. Evidence: what Network depends on (2026-11-06)

`core/netaudit` has about 21,500 lines (non-test), and 26 files elsewhere import it. Its imports outside itself are the SDK's shopping list:

| Import | Count | Becomes |
|---|---|---|
| `llm-proxy/models` (config and shared types) | 63 files | Stack-owned settings types; same persisted keys (`netaudit:`) |
| `core/tools`, `core/toolregistry` | 4, 1 | Tool declarations in the manifest |
| `assistant/prompts` | 1 | A prompt fragment the stack supplies (Constitution III.13 still applies) |
| `platform/progexec`, `platform/sandbox` | 4, 1 | The isolation library, through the SDK (P1) |
| `platform/scanscope`, `hostwatch`, `mcast`, `neighbors` | 4, 4, 1, - | Move into the stack, or the SDK if shared |
| sub-packages (`routerlog`, `upnp`, `webprobe`, `vuln*`, `dnswire`, `oui`, `names`, `phonehome`, `wifilook`, ...) | - | Already separate directories: the natural leaf-first order |

**Not yet known, and P2 must find out:** how much storage, event and settings state Network shares with the core (the session store, the
event bus, the global settings). Each shared handle must become a port (IV.6) before the move.

## 3. Target shape

```
go.work                optional and gitignored (the repo ignores it): joins the modules below for editors; nothing depends on it
stacks/                outside backend/; each folder is its own Go module
  sdk/                 contracts only (module llm-proxy-stacks/sdk, package stacksdk): manifest, ports, data types, guard + isolation entry points
  network/             the Network stack: requires the SDK and nothing else of ours
  example/             the reference stack
backend/               the service (module llm-proxy); requires the SDK, replace => ../stacks/sdk
  internal/app/stacks.go   the one place that wires stacks into the core (registers manifests)
```

Pattern: modular monolith with ports and adapters; the SDK holds the ports, `internal/app/stacks.go` is the composition root (ADR-0032, Pattern).
Several Go modules, one binary, one unit. The core calls a stack through an interface, which is an in-process call: no hop. A stack's ports carry
plain data only (IV.6), so it could be moved to its own process later without touching callers (§6).

## 4. Decisions to settle before P3 (recommendation first)

| | Question | Recommendation |
|---|---|---|
| D1 | Where do the SDK and stacks live? | `stacks/sdk` and `stacks/<name>/` at the repository root, each its own Go module; `replace` lines make every module build without a workspace (owner's choice, 2026-11-07; first draft was `backend/stacks` in one module). Moving a stack to its own repository is moving its folder. |
| D2 | Who owns the scan scope? | Policy in the core's settings; the stack reads it as a value through the SDK and enforces it (Constitution II.3 item 3). The stack cannot widen it (IV.6). |
| D3 | Settings types | Move to the stack; the persisted `netaudit:` keys and file format do not change. |
| D4 | UI | Stays in the main frontend bundle (`go:embed`). A declarative description rendered by the shell is future work (§6). |

## 5. Phases

Each phase is its own change set, built and reviewed on its own; none starts before the one above it passes.

**P0 - Decisions (docs only).** Accept ADR-0032 and ADR-0033; merge Constitution IV.6 (proposed in this branch). *Accept when:*
`./scripts/check-agent-harness.sh` passes and the owner has accepted both ADRs and IV.6.

**P1 - Isolation as a standalone library, no behaviour change.** Extract `platform/sandbox` and the guarded-program runner (`progexec`)
behind one package with a `Profile` type and a conformance suite (filesystem jail, TCP denial, Unix-socket denial, resource limits, clean
environment). List every existing relaxation (`sandboxing.unix_sockets`, the sandbox and terminal switches, the 30-minute guardrail
override) and decide each; measure whether an approved override skips the terminal allow-list checks. *Accept when:* `go build ./... && go test ./...`,
`go run ./tools/check-complexity/` and the conformance suite pass; no test changed except by moving; the relaxation inventory is in the ledger.

**P2 - `stacksdk`, the manifest and the two enforcement tests.** Interfaces and data types only: manifest (modules, settings schema, tools,
ports), config access, events, logger, data directory, the network guard (`DialContext`, boundary checks), the isolation library, an error
type with a code. `core/stack` reads manifests; `stack.New` still refuses an inconsistent description. Add (a) the `go list` import test
and (b) the JSON round-trip test for every port call. Produce the coupling inventory of Network's storage, events and settings (§2).
*Accept when:* a test stack compiles importing only `stacksdk`; an import of `internal/`, `models` or another stack from `stacks/` fails the
build; a function-valued or channel-valued port field fails the round-trip test; a stack cannot dial without the guard (test).

**P3a - Thin proof.** Move one leaf sub-package (`oui` or `dnswire`) to `stacks/network/`. *Accept when:* build, tests and both enforcement
tests pass and nothing observable changes. If this is awkward, fix the SDK before going further.

**P3b - Move the rest of Network, still compiled in.** Leaf sub-packages first, then the core of `netaudit`, then the 26 importing files
through ports. Settings types move with their persisted keys unchanged. *Accept when:* every moved test passes unchanged in the new place;
an existing `settings.yml` loads and saves identically; the Modules tab, routes and agent tools behave as before; the frontend is
untouched; **no performance regression:** start-up time and the device-list and findings endpoints are measured before and after on the same
machine, and the figures are recorded.

**P4 - Harvest and close.** `docs/architecture/map.md`, the network-audit and guardrails hubs, SPEC-006, a pitfalls entry for each trap
found, the learnings ledger; archive this plan.

## 6. Parked and future work (not in this plan)

- **A stack as its own process or unit.** Out of scope on the owner's decision (2026-11-06). The cost is a hop per call, a second binary, a
  supervisor, authentication and an approval flow. Revisit only when a feature needs a privilege the main unit refuses (the blocked plan
  names them). Constitution IV.6 keeps it cheap: plain-data ports, a stack-owned store, typed events and errors, tools as data.
- **Local inference as a stack.** The port already exists (the OpenAI-compatible protocol). Today the main unit carries `MemoryMax=85%` and
  `LimitMEMLOCK=infinity` because it launches `llama-server` as a child. As a package it changes nothing about that; only a separate process would.
- **Agents as a stack, Zeek and Suricata readers, a declarative stack UI, separate repositories** (after two stacks have lived on the SDK).

## 7. Risks

- **P3b is the largest change** (21,500 lines, 26 importing files). Slice by sub-package; P3a proves the boundary first.
- **The SDK grows to mirror `internal/`.** Keep it to contracts; anything with logic stays in the stack or the core.
- **Hidden shared state** (a database handle, the event bus). The coupling inventory in P2 exists to find it early.
- **Features that need a privilege stay blocked** by this plan; the owner accepted that on 2026-11-06.

## Remaining Work

**P1 built 2026-11-06 (branch `stacks-as-packages-docs`, uncommitted).** `backend/internal/platform/isolation` is the one door: a `Profile`
(root, read paths, no-network, Unix-socket relaxation, explicit environment, timeout, output cap; all relaxations off by default) and a
`Runner`, over the unchanged `platform/sandbox` and `platform/progexec`. `isolation/isolationtest` is the conformance suite (filesystem
jail, TCP denial, Unix-socket denial incl. the per-spawn and operator relaxations, clean environment, per-file size limit, honest
reporting). The relaxation inventory and the override measurement are in `docs/learnings/guardrails.md`. Existing importers (22 files)
still import `sandbox` and `progexec` directly and no test moved; switching them to `isolation` is a mechanical follow-up inside P2.
**P2 built 2026-11-06 (same branch, uncommitted).** `stacks/sdk` (manifest and `Validate`, `Stack`/`Host` and the ports Config,
Events, Logger, DataDir, Guard, Isolation, `Error` with a code, `CheckPlainData`; stdlib only); `stacks/example` (the reference
stack, not registered); `core/stack` reads manifests (`StackFromManifest`, `ModulesFromManifest`, `Wiring`) and `StackID.Valid` is now
a slug check (which stacks exist is decided by what is registered); `internal/archtest` holds the fitness functions: stacks import only
the SDK, the SDK imports only the standard library, only `internal/app/stacks.go` imports a stack, and a source scan refuses
`net.Dial/Listen`, `http.Get/DefaultClient`, `exec.Command`, `syscall.Exec/Socket` and the like in a stack. The JSON round-trip test
over `stacksdk.PortTypes()` and the plain-data check are in `stacksdk`. The coupling inventory is
[`docs/audits/2026-11-06-network-stack-coupling-inventory.md`](../../audits/2026-11-06-network-stack-coupling-inventory.md). Not built:
the core-side adapters for the ports (they arrive with P3a, where something uses them). Open for P3: an inbound-socket `Listen` port
(decoy, router log and Nearby cannot move without it), the `nmcli` Unix-socket entry, per-workspace stores.
**Layout changed 2026-11-07 (owner):** the SDK and stacks moved outside `backend/` into `stacks/sdk` and `stacks/example`, each its own Go module
(`llm-proxy-stacks/sdk`, `llm-proxy-stacks/example`), `go.work` is gitignored in this repo and optional; `backend/go.mod` requires the SDK with a `replace`. The compiler now refuses
`internal/` imports; a workspace still lets a stack see `llm-proxy/models`, so the archtest also reads each stack's `go.mod` and CI builds/tests each stack
with `GOWORK=off`. ADR-0032, Constitution IV.6 and D1 were updated to match.
CI (same branch): the boundary tests and isolation suite run inside the existing `backend` job (`go test ./... -race`); one small step per job runs the stack modules (build, vet, test) with the workspace off; the sandbox-conformance job also runs the isolation suite. The macOS conformance job now runs only when a PR touches the
sandbox, progexec, isolation, process, module files, `main.go` or `ci.yml` (and on manual runs): its runner queue made 6 of the last
30 runs take 11 to 22 minutes against a median of 4. Not run on GitHub yet.
**P3a built 2026-11-07 (branch `stacks-p3a-thin-proof`).** `oui` (card maker from the MAC; stdlib only; embeds `oui.tsv.gz`) and its generator
`gen-oui` moved to `stacks/network` (module `llm-proxy-stacks/network`, no code but these yet; no manifest or `Stack` until something needs
one). The move was verbatim and the existing tests passed, including the `netaudit` test that reads a real vendor name through the embedded
file across the module boundary. **Findings that fix the plan:** (1) a leaf is not a leaf while core code calls it: both callers of `oui` are in
`core/netaudit`, which IV.6 forbids importing a stack. Decision (owner, 2026-11-07): a shrink-only `migrationDebt` list in the boundary test
(one entry: `core/netaudit` -> `network/oui`), with one sentence in IV.6; it must be empty when P3b ends, and a test fails on a stale entry. P3b
should therefore move `core/netaudit` itself early, not last, or the list grows with every leaf. (2) `check-complexity` walked only `backend/internal`, so
moved code would have left the gate (it also found a function of mine over the limit in the SDK, refactored); it now walks `stacks/` too.
(3) Stack coverage is not in `coverage.out` or the CRAP report (separate modules); CI prints it per stack with `-cover`. Merging profiles
is not done because the CRAP scanner could not be run to check its path handling. (4) The boundary tests run stack modules with `GOWORK=off`,
so a developer's local `go.work` cannot change the result.
**P3b design written 2026-11-07:** [`network-stack-p3b-design.md`](network-stack-p3b-design.md) (corrects the P3a advice to move `core/netaudit` early: it is the hub and moves last; consumers move with each leaf); decisions D5 to D9 accepted by the owner 2026-11-07, with the conditions: no performance regression, no extra processing, no unnecessary complexity (design note section 4a). Next: S1.
ADR-0032, ADR-0033 and IV.6 were accepted by merging #201 (2026-11-07). Owner decision still waiting: the override finding (approved calls
skip the checks after the one shown). Next: P3b on the owner's go-ahead.
