import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'

const service = vi.hoisted(() => ({ latest: vi.fn(), markRead: vi.fn() }))
vi.mock('../../../services/newsroom/briefService', () => ({ BriefService: service }))

import { resetBriefUnread, useBriefUnread } from '../../../composables/newsroom/useBriefUnread'

const entry = (over = {}) => ({ edition_id: 2, name: 'Morning', issue_id: 5, made_at: '2026-11-23T06:00:00Z', read: false, ...over })
let seen: ReturnType<typeof useBriefUnread>
const Probe = defineComponent({ setup() { seen = useBriefUnread(); return () => h('i') } })

beforeEach(() => { vi.clearAllMocks(); resetBriefUnread() })

describe('useBriefUnread', () => {
  it('counts the editions whose newest issue is unread', async () => {
    service.latest.mockResolvedValue([entry(), entry({ edition_id: 3, issue_id: 6, read: true })])
    mount(Probe)
    await flushPromises()
    expect(seen.unreadCount.value).toBe(1)
    expect(seen.isUnread(2)).toBe(true)
    expect(seen.isUnread(3)).toBe(false)
  })

  it('names the page a critical fact of an unread edition belongs to, until it is read', async () => {
    service.latest.mockResolvedValue([entry({ critical: ['radio_open_or_weak_ap', 'network_unavailable'] }), entry({ edition_id: 3, issue_id: 6, read: true, critical: ['radio_rogue_twin'] })])
    mount(Probe)
    await flushPromises()
    expect(seen.criticalPages.value).toEqual(['radio', 'network'])
    service.markRead.mockResolvedValue(undefined)
    await seen.markRead(5)
    expect(seen.criticalPages.value).toEqual([])
  })

  it('shows nothing, and no error, when the server cannot answer', async () => {
    service.latest.mockRejectedValue(new Error('Briefs is switched off'))
    mount(Probe)
    await flushPromises()
    expect(seen.unreadCount.value).toBe(0)
    expect(seen.loaded.value).toBe(true)
  })

  it('shares one poller between components', async () => {
    service.latest.mockResolvedValue([])
    const a = mount(Probe)
    const b = mount(Probe)
    await flushPromises()
    expect(service.latest).toHaveBeenCalledTimes(1)
    a.unmount()
    b.unmount()
  })

  it('takes the dot away at once and tells the server; reading again asks nothing more', async () => {
    service.latest.mockResolvedValue([entry()])
    service.markRead.mockResolvedValue(undefined)
    mount(Probe)
    await flushPromises()
    await seen.markRead(5)
    expect(seen.unreadCount.value).toBe(0)
    expect(service.markRead).toHaveBeenCalledWith(5)
    await seen.markRead(5)
    expect(service.markRead).toHaveBeenCalledTimes(1)
  })

  it('reads an issue the poll has not seen yet, then asks again so no dot waits', async () => {
    service.latest.mockResolvedValueOnce([]).mockResolvedValueOnce([entry({ issue_id: 9, read: true })])
    service.markRead.mockResolvedValue(undefined)
    mount(Probe)
    await flushPromises()
    await seen.markRead(9)
    expect(service.markRead).toHaveBeenCalledWith(9)
    expect(service.latest).toHaveBeenCalledTimes(2)
    expect(seen.unreadCount.value).toBe(0)
  })
})
