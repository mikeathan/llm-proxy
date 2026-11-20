import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { computed, ref } from 'vue'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../router'
import type { NetworkOverview } from '../../../types/netaudit'

// The cards read one shared snapshot; here it is whatever the test puts in.
const overview = ref<NetworkOverview | null>(null)
const error = ref('')
const hasAudit = ref(true)
vi.mock('../../../composables/netaudit/useNetworkSnapshot', () => ({ useNetworkSnapshot: () => ({ overview, error, hasAudit: computed(() => hasAudit.value) }) }))
vi.mock('../../../composables/netaudit/useModuleSources', () => ({ useModuleSources: () => ({ modules: ref([]), load: vi.fn().mockResolvedValue(undefined) }) }))
vi.mock('../../../components/netaudit/FindingsReport.vue', () => ({ default: { template: '<div data-test="findings-report" />' } }))
const audit = ref<{ workspaceId: string; automation: string } | null>(null)
vi.mock('../../../composables/netaudit/useNetworkAudit', () => ({ useNetworkAudit: () => ({ audit, loaded: ref(true) }) }))
vi.mock('../../../services/stacks/stacksService', () => ({ StacksService: { stack: vi.fn().mockResolvedValue(null) } }))

import OverviewVerdict from '../../../components/netaudit/overview/OverviewVerdict.vue'
import OverviewFigures from '../../../components/netaudit/overview/OverviewFigures.vue'
import OverviewChanges from '../../../components/netaudit/overview/OverviewChanges.vue'
import NetworkOverviewCard from '../../../components/dashboard/cards/NetworkOverviewCard.vue'
import NetworkFindingsCard from '../../../components/dashboard/cards/NetworkFindingsCard.vue'

const make = (over: Partial<NetworkOverview> = {}): NetworkOverview => ({
  posture: 'quiet',
  devices: { known: 12, unnamed: 3, new: 0, active: 9 },
  findings: { scanned: true, scanned_at: '2026-10-30T02:00:00Z', open: 4, serious: 1, ports: 31, new: 2 },
  gateway_changed: false, shared_address: false, presence: { last_check: '2026-10-30T11:50:00Z' },
  series: [{ date: '2026-10-29', devices_up: 8, devices: 11, unnamed: 3, findings: 5, serious: 1, audited: true }, { date: '2026-10-30', devices_up: 9, devices: 12, unnamed: 3, findings: 4, serious: 1, audited: true }],
  recent: [{ kind: 'new_device', at: '2026-10-30T10:00:00Z', ip: '192.168.1.77' }], ...over,
})

const router = createAppRouter(createMemoryHistory())
const show = (card: object) => mount(card, { global: { plugins: [router] } })

beforeEach(() => {
  overview.value = make()
  error.value = ''
  hasAudit.value = true
})

describe('OverviewVerdict', () => {
  it('says the verdict and shows nothing to do while the network is quiet', () => {
    overview.value = make({ findings: { scanned: true, open: 1, serious: 0, new: 0 } })
    const w = show(OverviewVerdict)
    expect(w.text()).toContain('Quiet')
    expect(w.text()).toContain('Your network looks as you left it')
    expect(w.find('[data-test="next-action"]').exists()).toBe(false)
  })

  it('lists why something needs a look, and links the one next step to where it is done', () => {
    overview.value = make({ posture: 'attention', gateway_changed: true, devices: { known: 12, unnamed: 3, new: 1, active: 9 }, findings: { scanned: true, open: 2, serious: 0, new: 0 } })
    const w = show(OverviewVerdict)
    expect(w.text()).toContain('Something needs a look')
    expect(w.findAll('[aria-label="What needs a look"] li').map((l) => l.text())).toEqual(["The router's hardware address changed", '1 new device nobody has approved'])
    const next = w.get('[data-test="next-action"]')
    expect(next.text()).toBe('Review devices →')
    expect(next.attributes('href')).toBe('/dashboard?board=network-devices&card=network-devices')
  })

  it('says it has not looked yet instead of calling an unwatched network quiet', () => {
    overview.value = make({ posture: 'unknown', series: [], recent: [], devices: { known: 0, unnamed: 0, new: 0, active: 0 }, findings: { scanned: false, open: 0, serious: 0, new: 0 }, presence: {} })
    hasAudit.value = false
    expect(show(OverviewVerdict).text()).toContain('Not looked at yet')
  })

  it('shows why the new-device check cannot look, as an alert', () => {
    overview.value = make({ presence: { problem: 'only Linux can list devices' } })
    expect(show(OverviewVerdict).get('[role="alert"]').text()).toContain('only Linux can list devices')
  })

  it('says it is reading, and says when it could not', () => {
    overview.value = null
    expect(show(OverviewVerdict).text()).toContain('Reading the network')
    error.value = 'the server is down'
    expect(show(OverviewVerdict).text()).toContain('the server is down')
  })
})

