import { ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import type { RadioPresenceDevice } from '../../types/radio'

/** Radio · My devices: where each of the owner's devices has been, hour by hour. A failed read leaves the strip out; the list of devices still works. */
export function useRadioPresence() {
  const devices = ref<RadioPresenceDevice[]>([])

  async function load(): Promise<void> {
    try {
      devices.value = (await RadioService.presence()).devices
    } catch {
      devices.value = []
    }
  }

  return { devices, load }
}
