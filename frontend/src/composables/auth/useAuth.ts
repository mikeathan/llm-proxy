import { ref } from 'vue'
import { AuthService } from '../../services/auth/authService'
import { signalUnauthorized } from '../../services/httpClient'
import type { AuthStatus } from '../../types/auth'

// What the server last said about sign-in. One copy for the whole app: the router guard and the pages read it.
const status = ref<AuthStatus | null>(null)

/** Reads sign-in's state from the server. null when it could not be read: the pages then load, and the API says no itself. */
async function refresh(): Promise<AuthStatus | null> {
  try {
    status.value = await AuthService.status()
  } catch {
    status.value = null
  }
  return status.value
}

/** The state, read once on first use. */
async function ensureStatus(): Promise<AuthStatus | null> {
  return status.value ?? refresh()
}

/**
 * For a stream that failed without saying why (an EventSource cannot see a status code): asks the server whether the session
 * is the problem (not signed in, or sign-in reset under it), and if so sends the user to sign in or set up, instead of
 * retrying for ever.
 */
async function checkSession(): Promise<void> {
  const st = await refresh()
  if (st && (!st.set_up || !st.authenticated)) signalUnauthorized()
}

async function login(passphrase: string): Promise<void> {
  await AuthService.login(passphrase)
  await refresh()
}

async function setup(code: string, passphrase: string): Promise<void> {
  await AuthService.setup(code, passphrase)
  await refresh()
}

async function logout(): Promise<void> {
  try {
    await AuthService.logout()
  } finally {
    await refresh()
  }
}

export function useAuth() {
  return { status, refresh, ensureStatus, checkSession, login, setup, logout }
}
