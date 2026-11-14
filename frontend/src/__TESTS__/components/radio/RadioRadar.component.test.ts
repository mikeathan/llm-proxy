import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RadioLive, RadioLiveDevice } from '../../../types/radio'

const service = vi.hoisted(() => ({ live: vi.fn(), calibrate: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))

import RadioRadar from '../../../components/radio/RadioRadar.vue'

const dev = (over: Partial<RadioLiveDevice>): RadioLiveDevice => ({
  id: 'aa', mine: false, class: 'neighbour', dbm: -70, near_m: 5, far_m: 20, ring: 'house', angle: 90, trend: 'steady', channel: 6, heard: true, ...over,
})
const view = (devices: RadioLiveDevice[], over: Partial<RadioLive['model']> = {}): RadioLive => ({
  listening: true, cycles: 4, model: { ref_24_dbm: -40, ref_5_dbm: -47, calibrated: false, provisional: true, ...over }, devices, rings: {},
})

beforeEach(() => {
  vi.clearAllMocks()
  service.live.mockResolvedValue(view([dev({ id: 'far1', near_m: 8, far_m: 40, ring: 'nearby' }), dev({ id: 'mine1', mine: true, label: 'my phone', class: 'personal', near_m: 1.2, far_m: 3 }), dev({ id: 'close1', near_m: 2, far_m: 6, trend: 'closer' })]))
  service.calibrate.mockResolvedValue({ calibrated: true })
})

async function show() {
  const w = mount(RadioRadar)
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
})

