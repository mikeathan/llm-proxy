import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RadioScan } from '../../../types/radio'

const service = vi.hoisted(() => ({ scan: vi.fn(), saveScan: vi.fn(), listening: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))

import RadioScanPlan from '../../../components/radio/RadioScanPlan.vue'

const DEFAULT = { channels: [1, 6, 11, 36, 60, 149], window_seconds: 20, cycle_seconds: 300 }
const scan = (over: Partial<RadioScan> = {}): RadioScan => ({
  plan: DEFAULT, default: DEFAULT, custom: false, windows_per_hour: 72,
  limits: { channels: [1, 6, 11, 36, 60, 149, 165], max_channels: 16, min_window_seconds: 1, max_window_seconds: 30, min_cycle_seconds: 60, max_windows_per_hour: 200 },
  ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  service.scan.mockResolvedValue(scan())
  service.listening.mockResolvedValue({ listening: false })
  service.saveScan.mockImplementation(async (p) => scan({ plan: p, custom: true, windows_per_hour: Math.floor((p.channels.length * 3600) / p.cycle_seconds) }))
})

async function show() {
  const w = mount(RadioScanPlan)
  await flushPromises()
  return w
}
async function open(w: Awaited<ReturnType<typeof show>>) {
  await w.get('[data-test="radio-scan-toggle"]').trigger('click')
  await flushPromises()
}

describe('RadioScanPlan', () => {
  it('says the plan in a sentence, and calls the default the default', async () => {
    const w = await show()
    expect(w.get('[data-test="radio-scan-summary"]').text()).toBe('6 channels, 20 seconds each, every 5 minutes: 72 windows an hour (the default).')
    expect(w.find('[data-test="radio-scan-editor"]').exists()).toBe(false)
  })

  it('opens an editor with the current channels pressed, and saves a changed plan sorted by the server', async () => {
    const w = await show()
    await open(w)
    const pressed = w.findAll('[data-test="radio-scan-channel"]').filter((b) => b.attributes('aria-pressed') === 'true').map((b) => b.attributes('data-channel'))
    expect(pressed).toEqual(['1', '6', '11', '36', '60', '149'])
    await w.get('[data-channel="149"]').trigger('click')
    await w.get('[data-channel="165"]').trigger('click')
    await w.get('[data-test="radio-scan-save"]').trigger('click')
    await flushPromises()
    expect(service.saveScan).toHaveBeenCalledWith({ channels: [1, 6, 11, 36, 60, 165], window_seconds: 20, cycle_seconds: 300 })
    expect(w.get('[data-test="radio-scan-saved"]').text()).toContain('next time you start listening')
    expect(w.find('[data-test="radio-scan-reset"]').exists()).toBe(true)
  })

  it('refuses a plan the server would refuse, with the reason, and offers no Save', async () => {
    const w = await show()
    await open(w)
    for (const c of [1, 6, 11, 36, 60, 149]) await w.get(`[data-channel="${c}"]`).trigger('click')
    expect(w.get('[data-test="radio-scan-check"]').text()).toBe('Choose at least one channel.')
    expect(w.get('[data-test="radio-scan-save"]').attributes('disabled')).toBeDefined()
    for (const c of [1, 6, 11, 36, 60, 149]) await w.get(`[data-channel="${c}"]`).trigger('click')
    await w.find('select[aria-label="Time on each channel"]').setValue('30')
    await w.find('select[aria-label="Come round every"]').setValue('60')
    expect(w.get('[data-test="radio-scan-check"]').text()).toContain('more than 200 windows an hour')
    expect(w.get('[data-test="radio-scan-save"]').attributes('disabled')).toBeDefined()
  })

  it('puts the default back with one button and closes the editor to a run that is listening', async () => {
    service.scan.mockResolvedValue(scan({ custom: true, plan: { channels: [1, 6], window_seconds: 10, cycle_seconds: 120 }, windows_per_hour: 60 }))
    const w = await show()
    await open(w)
    await w.get('[data-test="radio-scan-reset"]').trigger('click')
    await flushPromises()
    expect(service.saveScan).toHaveBeenCalledWith({ channels: [], window_seconds: 0, cycle_seconds: 0 })
    service.listening.mockResolvedValue({ listening: true })
    const running = await show()
    await open(running)
    expect(running.get('[data-test="radio-scan-running"]').text()).toContain('Stop it to change the plan')
    expect(running.get('[data-test="radio-scan-save"]').attributes('disabled')).toBeDefined()
  })

  it('shows the server’s sentence when a save is refused', async () => {
    service.saveScan.mockRejectedValue(new Error('channel 99 cannot be listened on'))
    const w = await show()
    await open(w)
    await w.get('[data-channel="165"]').trigger('click')
    await w.get('[data-test="radio-scan-save"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="radio-scan-save-error"]').text()).toBe('channel 99 cannot be listened on')
  })
})
