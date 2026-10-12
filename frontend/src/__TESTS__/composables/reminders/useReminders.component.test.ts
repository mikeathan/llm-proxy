import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { defineComponent, h } from 'vue'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { ReminderList } from '../../../types/reminders'

const { listMock, editMock, removeMock } = vi.hoisted(() => ({ listMock: vi.fn(), editMock: vi.fn(), removeMock: vi.fn() }))
vi.mock('../../../services/reminders/reminderService', () => ({ ReminderService: { list: listMock, edit: editMock, remove: removeMock } }))

import { useReminders } from '../../../composables/reminders/useReminders'
import { POLL_INTERVAL_MS } from '../../../constants/api'

enableAutoUnmount(afterEach)

const LIST: ReminderList = {
  open: [{ id: 1, message: 'a', fire_at: '2099-01-01T00:00:00Z', timezone: 'UTC', connector: '', channel: 'in-app', status: 'scheduled' }],
  recent: [],
}

// A component that just uses the composable, exposing what it returned.
function consumer() {
  let api!: ReturnType<typeof useReminders>
  const w = mount(defineComponent({ setup() { api = useReminders(); return () => h('div') } }))
  return { w, api: () => api }
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  listMock.mockResolvedValue(LIST)
})
afterEach(() => vi.useRealTimers())

describe('useReminders (shared)', () => {
  it('loads when the first consumer appears and shares the result', async () => {
    const a = consumer()
    await flushPromises()
    expect(a.api().reminders.value).toEqual(LIST)
    expect(a.api().loading.value).toBe(false)
    const b = consumer()
    expect(b.api().reminders.value).toEqual(LIST) // already there: no empty flash for the second consumer
    expect(b.api().reminders).toBe(a.api().reminders)
  })

  it('polls with one timer however many consumers there are', async () => {
    consumer()
    consumer()
    consumer()
    await flushPromises()
    listMock.mockClear()
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS)
    expect(listMock).toHaveBeenCalledTimes(1)
  })

  it('stops polling and drops its data when the last consumer goes', async () => {
    const a = consumer()
    const b = consumer()
    await flushPromises()
    a.w.unmount()
    listMock.mockClear()
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS)
    expect(listMock).toHaveBeenCalledTimes(1) // b is still watching
    b.w.unmount()
    listMock.mockClear()
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 3)
    expect(listMock).not.toHaveBeenCalled()
    const c = consumer()
    expect(c.api().loading.value).toBe(true) // nothing stale from before
  })

  it('pauses while the tab is hidden and refreshes when it is shown again', async () => {
    consumer()
    await flushPromises()
    const state = vi.spyOn(document, 'visibilityState', 'get')
    state.mockReturnValue('hidden')
    document.dispatchEvent(new Event('visibilitychange'))
    listMock.mockClear()
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 3)
    expect(listMock).not.toHaveBeenCalled()
    state.mockReturnValue('visible')
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(listMock).toHaveBeenCalledTimes(1)
    state.mockRestore()
  })

  it('refreshes the list after a successful change and reports a refused one', async () => {
    const a = consumer()
    await flushPromises()
    listMock.mockClear()
    editMock.mockResolvedValue({})
    expect(await a.api().save(1, { message: 'b', connector: '' })).toBe(true)
    expect(listMock).toHaveBeenCalledTimes(1)

    editMock.mockRejectedValue(new Error('reminder time is in the past'))
    expect(await a.api().save(1, { message: 'b', connector: '' })).toBe(false)
    expect(a.api().actionError.value).toBe('reminder time is in the past')
  })

  it('says why the list could not be read', async () => {
    listMock.mockRejectedValue(new Error('failed to list reminders'))
    const a = consumer()
    await flushPromises()
    expect(a.api().error.value).toBe('failed to list reminders')
  })
})
