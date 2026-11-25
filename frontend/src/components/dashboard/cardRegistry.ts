import { defineAsyncComponent } from 'vue'
import { toActivity, toAgents, toAutomations, toModels, toNetwork, toNetworkSettings, toNewsroom, toRadio, toReminders, toWorkspaces } from '../../router/routes'
import { ACTIVITY_TITLE } from '../../constants/netaudit'
import type { DashboardCardDef } from '../../types/dashboard'

// The catalogue of dashboard cards: one entry per card, defined once. A board only places them, so the same card can sit on Home and on its
// stack's board. Each entry says where the card comes from, its default size and which control page its arrow opens. Card components are
// loaded when first shown, so a board costs only the cards on it.
const card = (name: string) => defineAsyncComponent(() => import(`./cards/${name}.vue`))

export const CARD_DEFS: readonly DashboardCardDef[] = [
  { id: 'assistant-recent', source: 'assistant', title: 'Recent conversations', component: card('AssistantRecentCard'), cols: 4, rows: 1, control: () => toWorkspaces() },
  { id: 'agents-activity', source: 'agents', title: 'Agent activity', component: card('AgentsActivityCard'), cols: 6, rows: 1, control: () => toAgents() },
  { id: 'host-health', source: 'host', title: 'Host health', component: card('HostHealthCard'), cols: 12, rows: 1, control: toModels },
  { id: 'running-now', source: 'host', title: 'Running now', component: card('RunningNowCard'), cols: 4, rows: 1, control: () => toActivity() },
  { id: 'recent-runs', source: 'automations', title: 'Recent runs', component: card('RecentRunsCard'), cols: 8, rows: 1, control: toAutomations },
  { id: 'reminders', source: 'reminders', title: 'Reminders', component: card('RemindersCard'), cols: 4, rows: 1, control: toReminders },
  { id: 'newsroom-brief', source: 'newsroom', title: 'Brief', component: card('NewsroomBriefCard'), cols: 12, rows: 1, control: () => toNewsroom('briefs') },
  { id: 'newsroom-today', source: 'newsroom', title: 'Today', component: card('NewsroomTodayCard'), cols: 6, rows: 1, control: () => toNewsroom('briefs') },
  { id: 'newsroom-latest', source: 'newsroom', title: 'Newsroom', component: card('NewsroomLatestCard'), cols: 12, rows: 1, control: () => toNewsroom() },
  { id: 'network-overview', source: 'network', title: 'Network overview', component: card('NetworkOverviewCard'), cols: 12, rows: 1, control: () => toNetwork('modules') },
  { id: 'network-devices', source: 'network', title: 'Devices', component: card('NetworkDevicesCard'), cols: 12, rows: 1, control: () => toNetworkSettings('devices') },
  { id: 'network-findings', source: 'network', title: 'Findings', component: card('NetworkFindingsCard'), cols: 12, rows: 1, control: () => toNetwork('modules') },
  { id: 'network-audits', source: 'network', title: 'Audits', component: card('NetworkAuditsCard'), cols: 6, rows: 1, control: () => toNetworkSettings('scanning') },
  { id: 'network-timeline', source: 'network', title: ACTIVITY_TITLE, component: card('NetworkTimelineCard'), cols: 6, rows: 1, control: () => toNetwork('modules') },
  { id: 'network-traffic', source: 'network', title: 'Traffic', component: card('NetworkTrafficCard'), cols: 12, rows: 1, control: () => toNetworkSettings('helper') },
  { id: 'radio-around', source: 'radio', title: 'Around you', component: card('RadioAroundCard'), cols: 4, rows: 1, control: () => toRadio() },
  { id: 'radio-devices', source: 'radio', title: 'My devices', component: card('RadioMyDevicesCard'), cols: 6, rows: 1, control: () => toRadio() },
  { id: 'radio-sensors', source: 'radio', title: 'Radios on this computer', component: card('RadioSensorsCard'), cols: 6, rows: 1, control: () => toRadio('modules') },
  { id: 'radio-manage', source: 'radio', title: 'Claim and manage devices', component: card('RadioManageCard'), cols: 6, rows: 1, control: () => toRadio() },
  { id: 'radio-networks', source: 'radio', title: 'Your Wi-Fi networks', component: card('RadioNetworksCard'), cols: 6, rows: 1, control: () => toNetworkSettings('devices') },
  { id: 'radio-findings', source: 'radio', title: 'Things to look at', component: card('RadioFindingsCard'), cols: 12, rows: 1, control: () => toRadio() },
]

export const CARD_DEFS_BY_ID: ReadonlyMap<string, DashboardCardDef> = new Map(CARD_DEFS.map((d) => [d.id, d]))
export const KNOWN_CARD_IDS: ReadonlySet<string> = new Set(CARD_DEFS.map((d) => d.id))
