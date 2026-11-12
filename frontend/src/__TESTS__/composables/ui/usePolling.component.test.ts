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

  // A tab that sat in the background (a phone with the screen off) has stale data and slow timers when it returns: the poll runs
  // at once instead of waiting for its next tick.
  it('refreshes at once when the tab becomes visible again', () => {
    vi.useFakeTimers()
    const task = vi.fn()
    const wrapper = mount(hostWith(task))
    const visibility = vi.spyOn(document, 'visibilityState', 'get')

    visibility.mockReturnValue('hidden')
    document.dispatchEvent(new Event('visibilitychange'))
    expect(task).not.toHaveBeenCalled()

    visibility.mockReturnValue('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    expect(task).toHaveBeenCalledTimes(1)

    wrapper.unmount()
    document.dispatchEvent(new Event('visibilitychange'))
    expect(task).toHaveBeenCalledTimes(1)
    visibility.mockRestore()
  })

  it('does not refresh on return while a kept-alive view is in the background', async () => {
    vi.useFakeTimers()
    const task = vi.fn()
    const Poller = hostWith(task)
    const shown = ref(true)
    mount(defineComponent({ render: () => h(KeepAlive, null, [shown.value ? h(Poller) : h('p')]) }))
    shown.value = false
    await nextTick()
    document.dispatchEvent(new Event('visibilitychange'))
    expect(task).not.toHaveBeenCalled()
  })
})
