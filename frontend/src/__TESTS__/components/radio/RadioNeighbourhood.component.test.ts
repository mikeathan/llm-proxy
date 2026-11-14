import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RadioAir, RadioAirDevice } from '../../../types/radio'

const service = vi.hoisted(() => ({ air: vi.fn(), airDevices: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))

import RadioNeighbourhood from '../../../components/radio/RadioNeighbourhood.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'

const air = (over: Partial<RadioAir> = {}): RadioAir => ({
  rules: { version: 1, provisional: true, window_days: 7, min_cycles_listened: 12 },
  listened_cycles: 30,
  counts: { infrastructure: 4, personal: 2, neighbour: 9, transient: 1, unknown: 3 },
  total: 19,
  changes: [{ day: '2026-11-14', from: '', to: 'infrastructure', version: 1, reason: 'always_here_steady_signal', n: 4 }],
  ...over,
})
const dev = (over: Partial<RadioAirDevice> = {}): RadioAirDevice => ({
  id: 'a1b2c3d4', class: 'neighbour', why: 'never_strong', signal: 'weak', first_seen: '2026-11-12', last_seen: '2026-11-14', cycles_heard: 20, need_heard: 3,
  retry_percent: 12, channels: [1, 11], random_address: false, joined_own: false, ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  useNetauditViewMode().set('simple')
  service.air.mockResolvedValue(air())
  service.airDevices.mockResolvedValue({ listened_cycles: 30, min_listened_cycles: 12, total: 1, devices: [dev()] })
})

async function show() {
  const w = mount(RadioNeighbourhood)
  await flushPromises()
  return w
}

describe('RadioNeighbourhood', () => {
  it('counts the kinds and opens one onto its devices, each a short id with a signal word', async () => {
    const w = await show()
    expect(w.get('[data-test="radio-around-total"]').text()).toBe('19 devices around you')
    expect(w.findAll('[data-test="radio-device-list"]')).toHaveLength(0)
    await w.get('[data-class="neighbour"] [data-test="radio-class-open"]').trigger('click')
    await flushPromises()
    expect(service.airDevices).toHaveBeenCalledWith('neighbour', undefined)
    const row = w.get('[data-test="radio-air-device"]')
    expect(row.text()).toContain('a1b2c3d4')
    expect(row.get('[data-test="radio-air-signal"]').text()).toBe('Weak')
    expect(row.text()).toContain('2026-11-14')
  })

  it('closes a kind again', async () => {
    const w = await show()
    const open = w.get('[data-class="neighbour"] [data-test="radio-class-open"]')
    await open.trigger('click')
    await flushPromises()
    expect(open.attributes('aria-expanded')).toBe('true')
    await open.trigger('click')
    expect(w.findAll('[data-test="radio-air-device"]')).toHaveLength(0)
  })

  it('says what an unsorted device is waiting for, with the progress', async () => {
    service.airDevices.mockResolvedValue({ listened_cycles: 4, min_listened_cycles: 12, total: 1, devices: [dev({ class: 'unknown', why: 'needs_more_listening', cycles_heard: 2, need_heard: 3, signal: 'unknown' })] })
    const w = await show()
    await w.get('[data-class="unknown"] [data-test="radio-class-open"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="radio-air-waiting"]').text()).toBe('Waiting: the sensor has listened through 4 of the 12 cycles it needs, and this device has been heard in 2 of the 3 cycles it needs.')
    expect(w.get('[data-test="radio-air-progress"]').attributes('aria-valuenow')).toBe('33')
  })

  it('keeps retries, channels and sorting for Advanced', async () => {
    let w = await show()
    await w.get('[data-class="neighbour"] [data-test="radio-class-open"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-test="radio-air-retries"]').exists()).toBe(false)
    expect(w.find('[data-test="radio-air-sort"]').exists()).toBe(false)
    useNetauditViewMode().set('advanced')
    w = await show()
    await w.get('[data-class="neighbour"] [data-test="radio-class-open"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="radio-air-retries"]').text()).toContain('12%')
    expect(w.get('[data-test="radio-air-channels"]').text()).toContain('1, 11')
    expect(w.get('[data-test="radio-air-heard"]').text()).toContain('20 of 30 cycles')
    await w.get('[data-test="radio-air-sort"] select').setValue('retries')
    await flushPromises()
    expect(service.airDevices).toHaveBeenLastCalledWith('neighbour', 'retries')
  })

  it('says a sorted device that went quiet is not waiting for evidence', async () => {
    service.airDevices.mockResolvedValue({ listened_cycles: 30, min_listened_cycles: 12, total: 1, devices: [dev({ cycles_heard: 0, why: 'not_heard_lately' })] })
    const w = await show()
    await w.get('[data-class="neighbour"] [data-test="radio-class-open"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-test="radio-air-waiting"]').exists()).toBe(false)
    expect(w.get('[data-test="radio-air-why"]').text()).toContain('not heard in the last few days')
  })

  it('shows what changed, and says when nothing was heard yet', async () => {
    let w = await show()
    expect(w.get('[data-test="radio-change"]').text()).toContain('4 devices first sorted as always here')
    service.air.mockResolvedValue(air({ listened_cycles: 0, total: 0, changes: [] }))
    w = await show()
    expect(w.get('[data-test="radio-around-empty"]').text()).toContain('Start listening')
  })

  it('shows a failed read with the reason, and an empty kind plainly', async () => {
    service.airDevices.mockResolvedValue({ listened_cycles: 30, min_listened_cycles: 12, total: 0, devices: [] })
    const w = await show()
    await w.get('[data-class="transient"] [data-test="radio-class-open"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="radio-device-none"]').text()).toBe('No device of this kind yet.')
    service.airDevices.mockRejectedValue(new Error('boom'))
    await w.get('[data-class="personal"] [data-test="radio-class-open"]').trigger('click')
    await flushPromises()
    expect(w.find('[data-test="radio-neigh-error"]').exists()).toBe(true)
  })
})
