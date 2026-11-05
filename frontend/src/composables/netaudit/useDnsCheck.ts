import { ref } from 'vue'
import { NetauditService } from '../../services/netaudit/netauditService'
import { errorMessage } from '../../utils/errors'
import { DNS_CHECK_LOAD_FAILED, DNS_CHECK_SAVE_FAILED } from '../../constants/netaudit'
import type { DnsCheckState, DnsCheckUpdate } from '../../types/netaudit'

/** The DNS check as its card shows it. A change answers with the whole new state, which replaces the old. */
export function useDnsCheck() {
  const state = ref<DnsCheckState | null>(null)
  const error = ref('')
  const busy = ref(false)

  async function load(): Promise<void> {
    try {
      state.value = await NetauditService.dnsCheck()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, DNS_CHECK_LOAD_FAILED)
    }
  }

  async function save(update: DnsCheckUpdate): Promise<boolean> {
    busy.value = true
    try {
      state.value = await NetauditService.setDnsCheck(update)
      error.value = ''
      return true
    } catch (err) {
      error.value = errorMessage(err, DNS_CHECK_SAVE_FAILED)
      return false
    } finally {
      busy.value = false
    }
  }

  return { state, error, busy, load, save }
}
