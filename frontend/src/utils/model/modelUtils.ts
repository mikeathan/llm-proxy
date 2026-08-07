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
  reasoning_enabled: boolean;
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
): { max_steps: number; context_budget: number; max_tokens: number; temperature: number; reasoning_budget: number; reasoning_enabled: boolean; timeout_minutes: number; tool_call_format: string; prefill: boolean; tool_timeout_seconds: number; filesystem_tool_timeout_seconds: number; max_plan_duration_minutes: number; max_plan_steps: number; guardrail_timeout_seconds: number; guardrail_timeout_behavior: string } {
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
    slot_timeout: 0,
  };
}
