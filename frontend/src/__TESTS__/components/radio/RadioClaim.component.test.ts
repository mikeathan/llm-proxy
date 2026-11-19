import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RadioLive, RadioLiveDevice } from '../../../types/radio'

const service = vi.hoisted(() => ({ live: vi.fn(), calibrate: vi.fn(), track: vi.fn(), airwaves: vi.fn(), markMine: vi.fn(), listening: vi.fn(), ask: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))
vi.mock('vue-router', async (orig) => ({ ...(await orig<typeof import('vue-router')>()), useRouter: () => ({ push: vi.fn() }) }))

import RadioClaim from '../../../components/radio/RadioClaim.vue'

const dev = (over: Partial<RadioLiveDevice>): RadioLiveDevice => ({
  id: 'aa', mine: false, class: 'neighbour', dbm: -70, near_m: 5, far_m: 20, ring: 'house', angle: 90, trend: 'steady', channel: 6, heard: true, ...over,
})
const view = (devices: RadioLiveDevice[]): RadioLive => ({
  listening: true, cycles: 4, model: { ref_24_dbm: -40, ref_5_dbm: -47, calibrated: false, provisional: true }, devices, rings: {}, cycle_seconds: 300, last_cycle_at: 0, listened_cycles: 5, min_listened_cycles: 12,
})

// The radio polls on timers; a wrapper left mounted would tick after the environment is gone.
const mounted: Array<{ unmount: () => void }> = []
afterEach(() => { mounted.splice(0).forEach((w) => w.unmount()) })

beforeEach(() => {
  vi.clearAllMocks()
  service.live.mockResolvedValue(view([
    dev({ id: 'a1', class: 'infrastructure', channel: 60, near_m: 2, far_m: 6, last_at: 1 }),
    dev({ id: 'b2', class: 'neighbour', channel: 60, near_m: 8, far_m: 30, last_at: 1 }),
    dev({ id: 'c3', class: 'neighbour', channel: 11, near_m: 9, far_m: 33, last_at: 1 }),
  ]))
  service.markMine.mockResolvedValue({ label: 'x' })
  service.ask.mockResolvedValue({ devices: [], learned: { mine: 0, not_mine: 0 } })
})

async function show() {
  const w = mount(RadioClaim, { global: { stubs: { RouterLink: { props: ['to'], template: '<a :data-to="JSON.stringify(to)"><slot /></a>' } } } })
  mounted.push(w)
  await flushPromises()
  return w
}

describe('RadioClaim', () => {
  it('says what it is for and holds both ways to claim a device in one place', async () => {
    const w = await show()
    expect(w.find('[data-test="radio-claim"]').exists()).toBe(true)
    expect(w.find('[data-test="radio-find-open"]').exists()).toBe(true)
  })

  it('finds my device: lists the strongest devices heard now and names the chosen one', async () => {
    const w = await show()
    await w.get('[data-test="radio-find-open"]').trigger('click')
    const found = w.findAll('[data-test="radio-find-candidate"]')
    expect(found).toHaveLength(3)
    await found[0]!.get('button').trigger('click')
    await w.get('[data-test="radio-find-input"]').setValue('my tablet')
    await w.get('[data-test="radio-find-save"]').trigger('submit')
    await flushPromises()
    expect(service.markMine).toHaveBeenCalledTimes(1)
    expect(service.markMine.mock.calls[0]![1]).toBe('my tablet')
  })
})
