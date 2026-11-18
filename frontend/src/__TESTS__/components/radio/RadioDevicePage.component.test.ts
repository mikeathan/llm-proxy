import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RadioDeviceView as DeviceView, RadioLiveDevice } from '../../../types/radio'

const service = vi.hoisted(() => ({ device: vi.fn(), markMine: vi.fn(), setClass: vi.fn(), setKind: vi.fn(), calibrate: vi.fn(), track: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))
vi.mock('vue-router', async (orig) => ({ ...(await orig<typeof import('vue-router')>()), useRoute: () => ({ params: { id: '2a6e3b0d' } }) }))

import RadioDeviceView from '../../../views/RadioDeviceView.vue'
import { HttpError } from '../../../services/httpClient'

const sky: RadioLiveDevice = {
  id: '2a6e3b0d', mine: false, class: 'neighbour', dbm: -71, near_m: 9, far_m: 30, ring: 'nearby', angle: 40, trend: 'steady', channel: 6, heard: true, network: 'SKY1234',
  ident: { kind: 'router', kind_confidence: 90, kind_source: 'sends_beacons', brand: 'SKY UK', confidence: 72, net: undefined, clients: 2 },
  detail: { id: '2a6e3b0d', class: 'neighbour', why: 'access_point_comes_and_goes', signal: 'weak', first_seen: '2026-11-17', last_seen: '2026-11-17', cycles_heard: 11, need_heard: 3, retry_percent: 0, channels: [6], random_address: false, joined_own: false, maker: 'SKY UK', wifi_generation: 6 },
}
const page = (over: Partial<DeviceView> = {}): DeviceView => ({
  device: sky, chances: { neighbour: 72, infrastructure: 20, personal: 5, transient: 3 }, factors: [{ feature: 'signal_max', toward: 'neighbour' }],
  radios: [], clients: [{ id: 'c0ffee01', mine: false, class: 'transient', heard: true, dbm: -80 }], history: [{ day: '2026-11-17', from: 'transient', to: 'neighbour', version: 3, reason: 'matches_what_was_learned', n: 1 }],
  model: { ref_24_dbm: -40, ref_5_dbm: -47, calibrated: false, provisional: true }, ...over,
})

const mounted: Array<{ unmount: () => void }> = []
afterEach(() => { mounted.splice(0).forEach((w) => w.unmount()) })
beforeEach(() => {
  vi.clearAllMocks()
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

  it('says plainly when the device is no longer in the picture', async () => {
    service.device.mockRejectedValue(new HttpError('gone', '', 404))
    const w = await show()
    expect(w.get('[data-test="radio-device-error"]').text()).toContain('not in the picture')
  })
})
