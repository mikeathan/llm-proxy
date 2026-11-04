import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import TimelineTab from '../../../components/netaudit/TimelineTab.vue'
import { NetauditService } from '../../../services/netaudit/netauditService'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'
import { ACTIVITY_LIMIT_ADVANCED, ACTIVITY_LIMIT_SIMPLE } from '../../../constants/netaudit'
import type { ActivityRow } from '../../../types/netaudit'

const at = (h: number) => new Date(2026, 10, 4, h, 0, 0).toISOString()
const ROWS: ActivityRow[] = [
  { kind: 'related', group: 'alerts', at: at(14), mac: 'aa:bb:cc:00:00:09', ip: '192.168.1.77' },
  { kind: 'new_device', group: 'devices', at: at(13), mac: 'aa:bb:cc:00:00:09', ip: '192.168.1.77', name: 'Camera' },
  { kind: 'ports_changed', group: 'ports', at: at(12), day: true, opened: 2, closed: 0 },
]
// Every tab is unmounted after its test: a mounted one would react to the next test's change of view and read for real.
const mounted: VueWrapper[] = []
const mountTab = () => {
  const w = mount(TimelineTab, { props: { audit: { workspaceId: 'ws', automation: 'nightly' } } })
  mounted.push(w)
  return w
}

describe('TimelineTab', () => {
  afterEach(() => mounted.splice(0).forEach((w) => w.unmount()))
  beforeEach(() => {
    vi.restoreAllMocks()
    useNetauditViewMode().set('simple')
  })

  it('reads the timeline once, for the audit it was given, and lists it with the words composed from typed fields', async () => {
    const spy = vi.spyOn(NetauditService, 'activity').mockResolvedValue({ rows: ROWS })
    const w = mountTab()
    await flushPromises()
    expect(spy).toHaveBeenCalledTimes(1)
    expect(spy).toHaveBeenCalledWith({ workspaceId: 'ws', automation: 'nightly' }, ACTIVITY_LIMIT_SIMPLE)
    const text = w.text()
    expect(text).toContain('Camera appeared on the network')
    expect(text).toContain('2 ports opened')
    expect(text).toContain('Possibly related')
    expect(w.findAll('[data-test="activity-day"]')).toHaveLength(1)
  })

  it('shows addresses only in the Advanced view, and asks for more rows there', async () => {
    const spy = vi.spyOn(NetauditService, 'activity').mockResolvedValue({ rows: ROWS })
    const w = mountTab()
    await flushPromises()
    expect(w.find('[data-test="activity-address"]').exists()).toBe(false)
    useNetauditViewMode().set('advanced')
    await flushPromises()
    expect(spy).toHaveBeenLastCalledWith({ workspaceId: 'ws', automation: 'nightly' }, ACTIVITY_LIMIT_ADVANCED)
    expect(w.get('[data-test="activity-address"]').text()).toBe('aa:bb:cc:00:00:09 · 192.168.1.77')
  })

  it('filters in the browser, and says so when nothing of that kind happened', async () => {
    const spy = vi.spyOn(NetauditService, 'activity').mockResolvedValue({ rows: ROWS })
    const w = mountTab()
    await flushPromises()
    await w.get('[role="radio"][aria-checked="false"]:nth-of-type(3)').trigger('click') // Ports
    expect(w.findAll('li')).toHaveLength(1)
    await w.findAll('[role="radio"]').find((b) => b.text() === 'Findings')!.trigger('click')
    expect(w.text()).toContain('Nothing of this kind has happened lately.')
    expect(spy).toHaveBeenCalledTimes(1)
  })

  it('says a quiet network is quiet', async () => {
    vi.spyOn(NetauditService, 'activity').mockResolvedValue({ rows: [] })
    const w = mountTab()
    await flushPromises()
    expect(w.text()).toContain('Nothing has happened lately.')
  })

  it('says it could not read, and keeps what it had when a refresh fails', async () => {
    const spy = vi.spyOn(NetauditService, 'activity').mockRejectedValueOnce(new Error('down'))
    const w = mountTab()
    await flushPromises()
    expect(w.text()).toContain('The activity could not be read')
    spy.mockResolvedValueOnce({ rows: ROWS })
    await w.get('button:not([role])').trigger('click')
    await flushPromises()
    expect(w.text()).toContain('Camera appeared on the network')
    spy.mockRejectedValueOnce(new Error('down again'))
    await w.findAll('button').find((b) => b.text() === 'Refresh')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('Camera appeared on the network')
    expect(w.get('[role="alert"]').text()).toContain('down again')
  })

  it('drops an older answer that arrives after a newer one', async () => {
    let first!: (v: { rows: ActivityRow[] }) => void
    const spy = vi.spyOn(NetauditService, 'activity')
      .mockImplementationOnce(() => new Promise((resolve) => { first = resolve }))
      .mockResolvedValueOnce({ rows: [ROWS[1]!] })
    const w = mountTab()
    await w.findAll('button').find((b) => b.text() === 'Refresh')!.trigger('click')
    await flushPromises()
    first({ rows: [ROWS[2]!] })
    await flushPromises()
    expect(spy).toHaveBeenCalledTimes(2)
    expect(w.text()).toContain('Camera appeared on the network')
    expect(w.text()).not.toContain('2 ports opened')
  })

  it('shows a name the operator typed as text, never as markup', async () => {
    vi.spyOn(NetauditService, 'activity').mockResolvedValue({ rows: [{ kind: 'new_device', group: 'devices', at: at(13), name: '<img src=x onerror=alert(1)>' }] })
    const w = mountTab()
    await flushPromises()
    expect(w.find('img').exists()).toBe(false)
    expect(w.text()).toContain('<img src=x onerror=alert(1)> appeared on the network')
  })

  describe('Explain this week', () => {
    const explainButton = (w: VueWrapper) => w.get('[data-test="explain-button"]')

    it('is offered only when there is an audit to take the workspace from', async () => {
      vi.spyOn(NetauditService, 'activity').mockResolvedValue({ rows: ROWS })
      const w = mount(TimelineTab, { props: { audit: null } })
      mounted.push(w)
      await flushPromises()
      expect(w.find('[data-test="explain"]').exists()).toBe(false)
    })

    it('asks once per click, shows the answer as text with the note that says what the model saw', async () => {
      vi.spyOn(NetauditService, 'activity').mockResolvedValue({ rows: ROWS })
      const ask = vi.spyOn(NetauditService, 'explainTimeline').mockResolvedValue({ summary: 'A camera joined.\n<img src=x onerror=alert(1)>', model: 'qwen-local' })
      const w = mountTab()
      await flushPromises()
      await explainButton(w).trigger('click')
      await flushPromises()
      expect(ask).toHaveBeenCalledWith({ workspaceId: 'ws', automation: 'nightly' })
      const result = w.get('[data-test="explain-result"]')
      expect(result.text()).toContain('A camera joined.')
      expect(result.text()).toContain('<img src=x onerror=alert(1)>')
      expect(result.text()).toContain('not shown any device name or address')
      expect(result.text()).toContain('qwen-local')
      expect(w.find('img').exists()).toBe(false)
    })

    it('does not ask again while it is asking', async () => {
      vi.spyOn(NetauditService, 'activity').mockResolvedValue({ rows: ROWS })
      let finish!: (v: { summary: string }) => void
      const ask = vi.spyOn(NetauditService, 'explainTimeline').mockImplementation(() => new Promise((resolve) => { finish = resolve }))
      const w = mountTab()
      await flushPromises()
      await explainButton(w).trigger('click')
      await explainButton(w).trigger('click')
      expect(ask).toHaveBeenCalledTimes(1)
      expect(explainButton(w).text()).toContain('Writing the summary')
      finish({ summary: 'Quiet.' })
      await flushPromises()
      expect(w.get('[data-test="explain-result"]').text()).toContain('Quiet.')
    })

    it('shows the reason in the server\'s own words when it is refused', async () => {
      vi.spyOn(NetauditService, 'activity').mockResolvedValue({ rows: ROWS })
      vi.spyOn(NetauditService, 'explainTimeline').mockRejectedValue(new Error('The model for this workspace runs in the cloud, and network data may only go to a model on this machine.'))
      const w = mountTab()
      await flushPromises()
      await explainButton(w).trigger('click')
      await flushPromises()
      expect(w.get('[data-test="explain-error"]').text()).toContain('on this machine')
      expect(w.find('[data-test="explain-result"]').exists()).toBe(false)
    })

    it('says there is nothing to explain when the server found nothing from the week', async () => {
      vi.spyOn(NetauditService, 'activity').mockResolvedValue({ rows: ROWS })
      vi.spyOn(NetauditService, 'explainTimeline').mockResolvedValue({ summary: '' })
      const w = mountTab()
      await flushPromises()
      await explainButton(w).trigger('click')
      await flushPromises()
      expect(w.get('[data-test="explain-nothing"]').text()).toContain('nothing from the last week')
    })
  })

  describe('reading a long timeline', () => {
    const DAY = 86_400_000
    const daysAgo = (d: number, h = 10, m = 0) => new Date(new Date(Date.now() - d * DAY).setHours(h, m, 0, 0)).toISOString()
    const device = (n: number, d: number): ActivityRow => ({ kind: 'new_device', group: 'devices', at: daysAgo(d, 8 + n), name: `Dev${n}`, ip: `192.168.1.${n}`, mac: `aa:bb:cc:00:00:0${n}` })
    const LONG: ActivityRow[] = [
      { kind: 'alert', group: 'alerts', at: daysAgo(0, 11, 30), alert_kind: 'presence', text: 'Network audit: 1 new device: 192.168.1.9', count: 1 },
      { kind: 'new_device', group: 'devices', at: daysAgo(0, 11), name: 'Today device', ip: '192.168.1.9' },
      { kind: 'ports_changed', group: 'ports', at: daysAgo(1, 23), day: true, opened: 1, closed: 0 },
      { kind: 'address_changed', group: 'devices', at: daysAgo(2, 9), ip: '192.168.1.2' },
      device(1, 5), device(2, 5), device(3, 5), device(4, 5),
    ]
    const days = (w: VueWrapper) => w.findAll('[data-test="activity-day"]')

    beforeEach(() => vi.spyOn(NetauditService, 'activity').mockResolvedValue({ rows: LONG }))

    it('opens the latest two days and shows an older day as one line of counts until it is opened', async () => {
      const w = mountTab()
      await flushPromises()
      const all = days(w)
      expect(all).toHaveLength(4)
      expect(all[0]!.find('ul').exists()).toBe(true)
      expect(all[1]!.find('ul').exists()).toBe(true)
      expect(all[2]!.find('ul').exists()).toBe(false)
      expect(all[3]!.find('[data-test="day-summary"]').text()).toBe('4 new devices')
      expect(all[3]!.find('ul').exists()).toBe(false)
      await all[3]!.get('[data-test="day-toggle"]').trigger('click')
      expect(days(w)[3]!.find('ul').exists()).toBe(true)
      expect(days(w)[3]!.get('[data-test="day-toggle"]').attributes('aria-expanded')).toBe('true')
    })

    it('in Simple folds four new devices into one line and leaves out the alert that repeats a device', async () => {
      const w = mountTab()
      await flushPromises()
      await days(w)[3]!.get('[data-test="day-toggle"]').trigger('click')
      const last = days(w)[3]!
      expect(last.findAll('li')).toHaveLength(1)
      expect(last.text()).toContain('4 new devices appeared: Dev1, Dev2, Dev3 and 1 more')
      expect(w.text()).not.toContain('Network audit: 1 new device')
    })

    it('in Advanced lists every row, with the repeated alert too', async () => {
      useNetauditViewMode().set('advanced')
      const w = mountTab()
      await flushPromises()
      await days(w)[3]!.get('[data-test="day-toggle"]').trigger('click')
      expect(days(w)[3]!.findAll('li')).toHaveLength(4)
      expect(w.text()).toContain('Network audit: 1 new device')
    })

    it('opens every day when a filter is chosen, because the person is looking for something', async () => {
      const w = mountTab()
      await flushPromises()
      await w.findAll('[role="radio"]').find((b) => b.text() === 'Devices')!.trigger('click')
      expect(days(w).every((d) => d.find('ul').exists())).toBe(true)
    })
  })

  describe('the router card', () => {
    const router = { today: 1204, today_ports: [{ port: 22, count: 700 }, { port: 3306, count: 300 }], days: [{ date: '2026-11-03', total: 800 }, { date: '2026-11-04', total: 1204 }] }
    const blockRow: ActivityRow = { kind: 'router_blocks', group: 'router', at: at(12), day: true, count: 800, ports: [{ port: 22, count: 500 }] }

    it('is absent, and so is the Router filter, when the router has never been counted', async () => {
      vi.spyOn(NetauditService, 'activity').mockResolvedValue({ rows: ROWS })
      const w = mountTab()
      await flushPromises()
      expect(w.find('[data-test="router-card"]').exists()).toBe(false)
      expect(w.findAll('[role="radio"]').map((b) => b.text())).not.toContain('Router')
    })

    it('shows today\'s count, the busiest ports and what the number is, and offers the Router filter', async () => {
      vi.spyOn(NetauditService, 'activity').mockResolvedValue({ rows: [...ROWS, blockRow], router })
      const w = mountTab()
      await flushPromises()
      const card = w.get('[data-test="router-card"]')
      expect(card.text()).toContain('1,204')
      expect(card.text()).toContain('SSH (22) 700 · MySQL (3306) 300')
      expect(card.text()).toContain('cannot check')
      expect(w.findAll('[role="radio"]').map((b) => b.text())).toContain('Router')
      await w.findAll('[role="radio"]').find((b) => b.text() === 'Router')!.trigger('click')
      expect(w.findAll('li')).toHaveLength(1)
      expect(w.text()).toContain('The router refused 800 connection attempts from the internet, mostly to SSH (22)')
    })
  })
})

