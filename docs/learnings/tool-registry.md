---
subsystem: tool-registry
last_updated: 2026-10-26
---

# Learnings — Tool catalog, manifests and registration

What was tried and what happened. Read before proposing an approach in this area; add a line when a run,
eval, audit or merged phase teaches something (newest last within each section). Format and rules:
[`README.md`](README.md).

## Dead ends — do not retry without new evidence

_None recorded yet._

## Root causes and fixes that held

- **Tried:** A trailing comma in `manifests/*.json` → **Go's strict unmarshal dropped the tool with only a warning; the model picked the nearest tool instead**. **Lesson:** validate manifests after edits. Evidence: [memory-injection-investigation.md](../audits/memory-injection-investigation.md).
- **Tried:** Tool categories defined in three places → **drift (`edit_file_block` missing from one)**. **Lesson:** one tool catalog (`models/tools.go`) and a `toolregistry` package. Evidence: [2026-10-22-architecture-review.md](../audits/2026-10-22-architecture-review.md), [network-audit.md](../PLANS/cross-cutting/network-audit.md).
