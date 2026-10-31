import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '../../../../router'
import NotificationBell from '../../../../components/common/display/NotificationBell.vue'
import { toAutomation, toNetwork, toReminders, toSettings, toWorkspace } from '../../../../router/routes'
import type { NetworkAlertItem, ReminderAlertItem, RunEndItem, RunNotification } from '../../../../types/notifications'

const failed: RunEndItem = { id: 'ws/a@t', kind: 'automation', label: 'ws/a', workspace: 'ws', automationId: 'ws/a', outcome: 'failed', error: 'boom', target: toAutomation('ws/a'), destination: 'automations' }
const fired: ReminderAlertItem = { id: 'reminder-1-1', kind: 'reminder', message: 'call bank', event: 'due', channel: 'in-app', target: toReminders(), destination: 'reminders' }
const lost: ReminderAlertItem = { ...fired, id: 'reminder-2-1', message: 'stand up', event: 'failed' }
const ended: RunEndItem = { id: 'chat:ws@t', kind: 'assistant', label: 'ws', workspace: 'ws', outcome: 'ended', target: toWorkspace('ws'), destination: 'workspaces' }

const decoyAlert: NetworkAlertItem = { id: 'network-1', kind: 'network', alertKind: 'decoy', message: 'Network audit: a device connected to a decoy service.', delivery: 'no_connector', count: 2, repeatKey: 'decoy:m', target: toNetwork('devices'), destination: 'network' }

function mountBell(notifications: RunNotification[]) {
  return mount(NotificationBell, {
    props: { notifications },
    global: { plugins: [createAppRouter(createMemoryHistory())], stubs: { Icon: true } },
    attachTo: document.body,
  })
}

describe('NotificationBell', () => {
  it('labels the bell with the unread count and opens the panel', async () => {
    const w = mountBell([{ id: 'b1', createdAt: 0, items: [failed] }])
    const bell = w.get('button[aria-haspopup]')
    expect(bell.attributes('aria-label')).toBe('1 unread notification')
    expect(bell.attributes('aria-expanded')).toBe('false')
    await bell.trigger('click')
    expect(bell.attributes('aria-expanded')).toBe('true')
    expect(w.text()).toContain('Automation ws/a failed')
    expect(w.text()).toContain('boom')
    w.unmount()
  })

  it('shows several runs of one tick under one summary', async () => {
    const w = mountBell([{ id: 'b1', createdAt: 0, items: [failed, ended] }])
    await w.get('button[aria-haspopup]').trigger('click')
    expect(w.text()).toContain('2 runs ended, 1 failed')
    expect(w.findAll('[data-test="notification-item"]')).toHaveLength(2)
    w.unmount()
  })

  it('opens a run through its link and dismisses a notification', async () => {
    const w = mountBell([{ id: 'b1', createdAt: 0, items: [ended] }])
    await w.get('button[aria-haspopup]').trigger('click')
    const link = w.get('[data-test="notification-item"] a')
    expect(link.attributes('href')).toBe('/workspaces/ws')
    await link.trigger('click')
    expect(w.emitted('open')).toEqual([[ended]])

    await w.get('button[aria-haspopup]').trigger('click')
    await w.get('button[aria-label="Dismiss notification"]').trigger('click')
    expect(w.emitted('dismiss')).toEqual([['b1']])
    w.unmount()
  })

  it('says when there is nothing new', async () => {
    const w = mountBell([])
    expect(w.get('button[aria-haspopup]').attributes('aria-label')).toBe('No unread notifications')
    await w.get('button[aria-haspopup]').trigger('click')
    expect(w.text()).toContain('Nothing new since you looked')
    w.unmount()
  })

  it('lists a reminder that fired in words, linked to the Reminders page', async () => {
    const w = mountBell([{ id: 'b1', createdAt: 0, items: [fired] }])
    await w.get('button[aria-haspopup]').trigger('click')
    expect(w.text()).toContain('Reminder: call bank')
    expect(w.get('[data-test="notification-item"]').text()).toContain('fired')
    expect(w.get('[data-test="notification-item"] a').attributes('href')).toBe(w.vm.$router.resolve(toReminders()).href)
    w.unmount()
  })

  it('marks a reminder that did not arrive as an error and counts it with runs', async () => {
    const w = mountBell([{ id: 'b1', createdAt: 0, items: [lost] }, { id: 'b2', createdAt: 1, items: [failed] }])
    expect(w.get('button[aria-haspopup]').attributes('aria-label')).toBe('2 unread notifications')
    await w.get('button[aria-haspopup]').trigger('click')
    expect(w.text()).toContain('Reminder could not be sent: stand up')
    expect(w.findAll('.text-state-error').map((el) => el.text())).toContain('failed')
    w.unmount()
  })

  it('lists a network alert in words, linked to the Network page, and says it was not sent to a connector', async () => {
    const w = mountBell([{ id: 'b1', createdAt: 0, items: [decoyAlert] }])
    await w.get('button[aria-haspopup]').trigger('click')
    const row = w.get('[data-test="notification-item"]')
    expect(row.text()).toContain('a device connected to a decoy service')
    expect(row.text()).toContain('Raised 2 times')
    expect(row.text()).toContain('Not sent to your connectors: none is set up.')
    expect(row.get('a').attributes('href')).toBe(w.vm.$router.resolve(toNetwork('devices')).href)
    const fix = row.get('[data-test="notification-fix"]')
    expect(fix.text()).toBe('Set up a connector')
    expect(fix.attributes('href')).toBe(w.vm.$router.resolve(toSettings('communication')).href)
    await fix.trigger('click')
    expect(w.emitted('open')).toEqual([[decoyAlert]])
    w.unmount()
  })

  it('shows no fix link for an alert that was sent', async () => {
    const w = mountBell([{ id: 'b1', createdAt: 0, items: [{ ...decoyAlert, delivery: 'sent', count: 1 }] }])
    await w.get('button[aria-haspopup]').trigger('click')
    expect(w.find('[data-test="notification-fix"]').exists()).toBe(false)
    w.unmount()
  })
})
