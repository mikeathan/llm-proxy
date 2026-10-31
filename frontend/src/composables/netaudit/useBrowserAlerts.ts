import { ref, type Ref } from 'vue'
import type { BrowserAlertsEnv, NetworkAlertItem } from '../../types/notifications'
import { summarizeBatch } from '../../utils/notifications/present'
import { usePersistedState } from '../ui/usePersistedState'

// A browser notice for a Network alert raised while this tab is hidden, if the person turned it on in this browser. It is
// opt-in: the browser is asked for permission only when the person turns the switch on, never on load. It carries the same
// fixed text as the bell and works while a tab is open, hidden or in the background; it does not work once the browser is
// closed (no push service is used).

const NOTICE_TITLE = 'Network alert'
const PROBLEM_UNSUPPORTED = 'This browser does not support notifications.'
const PROBLEM_RAISE_FAILED = 'This browser cannot show notices from a web page (some phone browsers only allow them from an installed app).'
const PROBLEM_BLOCKED = 'The browser blocked notifications for this site. Allow them in the browser\'s site settings, then turn this on again.'

export function createBrowserAlerts(env: BrowserAlertsEnv, enabled: Ref<boolean>) {
  // A browser without notifications says so from the start, so the switch is never a silent no-op.
  const problem = ref(env.supported ? '' : PROBLEM_UNSUPPORTED)

  async function enable(): Promise<void> {
    problem.value = ''
    if (!env.supported) {
      enabled.value = false
      problem.value = PROBLEM_UNSUPPORTED
      return
    }
    const permission = env.permission() === 'default' ? await env.requestPermission() : env.permission()
    enabled.value = permission === 'granted'
    if (!enabled.value) problem.value = PROBLEM_BLOCKED
  }

  function disable() {
    problem.value = ''
    enabled.value = false
  }

  /** Raises one notice for a batch of new alerts, when the tab is hidden and the person allowed it. */
  function announce(items: NetworkAlertItem[]) {
    if (!items.length || !enabled.value || !env.supported || env.permission() !== 'granted' || !env.hidden()) return
    // One tag per repeating alert, so a notice for the same alert replaces the earlier one.
    try {
      env.raise(NOTICE_TITLE, { body: summarizeBatch(items), tag: items.length === 1 ? items[0]!.repeatKey : undefined })
    } catch {
      // Some browsers (Chrome on Android) permit notifications but throw when a page constructs one. The bell is unaffected.
      problem.value = PROBLEM_RAISE_FAILED
    }
  }

  return { enabled, problem, supported: env.supported, enable, disable, announce }
}

type BrowserAlerts = ReturnType<typeof createBrowserAlerts>

const ENABLED_KEY = 'netaudit.browser-alerts'
const ENABLED_VERSION = 1

let instance: BrowserAlerts | null = null

/** The per-browser switch and notices (one for the app: the bootstrap announces through it and the Network page switches it). */
export function useBrowserAlerts(): BrowserAlerts {
  if (instance) return instance
  const supported = typeof Notification !== 'undefined'
  const enabled = usePersistedState<boolean>(ENABLED_KEY, { version: ENABLED_VERSION, fallback: false, parse: (value) => value === true })
  instance = createBrowserAlerts({
    supported,
    permission: () => (supported ? Notification.permission : 'denied'),
    requestPermission: () => Notification.requestPermission(),
    raise: (title, options) => { new Notification(title, options) },
    hidden: () => document.hidden,
  }, enabled)
  return instance
}
