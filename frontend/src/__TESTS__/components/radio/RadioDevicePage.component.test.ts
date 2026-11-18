import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RadioDeviceView as DeviceView, RadioLiveDevice } from '../../../types/radio'

const service = vi.hoisted(() => ({ device: vi.fn(), markMine: vi.fn(), setClass: vi.fn(), setKind: vi.fn(), calibrate: vi.fn(), track: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))
vi.mock('vue-router', async (orig) => ({ ...(await orig<typeof import('vue-router')>()), useRoute: () => ({ params: { id: '2a6e3b0d' } }) }))

import RadioDeviceView from '../../../views/RadioDeviceView.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'
import { HttpError } from '../../../services/httpClient'

const sky: RadioLiveDevice = {
  id: '2a6e3b0d', mine: false, class: 'neighbour', dbm: -71, near_m: 9, far_m: 30, ring: 'nearby', angle: 40, trend: 'steady', channel: 6, heard: true, network: 'SKY1234',
  ident: { kind: 'router', kind_confidence: 90, kind_source: 'sends_beacons', brand: 'SKY UK', confidence: 72, net: undefined, clients: 2 },
  detail: { id: '2a6e3b0d', class: 'neighbour', why: 'access_point_comes_and_goes', signal: 'weak', first_seen: '2026-11-17', last_seen: '2026-11-17', cycles_heard: 11, need_heard: 3, retry_percent: 0, channels: [6], random_address: false, joined_own: false, maker: 'SKY UK', wifi_generation: 6 },
}
const page = (over: Partial<DeviceView> = {}): DeviceView => ({
  device: sky, chances: { neighbour: 72, infrastructure: 20, personal: 5, transient: 3 }, factors: [{ feature: 'signal_max', toward: 'neighbour' }],
  radios: [], clients: [{ id: 'c0ffee01', mine: false, class: 'transient', heard: true, dbm: -80 }], history: [{ day: '2026-11-17', from: 'transient', to: 'neighbour', version: 3, reason: 'matches_what_was_learned', n: 1 }],
  model: { ref_24_dbm: -40, ref_5_dbm: -47, calibrated: false, provisional: true }, listened_cycles: 5, min_listened_cycles: 12, ...over,
})

const mounted: Array<{ unmount: () => void }> = []
afterEach(() => { mounted.splice(0).forEach((w) => w.unmount()) })
beforeEach(() => {
  vi.clearAllMocks()
  useNetauditViewMode().set('simple')
  service.device.mockResolvedValue(page())
  service.setClass.mockResolvedValue({ class: 'neighbour' })
  service.setKind.mockResolvedValue({ kind: 'tv' })
  service.markMine.mockResolvedValue({ label: 'x' })
  service.track.mockRejectedValue(new HttpError('none', '', 404))
})

async function show() {
  const w = mount(RadioDeviceView, { global: { stubs: { RouterLink: { props: ['to'], template: '<a data-test="link"><slot /></a>' } } } })
  mounted.push(w)
  await flushPromises()
  return w
}

describe('RadioDeviceView', () => {
  it('names the device by what it is and says whose it probably is, never by its short id', async () => {
    const w = await show()
    expect(w.get('[data-test="radio-device-title"]').text()).toBe('SKY UK router')
    expect(w.get('[data-test="radio-device-conclusion"]').text()).toBe('Probably: a neighbour’s router')
    expect(w.get('[data-test="radio-device-kind"]').text()).toContain('it broadcasts a Wi-Fi network')
    expect(w.get('[data-test="radio-device-factors"]').text()).toContain('its strongest signal')
  })

  it('draws it alone on the radar as a ring from its nearest to its farthest distance', async () => {
    const w = await show()
    expect(w.findAll('[data-test="radio-dot"]')).toHaveLength(1)
    expect(w.find('[data-test="radio-solo-band"]').exists()).toBe(true)
    expect(w.get('[data-test="radio-device-range"]').text()).toContain('9–30 m')
  })

  it('lists the devices that use it and saves the owner’s answers', async () => {
    const w = await show()
    expect(w.findAll('[data-test="radio-member"]')).toHaveLength(1)
    await w.get('[data-test="radio-device-not-mine"]').trigger('click')
    await flushPromises()
    expect(service.setClass).toHaveBeenCalledWith('2a6e3b0d', 'not_mine')
    expect(w.find('[data-test="radio-device-saved"]').exists()).toBe(true)
    await w.get('[data-test="radio-device-mine"]').trigger('click')
    await w.get('[data-test="radio-device-mine-input"]').setValue('Hall hub')
    await w.get('[data-test="radio-device-mine-save"]').trigger('submit')
    await flushPromises()
    expect(service.markMine).toHaveBeenCalledWith('2a6e3b0d', 'Hall hub')
  })

  it('says when it was first and last heard, and no trend until there are enough readings', async () => {
    service.device.mockResolvedValue(page({ device: { ...sky, trend: 'unknown' } }))
    const w = await show()
    expect(w.get('[data-test="radio-device-heard"]').text()).toBe('First heard 2026-11-17 · last heard 2026-11-17')
    expect(w.get('[data-test="radio-device-where"]').text()).not.toContain('too new to tell')
  })

  it('says its kind came from your name for it on the Network page', async () => {
    const mac = { ...sky, class: 'personal' as const, ident: { kind: 'computer' as const, kind_confidence: 75, kind_source: 'your_name_for_it' as const, lan_name: 'M4 MacBook air', on_your_wifi: true, own_cycles: 3, confidence: 100 } }
    service.device.mockResolvedValue(page({ device: mac }))
    const w = await show()
    expect(w.get('[data-test="radio-device-title"]').text()).toBe('M4 MacBook air')
    expect(w.get('[data-test="radio-device-kind"]').text()).toContain('from your name for it on the Network page')
  })

  it('draws its track and calibrates against your own router at a distance you give', async () => {
    service.track.mockResolvedValue({ id: '2a6e3b0d', mine: false, trend: 'steady', stored: false, now: 2000, points: [
      { at: 1000, dbm: -80, near_m: 10, far_m: 40, run: 0 }, { at: 1300, dbm: -70, near_m: 5, far_m: 20, run: 0 },
      { at: 1900, dbm: -60, near_m: 2, far_m: 6, run: 1 }, { at: 2000, dbm: -58, near_m: 2, far_m: 5, run: 1 },
    ] })
    service.calibrate.mockResolvedValue({ calibrated: true })
    service.device.mockResolvedValue(page({ device: { ...sky, ident: { ...sky.ident!, your_ap: true } } }))
    const w = await show()
    expect(w.findAll('[data-test="radio-track-line"]')).toHaveLength(2) // two runs: a gap between them
    await w.get('[data-test="radio-device-calibrate"]').trigger('submit')
    await flushPromises()
    expect(service.calibrate).toHaveBeenCalledWith('2a6e3b0d', 1)
  })

  it('says what an unsorted device is waiting for, and keeps heard, retries and channels for Advanced', async () => {
    const waiting = { ...sky, class: 'unknown' as const, detail: { ...sky.detail!, class: 'unknown' as const, why: 'needs_more_listening', cycles_heard: 2, retry_percent: 12, channels: [1, 11] } }
    service.device.mockResolvedValue(page({ device: waiting }))
    let w = await show()
    expect(w.get('[data-test="radio-device-waiting"]').text()).toContain('Waiting: the sensor has listened through 5 of the 12 cycles it needs')
    expect(w.find('[data-test="radio-device-counts"]').exists()).toBe(false)
    useNetauditViewMode().set('advanced')
    service.device.mockResolvedValue(page({ device: { ...waiting, detail: { ...waiting.detail, channels: null as unknown as number[] } } }))
    w = await show()
    expect(w.get('[data-test="radio-device-heard-in"]').text()).toBe('2 of 5 cycles')
    expect(w.get('[data-test="radio-device-retries"]').text()).toBe('12%')
    expect(w.get('[data-test="radio-device-channels"]').text()).toBe('none') // a null channel list must not blank the page
  })

  it('says why a maker cannot be read for a private address, how strong it is, and what naming it keeps', async () => {
    service.device.mockResolvedValue(page({ device: { ...sky, detail: { ...sky.detail!, random_address: true } } }))
    const w = await show()
    expect(w.get('[data-test="radio-device-address"]').text()).toContain('made-up address')
    expect(w.get('[data-test="radio-device-signal"]').text()).toContain('far, or behind walls')
    await w.get('[data-test="radio-device-mine"]').trigger('click')
    expect(w.get('[data-test="radio-device-mine-note"]').text()).toContain('not its address')
    expect(w.html()).not.toMatch(/([0-9a-f]{2}:){5}[0-9a-f]{2}/i)
  })

  it('says plainly when the device is no longer in the picture', async () => {
    service.device.mockRejectedValue(new HttpError('gone', '', 404))
    const w = await show()
    expect(w.get('[data-test="radio-device-error"]').text()).toContain('not in the picture')
  })
})
