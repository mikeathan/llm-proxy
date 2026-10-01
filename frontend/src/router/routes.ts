import type { RouteLocationNamedRaw } from 'vue-router'
import { ROUTE_NAMES, WORKSPACE_SECTIONS } from '../types/routes'
import type { ActivityFilters, RouteSnapshot, SettingsSection, WorkspaceLocation, WorkspaceSection } from '../types/routes'

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

export const toAutomationNew = (): RouteLocationNamedRaw => ({ name: ROUTE_NAMES.automationNew })

export const toAutomationRecordings = (): RouteLocationNamedRaw => ({ name: ROUTE_NAMES.automationRecordings })

export const toAutomation = (id: string): RouteLocationNamedRaw => ({ name: ROUTE_NAMES.automation, params: { id } })

export const toAutomationEdit = (id: string): RouteLocationNamedRaw => ({
  name: ROUTE_NAMES.automationEdit,
  params: { id },
})

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
