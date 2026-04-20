import type { SettingsTab } from "../types/admin";

/**
 * Domain logic for settings categorization and validation.
 */

/**
 * Checks if a tab is a provider-specific tab.
 */
export const isProviderTab = (tab: SettingsTab): boolean => {
  return tab !== "local" && tab !== "mcp" && tab !== "guardrails";
};

/**
 * categorizes settings tabs into groups for UI navigation.
 */
export const getSettingsGroups = (tabs: SettingsTab[]) => {
  return [
    {
      name: "System",
      tabs: tabs.filter(t => t === 'local' || t === 'guardrails')
    },
    {
      name: "Cloud Providers",
      tabs: tabs.filter(t => isProviderTab(t))
    },
    {
      name: "Extensions",
      tabs: tabs.filter(t => t === 'mcp')
    }
  ];
};
