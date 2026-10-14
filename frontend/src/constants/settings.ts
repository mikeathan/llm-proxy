import type { SettingsGroupId, SettingsTab } from "../types/admin"

// The Settings tabs (plan frontend-redesign-v2, V3). Sections are the existing
// `/settings/<section>` routes; the Models tab also takes every cloud provider
// the manifests discover, so it is not listed here beyond its fixed section.
export const SETTINGS_GROUP_LABELS: Record<SettingsGroupId, string> = {
  general: "General",
  models: "Models",
  safety: "Safety",
  connectors: "Connectors",
  appearance: "Appearance",
  system: "System",
}

export const SETTINGS_GROUP_SECTIONS: Record<SettingsGroupId, readonly SettingsTab[]> = {
  general: ["local"],
  models: ["local-models"],
  safety: ["security", "guardrails"],
  connectors: ["communication", "search", "mcp"],
  appearance: ["appearance"],
  system: ["processes"],
}
