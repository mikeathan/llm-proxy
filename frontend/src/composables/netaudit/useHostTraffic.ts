import { computed, ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import {
  TRAFFIC_ACTION_NOT_INSTALLED, TRAFFIC_ACTION_OFF, TRAFFIC_FAILED, TRAFFIC_LOAD_FAILED, TRAFFIC_NOT_INSTALLED, TRAFFIC_NOT_READY, TRAFFIC_NOT_READY_FALLBACK,
  TRAFFIC_STATUS_UNKNOWN, TRAFFIC_UNSUPPORTED,
} from '../../constants/netaudit'
import type { HelperStatus, HostTraffic, TrafficAvailability } from '../../types/netaudit'

const ACTION = 'capture_host'
const OK: TrafficAvailability = { ok: true, reason: '' }

// A look runs for up to a minute, longer than a visit to this tab: its state is shared by every use of the composable, so a look started
// before leaving the tab is still shown as running when the tab is opened again, and its result lands in the tab that is open when it ends.
const summary = ref<HostTraffic | null>(null)
const running = ref(false)
const error = ref('')

/** Why the look cannot be pressed with the helper in this state, and the page that fixes it; ok when it can. */
function availabilityOf(status: HelperStatus | null): TrafficAvailability {
  if (!status) return { ok: false, reason: TRAFFIC_STATUS_UNKNOWN }
  if (!status.supported) return { ok: false, reason: TRAFFIC_UNSUPPORTED }
  if (!status.installed) return { ok: false, reason: TRAFFIC_NOT_INSTALLED, fixTab: 'settings' }
  if (!status.ready) return { ok: false, reason: TRAFFIC_NOT_READY(status.not_ready || TRAFFIC_NOT_READY_FALLBACK), fixTab: 'settings' }
  const action = status.actions.find((a) => a.id === ACTION)
  if (!action || !action.on) return { ok: false, reason: TRAFFIC_ACTION_OFF, fixTab: 'settings' }
  if (action.installed === false) return { ok: false, reason: TRAFFIC_ACTION_NOT_INSTALLED, fixTab: 'settings' }
  return OK
}

/**
 * The capture_host look at this computer's own traffic (the privileged helper): offered only when the helper is ready and the action is on
 * and in the helper's own list; each press asks for the passphrase (the server's step-up). The last summary comes from the server, so it
 * shows after a reload. It only reports what one link carried for a few seconds and makes no finding.
 */
export function useHostTraffic() {
  const status = ref<HelperStatus | null>(null)
  const loaded = ref(false)
  const loadError = ref('')

  const availability = computed(() => availabilityOf(status.value))

  // A failed read of the helper's state offers nothing and says so; a failed read of the last look leaves none and says so.
  async function load(): Promise<void> {
    loadError.value = ''
    try {
      status.value = await NetauditService.helper()
    } catch {
      status.value = null
    }
    try {
      const last = await NetauditService.lastHostTraffic()
      // A look still running will replace this with its own result; do not show an older one over it.
      if (!running.value) summary.value = last
    } catch (err) {
      if (!running.value) summary.value = null
      loadError.value = errorMessage(err, TRAFFIC_LOAD_FAILED)
    }
    loaded.value = true
  }

  /** Listens once; true when the server answered with a summary it could read. */
  async function listen(): Promise<boolean> {
    if (running.value) return false
    running.value = true
    error.value = ''
    try {
      const answer = await NetauditService.captureHost()
      if (!answer.parsed) {
        error.value = answer.parse_error || TRAFFIC_FAILED
        return false
      }
      // The server kept it with the time and the interface; read that copy so the page shows what a reload would.
      summary.value = (await NetauditService.lastHostTraffic()) ?? answer.parsed
      return true
    } catch (err) {
      error.value = errorMessage(err, TRAFFIC_FAILED)
      return false
    } finally {
      running.value = false
    }
  }

  return { availability, summary, loaded, running, error, loadError, load, listen }
}
