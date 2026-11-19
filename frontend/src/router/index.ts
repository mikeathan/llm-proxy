import { createRouter, createWebHistory, type LocationQuery, type RouteLocationRaw, type RouteRecordRaw, type Router, type RouterHistory, type RouterScrollBehavior } from 'vue-router'
import { AGENTS_TABS, NETWORK_TABS, RADIO_TABS, REMINDERS_TABS, ROUTE_NAMES, WORKSPACE_SECTIONS } from '../types/routes'
import { toAgents, toDashboard, toWorkspaceFile, toWorkspaceSection } from './routes'
import { homeLocation } from '../utils/navigation'
import { DASHBOARD_TITLE } from '../constants/dashboard'

// Route table (plan D18). Every page is a lazy chunk. Routes that share a
// loader render the same component, so moving between them keeps one instance
// (e.g. every Workspaces route reuses WorkspacesView and its state).

const DashboardView = () => import('../views/DashboardView.vue')
const WorkspacesView = () => import('../views/WorkspacesView.vue')
const AutomationsView = () => import('../views/AutomationsView.vue')
const NetworkView = () => import('../views/NetworkView.vue')
const AgentsView = () => import('../views/AgentsView.vue')
const StacksView = () => import('../views/StacksView.vue')
const RadioView = () => import('../views/RadioView.vue')
const RadioDeviceView = () => import('../views/RadioDeviceView.vue')
const RemindersView = () => import('../views/RemindersView.vue')
const ModelsView = () => import('../views/ModelsView.vue')
const ActivityView = () => import('../views/ActivityView.vue')
const SettingsView = () => import('../views/SettingsView.vue')
const NotFoundView = () => import('../views/NotFoundView.vue')
const LoginView = () => import('../views/LoginView.vue')
const SetupView = () => import('../views/SetupView.vue')

const WORKSPACES_META = { destination: 'workspaces', keepAlive: true, fullWidth: true, title: 'Workspaces' } as const
// The assistant is a Workspaces route (one kept-alive view), titled for what it is.
const ASSISTANT_META = { ...WORKSPACES_META, title: 'Assistant' } as const
const AUTOMATIONS_META = { destination: 'automations', fullWidth: true, title: 'Automations' } as const
const NETWORK_TAB_CARDS: Readonly<Record<string, string>> = { devices: 'network-devices', findings: 'network-findings', traffic: 'network-traffic', audits: 'network-audits' }
const toBoardCard = (board: string, card: string, query: LocationQuery): RouteLocationRaw => ({ ...toDashboard(board, card), query: { ...query, board, card } })
const NETWORK_TAB_PATTERN = NETWORK_TABS.join('|')
const AGENTS_TAB_PATTERN = AGENTS_TABS.join('|')
const RADIO_TAB_PATTERN = RADIO_TABS.join('|')
const REMINDERS_TAB_PATTERN = REMINDERS_TABS.join('|')
const SECTION_PATTERN = WORKSPACE_SECTIONS.join('|')

// Dev-only living design reference (D23): `import.meta.env.DEV` is a build-time
// constant, so the production bundle drops the record and its chunk.
const devRoutes: RouteRecordRaw[] = import.meta.env.DEV
  ? [{ path: '/design', name: ROUTE_NAMES.design, component: () => import('../views/DesignView.vue'), meta: { bare: true, title: 'Design' } }]
  : []

