import type { Router } from 'vue-router'
import type { StaleChunkReloadDeps } from '../types/routes'
import { browserStorage, persistedKey } from '../composables/ui/usePersistedState'

// Every page is a lazy chunk with a content hash in its name. After a deploy, a tab still running the old build
// asks for chunks the server no longer has, and a navigation would fail silently. When that happens the app
// reloads once onto the new build. A short guard stops a broken deploy from reloading in a loop.

// The browser wording for a dynamic import that could not be fetched (Chromium, Safari, Firefox).
const CHUNK_ERROR_PATTERNS = [
  /Failed to fetch dynamically imported module/i,
  /Importing a module script failed/i,
  /error loading dynamically imported module/i,
]
const RELOAD_GUARD_KEY = persistedKey('stale-chunk-reload-at')
const RELOAD_GUARD_MS = 10_000
const VITE_PRELOAD_ERROR = 'vite:preloadError'

export function isChunkLoadError(err: unknown): boolean {
  return err instanceof Error && CHUNK_ERROR_PATTERNS.some((pattern) => pattern.test(err.message))
}

function reloadOnce(deps: StaleChunkReloadDeps, href: string) {
  const last = Number(deps.storage?.getItem(RELOAD_GUARD_KEY) ?? 0)
  if (deps.now() - last < RELOAD_GUARD_MS) return
  try {
    deps.storage?.setItem(RELOAD_GUARD_KEY, String(deps.now()))
  } catch {
    // Blocked storage: reload anyway; the guard is best effort.
  }
  deps.navigate(href)
}

function browserDeps(): StaleChunkReloadDeps {
  return {
    now: Date.now,
    storage: browserStorage(),
    navigate: (href) => window.location.assign(href),
    currentHref: () => window.location.pathname + window.location.search + window.location.hash,
    addWindowListener: (type, listener) => window.addEventListener(type, listener),
  }
}

/** Wires the reload into the router (a page chunk that fails to load) and Vite (a preloaded dependency). */
export function installStaleChunkReload(router: Router, deps: StaleChunkReloadDeps = browserDeps()) {
  router.onError((err, to) => {
    if (isChunkLoadError(err)) reloadOnce(deps, router.resolve(to).href)
  })
  deps.addWindowListener(VITE_PRELOAD_ERROR, (event) => {
    event.preventDefault()
    reloadOnce(deps, deps.currentHref())
  })
}
