import { ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import { errorMessage } from '../../utils/errors'
import { RADIO_NETWORKS_FAILED } from '../../constants/radio'
import type { RadioNetworks } from '../../types/radio'

/**
 * Radio · Your networks: the names of the owner's own Wi-Fi and the access points heard broadcasting them. The names and which access points
 * are the owner's are set on the Network page (the one list); here only the guest flag, Radio's own evidence rule, is changed.
 */
export function useRadioNetworks() {
  const networks = ref<RadioNetworks | null>(null)
  const error = ref('')
  const busy = ref(false)

  async function run(work: () => Promise<unknown>): Promise<void> {
    busy.value = true
    try {
      await work()
      networks.value = await RadioService.networks()
      error.value = ''
    } catch (err) {
      error.value = errorMessage(err, RADIO_NETWORKS_FAILED)
    } finally {
      busy.value = false
    }
  }

  return {
    networks, error, busy,
    load: () => run(async () => {}),
    setGuest: (id: string, guest: boolean) => run(() => RadioService.setGuest(id, guest)),
  }
}
