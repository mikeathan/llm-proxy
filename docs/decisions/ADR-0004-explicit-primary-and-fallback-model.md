---
id: ADR-0004
title: No model auto-bootstrap; explicit primary and fallback
status: accepted
date: 2026-08-14
subsystems: [orchestrator, admin-ui]
supersedes:
superseded_by:
---

# ADR-0004: No model auto-bootstrap; explicit primary and fallback

_Recorded 2026-10-26 from the plan and evidence below; the decision date is when it was taken._

## Context
Saving provider credentials silently created a default model and set it as primary, and an empty primary fell back to the first catalogue entry. Users got a model they never chose, and references could dangle after a model was removed.

## Decision
We will never pick a model for the user. Primary is used if set, else fallback, else the request fails with a clear message. A primary or fallback that points at a removed model is cleared; one that does not exist cannot be set. A persistent banner says when no primary (critical) or only a fallback (notice) is configured.

## Consequences
First-run needs an explicit choice, surfaced by the banner. No silent `Catalogue[0]` behavior to debug.

## Alternatives considered
- **Auto-create a default on first credentials** — the removed behavior.
- **Fall back to any available model** — surprising cost and quality changes.

## Links
- [`ARCHIVE/cross-cutting/primary-model-warning-banner.md`](../PLANS/ARCHIVE/cross-cutting/primary-model-warning-banner.md)
