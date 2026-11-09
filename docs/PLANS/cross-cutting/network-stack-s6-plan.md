---
status: partial
date: 2026-11-09
last_reviewed: 2026-11-09
related_specs: [SPEC-003, SPEC-006]
constitution_references: [IV.6]
evidence: branch network-small-items (local, uncommitted, v0.63.0); docs/decisions/ADR-0037 (accepted); docs/decisions/ADR-0036; docs/PLANS/cross-cutting/network-stack-p3b-design.md sections 4 and 4e; counts taken from backend/internal/core/netaudit on 2026-11-09 (go list, grep)
---

# S6: what is left of moving `core/netaudit` into the Network stack (2026-11-09)

Slice S6 of `network-stack-p3b-design.md` was one line: "the root `netaudit`, `decoy`, then delete `migrationDebt`". Reading the code shows it is
not one slice, and that its stated goal (an empty debt list) is already met. This plan says what was built, what the numbers are, and what the owner
must decide before the rest is worth starting.

## Built on 2026-11-09 (v0.63.0)

- **`migrationDebt` is gone.** Its one entry was `core/netaudit -> network/oui`, for two calls of `oui.Vendor` (`devicesview.go`, `deviceclass.go`).
  The core now defines `netaudit.VendorLookup` (`func(mac string) string`, nil-safe) and the composition root gives it the registry
  (`app/stacks.go`, `cardMakerOf = oui.Vendor`): `ViewOf` and `ClassMismatchFindings` take it, `ClassChecks` has a `Vendors` field, the devices
  handler has `WithVendors`, the Nearby view has `vendors`. The core tests no longer depend on the stack's data; `TestCardMakerOf_IsTheNetworkStacksRegistry`
  proves the wiring against the real registry. The shrink-only list, its helper and its test are deleted from `archtest/boundary_test.go`; the rule is
  now flat: nothing under `backend/internal/` imports a stack except `internal/app/stacks.go`. **Constitution IV.6 lost its transitional clause** about the
  list (a wording change for the owner to approve by merging; no rule was weakened).
- **ADR-0037 option C:** `vulnindex`, `vulnmatch`, `vulnfeed`, `ubuntuoval` moved to `internal/core/vulndata/` (no code change).
- The privileged-helper small items (D9, the lying-resolver test, OS-guess demotion) are in the same branch; see the handoff.

## What is left, measured (2026-11-09)

| Fact | Value |
|---|---|
| Root `core/netaudit` non-test files / lines | 77 files, 13,703 lines |
| Sub-packages still under `core/netaudit/` | 16: `decoy dnswire framewire knownbad names nearby nmap outsidelookup phonehome portprobe routerlog scantool udpchecks upnp webprobe wifilook` |
| Service packages that import the root | 21 non-test files: 11 in `internal/app`, 10 in `transport/http/handlers`; 38 handler files are named `netaudit*` |
| `app/netaudit.go` (the wiring) | 915 lines, plus 7 more non-test `app/netaudit_*.go` files |
| Root files that use `models` | 100 of them; most used types: `DeviceInventory` (130 uses), `ScanBaseline` (104), `NetworkDevice` (96), `NetauditConfig` (56), `ScanAck` (43), `NetworkDay` (40) |
| Platform packages the root imports | `platform/hostwatch` (hostwatch, hostprivilege, firewallguide, patchfindings), `platform/scanscope` (scan, scope), `platform/isolation` (readiness) |
| Sub-packages with a core or platform dependency | `nmap`, `scantool`, `wifilook` (`platform/isolation`; `scantool` also `platform/scanscope` and `core/toolregistry`), `nearby` (`platform/mcast`), `outsidelookup` (`core/tools`), `framewire` (`platform/privhelper/wire`), `portprobe` (the root); the rest (`decoy dnswire knownbad names phonehome routerlog udpchecks upnp webprobe`) import nothing from the service |

## Why "move the root" is not a slice

