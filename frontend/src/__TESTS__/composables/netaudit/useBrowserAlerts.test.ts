import { describe, it, expect, vi } from 'vitest'
import { ref } from 'vue'
import { createBrowserAlerts } from '../../../composables/netaudit/useBrowserAlerts'
import { toNetwork } from '../../../router/routes'
import type { BrowserAlertsEnv, NetworkAlertItem } from '../../../types/notifications'

const item = (id: number, over: Partial<NetworkAlertItem> = {}): NetworkAlertItem =>
  ({ id: `network-${id}`, kind: 'network', alertKind: 'decoy', message: `alert ${id}`, delivery: 'sent', count: 1, repeatKey: `decoy:alert ${id}`, target: toNetwork('devices'), destination: 'network', ...over })

function setup(over: { raiseFails?: boolean; permission?: NotificationPermission; hidden?: boolean; enabled?: boolean; ask?: NotificationPermission; supported?: boolean } = {}) {
  let permission: NotificationPermission = over.permission ?? 'granted'
  const raised: Array<{ title: string; options: NotificationOptions }> = []
  const requestPermission = vi.fn(async () => (permission = over.ask ?? 'granted'))
  const env: BrowserAlertsEnv = {
    supported: over.supported ?? true,
    permission: () => permission,
    requestPermission,
    raise: (title, options) => {
      if (over.raiseFails) throw new TypeError('Illegal constructor')
      raised.push({ title, options })
    },
    hidden: () => over.hidden ?? true,
  }
  const enabled = ref(over.enabled ?? true)
  const alerts = createBrowserAlerts(env, enabled)
  return { alerts, raised, requestPermission, enabled }
}

describe('browser alerts', () => {
  it('raises a notice with the alert\'s fixed text when the tab is hidden, the switch is on and the browser allows it', () => {
    const { alerts, raised } = setup()
    alerts.announce([item(1)])
    expect(raised).toEqual([{ title: 'Network alert', options: { body: 'alert 1', tag: 'decoy:alert 1' } }])
  })

  it('says it once for a batch', () => {
    const { alerts, raised } = setup()
    alerts.announce([item(1), item(2)])
    expect(raised).toHaveLength(1)
    expect(raised[0]!.options.body).toBe('2 network alerts')
  })

  it.each([
    ['the tab is visible', { hidden: false }],
    ['the switch is off', { enabled: false }],
    ['the browser has not been asked', { permission: 'default' as const }],
    ['the browser refused', { permission: 'denied' as const }],
    ['the browser has no notifications', { supported: false }],
  ])('raises nothing when %s', (_name, over) => {
    const { alerts, raised } = setup(over)
    alerts.announce([item(1)])
    expect(raised).toEqual([])
  })

  it('asks the browser only when the person turns the switch on', async () => {
    const { alerts, requestPermission, enabled } = setup({ permission: 'default', enabled: false })
    expect(requestPermission).not.toHaveBeenCalled()
    await alerts.enable()
    expect(requestPermission).toHaveBeenCalledTimes(1)
    expect(enabled.value).toBe(true)
    expect(alerts.problem.value).toBe('')
  })

  it('does not ask again when the browser already allowed it', async () => {
    const { alerts, requestPermission, enabled } = setup({ permission: 'granted', enabled: false })
    await alerts.enable()
    expect(requestPermission).not.toHaveBeenCalled()
    expect(enabled.value).toBe(true)
  })

  it('stays off and says how to allow it when the person refuses', async () => {
    const { alerts, enabled } = setup({ permission: 'default', enabled: false, ask: 'denied' })
    await alerts.enable()
    expect(enabled.value).toBe(false)
    expect(alerts.problem.value).toContain('blocked')
  })

  it('says when the browser cannot show notifications and does not turn on', async () => {
    const { alerts, enabled, requestPermission } = setup({ supported: false, enabled: false })
    await alerts.enable()
    expect(requestPermission).not.toHaveBeenCalled()
    expect(enabled.value).toBe(false)
    expect(alerts.problem.value).toContain('does not support')
  })

  it('turns off without touching the browser\'s permission', () => {
    const { alerts, enabled, requestPermission } = setup()
    alerts.disable()
    expect(enabled.value).toBe(false)
    expect(requestPermission).not.toHaveBeenCalled()
  })

  it('says so and does not throw when the browser refuses to show a notice from a page', () => {
    const { alerts } = setup({ raiseFails: true })
    expect(() => alerts.announce([item(1)])).not.toThrow()
    expect(alerts.problem.value).toContain('cannot show notices')
  })
})
