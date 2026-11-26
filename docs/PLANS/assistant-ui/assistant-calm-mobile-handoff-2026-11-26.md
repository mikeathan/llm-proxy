---
status: partial
date: 2026-11-26
last_reviewed: 2026-11-26
related_specs: [SPEC-003, SPEC-005, SPEC-007]
related_plans: [assistant-ui/assistant-calm-mobile.md]
---

# Handoff: a calmer assistant, automations and the model-runtime fixes (v0.145.0)

**Status:** partial — built and checked on branch `assistant-calm-mobile`; waits for the owner to merge, deploy and do
the vertex steps below.

## Branch

`assistant-calm-mobile`, from `origin/main` at `1a5306f7` (v0.144.0). Pushed; pull request #318
(https://github.com/mikeathan/llm-proxy/pull/318), not merged. One PR; merge order does not matter.

## Built

- Service: B1–B3 (local model start/stop/switch) and B4 (an **OpenCode Go** provider with the `x-opencode-session`
  header) from the [review](../../audits/2026-11-26-assistant-automations-service-review.md); SPEC-005 1.7.
- UI: the plan's phases 1–4 ([plan](assistant-calm-mobile.md)); SPEC-003 2.78.

## Checks run

`go test ./...`, `go test -race` on `internal/core/llm` and the handlers, `go run ./tools/check-complexity/` (no new
violations), `npm test` (3,687), `npm run build`, the full `npm run test:visual` once and the assistant, automations,
models and dashboard specs after the last change, `./scripts/check-agent-harness.sh`.

**Not run:** any call to OpenCode itself (no key on this machine): whether `reasoning_effort` is accepted for its models
and whether the session id stays the same across turns of a real chat (it does if the system prompt does not change
between turns) are untested. Nothing was tried on vertex.

## Left, in order

1. Owner: merge, deploy to vertex.
2. Owner, in Models › Providers on vertex: add the OpenCode key under **OpenCode Go**, add `glm-5.3-flash` and
   `deepseek-v4.1-flash` from it, then remove the two old models and the NVIDIA "opencode" key. Remove the six orphaned
   model overrides (review B5).
3. Check: one chat turn on `glm-5.3-flash`; if it fails, the failure card's Details names the provider's answer. The
   app log after the next local model start should show no "stale metadata corrected" warning (review B6), and changing
   the log level should no longer log "Signaling stop to local model".
4. The plan's Remaining Work (retry with another model, Undo on memory saves, failed chats in the list, desktop header).

## Traps found

- Measure in CSS pixels in the page; screenshots at device scale 2 double every figure.
- `content-visibility: auto` makes one scroll-to-bottom miss (pitfall 80).
- A setting pushed to every model restarted the running one (pitfall 81).
- Chrome's touch emulation does not reproduce a tap landing on a control that moves after blur; the composer's
  300 ms blur delay is a guard that no test proves necessary.

## First thing to open

`docs/audits/2026-11-26-assistant-automations-service-review.md` → "Status after v0.145.0".
