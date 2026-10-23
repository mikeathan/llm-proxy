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

// The labels of the General and Safety sections. `local` is also the Local Engine provider's label on the
// Models page, which is why the Settings row has its own wording for it.
export const SETTINGS_SECTION_LABELS: Partial<Record<SettingsTab, string>> = {
  local: "Routing & engine",
  runs: "Runs",
  agents: "Agents",
  memory: "Memory & reminders",
  storage: "Storage & logging",
  security: "Sandboxing",
  limits: "Limits & reset",
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
  general: ["local", "runs", "agents", "memory", "storage"],
  models: ["local-models"],
  safety: ["security", "limits", "guardrails"],
  connectors: ["communication", "search", "mcp"],
  appearance: ["appearance"],
  system: ["processes"],
}

// The Agents editor groups the settings the server describes (profile-schema) into panels; a setting the server adds
// that is not listed here lands in a last panel, so it is never hidden.
export const AGENT_FORM_SECTIONS: readonly { title: string; keys: readonly string[] }[] = [
  { title: 'About', keys: ['name', 'summary'] },
  { title: 'Instructions', keys: ['instructions'] },
  { title: 'Behaviour', keys: ['answer_first', 'may_ask', 'max_steps', 'reminders'] },
  { title: 'Tools', keys: ['tools'] },
  { title: 'Memory', keys: ['memory', 'memory_save'] },
  { title: 'Helpers', keys: ['delegates'] },
]
export const AGENT_FORM_OTHER_TITLE = 'More'

// Helpers (delegated sub-agents): the pool's default and ceiling, as the backend's models.AgentConfig sets them.
export const SUBAGENT_DEFAULT_CONCURRENCY = 3
export const SUBAGENT_MAX_CONCURRENCY = 10
