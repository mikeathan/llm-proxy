import { ref } from 'vue'
import { AuthService } from '../../services/auth/authService'
import { errorMessage } from '../../utils/errors'
import { useAuth } from './useAuth'
import type { AuthEvent, AuthSession, AuthToken, NewAuthToken, TokenScope } from '../../types/auth'

const LOAD_FAILED = 'The sign-in details could not be read.'
const PASSPHRASE_FAILED = 'The passphrase could not be changed.'
const REVOKE_SESSION_FAILED = 'That session could not be ended.'
const SIGN_OUT_ALL_FAILED = 'Signing out everywhere failed.'
const TOKEN_CREATE_FAILED = 'The token could not be made.'
const TOKEN_REVOKE_FAILED = 'The token could not be revoked.'

/** Settings · Sign-in: where this operator is signed in, their script tokens, the recent sign-in log, and the passphrase. */
export function useSignInSettings() {
  const auth = useAuth()
  const sessions = ref<AuthSession[]>([])
  const tokens = ref<AuthToken[]>([])
  const events = ref<AuthEvent[]>([])
  const loading = ref(false)
  const error = ref('')
  const actionError = ref('')
  /** A token just made; its secret is shown once and then forgotten. */
  const created = ref<NewAuthToken | null>(null)

  async function load(): Promise<void> {
    loading.value = true
    try {
      ;[sessions.value, tokens.value, events.value] = await Promise.all([AuthService.sessions(), AuthService.tokens(), AuthService.events()])
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, LOAD_FAILED)
    } finally {
      loading.value = false
    }
  }

  async function act(run: () => Promise<unknown>, failed: string): Promise<boolean> {
    try {
      await run()
      actionError.value = ''
    } catch (err) {
      actionError.value = errorMessage(err, failed)
      return false
    }
    await load()
    return true
  }

  const changePassphrase = (oldPassphrase: string, newPassphrase: string) =>
    act(() => AuthService.changePassphrase(oldPassphrase, newPassphrase), PASSPHRASE_FAILED)

  const revokeSession = (id: string) => act(() => AuthService.revokeSession(id), REVOKE_SESSION_FAILED)

  /** Ends every session, this one too: the browser lands on the login page. */
  async function signOutEverywhere(): Promise<boolean> {
    try {
      await AuthService.revokeAllSessions()
    } catch (err) {
      actionError.value = errorMessage(err, SIGN_OUT_ALL_FAILED)
      return false
    }
    await auth.refresh()
    return true
  }

  async function createToken(name: string, scope: TokenScope): Promise<boolean> {
    try {
      created.value = await AuthService.createToken(name, scope)
      actionError.value = ''
    } catch (err) {
      actionError.value = errorMessage(err, TOKEN_CREATE_FAILED)
      return false
    }
    await load()
    return true
  }

  const revokeToken = (id: string) => act(() => AuthService.revokeToken(id), TOKEN_REVOKE_FAILED)
  const dismissCreated = () => { created.value = null }

  return { sessions, tokens, events, loading, error, actionError, created, load, changePassphrase, revokeSession, signOutEverywhere, createToken, revokeToken, dismissCreated }
}
