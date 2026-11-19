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
vi.mock('../../../services/stacks/stacksService', () => ({ StacksService: { stack: vi.fn().mockResolvedValue(null) } }))

import NetworkPostureCard from '../../../components/dashboard/cards/NetworkPostureCard.vue'
import NetworkFiguresCard from '../../../components/dashboard/cards/NetworkFiguresCard.vue'
import NetworkChangesCard from '../../../components/dashboard/cards/NetworkChangesCard.vue'
import NetworkAttackPathsCard from '../../../components/dashboard/cards/NetworkAttackPathsCard.vue'
import NetworkStandingCard from '../../../components/dashboard/cards/NetworkStandingCard.vue'

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

describe('NetworkPostureCard', () => {
  it('says the verdict and shows nothing to do while the network is quiet', () => {
    overview.value = make({ findings: { scanned: true, open: 1, serious: 0, new: 0 } })
    const w = show(NetworkPostureCard)
    expect(w.text()).toContain('Quiet')
    expect(w.text()).toContain('Your network looks as you left it')
    expect(w.find('[data-test="next-action"]').exists()).toBe(false)
  })

  it('lists why something needs a look, and links the one next step to where it is done', () => {
    overview.value = make({ posture: 'attention', gateway_changed: true, devices: { known: 12, unnamed: 3, new: 1, active: 9 }, findings: { scanned: true, open: 2, serious: 0, new: 0 } })
    const w = show(NetworkPostureCard)
    expect(w.text()).toContain('Something needs a look')
    expect(w.findAll('[aria-label="What needs a look"] li').map((l) => l.text())).toEqual(["The router's hardware address changed", '1 new device nobody has approved'])
    const next = w.get('[data-test="next-action"]')
    expect(next.text()).toBe('Review devices →')
    expect(next.attributes('href')).toBe('/network/devices')
  })

  it('says it has not looked yet instead of calling an unwatched network quiet', () => {
    overview.value = make({ posture: 'unknown', series: [], recent: [], devices: { known: 0, unnamed: 0, new: 0, active: 0 }, findings: { scanned: false, open: 0, serious: 0, new: 0 }, presence: {} })
    hasAudit.value = false
    expect(show(NetworkPostureCard).text()).toContain('Not looked at yet')
  })

  it('shows why the new-device check cannot look, as an alert', () => {
    overview.value = make({ presence: { problem: 'only Linux can list devices' } })
    expect(show(NetworkPostureCard).get('[role="alert"]').text()).toContain('only Linux can list devices')
  })

  it('says it is reading, and says when it could not', () => {
    overview.value = null
    expect(show(NetworkPostureCard).text()).toContain('Reading the network')
    error.value = 'the server is down'
    expect(show(NetworkPostureCard).text()).toContain('the server is down')
  })
})

describe('NetworkFiguresCard', () => {
  it('shows the devices and problems with what they mean', () => {
    const w = show(NetworkFiguresCard)
    const values = w.findAll('[data-test="stat-value"]').map((v) => v.text())
    expect(values[0]).toBe('12')
    expect(values[1]).toBe('4')
    expect(w.text()).toContain('9 seen in the last day · 3 unnamed')
    expect(w.text()).toContain('1 serious · 31 open ports')
  })

  it('does not show problems as zero when no audit is set up', () => {
    hasAudit.value = false
    const w = show(NetworkFiguresCard)
    expect(w.findAll('[data-test="stat-value"]')[1]!.text()).toBe('–')
    expect(w.text()).toContain('No network audit is set up')
  })
})

describe('NetworkChangesCard', () => {
  it('lists what changed lately, and says when nothing did', () => {
    expect(show(NetworkChangesCard).text()).toContain('192.168.1.77 appeared on the network')
    overview.value = make({ recent: [] })
    expect(show(NetworkChangesCard).text()).toContain('Nothing has changed in the last two weeks.')
  })
})

describe('NetworkAttackPathsCard and NetworkStandingCard', () => {
  const path = { id: 'network|this-computer', entry: { kind: 'network' as const, text: 'Any device on your network' }, steps: [{ key: 'k', check: 'c', title: 'SSH accepts password login', severity: 'low' }], prize: { kind: 'computer' as const, where: 'this-computer' }, fix: { key: 'k', text: 'Turn it off.' }, breaks: 1 }

  it('shows the ways in only while the module is on and an audit has run', () => {
    overview.value = make({ attack_paths: [path], attack_paths_on: true })
    expect(show(NetworkAttackPathsCard).find('[data-test="attack-paths"]').exists()).toBe(true)
    overview.value = make({ attack_paths: [], attack_paths_on: false })
    const off = show(NetworkAttackPathsCard)
    expect(off.find('[data-test="attack-paths"]').exists()).toBe(false)
    expect(off.find('[data-test="network-attack-paths-off"]').exists()).toBe(true)
  })

  it('says the standing fills in after the first audit', () => {
    overview.value = make({ findings: { scanned: false, open: 0, serious: 0, new: 0 } })
    expect(show(NetworkStandingCard).find('[data-test="network-standing-none"]').exists()).toBe(true)
  })
})
