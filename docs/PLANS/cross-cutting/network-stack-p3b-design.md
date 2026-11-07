---
status: approved
date: 2026-11-07
last_reviewed: 2026-11-07
related_specs: [SPEC-006]
constitution_references: [I.1, I.6, I.8, I.9, II.3, IV.6, V.1]
evidence: Measured 2026-11-07 at origin/main f3c98e8b (v0.55.4) with `go list -json ./...` over backend/internal/core/netaudit/... and grep over internal/app and internal/transport/http/handlers; nothing built or run for this note. Builds on the P2 coupling inventory (docs/audits/2026-11-06-network-stack-coupling-inventory.md) and the P3a findings.
---

# P3b design: how Network moves into `stacks/network`

**Status:** approved 2026-11-07; design only, no code yet. The owner accepted D5 to D9 as recommended, on three conditions that every slice must meet (section 4a). Parent plan:
[`stacks-as-packages-and-network-stack.md`](stacks-as-packages-and-network-stack.md).

## 1. What the numbers say

`core/netaudit` is 75 files and about 21,400 non-test lines, with 19 sub-packages. Measured by import, not by size:

- **Every sub-package is consumed from outside `netaudit`**, almost always by `internal/app` (10 files import `netaudit`) and often by
  `transport/http/handlers` (16 handler files, 91 distinct `netaudit.*` symbols, mostly view and response types). The root package imports
  10 of its own sub-packages (`dnscheck`, `outsidelookup`, `phonehome`, `routerlog`, `ubuntuoval`, `upnp`, `vulnfeed`, `vulnmatch`,
  `webprobe`, `wifilook`); `nmap`, `portprobe` and `scantool` import the root.
- **Outbound ties are few and already named**: `models` (11 packages), `core/tools` (3), `platform/progexec` (4), `platform/scanscope` (2),
  `platform/hostwatch`, `platform/mcast`, `platform/sandbox`, `core/toolregistry` and `assistant/prompts` (1 each).
- **No sub-package is a true leaf for the core**: `oui` was only movable because the debt list let `core/netaudit` keep calling it.

**This corrects a P3a recommendation.** I said P3b should move `core/netaudit` itself early. The data says it cannot: the root is the
hub (it imports 10 sub-packages and is imported by 3), so it moves last. Moving leaves first is right, but a leaf moves with its
consumers, not alone.

## 2. The real problem is the consumers

A leaf's callers are in `internal/app` (wiring) and in the HTTP handlers. IV.6 allows only `internal/app/stacks.go` to import a stack, and
`TestOnlyTheCompositionRootImportsAStack` enforces it file by file, so the nine `app/netaudit*.go` files cannot import a stack and
16 handlers cannot serialise stack types. A debt entry per consumer would grow with every slice, the opposite of what the list is for.

## 3. Proposal: a stack owns its routes and its wiring, as data

1. **Routes (recommended, D5).** The manifest declares each route as data: method, path, access level (admin, step-up, public). The
   stack implements one call, `Handle(ctx, Request) Response`, where both are plain data (method, path, query, body bytes; status, body
   bytes). The core mounts the routes under the same URLs, applies authentication, step-up and the rest of the middleware, then calls the
   stack in-process. The frontend and the JSON do not change. No latency is added: a handler already reads and writes the same bytes. The
   16 handler files and the `app/netaudit*.go` wiring move into the stack with their leaf; `stacks.go` keeps one constructor call per stack.
   Checked: none of the Network handlers streams (no `text/event-stream`), so a request-response shape fits.
2. **Settings (D8).** The core stores the stack's settings as opaque JSON under the unchanged `netaudit:` key, and the manifest carries
   the schema. `models.Config` stops holding a Network type. This is the riskiest step (61 uses of `NetauditConfig`); it needs the
   plan's acceptance test first: an existing `settings.yml` loads and saves byte-for-byte the same. **Unverified**: how `models.Config`
   embeds the Network settings today and how `config-persistence` round-trips them; read before slice 1.
3. **Inbound sockets (D6).** Instead of a `Listen` port that returns a live listener (a capability, not plain data), the core keeps
   every listening socket and its guard rules (I.6, I.8, I.9) and hands the stack what arrives: `Ingest(datagram)` for the router log and
   multicast. The decoy is an interactive TCP listener; it needs a closer look and may stay in the core for now.