export const routes: RouteRecordRaw[] = [
  // The app opens on the dashboard: its Home board mixes cards from every part of the system.
  { path: '/', redirect: () => homeLocation() },
  { path: '/dashboard', name: ROUTE_NAMES.dashboard, component: DashboardView, meta: { destination: 'dashboard', fullWidth: true, title: DASHBOARD_TITLE } },

  { path: '/workspaces', name: ROUTE_NAMES.workspaces, component: WorkspacesView, meta: WORKSPACES_META },
  {
    path: '/workspaces/:ws',
    name: ROUTE_NAMES.workspace,
    redirect: (to) => toWorkspaceFile(String(to.params.ws)),
  },
  { path: '/workspaces/:ws/files/:path(.*)*', name: ROUTE_NAMES.workspaceFiles, component: WorkspacesView, meta: WORKSPACES_META },
  { path: '/workspaces/:ws/assistant/:conversationId?', name: ROUTE_NAMES.workspaceAssistant, component: WorkspacesView, meta: ASSISTANT_META },
  // The workspace guardrail policy moved from Security to Settings (Phase 5); old links still land.
  {
    path: '/workspaces/:ws/security',
    redirect: (to) => toWorkspaceSection(String(to.params.ws), 'settings'),
  },
  { path: `/workspaces/:ws/:section(${SECTION_PATTERN})`, name: ROUTE_NAMES.workspaceSection, component: WorkspacesView, meta: WORKSPACES_META },

  { path: '/automations', name: ROUTE_NAMES.automations, component: AutomationsView, meta: AUTOMATIONS_META },
  { path: '/automations/new', name: ROUTE_NAMES.automationNew, component: AutomationsView, meta: AUTOMATIONS_META },
  { path: '/automations/recordings', name: ROUTE_NAMES.automationRecordings, component: AutomationsView, meta: AUTOMATIONS_META },
  { path: '/automations/:id', name: ROUTE_NAMES.automation, component: AutomationsView, meta: AUTOMATIONS_META },
  { path: '/automations/:id/edit', name: ROUTE_NAMES.automationEdit, component: AutomationsView, meta: AUTOMATIONS_META },

  // The tabs that became cards of the dashboard's boards keep their addresses: a bookmark or an old link lands on the card (and keeps a `?finding=` or `?device=`).
  { path: `/network/:tab(${Object.keys(NETWORK_TAB_CARDS).join('|')})`, redirect: (to) => toBoardCard('network', NETWORK_TAB_CARDS[String(to.params.tab)]!, to.query) },
  { path: '/radio/devices', redirect: (to) => toBoardCard('radio', 'radio-devices', to.query) },
  { path: '/radio/sensors', redirect: (to) => toBoardCard('radio', 'radio-sensors', to.query) },
  { path: `/network/:tab(${NETWORK_TAB_PATTERN})?`, name: ROUTE_NAMES.network, component: NetworkView, meta: { destination: 'network', fullWidth: true, title: 'Network' } },
  { path: '/stacks', name: ROUTE_NAMES.stacks, component: StacksView, meta: { destination: 'stacks', title: 'Stacks' } },
  { path: `/agents/:tab(${AGENTS_TAB_PATTERN})?`, name: ROUTE_NAMES.agents, component: AgentsView, meta: { destination: 'agents', fullWidth: true, title: 'Agents' } },
  { path: `/radio/:tab(${RADIO_TAB_PATTERN})?`, name: ROUTE_NAMES.radio, component: RadioView, meta: { destination: 'radio', fullWidth: true, title: 'Radio' } },
  // One device in full. Its own segment ("device"), so the tab pattern above never captures it.
  { path: '/radio/device/:id([0-9a-f]{8})', name: ROUTE_NAMES.radioDevice, component: RadioDeviceView, meta: { destination: 'radio', fullWidth: true, title: 'Radio device' } },
  { path: `/reminders/:tab(${REMINDERS_TAB_PATTERN})?`, name: ROUTE_NAMES.reminders, component: RemindersView, meta: { destination: 'reminders', title: 'Reminders' } },

  { path: '/models', name: ROUTE_NAMES.models, component: ModelsView, meta: { destination: 'models', title: 'Models' } },
  { path: '/activity', name: ROUTE_NAMES.activity, component: ActivityView, meta: { destination: 'activity', keepAlive: true, title: 'Activity' } },
  // MCP servers moved from Settings to the Agents stack's Tools tab (ADR-0069); an old link or bookmark lands there.
  { path: '/settings/mcp', redirect: () => toAgents('tools') },
  { path: '/settings/:section?', name: ROUTE_NAMES.settings, component: SettingsView, meta: { destination: 'settings', workspaceScope: 'all', title: 'Settings' } },

  // Sign-in (plan admin-api-authentication): bare pages, open to someone who is not signed in. The guard in
  // composables/auth/startAuth.ts decides who is sent where.
  { path: '/login', name: ROUTE_NAMES.login, component: LoginView, meta: { bare: true, public: true, title: 'Sign in' } },
  { path: '/setup', name: ROUTE_NAMES.setup, component: SetupView, meta: { bare: true, public: true, title: 'Set up sign-in' } },

  ...devRoutes,

  // Last: a bad link renders inside the shell — never a silent redirect.
  { path: '/:pathMatch(.*)*', name: ROUTE_NAMES.notFound, component: NotFoundView, meta: { title: 'Not found' } },
]

export const scrollBehavior: RouterScrollBehavior = (_to, _from, savedPosition) => savedPosition ?? { top: 0 }

/** Creates the app router. History defaults to HTML5 history under Vite's `base`. */
export function createAppRouter(history: RouterHistory = createWebHistory(import.meta.env.BASE_URL)): Router {
  return createRouter({ history, routes, scrollBehavior })
}
