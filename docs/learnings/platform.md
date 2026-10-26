---
subsystem: platform
last_updated: 2026-10-26
---

# Learnings — Storage, config, events, logging and backend performance

What was tried and what happened. Read before proposing an approach in this area; add a line when a run,
eval, audit or merged phase teaches something (newest last within each section). Format and rules:
[`README.md`](README.md).

## Dead ends — do not retry without new evidence

- **Tried:** Throttling session checkpoints → **rejected: the reload contract says "unconditional" and a test pins it**. **Lesson:** change the contract and the test together, with a measurement. Evidence: [2026-08-28-ops-performance-review.md](../audits/2026-08-28-ops-performance-review.md), [backend-hot-paths-and-leak-hardening.md](../PLANS/cross-cutting/backend-hot-paths-and-leak-hardening.md).
- **Tried:** Incrementalising `FilterStreamingMarkup` / the repetition guard → **deferred**. **Lesson:** chunk-boundary correctness needs a property/fuzz test first. Evidence: [backend-hot-paths-and-leak-hardening.md](../PLANS/cross-cutting/backend-hot-paths-and-leak-hardening.md).

## Root causes and fixes that held

- **Tried:** Bounding the event-bus replay buffer by count only → **tens of MB after a heavy run**. **Lesson:** bound by bytes too (4 MiB/channel). Evidence: [2026-08-28-ops-performance-review.md](../audits/2026-08-28-ops-performance-review.md).
- **Tried:** Static audits (~100 findings) → **most fixed, none proven at runtime**. **Lesson:** add runtime gauges and a soak test. Evidence: [backend-hot-paths-and-leak-hardening.md](../PLANS/cross-cutting/backend-hot-paths-and-leak-hardening.md), [backend-audit-report.md](../audits/backend-audit-report.md), [codebase-audit-report.md](../audits/codebase-audit-report.md).
- **Tried:** Rebuilding the metrics service with only the GPU block → **the background sampler never ran; every `/metrics` call spawned a sampler process**. **Lesson:** pass the full metrics config; sample on a timer. Evidence: [gpu-performance-audit.md](../audits/gpu-performance-audit.md).
- **Tried:** `go test ./...` as root in a container without IPv6 (2026-10-26) → **4 tests in `core/egress`, `platform/paths` and `platform/storage` fail, identically on an untouched `HEAD`**. **Lesson:** read-only-directory tests cannot fail for root and the `[::1]` egress case needs IPv6 — run them as a normal user (CI does) before treating them as regressions. Evidence: [paths_test.go](../../backend/internal/platform/paths/paths_test.go), [atomic_test.go](../../backend/internal/platform/storage/atomic_test.go), [egress_test.go](../../backend/internal/core/egress/egress_test.go).
