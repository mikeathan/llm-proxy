import type { Router } from 'vue-router'
import { ROUTE_NAMES } from '../../types/routes'
import { setReauthHandler, setUnauthorizedHandler } from '../../services/httpClient'
import { decideRoute } from '../../utils/auth/guard'
import { useAuth } from './useAuth'
import { useReauth } from './useReauth'

/**
 * Wires sign-in into the app, once, from main.ts (the one place app-level lifecycle lives): the router guard that sends
 * someone who must sign in to the login page, what happens when the server says the session ended or there is nothing to sign in with, and the prompt for the
 * passphrase when a destructive action asks for it.
 */
export function startAuth(router: Router): void {
  const auth = useAuth()
  setUnauthorizedHandler(() => {
    const here = router.currentRoute.value
    if (here.meta.public) return
    // Ask the server why: the session ended (sign in again, and come back here), or the sign-in was reset under the page
    // and there is no passphrase any more (set it up). The state it keeps is replaced, not guessed.
    void auth.refresh().then((st) => router.replace(st && !st.set_up ? { name: ROUTE_NAMES.setup } : { name: ROUTE_NAMES.login, query: { next: here.fullPath } }))
  })
  setReauthHandler(useReauth().requestReauth)
  router.beforeEach(async (to) => {
    // The sign-in pages ask the server again: what it said at start (no setup code yet, no passphrase yet) goes stale
    // while the page is open, and a setup page that redirects home would leave no place to enter the code.
    const status = to.meta.public ? await auth.refresh() : await auth.ensureStatus()
    return status ? decideRoute(status, to) : true
  })
}
