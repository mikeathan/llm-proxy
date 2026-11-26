import type { ProviderType } from "../types/admin";

// Canonical provider key set — single source of truth. The backend mirrors this
// in models.ProviderIDs(); keep the two in sync. The label table below keys
// off this list (via ProviderType).
export const PROVIDER_IDS = [
  "local",
  "gemini",
  "openai",
  "openrouter",
  "nvidia",
] as const;

// Display labels for every provider, keyed by ProviderType so adding a provider without a label fails type-checking. A provider the
// manifests add at runtime is named by its manifest (useProviders.getLabel).
export const PROVIDER_LABELS: Record<ProviderType, string> = {
  local: "Local",
  gemini: "Google Gemini",
  openai: "OpenAI / Compatible",
  openrouter: "OpenRouter",
  nvidia: "NVIDIA NIM",
};
