import type { ProviderType, AgentDefaults } from "../types/admin";
import type { Model } from "../types/model";

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
  reasoning_budget: number;
  slot_timeout: number;
  tool_call_format: string;
  prefill: boolean;
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
): { max_steps: number; context_budget: number; max_tokens: number; reasoning_budget: number; tool_call_format: string; prefill: boolean } {
  const hints = providerTuningHints(provider);
  return {
    max_steps: defaults.max_steps,
    context_budget: defaults.context_budget,
    max_tokens: defaults.max_tokens,
    reasoning_budget: defaults.reasoning_budget,
    tool_call_format: hints.tool_call_format,
    prefill: hints.prefill,
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
    reasoning_budget: tuning.reasoning_budget,
    slot_timeout: 0,
  };
}
