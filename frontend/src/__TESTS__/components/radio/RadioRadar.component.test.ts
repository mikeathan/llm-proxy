import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RadioLive, RadioLiveDevice } from '../../../types/radio'

const service = vi.hoisted(() => ({ live: vi.fn(), calibrate: vi.fn(), track: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))

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
  service.live.mockResolvedValue(view([dev({ id: 'far1', near_m: 8, far_m: 40, ring: 'nearby' }), dev({ id: 'mine1', mine: true, label: 'my phone', class: 'personal', near_m: 1.2, far_m: 3 }), dev({ id: 'close1', near_m: 2, far_m: 6, trend: 'closer' })]))
  service.calibrate.mockResolvedValue({ calibrated: true })
  service.track.mockResolvedValue({ id: 'close1', mine: false, trend: 'closer', stored: false, now: 2000, points: [
    { at: 1000, dbm: -80, near_m: 10, far_m: 40, run: 0 }, { at: 1300, dbm: -70, near_m: 5, far_m: 20, run: 0 },
    { at: 1900, dbm: -60, near_m: 2, far_m: 6, run: 1 }, { at: 2000, dbm: -58, near_m: 2, far_m: 5, run: 1 },
  ] })
})

async function show() {
  const w = mount(RadioRadar, { global: { stubs: { RouterLink: { template: '<a data-test="radio-status-start"><slot /></a>' } } } })
  await flushPromises()
  return w
}