4. **Storage (D7).** The stack owns its files (IV.6) under a per-workspace data directory the core hands it; the files stay at their
   current paths, so nothing migrates. This needs `DataDir` to take a workspace. **Unverified**: the exact files and who else reads them.
5. **Alerts and tools.** Alerts leave through the SDK's typed events; tools are declared in the manifest and called by name with JSON;
   the guard's errors become SDK codes; `RedactSecrets` gets an SDK home or a copy (inventory §3).

## 4. Slicing (each slice its own PR, no behaviour change)

| Slice | Content | Proves |
|---|---|---|
| S1 | SDK `Route` + `Handle`, the core's mount, and the settings-as-data path, shown on the example stack only; measure and record the device-list and findings endpoints before and after (baseline taken here) | routes and settings without Network |
| S2 | First real vertical: **DNS check** (`dnscheck`, `outsidelookup`, their config, handlers and wiring). Needs only the guard, settings and routes (D9) | guard, errors, routes on real code |
| S3 | Pure data leaves with handlers: `vulnindex`, `vulnfeed`, `vulnmatch`, `ubuntuoval`, `upnp`, `webprobe`, `dnswire`, `names`, `nearby` | store ownership, per-workspace paths |
| S4 | Ingest-fed: `routerlog`, `knownbad`, `phonehome`, then `mcast` users | the ingest port |
| S5 | Programs: `nmap`, `portprobe`, `scantool`, `wifilook` (isolation, the `nmcli` Unix-socket table entry, the agent tool and prompt fragment) | isolation port and tools |
| S6 | The root `netaudit`, `decoy`, then delete `migrationDebt` | the end state |

Each slice ends with: build, tests, both boundary tests, complexity gate, harness check, an existing `settings.yml` loading
unchanged, and the measured figures of section 4a recorded beside the baseline with no regression.

## 4a. Owner's conditions (2026-11-07): clean architecture, and nothing gets slower or heavier

"Accept all of them, as long as we do not have performance issues, unnecessary complexity or more processing; just clean architecture."
Every slice is checked against these, and a slice that fails one is not merged:

- **No performance regression.** Before S1, record the baseline on one machine: latency of the device-list and findings endpoints,
  start-up time, and allocations per request (a Go benchmark of the route path). After each slice the same figures are recorded beside
  them. The route call is an in-process function call, and the request and response bodies pass as the same bytes a handler reads and
  writes today.
- **No more processing.** No extra marshal, unmarshal or copy of a body on the hot path; no goroutine, queue or timer is added by a port.
  A port that needs one is redesigned, not accepted.
- **No unnecessary complexity.** A port exists only when a stack needs it today. Code that moves is removed from its old place in
  the same slice, so total code does not grow; `go run ./tools/check-complexity/` (now covering `stacks/`) has no new violation; a slice
  that adds more lines of glue than it moves needs its reason written in the PR.

## 5. Decisions (all accepted 2026-11-07)

| | Question | Recommendation |
|---|---|---|
| D5 | How do HTTP handlers reach a stack? (accepted) | The stack owns its routes as plain-data request and response (section 3.1). The alternatives are typed interfaces with a debt entry per handler (grows, ends in the wrong place) or moving the view types into the SDK (the SDK becomes a copy of `netaudit`). |
| D6 | Inbound sockets | The core keeps the sockets and pushes datagrams in; no `Listen` port. |
| D7 | Storage | The stack owns its files under a per-workspace data directory; paths unchanged. |
| D8 | Settings | Opaque JSON under the unchanged key; schema in the manifest. |
| D9 | First real slice | DNS check (small, needs only guard, settings, routes). |

## 6. Risks

- **The SDK grows to fit Network.** Each new port must serve a second stack in principle, or stay in the core.
- **Settings round trip** is the one place a user's data could be lost; slice 1 does not touch Network so it can be tested first.
- **Handler logic is not trivial** (4,584 lines across the 16 handlers and the `app/netaudit*.go` wiring); moving it needs its tests
  to move unchanged.
