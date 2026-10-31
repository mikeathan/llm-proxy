import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import NetworkOverviewPanel from '../../../components/netaudit/NetworkOverviewPanel.vue'
import type { NetworkOverview } from '../../../types/netaudit'

const overview = (over: Partial<NetworkOverview> = {}): NetworkOverview => ({
  posture: 'quiet',
  devices: { known: 12, unnamed: 3, new: 0, active: 9 },
  findings: { scanned: true, scanned_at: '2026-10-30T02:00:00Z', open: 4, serious: 1, new: 2 },
  gateway_changed: false, shared_address: false, presence: { last_check: '2026-10-30T11:50:00Z' },
  series: [{ date: '2026-10-29', devices_up: 8, devices: 11, unnamed: 3, findings: 5, serious: 1, audited: true }, { date: '2026-10-30', devices_up: 9, devices: 12, unnamed: 3, findings: 4, serious: 1, audited: true }],
  recent: [{ kind: 'new_device', at: '2026-10-30T10:00:00Z', ip: '192.168.1.77' }], ...over,
})

describe('NetworkOverviewPanel', () => {
  it('shows a quiet verdict, the figures and what changed', () => {
    const w = mount(NetworkOverviewPanel, { props: { overview: overview(), hasAudit: true } })
    expect(w.get('[data-test="posture"]').text()).toContain('Your network looks as you left it')
    const values = w.findAll('[data-test="stat-value"]').map((v) => v.text())
    expect(values[0]).toBe('12')
    expect(values[1]).toBe('4')
    expect(w.text()).toContain('9 seen in the last day · 3 unnamed')
    expect(w.text()).toContain('1 serious · 2 new')
    expect(w.text()).toContain('192.168.1.77 appeared on the network')
  })

  it('lists why something needs a look', () => {
    const w = mount(NetworkOverviewPanel, { props: { overview: overview({ posture: 'attention', gateway_changed: true, devices: { known: 12, unnamed: 3, new: 1, active: 9 }, findings: { scanned: true, open: 2, serious: 0, new: 0 } }), hasAudit: true } })
    expect(w.get('[data-test="posture"]').text()).toContain('Something needs a look')
    const reasons = w.findAll('[aria-label="What needs a look"] li').map((l) => l.text())
    expect(reasons).toEqual(["The router's hardware address changed", '1 new device nobody has approved'])
  })

  it('does not show findings as zero when there is no audit', () => {
    const w = mount(NetworkOverviewPanel, { props: { overview: overview(), hasAudit: false } })
    expect(w.findAll('[data-test="stat-value"]')[1]!.text()).toBe('–')
    expect(w.text()).toContain('No network audit is set up')
  })

  it('says it has not looked yet instead of calling an unwatched network quiet', () => {
    const w = mount(NetworkOverviewPanel, { props: { overview: overview({ posture: 'unknown', series: [], recent: [], devices: { known: 0, unnamed: 0, new: 0, active: 0 }, findings: { scanned: false, open: 0, serious: 0, new: 0 }, presence: {} }), hasAudit: false } })
    expect(w.get('[data-test="posture"]').text()).toContain('Not looked at yet')
    expect(w.text()).toContain('Nothing has changed in the last two weeks.')
  })

  it('shows why the new-device check cannot look, as an alert', () => {
    const w = mount(NetworkOverviewPanel, { props: { overview: overview({ presence: { problem: 'only Linux can list devices' } }), hasAudit: true } })
    expect(w.get('[role="alert"]').text()).toContain('only Linux can list devices')
  })
})
