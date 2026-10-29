import { SETTINGS_GROUP_LABELS, SETTINGS_GROUP_SECTIONS } from "../constants/settings";
import { SETTINGS_GROUP_IDS } from "../types/admin";
import type { SettingsTab, SettingsTabGroup } from "../types/admin";

/**
 * Domain logic for settings categorization and validation.
 */

// Every section that is not a cloud provider; providers are discovered at runtime.
const FIXED_SECTIONS: ReadonlySet<SettingsTab> = new Set<SettingsTab>([
  "appearance", "local", "runs", "memory", "storage", "limits", "local-models", "mcp",
  "guardrails", "security", "processes", "communication", "search", "agents", "network-scan", "sign-in",
]);

/**
 * Checks if a tab is a provider-specific tab.
 */
export const isProviderTab = (tab: SettingsTab): boolean => !FIXED_SECTIONS.has(tab);

/**
 * Groups the Settings sections into the six tabs (plan frontend-redesign-v2, V3).
 * Cloud providers are discovered at runtime, so the Models tab takes whichever the
 * manifests list, in their order; Security is always offered. A tab with no
 * sections is left out.
 */
export const getSettingsTabGroups = (tabs: SettingsTab[]): SettingsTabGroup[] => {
  const present = new Set<SettingsTab>(['security', ...tabs])
  return SETTINGS_GROUP_IDS.map((id) => {
    const fixed = SETTINGS_GROUP_SECTIONS[id].filter((section) => present.has(section))
    const sections = id === 'models' ? [...fixed, ...tabs.filter(isProviderTab)] : fixed
    return { id, label: SETTINGS_GROUP_LABELS[id], sections }
  }).filter((group) => group.sections.length > 0)
}

export const groupOfSection = (groups: SettingsTabGroup[], section: SettingsTab): SettingsTabGroup | undefined =>
  groups.find((group) => group.sections.includes(section))
