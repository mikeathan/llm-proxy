import { describe, it, expect, vi, afterEach } from 'vitest'
import { defineComponent, h } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { NewsroomService } from '../../../services/newsroom/newsroomService'
import { resetAlertsUnread, useAlertsUnread } from '../../../composables/newsroom/useAlertsUnread'

afterEach(() => {
  resetAlertsUnread()
  vi.restoreAllMocks()
})

const Probe = defineComponent({ setup: () => useAlertsUnread(), render: () => h('div') })

describe('useAlertsUnread', () => {
  it('asks the server once however many places show the count, and again on refresh', async () => {
    const count = vi.spyOn(NewsroomService, 'alertCounts').mockResolvedValue({ unread: 3, leftOut: 2 })
    const a = mount(Probe)
    const b = mount(Probe)
    await flushPromises()
    expect(count).toHaveBeenCalledTimes(1)
    expect((a.vm as unknown as { unread: number }).unread).toBe(3)
    expect((a.vm as unknown as { leftOut: number }).leftOut).toBe(2)
    count.mockResolvedValue({ unread: 1, leftOut: 0 })
    await (b.vm as unknown as { refresh: () => Promise<void> }).refresh()
    expect((a.vm as unknown as { unread: number }).unread).toBe(1)
    a.unmount()
    b.unmount()
  })

  it('shows no count when the server cannot answer', async () => {
    vi.spyOn(NewsroomService, 'alertCounts').mockRejectedValue(new Error('off'))
    const a = mount(Probe)
    await flushPromises()
    expect((a.vm as unknown as { unread: number }).unread).toBe(0)
    a.unmount()
  })
})
