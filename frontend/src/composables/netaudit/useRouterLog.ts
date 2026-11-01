import { ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { ROUTER_LOG_LOAD_FAILED, ROUTER_LOG_SAVE_FAILED } from '../../constants/netaudit'
import type { RouterLogState } from '../../types/netaudit'

/** The router log listener as the settings card shows it. A change answers with the whole new state, which replaces the old. */
export function useRouterLog() {
  const state = ref<RouterLogState | null>(null)
  const error = ref('')
  const busy = ref(false)

  async function load(): Promise<void> {
    try {
      state.value = await NetauditService.routerLog()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, ROUTER_LOG_LOAD_FAILED)
    }
  }

  async function set(enabled: boolean): Promise<void> {
    busy.value = true
    try {
      state.value = await NetauditService.setRouterLog(enabled)
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, ROUTER_LOG_SAVE_FAILED)
    } finally {
      busy.value = false
    }
  }

  return { state, error, busy, load, set }
}
