import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import NetworkOverviewPanel from '../../../components/netaudit/NetworkOverviewPanel.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'
import type { NetworkOverview } from '../../../types/netaudit'

const overview = (over: Partial<NetworkOverview> = {}): NetworkOverview => ({
  posture: 'quiet',
  devices: { known: 12, unnamed: 3, new: 0, active: 9 },
  findings: { scanned: true, scanned_at: '2026-10-30T02:00:00Z', open: 4, serious: 1, ports: 31, new: 2 },
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
    // The tile counts problems, as the Findings tab does, and says the open ports apart.
    expect(w.text()).toContain('Problems')
    expect(w.text()).toContain('1 serious · 31 open ports')
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

  it('offers one next step on the verdict card and says where it goes', async () => {
    const w = mount(NetworkOverviewPanel, { props: { overview: overview({ posture: 'attention', devices: { known: 12, unnamed: 3, new: 1, active: 9 } }), hasAudit: true } })
    const button = w.get('[data-test="next-action"]')
    expect(button.text()).toBe('Review devices')
    await button.trigger('click')
    expect(w.emitted('open-tab')).toEqual([['devices']])
  })

  it('offers nothing to do while the network is quiet', () => {
    const w = mount(NetworkOverviewPanel, { props: { overview: overview({ findings: { scanned: true, open: 1, serious: 0, new: 0 } }), hasAudit: true } })
    expect(w.find('[data-test="next-action"]').exists()).toBe(false)
  })

  it('reads the same in Simple and Advanced: the overview has one verdict and no detail to fold', () => {
    useNetauditViewMode().set('simple')
    const simple = mount(NetworkOverviewPanel, { props: { overview: overview(), hasAudit: true } }).text()
    useNetauditViewMode().set('advanced')
    const advanced = mount(NetworkOverviewPanel, { props: { overview: overview(), hasAudit: true } }).text()
    expect(advanced).toBe(simple)
  })

  it('says a check that never ran has not looked, instead of counting days from the year 1', () => {
    const NEVER = '0001-01-01T00:00:00Z'
    const w = mount(NetworkOverviewPanel, { props: { overview: overview({ presence: { last_check: NEVER }, findings: { scanned: true, scanned_at: NEVER, open: 0, serious: 0, new: 0 } }), hasAudit: true } })
    expect(w.text()).not.toMatch(/\d{3},\d{3}/)
    const values = w.findAll('[data-test="stat-value"]').map((v) => v.text())
    expect(values[2]).toBe('Never')
    expect(values[3]).toBe('–')
    expect(w.text()).toContain('has not looked yet')
  })

  it('draws a value that is words in body type and regular weight, not as a bold figure', () => {
    const w = mount(NetworkOverviewPanel, { props: { overview: overview(), hasAudit: true } })
    const values = w.findAll('[data-test="stat-value"]')
    expect(values[0]!.classes()).toContain('font-mono')
    for (const v of [values[2]!, values[3]!]) {
      expect(v.classes()).toContain('font-normal')
      expect(v.classes()).not.toContain('font-mono')
      expect(v.classes()).not.toContain('font-medium')
    }
  })

  it('offers to see the timeline, and says where to go without going there itself', async () => {
    const w = mount(NetworkOverviewPanel, { props: { overview: overview(), hasAudit: true } })
    await w.get('[data-test="see-all-activity"]').trigger('click')
    expect(w.emitted('open-tab')).toEqual([['timeline']])
  })

  it('shows the router figure only once the router has been counted', () => {
    const none = mount(NetworkOverviewPanel, { props: { overview: overview(), hasAudit: true } })
    expect(none.find('[data-test="router-stat"]').exists()).toBe(false)
    const day = (date: string, blocked?: number) => ({ date, devices_up: 1, devices: 1, unnamed: 0, findings: 0, serious: 0, blocked })
    const today = new Date()
    const iso = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`
    const some = mount(NetworkOverviewPanel, { props: { overview: overview({ series: [day('2026-11-01', 5), day(iso, 1204)] }), hasAudit: true } })
    const stat = some.get('[data-test="router-stat"]')
    expect(stat.text()).toContain('Refused at the router')
    expect(stat.text()).toContain('1,204')
  })
})

