import type { ProviderType, SettingsTab } from "../types/admin";

export const CLOUD_PROVIDERS: ProviderType[] = [
  "gemini",
  "openai",
  "openrouter",
  "mulerouter",
  "nvidia",
  "vertex",
];

export const ALL_PROVIDERS: ProviderType[] = ["local", ...CLOUD_PROVIDERS];

export const SETTINGS_TABS: SettingsTab[] = [
  "local",
  "local-models",
  "security",
  "guardrails",
  "gemini",
  "openai",
  "openrouter",
  "mulerouter",
  "nvidia",
  "vertex",
  "mcp",
];

export const PROVIDER_ICONS: Record<SettingsTab, string> = {
  local: "💻",
  "local-models": "🤖",
  security: "📟",
  guardrails: "🛡️",
  gemini: "✨",
  openai: "🤖",
  openrouter: "🚀",
  mulerouter: "🐎",
  nvidia: "🟢",
  vertex: "⛰️",
  mcp: "🔌",
};

export const PROVIDER_LABELS: Record<SettingsTab, string> = {
  local: "Local Engine",
  "local-models": "Local Models",
  security: "Host Terminal",
  guardrails: "Agent Guardrails",
  gemini: "Google Gemini",
  openai: "OpenAI / Compatible",
  openrouter: "OpenRouter",
  mulerouter: "MuleRouter",
  nvidia: "NVIDIA NIM",
  vertex: "Google Vertex AI",
  mcp: "MCP Servers",
};

export const PROVIDER_STYLES: Record<ProviderType, string> = {
  local: "bg-blue-900/30 text-blue-400 border-blue-500/30",
  gemini: "bg-purple-900/30 text-purple-400 border-purple-500/30",
  openai: "bg-green-900/30 text-green-400 border-green-500/30",
  openrouter: "bg-orange-900/30 text-orange-400 border-orange-500/30",
  mulerouter: "bg-indigo-900/30 text-indigo-400 border-indigo-500/30",
  nvidia: "bg-emerald-900/30 text-emerald-400 border-emerald-500/30",
  vertex: "bg-red-900/30 text-red-400 border-red-500/30",
};
