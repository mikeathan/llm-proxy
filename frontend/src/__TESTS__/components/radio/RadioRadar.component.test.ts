import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RadioLive, RadioLiveDevice } from '../../../types/radio'

const service = vi.hoisted(() => ({ live: vi.fn(), calibrate: vi.fn(), track: vi.fn(), airwaves: vi.fn(), markMine: vi.fn(), listening: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))
const push = vi.hoisted(() => vi.fn())
vi.mock('vue-router', async (orig) => ({ ...(await orig<typeof import('vue-router')>()), useRouter: () => ({ push }) }))

import RadioRadar from '../../../components/radio/RadioRadar.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'

const dev = (over: Partial<RadioLiveDevice>): RadioLiveDevice => ({
  id: 'aa', mine: false, class: 'neighbour', dbm: -70, near_m: 5, far_m: 20, ring: 'house', angle: 90, trend: 'steady', channel: 6, heard: true, ...over,
})
const view = (devices: RadioLiveDevice[], over: Partial<RadioLive['model']> = {}): RadioLive => ({
  listening: true, cycles: 4, model: { ref_24_dbm: -40, ref_5_dbm: -47, calibrated: false, provisional: true, ...over }, devices, rings: {}, cycle_seconds: 300, last_cycle_at: 0, listened_cycles: 5, min_listened_cycles: 12,
})

beforeEach(() => {
  vi.clearAllMocks()
  useNetauditViewMode().set('simple')
  service.listening.mockResolvedValue({ listening: false, cycles: 0, frames: 0, windows: [], plan: { channels: [1], window_seconds: 20, cycle_seconds: 300 } })
  service.live.mockResolvedValue(view([dev({ id: 'far1', near_m: 8, far_m: 40, ring: 'nearby' }), dev({ id: 'mine1', mine: true, label: 'my phone', class: 'personal', near_m: 1.2, far_m: 3 }), dev({ id: 'close1', near_m: 2, far_m: 6, trend: 'closer' })]))
  service.calibrate.mockResolvedValue({ calibrated: true })
  service.airwaves.mockResolvedValue({ channels: [], classes: [], routers: [], near_unknown: 0, listened_cycles: 5 })
  service.markMine.mockResolvedValue({ label: 'x' })
  globalThis.localStorage?.clear?.()
  service.track.mockResolvedValue({ id: 'close1', mine: false, trend: 'closer', stored: false, now: 2000, points: [
    { at: 1000, dbm: -80, near_m: 10, far_m: 40, run: 0 }, { at: 1300, dbm: -70, near_m: 5, far_m: 20, run: 0 },
    { at: 1900, dbm: -60, near_m: 2, far_m: 6, run: 1 }, { at: 2000, dbm: -58, near_m: 2, far_m: 5, run: 1 },
  ] })
})

// Every wrapper is unmounted after its test: the radar polls on timers, and one left mounted ticks after the environment is torn down
// (`document is not defined`), an uncaught error that fails the CI run although every test passed.
const mounted: Array<{ unmount: () => void }> = []
afterEach(() => { mounted.splice(0).forEach((w) => w.unmount()) })

async function show() {
  const w = mount(RadioRadar, { global: { stubs: { RouterLink: { template: '<a data-test="radio-status-start"><slot /></a>' } } } })
  mounted.push(w)
  await flushPromises()
  return w
}

describe('RadioRadar', () => {
  it('zooms with the buttons and the wheel, and goes back to fitting everything', async () => {
    const w = await show()
    const edge = () => Number(w.get('[data-test="radio-radar-svg"]').attributes('data-edge'))
    const fitted = edge()
    expect(fitted).toBe(10) // framed on the typical device (median 3.5 m), not the far one at 18 m
    expect(w.findAll('[data-test="radio-dot"][data-beyond="1"]').map((d) => d.attributes('data-id'))).toEqual(['far1']) // on the rim, faded, and counted
    expect(w.get('[data-test="radio-beyond"]').text()).toBe('1 beyond the edge')
    await w.get('[data-test="radio-zoom-in"]').trigger('click')
    expect(edge()).toBe(5)
    await w.get('[data-test="radio-radar-svg"]').trigger('wheel', { deltaY: 100 })
    expect(edge()).toBe(10)
    await w.get('[data-test="radio-zoom-out"]').trigger('click')
    expect(edge()).toBe(20)
    expect(w.find('[data-test="radio-beyond"]').exists()).toBe(false)
    await w.get('[data-test="radio-zoom-in"]').trigger('click')
    await w.get('[data-test="radio-zoom-reset"]').trigger('click')
    expect(edge()).toBe(fitted)
  })

  it('draws one dot per device and labels only the owner’s device, by name', async () => {
    const w = await show()
    expect(w.get('[data-test="radio-radar-summary"]').text()).toBe('3 devices heard now, 1 of them yours')
    expect(w.findAll('[data-test="radio-dot"]')).toHaveLength(3)
    expect(w.findAll('[data-test="radio-dot-label"]').map((l) => l.text())).toEqual(['my phone · 1.2–3 m'])
  })

  it('lists the owner’s devices first and then the closest, with a range and a trend, never a point', async () => {
    const w = await show()
    const rows = w.findAll('[data-test="radio-radar-row"]')
    expect(rows.map((r) => r.text())).toEqual([expect.stringContaining('my phone'), expect.stringContaining('close1'), expect.stringContaining('far1')])
    expect(rows[0]!.text()).toContain('1.2–3 m')
    expect(rows[1]!.text()).toContain('getting closer')
  })

  it('says which box of your mesh a device is joined to, and that it is moving now', async () => {
    service.live.mockResolvedValue(view([dev({ id: 'mine1', mine: true, label: 'my phone', class: 'personal', moving: true, router: { box: '7726d4bd', main: true, band: '5G', dbm: -57, at: 1 } })]))
    const w = await show()
    expect(w.get('[data-test="radio-row-router"]').text()).toBe('via the main router')
    expect(w.get('[data-test="radio-row-moving"]').text()).toBe('moving now')
  })

  it('opens a device’s own page from its row or its dot, with no card in between', async () => {
    const w = await show()
    await w.findAll('[data-test="radio-radar-row-button"]')[1]!.trigger('click')
    expect(push).toHaveBeenLastCalledWith(expect.objectContaining({ params: { id: 'close1' } }))
    await w.get('[data-test="radio-dot"][data-id="far1"]').trigger('click')
    expect(push).toHaveBeenLastCalledWith(expect.objectContaining({ params: { id: 'far1' } }))
    expect(w.find('[data-test="radio-radar-chosen"]').exists()).toBe(false)
    expect(w.findAll('[data-test="radio-dot"]')).toHaveLength(3) // the picture stays whole
  })

  it('says no trend until it has enough readings, rather than calling an old device new', async () => {
    service.live.mockResolvedValue(view([dev({ id: 'ap1', trend: 'unknown' })]))
    const w = await show()
    expect(w.get('[data-test="radio-radar-row"]').text()).not.toContain('too new to tell')
  })

  it('says the distances are a starting guess until calibrated, and that direction is not measured', async () => {
    let w = await show()
    expect(w.get('[data-test="radio-radar-model"]').text()).toContain('starting guess')
    expect(w.text()).toContain('Which way a device lies is not measured')
    service.live.mockResolvedValue(view([dev({})], { calibrated: true, provisional: false }))
    w = await show()
    expect(w.get('[data-test="radio-radar-model"]').text()).toContain('Calibrated against your device')
  })

  it('says nothing is heard when the picture is empty, and shows a failed read', async () => {
    service.live.mockResolvedValue(view([]))
    let w = await show()
    expect(w.get('[data-test="radio-radar-empty"]').text()).toContain('Start listening')
    service.live.mockRejectedValue(new Error('down'))
    w = await show()
    expect(w.get('[data-test="radio-radar-error"]').text()).toBe('down')
  })

  it('draws the devices usually here as hollow dots and lists them only on request', async () => {
    service.live.mockResolvedValue(view([dev({ id: 'now1', near_m: 3, far_m: 9 }), dev({ id: 'usual1', usual: true, heard: false, mine: true, label: 'my laptop', near_m: 2, far_m: 5, trend: 'unknown' }), dev({ id: 'usual2', usual: true, heard: false, near_m: 9, far_m: 30, trend: 'unknown' })]))
    const w = await show()
    expect(w.get('[data-test="radio-radar-summary"]').text()).toBe('1 device heard now')
    expect(w.findAll('[data-test="radio-dot"]')).toHaveLength(1) // the ones usually here stay out until asked for
    expect(w.get('[data-test="radio-radar-usual"]').text()).toContain('2 devices are usually here and not heard now.')
    expect(w.get('[data-test="radio-radar-legend"]').text()).toContain('not where they were at any moment')
    expect(w.findAll('[data-test="radio-radar-row"]')).toHaveLength(1)
    await w.get('[data-test="radio-usual-toggle"]').trigger('click')
    expect(w.findAll('[data-test="radio-dot"]')).toHaveLength(3)
    expect(w.findAll('[data-test="radio-dot"][data-usual="1"]')).toHaveLength(2)
    const rows = w.findAll('[data-test="radio-radar-row"]')
    expect(rows).toHaveLength(3)
    expect(rows[1]!.text()).toContain('my laptop')
    expect(rows[1]!.text()).toContain('usually here')
    expect(rows[1]!.text()).not.toContain('too new to tell')
  })

  it('rings a device that is far from its own usual, and says by how much and which way', async () => {
    service.live.mockResolvedValue(view([dev({ id: 'moved', unusual: true, usual_dbm: -80, shift_db: 30, dbm: -50 }), dev({ id: 'calm', usual_dbm: -70, shift_db: 1 })]))
    const w = await show()
    expect(w.findAll('[data-test="radio-dot-unusual"]')).toHaveLength(1)
    expect(w.get('[data-test="radio-radar-unusual"]').text()).toBe('1 is unlike its usual.')
    expect(w.get('[data-tag="radio-row-unusual"]').text()).toBe('30 dB stronger than its usual -80 dBm: closer than usual')
  })

  it('filters the one list with chips that carry counts', async () => {
    const w = await show()
    expect(w.findAll('[data-test="radio-chip"]').map((c) => c.text())).toEqual(['All3', 'Yours1', 'Outside2'])
    await w.get('[data-filter="mine"]').trigger('click')
    expect(w.findAll('[data-test="radio-radar-row"]').map((r) => r.text())).toEqual([expect.stringContaining('my phone')])
    await w.get('[data-filter="neighbour"]').trigger('click')
    expect(w.findAll('[data-test="radio-radar-row"]')).toHaveLength(2)
  })

  it('says whether it is live: the time to the next reading and how far the sorting has got', async () => {
    service.live.mockResolvedValue({ ...view([dev({ id: 'x1' })]), last_cycle_at: Math.floor(Date.now() / 1000) - 100, cycle_seconds: 300 })
    const w = await show()
    expect(w.get('[data-test="radio-status-next"]').text()).toMatch(/^Listening\. Next reading in 3:\d\d$/)
    expect(w.get('[data-test="radio-status-sorting"]').text()).toBe('Sorting starts after 12 cycles (5 done)')
  })

  it('says it is not listening, and shows the way to start', async () => {
    service.live.mockResolvedValue({ ...view([dev({ id: 'u1', usual: true, heard: false })]), listening: false })
    const w = await show()
    expect(w.get('[data-test="radio-status-idle"]').text()).toContain('Not listening')
    expect(w.find('[data-test="radio-status-start"]').exists()).toBe(true)
  })

  it('says a watch was cut short by a restart, how long it had left, and sends the owner to resume it', async () => {
    service.live.mockResolvedValue({ ...view([dev({ id: 'u1', usual: true, heard: false })]), listening: false })
    service.listening.mockResolvedValue({ listening: false, cycles: 0, frames: 0, windows: [], plan: { channels: [1], window_seconds: 20, cycle_seconds: 300 }, resume: { adapter: 'wlx00', until: '2026-11-20T12:00:00Z', hours_left: 42, why: 'stack_stopped' } })
    const w = await show()
    expect(w.get('[data-test="radio-status-stopped"]').text()).toBe('The service restarted, so the watch stopped. 42 hours were left.')
    expect(w.get('[data-test="radio-status-resume"]').text()).toBe('Resume listening')
    expect(w.find('[data-test="radio-status-idle"]').exists()).toBe(false)
  })

  it('shows twelve rows and offers the rest', async () => {
    service.live.mockResolvedValue(view(Array.from({ length: 15 }, (_, i) => dev({ id: `d${i}`, near_m: 5 + i, far_m: 20 + i }))))
    const w = await show()
    expect(w.findAll('[data-test="radio-radar-row"]')).toHaveLength(12)
    await w.get('[data-test="radio-list-more"]').trigger('click')
    expect(w.findAll('[data-test="radio-radar-row"]')).toHaveLength(15)
  })

  it('gives a dot a tail and a pulse only while it is heard, and never an address', async () => {
    service.live.mockResolvedValue(view([dev({ id: 'h1', tail_m: [9, 8, 7] }), dev({ id: 'q1', heard: false, tail_m: [9, 8] })]))
    const w = await show()
    expect(w.findAll('[data-test="radio-dot-tail"]')).toHaveLength(3)
    expect(w.findAll('[data-test="radio-dot-pulse"]')).toHaveLength(1)
    expect(w.html()).not.toMatch(/([0-9a-f]{2}:){5}[0-9a-f]{2}/i)
  })

  describe('views, narrowing and naming', () => {
    const detail = (id: string, over: Record<string, unknown> = {}) => ({ id, class: 'infrastructure' as const, why: 'always_here_steady_signal', signal: 'strong' as const, first_seen: '2026-11-01', last_seen: '2026-11-15', cycles_heard: 50, need_heard: 12, retry_percent: 0, channels: [60], random_address: false, joined_own: false, ...over })
    const crowd = () => view([
      dev({ id: 'a1', class: 'infrastructure', channel: 60, near_m: 2, far_m: 6, last_at: 1, detail: detail('a1', { maker: 'TP-Link', wifi_generation: 5 }) }),
      dev({ id: 'b2', class: 'neighbour', channel: 60, near_m: 8, far_m: 30, last_at: 1, detail: detail('b2', { class: 'neighbour' }) }),
      dev({ id: 'c3', class: 'neighbour', channel: 11, near_m: 9, far_m: 33, unusual: true, usual_dbm: -80, shift_db: 20, last_at: 1, detail: detail('c3', { class: 'neighbour', random_address: true }) }),
    ])

    it('narrows the radar and the list together when a chip is chosen', async () => {
      service.live.mockResolvedValue(crowd())
      const w = await show()
      expect(w.findAll('[data-test="radio-dot"]')).toHaveLength(3)
      await w.get('[data-filter="infrastructure"]').trigger('click')
      expect(w.findAll('[data-test="radio-dot"]').map((d) => d.attributes('data-id'))).toEqual(['a1'])
      expect(w.findAll('[data-test="radio-radar-row"]')).toHaveLength(1)
      await w.get('[data-filter="unusual"]').trigger('click')
      expect(w.findAll('[data-test="radio-dot"]').map((d) => d.attributes('data-id'))).toEqual(['c3'])
    })

    it('opens a device from the list of what changed', async () => {
      service.live.mockResolvedValue(crowd())
      const w = await show()
      await w.get('[data-test="radio-view-select"]').setValue('changes')
      await w.get('[data-test="radio-change"]').trigger('click')
      expect(push).toHaveBeenLastCalledWith(expect.objectContaining({ params: { id: 'c3' } }))
    })

    it('shows each device by its maker, with what it is, Wi-Fi generation and channel, and never titles one by its short id', async () => {
      service.live.mockResolvedValue(crowd())
      const w = await show()
      const titles = w.findAll('[data-test="radio-row-title"]').map((t) => t.text())
      expect(titles).toEqual(['TP-Link', 'Unknown device', 'Device (private address)'])
      expect(w.findAll('[data-test="radio-row-facts"]')[0]!.text()).toBe('Access point · Wi-Fi 5 · channel 60, 5 GHz')
      expect(w.findAll('[data-test="radio-row-facts"]')[2]!.text()).toContain('Random address')
    })

    it('labels every dot with its range, and draws a line from the centre, only when asked', async () => {
      service.live.mockResolvedValue(crowd())
      const w = await show()
      expect(w.findAll('[data-test="radio-dot-label"]')).toHaveLength(0)
      expect(w.findAll('[data-test="radio-dot-spoke"]')).toHaveLength(0)
      await w.get('[data-test="radio-toggle-range"] input').setValue(true)
      await w.get('[data-test="radio-toggle-spoke"] input').setValue(true)
      const labels = w.findAll('[data-test="radio-dot-label"]').length
      expect(labels).toBeGreaterThan(0) // a label that would overlap one already drawn is left out
      expect(labels).toBeLessThanOrEqual(3)
      expect(w.findAll('[data-test="radio-dot-spoke"]')).toHaveLength(3)
    })

    it('colours by whose a device is, and by its provisional kind with a note when asked', async () => {
      service.live.mockResolvedValue(crowd())
      const w = await show()
      expect(w.find('[data-test="radio-own-legend"]').exists()).toBe(true)
      expect(w.find('[data-test="radio-legend"]').exists()).toBe(false)
      await w.get('[data-test="radio-toggle-colours"] input').setValue(true)
      expect(w.get('[data-test="radio-colours-note"]').text()).toContain('provisional')
      expect(w.find('[data-test="radio-legend"]').exists()).toBe(true)
      await w.get('[data-test="radio-toggle-colours"] input').setValue(false)
      expect(w.find('[data-test="radio-legend"]').exists()).toBe(false)
    })

    it('switches between the three views from one drop-down', async () => {
      service.live.mockResolvedValue(crowd())
      const w = await show()
      expect(w.find('[data-test="radio-view-radar"]').exists()).toBe(true)
      await w.get('[data-test="radio-view-select"]').setValue('changes')
      expect(w.find('[data-test="radio-view-changes"]').exists()).toBe(true)
      expect(w.findAll('[data-test="radio-change"]')).toHaveLength(1) // c3 is unusual; nothing is new today or missing
      await w.get('[data-test="radio-view-select"]').setValue('channels')
      expect(w.find('[data-test="radio-view-channels"]').exists()).toBe(true)
    })

    it('by channel: a pick narrows the radar and the list, and marks your own network on its channel', async () => {
      service.live.mockResolvedValue(crowd())
      service.airwaves.mockResolvedValue({ channels: [], classes: [], routers: [{ name: 'netsink', channel: 60 }], near_unknown: 0, listened_cycles: 5 })
      const w = await show()
      await w.get('[data-test="radio-view-select"]').setValue('channels')
      expect(w.get('[data-test="radio-channels-headline"]').text()).toBe('netsink is on channel 60 with 2 devices. Channel 11 is the quietest.')
      expect(w.get('[data-channel="60"] [data-test="radio-channel-mine"]').text()).toBe('your network')
      await w.get('[data-channel="11"]').trigger('click')
      expect(w.findAll('[data-test="radio-dot"]').map((d) => d.attributes('data-id'))).toEqual(['c3'])
      expect(w.findAll('[data-test="radio-radar-row"]')).toHaveLength(1)
      await w.get('[data-test="radio-channel-clear"]').trigger('click')
      expect(w.findAll('[data-test="radio-dot"]')).toHaveLength(3)
    })

    it('finds my device: lists the strongest devices heard now and names the chosen one', async () => {
      service.live.mockResolvedValue(crowd())
      const w = await show()
      await w.get('[data-test="radio-find-open"]').trigger('click')
      const found = w.findAll('[data-test="radio-find-candidate"]')
      expect(found).toHaveLength(3)
      await found[0]!.get('button').trigger('click')
      await w.get('[data-test="radio-find-input"]').setValue('my tablet')
      await w.get('[data-test="radio-find-save"]').trigger('submit')
      await flushPromises()
      expect(service.markMine).toHaveBeenCalledTimes(1)
      expect(service.markMine.mock.calls[0]![1]).toBe('my tablet')
    })
  })
})
