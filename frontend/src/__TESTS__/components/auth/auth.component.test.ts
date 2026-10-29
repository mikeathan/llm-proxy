import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { AuthStatus } from '../../../types/auth'

const service = vi.hoisted(() => ({
  status: vi.fn(), login: vi.fn(), setup: vi.fn(), logout: vi.fn(), reauth: vi.fn(), sessions: vi.fn(), revokeSession: vi.fn(),
  revokeAllSessions: vi.fn(), changePassphrase: vi.fn(), tokens: vi.fn(), createToken: vi.fn(), revokeToken: vi.fn(), events: vi.fn(),
}))
vi.mock('../../../services/auth/authService', () => ({ AuthService: service }))
const replace = vi.hoisted(() => vi.fn())
const routeState = vi.hoisted(() => ({ query: {} as Record<string, unknown> }))
vi.mock('vue-router', async (importOriginal) => ({ ...(await importOriginal<typeof import('vue-router')>()), useRouter: () => ({ replace, push: replace }), useRoute: () => routeState }))
const confirmMock = vi.hoisted(() => vi.fn())
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm: confirmMock }) }))

import LoginView from '../../../views/LoginView.vue'
import SetupView from '../../../views/SetupView.vue'
import ReauthDialog from '../../../components/auth/ReauthDialog.vue'
import AuthBanner from '../../../components/auth/AuthBanner.vue'
import SignInSettings from '../../../components/settings/SignInSettings.vue'
import { useAuth } from '../../../composables/auth/useAuth'
import { useReauth } from '../../../composables/auth/useReauth'

const status = (over: Partial<AuthStatus> = {}): AuthStatus => ({ required: true, setup_open: false, authenticated: false, elevated: false, encrypted: false, ...over })
const stubs = { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } }
const PASS = 'correct horse battery'

beforeEach(() => {
  for (const fn of Object.values(service)) fn.mockReset()
  replace.mockReset()
  routeState.query = {}
  confirmMock.mockReset().mockResolvedValue(true)
  service.status.mockResolvedValue(status())
  service.sessions.mockResolvedValue([])
  service.tokens.mockResolvedValue([])
  service.events.mockResolvedValue([])
  useAuth().status.value = status()
})

describe('LoginView', () => {
  it('signs in and goes to the page the user was going to', async () => {
    service.login.mockResolvedValue({})
    service.status.mockResolvedValue(status({ authenticated: true }))
    routeState.query = { next: '/workspaces/ws/assistant' }
    const w = mount(LoginView, stubs)
    await w.get('input[autocomplete="current-password"]').setValue(PASS)
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(service.login).toHaveBeenCalledWith(PASS)
    expect(replace).toHaveBeenCalledWith('/workspaces/ws/assistant')
  })

  it('never follows a next that leaves the app', async () => {
    service.login.mockResolvedValue({})
    routeState.query = { next: 'https://evil.example' }
    const w = mount(LoginView, stubs)
    await w.get('input[autocomplete="current-password"]').setValue(PASS)
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(replace).toHaveBeenCalledWith('/')
  })

  it('shows why a sign-in failed, empties the field, and stays on the page', async () => {
    service.login.mockRejectedValue(new Error('too many wrong passphrases; try again in 15 minutes'))
    const w = mount(LoginView, stubs)
    await w.get('input[autocomplete="current-password"]').setValue('wrong wrong wrong')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('try again in 15 minutes')
    expect((w.get('input[autocomplete="current-password"]').element as HTMLInputElement).value).toBe('')
    expect(replace).not.toHaveBeenCalled()
  })

  it('has a username field for password managers that no one sees, and a Show toggle', async () => {
    const w = mount(LoginView, stubs)
    const hidden = w.get('input[autocomplete="username"]')
    expect(hidden.classes()).toContain('sr-only')
    expect(hidden.attributes('aria-hidden')).toBe('true')
    const field = w.get('input[autocomplete="current-password"]')
    expect(field.attributes('type')).toBe('password')
    await w.findAll('button').find((b) => b.text() === 'Show')!.trigger('click')
    expect(field.attributes('type')).toBe('text')
  })

  it('does not send an empty passphrase', async () => {
    const w = mount(LoginView, stubs)
    await w.get('form').trigger('submit')
    expect(service.login).not.toHaveBeenCalled()
    expect(w.get('button[type="submit"]').attributes('disabled')).toBeDefined()
  })
})

