---
name: config-persistence
description: "Add or fix persisted configuration across settings, registry, workspace overrides, API/UI mapping, runtime application, and reload. Use when changing config fields, defaults, precedence, or storage behavior."
last_reviewed: 2026-10-04
---

# Configuration and Persistence

## Establish ownership

Read Constitution III and `docs/data-layout.md`; follow the affected contract through `docs/SPECS/README.md`. Trace one existing sibling field from shared model through request validation, persistence, load/defaults, runtime application, response mapping, and frontend when exposed. Use `docs/architecture.md` → File Change Checklist for the affected change type.

- `settings.yml`: system/user configuration and model tuning overrides. `backend/internal/platform/storage/app_config_store.go` exposes System/Settings projections of **one shared AppConfig store**, not independent documents.
- `registry.json`: model/provider catalogue and dynamic infrastructure definitions; do not store agent tuning overrides here.
- Workspace `config.yaml`: per-workspace overrides under the data root's metadata tree. Runtime metadata stays outside agent working files.
- Credentials: encrypted secrets through `SecretsStore`, never ordinary config or API views.

Use resolved data paths, not CWD-derived or hardcoded locations. Initialization, reload wiring, and long-lived coordination belong in the central app layer.

## Make the full round trip explicit

For each changed field, establish its owner, serialized name, default, validation, override precedence, and when changes take effect. Distinguish absent, explicit zero/false, and reset values according to the contract; do not let omission overwrite unrelated configuration.

Use existing atomic Store/Update APIs and merged projections. Inspect failure semantics before claiming rollback or a transaction across files; atomic writes to two stores are not inherently a multi-file transaction. Preserve read-copy isolation and unrelated fields.

Trace runtime refresh and notifications as carefully as disk writes. `ApplyModelOverrides` merges tuning into runtime models; local context/output budgets obey serving limits and workload classification. Verify removal/reset does not leave stale effective values. Reuse existing subscriptions, avoid self-reload loops and cross-view notifications, and give new watchers explicit lifecycle ownership.

Check old documents and missing fields against current loading/defaulting behavior. Implement compatibility handling only when the agreed contract requires it; do not invent a migration or change public semantics silently. Secret changes must preserve credential-to-model cascades and runtime synchronization.

## Verify

Extend existing tests for save/reload/effective-value round trips, default/zero/reset behavior, precedence, rejected values, persistence failures, and unrelated-field preservation as affected. Use temporary data roots; never operator data. Test concurrent updates or notifications when their behavior changes, then run race checks for concurrency/lifecycle edits.

Run backend gates and frontend gates when API/UI mappings change. Complete documentation stewardship for config semantics and user-facing setup. Keep generic implementation and testing rules in their existing skills.