1. **The shared types.** A stack cannot import `models`. The root's inventory, baseline, ack, fix, history and alert types are also read by the core's
   report, alerts, bell, UI handlers and automation. Moving the root means either moving those types into the stack (and giving the core plain-data
   views of them), or keeping them in the core and the stack depending on a port that serves them. Neither exists.
2. **Missing ports.** Moving the root's host watching needs a port for `platform/hostwatch`; scanning needs one for `platform/scanscope`'s policy
   (whether the SDK `Guard` covers it is not checked); the feeds need ADR-0037's lookup port; the ingest-fed leaves need S4's ingest port.
   Each must pass the P3b section 6 test: it serves a second stack in principle, or it stays in the core.
3. **Settings.** `settings.yml`'s `netaudit:` document is one struct of about 40 fields read by every sub-package; making it opaque is "the one risky
   flip" (P3b section 4d, finding 2) and needs a round-trip test on a real file.
4. **Cost to the operator is nil.** Nothing the operator sees would change. The reason to move is architecture only, so the bar is: it must not make
   anything slower, heavier or less safe, and each step must be a no-behaviour-change PR.

## Decision (owner chose B on 2026-11-09; recorded in [ADR-0038](../../decisions/ADR-0038-the-network-stack-owns-the-sensors-and-the-core-keeps-the-judgement.md)): what does the Network stack own at the end?

- **A. The whole audit engine** (collection, scan, inventory, baseline, report, history, ack, fixes, alerts). About 13,700 lines plus the sub-packages;
  needs the `models` types moved or ported, 4 to 6 new ports, and the settings flip. The core would keep delivery (bell, connectors), the agent tool
  and hosting.
- **B. Sensors in the stack, judgement in the core (recommended).** The stack owns what is Network-specific and port-hungry: the programs (`nmap`,
  `portprobe`, `scantool`, `wifilook`), the ingest-fed watchers (`routerlog`, `phonehome`, `knownbad`), `upnp`, `webprobe`, `udpchecks`, `nearby`,
  `decoy`, the DNS check (done). They offer **findings by id** through the mechanism ADR-0036 already built (`Stack.Findings`, core-worded). The core
  keeps the report, baseline, history, ack, fixes, alert delivery and the `models` types, which the rest of the product already reads. Each sensor moves
  when its caller in the root is changed to pull its findings, one PR per sensor, with no `models` type crossing a port.
- **C. Stop here.** The debt list is empty; leave `core/netaudit` as the core's audit module and only add stacks for new modules (Agents, new checks).
  Cheapest, and consistent with the stack architecture, since the rule that matters (the core never imports a stack) holds.

**Why B over A:** the types the root shares with the rest of the product are the expensive part of A, and ADR-0036 already gives sensors a way to
report without them. **Why not C yet:** the owner set the aim of Network as a stack; B delivers it in small safe steps and C stays available after
any step. If the owner wants C, this plan is done.

## Order under B (each its own PR; stop at any line)

1. **ADR-0038** (the decision above) and the findings-by-id contract for sensors that today return `Finding` values with device and port data (ADR-0036
   carries only id, subject address, count, names and since; a sensor that needs more adds a typed field there, with its wording table in the core).
2. **The lookup port** (ADR-0037), with its first caller: the flaws/known-vulns matching, optional in the manifest.
3. **Pure leaves with no root caller left to convert, one at a time:** `udpchecks` (findings already built in the core: `udpfindings.go`), `webprobe`
   (9 symbols used by `webfindings.go`, `tlsfindings.go`), `upnp` (carried by `exposure.go`, `router.go`), `decoy`. For each: add its findings to
   `Stack.Findings`, delete the root file that wrapped it, keep the wording table in the core.
