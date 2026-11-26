import type { GeneralPanel, SettingsGroupId, SettingsTab } from "../types/admin"

// The Settings tabs (plan frontend-redesign-v2, V3). Sections are the existing `/settings/<section>` routes. Models and the model
// processes are on the Models page (ADR-0090).
export const SETTINGS_GROUP_LABELS: Record<SettingsGroupId, string> = {
  general: "General",
  safety: "Safety",
  connectors: "Connectors",
  appearance: "Appearance",
}

// The label of every Settings section.
export const SETTINGS_SECTION_LABELS: Record<SettingsTab, string> = {
  appearance: "Appearance",
  agents: "Agents",
  memory: "Memory & reminders",
  storage: "Storage & logging",
  security: "Sandboxing",
  limits: "Limits & reset",
  "sign-in": "Sign-in & access",
  guardrails: "Agent Guardrails",
  communication: "Communication",
  search: "Search",
}

// General is short sections (plan frontend-redesign-v2); each panel of GlobalSettings belongs to one.
export const GENERAL_PANEL_SECTION: Record<GeneralPanel, SettingsTab> = {
  memory: "memory",
  reminders: "memory",
  storage: "storage",
  logging: "storage",
}

export const SETTINGS_GROUP_SECTIONS: Record<SettingsGroupId, readonly SettingsTab[]> = {
  general: ["agents", "memory", "storage"],
  safety: ["security", "sign-in", "limits", "guardrails"],
  connectors: ["communication", "search"],
  appearance: ["appearance"],
}

// The Agents editor groups the settings the server describes (profile-schema) into panels; a setting the server adds
// that is not listed here lands in a last panel, so it is never hidden.
export const AGENT_FORM_SECTIONS: readonly { title: string; keys: readonly string[] }[] = [
  { title: 'About', keys: ['name', 'summary'] },
  { title: 'Instructions', keys: ['instructions'] },
  { title: 'Behaviour', keys: ['answer_first', 'may_ask', 'max_steps', 'reminders'] },
  { title: 'Tools', keys: ['tools'] },
  { title: 'Memory', keys: ['memory', 'memory_save'] },
  { title: 'Privacy', keys: ['local_only'] },
  { title: 'Helpers', keys: ['delegates'] },
]
export const AGENT_FORM_OTHER_TITLE = 'More'

// Helpers (delegated sub-agents): the pool's default and ceiling, as the backend's models.AgentConfig sets them.
export const SUBAGENT_DEFAULT_CONCURRENCY = 3
export const SUBAGENT_MAX_CONCURRENCY = 10
