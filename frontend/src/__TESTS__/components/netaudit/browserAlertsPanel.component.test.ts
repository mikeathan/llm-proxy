import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

// A storage of its own per case: the switch is kept in the browser, and a real localStorage would carry one case's "on" into the next.
function freshStorage(): Storage {
  const data = new Map<string, string>()
  return {
    get length() { return data.size },
    clear: () => data.clear(),
    getItem: (key: string) => data.get(key) ?? null,
    key: (index: number) => [...data.keys()][index] ?? null,
    removeItem: (key: string) => { data.delete(key) },
    setItem: (key: string, value: string) => { data.set(key, value) },
  }
}

// The panel shares one app-wide instance, so each case loads a fresh module with its own fake Notification and storage.
async function mountPanel(notification: unknown) {
  vi.resetModules()
  vi.stubGlobal('localStorage', freshStorage())
  vi.stubGlobal('Notification', notification)
  const { default: BrowserAlertsPanel } = await import('../../../components/netaudit/BrowserAlertsPanel.vue')
  const w = mount(BrowserAlertsPanel)
  await flushPromises()
  return w
}

describe('BrowserAlertsPanel', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('does not ask the browser for permission just by being shown', async () => {
    const requestPermission = vi.fn(async () => 'granted')
    await mountPanel(Object.assign(vi.fn(), { permission: 'default', requestPermission }))
    expect(requestPermission).not.toHaveBeenCalled()
  })

  it('asks when the switch is turned on, and turns on once allowed', async () => {
    const requestPermission = vi.fn(async () => 'granted')
    const w = await mountPanel(Object.assign(vi.fn(), { permission: 'default', requestPermission }))
    await w.get('[role="switch"]').setValue(true)
    await flushPromises()
    expect(requestPermission).toHaveBeenCalledTimes(1)
    expect((w.get('[role="switch"]').element as HTMLInputElement).checked).toBe(true)
    expect(w.find('[data-test="browser-alerts-problem"]').exists()).toBe(false)
  })

  it('stays off and says how to allow it when the browser refuses', async () => {
    const w = await mountPanel(Object.assign(vi.fn(), { permission: 'default', requestPermission: vi.fn(async () => 'denied') }))
    await w.get('[role="switch"]').setValue(true)
    await flushPromises()
    expect((w.get('[role="switch"]').element as HTMLInputElement).checked).toBe(false)
    expect(w.get('[data-test="browser-alerts-problem"]').text()).toContain('blocked')
  })

  it('says so, with the switch disabled, when the browser has no notifications', async () => {
    const w = await mountPanel(undefined)
    expect(w.get('[role="switch"]').attributes('disabled')).toBeDefined()
    expect(w.get('[data-test="browser-alerts-problem"]').text()).toContain('does not support')
  })

  it('tells the person it does not work once the browser is closed', async () => {
    const w = await mountPanel(Object.assign(vi.fn(), { permission: 'granted', requestPermission: vi.fn() }))
    expect(w.text()).toContain('once it is closed nothing is shown')
  })
})
