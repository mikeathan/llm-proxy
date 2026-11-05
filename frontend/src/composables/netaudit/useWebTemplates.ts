import { ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { WEB_TEMPLATES_DOWNLOAD_FAILED, WEB_TEMPLATES_LOAD_FAILED } from '../../constants/netaudit'
import type { WebTemplatesState } from '../../types/netaudit'

/** The reviewed web-check templates as their card shows them. A download answers with the new status, which replaces the old. */
export function useWebTemplates() {
  const state = ref<WebTemplatesState | null>(null)
  const error = ref('')
  const busy = ref(false)

  async function load(): Promise<void> {
    try {
      state.value = await NetauditService.webTemplates()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, WEB_TEMPLATES_LOAD_FAILED)
    }
  }

  async function download(): Promise<void> {
    busy.value = true
    try {
      state.value = await NetauditService.downloadWebTemplates()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, WEB_TEMPLATES_DOWNLOAD_FAILED)
    } finally {
      busy.value = false
    }
  }

  return { state, error, busy, load, download }
}
