import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RadioView } from '../../../types/radio'

const service = vi.hoisted(() => ({ sensors: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))
const push = vi.hoisted(() => vi.fn())
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))

import RadioSensors from '../../../components/radio/RadioSensors.vue'

const view = (over: Partial<RadioView> = {}): RadioView => ({
  enabled: true,
  unused: 3,
  sensors: [{
    id: 'wlx00', kind: 'wifi', bus: 'usb', driver: 'mt7601u', usb_id: '148f:7601', mode: 'managed', eligible: true, why: 'a USB adapter that is not carrying this computer’s connection',
    capabilities: [
      { id: 'discovery', title: 'Found and described', use: 'used', known: true, note: 'Read from the system’s own records.' },
      { id: 'frame_injection', title: 'Send frames', use: 'unused_by_design', known: false, note: 'Never sent.' },
      { id: 'monitor_capture', title: 'Listen to nearby Wi-Fi', use: 'unused_not_built', known: false, note: 'Not built yet.' },
    ],
  }],
  ...over,
})

async function show(v: RadioView) {
  service.sensors.mockResolvedValue(v)
  const w = mount(RadioSensors)
  await flushPromises()
  return w
}

beforeEach(() => vi.clearAllMocks())

describe('RadioSensors', () => {
  it('lists each radio with whether it may listen and what the stack does about each capability', async () => {
    const w = await show(view())
    expect(w.get('[data-test="radio-sensor"]').text()).toContain('Can listen')
    const uses = w.findAll('[data-test="radio-capability"]').map((c) => c.attributes('data-use'))
    expect(uses).toEqual(['used', 'unused_by_design', 'unused_not_built'])
    expect(w.get('[data-test="radio-unused"]').text()).toBe('3 capabilities are found and not used yet.')
    expect(w.text()).toContain('not read yet')
  })

  it('says the stack is off, and takes you to Modules', async () => {
    const w = await show(view({ enabled: false }))
    expect(w.get('[data-test="radio-off"]').text()).toContain('Nothing listens until you switch it on')
    await w.get('[data-test="radio-open-modules"]').trigger('click')
    expect(push).toHaveBeenCalledWith({ name: 'radio', params: { tab: 'modules' } })
  })

  it('says plainly when there is no adapter, with the server’s reason', async () => {
    const w = await show(view({ sensors: [], unused: 0, reason: 'no wireless adapter found' }))
    expect(w.text()).toContain('No radio to listen with')
    expect(w.text()).toContain('no wireless adapter found')
    expect(w.findAll('[data-test="radio-sensor"]')).toHaveLength(0)
  })

  it('says why the radios could not be read, and keeps asking possible with Look again', async () => {
    service.sensors.mockRejectedValueOnce(new Error('boom'))
    const w = mount(RadioSensors)
    await flushPromises()
    expect(w.text()).toContain('The radios could not be read')
    service.sensors.mockResolvedValue(view())
    await w.get('[data-test="radio-reload"]').trigger('click')
    await flushPromises()
    expect(w.findAll('[data-test="radio-sensor"]')).toHaveLength(1)
  })
})
