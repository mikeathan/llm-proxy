import { describe, it, expect, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, KeepAlive, nextTick, ref } from 'vue'
import { usePolling } from '../../../composables/ui/usePolling'

const INTERVAL_MS = 1000

function hostWith(task: () => void) {
  return defineComponent({
    name: 'Poller',
    setup() {
      usePolling(task, INTERVAL_MS)
      return () => h('div')
    },
  })
}

describe('usePolling', () => {
  afterEach(() => vi.useRealTimers())

  it('runs on the interval while mounted and stops on unmount', () => {
    vi.useFakeTimers()
    const task = vi.fn()
    const wrapper = mount(hostWith(task))
    vi.advanceTimersByTime(INTERVAL_MS * 2)
    expect(task).toHaveBeenCalledTimes(2)
    wrapper.unmount()
    vi.advanceTimersByTime(INTERVAL_MS * 3)
    expect(task).toHaveBeenCalledTimes(2)
  })

  it('pauses while kept alive in the background and refreshes on return', async () => {
    vi.useFakeTimers()
    const task = vi.fn()
    const Poller = hostWith(task)
    const shown = ref(true)
    mount(defineComponent({
      render: () => h(KeepAlive, null, [shown.value ? h(Poller) : h('p')]),
    }))
    vi.advanceTimersByTime(INTERVAL_MS)
    expect(task).toHaveBeenCalledTimes(1)

    shown.value = false
    await nextTick()
    vi.advanceTimersByTime(INTERVAL_MS * 5)
    expect(task).toHaveBeenCalledTimes(1)

    shown.value = true
    await nextTick()
    expect(task).toHaveBeenCalledTimes(2)
    vi.advanceTimersByTime(INTERVAL_MS)
    expect(task).toHaveBeenCalledTimes(3)
  })
})
