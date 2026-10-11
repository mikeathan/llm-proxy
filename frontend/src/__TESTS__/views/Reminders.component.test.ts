import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import type { ReminderItem, ReminderList } from '../../types/reminders'

const { listMock, editMock, removeMock, confirmMock } = vi.hoisted(() => ({
  listMock: vi.fn(), editMock: vi.fn(), removeMock: vi.fn(), confirmMock: vi.fn(),
}))
vi.mock('../../services/reminders/reminderService', () => ({
  ReminderService: { list: listMock, edit: editMock, remove: removeMock },
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
    // channels offered: automatic, web UI only, and only the enabled connector types
    const channels = w.findAll('select').find((s) => s.attributes('aria-label') === 'Channel')!
    expect(channels.findAll('option').map((o) => o.attributes('value'))).toEqual(['', 'in-app', 'telegram'])

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
})
