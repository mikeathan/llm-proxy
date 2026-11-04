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
})
