import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import type { AuthStatus } from '../../../types/auth'

const service = vi.hoisted(() => ({ status: vi.fn(), login: vi.fn(), setup: vi.fn(), logout: vi.fn(), reauth: vi.fn() }))
vi.mock('../../../services/auth/authService', () => ({ AuthService: service }))

import { useAuth } from '../../../composables/auth/useAuth'
import { useReauth } from '../../../composables/auth/useReauth'
import { startAuth } from '../../../composables/auth/startAuth'
import { apiFetch, setReauthHandler, setUnauthorizedHandler } from '../../../services/httpClient'

const status = (over: Partial<AuthStatus> = {}): AuthStatus => ({ required: true, setup_open: false, authenticated: false, elevated: false, encrypted: false, ...over })

beforeEach(() => {
  for (const fn of Object.values(service)) fn.mockReset()
  useAuth().status.value = null
  setUnauthorizedHandler(null)
  setReauthHandler(null)
})

describe('useAuth', () => {
  it('reads the state once and remembers it', async () => {
    service.status.mockResolvedValue(status({ required: false }))
    const auth = useAuth()
    await auth.ensureStatus()
    await auth.ensureStatus()
    expect(service.status).toHaveBeenCalledTimes(1)
    expect(auth.status.value?.required).toBe(false)
  })

  it('leaves the state unknown when it cannot be read, so the pages load and the API says no itself', async () => {
    service.status.mockRejectedValue(new Error('down'))
    expect(await useAuth().refresh()).toBeNull()
  })

  it('signs in and reads the state again', async () => {
    service.login.mockResolvedValue({})
    service.status.mockResolvedValue(status({ authenticated: true }))
    await useAuth().login('a long enough passphrase')
    expect(service.login).toHaveBeenCalledWith('a long enough passphrase')
    expect(useAuth().status.value?.authenticated).toBe(true)
  })

  it('lets a failed sign-in reach the caller to show', async () => {
    service.login.mockRejectedValue(new Error('That passphrase is not right.'))
    await expect(useAuth().login('x')).rejects.toThrow('not right')
  })

  it('marks the session ended without asking the server', () => {
    const auth = useAuth()
    auth.status.value = status({ required: false, authenticated: true, elevated: true })
    auth.markSignedOut()
    expect(auth.status.value).toMatchObject({ required: true, authenticated: false, elevated: false })
  })

  it('checkSession sends the user to sign in only when the server says the session ended', async () => {
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    service.status.mockResolvedValue(status({ authenticated: true }))
    await useAuth().checkSession()
    expect(handler).not.toHaveBeenCalled()
    service.status.mockResolvedValue(status({ authenticated: false }))
    await useAuth().checkSession()
    expect(handler).toHaveBeenCalledTimes(1)
    service.status.mockResolvedValue(status({ required: false }))
    await useAuth().checkSession()
    expect(handler).toHaveBeenCalledTimes(1)
  })
})

describe('useReauth', () => {
  it('resolves true when the passphrase is confirmed', async () => {
    service.reauth.mockResolvedValue({})
    const r = useReauth()
    const asked = r.requestReauth()
    expect(r.open.value).toBe(true)
    await r.submit('a long enough passphrase')
    expect(await asked).toBe(true)
    expect(r.open.value).toBe(false)
  })

  it('stays open with the reason when the passphrase is wrong, and resolves false on cancel', async () => {
    service.reauth.mockRejectedValue(new Error('That passphrase is not right.'))
    const r = useReauth()
    const asked = r.requestReauth()
    await r.submit('wrong wrong wrong')
    expect(r.open.value).toBe(true)
    expect(r.error.value).toContain('not right')
    r.cancel()
    expect(await asked).toBe(false)
  })

  it('shares one prompt between requests that ask together', async () => {
    const r = useReauth()
    const a = r.requestReauth()
    const b = r.requestReauth()
    expect(a).toBe(b)
    r.cancel()
    await a
  })
})

describe('startAuth', () => {
  function appRouter() {
    const page = { template: '<div />' }
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'overview', component: page },
        { path: '/workspaces', name: 'workspaces', component: page },
        { path: '/login', name: 'login', component: page, meta: { public: true } },
        { path: '/setup', name: 'setup', component: page, meta: { public: true } },
      ],
    })
    startAuth(router)
    return router
  }

  it('sends someone who must sign in to the login page, remembering the page', async () => {
    service.status.mockResolvedValue(status())
    const router = appRouter()
    await router.push('/workspaces')
    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query.next).toBe('/workspaces')
  })

  it('lets everything through while sign-in is not set up', async () => {
    service.status.mockResolvedValue(status({ required: false }))
    const router = appRouter()
    await router.push('/workspaces')
    expect(router.currentRoute.value.name).toBe('workspaces')
  })

  it('lets pages load when the state cannot be read', async () => {
    service.status.mockRejectedValue(new Error('down'))
    const router = appRouter()
    await router.push('/workspaces')
    expect(router.currentRoute.value.name).toBe('workspaces')
  })

  it('shows the setup page when a setup code was made after the app loaded', async () => {
    // The state is read once at start; the code is made on the server later, while the page is open. Following the link to
    // the setup page must ask the server again, not trust what it said before the code existed.
    service.status.mockResolvedValueOnce(status({ required: false, setup_open: false }))
    const router = appRouter()
    await router.push('/workspaces')
    expect(router.currentRoute.value.name).toBe('workspaces')
    service.status.mockResolvedValue(status({ required: false, setup_open: true }))
    await router.push('/setup')
    expect(router.currentRoute.value.name).toBe('setup')
  })

  it('shows the login page after sign-in was set up while the app was open', async () => {
    service.status.mockResolvedValueOnce(status({ required: false }))
    const router = appRouter()
    await router.push('/workspaces')
    service.status.mockResolvedValue(status({ required: true }))
    await router.push('/login')
    expect(router.currentRoute.value.name).toBe('login')
  })

  it('goes to the login page when any request is answered 401', async () => {
    service.status.mockResolvedValue(status({ authenticated: true }))
    const router = appRouter()
    await router.push('/workspaces')
    expect(router.currentRoute.value.name).toBe('workspaces')
    service.status.mockResolvedValue(status({ authenticated: false })) // what the server now says: the session ended
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{}', { status: 401 })))
    await apiFetch('/admin/api/state')
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query.next).toBe('/workspaces')
    vi.unstubAllGlobals()
  })

  it('asks for the passphrase again when a destructive action needs it, then repeats the action', async () => {
    service.status.mockResolvedValue(status({ authenticated: true }))
    service.reauth.mockResolvedValue({})
    appRouter()
    const calls = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: 'reauth_required' }), { status: 403 }))
      .mockResolvedValueOnce(new Response('{}', { status: 200 }))
    vi.stubGlobal('fetch', calls)
    const done = apiFetch('/admin/api/system/wipeout', { method: 'POST', body: '{}' })
    await flushPromises()
    expect(useReauth().open.value).toBe(true)
    await useReauth().submit('a long enough passphrase')
    expect((await done).status).toBe(200)
    expect(calls).toHaveBeenCalledTimes(2)
    vi.unstubAllGlobals()
  })
})
