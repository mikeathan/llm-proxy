---
id: ADR-0035
title: A stack reaches the service only through named ports and keeps only the files it lists
status: proposed
date: 2026-11-08
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0035: A stack reaches the service only through named ports and keeps only the files it lists

Proposed: it is in force when the owner accepts it. Carries out [ADR-0032](ADR-0032-a-stack-is-a-package-with-a-manifest-kept-apart-by-tests.md)
for the first real vertical (the router DNS check moved into `stacks/network`, plan P3b slice S2).

## Context
A stack imports only the SDK, so everything the DNS check used from the service had to become a port or stay behind. Reading the code (2026-11-08)
showed what it used: the guarded HTTPS GET with its boundary, redirect, size and status checks; the guarded dial to the router; the typed
settings document; the neighbour table's default gateway; the service's log; one file in the data folder; the factory reset; and the monitor's
view of its last cycle.

## Decision
- **Named ports, plain data, fixed words.** `Fetch` (one guarded GET: a URL, an accept type and a size cap; no method, header or client field, pinned by
  a test), `Gateway` (the default gateway as text, or not found), `Config` (the settings as JSON), `DataDir`, `Guard`, `Log`. An adapter turns a
  refusal into a code (`denied`, `unavailable` when switched off, `timeout`, `invalid`, `internal`) with a fixed sentence: a stack never sees the guard's
  wording, which can name a host or an address. `tools.ErrGuardRefused` lets the adapter tell a refusal from "switched off" without changing any message.
- **`Config.Save` merges and never decodes into the stored value.** The document is overlaid on the current settings and decoded into a fresh value.
  Decoding into a shallow copy rewrote slices the collector was reading (found by the race detector); a regression test holds it.
- **`DataDir` is a named-file interface, not a path.** `Read`, `Write` (atomic, the core's own user-content permissions) and `Quarantine` reach only the
  files in `Manifest.Storage`; any other name is `denied`. A factory reset removes exactly those files. No core source may name a file a manifest claims
  (a test searches for it), and two manifests may not claim one.
- **`Stack.Status` reports each module's last cycle** as data (`ModuleStatus`), for the stalled-monitor view; the registry's module is checked against the
  manifest's spec by a test so the two cannot drift.
- **Routes are mounted on the core's `Router`** (so they are in the route list the classification tests read), declared in the manifest, and the
  step-up write comes from the manifest, not from the core table.
- **`outsidelookup` stays in the core.** The root `netaudit` still uses its client, which reads the core's `tools` types; moving it would force a rewrite
  of a feature that has its own slice, and the stack needs only an https URL check (copied, eight lines), so no new `migrationDebt` pair is added.
- **No caching of settings.** The settings are read when needed (the collector's loop polls every 30 seconds when off), so `ConfigChanged` has nothing to do
  and no save path needs to notify the stack.

## Consequences
Every later vertical reuses these ports; the next one that needs something not here adds a port with a reason, as `Gateway` was added. The stack's tests
run with fake ports and the module builds alone with the workspace off. The cost: a JSON round trip per settings read (a few times per cycle) and a
`Status` call on the monitor's tick; the section 4b figures were re-measured and are recorded in the plan.

**Known limit, until S6:** the `Config` port shows the whole `netaudit` document and a `Save` may name any field in it. The DNS check names only its
five (`dnscheck/settings.go`, keys listed there), but the SDK does not yet stop a stack naming others; the opaque, schema-checked document at S6 closes it.

## Alternatives considered
- **The stack reads the routing table itself** — IV.6 keeps host reads in the core; one port is smaller than a second reader.
- **`DataDir` as a path** — a stack could write anywhere the service can; the named files make ownership data a test can read.
- **Cache the settings and call `ConfigChanged`** — needs every save path to notify, for no measurable gain.
- **Move `outsidelookup` now** — see above.

## Links
- [`../PLANS/cross-cutting/network-stack-p3b-design.md`](../PLANS/cross-cutting/network-stack-p3b-design.md) section 4d and its progress notes.
- `backend/internal/core/stackport/`, `stacks/sdk/ports.go`, `stacks/network/`.
