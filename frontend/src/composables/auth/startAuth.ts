import type { Router } from 'vue-router'
import { ROUTE_NAMES } from '../../types/routes'
import { setReauthHandler, setUnauthorizedHandler } from '../../services/httpClient'
import { decideRoute } from '../../utils/auth/guard'
import { useAuth } from './useAuth'
import { useReauth } from './useReauth'

/**
 * Wires sign-in into the app, once, from main.ts (the one place app-level lifecycle lives): the router guard that sends
 * someone who must sign in to the login page, what happens when the server says the session ended, and the prompt for the
 * passphrase when a destructive action asks for it.
 */
export function startAuth(router: Router): void {
  const auth = useAuth()
  setUnauthorizedHandler(() => {
    auth.markSignedOut()
    const here = router.currentRoute.value
    if (here.meta.public) return
    void router.replace({ name: ROUTE_NAMES.login, query: { next: here.fullPath } })
  })
  setReauthHandler(useReauth().requestReauth)
  router.beforeEach(async (to) => {
    const status = await auth.ensureStatus()
    return status ? decideRoute(status, to) : true
  })
}
