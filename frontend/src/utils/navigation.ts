import type { RouteLocationNamedRaw } from 'vue-router'
import { toActivity, toAgents, toAutomations, toDashboard, toModels, toNetwork, toRadio, toReminders, toSettings, toStacks, toWorkspaceAssistant, toWorkspaceFile, toWorkspaces } from '../router/routes'
import { ROUTE_NAMES } from '../types/routes'
import { HOME_BOARD_ID } from '../constants/dashboard'
import type { Destination, NavEntry, NavGroup, NavGroupId, NavId, NavItem } from '../types/routes'
import type { NotificationItem } from '../types/notifications'
import { STACKS_GROUP_LABEL } from '../constants/stacks'
import { STACK_IDS } from '../types/stacks'
import type { StackId } from '../types/stacks'

// The shell's destinations (plan frontend-redesign-v2, V1/V6), built once for both
// navigations: the sidebar and the phone's bottom bar. The Dashboard leads (it is where the app opens), then the Assistant; Workspace is
// the one entry for the active workspace (Files, Memory, Playbooks, Heartbeat, its
// Settings). Both open the workspace last used, or the workspace list while none is
// known. Pure, so the two navigations cannot drift apart.

interface NavContext {
  /** The stacks with a module on; null while unknown, which shows every stack. */
  stacksOn?: ReadonlySet<string> | null
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
  group?: NavGroupId
  /** A stack's entry shows only while the stack has a module on. */
  stack?: StackId
  to: (ws: string | null) => RouteLocationNamedRaw
  current: (ctx: NavContext) => boolean
}

const onDestination = (destination: Destination) => (ctx: NavContext) => ctx.destination === destination


const SPECS: readonly NavSpec[] = [
  { id: 'dashboard', label: 'Dashboard', icon: 'nav-overview', tab: true, to: () => toDashboard(HOME_BOARD_ID), current: onDestination('dashboard') },
  {
    id: 'assistant',
    label: 'Assistant',
    icon: 'chat',
    tab: true,
    to: (ws) => (ws ? toWorkspaceAssistant(ws) : toWorkspaces()),
    current: (ctx) => ctx.routeName === ROUTE_NAMES.workspaceAssistant,
  },
  {
    id: 'workspace',
    label: 'Workspace',
    icon: 'nav-workspaces',
    tab: true,
    to: (ws) => (ws ? toWorkspaceFile(ws) : toWorkspaces()),
    current: (ctx) => ctx.destination === 'workspaces' && ctx.routeName !== ROUTE_NAMES.workspaceAssistant,
  },
  { id: 'automations', label: 'Automations', icon: 'nav-automations', tab: true, to: toAutomations, current: onDestination('automations') },
  { id: 'network', label: 'Network', icon: 'nav-network', tab: false, group: 'stacks', stack: 'network', to: () => toNetwork(), current: onDestination('network') },
  { id: 'agents', label: 'Agents', icon: 'nav-agents', tab: false, group: 'stacks', stack: 'agents', to: () => toAgents(), current: onDestination('agents') },
  { id: 'radio', label: 'Radio', icon: 'nav-radio', tab: false, group: 'stacks', stack: 'radio', to: () => toRadio(), current: onDestination('radio') },
  { id: 'stacks', label: 'Add a stack', icon: 'nav-stacks', tab: false, group: 'stacks', to: () => toStacks(), current: onDestination('stacks') },
  { id: 'reminders', label: 'Reminders', icon: 'nav-reminders', tab: false, to: toReminders, current: onDestination('reminders') },
  { id: 'models', label: 'Models', icon: 'nav-models', tab: false, to: toModels, current: onDestination('models') },
  { id: 'activity', label: 'Activity', icon: 'nav-activity', tab: false, to: () => toActivity(), current: onDestination('activity') },
  { id: 'settings', label: 'Settings', icon: 'nav-settings', tab: false, to: () => toSettings(), current: onDestination('settings') },
]

/** The entry that switches a stack on shows only while one is off (and the list is known). */
const stackIsOff = (on: ReadonlySet<string> | null | undefined): boolean => !!on && STACK_IDS.some((id) => !on.has(id))

export function buildNavigation(ctx: NavContext): NavItem[] {
  return SPECS.filter((spec) => spec.id === 'stacks' ? stackIsOff(ctx.stacksOn) : !spec.stack || !ctx.stacksOn || ctx.stacksOn.has(spec.stack)).map((spec) => ({
    id: spec.id,
    label: spec.label,
    icon: spec.icon,
    tab: spec.tab,
    ...(spec.group ? { group: spec.group } : {}),
    to: spec.to(ctx.lastWorkspace),
    current: spec.current(ctx),
    badge: ctx.badges[spec.id] ?? 0,
  }))
}

const GROUPS: Record<NavGroupId, Pick<NavGroup, 'label' | 'icon'>> = {
  stacks: { label: STACKS_GROUP_LABEL, icon: 'nav-stacks' },
}

// Folds the flat list into display rows: a group appears once, where its first destination
// sat, and holds all of its destinations. Pure, shared by the sidebar and the More sheet.
export function groupNavigation(items: readonly NavItem[]): NavEntry[] {
  const entries: NavEntry[] = []
  const groups = new Map<NavGroupId, NavGroup>()
  for (const item of items) {
    if (!item.group) {
      entries.push({ kind: 'item', item })
      continue
    }
    let group = groups.get(item.group)
    if (!group) {
      group = { id: item.group, ...GROUPS[item.group], items: [], current: false, badge: 0 }
      groups.set(item.group, group)
      entries.push({ kind: 'group', group })
    }
    group.items.push(item)
    group.current ||= item.current
    group.badge += item.badge
  }
  return entries
}

// Which entry an unread item badges: an assistant run the Assistant, anything else the
// destination that owns it (a reminder, Reminders) (Workspaces' own destination is the Workspace entry).
export function badgeTarget(item: Pick<NotificationItem, 'kind' | 'destination'>): NavId {
  if (item.kind === 'assistant') return 'assistant'
  return item.destination === 'workspaces' ? 'workspace' : item.destination
}

// Where the app opens: the dashboard, whose Home board mixes cards from every part of the system.
export const homeLocation = (): RouteLocationNamedRaw => toDashboard()