4. **The ingest port (S4)**, order fixed 2026-11-09 after ADR-0039: (a) `decoy` first (an observation is a source address, a port and a time, no text); (b) `routerlog` counters and the DNS-query leg; (c) `phonehome` only after its alert `Told` record is split from its names; (d) `knownbad` last, with the ADR-0037 feed; (e) `nearby` through the exchange port. The core keeps every socket and the syslog parsers.
5. **Programs (S5):** `nmap`, `scantool` and `wifilook` stay in the core (owner, 2026-11-09, ADR-0039 option b); `portprobe` is undecided.
6. **Settings flip and the Modules screen definitions** move last, with the round-trip test; only needed if every module has moved.

## Not measured, needed before step 3 and later

- Whether the SDK `Guard` port covers `platform/scanscope`'s declared-range check (read `scan.go`, `scope.go` and `stackport` first).
- Per-step cost: re-run the baseline figures of P3b section 4b after each PR.
- On vertex: nothing needed for steps 1 to 3; step 4 changes who owns the multicast socket, which needs a real run.

## Remaining Work

**Update 2026-11-09 (branch `network-ingest-exchange-ports`):** ADR-0039 accepted; the order of step 4 and the decoy blast radius (twelve core files read `models.DecoyTouch`) are in the handoff. No code yet.

**Decided:** B. ADR-0038 is accepted. Built so far on the branch: the ADR, and `TestUDPChecks_TheFindingsOfAnAnswerAreByteIdentical`, which pins the
nightly UDP findings so a later move can be held to them.

**Built last (2026-11-09, v0.66.0): `upnp`** with `LookRouter` (ADR-0038, "Third sensor"); the shape was one method for the probe, the forwarded ports and the internet address. `decoy` was not on the list that mattered here and is a listener, so it goes with the ingest port.

**Built after that (2026-11-09, v0.65.0): `webprobe`** with its own typed methods; see ADR-0038. Behaviour differences to know: it now honours the Guardrails local-network switch and no longer asks for compressed answers (SPEC-003 2.53).

**Built next (2026-11-09, v0.64.0): step 1, the scoped dial, the sensor method and `udpchecks`** (ADR-0038, "Built 2026-11-09"). The gap found first (the stack's
`Guard` did not apply the declared scan scope) is closed by the scoped dial, with a test that an address outside the scope is never dialled and that fails when
the scope check is removed.

**Next, in order (each its own PR; the first two are built together with the first sensor, because neither has a caller without it):**

1. ~~**Scoped dial + input-bearing sensor method + `udpchecks` move.**~~ **Done 2026-11-09.** Plan as written: SDK: a scoped dial port (declared scope, then the guarded dial, refusal as `CodeDenied`)
   and an optional `Look`-style method with plain-data input and typed answers (rule 3 of ADR-0038); stack: `udpchecks` and its tests moved unchanged;
   core: `UDPChecks` keeps the carry rule, the replace-a-helper-finding rule and the typed switch, and builds today's `Finding` from the typed answer. Gates: the
   parity test written first, a test that an address outside the declared scope is refused through the stack path, `StackFinding` (or the new answer) gets a
   typed port field, both boundary tests, complexity, the harness check.
2. ~~`webprobe`~~ **done 2026-11-09 (v0.65.0, ADR-0038 "Second sensor")**; `upnp` **done 2026-11-09 (v0.66.0, ADR-0038 "Third sensor")**. Original wording: `webprobe` and `upnp` on the same contract. The shape check is in ADR-0038: `webprobe` needs richer typed facts than `Pair`, so decide that from `webfindings.go` and `tlsfindings.go` first; `upnp` is carried by `exposure.go` and `router.go`, so start with the one whose root caller is smaller.
3. The ingest port (S4) for `routerlog`, `phonehome`, `knownbad`, `nearby`.
4. Programs (S5): `nmap`, `portprobe`, `scantool`, `wifilook`.
5. The ADR-0037 lookup port, with the first matching code that needs it; then the settings flip and the Modules definitions, last.

Delete this file when the work is done (harvest into the P3b design and the ledger first).