describe('RadioRadar', () => {
  it('draws one dot per device and labels only the owner’s device, by name', async () => {
    const w = await show()
    expect(w.get('[data-test="radio-radar-summary"]').text()).toBe('3 devices heard now, 1 of them yours')
    expect(w.findAll('[data-test="radio-dot"]')).toHaveLength(3)
    expect(w.findAll('[data-test="radio-dot-label"]').map((l) => l.text())).toEqual(['my phone'])
  })

  it('lists the owner’s devices first and then the closest, with a range and a trend, never a point', async () => {
    const w = await show()
    const rows = w.findAll('[data-test="radio-radar-row"]')
    expect(rows.map((r) => r.text())).toEqual([expect.stringContaining('my phone'), expect.stringContaining('close1'), expect.stringContaining('far1')])
    expect(rows[0]!.text()).toContain('1.2–3 m')
    expect(rows[1]!.text()).toContain('getting closer')
  })

  it('labels a neighbour only once it is chosen, and offers calibration only for the owner’s device', async () => {
    const w = await show()
    await w.findAll('[data-test="radio-radar-row"]')[1]!.get('button').trigger('click')
    expect(w.findAll('[data-test="radio-dot-label"]').map((l) => l.text())).toEqual(['my phone', 'close1'])
    expect(w.find('[data-test="radio-calibrate"]').exists()).toBe(false)
    await w.findAll('[data-test="radio-radar-row"]')[0]!.get('button').trigger('click')
    await w.get('[data-test="radio-calibrate"]').trigger('click')
    await flushPromises()
    expect(service.calibrate).toHaveBeenCalledWith('mine1')
    expect(service.live).toHaveBeenCalledTimes(2) // reloaded to show the new distances
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

  it('draws the devices usually here as hollow dots, lists them only on request, and never offers to calibrate from one', async () => {
    service.live.mockResolvedValue(view([dev({ id: 'now1', near_m: 3, far_m: 9 }), dev({ id: 'usual1', usual: true, heard: false, mine: true, label: 'my laptop', near_m: 2, far_m: 5, trend: 'unknown' }), dev({ id: 'usual2', usual: true, heard: false, near_m: 9, far_m: 30, trend: 'unknown' })]))
    const w = await show()
    expect(w.get('[data-test="radio-radar-summary"]').text()).toBe('1 device heard now')
    expect(w.findAll('[data-test="radio-dot"]')).toHaveLength(3)
    expect(w.findAll('[data-test="radio-dot"][data-usual="1"]')).toHaveLength(2)
    expect(w.get('[data-test="radio-radar-usual"]').text()).toContain('2 devices are usually here and not heard now.')
    expect(w.get('[data-test="radio-radar-legend"]').text()).toContain('not where they were at any moment')
    expect(w.findAll('[data-test="radio-radar-row"]')).toHaveLength(1)
    await w.get('[data-test="radio-usual-toggle"]').trigger('click')
    const rows = w.findAll('[data-test="radio-radar-row"]')
    expect(rows).toHaveLength(3)
    expect(rows[1]!.text()).toContain('my laptop')
    expect(rows[1]!.text()).toContain('usually here')
    expect(rows[1]!.text()).not.toContain('too new to tell')
    await rows[1]!.get('button').trigger('click')
    expect(w.get('[data-test="radio-radar-chosen"]').text()).toContain('usually')
    expect(w.find('[data-test="radio-calibrate"]').exists()).toBe(false)
  })

  it('rings a device that is far from its own usual, and says by how much and which way', async () => {
    service.live.mockResolvedValue(view([dev({ id: 'moved', unusual: true, usual_dbm: -80, shift_db: 30, dbm: -50 }), dev({ id: 'calm', usual_dbm: -70, shift_db: 1 })]))
    const w = await show()
    expect(w.findAll('[data-test="radio-dot-unusual"]')).toHaveLength(1)
    expect(w.get('[data-test="radio-radar-unusual"]').text()).toBe('1 is unlike its usual.')
    expect(w.get('[data-test="radio-row-unusual"]').text()).toBe('30 dB stronger than its usual -80 dBm: closer than usual')
  })

  it('draws a chosen device’s track in runs, leaving a gap for a cycle it was silent in', async () => {
    const w = await show()
    await w.findAll('[data-test="radio-radar-row"]')[1]?.find('button').trigger('click')
    await flushPromises()
    expect(service.track).toHaveBeenCalledWith('close1')
    expect(w.findAll('[data-test="radio-track-line"]')).toHaveLength(2) // two runs: a gap between them
    expect(w.get('[data-test="radio-track-summary"]').text()).toContain('Heard 4 times in the last 17 minutes')
  })

  it('says there is no track yet, without an error, when the server has none', async () => {
    service.track.mockRejectedValue(new Error('this device has no track in this run'))
    const w = await show()
    await w.findAll('[data-test="radio-radar-row"]')[1]?.find('button').trigger('click')
    await flushPromises()
    expect(w.find('[data-test="radio-track-none"]').exists()).toBe(true)
    expect(w.find('[data-test="radio-track-error"]').exists()).toBe(false)
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

  it('opens a device with what it is waiting for, or why it was sorted, and keeps retries and channels for Advanced', async () => {
    const detail = { id: 'w1', class: 'unknown' as const, why: 'needs_more_listening', signal: 'weak' as const, first_seen: '2026-11-12', last_seen: '2026-11-14', cycles_heard: 2, need_heard: 3, retry_percent: 12, channels: [1, 11], random_address: true, joined_own: false }
    service.live.mockResolvedValue(view([
      dev({ id: 'w1', class: 'unknown', detail }),
      dev({ id: 's1', class: 'neighbour', detail: { ...detail, id: 's1', class: 'neighbour', why: 'never_strong' } }),
      dev({ id: 'n1', class: 'neighbour', detail: { ...detail, id: 'n1', channels: null as unknown as number[] } }),
    ]))
    let w = await show()
    await w.findAll('[data-test="radio-radar-row"]')[0]!.find('button').trigger('click')
    await flushPromises()
    expect(w.find('[data-test="radio-air-waiting"]').exists() || w.find('[data-test="radio-air-why"]').exists()).toBe(true)
    expect(w.find('[data-test="radio-air-retries"]').exists()).toBe(false)
    useNetauditViewMode().set('advanced')
    w = await show()
    for (const row of w.findAll('[data-test="radio-radar-row"]')) {
      await row.find('button').trigger('click')
      await flushPromises()
      expect(w.get('[data-test="radio-air-retries"]').text()).toBe('12%') // a device with a null channel list must not blank the panel
      expect(w.get('[data-test="radio-air-channels"]').text()).toMatch(/1, 11|none/)
      await row.find('button').trigger('click')
    }
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
})
