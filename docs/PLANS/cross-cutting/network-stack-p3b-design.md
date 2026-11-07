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
| S1 (built, section 4c) | SDK `Route` + `Handle`, the core's mount and step-up wiring, shown on the example stack and a test double only; the baseline (section 4b). The settings adapter moved to S2 (finding 3) | routes without Network |
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

## 4b. Baseline (recorded 2026-11-07, before any Network code moves)

Taken on the owner's development machine (Apple silicon, Go 1.26.2) at `origin/main` f3c98e8b plus the benchmark file; run again after each slice
on the same machine and write the new figures beside these.

| Measure | How | Baseline |
|---|---|---|
| Device list, 200 devices | `go test ./internal/transport/http/handlers -run '^$' -bench Baseline -benchmem -count 5` (`BenchmarkBaseline_DeviceList`) | about 245 to 254 us, 1.48 MB and 1,824 allocations per request |
| Findings, 500 findings | `BenchmarkBaseline_Findings`, same command | about 273 to 277 us, 586 KB and 2,541 allocations per request |
| Start-up to the first answer | built binary, `--data` temporary root with `server.bind` on a spare port, polled until `/admin/api/auth/status` answered 200, five runs | 84 to 119 ms |

The two benchmarks go through the handler and its JSON encoding with the tests' fakes; the network and the sign-in check are not in them. Start-up
needs `settings.yml` at mode 0600 or the program exits at once (my first five runs measured my own timeout).

## 4c. S1 built (2026-11-07, branch `stacks-p3b-design`, PR #203)

- **SDK:** `RouteSpec` (method, path, access), `Request`, `Response`, `Stack.Handle`; the manifest refuses a bad method, an unclean path, an
  unknown access and a duplicate; `Request` and `Response` pass the plain-data and JSON round-trip tests. There is no "public" access: a route
  is never open, and an empty access means admin.
- **Core:** `MountStackRoutes` (`transport/http/stackroutes.go`) mounts each route and maps a stack's error code to the status the core's own handlers use;
  `AuthenticateWith` takes the step-up routes a stack declares (`StackStepUpRoutes`), so a stack cannot lower its access, and the real middleware
  is what the test drives. **Not wired into the service yet:** nothing in `internal/app` mounts a stack until S2 has a real one, so these three
  functions are used only by their tests today (`Authenticate` itself now calls `AuthenticateWith`).
- **Measured cost of the mount:** for a body-less GET, 19 allocations and 6,162 B per request through the mount and through a hand-written handler,
  with timings within noise (`BenchmarkStackRoute` against `BenchmarkDirectHandlerForComparison`). With a body, the mount does one `ReadAll` where a handler
  decoded the body; not measured separately.
- **Reference stack:** declares `GET /admin/api/example/echo/{text}` and a step-up `PUT /admin/api/example/echo`.

**Findings that refine the design (found by reading, not guessing):**

1. **The core refuses routes it cannot keep safe.** Authentication is a path-keyed middleware over `/admin/api/*` with a public list for the sign-in routes
   and a step-up table of exact paths that skips reads. So `MountStackRoutes` refuses a route outside `/admin/api/`, under `/admin/api/auth/`, a step-up
   read (it would protect nothing) and a step-up route with a wildcard (the table matches exact paths); a stack's step-up writes today are all exact.
   When a Network route moves, its entry leaves `stepUpExact` and the manifest declares it; `app/route_classes_test.go` must then read both sources.
2. **D8 refined: settings stay typed in the core until the last slice.** The `netaudit:` document is one struct of about 40 fields that every sub-package
   and handler reads (`AppConfig.Netaudit`, 23 uses, behind `netauditView`), so a slice cannot take only its own fields. And the store decodes
   `settings.yml` without strict fields, so a key with no field is dropped on save: making it opaque needs an inline map and is the one risky flip.
   Until the root moves (S6) the core keeps the typed field, and the SDK `Config` port is an adapter that converts to JSON and merges a saved document
   over the current one (a partial document must not clobber the other fields; its fields carry no `omitempty`). The flip happens once, at S6, with the
   round-trip test.
3. **S2 should carry the `Config` adapter**, because DNS check is its first user; building it earlier would be a port with no stack needing it today.

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
