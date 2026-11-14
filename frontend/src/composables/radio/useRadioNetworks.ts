import { ref } from 'vue'
import { RadioService } from '../../services/radio/radioService'
import { errorMessage } from '../../utils/errors'
import { RADIO_NETWORKS_FAILED } from '../../constants/radio'
import type { RadioNetworks } from '../../types/radio'

/** Radio · Your networks: the names of the owner's own Wi-Fi, and the access points that broadcast them until the owner confirms which are theirs. */
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
    addName: (name: string) => run(() => RadioService.setNetworkNames([...(networks.value?.names ?? []), name])),
    removeName: (name: string) => run(() => RadioService.setNetworkNames((networks.value?.names ?? []).filter((n) => n !== name))),
    confirm: (id: string) => run(() => RadioService.confirmNetwork(id)),
    reject: (id: string) => run(() => RadioService.dropNetwork(id)),
  }
}
