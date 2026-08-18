import type { SettingsTab } from "../types/admin";

// Canonical provider key set — single source of truth. The backend mirrors this
// in models.ProviderIDs(); keep the two in sync. Per-concern tables (icons,
// labels, styles) key off this list; a missing entry falls back to a default.
export const PROVIDER_IDS = [
  "local",
  "gemini",
  "openai",
  "openrouter",
  "nvidia",
] as const;

interface ProviderMeta {
  icon: string;
  label: string;
  style: string;
}

// Single aggregated metadata record. Replaces the previously separate
// PROVIDER_ICONS / PROVIDER_LABELS / PROVIDER_STYLES maps so a provider's
// display data lives in exactly one place.
export const PROVIDER_META: Record<SettingsTab, ProviderMeta> = {
  local: { icon: "💻", label: "Local Engine", style: "bg-blue-900/30 text-blue-400 border-blue-500/30" },
  "local-models": { icon: "🤖", label: "Local Models", style: "bg-blue-900/30 text-blue-400 border-blue-500/30" },
  security: { icon: "📟", label: "Host Terminal", style: "bg-gray-900/30 text-gray-400 border-gray-500/30" },
  guardrails: { icon: "🛡️", label: "Agent Guardrails", style: "bg-gray-900/30 text-gray-400 border-gray-500/30" },
  processes: { icon: "🖥️", label: "Model Processes", style: "bg-gray-900/30 text-gray-400 border-gray-500/30" },
  gemini: { icon: "✨", label: "Google Gemini", style: "bg-purple-900/30 text-purple-400 border-purple-500/30" },
  openai: { icon: "🤖", label: "OpenAI / Compatible", style: "bg-green-900/30 text-green-400 border-green-500/30" },
  openrouter: { icon: "🚀", label: "OpenRouter", style: "bg-orange-900/30 text-orange-400 border-orange-500/30" },
  nvidia: { icon: "🟢", label: "NVIDIA NIM", style: "bg-emerald-900/30 text-emerald-400 border-emerald-500/30" },
  mcp: { icon: "🔌", label: "MCP Servers", style: "bg-gray-900/30 text-gray-400 border-gray-500/30" },
  communication: { icon: "📡", label: "Communication", style: "bg-gray-900/30 text-gray-400 border-gray-500/30" },
};

// Thin derived re-exports for any caller still referencing the old names.
export const PROVIDER_ICONS: Record<SettingsTab, string> = Object.fromEntries(
  Object.entries(PROVIDER_META).map(([k, v]) => [k, v.icon]),
) as Record<SettingsTab, string>;

export const PROVIDER_LABELS: Record<SettingsTab, string> = Object.fromEntries(
  Object.entries(PROVIDER_META).map(([k, v]) => [k, v.label]),
) as Record<SettingsTab, string>;

export const PROVIDER_STYLES: Record<string, string> = Object.fromEntries(
  Object.entries(PROVIDER_META).map(([k, v]) => [k, v.style]),
);

export const DEFAULT_PROVIDER_STYLE = "bg-gray-900/30 text-gray-400 border-gray-500/30";
