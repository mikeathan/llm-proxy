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
| S2 | First real vertical: **DNS check** with `outsidelookup` (section 4d: a `Fetch` port, the `Config` adapter, module status, reset and lifecycle, stack-owned types and file, routes), one PR | fetch, config, status and routes on real code |
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

**Re-recorded 2026-11-08** on the same machine, at `origin/main` 250d9f15 plus the A and B changes of branch `network-flaky-test-isolation-switch-dns-s2` and before any S2 code (the device list grew since 2026-11-07: new fields on `NetworkDevice`), `-count 5`: device list about 264 to 268 us, 1.51 MB and 2,224 to 2,233 allocations per request; findings about 275 to 278 us, 586 KB and 2,541 allocations per request. S2 is measured against these, not the rows above.

**S2 measured 2026-11-08** (same machine, after the DNS check moved into `stacks/network`, `-count 5`): device list about 269 to 275 us, 1.51 MB and 2,224 allocations (2,234 once); findings about 276 to 278 us, 587 KB and 2,541 allocations. Allocations and bytes are identical to the re-recorded baseline; the device list is about 2% slower in time, within the run-to-run spread of that baseline (264 to 268 us), and the DNS check is not on that path. Start-up to the first answer, five runs with the same method: four runs at 64 to 69 ms; the first run did not answer in 7.8 s, which was my script (a first start on a fresh data folder that had to write its settings before the bind line could be set), not the service. The condition of section 4a holds: no more allocations, no goroutine beyond the collector's own (the host starts the stack and stops it with `context.AfterFunc`, so it holds none), and code removed from `internal/` in the same change.

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

## 4d. S2 read before building (2026-11-07): DNS check is a bigger first slice than assumed

Read: `core/netaudit/dnscheck` (2 files, 265 lines), `outsidelookup` (3 files, about 160), `dnscheckconfig.go` (156), `dnscollector.go` (261),
the handler (94), `persistence/networkdns.go`, and the wiring in `app/netaudit_alerts.go`, `netaudit.go`, `netaudit_monitor.go`, `stacks.go`, `routes.go`.
It is still the smallest real vertical, but it needs six things the SDK and the core do not have yet, so S2 is one PR with these parts, in this order:

| Part | Why DNS check needs it | Shape |
|---|---|---|
| 1. `Fetch` port | `dnscheck`'s outside resolver and `outsidelookup` make guarded HTTPS GETs through `NetworkTools.Open*`, which checks the internet boundary, redirects, size and status; the SDK `Guard` offers only `DialContext`, which would make the stack re-implement those checks | plain data: `FetchRequest{URL, Accept, MaxBytes}` to `FetchResponse{Status, Body}`; the core adapter calls `NetworkTools` and reads the body once, as the callers do today; refusals become `denied` and `unavailable` codes |
| 2. `Config` adapter | the switch, interval, names, server and resolver address are fields of the typed `models.NetauditConfig` (D8 refinement 2) | the core converts to JSON and merges a saved document over the current one; the stack's fields carry no `omitempty` |
| 3. Module status | the stalled-monitor view reads `DNSCollector.Status()` and the settings (`dnsReading`); the registry reads the switch and "ready" | a stack call returning plain data per module: enabled, last run, problem |
| 4. Reset and lifecycle | the log is registered for factory reset (`withReset`) and the collector's loop starts with the app | a stack call to delete its stored data; `Start(ctx, host)` runs the collector under the app's context |
| 5. Stack-owned types and file | `models.DNSComparison` and its constants, `DNSSettings`, `DNSStatus`, `DNSSummary` leave `models` and the root; the log is one file in the data folder (`network-dns-checks.json`) | same JSON tags, same file name and place (D7: paths unchanged), so an existing log reads unchanged |
| 6. Routes | `GET` and `PUT /admin/api/netaudit/dns-check`; the PUT is already in the step-up table | declared in the manifest (the PUT as `step_up`); its table entry moves to the manifest and `route_classes_test.go` reads both |

**What stays behind, and the new debt.** `outsidelookup` moves with `dnscheck` (it supplies an error value and a URL validator to it), but the root
`netaudit` (`exposure.go`, `outsidelookupconfig.go`) still uses it, so `migrationDebt` gains one pair (`core/netaudit` to `network/outsidelookup`).
The one `app` file that builds the `outsidelookup` client moves into `app/stacks.go`, since only the composition root may import a stack. The outside
lookup's own handler, settings and exposure view stay for their own slice.

**S2 progress (2026-11-08, branch `network-flaky-test-isolation-switch-dns-s2`, uncommitted).** Part 1 is built and tested: `PortFetch`, `FetchRequest`, `FetchResponse` (no method, header or client field; a test pins the field list) and `Host.Fetch` in the SDK; `Manifest.Storage` with its plain-file-name and no-duplicate checks (the cross-stack and core-store check waits for the registry); the core adapter `internal/core/stackport.Fetch` over `OpenAccepting` with one body read, a 1 MiB ceiling and fixed sentences per code; and `tools.ErrGuardRefused` so a guard refusal is told apart from "switched off" and from a failed request without changing any message. **Fetch codes:** `unavailable` = network or internet switched off (the DNS check stops quietly, as `ErrInternetOff` does today), `denied` = the guard refused, `timeout`, `invalid`, and `internal` for anything else including a non-200 answer. **Part 2 built (same day):** `stackport.Config[T]` (generic over the typed settings document, so the Agents stack can reuse it; Load shows every field, Save merges and a document that does not fit saves nothing, with fixed sentences) and `internal/app/stack_config_roundtrip_test.go`: a `settings.yml` with **every** `NetauditConfig` field set (filled by reflection, so a field added later is covered and an unsupported kind fails the test) loads and saves through the adapter byte for byte, and a partial document changes only what it names. The SDK's `Config.Save` comment now says "merges".

