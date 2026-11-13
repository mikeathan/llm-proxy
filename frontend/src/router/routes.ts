import type { RouteLocationNamedRaw } from 'vue-router'
import { ROUTE_NAMES, WORKSPACE_SECTIONS } from '../types/routes'
import type { RequirementFixSection } from '../types/netaudit'
import type { ActivityFilters, AgentsTab, AutomationStart, NetworkSettingsGroup, NetworkTab, RadioTab, RouteSnapshot, SettingsSection, WorkspaceLocation, WorkspaceSection } from '../types/routes'

// Typed navigation targets (plan D18): the only way the app builds a route.
// Each returns a named location, so a path change touches the route table only.

const PATH_SEPARATOR = '/'

export const toOverview = (): RouteLocationNamedRaw => ({ name: ROUTE_NAMES.overview })

export const toWorkspaces = (): RouteLocationNamedRaw => ({ name: ROUTE_NAMES.workspaces })

export const toWorkspace = (ws: string): RouteLocationNamedRaw => ({ name: ROUTE_NAMES.workspace, params: { ws } })

/** A file inside a workspace; a nested path keeps its segments in the URL. */
export const toWorkspaceFile = (ws: string, path = ''): RouteLocationNamedRaw => ({
  name: ROUTE_NAMES.workspaceFiles,
  params: { ws, path: path.split(PATH_SEPARATOR).filter(Boolean) },
})

export const toWorkspaceAssistant = (ws: string, conversationId?: string): RouteLocationNamedRaw => ({
  name: ROUTE_NAMES.workspaceAssistant,
  params: conversationId ? { ws, conversationId } : { ws },
})

export const toWorkspaceSection = (ws: string, section: WorkspaceSection): RouteLocationNamedRaw => ({
  name: ROUTE_NAMES.workspaceSection,
  params: { ws, section },
})

export const toAutomations = (): RouteLocationNamedRaw => ({ name: ROUTE_NAMES.automations })

// The new-automation form, optionally set up from a playbook in a workspace (the playbook library's "Create automation").
export const toAutomationNew = (start?: AutomationStart): RouteLocationNamedRaw => ({
  name: ROUTE_NAMES.automationNew,
  ...(start ? { query: { ...start } } : {}),
})

export const toAutomationRecordings = (): RouteLocationNamedRaw => ({ name: ROUTE_NAMES.automationRecordings })

export const toAutomation = (id: string): RouteLocationNamedRaw => ({ name: ROUTE_NAMES.automation, params: { id } })

export const toAutomationEdit = (id: string): RouteLocationNamedRaw => ({
  name: ROUTE_NAMES.automationEdit,
  params: { id },
})

/** The Network page; without a tab it opens on the overview. A hash scrolls to a control on that tab. */
export const toNetwork = (tab?: NetworkTab, hash?: string): RouteLocationNamedRaw => ({
  name: ROUTE_NAMES.network,
  params: tab ? { tab } : {},
  ...(hash ? { hash } : {}),
})

/** One group of the Network page's Settings tab (what to scan, watching, outside services, downloaded data, the helper). */
export const toNetworkSettings = (group: NetworkSettingsGroup): RouteLocationNamedRaw => ({ name: ROUTE_NAMES.network, params: { tab: 'settings' }, query: { group } })

/** The Findings tab, scrolled to one finding (its key) and marked for a moment. A finding that is gone from the report says so. */
export const toFinding = (key: string): RouteLocationNamedRaw => ({ name: ROUTE_NAMES.network, params: { tab: 'findings' }, query: { finding: key } })

/** The Findings tab, scrolled to the first finding of one device (its address) with all of that device's findings marked. */
export const toDeviceFindings = (device: string): RouteLocationNamedRaw => ({ name: ROUTE_NAMES.network, params: { tab: 'findings' }, query: { device } })

/** The Agents page; without a tab it opens on the activity. */
export const toAgents = (tab?: AgentsTab): RouteLocationNamedRaw => ({
  name: ROUTE_NAMES.agents,
  params: tab ? { tab } : {},
})

/** The Radio page; without a tab it opens on the sensors. */
export const toRadio = (tab?: RadioTab): RouteLocationNamedRaw => ({
  name: ROUTE_NAMES.radio,
  params: tab ? { tab } : {},
})

/** The place a requirement's fix points at: `network-scan` is the Network page's Settings tab, anything else a Settings section. */
export const toRequirementFix = (section: RequirementFixSection): RouteLocationNamedRaw =>
  section === 'network-scan' ? toNetwork('settings') : toSettings(section)

export const toReminders = (): RouteLocationNamedRaw => ({ name: ROUTE_NAMES.reminders })

export const toModels = (): RouteLocationNamedRaw => ({ name: ROUTE_NAMES.models })

/** Activity with its filters in the query; unset or empty filters are omitted. */
export const toActivity = (filters: ActivityFilters = {}): RouteLocationNamedRaw => {
  const query = Object.fromEntries(Object.entries(filters).filter(([, value]) => Boolean(value)))
  return { name: ROUTE_NAMES.activity, query }
}

export const toSettings = (section?: SettingsSection): RouteLocationNamedRaw => ({
  name: ROUTE_NAMES.settings,
  params: section ? { section } : {},
})

/** The same place in another workspace: its assistant, or its section, else its files (a file path does not carry over). */
export function toSameSectionIn(from: WorkspaceLocation, ws: string): RouteLocationNamedRaw {
  if (from.assistant) return toWorkspaceAssistant(ws)
  if (from.section) return toWorkspaceSection(ws, from.section)
  return toWorkspaceFile(ws)
}

const stringParam = (value: string | string[] | undefined): string | null =>
  typeof value === 'string' && value ? value : null

const isWorkspaceSection = (value: string | null): value is WorkspaceSection =>
  (WORKSPACE_SECTIONS as readonly (string | null)[]).includes(value)

/** Reads what a Workspaces route addresses; the inverse of the builders above. */
export function workspaceLocation(route: { name?: RouteSnapshot['name'] | null; params: RouteSnapshot['params'] }): WorkspaceLocation {
  const { params, name } = route
  const section = stringParam(params.section)
  return {
    ws: stringParam(params.ws),
    filePath: name === ROUTE_NAMES.workspaceFiles && Array.isArray(params.path) ? params.path.join(PATH_SEPARATOR) : '',
    section: name === ROUTE_NAMES.workspaceSection && isWorkspaceSection(section) ? section : null,
    assistant: name === ROUTE_NAMES.workspaceAssistant,
    conversationId: name === ROUTE_NAMES.workspaceAssistant ? stringParam(params.conversationId) : null,
  }
}
