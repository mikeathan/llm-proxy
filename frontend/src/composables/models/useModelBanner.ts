import { watch } from 'vue'
import { useModels } from './useModels'
import { useAppBanner } from '../ui/useAppBanner'
import { computeModelBanner } from './modelBanner'
import type { ModelBanner } from './modelBanner'

// Owns the model-configuration warning logic and is the canonical emitter of
// that message to the main app banner. Any other UI logic that needs to surface
// a persistent warning follows the same pattern: derive from state, then show a
// fully-formed AppBannerMessage on the shared bus.
//
// Precedence is handled here (component level), not by the bus: a transient
// (non-persistent) error may temporarily override this warning, and when that
// error clears we re-assert the warning if it is still unresolved.

const { state } = useModels()
const { active: bannerActive, show: showBanner, clear: clearBanner } = useAppBanner()

// `lastShownKey` is the key of the message we last pushed to the bus (our own
// persistent warning). While a transient (non-persistent) error is showing we
// reset it to '' so that when the error clears we re-assert the warning (which
// may have changed while it was covered).
let lastShownKey = ''

function warningKey(w: ModelBanner | null): string {
  return w ? `${w.severity}|${w.message}` : ''
}

// Push the current model warning to the bus, but never clobber a visible
// transient (non-persistent) error. When a transient is showing we back off and
// reset lastShownKey; when it clears we re-assert the warning if still
// unresolved.
function syncModelBanner(): void {
  const warning = computeModelBanner(state.value)
  const key = warningKey(warning)

  const current = bannerActive.value
  if (current && !current.persistent) {
    // A transient error is showing: don't overwrite it. Reset lastShownKey so
    // that when the error clears we re-assert the warning (which may have
    // changed while it was covered).
    lastShownKey = ''
    return
  }

  if (key === lastShownKey) return // no change
  lastShownKey = key
  if (warning) {
    showBanner({
      severity: warning.severity,
      message: warning.message,
      html: warning.html,
      persistent: true,
      action: warning.action,
    })
  } else {
    clearBanner()
  }
}

// The watchers are registered once at module load (like the other singleton
// composables in this codebase), so there is no per-component startup wiring to
// remember. State is null initially, so the immediate sync is a no-op; the
// first real refresh (on app startup) populates state and the state watch fires.
// Watchers are async-flushed and cheap (a single scan of the model list), so
// there is no timing risk or performance concern.
watch(state, syncModelBanner)
watch(bannerActive, syncModelBanner)
syncModelBanner()

export function useModelBanner() {
  return { recompute: syncModelBanner }
}
