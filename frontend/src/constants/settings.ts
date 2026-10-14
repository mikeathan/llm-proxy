import type { GeneralPanel, SettingsGroupId, SettingsTab } from "../types/admin"

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

// The labels of the General sections. `local` is also the Local Engine provider's label on the
// Models page, which is why the Settings row has its own wording for it.
export const SETTINGS_SECTION_LABELS: Partial<Record<SettingsTab, string>> = {
  local: "Routing & engine",
  runs: "Runs",
  memory: "Memory & reminders",
  storage: "Storage & logging",
}

// General is four short sections (plan frontend-redesign-v2); each panel of GlobalSettings belongs to one.
export const GENERAL_PANEL_SECTION: Record<GeneralPanel, SettingsTab> = {
  routing: "local",
  engine: "local",
  lifecycle: "local",
  gpu: "local",
  scheduler: "runs",
  external: "runs",
  memory: "memory",
  reminders: "memory",
  storage: "storage",
  logging: "storage",
}

export const SETTINGS_GROUP_SECTIONS: Record<SettingsGroupId, readonly SettingsTab[]> = {
  general: ["local", "runs", "memory", "storage"],
  models: ["local-models"],
  safety: ["security", "guardrails"],
  connectors: ["communication", "search", "mcp"],
  appearance: ["appearance"],
  system: ["processes"],
}
