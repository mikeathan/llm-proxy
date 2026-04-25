import type { SettingsTab } from "../types/admin";

/**
 * Domain logic for settings categorization and validation.
 */

/**
 * Checks if a tab is a provider-specific tab.
 */
export const isProviderTab = (tab: SettingsTab): boolean => {
  return tab !== "local" && tab !== "mcp" && tab !== "guardrails" && tab !== "security";
};

/**
 * categorizes settings tabs into groups for UI navigation.
 */
export const getSettingsGroups = (tabs: SettingsTab[]) => {
  // ensure security is injected if not already present in the source tabs
  const enhancedTabs = tabs.includes('security') ? tabs : ['security', ...tabs]
  
  return [
    {
      name: "System",
      tabs: enhancedTabs.filter(t => t === 'local' || t === 'guardrails' || t === 'security')
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
