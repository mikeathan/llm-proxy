import type { RouteLocationNamedRaw } from 'vue-router'
import { toActivity, toAutomations, toModels, toOverview, toReminders, toSettings, toWorkspaceAssistant, toWorkspaceFile, toWorkspaces } from '../router/routes'
import { ROUTE_NAMES } from '../types/routes'
import type { Destination, NavId, NavItem } from '../types/routes'
import type { RunEndItem } from '../types/notifications'

// The shell's destinations (plan frontend-redesign-v2, V1/V6), built once for both
// navigations: the sidebar and the phone's bottom bar. Assistant leads; Workspace is
// the one entry for the active workspace (Files, Memory, Playbooks, Heartbeat, its
// Settings). Both open the workspace last used, or the workspace list while none is
// known. Pure, so the two navigations cannot drift apart.

interface NavContext {
  lastWorkspace: string | null
  routeName: string | null | undefined
  destination: Destination | undefined
  badges: Partial<Record<NavId, number>>
}

interface NavSpec {
  id: NavId
  label: string
  icon: string
  /** Shown in the phone tab bar; the rest sit under More. */
  tab: boolean
  to: (ws: string | null) => RouteLocationNamedRaw
  current: (ctx: NavContext) => boolean
}

const onDestination = (destination: Destination) => (ctx: NavContext) => ctx.destination === destination

const SPECS: readonly NavSpec[] = [
  {
    id: 'assistant',
    label: 'Assistant',
    icon: 'chat',
    tab: true,
    to: (ws) => (ws ? toWorkspaceAssistant(ws) : toWorkspaces()),
    current: (ctx) => ctx.routeName === ROUTE_NAMES.workspaceAssistant,
  },
  { id: 'overview', label: 'Overview', icon: 'nav-overview', tab: false, to: toOverview, current: onDestination('overview') },
  {
    id: 'workspace',
    label: 'Workspace',
    icon: 'nav-workspaces',
    tab: true,
    to: (ws) => (ws ? toWorkspaceFile(ws) : toWorkspaces()),
    current: (ctx) => ctx.destination === 'workspaces' && ctx.routeName !== ROUTE_NAMES.workspaceAssistant,
  },
  { id: 'automations', label: 'Automations', icon: 'nav-automations', tab: true, to: toAutomations, current: onDestination('automations') },
  { id: 'reminders', label: 'Reminders', icon: 'nav-reminders', tab: true, to: toReminders, current: onDestination('reminders') },
  { id: 'models', label: 'Models', icon: 'nav-models', tab: false, to: toModels, current: onDestination('models') },
  { id: 'activity', label: 'Activity', icon: 'nav-activity', tab: false, to: () => toActivity(), current: onDestination('activity') },
  { id: 'settings', label: 'Settings', icon: 'nav-settings', tab: false, to: () => toSettings(), current: onDestination('settings') },
]

export function buildNavigation(ctx: NavContext): NavItem[] {
  return SPECS.map((spec) => ({
    id: spec.id,
    label: spec.label,
    icon: spec.icon,
    tab: spec.tab,
    to: spec.to(ctx.lastWorkspace),
    current: spec.current(ctx),
    badge: ctx.badges[spec.id] ?? 0,
  }))
}

// Which entry an unread run badges: an assistant run the Assistant, anything else the
// destination that owns it (Workspaces' own destination is the Workspace entry).
export function badgeTarget(item: Pick<RunEndItem, 'kind' | 'destination'>): NavId {
  if (item.kind === 'assistant') return 'assistant'
  return item.destination === 'workspaces' ? 'workspace' : item.destination
}

// Where the app opens: the assistant of the workspace last used, else the Overview.
export const homeLocation = (lastWorkspace: string | null): RouteLocationNamedRaw =>
  lastWorkspace ? toWorkspaceAssistant(lastWorkspace) : toOverview()
