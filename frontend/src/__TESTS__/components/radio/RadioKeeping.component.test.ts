import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RadioKeeping } from '../../../types/radio'

const service = vi.hoisted(() => ({ keeping: vi.fn(), setKeeping: vi.fn(), dropTracks: vi.fn(), forgetNeighbours: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))

import RadioKeepingPanel from '../../../components/radio/RadioKeeping.vue'

const view = (over: Partial<RadioKeeping> = {}): RadioKeeping => ({ track_hours: 0, choices: [0, 24, 48], stored_points: 0, ...over })

beforeEach(() => {
  vi.clearAllMocks()
  service.keeping.mockResolvedValue(view())
})

async function show() {
  const w = mount(RadioKeepingPanel)
  await flushPromises()
  return w
}

describe('RadioKeeping', () => {
  it('is off by default, says what that means and offers nothing to delete', async () => {
    const w = await show()
    expect(w.get('[data-test="radio-keeping-note"]').text()).toContain('Off:')
    expect(w.find('[data-test="radio-keeping-stored"]').exists()).toBe(false)
    expect(w.findAll('[role="radio"]').map((r) => r.text())).toEqual(['Off', '24 hours', '48 hours'])
  })

  it('turns the track on for 24 hours, then shows what is stored with a way to delete it', async () => {
    service.setKeeping.mockResolvedValue(view({ track_hours: 24, stored_points: 120, oldest: '2026-11-14T10:00:00Z' }))
    const w = await show()
    await w.findAll('[role="radio"]')[1]?.trigger('click')
    await flushPromises()
    expect(service.setKeeping).toHaveBeenCalledWith(24)
    expect(w.get('[data-test="radio-keeping-stored"]').text()).toContain('120 points stored')
    service.dropTracks.mockResolvedValue(view({ track_hours: 24 }))
    await w.get('[data-test="radio-keeping-drop"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="radio-keeping-done"]').text()).toContain('deleted')
  })

  it('says the stored tracks are deleted when the track is turned off', async () => {
    service.keeping.mockResolvedValue(view({ track_hours: 48, stored_points: 5 }))
    service.setKeeping.mockResolvedValue(view())
    const w = await show()
    await w.findAll('[role="radio"]')[0]?.trigger('click')
    await flushPromises()
    expect(service.setKeeping).toHaveBeenCalledWith(0)
    expect(w.get('[data-test="radio-keeping-done"]').text()).toContain('deleted')
  })

  it('forgets the neighbours and says so', async () => {
    service.forgetNeighbours.mockResolvedValue({ forgotten: true })
    const w = await show()
    await w.get('[data-test="radio-forget"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="radio-keeping-done"]').text()).toContain('forgotten')
  })

  it('shows a refusal in a sentence', async () => {
    service.setKeeping.mockRejectedValue(new Error('tracks are kept for 24 or 48 hours, or not at all'))
    const w = await show()
    await w.findAll('[role="radio"]')[1]?.trigger('click')
    await flushPromises()
    expect(w.get('[data-test="radio-keeping-error"]').text()).toContain('24 or 48')
  })
})
