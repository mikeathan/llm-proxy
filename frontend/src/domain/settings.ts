import { SETTINGS_GROUP_LABELS, SETTINGS_GROUP_SECTIONS } from "../constants/settings";
import { SETTINGS_GROUP_IDS } from "../types/admin";
import type { SettingsTab, SettingsTabGroup } from "../types/admin";

/**
 * Domain logic for settings categorization.
 */

/** Every Settings section, in the order of its tabs. */
export const SETTINGS_SECTIONS: readonly SettingsTab[] = SETTINGS_GROUP_IDS.flatMap((id) => SETTINGS_GROUP_SECTIONS[id]);

/** Whether a string from the address is a Settings section. */
export const isSettingsSection = (section: string): section is SettingsTab => (SETTINGS_SECTIONS as readonly string[]).includes(section);

/** The Settings tabs (plan frontend-redesign-v2, V3), each with its sections. */
export const getSettingsTabGroups = (): SettingsTabGroup[] =>
  SETTINGS_GROUP_IDS.map((id) => ({ id, label: SETTINGS_GROUP_LABELS[id], sections: [...SETTINGS_GROUP_SECTIONS[id]] }));

export const groupOfSection = (groups: SettingsTabGroup[], section: SettingsTab): SettingsTabGroup | undefined =>
  groups.find((group) => group.sections.includes(section))
