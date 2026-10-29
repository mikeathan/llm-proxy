import type { RouteLocationRaw } from 'vue-router'
import { ROUTE_NAMES } from '../../types/routes'
import type { AuthStatus } from '../../types/auth'

/** Where to send someone after signing in when the page they wanted is unknown. */
const HOME_PATH = '/'

/**
 * A page to return to after signing in. Only a path inside this app counts: a `next` that names another site, a
 * protocol-relative address, or the sign-in pages themselves is dropped, so a crafted link cannot send the user elsewhere.
 */
export function safeNext(next: unknown): string {
  if (typeof next !== 'string' || !next.startsWith('/') || next.startsWith('//') || next.includes('\\')) return HOME_PATH
  const path = next.split(/[?#]/)[0] ?? ''
  if (path === '/login' || path === '/setup' || path.startsWith('/login/') || path.startsWith('/setup/')) return HOME_PATH
  return next
}

/**
 * What the router does with a navigation, given what the server said about sign-in. `true` lets it through.
 * Signed in, or sign-in not set up: the sign-in pages have nothing to offer and send you home. Sign-in required and not
 * signed in: everything goes to the login page, remembering where it was going. `setup` needs an open setup code.
 */
export function decideRoute(status: AuthStatus, to: { name?: unknown; meta: { public?: boolean }; fullPath: string }): RouteLocationRaw | true {
  const open = !status.required || status.authenticated
  if (to.meta.public) {
    if (to.name === ROUTE_NAMES.setup) return status.required ? { name: open ? ROUTE_NAMES.overview : ROUTE_NAMES.login } : status.setup_open ? true : { name: ROUTE_NAMES.overview }
    return open ? { path: HOME_PATH } : true
  }
  if (open) return true
  return { name: ROUTE_NAMES.login, query: { next: to.fullPath } }
}
