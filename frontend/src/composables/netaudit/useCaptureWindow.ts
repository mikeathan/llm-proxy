import { computed, ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { CAPTURE_FAILED } from '../../constants/netaudit'
import type { CaptureSummary, HelperStatus } from '../../types/netaudit'

/**
 * The capture_window listen (the privileged helper): offered only when the helper is ready and the action is on and in the helper's own
 * list; each press asks for the passphrase (the server's step-up). The last summary comes from the server, so it shows after a reload. It
 * only reports what one link heard for a few seconds and makes no finding.
 */
export function useCaptureWindow() {
  const status = ref<HelperStatus | null>(null)
  const summary = ref<CaptureSummary | null>(null)
  const loaded = ref(false)
  const running = ref(false)
  const error = ref('')

  const available = computed(() => {
    const st = status.value
    return Boolean(st?.ready && st.actions.some((a) => a.id === 'capture_window' && a.on && a.installed !== false))
  })

  // A failed read of the helper's state offers nothing: the Network settings card is where its trouble is shown.
  async function load(): Promise<void> {
    try {
      status.value = await NetauditService.helper()
    } catch {
      status.value = null
    }
    try {
      summary.value = await NetauditService.lastCapture()
    } catch {
      summary.value = null
    }
    loaded.value = true
  }

  /** Listens once; true when the server answered with a summary it could read. */
  async function listen(): Promise<boolean> {
    if (running.value) return false
    running.value = true
    error.value = ''
    try {
      const answer = await NetauditService.captureWindow()
      if (!answer.parsed) {
        error.value = answer.parse_error || CAPTURE_FAILED
        return false
      }
      // The server kept it with the time and the interface; read that copy so the page shows what a reload would.
      summary.value = (await NetauditService.lastCapture()) ?? answer.parsed
      return true
    } catch (err) {
      error.value = errorMessage(err, CAPTURE_FAILED)
      return false
    } finally {
      running.value = false
    }
  }

  return { available, summary, loaded, running, error, load, listen }
}
