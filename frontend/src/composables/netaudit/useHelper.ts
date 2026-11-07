import { ref } from 'vue'
import { AdminApiService } from '../../services/admin/adminService'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { HELPER_LOAD_FAILED, HELPER_SAVE_FAILED } from '../../constants/netaudit'
import type { HelperStatus } from '../../types/netaudit'

/**
 * The privileged helper as its settings card shows it: where it stands, and the operator's switch per action. A switch is the host
 * setting sandboxing.helper_actions, which needs the passphrase again (the page asks for it); the whole host settings are read and written
 * back with only that list changed, so nothing else in them moves. The status is read again afterwards.
 */
export function useHelper() {
  const status = ref<HelperStatus | null>(null)
  const error = ref('')
  const busy = ref(false)

  async function load(): Promise<void> {
    try {
      status.value = await NetauditService.helper()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, HELPER_LOAD_FAILED)
    }
  }

  async function setAction(id: string, on: boolean): Promise<void> {
    busy.value = true
    try {
      const host = await AdminApiService.fetchHostSettings()
      const current = host.sandboxing.helper_actions ?? []
      const next = on ? [...new Set([...current, id])] : current.filter((a) => a !== id)
      // The read-time projection is not part of what is saved.
      const { effective: _effective, ...saved } = host
      await AdminApiService.updateHostSettings({ ...saved, sandboxing: { ...host.sandboxing, helper_actions: next } })
      await load()
      error.value = ''
    } catch (err) {
      // Read the real state again first: a successful read clears the error, and this one must stay on screen.
      const failure = errorMessage(err, HELPER_SAVE_FAILED)
      await load()
      error.value = failure
    } finally {
      busy.value = false
    }
  }

  return { status, error, busy, load, setAction }
}
