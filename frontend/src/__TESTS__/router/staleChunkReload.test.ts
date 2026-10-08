import { describe, it, expect, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { installStaleChunkReload, isChunkLoadError } from '../../router/staleChunkReload'
import type { StaleChunkReloadDeps } from '../../types/routes'

// After a deploy, a tab still running the old build asks for page chunks the server no longer has. The
// click must not silently do nothing: the app reloads once onto the new build, and never loops.

const CHROME = new TypeError('Failed to fetch dynamically imported module: http://vertex.local:4001/admin/assets/SettingsView-old.js')
const SAFARI = new TypeError('Importing a module script failed.')
const FIREFOX = new TypeError('error loading dynamically imported module: http://h/admin/assets/x.js')

describe('isChunkLoadError', () => {
  it.each([CHROME, SAFARI, FIREFOX])('recognises a failed page chunk: %s', (err) => {
    expect(isChunkLoadError(err)).toBe(true)
  })
  it('ignores other navigation errors', () => {
    expect(isChunkLoadError(new Error('Navigation cancelled'))).toBe(false)
    expect(isChunkLoadError('boom')).toBe(false)
  })
})

function harness(now = 1_000_000) {
  const store = new Map<string, string>()
  const listeners: Record<string, (e: Event) => void> = {}
  const deps: StaleChunkReloadDeps = {
    now: () => now,
    storage: { getItem: (k) => store.get(k) ?? null, setItem: (k, v) => void store.set(k, v) },
    navigate: vi.fn(),
    currentHref: () => '/admin/overview',
    addWindowListener: (type, fn) => {
      listeners[type] = fn
    },
  }
  const router = createRouter({
    history: createMemoryHistory('/admin/'),
    routes: [
      { path: '/overview', component: { template: '<div/>' } },
      { path: '/settings', component: () => Promise.reject(CHROME) },
      { path: '/broken', component: () => Promise.reject(new Error('render bug')) },
    ],
  })
  installStaleChunkReload(router, deps)
  return { router, deps, listeners, setNow: (t: number) => (now = t) }
}

describe('installStaleChunkReload', () => {
  it('reloads onto the page that was asked for when its chunk is gone', async () => {
    const { router, deps } = harness()
    await router.push('/overview')
    await router.push('/settings').catch(() => {})
    expect(deps.navigate).toHaveBeenCalledWith('/admin/settings')
  })

  it('reloads only once in a short window, so a broken deploy cannot loop', async () => {
    const h = harness()
    await h.router.push('/settings').catch(() => {})
    await h.router.push('/settings').catch(() => {})
    expect(h.deps.navigate).toHaveBeenCalledTimes(1)
    h.setNow(1_000_000 + 60_000) // a later deploy is a new incident
    await h.router.push('/settings').catch(() => {})
    expect(h.deps.navigate).toHaveBeenCalledTimes(2)
  })

  it('leaves other navigation errors alone', async () => {
    const { router, deps } = harness()
    await router.push('/broken').catch(() => {})
    expect(deps.navigate).not.toHaveBeenCalled()
  })

  it("handles Vite's preload error for chunks the page already references", () => {
    const { listeners, deps } = harness()
    const event = new Event('vite:preloadError', { cancelable: true })
    listeners['vite:preloadError']!(event)
    expect(event.defaultPrevented).toBe(true)
    expect(deps.navigate).toHaveBeenCalledWith('/admin/overview')
  })
})