**Gap found while reading the collector for part 3 (not in the table above):** the stack needs more from the core than parts 1 to 6 list. (a) The router is asked at the operator's DNS server **or the default gateway**, and the gateway comes from the core's neighbour reader (`cachedGateway`, `app/netaudit_alerts.go:163`): the stack has no way to learn it, so a small port is needed (proposal: `Gateway.Default(ctx) (string, error)`, plain data, answered from the same cached reader; any other design makes the stack read the routing table itself, which Constitution IV.6 keeps in the core). (b) `Guard.DialContext` and `Boundary` have **no core adapter yet** (`NetworkTools.DialContext()` is the dial function; `Boundary` has no implementation), as have `Log` (the collector's `OnError`) and `DataDir`. (c) Nothing in `internal/app` hosts a stack yet: it must build the `Host`, call `Start` under the app context, mount `Manifest.Routes` through `MountStackRoutes` and `StackStepUpRoutes`, call `ConfigChanged` after a settings save, and read module status. Each is small, but together they are the real work of S2 and the plan's six-part table understated them. The owner decides (a), since it is a new SDK port.

**S2 built (2026-11-08, v0.58.0, same branch).** Parts 3 to 6 and the gap items are done; nothing is left of S2 except what is listed below as left on purpose.

- **The stack:** `stacks/network` (its `go.mod` now requires the SDK; it builds alone with the workspace off) with `dnscheck/` (Router over `Guard`, Outside over `Fetch`, the collector, the settings over `Config`, the log over `DataDir`) and `network.go` (manifest, `Start`/`Stop`, `Status`, `Handle` for `GET` and the step-up `PUT /admin/api/netaudit/dns-check`). The JSON of the route and the log file (`network-dns-checks.json`, same name and place) are unchanged, so the frontend did not change.
- **The core:** `core/stackport` (`Fetch`, `Config[T]`, `Guard`, `Log`, `DataDir`, `Gateway`), `app/stackhost.go` (checks the manifest and that the host gives exactly the ports it requires, starts the stack, stops it with the context, reads a module's status, refuses a file two stacks claim), `app/stacks.go` (the only file that imports the stack; `newNetworkHost`, the factory-reset removal of the manifest's files), the routes mounted on the `Router` (`MountStackRoutes` now takes a `Registrar`) and the step-up write from the manifest (`AuthenticateWith`), and the monitor reading `ModuleStatus`.
- **Deleted from `internal/`:** `core/netaudit` `dnscheck/`, `dnscollector.go`, `dnscheckconfig.go`, `models.DNSComparison`, `persistence/networkdns.go`, the DNS check handler and its table entry in `stepUpExact`. **Added to the SDK beyond the table above:** `Gateway`, `Stack.Status`/`ModuleStatus`, and `DataDir` as named-file calls (ADR-0035).
- **Left on purpose:** `outsidelookup` stays in the core (no new `migrationDebt` pair; see ADR-0035); the registry's Network modules are still defined in `app/stacks.go` (a test pins the DNS module to the manifest's spec); the settings stay typed in the core until S6.
- **Bug found by the race detector on the way:** `Config.Save` decoded into a shallow copy of the stored settings and rewrote slices the collector was reading; fixed by decoding into a fresh value, with a regression test (ADR-0035).
- **Next:** S3 (section 4), starting from the pure data leaves. Read ADR-0035 first; reuse the ports, and add one only with a reason.

**D10 decided (owner: "do what is best and clean", 2026-11-07): the stack gets the data folder, and its manifest declares the files it owns.**
D7 says paths stay as they are, and the DNS log sits directly in the application's data folder. A subfolder with a one-time rename was rejected: it is a
migration with a failure mode, and Network has dozens of stores (baselines, inventory, history and more), each of which would need it. Instead:
`Manifest.Storage` lists, by file name, what the stack owns (S2 part 5 adds the field; for DNS check, `network-dns-checks.json`). The core
validates it: a name is a plain file name (no separator), no two stacks claim one, and no core store uses a name a stack claims; a test pins it. The
ownership is then data a test can read, not a convention, and the list says exactly what to carry along when a stack is extracted to its own repository.
A later move into a per-stack subfolder stays possible, because every file is named.

**Cost expectation.** Nothing on this path is hot: the collector runs every 15 to 360 minutes, and the page reads one small summary. The only per-call
additions are the `Fetch` adapter (one body read, as today) and the route mount measured in 4c. The baselines in 4b do not touch DNS, so they are
re-run to show nothing else moved.

**Where S2 stops being safe to push in a series:** part 1 to 4 add SDK and core code with no user until part 5 and 6 land, so they ship together in one PR,
with the move deleting the old code in the same change.

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
