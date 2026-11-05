import { onMounted, ref } from 'vue'
import { errorMessage } from '../../utils/errors'
import type { NetauditSwitch } from '../../types/netaudit'

/**
 * Whether one on/off setting of the network feature is on, read when it mounts, so a detail card can say so. The setting is
 * switched on its stack's Modules tab and nowhere else. `initial` is what is shown until the server answers.
 */
export function useSwitchState(read: () => Promise<NetauditSwitch>, loadMessage: string, initial = false) {
  const enabled = ref(initial)
  const error = ref('')

  async function load(): Promise<void> {
    try {
      enabled.value = (await read()).enabled
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, loadMessage)
    }
  }

  onMounted(load)
  return { enabled, error, load }
}
