import type { RouteLocationNormalizedLoaded } from 'vue-router'
import type { SettingsTab } from './admin'

// Route names (plan D18). Navigation goes through the typed builders in
// router/routes.ts; components never concatenate paths or spell a name.
export const ROUTE_NAMES = {
  overview: 'overview',
  workspaces: 'workspaces',
  workspace: 'workspace',
  workspaceFiles: 'workspace-files',
  workspaceAssistant: 'workspace-assistant',
  workspaceSection: 'workspace-section',
  automations: 'automations',
  automationNew: 'automation-new',
  automationRecordings: 'automation-recordings',
  automation: 'automation',
  automationEdit: 'automation-edit',
  models: 'models',
  activity: 'activity',
  settings: 'settings',
  design: 'design',
  notFound: 'not-found',
} as const

export type RouteName = (typeof ROUTE_NAMES)[keyof typeof ROUTE_NAMES]

// The six top-level destinations shown in the sidebar (plan D9).
export type Destination = 'overview' | 'workspaces' | 'automations' | 'models' | 'activity' | 'settings'

// Workspace sections addressable at /workspaces/:ws/:section. Files and the
// assistant have their own routes; `settings` is the workspace guardrail layer.
export const WORKSPACE_SECTIONS = ['memory', 'playbooks', 'settings'] as const
export type WorkspaceSection = (typeof WORKSPACE_SECTIONS)[number]

// What a Workspaces route addresses (parsed by router/routes.ts →
// workspaceLocation): the workspace, an open file, a section or the assistant.
export interface WorkspaceLocation {
  ws: string | null
  /** Workspace-relative file path; '' when no file is addressed. */
  filePath: string
  section: WorkspaceSection | null
  assistant: boolean
  conversationId: string | null
}

// The parts of a route a view reads, captured as a plain value.
export type RouteSnapshot = Pick<RouteLocationNormalizedLoaded, 'name' | 'params' | 'query' | 'fullPath'>

// Global Settings sections at /settings/:section.
export type SettingsSection = SettingsTab

// The sections a workspace page links to (WorkspaceHeader).
export type WorkspaceNavSection = 'files' | 'assistant' | WorkspaceSection

// Run outcomes the Activity status filter offers (the run ledger records
// finished automation runs only, so "running" is not a ledger state).
export const RUN_STATUS_FILTERS = ['completed', 'failed'] as const
export type RunStatusFilter = (typeof RUN_STATUS_FILTERS)[number]

// Activity filters live in the route query so a filtered view is linkable.
export interface ActivityFilters {
  kind?: string
  status?: string
  workspace?: string
  q?: string
  from?: string
  to?: string
}

declare module 'vue-router' {
  interface RouteMeta {
    // Sidebar destination this route belongs to; the active item derives from it.
    destination?: Destination
    // Kept alive across destination switches (plan D19).
    keepAlive?: boolean
    // Rendered without the app shell (the dev-only /design reference).
    bare?: boolean
    // Page title segment for document.title.
    title?: string
    // Uses the full content width (IDE-style panes) instead of the reading column.
    fullWidth?: boolean
  }
}
