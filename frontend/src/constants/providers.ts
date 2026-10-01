import type { SettingsTab } from "../types/admin";

// Canonical provider key set — single source of truth. The backend mirrors this
// in models.ProviderIDs(); keep the two in sync. The label table below keys
// off this list (via SettingsTab).
export const PROVIDER_IDS = [
  "local",
  "gemini",
  "openai",
  "openrouter",
  "nvidia",
] as const;

// Display labels for every Settings category and provider. Keyed by the full
// SettingsTab union, so adding a category or provider without a label fails
// type-checking. (Emoji icons and palette "styles" lived here too; nothing
// shows them since the retro redesign, so they were removed.)
export const PROVIDER_LABELS: Record<SettingsTab, string> = {
  appearance: "Appearance",
  local: "Local Engine",
  "local-models": "Local Models",
  security: "Security & sandboxing",
  guardrails: "Agent Guardrails",
  processes: "Model Processes",
  gemini: "Google Gemini",
  openai: "OpenAI / Compatible",
  openrouter: "OpenRouter",
  nvidia: "NVIDIA NIM",
  mcp: "MCP Servers",
  communication: "Communication",
  search: "Search",
};
