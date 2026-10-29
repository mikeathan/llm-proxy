import { ref } from 'vue'
import { AuthService } from '../../services/auth/authService'
import { signalUnauthorized } from '../../services/httpClient'
import type { AuthStatus } from '../../types/auth'

// What the server last said about sign-in. One copy for the whole app: the router guard, the pages and the banner read it.
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

/** The server said the session ended (a 401): the page must show the login, whatever it thought before. */
/**
 * For a stream that failed without saying why (an EventSource cannot see a status code): asks the server whether the session
 * is the problem, and if so sends the user to sign in, instead of retrying for ever.
 */
async function checkSession(): Promise<void> {
  const st = await refresh()
  if (st && st.required && !st.authenticated) signalUnauthorized()
}

function markSignedOut(): void {
  if (status.value) status.value = { ...status.value, required: true, authenticated: false, elevated: false }
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
  return { status, refresh, ensureStatus, checkSession, markSignedOut, login, setup, logout }
}
