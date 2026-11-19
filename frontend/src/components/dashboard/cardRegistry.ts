import { defineAsyncComponent } from 'vue'
import { toActivity, toAutomations, toModels, toNetwork, toRadio, toReminders } from '../../router/routes'
import type { DashboardCardDef } from '../../types/dashboard'

// The catalogue of dashboard cards: one entry per card, defined once. A board only places them, so the same card can sit on Home and on its
// stack's board. Each entry says where the card comes from, its default size and which control page its arrow opens. Card components are
// loaded when first shown, so a board costs only the cards on it.
const card = (name: string) => defineAsyncComponent(() => import(`./cards/${name}.vue`))

export const CARD_DEFS: readonly DashboardCardDef[] = [
  { id: 'host-health', source: 'host', title: 'Host health', component: card('HostHealthCard'), cols: 12, rows: 1, control: toModels },
  { id: 'running-now', source: 'host', title: 'Running now', component: card('RunningNowCard'), cols: 4, rows: 1, control: () => toActivity() },
  { id: 'recent-runs', source: 'automations', title: 'Recent runs', component: card('RecentRunsCard'), cols: 8, rows: 1, control: toAutomations },
  { id: 'reminders', source: 'reminders', title: 'Reminders', component: card('RemindersCard'), cols: 4, rows: 1, control: toReminders },
  { id: 'network-posture', source: 'network', title: 'Network posture', component: card('NetworkPostureCard'), cols: 4, rows: 1, control: () => toNetwork('findings') },
  { id: 'network-changes', source: 'network', title: 'What changed', component: card('NetworkChangesCard'), cols: 4, rows: 1, control: () => toNetwork('timeline') },
  { id: 'network-figures', source: 'network', title: 'Network figures', component: card('NetworkFiguresCard'), cols: 12, rows: 1, control: () => toNetwork('devices') },
  { id: 'network-coverage', source: 'network', title: 'What is watching', component: card('NetworkCoverageCard'), cols: 12, rows: 1, control: () => toNetwork('modules') },
  { id: 'network-attack-paths', source: 'network', title: 'Ways in', component: card('NetworkAttackPathsCard'), cols: 6, rows: 1, control: () => toNetwork('findings') },
  { id: 'network-standing', source: 'network', title: 'Are we getting better', component: card('NetworkStandingCard'), cols: 6, rows: 1, control: () => toNetwork('findings') },
  { id: 'radio-around', source: 'radio', title: 'Around you', component: card('RadioAroundCard'), cols: 4, rows: 1, control: () => toRadio() },
  { id: 'radio-findings', source: 'radio', title: 'Things to look at', component: card('RadioFindingsCard'), cols: 12, rows: 1, control: () => toRadio() },
]

export const CARD_DEFS_BY_ID: ReadonlyMap<string, DashboardCardDef> = new Map(CARD_DEFS.map((d) => [d.id, d]))
export const KNOWN_CARD_IDS: ReadonlySet<string> = new Set(CARD_DEFS.map((d) => d.id))