describe('OverviewFigures', () => {
  it('shows the devices and problems with what they mean', () => {
    const w = show(OverviewFigures)
    const values = w.findAll('[data-test="stat-value"]').map((v) => v.text())
    expect(values[0]).toBe('12')
    expect(values[1]).toBe('4')
    expect(w.text()).toContain('9 seen in the last day · 3 unnamed')
    expect(w.text()).toContain('1 serious · 31 open ports')
  })

  it('does not show problems as zero when no audit is set up', () => {
    hasAudit.value = false
    const w = show(OverviewFigures)
    expect(w.findAll('[data-test="stat-value"]')[1]!.text()).toBe('–')
    expect(w.text()).toContain('No network audit is set up')
  })
})

describe('OverviewChanges', () => {
  it('lists what changed lately, and says when nothing did', () => {
    expect(show(OverviewChanges).text()).toContain('192.168.1.77 appeared on the network')
    overview.value = make({ recent: [] })
    expect(show(OverviewChanges).text()).toContain('Nothing has changed in the last two weeks.')
  })
})

describe('NetworkOverviewCard', () => {
  it('puts the verdict, the changes, the figures, the standing and the coverage in one card, and links to the rest of the activity', () => {
    const w = show(NetworkOverviewCard)
    for (const part of ['network-overview-verdict', 'network-overview-changes', 'network-overview-figures']) expect(w.find(`[data-test="${part}"]`).exists()).toBe(true)
    expect(w.get('[data-test="all-activity"]').attributes('href')).toBe('/dashboard?board=network-activity&card=network-timeline')
  })

  it('says the standing fills in after the first audit', () => {
    overview.value = make({ findings: { scanned: false, open: 0, serious: 0, new: 0 } })
    expect(show(NetworkOverviewCard).find('[data-test="network-standing-none"]').exists()).toBe(true)
  })

  it('shows only the latest few changes', () => {
    overview.value = make({ recent: Array.from({ length: 9 }, (_, i) => ({ kind: 'new_device' as const, at: '2026-10-30T10:00:00Z', ip: `192.168.1.${i + 10}` })) })
    expect(show(OverviewChanges).findAll('[aria-label="Recent changes"] li')).toHaveLength(5)
  })
})

describe('NetworkFindingsCard', () => {
  const path = { id: 'network|this-computer', entry: { kind: 'network' as const, text: 'Any device on your network' }, steps: [{ key: 'k', check: 'c', title: 'SSH accepts password login', severity: 'low' }], prize: { kind: 'computer' as const, where: 'this-computer' }, fix: { key: 'k', text: 'Turn it off.' }, breaks: 1 }

  it('offers the ways in as a second view only once an audit has run with the module on', async () => {
    audit.value = { workspaceId: 'w', automation: 'a' }
    overview.value = make({ attack_paths: [path], attack_paths_on: false })
    expect(show(NetworkFindingsCard).find('[data-test="findings-view"]').exists()).toBe(false)
    overview.value = make({ attack_paths: [path], attack_paths_on: true })
    const w = show(NetworkFindingsCard)
    expect(w.find('[data-test="findings-view"]').exists()).toBe(true)
    expect(w.find('[data-test="attack-paths"]').exists()).toBe(false)
    await w.findAll('[role="radio"]')[1]!.trigger('click')
    expect(w.find('[data-test="attack-paths"]').exists()).toBe(true)
  })
})
