import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import type { ReminderItem, ReminderList } from '../../types/reminders'

const { listMock, editMock, removeMock, confirmMock, createMock, pauseMock, resumeMock, skipMock } = vi.hoisted(() => ({
  listMock: vi.fn(), editMock: vi.fn(), removeMock: vi.fn(), confirmMock: vi.fn(),
  createMock: vi.fn(), pauseMock: vi.fn(), resumeMock: vi.fn(), skipMock: vi.fn(),
}))
vi.mock('../../services/reminders/reminderService', () => ({
  ReminderService: { list: listMock, edit: editMock, remove: removeMock, create: createMock, pause: pauseMock, resume: resumeMock, skip: skipMock },
}))
vi.mock('../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm: confirmMock }) }))
vi.mock('../../composables/models/useModels', () => ({
  useModels: () => ({
    state: ref({ config: { communication: { connectors: {
      'my-telegram': { type: 'telegram', enabled: true, settings: {} },
      'off-slack': { type: 'slack', enabled: false, settings: {} },
    } } } }),
  }),
}))
vi.mock('../../services/automation/dispatcherService', () => ({ DispatcherService: { previewSchedule: vi.fn() } }))

import RemindersView from '../../views/RemindersView.vue'

const item = (over: Partial<ReminderItem>): ReminderItem => ({
  id: 1, message: 'Call the bank', fire_at: '2099-10-12T08:00:00Z', timezone: 'Europe/London',
  connector: '', channel: 'telegram', status: 'scheduled', ...over,
})

async function mountView(list: ReminderList) {
  listMock.mockResolvedValue(list)
  const w = mount(RemindersView, { global: { stubs: { Icon: true, BrandMark: true } } })
  await flushPromises()
  return w
}
const buttonByName = (w: ReturnType<typeof mount>, name: RegExp) => w.findAll('button').find((b) => name.test(b.attributes('aria-label') ?? b.text()))

enableAutoUnmount(afterEach)

beforeEach(() => {
  vi.clearAllMocks()
  confirmMock.mockResolvedValue(true)
})

describe('RemindersView', () => {
  it('lists upcoming reminders with when, channel and repeat, and history apart', async () => {
    const w = await mountView({
      open: [item({}), item({ id: 2, message: 'Standup', cron: '0 9 * * 1', channel: 'in-app', connector: 'in-app' })],
      recent: [item({ id: 3, message: 'Old one', status: 'failed', last_error: 'connector down' }), item({ id: 4, message: 'Sent one', status: 'done' })],
    })
    expect(w.text()).toContain('Call the bank')
    expect(w.text()).toContain('telegram')
    expect(w.text()).toContain('Standup')
    expect(w.text()).toContain('Every Monday at 09:00')
    expect(w.text()).toContain('Old one')
    expect(w.text()).toContain('connector down')
    expect(w.text()).toMatch(/sent/i)
  })

  it('shows messages as plain text', async () => {
    const w = await mountView({ open: [item({ message: '<img src=x onerror=alert(1)>' })], recent: [] })
    expect(w.find('img').exists()).toBe(false)
    expect(w.text()).toContain('<img src=x onerror=alert(1)>')
  })

  it('invites the user to ask the assistant when there is nothing', async () => {
    const w = await mountView({ open: [], recent: [] })
    expect(w.text()).toMatch(/no reminders/i)
    expect(w.text()).toMatch(/remind me/i)
  })

  it('says so when the list cannot be read', async () => {
    listMock.mockRejectedValue(new Error('failed to list reminders'))
    const w = mount(RemindersView, { global: { stubs: { Icon: true, BrandMark: true } } })
    await flushPromises()
    expect(w.text()).toContain('failed to list reminders')
    expect(w.text()).not.toMatch(/no reminders/i)
  })

  it('edits a reminder: opens the form, saves, and shows the refreshed list', async () => {
    const w = await mountView({ open: [item({})], recent: [] })
    await buttonByName(w, /edit/i)!.trigger('click')
    const message = w.find('textarea')
    expect((message.element as HTMLTextAreaElement).value).toBe('Call the bank')
    // channels offered: the default, the web UI alone, and only the enabled connector types
    const channels = w.findAll('select').find((s) => s.attributes('aria-label') === 'Channel')!
    expect(channels.findAll('option').map((o) => [o.attributes('value'), o.text()])).toEqual([
      ['', 'Default channel'],
      ['in-app', 'Web UI only'],
      ['telegram', 'telegram'],
    ])

    editMock.mockResolvedValue(item({ message: 'Call the bank today' }))
    listMock.mockResolvedValue({ open: [item({ message: 'Call the bank today' })], recent: [] })
    await message.setValue('Call the bank today')
    await w.find('form').trigger('submit')
    await flushPromises()
    expect(editMock).toHaveBeenCalledWith(1, expect.objectContaining({ message: 'Call the bank today', fire_at: '2099-10-12T08:00:00.000Z', connector: '' }))
    expect(w.find('form').exists()).toBe(false)
    expect(w.text()).toContain('Call the bank today')
  })

  it("keeps the form open and shows the server's reason when the edit is refused", async () => {
    const w = await mountView({ open: [item({})], recent: [] })
    await buttonByName(w, /edit/i)!.trigger('click')
    editMock.mockRejectedValue(new Error('invalid reminder: reminder time is in the past'))
    await w.find('form').trigger('submit')
    await flushPromises()
    expect(w.find('form').exists()).toBe(true)
    expect(w.get('[role="alert"]').text()).toContain('reminder time is in the past')
  })

  it('cancelling the form changes nothing', async () => {
    const w = await mountView({ open: [item({})], recent: [] })
    await buttonByName(w, /edit/i)!.trigger('click')
    await w.findAll('button').find((b) => /^cancel$/i.test(b.text()))!.trigger('click')
    expect(w.find('form').exists()).toBe(false)
    expect(editMock).not.toHaveBeenCalled()
  })

  it('deletes only after the user confirms', async () => {
    const w = await mountView({ open: [item({})], recent: [] })
    confirmMock.mockResolvedValueOnce(false)
    await buttonByName(w, /delete/i)!.trigger('click')
    await flushPromises()
    expect(removeMock).not.toHaveBeenCalled()

    removeMock.mockResolvedValue(undefined)
    listMock.mockResolvedValue({ open: [], recent: [] })
    await buttonByName(w, /delete/i)!.trigger('click')
    await flushPromises()
    expect(confirmMock).toHaveBeenCalledWith(expect.objectContaining({ type: 'warning' }))
    expect(removeMock).toHaveBeenCalledWith(1)
    expect(w.text()).not.toContain('Call the bank')
  })

  it('deletes a finished reminder from the history', async () => {
    const w = await mountView({ open: [], recent: [item({ id: 9, status: 'done', message: 'Sent one' })] })
    removeMock.mockResolvedValue(undefined)
    await buttonByName(w, /delete/i)!.trigger('click')
    await flushPromises()
    expect(removeMock).toHaveBeenCalledWith(9)
  })

  describe('new reminder', () => {
    it('creates one from the page, in the browsers timezone', async () => {
      const w = await mountView({ open: [], recent: [] })
      await w.findAll('button').find((b) => /new reminder/i.test(b.text()))!.trigger('click')
      expect(w.find('form').exists()).toBe(true)
      createMock.mockResolvedValue(item({ id: 12, message: 'Renew passport' }))
      listMock.mockResolvedValue({ open: [item({ id: 12, message: 'Renew passport' })], recent: [] })
      await w.find('textarea').setValue('Renew passport')
      await w.find('form').trigger('submit')
      await flushPromises()
      expect(createMock).toHaveBeenCalledWith(expect.objectContaining({
        message: 'Renew passport', connector: '', timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
        fire_at: expect.stringMatching(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:00\.000Z$/),
      }))
      expect(w.find('form').exists()).toBe(false)
      expect(w.text()).toContain('Renew passport')
    })

    it('offers it from an empty page too, and cancelling closes it', async () => {
      const w = await mountView({ open: [], recent: [] })
      expect(w.text()).toMatch(/no reminders/i)
      await w.findAll('button').find((b) => /new reminder/i.test(b.text()))!.trigger('click')
      await w.findAll('button').find((b) => /^cancel$/i.test(b.text()))!.trigger('click')
      expect(w.find('form').exists()).toBe(false)
      expect(createMock).not.toHaveBeenCalled()
    })
  })

  describe('reschedule', () => {
    it.each(['done', 'missed', 'failed'] as const)('re-arms a %s reminder from the history', async (status) => {
      const w = await mountView({ open: [], recent: [item({ id: 9, status, message: 'Old one', fire_at: '2026-10-01T08:00:00Z' })] })
      await buttonByName(w, /reschedule/i)!.trigger('click')
      expect(w.find('button[type="submit"]').text()).toBe('Reschedule')
      editMock.mockResolvedValue(item({ id: 9 }))
      await w.find('input[type="datetime-local"]').setValue('2099-11-01T10:00')
      await w.find('form').trigger('submit')
      await flushPromises()
      expect(editMock).toHaveBeenCalledWith(9, expect.objectContaining({ message: 'Old one', fire_at: expect.stringContaining('2099-11-01') }))
    })
  })

  describe('repeating reminders', () => {
    const standup = (over: Partial<ReminderItem> = {}) => item({ id: 5, message: 'Standup', cron: '0 9 * * 1', ...over })

    it('pauses and skips a repeating reminder, and not a one-off', async () => {
      const w = await mountView({ open: [standup(), item({ id: 6, message: 'One-off' })], recent: [] })
      expect(w.findAll('button').filter((b) => /pause reminder/i.test(b.attributes('aria-label') ?? ''))).toHaveLength(1)
      expect(w.findAll('button').filter((b) => /skip next/i.test(b.attributes('aria-label') ?? ''))).toHaveLength(1)

      pauseMock.mockResolvedValue(standup({ status: 'paused' }))
      await buttonByName(w, /pause reminder/i)!.trigger('click')
      await flushPromises()
      expect(pauseMock).toHaveBeenCalledWith(5)

      skipMock.mockResolvedValue(standup())
      await buttonByName(w, /skip next/i)!.trigger('click')
      await flushPromises()
      expect(skipMock).toHaveBeenCalledWith(5)
    })

    it('shows a paused reminder as paused and resumes it', async () => {
      const w = await mountView({ open: [standup({ status: 'paused' })], recent: [] })
      expect(w.text()).toMatch(/paused/i)
      expect(buttonByName(w, /skip next/i)).toBeUndefined() // nothing to skip while paused
      resumeMock.mockResolvedValue(standup())
      await buttonByName(w, /resume reminder/i)!.trigger('click')
      await flushPromises()
      expect(resumeMock).toHaveBeenCalledWith(5)
    })

    it("tells the user why a pause or skip was refused", async () => {
      const w = await mountView({ open: [standup()], recent: [] })
      skipMock.mockRejectedValue(new Error('that is its last occurrence: delete it instead'))
      await buttonByName(w, /skip next/i)!.trigger('click')
      await flushPromises()
      expect(w.get('[role="alert"]').text()).toContain('last occurrence')
    })
  })
})
