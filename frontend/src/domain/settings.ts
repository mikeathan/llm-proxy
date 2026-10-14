import { SETTINGS_GROUP_LABELS, SETTINGS_GROUP_SECTIONS } from "../constants/settings";
import { SETTINGS_GROUP_IDS } from "../types/admin";
import type { SettingsTab, SettingsTabGroup } from "../types/admin";

/**
 * Domain logic for settings categorization and validation.
 */

/**
 * Checks if a tab is a provider-specific tab.
 */
export const isProviderTab = (tab: SettingsTab): boolean => {
  return tab !== "appearance" && tab !== "local" && tab !== "runs" && tab !== "memory" && tab !== "storage" && tab !== "limits" && tab !== "local-models" && tab !== "mcp" && tab !== "guardrails" && tab !== "security" && tab !== "processes" && tab !== "communication" && tab !== "search";
};

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
