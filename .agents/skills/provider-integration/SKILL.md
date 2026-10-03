---
name: provider-integration
description: "Add or fix LLM provider integration: manifests, credential/endpoint resolution, request and streaming compatibility, capabilities, and failover. Use for inference providers, not search providers or communication connectors."
last_reviewed: 2026-10-04
---

# LLM Provider Integration

## Trace before changing

Follow the affected path through `backend/internal/core/llm/providers/`, runtime manager, and `backend/internal/core/proxy/`. Start with an existing provider of the same archetype; reuse its factory and manifest mechanism before adding an adapter. Registration alone does not establish request/response compatibility.

Read Constitution I (provider transport carve-out), II.5 (native/XML tools), and the relevant contracts routed by `docs/SPECS/README.md`: agent loop, parser, and orchestrator as affected. Use the existing Go rules, engineering checklists, and TDD workflow rather than duplicating them.

## Preserve the boundaries

- Registrar owns credential hydration and effective endpoint resolution. Preserve per-credential overrides and fail-closed errors for unknown providers or unresolved explicit keys. Never send masked credentials or expose keys in logs, errors, recordings, or user-visible URLs; inspect redaction where the provider requires query-parameter authentication.
- Provider infrastructure uses the injected `HTTPDoer` and shared pooled transport (45s timeout; 5s child contexts for local probes). Agent tools retain `NetworkTools` guardrails. Reuse these paths; do not introduce bare clients or move traffic between classes.
- Trace actual request serialization and response parsing: model IDs, authentication, limits, reasoning, tool schemas, tool-result messages, and finish reasons. Keep native/XML selection and explicit overrides intact; normalize provider differences at the existing seam. All prompt text remains centralized.
- For streaming changes, cover fragmented tool arguments, interleaved reasoning/content, completion, malformed/error responses, cancellation, and body closure. A non-streaming mock cannot establish streaming correctness.
- Check metadata and capability propagation into runtime tuning and admin views. Respect endpoint-based workload classification and local serving limits; provider labels alone do not determine workload. Preserve transitional-versus-terminal failover semantics.

## Verify the changed behavior

Extend matching provider/proxy tests with stubbed transport or local fixtures: exact request mapping, successful parsing, relevant status errors, missing credentials, cancellation, and streaming where touched. Check registrar tests when registration or resolution changes; update production and mock interfaces together if needed. Use existing recording/replay guidance, confirming replay tests execute rather than skip.

Run the repo backend gates; run race checks only for concurrency/lifecycle changes. Update affected contracts and docs through documentation stewardship. Live provider calls require the task's existing network and credential authorization; offline tests are the default.
