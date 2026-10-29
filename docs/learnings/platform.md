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
- **Tried:** `TestApplyChildLimits_ASBehavior` called `ApplyChildLimits` on the test process itself (2026-10-27) → **on Linux CI the 2 GiB `RLIMIT_AS` cap starved the race detector: `ThreadSanitizer: out of memory ... TracePart`, package `platform/process` failed; passes on macOS where the cap is skipped**. **Lesson:** never lower rlimits on a test binary; test the logic through the `applyLimits` setter seam. Evidence: [rlimit_unix.go](../../backend/internal/platform/process/rlimit_unix.go), [rlimit_unix_test.go](../../backend/internal/platform/process/rlimit_unix_test.go).
- **Tried:** `setup.sh --unattended` reading the saved paths by `source`-ing `$SVC_ROOT/.setup-paths` and recovering the binary path from the unit's `ExecStart` (2026-10-28) → **the service user owns the data root, so it could rewrite a file root executes; and the unit renderer never wrote `ExecStart`, so a custom binary path silently reverted to the default**. **Lesson:** keep replayed choices in a root-owned file outside the data root, parse `KEY=value` with `sed` (never source), and render every value the next run reads back — the round-trip test is `render_unit` → `resolve_unit_settings`. Evidence: [setup.sh](../../setup.sh), [test-setup-ui.sh](../../scripts/test-setup-ui.sh), [service_setup.md](../service_setup.md).
- 2026-10-29: an Origin or CSRF check that skips GET does not protect a GET that has side effects. `GET /admin/api/providers/test` builds a provider from `api_key_name` and `base_url` in the query, and the registrar fills in the stored key, so a cross-site `<img>` could send a stored key to a caller-chosen URL (read, not yet run; plan `admin-api-authentication-design.md` E5/E6). Gate Fetch-Metadata on every method of `/admin/api`, keep secrets out of URLs, and never accept a stored key together with a caller `base_url`.
- 2026-10-29: do not print a generated secret at startup or recover a lockout through a loopback endpoint: the journal and `app-logs` carry the first, and the agent's network-on shell reaches the second. Recovery is a CLI run as the server's OS user.
