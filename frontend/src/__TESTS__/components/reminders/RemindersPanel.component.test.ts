import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import RemindersPanel from '../../../components/reminders/RemindersPanel.vue'
import { ReminderService } from '../../../services/reminders/reminderService'
import type { ReminderItem, ReminderList } from '../../../types/reminders'

vi.mock('../../../services/reminders/reminderService', () => ({ ReminderService: { list: vi.fn() } }))

const item = (over: Partial<ReminderItem>): ReminderItem => ({
  id: 1, message: 'Call the bank', fire_at: '2099-10-12T08:00:00Z', timezone: 'Europe/London', connector: '', channel: 'telegram', status: 'scheduled', ...over,
})

async function mountPanel(list: ReminderList) {
  vi.mocked(ReminderService.list).mockResolvedValue(list)
  return mountAndSettle()
}

async function mountFailing(message: string) {
  vi.mocked(ReminderService.list).mockRejectedValue(new Error(message))
  return mountAndSettle()
}

async function mountAndSettle() {
  const w = mount(RemindersPanel, { global: { stubs: { Icon: true, BrandMark: true } } })
  await flushPromises()
  return w
}

beforeEach(() => {
  vi.mocked(ReminderService.list).mockReset()
})

describe('RemindersPanel', () => {
  it('lists upcoming reminders with their channel', async () => {
    const w = await mountPanel({ open: [item({}), item({ id: 2, message: 'Standup', cron: '0 9 * * 1', channel: 'in-app' })], recent: [] })
    expect(w.text()).toContain('Call the bank')
    expect(w.text()).toContain('telegram')
    expect(w.text()).toContain('Standup')
    expect(w.text()).toContain('in-app')
    expect(w.text()).toMatch(/repeats/i)
  })

  it('shows the message as plain text, never as markup', async () => {
    const w = await mountPanel({ open: [item({ message: '<img src=x onerror=alert(1)>' })], recent: [] })
    expect(w.find('img').exists()).toBe(false)
    expect(w.text()).toContain('<img src=x onerror=alert(1)>')
  })

  it('tells missed and failed reminders apart from sent ones', async () => {
    const w = await mountPanel({
      open: [],
      recent: [
        item({ id: 3, status: 'done', message: 'Sent one' }),
        item({ id: 4, status: 'missed', message: 'Missed one' }),
        item({ id: 5, status: 'failed', message: 'Failed one', last_error: 'connector down' }),
      ],
    })
    expect(w.text()).toMatch(/sent/i)
    expect(w.text()).toMatch(/missed/i)
    expect(w.text()).toMatch(/failed/i)
    expect(w.text()).toContain('connector down')
  })

  it('invites the user to ask the assistant when there is nothing', async () => {
    const w = await mountPanel({ open: [], recent: [] })
    expect(w.text()).toMatch(/no reminders/i)
    expect(w.text()).toMatch(/remind me/i)
  })

  it('reports a failed load instead of an empty list', async () => {
    const w = await mountFailing('failed to list reminders')
    expect(w.text()).toContain('failed to list reminders')
    expect(w.text()).not.toMatch(/no reminders/i)
  })
})
