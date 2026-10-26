---
title: Per-Model Reasoning Overrides and Settings Layout Repair
status: complete
date: 2026-08-02
related_specs: [SPEC-005]
---

# Intent

Add a nullable per-model reasoning toggle for provider-native reasoning modes,
starting with NVIDIA and OpenRouter, and repair the settings form styling lost
when tuning fields were extracted into a child component.

## Scope

### Per-model reasoning override

- Add nullable `reasoning_enabled` configuration so unset, enabled, and
  disabled states remain distinct.
- Persist the field under `settings.yml` model overrides.
- Preserve provider defaults when unset. NVIDIA and OpenRouter default to
  enabled.
- Apply the override only to provider-native reasoning fields:
  - NVIDIA: `chat_template_kwargs.enable_thinking`.
  - OpenRouter: `reasoning.enabled`.
- Serialize explicit OpenRouter `false`; do not allow `omitempty` to erase it.
- Leave local, OpenAI, and Gemini reasoning semantics unchanged until their
  provider-specific disable behavior is separately verified.
- Expose the setting in the per-model settings UI with an enabled default for
  NVIDIA/OpenRouter models.

### Settings layout repair

- Move tuning-form styles from the parent scoped stylesheet into
  `ModelTuningFields.vue`.
- Restore the tuning grid, labels, dark inputs, borders, spacing, and select
  appearance.
- Keep unrelated provider-card styles scoped to `ProviderModelsCard.vue`.
- Preserve responsive behavior and keyboard/focus styling.

## Expected Files

Backend:

- `backend/models/config.go`
- `backend/models/infrastructure.go`
- `backend/models/llm_messages.go`
- `backend/internal/core/assistant/agent.go`
- `backend/internal/core/assistant/reasoning_param.go`
- `backend/internal/core/llm/manager.go`
- `backend/internal/transport/http/handlers/model_handlers.go`
- Relevant backend tests.

Frontend:

- `frontend/src/components/settings/ModelTuningFields.vue`
- `frontend/src/composables/settings/useProviderModels.ts`
- `frontend/src/utils/model/modelUtils.ts`
- `frontend/src/types/model.ts`
- Relevant frontend types/tests.

Documentation:

- Update SPEC-005 if provider reasoning behavior changes the contract.
- Add this plan to `docs/INDEX.md`.
- Record the scoped-child stylesheet pitfall in architecture documentation if
  it is not already documented.

## Test Plan

- Test unset, true, and false reasoning overrides.
- Test NVIDIA wire output for `enable_thinking`.
- Test OpenRouter wire output for explicit `enabled: false`.
- Test persistence and runtime application of model overrides.
- Build and test backend; run complexity check.
- Build frontend.
- Manually verify NVIDIA and OpenRouter settings controls and responsive layout.

## Non-Goals

- No change to `MaxSteps`.
- No generic reasoning-disable behavior for local, OpenAI, or Gemini models.
- No change to cloud token-budget calculation or provider credential
  provisioning.
