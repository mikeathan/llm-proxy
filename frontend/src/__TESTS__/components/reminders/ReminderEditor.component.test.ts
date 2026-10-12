import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ReminderEditor from '../../../components/reminders/ReminderEditor.vue'
import { fieldByLabel } from '../../helpers/fieldByLabel'
import type { ReminderItem } from '../../../types/reminders'
import type { ChoiceOption } from '../../../types/ui'

vi.mock('../../../services/automation/dispatcherService', () => ({ DispatcherService: { previewSchedule: vi.fn() } }))

const CHANNELS: ChoiceOption[] = [
  { value: '', label: 'Automatic' },
  { value: 'in-app', label: 'Web UI only' },
  { value: 'telegram', label: 'telegram' },
]

const oneOff = (over: Partial<ReminderItem> = {}): ReminderItem => ({
  id: 7, message: 'Call the bank', fire_at: '2026-10-12T08:00:00Z', timezone: 'Europe/London',
  connector: '', channel: 'telegram', status: 'scheduled', ...over,
})
const recurring = (over: Partial<ReminderItem> = {}): ReminderItem =>
  oneOff({ cron: '0 9 * * 1', message: 'Standup', ...over })

function mountEditor(reminder: ReminderItem, props: Record<string, unknown> = {}) {
  const onSave = vi.fn()
  const onCancel = vi.fn()
  const w = mount(ReminderEditor, {
    props: { reminder, channels: CHANNELS, saving: false, error: '', ...props },
    attrs: { onSave, onCancel },
    global: { stubs: { Icon: true } },
  })
  return { w, onSave, onCancel }
}

