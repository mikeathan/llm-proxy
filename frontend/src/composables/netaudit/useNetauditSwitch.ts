import { onMounted, ref } from 'vue'
import { errorMessage } from '../../utils/errors'
import type { NetauditSwitch } from '../../types/netaudit'

/**
 * One on/off setting of the network feature (the weekly summary, asking devices their names): read when it mounts, saved
 * at once, and shown as the server last confirmed it. `read` and `write` are the service calls; the two messages are the
 * words for a failure of each.
 */
export function useNetauditSwitch(
  read: () => Promise<NetauditSwitch>,
  write: (enabled: boolean) => Promise<NetauditSwitch>,
  messages: { load: string; save: string },
  initial = false,
) {
  const enabled = ref(initial)
  const error = ref('')
  const busy = ref(false)

  async function load(): Promise<void> {
    try {
      enabled.value = (await read()).enabled
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, messages.load)
    }
  }

  async function set(value: boolean): Promise<void> {
    busy.value = true
    try {
      enabled.value = (await write(value)).enabled
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, messages.save)
    } finally {
      busy.value = false
    }
  }

  onMounted(load)
  return { enabled, error, busy, load, set }
}
