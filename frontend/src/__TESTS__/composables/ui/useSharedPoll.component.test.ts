import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { sharedPoll } from '../../../composables/ui/useSharedPoll'

afterEach(() => vi.useRealTimers())

describe('sharedPoll', () => {
  it('asks once for every consumer on screen, on one timer that stops with the last', () => {
    vi.useFakeTimers()
    const load = vi.fn()
    const poll = sharedPoll(load, 1000)
    const Consumer = defineComponent({ setup: () => { poll.use(); return () => null } })
    const a = mount(Consumer)
    const b = mount(Consumer)
    expect(load).toHaveBeenCalledTimes(1)
    vi.advanceTimersByTime(1000)
    expect(load).toHaveBeenCalledTimes(2)
    a.unmount()
    vi.advanceTimersByTime(1000)
    expect(load).toHaveBeenCalledTimes(3)
    b.unmount()
    vi.advanceTimersByTime(5000)
    expect(load).toHaveBeenCalledTimes(3)
  })
})
