import { describe, expect, it } from 'vitest'
import { decideRoute, safeNext } from '../../../utils/auth/guard'
import { ROUTE_NAMES } from '../../../types/routes'
import type { AuthStatus } from '../../../types/auth'

const status = (over: Partial<AuthStatus> = {}): AuthStatus => ({ required: true, setup_open: false, authenticated: false, elevated: false, encrypted: false, ...over })
const page = { name: ROUTE_NAMES.overview, meta: {}, fullPath: '/workspaces/ws/assistant?x=1' }
const login = { name: ROUTE_NAMES.login, meta: { public: true }, fullPath: '/login' }
const setup = { name: ROUTE_NAMES.setup, meta: { public: true }, fullPath: '/setup' }

describe('decideRoute', () => {
  it('lets every page through while sign-in is not set up', () => {
    expect(decideRoute(status({ required: false }), page)).toBe(true)
  })

  it('lets a signed-in user through', () => {
    expect(decideRoute(status({ authenticated: true }), page)).toBe(true)
  })

  it('sends someone who is not signed in to the login page, remembering where they were going', () => {
    expect(decideRoute(status(), page)).toEqual({ name: ROUTE_NAMES.login, query: { next: '/workspaces/ws/assistant?x=1' } })
  })

  it('shows the login page to someone who must sign in, and sends everyone else home', () => {
    expect(decideRoute(status(), login)).toBe(true)
    expect(decideRoute(status({ authenticated: true }), login)).toEqual({ path: '/' })
    expect(decideRoute(status({ required: false }), login)).toEqual({ path: '/' })
  })

  it('offers setup only while a setup code is open and no passphrase exists', () => {
    expect(decideRoute(status({ required: false, setup_open: true }), setup)).toBe(true)
    expect(decideRoute(status({ required: false, setup_open: false }), setup)).toEqual({ name: ROUTE_NAMES.overview })
    expect(decideRoute(status({ required: true }), setup)).toEqual({ name: ROUTE_NAMES.login })
    expect(decideRoute(status({ required: true, authenticated: true }), setup)).toEqual({ name: ROUTE_NAMES.overview })
  })
})

describe('safeNext', () => {
  it('keeps a path inside the app', () => {
    expect(safeNext('/workspaces/ws/assistant?x=1#a')).toBe('/workspaces/ws/assistant?x=1#a')
  })

  it('drops anything that could send the user elsewhere', () => {
    for (const bad of ['https://evil.example', '//evil.example', '/\\evil.example', 'javascript:alert(1)', '', undefined, null, 42, ['/a']]) {
      expect(safeNext(bad)).toBe('/')
    }
  })

  it('does not return to the sign-in pages', () => {
    for (const p of ['/login', '/login?next=/', '/setup', '/setup/x']) expect(safeNext(p)).toBe('/')
  })
})
