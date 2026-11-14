import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RadioAirwaves } from '../../../types/radio'

const service = vi.hoisted(() => ({ airwaves: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))

import RadioAirwavesCard from '../../../components/radio/RadioAirwaves.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'

const view = (over: Partial<RadioAirwaves> = {}): RadioAirwaves => ({
  channels: [{ channel: 1, devices: 4 }, { channel: 6, devices: 12 }, { channel: 60, devices: 3 }],
  classes: [{ class: 'infrastructure', devices: 4, frames: 1000, retry_percent: 8 }, { class: 'neighbour', devices: 9, frames: 400, retry_percent: 31 }],
  routers: [{ name: 'netsink', channel: 60 }],
  near_unknown: 2, listened_cycles: 30, ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  useNetauditViewMode().set('simple')
  service.airwaves.mockResolvedValue(view())
})

async function show() {
  const w = mount(RadioAirwavesCard)
  await flushPromises()
  return w
}

describe('RadioAirwaves', () => {
  it('says in words which channel is busiest, where the router is, and how many unsorted devices are close', async () => {
    const w = await show()
    expect(w.get('[data-test="radio-air-busiest"]').text()).toBe('Busiest: channel 6, 12 devices.')
    expect(w.get('[data-test="radio-air-router"]').text()).toBe('netsink: channel 60')
    expect(w.get('[data-test="radio-air-near"]').text()).toContain('2 devices that are not yet sorted have a strong signal')
    expect(w.find('[data-test="radio-air-channel"]').exists()).toBe(false)
  })

  it('shows the tables in Advanced: a bar per channel, and retries by kind of device', async () => {
    useNetauditViewMode().set('advanced')
    const w = await show()
    const rows = w.findAll('[data-test="radio-air-channel"]')
    expect(rows.map((r) => r.attributes('data-channel'))).toEqual(['1', '6', '60'])
    expect(rows[1]!.text()).toContain('12')
    expect(rows[1]!.get('[data-test="radio-air-channel-bar"]').attributes('style')).toContain('width: 100%')
    const classes = w.findAll('[data-test="radio-air-class"]')
    expect(classes[1]!.text()).toContain('Outside your walls')
    expect(classes[1]!.text()).toContain('31% retried')
    expect(w.get('[data-test="radio-air-cap-note"]').text()).toContain('4,000 frames')
  })

  it('says nothing was counted yet, and shows a failed read', async () => {
    service.airwaves.mockResolvedValue(view({ channels: [], classes: [], routers: [], near_unknown: 0, listened_cycles: 0 }))
    let w = await show()
    expect(w.get('[data-test="radio-air-empty"]').text()).toContain('Start listening')
    service.airwaves.mockRejectedValue(new Error('down'))
    w = await show()
    expect(w.get('[data-test="radio-air-error"]').text()).toBe('down')
  })
})
