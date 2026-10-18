import { describe, it, expect } from 'vitest'
import { describeNotification, summarizeBatch } from '../../../utils/notifications/present'
import { toAutomation, toReminders } from '../../../router/routes'
import type { ReminderAlertItem, ReminderEventKind, RunEndItem } from '../../../types/notifications'

const reminder = (event: ReminderEventKind, message = 'call bank'): ReminderAlertItem =>
  ({ id: `reminder-1-${event}`, kind: 'reminder', message, event, channel: 'in-app', target: toReminders(), destination: 'reminders' })
const run = (outcome: RunEndItem['outcome'], kind: RunEndItem['kind'] = 'automation', label = 'ws/news'): RunEndItem =>
  ({ id: `run-${label}`, kind, label, workspace: 'ws', outcome, target: toAutomation('ws/news'), destination: 'automations' })

describe('summarizeBatch', () => {
  it.each([
    ['due', 'Reminder: call bank'],
    ['late', 'Reminder (late): call bank'],
    ['partial', 'Reminder: call bank (not sent to every channel)'],
    ['failed', 'Reminder could not be sent: call bank'],
    ['missed', 'Reminder missed: call bank'],
  ] as const)('says what one %s reminder did', (event, expected) => {
    expect(summarizeBatch([reminder(event)])).toBe(expected)
  })

  it('counts several reminders and how many did not arrive', () => {
    expect(summarizeBatch([reminder('due'), reminder('late')])).toBe('2 reminders fired')
    expect(summarizeBatch([reminder('due'), reminder('failed'), reminder('missed')])).toBe('3 reminders, 2 not delivered')
  })

  it('keeps the run wording for runs', () => {
    expect(summarizeBatch([run('completed')])).toBe('Automation ws/news completed')
    expect(summarizeBatch([run('ended', 'assistant')])).toBe('Assistant run in ws ended')
    expect(summarizeBatch([run('failed'), run('completed', 'automation', 'ws/other')])).toBe('2 runs ended, 1 failed')
  })
})

describe('describeNotification', () => {
  it('spells a reminder out in words and marks what did not arrive as an error', () => {
    expect(describeNotification(reminder('due'))).toEqual({ label: 'fired', tone: 'success', text: 'call bank' })
    expect(describeNotification(reminder('late'))).toEqual({ label: 'late', tone: 'success', text: 'call bank' })
    expect(describeNotification(reminder('partial'))).toEqual({ label: 'partly sent', tone: 'warning', text: 'call bank' })
    expect(describeNotification(reminder('failed'))).toEqual({ label: 'failed', tone: 'error', text: 'call bank' })
    expect(describeNotification(reminder('missed'))).toEqual({ label: 'missed', tone: 'error', text: 'call bank' })
  })

  it('describes a run as before: its outcome, and the workspace for an assistant run', () => {
    expect(describeNotification(run('failed'))).toEqual({ label: 'failed', tone: 'error', text: 'ws/news' })
    expect(describeNotification(run('ended', 'assistant'))).toEqual({ label: 'ended', tone: 'muted', text: 'assistant · ws' })
    expect(describeNotification({ ...run('failed'), error: 'boom' })).toMatchObject({ detail: 'boom' })
  })
})