describe('SetupView', () => {
  async function fill(w: ReturnType<typeof mount>, code: string, pass: string, repeat: string) {
    await w.get('input[autocomplete="one-time-code"]').setValue(code)
    const fields = w.findAll('input[autocomplete="new-password"]')
    await fields[0]!.setValue(pass)
    await fields[1]!.setValue(repeat)
  }

  it('sets the passphrase from the code and goes in', async () => {
    service.setup.mockResolvedValue({})
    const w = mount(SetupView, stubs)
    await fill(w, ' abcd-2345 ', PASS, PASS)
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(service.setup).toHaveBeenCalledWith('abcd-2345', PASS)
    expect(replace).toHaveBeenCalledWith('/')
  })

  it('says what is wrong before asking: too short, or the two differ', async () => {
    const w = mount(SetupView, stubs)
    await fill(w, 'ABCD-2345', 'short', 'shorter')
    expect(w.text()).toContain('at least 12')
    expect(w.text()).toContain('do not match')
    expect(w.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    await w.get('form').trigger('submit')
    expect(service.setup).not.toHaveBeenCalled()
  })

  it('shows the server\'s reason when the code is wrong', async () => {
    service.setup.mockRejectedValue(new Error('that setup code is not right'))
    const w = mount(SetupView, stubs)
    await fill(w, 'ABCD-2345', PASS, PASS)
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(w.text()).toContain('that setup code is not right')
    expect(replace).not.toHaveBeenCalled()
  })
})

describe('ReauthDialog', () => {
  it('is hidden until something needs the passphrase, then confirms it and lets the action go on', async () => {
    service.reauth.mockResolvedValue({})
    const w = mount(ReauthDialog)
    expect(w.find('[role="dialog"]').exists()).toBe(false)
    const asked = useReauth().requestReauth()
    await flushPromises()
    await w.get('input[autocomplete="current-password"]').setValue(PASS)
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(service.reauth).toHaveBeenCalledWith(PASS)
    expect(await asked).toBe(true)
    expect(w.find('[role="dialog"]').exists()).toBe(false)
  })

  it('cancels without confirming', async () => {
    const w = mount(ReauthDialog)
    const asked = useReauth().requestReauth()
    await flushPromises()
    await w.findAll('button').find((b) => b.text() === 'Cancel')!.trigger('click')
    expect(await asked).toBe(false)
    expect(service.reauth).not.toHaveBeenCalled()
  })

  it('stays open and says why when the passphrase is wrong', async () => {
    service.reauth.mockRejectedValue(new Error('That passphrase is not right.'))
    const w = mount(ReauthDialog)
    const asked = useReauth().requestReauth()
    await flushPromises()
    await w.get('input[autocomplete="current-password"]').setValue('wrong wrong wrong')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('not right')
    useReauth().cancel()
    await asked
  })
})

describe('AuthBanner', () => {
  it('shows while sign-in is not set up, mentioning an unencrypted connection, and links to the setting', () => {
    useAuth().status.value = status({ required: false, encrypted: false })
    const w = mount(AuthBanner, stubs)
    expect(w.get('[data-test="auth-banner"]').text()).toContain('Sign-in is not set up')
    expect(w.text()).toContain('not encrypted')
  })

  it('is gone once sign-in is set up, or before the state is known', () => {
    useAuth().status.value = status({ required: true })
    expect(mount(AuthBanner, stubs).find('[data-test="auth-banner"]').exists()).toBe(false)
    useAuth().status.value = null
    expect(mount(AuthBanner, stubs).find('[data-test="auth-banner"]').exists()).toBe(false)
  })
})

describe('SignInSettings', () => {
  const session = (over = {}) => ({ id: 's1', browser: 'Firefox on Mac', address: '10.0.0.2', created_at: '2026-10-29T08:00:00Z', last_seen: '2026-10-29T09:00:00Z', current: false, ...over })

  it('explains how to set up when sign-in is not set up, and loads nothing', () => {
    useAuth().status.value = status({ required: false })
    const w = mount(SignInSettings, stubs)
    expect(w.get('[data-test="not-set-up"]').text()).toContain('auth setup-code')
    expect(service.sessions).not.toHaveBeenCalled()
  })

  it('lists where you are signed in, marks this one, and ends another', async () => {
    service.sessions.mockResolvedValue([session({ id: 'me', current: true }), session({ id: 'other', browser: 'Safari on iPhone' })])
    const w = mount(SignInSettings, stubs)
    await flushPromises()
    const rows = w.findAll('[data-test="sessions"] li')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('this one')
    expect(rows[0]!.findAll('button')).toHaveLength(0)
    service.revokeSession.mockResolvedValue(undefined)
    await rows[1]!.get('button').trigger('click')
    await flushPromises()
    expect(service.revokeSession).toHaveBeenCalledWith('other')
  })

  it('signs out everywhere after confirming, and lands on the login page', async () => {
    service.sessions.mockResolvedValue([session({ current: true })])
    service.revokeAllSessions.mockResolvedValue(undefined)
    const w = mount(SignInSettings, stubs)
    await flushPromises()
    await w.get('[data-test="sign-out-everywhere"]').trigger('click')
    await flushPromises()
    expect(confirmMock).toHaveBeenCalled()
    expect(service.revokeAllSessions).toHaveBeenCalled()
    expect(replace).toHaveBeenCalledWith({ name: 'login' })
  })

  it('does nothing when the confirmation is declined', async () => {
    confirmMock.mockResolvedValue(false)
    service.sessions.mockResolvedValue([session({ current: true })])
    const w = mount(SignInSettings, stubs)
    await flushPromises()
    await w.get('[data-test="sign-out-everywhere"]').trigger('click')
    await flushPromises()
    expect(service.revokeAllSessions).not.toHaveBeenCalled()
  })

  it('makes a token, shows its secret once, and never lists it', async () => {
    service.createToken.mockResolvedValue({ id: 't1', name: 'eval scripts', scope: 'admin', token: 'lp_secretsecretsecret' })
    const w = mount(SignInSettings, stubs)
    await flushPromises()
    await w.get('input[placeholder="eval scripts"]').setValue('eval scripts')
    await w.get('form[class*="grid"]').trigger('submit')
    await flushPromises()
    expect(service.createToken).toHaveBeenCalledWith('eval scripts', 'admin')
    expect(w.get('[data-test="token-value"]').text()).toBe('lp_secretsecretsecret')
    service.tokens.mockResolvedValue([{ id: 't1', name: 'eval scripts', scope: 'admin', created_at: '2026-10-29T09:00:00Z' }])
    await w.findAll('button').find((b) => b.text() === 'I have copied it')!.trigger('click')
    await flushPromises()
    expect(w.find('[data-test="new-token"]').exists()).toBe(false)
    expect(w.text()).not.toContain('lp_secretsecretsecret')
  })

  it('revokes a token after confirming', async () => {
    service.tokens.mockResolvedValue([{ id: 't1', name: 'eval scripts', scope: 'admin', created_at: '2026-10-29T09:00:00Z' }])
    service.revokeToken.mockResolvedValue(undefined)
    const w = mount(SignInSettings, stubs)
    await flushPromises()
    await w.get('[data-test="tokens"] button').trigger('click')
    await flushPromises()
    expect(service.revokeToken).toHaveBeenCalledWith('t1')
  })

  it('changes the passphrase, asks again for a short one, and reports a refusal', async () => {
    const w = mount(SignInSettings, stubs)
    await flushPromises()
    const fields = w.findAll('input[autocomplete$="password"]')
    await fields[0]!.setValue(PASS)
    await fields[1]!.setValue('short')
    expect(w.text()).toContain('at least 12')
    expect(w.findAll('button[type="submit"]')[0]!.attributes('disabled')).toBeDefined()
    await fields[1]!.setValue('a brand new passphrase')
    service.changePassphrase.mockResolvedValue({})
    await w.findAll('form')[0]!.trigger('submit')
    await flushPromises()
    expect(service.changePassphrase).toHaveBeenCalledWith(PASS, 'a brand new passphrase')
    expect(w.find('[data-test="pass-done"]').exists()).toBe(true)

    service.changePassphrase.mockRejectedValue(new Error('That passphrase is not right.'))
    await fields[0]!.setValue(PASS)
    await fields[1]!.setValue('another new passphrase')
    await w.findAll('form')[0]!.trigger('submit')
    await flushPromises()
    expect(w.get('[data-test="action-error"]').text()).toContain('not right')
  })

  it('writes the sign-in log in words, newest first', async () => {
    service.events.mockResolvedValue([
      { at: '2026-10-29T08:00:00Z', kind: 'login', address: '10.0.0.2' },
      { at: '2026-10-29T09:00:00Z', kind: 'login_failed', address: '10.0.0.9' },
    ])
    const w = mount(SignInSettings, stubs)
    await flushPromises()
    const lines = w.findAll('[data-test="events"] li')
    expect(lines[0]!.text()).toContain('A wrong passphrase was tried')
    expect(lines[1]!.text()).toContain('Signed in')
  })
})
