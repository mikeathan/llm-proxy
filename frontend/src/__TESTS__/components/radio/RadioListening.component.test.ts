import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const service = vi.hoisted(() => ({
  sensors: vi.fn(), listening: vi.fn(), plan: vi.fn(), beginSession: vi.fn(), start: vi.fn(), stop: vi.fn(), openSessions: vi.fn(), endSession: vi.fn(),
  ownerDevices: vi.fn(), addOwnerDevice: vi.fn(), removeOwnerDevice: vi.fn(), forgetNeighbours: vi.fn(),
}))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))

import RadioListening from '../../../components/radio/RadioListening.vue'
import RadioOwnerDevices from '../../../components/radio/RadioOwnerDevices.vue'

const plan = { channels: [1, 60], window_seconds: 20, cycle_seconds: 300 }
const idle = { listening: false, cycles: 0, frames: 0, windows: [], plan }
const sensors = (over = {}) => ({ enabled: true, unused: 0, sensors: [{ id: 'wlx00', kind: 'wifi', eligible: true, why: '', capabilities: [] }], ...over })

beforeEach(() => {
  vi.clearAllMocks()
  service.sensors.mockResolvedValue(sensors())
  service.listening.mockResolvedValue(idle)
})

async function show() {
  const w = mount(RadioListening)
  await flushPromises()
  return w
}

describe('RadioListening', () => {
  it('starts with the chosen adapter and time', async () => {
    service.plan.mockResolvedValue({ adapter: 'wlx00', plan, requests: [], max_hours: 12 })
    service.beginSession.mockResolvedValue({ id: 's1', stack: 'radio' })
    service.start.mockResolvedValue({ ...idle, listening: true, until: '2026-11-14T10:00:00Z' })
    const w = await show()
    await w.get('[data-test="radio-listen-start"]').trigger('click')
    await flushPromises()
    expect(service.start).toHaveBeenCalledWith('wlx00', 1)
    expect(w.find('[data-test="radio-listen-running"]').exists()).toBe(true)
    expect(w.find('[data-test="radio-listen-start"]').exists()).toBe(false)
  })

  it('says why it cannot start, with the fix: stack off, and no adapter', async () => {
    service.sensors.mockResolvedValue(sensors({ enabled: false }))
    let w = await show()
    expect(w.get('[data-test="radio-listen-off"]').text()).toContain('Switch the Radio stack on')
    expect(w.find('[data-test="radio-listen-start"]').exists()).toBe(false)
    service.sensors.mockResolvedValue(sensors({ sensors: [] }))
    w = await show()
    expect(w.get('[data-test="radio-listen-noadapter"]').text()).toContain('Plug in a USB Wi-Fi adapter')
  })

  it('names the fix when the helper would not listen', async () => {
    service.listening.mockResolvedValue({ ...idle, ended: 'no_session', last_error: 'the action is switched off' })
    const w = await show()
    expect(w.get('[data-test="radio-listen-ended"]').text()).toContain('radio_listen switched on')
    expect(w.get('[data-test="radio-listen-last-error"]').text()).toBe('the action is switched off')
  })

  it('shows windows as counts and words, not addresses', async () => {
    service.listening.mockResolvedValue({ ...idle, listening: true, until: '2026-11-14T10:00:00Z', windows: [{ channel: 60, frames: 40, recorded: 40, unread: 0, stop: 'window', kernel_seen: 40, kernel_dropped: 0, elapsed_ms: 20000, on_channel: 40, off_channel: 0, channel_matched: true, at: '2026-11-13T10:00:00Z' }] })
    const w = await show()
    expect(w.get('[data-test="radio-window"]').text()).toContain('40 frames')
    expect(w.get('[data-test="radio-window"]').text()).toContain('on the channel asked')
  })
})

describe('RadioOwnerDevices', () => {
  const device = { id: 'ab', label: 'my phone', visits: 3, first_seen: '2026-11-10', last_seen: '2026-11-13' }

  it('lists devices by name and days, never an address, and adds and removes them', async () => {
    service.ownerDevices.mockResolvedValue([device])
    service.addOwnerDevice.mockResolvedValue({ label: 'x' })
    service.removeOwnerDevice.mockResolvedValue(undefined)
    const w = mount(RadioOwnerDevices)
    await flushPromises()
    expect(w.get('[data-test="radio-device"]').text()).toContain('my phone')
    await w.get('[data-test="radio-device-label"]').setValue('tv')
    await w.get('[data-test="radio-device-address"]').setValue('3a:91:5c:d2:7e:44')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(service.addOwnerDevice).toHaveBeenCalledWith('3a:91:5c:d2:7e:44', 'tv')
    await w.get('[data-test="radio-device-remove"]').trigger('click')
    await flushPromises()
    expect(service.removeOwnerDevice).toHaveBeenCalledWith('ab')
  })

  it('shows the server’s plain refusal and keeps what was typed', async () => {
    service.ownerDevices.mockResolvedValue([])
    const { HttpError } = await import('../../../services/httpClient')
    service.addOwnerDevice.mockRejectedValue(new HttpError('an address is six pairs of digits', undefined, 400))
    const w = mount(RadioOwnerDevices)
    await flushPromises()
    await w.get('[data-test="radio-device-label"]').setValue('tv')
    await w.get('[data-test="radio-device-address"]').setValue('nope')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(w.get('[data-test="radio-devices-error"]').text()).toBe('an address is six pairs of digits')
    expect((w.get('[data-test="radio-device-address"]').element as HTMLInputElement).value).toBe('nope')
  })

  it('forgets the neighbours and says so', async () => {
    service.ownerDevices.mockResolvedValue([])
    service.forgetNeighbours.mockResolvedValue({ forgotten: true })
    const w = mount(RadioOwnerDevices)
    await flushPromises()
    await w.get('[data-test="radio-forget"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="radio-forget-done"]').text()).toContain('forgotten')
  })
})
