---
status: partial
date: 2026-11-13
last_reviewed: 2026-11-18
related_specs: [SPEC-006]
constitution_references: [I.5, III.2, III.6, IV.6]
evidence: Read 2026-11-13 - backend/internal/platform/db (shared SQLite provider, modernc, WAL), platform/sizewatch (cached directory sums, accounting not a quota), platform/ledger/cleaner.go, platform/persistence (JSON files written atomically), stacks/sdk/manifest.go (Manifest.Storage), the Network "Downloaded data" card (frontend DestinationDataCard.vue). No system-wide data page exists today.
---

# One page for everything the service stores: size, age, limits, deletion

Owner direction (2026-11-13): "a system related page for managing stored data, sized, deletions etc, across all service related data", prompted by the Radio stack
([`radio-stack.md`](radio-stack.md)) but meant for the whole service. **Today** there is no such page: each store has its own cleanup (the ledger cleaner, workspace
`sizewatch` accounting, the Network "Downloaded data" card, per-file pruning such as `pruneLife`) and nothing shows the total.

## What the page shows

One row per store, from every source: **name and owner** (core or a stack) · **path** · **size on disk** · **rows or records** · **oldest and newest** · **retention rule** and who set it · **budget and percent used** · **actions**
(purge older than a date, clear all, and where a store supports it, export). A total against the free space of the data volume, with a warning level. Core stores: settings and state files, secrets (size only, never content),
the ledger and memory databases, run recordings and logs, reports, workspace folders (via `sizewatch`), downloaded data tables. Stack stores come from the stacks themselves.

## How (sketch, to be settled in a design pass)

- **A stack reports through an optional SDK interface** (`Storer`: `Report()` returns plain `StoreReport` records; `Purge(request)` takes a store id and a cut-off). The core never reads a stack's files; it asks (Constitution IV.6, ADR-0035).
- **Deletion is a control-plane action** (Constitution I.5): step-up required, the request names the store and the cut-off, nothing is deleted by a model or an automation, and the page says exactly what a purge removes before it runs.
- **Limits are enforced by the owner of the store** (Radio's cap and age job, the ledger's cleaner); the page shows them and lets the operator tighten them within bounds. A global low-disk watermark raises one alert and stops optional background
  writers (Radio first), never the run path.
- Sizes are measured by a cached, bounded walk or `stat` (the `sizewatch` approach); it is accounting, labelled as such, not a quota.

## Phases

- **D1 Read-only core report.** Core stores only, one endpoint, one page. *Check:* every file the service writes under its data folder is covered by a store row, or a test fails naming it (so a new store cannot hide).
- **D2 Stack interface.** `Storer` in the SDK, the Radio stack and the Agents stack report. *Check:* a stack that lists `Storage` in its manifest but reports nothing fails the manifest test.
- **D3 Purge and limits.** Purge with step-up, adjustable bounds, the audit line for each purge. *Check:* a purge without step-up is refused; a purge removes exactly what the preview said.
- **D4 Watermark.** The low-disk alert and the optional-writer stop. *Check:* a fake full volume stops Radio's writer and leaves a run working.

## Decisions (owner, 2026-11-18: "do what's best recommended")

- **Placement:** Settings → Storage & logging, a "What is stored" card under the workspace storage settings (the tab that already holds run
  logging and the workspaces folder). No new navigation entry.
- **Export:** out of scope for now.
- **Workspace folders:** size only, never a delete on this page (people's own work).
- **Deletion:** stays in the flows that already exist (each row says where, in fixed words: Clear runtime data, a factory reset, Network →
  Settings → Data, a run or recording delete). A new purge API is D3 below.

## Built (2026-11-18, branch `docs-plans-tracker-cleanup`)

- **D1** `internal/platform/datausage` (`Measure`: every top-level entry of the data folder is a row of its kind, and an entry no kind names
  is a row under its own name, marked not described, so nothing hides; folders through the 60 s `sizewatch` cache), the core's kinds in
  `internal/app/datausage.go` (`coreStoreKinds`, test `TestCoreStoreKinds_DescribeEveryDocumentedName`), `GET /admin/api/system/storage`
  (`handlers/storage_handlers.go`), the card `frontend/src/components/settings/StorageReportCard.vue`.
- **D2** each hosted stack's `Manifest.Storage` files are rows owned by the stack, with what a `stacksdk.StorageReporter` says of them (cap,
  records, oldest, keep-for; Radio implements it).
- **D4 (part)** the data folder's volume, free and size, with the host watch's level rule (`hostwatch.DiskLevel`, now exported), shown at
  the top of the card.

## Remaining work

- **D3 purge and limits:** a purge request per store id and cut-off with step-up and an audit line, adjustable bounds, the preview of what a
  purge removes. Needs a `Purge` method on the SDK interface for stacks (an SDK change) and a design for which core stores accept a cut-off.
- **D4 rest:** one alert when the level becomes low or critical, and stopping optional background writers (Radio first). There is no way for
  the core to pause a stack today (an SDK change); the low-disk finding of the host watch already exists in the nightly report.
- Owner check on vertex: open Settings → Storage & logging and read the rows; any "Not described" row is a store to add to `coreStoreKinds`.