describe('ReminderEditor', () => {
  it('starts from the reminder, with the time shown in its own zone', () => {
    const { w } = mountEditor(oneOff())
    expect((fieldByLabel(w, /^message/i).element as HTMLTextAreaElement).value).toBe('Call the bank')
    expect((fieldByLabel(w, /^when/i).element as HTMLInputElement).value).toBe('2026-10-12T09:00')
    expect((fieldByLabel(w, /^channel/i).element as HTMLSelectElement).value).toBe('')
    expect(w.text()).toContain('Europe/London')
  })

  it('saves a one-off with the new text, time and channel', async () => {
    const { w, onSave } = mountEditor(oneOff())
    await fieldByLabel(w, /^message/i).setValue('Call the bank about the card')
    await fieldByLabel(w, /^when/i).setValue('2026-10-13T10:30')
    await fieldByLabel(w, /^channel/i).setValue('telegram')
    await w.get('form').trigger('submit')
    expect(onSave).toHaveBeenCalledWith({
      message: 'Call the bank about the card',
      fire_at: '2026-10-13T09:30:00.000Z', // 10:30 BST
      connector: 'telegram',
    })
  })

  it('saves a recurring reminder with its repeat and end date, never a fire time', async () => {
    const { w, onSave } = mountEditor(recurring({ until: '2026-12-31T23:59:00Z' }))
    expect((fieldByLabel(w, /ends/i).element as HTMLInputElement).value).toBe('2026-12-31')
    await fieldByLabel(w, /ends/i).setValue('2027-01-31')
    await w.get('form').trigger('submit')
    const saved = onSave.mock.calls[0]![0]
    expect(saved).toMatchObject({ message: 'Standup', cron: '0 9 * * 1', connector: '' })
    expect(saved.fire_at).toBeUndefined()
    expect(saved.until).toBe('2027-01-31T23:59:00.000Z')
  })

  it('lets a recurring reminder run without an end', async () => {
    const { w, onSave } = mountEditor(recurring({ until: '2026-12-31T23:59:00Z' }))
    await fieldByLabel(w, /ends/i).setValue('')
    await w.get('form').trigger('submit')
    expect(onSave.mock.calls[0]![0].until).toBeUndefined()
  })

  it('offers only repeat choices that make sense for a reminder', () => {
    const { w } = mountEditor(recurring())
    const kinds = w.findAll('[data-test="kind"] option').map((o) => o.attributes('value'))
    expect(kinds).toEqual(['daily', 'weekdays', 'weekly', 'monthly', 'custom'])
  })

  it('will not save an empty message or a missing time', async () => {
    const { w, onSave } = mountEditor(oneOff())
    await fieldByLabel(w, /^message/i).setValue('   ')
    expect(w.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    await w.get('form').trigger('submit')
    await fieldByLabel(w, /^message/i).setValue('ok')
    await fieldByLabel(w, /^when/i).setValue('')
    await w.get('form').trigger('submit')
    expect(onSave).not.toHaveBeenCalled()
  })

  it("shows the server's reason and disables Save while saving", () => {
    const { w } = mountEditor(oneOff(), { error: 'reminder time is in the past', saving: true })
    expect(w.get('[role="alert"]').text()).toContain('reminder time is in the past')
    expect(w.get('button[type="submit"]').attributes('disabled')).toBeDefined()
  })

  it('cancels without saving', async () => {
    const { w, onSave, onCancel } = mountEditor(oneOff())
    const cancel = w.findAll('button').find((b) => /cancel/i.test(b.text()))!
    await cancel.trigger('click')
    expect(onCancel).toHaveBeenCalled()
    expect(onSave).not.toHaveBeenCalled()
  })

  describe('repeat toggle', () => {
    it('turns a one-off into a repeating reminder, starting from its time of day', async () => {
      const { w, onSave } = mountEditor(oneOff()) // 09:00 London
      await fieldByLabel(w, /^repeat/i).setValue(true)
      expect(w.find('[data-test="kind"]').exists()).toBe(true)
      expect(w.find('input[type="datetime-local"]').exists()).toBe(false)
      await w.get('form').trigger('submit')
      const saved = onSave.mock.calls[0]![0]
      expect(saved.cron).toBe('0 9 * * *')
      expect(saved.fire_at).toBeUndefined()
    })

    it('turns a repeating reminder into a one-off at a chosen time', async () => {
      const { w, onSave } = mountEditor(recurring())
      await fieldByLabel(w, /^repeat/i).setValue(false)
      await fieldByLabel(w, /^when/i).setValue('2026-10-20T11:00')
      await w.get('form').trigger('submit')
      const saved = onSave.mock.calls[0]![0]
      expect(saved.cron).toBeUndefined()
      expect(saved.until).toBeUndefined()
      expect(saved.fire_at).toBe('2026-10-20T10:00:00.000Z') // 11:00 BST
    })
  })

  describe('creating', () => {
    const blank = (): ReminderItem => ({ id: 0, message: '', fire_at: '2026-10-13T08:00:00Z', timezone: 'Europe/London', connector: '', channel: '', status: 'scheduled' })

    it('starts empty, offers Create, and sends the zone the time is in', async () => {
      const { w, onSave } = mountEditor(blank(), { creating: true })
      expect((fieldByLabel(w, /^message/i).element as HTMLTextAreaElement).value).toBe('')
      expect(w.get('button[type="submit"]').text()).toBe('Create')
      await w.get('form').trigger('submit')
      expect(onSave).not.toHaveBeenCalled() // a message is required
      await fieldByLabel(w, /^message/i).setValue('Renew passport')
      await w.get('form').trigger('submit')
      expect(onSave.mock.calls[0]![0]).toEqual({
        message: 'Renew passport', fire_at: '2026-10-13T08:00:00.000Z', connector: '', timezone: 'Europe/London',
      })
    })

    it('does not send a zone when editing', async () => {
      const { w, onSave } = mountEditor(oneOff())
      await w.get('form').trigger('submit')
      expect(onSave.mock.calls[0]![0].timezone).toBeUndefined()
    })
  })

  it('offers Reschedule on a finished reminder', async () => {
    const { w, onSave } = mountEditor(oneOff({ status: 'missed', fire_at: '2026-10-01T08:00:00Z' }), { reschedule: true })
    expect(w.get('button[type="submit"]').text()).toBe('Reschedule')
    await fieldByLabel(w, /^when/i).setValue('2026-10-20T11:00')
    await w.get('form').trigger('submit')
    expect(onSave.mock.calls[0]![0].fire_at).toBe('2026-10-20T10:00:00.000Z')
  })
})
