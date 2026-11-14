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
    id: 'wlx00', kind: 'wifi', role: 'can_listen', bus: 'usb', driver: 'mt7601u', usb_id: '148f:7601', mode: 'managed', eligible: true, why: 'a USB adapter that is not carrying this computer’s connection',
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
    expect(w.get('[data-test="radio-role"]').text()).toContain('Can listen')
    expect(w.get('[data-test="radio-found"]').text()).toBe('1 radio found, 1 can listen.')
    expect(w.get('[data-test="radio-probe"]').text()).toContain('read from it the first time you listen')
    const uses = w.findAll('[data-test="radio-capability"]').map((c) => c.attributes('data-use'))
    expect(uses).toEqual(['used', 'unused_by_design', 'unused_not_built'])
    expect(w.get('[data-test="radio-unused"]').text()).toBe('3 things could still be built.')
    expect(w.text()).toContain('not read from the adapter yet')
  })

  it('leads with a role, shows what the probe read, and gives a radio nothing will use one line and a reason', async () => {
    const base = view().sensors[0]!
    const w = await show(view({
      unused: 0,
      sensors: [
        { ...base, role: 'listening', probe: { read_at: '2026-11-14', modes: ['monitor'], bands: ['2.4 GHz', '5 GHz'], standards: ['Wi-Fi 4', 'Wi-Fi 5'], monitor: true } },
        { id: 'wlp2s0', kind: 'wifi', role: 'wont_use', eligible: false, why: 'not a USB adapter: the built-in card is never used for capture', capabilities: [base.capabilities[0]!, base.capabilities[2]!] },
      ],
    }))
    const rows = w.findAll('[data-test="radio-sensor"]')
    expect(rows[0]!.get('[data-test="radio-role"]').text()).toContain('Listening now')
    expect(rows[0]!.get('[data-test="radio-probe"]').text()).toBe('2.4 GHz and 5 GHz · Wi-Fi 4, Wi-Fi 5 · monitor mode supported')
    expect(rows[1]!.get('[data-test="radio-role"]').text()).toContain('Will not be used')
    expect(rows[1]!.get('[data-test="radio-why"]').text()).toContain('built-in card')
    expect(rows[1]!.findAll('[data-test="radio-capability"]')).toHaveLength(2)
    expect(w.get('[data-test="radio-found"]').text()).toBe('2 radios found, 1 can listen.')
    expect(w.find('[data-test="radio-unused"]').exists()).toBe(false)
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
