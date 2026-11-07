---
subsystem: network-audit
tags: [stacks, coupling, inventory, p2]
result: reference
status: complete
date: 2026-11-06
---

# Network stack: what `core/netaudit` shares with the rest of the program

Evidence for plan [`stacks-as-packages-and-network-stack.md`](../PLANS/cross-cutting/stacks-as-packages-and-network-stack.md) P2 (the
"coupling inventory"). Measured on 2026-11-06 at `c218fa09` with `go list` and `grep` over `backend/internal/core/netaudit/...` (the
package and its 25-odd sub-packages). Not instructions: it says what P3 must turn into ports.

## 1. Imports from outside `netaudit` (by package, not by file)

| Imported | Importing packages | What it is used for | Becomes |
|---|---|---|---|
| `models` | 11 | persisted shapes (`DeviceInventory` 122 uses, `ScanBaseline` 98, `NetworkDevice` 69, `NetauditConfig` 61, `VulnIndex` 48, `ScanAck` 47, `NetworkDay` 40, `DeepCheck`, `UbuntuIndex`, `Expectation`, `FixRecord`, `ScanFinding`, ...) | types owned by the stack; same persisted keys and file formats (D3) |
| `platform/progexec` | 4 (`netaudit`, `nmap`, `scantool`, `wifilook`) | running `nmap` and `nmcli` | `stacksdk.Isolation` |
| `core/tools` | 3 (`dnscheck`, `nmap`, `outsidelookup`) | `NetworkTools` (guarded HTTP), `ErrNetworkDisabled`, `ErrInternetBlocked`, `StatusError`, `RedactSecrets` | `stacksdk.Guard` plus an SDK error code; `RedactSecrets` is a function the stack needs a copy of or the SDK must offer |
| `platform/scanscope` | 2 | the declared scan networks | a value read through `stacksdk.Config` (D2); policy stays in the core |
| `platform/hostwatch`, `platform/mcast`, `platform/neighbors`, `platform/loginlog` | 1 each | this computer's ports, files, packages; multicast; the neighbour table; btmp | move into the stack (only Network uses them) |
| `platform/sandbox` | 1 (`netaudit`) | reading what the jail reports | `Isolation` / a status call |
| `assistant/prompts` | 1 (`scantool`) | the Security auditor's prompt fragment | a fragment the manifest or the tool supplies (Constitution III.13 still applies) |
| `core/toolregistry` | 1 (`scantool`) | registering the `network_scan` tool | the manifest's `Tools` |

The reverse direction: 26 non-test files outside `netaudit` import it (`internal/app/netaudit*.go`, `stacks*.go`, `bootstrap.go`, the
17 `transport/http/handlers/netaudit_*_handlers.go`, `models/netaudit_config.go` in comments only, `tools/gen-oui`). The handlers are
the largest group: they reach into `netaudit` types directly, so P3b needs a request/response port per handler group or the handlers
move with the stack's routes.

## 2. Shared state: stores, events, settings

- **Storage.** `netaudit` already depends on small store interfaces, not on `persistence` types: `FixStore`, `HostStore`, `PortStore`,
  `DNSStore`, `AlertStore`, `BaselineStore`, `ExpectationStore`, `HistoryStore`, `DeepStore`, `InventoryStore`, `AckStore`, `DecoyReader`,
  and the feeds' `Store`. The adapters are `*persistence.WorkspaceManager`, `HostWatchStore`, `DecoyTouchStore`, `VulnIndexStore`,
  `UbuntuIndexStore` in the core. Two ways to satisfy "a stack owns its storage" (IV.6): the stack keeps these files under its own
  `DataDir` (a migration: the files stay where they are and the stack is handed that path), or the core keeps owning them behind the
  existing interfaces. **Open decision for P3b.** Several stores are per-workspace (`WorkspaceManager`), which the SDK's single
  `DataDir` does not model.
- **Events.** `netaudit` does not use the event bus directly (no `eventbus` or `Subscribe` in the package). It reports through
  `AlertLog` (an `AlertStore` in the core) and the findings it returns. `stacksdk.Events` therefore has little to carry for Network
  today; it exists for the next stack.
- **Settings.** `models.NetauditConfig` (61 uses) is read through the core's settings store; the `netaudit:` keys must be unchanged when
  the type moves (D3). The `Switch` adapters in `internal/app/stacks.go` already put each module's setting behind an interface.

## 3. Findings P3 has to settle before moving code

1. **Inbound sockets have no SDK port.** `decoy` (`net.Listen`, Constitution I.6), `routerlog` (`net.ListenUDP`, I.8), `platform/mcast`
   (`ListenUDP`, I.9) open listening sockets. The boundary scan (`internal/archtest`) forbids `net.Listen*` in a stack, correctly, so
   P3 must add a `Listen` port to the SDK (with the same off-by-default and step-up rules as today) before these move. The three can
   stay in the core until then.
2. **The Wi-Fi look needs a Unix-socket relaxation.** `wifilook.go:194` sets `AllowUnixSockets: true` for `nmcli`. `stacksdk.RunRequest`
   deliberately has no such field (ADR-0033). The core's adapter must hold a fixed table keyed by program name; `nmcli` is the only
   entry. Decide in P3.
3. **`scantool` registers an agent tool and a prompt fragment**, which `core/toolregistry` and `assistant/prompts` own. The manifest
   declares the tool as data; the handler is called by name with JSON. The prompt fragment is not yet part of the manifest.
4. **`RedactSecrets` and the guard's error values** (`ErrNetworkDisabled`, `ErrInternetBlocked`) are used by the stack's clients. They
   become SDK error codes (`denied`, `unavailable`); the redaction function needs an SDK home or a copy.
5. **Per-workspace stores** (see §2) do not fit a single data directory.

Moving order that follows from this (for P3b, unchanged from the plan): leaf sub-packages with no core imports (`oui`, `dnswire`,
`names`, `upnp`, `vulnmatch`, `ubuntuoval`) first; then those that need only `Guard` and `models` types (`dnscheck`, `outsidelookup`,
`webprobe`, `portprobe`); then `nmap`, `scantool`, `wifilook` (Isolation); the listening ones last.
