import type { ProviderType, AgentDefaults } from "../../types/admin";
import type { LoopStrategy, LoopStrategyOption, LoopStrategyOptionKey, Model, ModelForm, TuningSettings } from "../../types/model";

/**
 * Returns the provider's default prefill flag.  tool_call_format is intentionally
 * left empty ("") for every provider so the backend applies its own default
 * (XML text mode for local, native for cloud) rather than persisting an
 * unnecessary override.
 */
function defaultPrefill(provider: ProviderType): boolean {
  return provider === "local";
}

/**
 * Reports whether a URL targets a local serving host (loopback / unspecified).
 * Used to provisionally classify an unsaved model's workload from a
 * credential's base_url — the backend remains authoritative after save.
 */
export function isLocalEndpoint(baseUrl?: string): boolean {
  if (!baseUrl) return false;
  try {
    const hostname = new URL(baseUrl).hostname.toLowerCase();
    return (
      hostname === "localhost" ||
      hostname === "::1" ||
      hostname === "0.0.0.0" ||
      hostname === "127.0.0.1" ||
      hostname.startsWith("127.")
    );
  } catch {
    return false;
  }
}

/**
 * Returns the default agent tuning settings for a given provider,
 * using backend-supplied defaults as the base.
 */
export function getDefaultModelSettings(
  provider: ProviderType,
  defaults: AgentDefaults,
): TuningSettings {
  return {
    max_steps: defaults.max_steps,
    context_budget: defaults.context_budget,
    max_tokens: defaults.max_tokens,
    temperature: defaults.temperature,
    reasoning_budget: defaults.reasoning_budget,
    reasoning_enabled: defaults.reasoning?.default_enabled ?? false,
    timeout_minutes: defaults.timeout_minutes,
    tool_call_format: "",
    prefill: defaultPrefill(provider),
    tool_timeout_seconds: defaults.tool_timeout_seconds,
    filesystem_tool_timeout_seconds: defaults.filesystem_tool_timeout_seconds,
    max_plan_duration_minutes: defaults.max_plan_duration_minutes,
    max_plan_steps: defaults.max_plan_steps,
    guardrail_timeout_seconds: defaults.guardrail_timeout_seconds,
    guardrail_timeout_behavior: defaults.guardrail_timeout_behavior,
    guardrail_approval_timeout_seconds: defaults.guardrail_approval_timeout_seconds,
    loop_strategy: defaults.loop_strategy ?? "",
  };
}

/**
 * Loop-strategy option copy. The option *list* is backend-driven
 * (`config.loop_strategy_options`, from the strategy registry) so a new strategy
 * needs no frontend edit; this map only carries the human-facing label/help text
 * keyed by the known values. Unknown values fall back to the raw value.
 */
export const LOOP_STRATEGY_COPY: Record<
  LoopStrategyOptionKey,
  { label: string; description: string }
> = {
  react: {
    label: "ReAct (auto)",
    description:
      "The model decides each next step and stops when it produces a final report. Simplest and most flexible; best default for most tasks.",
  },
  plan_execute: {
    label: "Plan & Execute",
    description:
      "Writes a step-by-step tool plan first, then executes each step in order. Best for multi-step tool tasks; falls back to ReAct if planning fails.",
  },
  evaluator_optimizer: {
    label: "Evaluator-Optimizer",
    description:
      "ReAct loop plus a self-review pass before finalizing (up to 2 rounds). Best for code/analysis where verifying the work matters.",
  },
};

/**
 * Builds the loop-strategy option list from the backend-surfaced names. Falls
 * back to the three known values when the list is empty; unknown values get the
 * raw value as label with no description.
 */
export function loopStrategyOptions(available?: string[]): LoopStrategyOption[] {
  const names = available && available.length > 0 ? available : Object.keys(LOOP_STRATEGY_COPY);
  return names.map((value) => {
    const copy = LOOP_STRATEGY_COPY[value as LoopStrategyOptionKey];
    return {
      value,
      label: copy?.label ?? value,
      description: copy?.description ?? "",
    };
  });
}

/**
 * Returns the description for a selected loop-strategy value (empty → react's).
 */
export function loopStrategyDescription(value: LoopStrategy): string {
  const key = (value || "react") as LoopStrategyOptionKey;
  return LOOP_STRATEGY_COPY[key]?.description ?? "";
}

/**
 * Calculates the next available port for a local model.
 */
export function getNextLocalPort(models: Model[]): number {
  const localModels = models.filter((m) => m.provider === "local");
  let port = 8081;
  for (const m of localModels) {
    if (m.port && m.port >= port) port = m.port + 1;
  }
  return port;
}

/**
 * Derives a friendly model name from an ID or filename.
 */
export function deriveModelName(modelId?: string, filename?: string): string {
  if (modelId) {
    const parts = modelId.split("/");
    const last = parts[parts.length - 1];
    return last || modelId;
  }
  if (filename) {
    return filename.replace(/\.gguf$/i, "").split("/").pop() || filename;
  }
  return "";
}

/**
 * Creates a fresh model form object with provider-specific defaults.
 */
export function createEmptyModelForm(
  provider: ProviderType,
  existingModels: Model[],
  defaults: AgentDefaults,
): ModelForm {
  const tuning = getDefaultModelSettings(provider, defaults);
  return {
    key: "",
    id: "",
    name: "",
    filename: "",
    port: getNextLocalPort(existingModels),
    args: "",
    ...tuning,
    slot_timeout: 0,
  };
}
