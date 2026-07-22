import type { ProviderType, AgentDefaults } from "../../types/admin";
import type { Model } from "../../types/model";

export interface ModelForm {
  key: string;
  id: string;
  name: string;
  filename: string;
  port: number;
  args: string;
  max_steps: number;
  context_budget: number;
  max_tokens: number;
  temperature: number;
  reasoning_budget: number;
  timeout_minutes: number;
  slot_timeout: number;
  tool_call_format: string;
  prefill: boolean;
  tool_timeout_seconds: number;
  filesystem_tool_timeout_seconds: number;
  max_plan_duration_minutes: number;
  max_plan_steps: number;
  guardrail_timeout_seconds: number;
  guardrail_timeout_behavior: string;
}

/**
 * Provider-specific hints applied on top of the backend's base defaults.
 * Local models need XML tool format + prefill; cloud models default to native.
 */
function providerTuningHints(provider: ProviderType): { tool_call_format: string; prefill: boolean } {
  if (provider === "local") {
    return { tool_call_format: "xml", prefill: true };
  }
  return { tool_call_format: "", prefill: false };
}

/**
 * Returns the default agent tuning settings for a given provider,
 * using backend-supplied defaults as the base.
 */
export function getDefaultModelSettings(
  provider: ProviderType,
  defaults: AgentDefaults,
): { max_steps: number; context_budget: number; max_tokens: number; temperature: number; reasoning_budget: number; timeout_minutes: number; tool_call_format: string; prefill: boolean; tool_timeout_seconds: number; filesystem_tool_timeout_seconds: number; max_plan_duration_minutes: number; max_plan_steps: number; guardrail_timeout_seconds: number; guardrail_timeout_behavior: string } {
  const hints = providerTuningHints(provider);
  return {
    max_steps: defaults.max_steps,
    context_budget: defaults.context_budget,
    max_tokens: defaults.max_tokens,
    temperature: defaults.temperature,
    reasoning_budget: defaults.reasoning_budget,
    timeout_minutes: defaults.timeout_minutes,
    tool_call_format: hints.tool_call_format,
    prefill: hints.prefill,
    tool_timeout_seconds: defaults.tool_timeout_seconds,
    filesystem_tool_timeout_seconds: defaults.filesystem_tool_timeout_seconds,
    max_plan_duration_minutes: defaults.max_plan_duration_minutes,
    max_plan_steps: defaults.max_plan_steps,
    guardrail_timeout_seconds: defaults.guardrail_timeout_seconds,
    guardrail_timeout_behavior: defaults.guardrail_timeout_behavior,
  };
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
 * Given a model's context length (n_ctx_train or limits.context),
 * returns the suggested form defaults for max_tokens and context_budget.
 * Called when the user picks a model from the provider dropdown — the
 * backend runs the same computation in ApplyMetadataDefaults on save,
 * but showing the values upfront lets the user adjust before submitting.
 *
 * Budget reserves max_tokens space in the context window for the response
 * output.  The result is rounded to the nearest 1000 to satisfy the
 * step="1000" constraint on the form inputs.
 */
export function computeDefaultsFromContext(ctxLen?: number): { context_budget: number; max_tokens: number } | null {
  if (!ctxLen || ctxLen <= 0) return null;
  const maxTokens = Math.floor(ctxLen / 4);
  const availableCtx = ctxLen - maxTokens;
  const budget = availableCtx * 2;
  return {
    context_budget: Math.round(budget / 1000) * 1000,
    max_tokens: maxTokens,
  };
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
    max_tokens: tuning.max_tokens,
    temperature: tuning.temperature,
    reasoning_budget: tuning.reasoning_budget,
    timeout_minutes: tuning.timeout_minutes,
    slot_timeout: 0,
  };
}
